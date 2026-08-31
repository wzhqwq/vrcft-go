package userconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"
)

var (
	ErrConflict           = errors.New("userconfig: settings conflict")
	ErrRevisionExhausted  = errors.New("userconfig: settings revision exhausted")
	ErrInvalidLoadedState = errors.New("userconfig: invalid loaded state")
)

// DocumentToken identifies the exact authoritative document observed by a
// Loaded value. Its fields intentionally remain private so callers cannot
// manufacture a successful optimistic-concurrency check.
type DocumentToken struct {
	exists   bool
	hash     [sha256.Size]byte
	size     int64
	modified int64
	identity documentIdentity
}

// Loaded is an owned snapshot of one settings document. Invalid documents do
// not prevent the diagnostic shell from offering Defaults for an explicit
// repair, but are never overwritten during ordinary loading.
type Loaded struct {
	Settings   *Settings
	Defaults   Candidate
	Invalid    bool
	Diagnostic error
	Token      DocumentToken
}

// SaveResult reports the authoritative snapshot after a durable save.
type SaveResult struct {
	Loaded  Loaded
	Changed bool
}

type storeFile interface {
	io.Reader
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Stat() (os.FileInfo, error)
	Close() error
}

type storeOps struct {
	open          func(string) (storeFile, error)
	read          func(storeFile, []byte) (int, error)
	stat          func(storeFile) (os.FileInfo, error)
	mkdirAll      func(string, os.FileMode) error
	chmod         func(string, os.FileMode) error
	createTemp    func(string, string) (storeFile, error)
	remove        func(string) error
	replace       func(string, string) error
	syncDirectory func(string) error
}

// Store serializes document reads and writes so an in-process writer cannot
// race an optimistic fingerprint check.
type Store struct {
	paths Paths
	ops   storeOps
	lock  chan struct{}
}

type documentRead struct {
	data  []byte
	token DocumentToken
}

type preparedTemporary struct {
	path  string
	token DocumentToken
}

func NewStore(paths Paths) (*Store, error) {
	if strings.TrimSpace(paths.SettingsDir) == "" || strings.TrimSpace(paths.SettingsFile) == "" {
		return nil, errors.New("userconfig: settings paths are required")
	}
	if strings.IndexByte(paths.SettingsDir, 0) >= 0 || strings.IndexByte(paths.SettingsFile, 0) >= 0 {
		return nil, errors.New("userconfig: settings paths contain NUL")
	}
	if _, err := canonicalCandidate(DefaultCandidate(paths)); err != nil {
		return nil, fmt.Errorf("userconfig: invalid default settings: %w", err)
	}
	store := &Store{paths: paths, ops: defaultStoreOps(), lock: make(chan struct{}, 1)}
	store.lock <- struct{}{}
	return store, nil
}

func defaultStoreOps() storeOps {
	return storeOps{
		open:     func(path string) (storeFile, error) { return os.Open(path) },
		read:     func(file storeFile, data []byte) (int, error) { return file.Read(data) },
		stat:     func(file storeFile) (os.FileInfo, error) { return file.Stat() },
		mkdirAll: os.MkdirAll,
		chmod:    os.Chmod,
		createTemp: func(dir, pattern string) (storeFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		remove:  os.Remove,
		replace: replaceSettingsFile,
		syncDirectory: func(dir string) error {
			file, err := os.Open(dir)
			if err != nil {
				return err
			}
			defer file.Close()
			return file.Sync()
		},
	}
}

func (s *Store) LoadOrCreate(ctx context.Context) (Loaded, error) {
	if err := s.acquire(ctx); err != nil {
		return Loaded{}, err
	}
	defer s.release()

	current, err := s.readCurrent(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return s.createDefaults(ctx)
	}
	if err != nil {
		return Loaded{}, err
	}
	loaded, err := s.decodeLoaded(current)
	if err != nil {
		return s.invalidLoaded(current.token, err)
	}
	return loaded, nil
}

// Validate applies all decode-independent semantic checks without I/O.
func (s *Store) Validate(candidate Candidate) (Candidate, error) {
	return Normalize(candidate)
}

func canonicalCandidate(candidate Candidate) (Candidate, error) {
	normalized, err := Normalize(candidate)
	if err != nil {
		return Candidate{}, err
	}
	if normalized.Plugins.DevRoots == nil {
		normalized.Plugins.DevRoots = []string{}
	}
	if normalized.Processing.Overrides == nil {
		normalized.Processing.Overrides = []ProcessingOverride{}
	}
	if normalized.Processing.MutualExclusion == nil {
		normalized.Processing.MutualExclusion = [][]string{}
	}
	return normalized, nil
}

func (s *Store) Save(ctx context.Context, loaded Loaded, candidate Candidate) (SaveResult, error) {
	if err := s.acquire(ctx); err != nil {
		return SaveResult{}, err
	}
	defer s.release()
	if err := ctx.Err(); err != nil {
		return SaveResult{}, err
	}
	normalized, err := canonicalCandidate(candidate)
	if err != nil {
		return SaveResult{}, err
	}
	current, err := s.readCurrent(ctx)
	if err != nil {
		return SaveResult{}, fmt.Errorf("userconfig: read authoritative settings: %w", err)
	}
	if current.token != loaded.Token {
		return SaveResult{}, ErrConflict
	}
	authoritative, decodeErr := s.decodeLoaded(current)
	if decodeErr != nil {
		authoritative, err = s.invalidLoaded(current.token, decodeErr)
		if err != nil {
			return SaveResult{}, err
		}
	}
	if !authoritative.Invalid {
		currentCandidate, err := canonicalCandidate(candidateFromSettings(*authoritative.Settings))
		if err != nil {
			return SaveResult{}, fmt.Errorf("userconfig: validate loaded settings: %w", err)
		}
		if reflect.DeepEqual(normalized, currentCandidate) {
			return SaveResult{Loaded: cloneLoaded(authoritative), Changed: false}, nil
		}
		if authoritative.Settings.Revision == math.MaxUint64 {
			return SaveResult{}, ErrRevisionExhausted
		}
	}

	revision := uint64(1)
	if !authoritative.Invalid {
		revision = authoritative.Settings.Revision + 1
	}
	next := Settings{
		SchemaVersion: SchemaVersion,
		Revision:      revision,
		Avatar:        normalized.Avatar,
		Plugins:       normalized.Plugins,
		Processing:    normalized.Processing,
		OSC:           normalized.OSC,
	}
	encoded, err := encodeSettingsDocument(next)
	if err != nil {
		return SaveResult{}, err
	}

	if authoritative.Invalid {
		backupTemp, err := s.copyCurrentToTemporary(ctx, authoritative.Token, ".config-invalid-*.tmp")
		if err != nil {
			return SaveResult{}, err
		}
		backupPath := s.paths.SettingsFile + ".invalid.bak"
		if err := s.replaceTemporary(ctx, backupTemp.path, backupPath, authoritative.Token); err != nil {
			return SaveResult{}, err
		}
	}

	temporary, err := s.writeTemporary(ctx, encoded, ".config-*.tmp")
	if err != nil {
		return SaveResult{}, err
	}
	if err := s.replaceTemporary(ctx, temporary.path, s.paths.SettingsFile, authoritative.Token); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Loaded: Loaded{Settings: settingsPointer(next.Clone()), Defaults: s.defaults(), Token: temporary.token}, Changed: true}, nil
}

func (s *Store) createDefaults(ctx context.Context) (Loaded, error) {
	candidate := s.defaults()
	settings := Settings{SchemaVersion: SchemaVersion, Revision: 1, Avatar: candidate.Avatar, Plugins: candidate.Plugins, Processing: candidate.Processing, OSC: candidate.OSC}
	encoded, err := encodeSettingsDocument(settings)
	if err != nil {
		return Loaded{}, err
	}
	temporary, err := s.writeTemporary(ctx, encoded, ".config-*.tmp")
	if err != nil {
		return Loaded{}, err
	}
	if err := s.replaceTemporary(ctx, temporary.path, s.paths.SettingsFile, DocumentToken{}); err != nil {
		return Loaded{}, err
	}
	return Loaded{Settings: settingsPointer(settings.Clone()), Defaults: candidate, Token: temporary.token}, nil
}

func (s *Store) defaults() Candidate {
	defaults, err := canonicalCandidate(DefaultCandidate(s.paths))
	if err != nil {
		// Paths are validated before Store construction; this is only reachable
		// for a malformed caller-supplied DefaultOSCRoot.
		return DefaultCandidate(s.paths)
	}
	return defaults
}

func (s *Store) invalidLoaded(token DocumentToken, diagnostic error) (Loaded, error) {
	return Loaded{Defaults: s.defaults(), Invalid: true, Diagnostic: fmt.Errorf("userconfig: invalid settings: %w", diagnostic), Token: token}, nil
}

func (s *Store) readCurrent(ctx context.Context) (documentRead, error) {
	if err := ctx.Err(); err != nil {
		return documentRead{}, err
	}
	file, err := s.ops.open(s.paths.SettingsFile)
	if err != nil {
		return documentRead{}, err
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return documentRead{}, err
	}
	info, err := s.ops.stat(file)
	if err != nil {
		_ = file.Close()
		return documentRead{}, fmt.Errorf("userconfig: stat settings: %w", err)
	}
	identity, err := documentIdentityForFile(file, info)
	if err != nil {
		_ = file.Close()
		return documentRead{}, fmt.Errorf("userconfig: identify settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return documentRead{}, err
	}
	hash := sha256.New()
	data := make([]byte, 0, MaxSettingsBytes+1)
	buffer := make([]byte, 32<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return documentRead{}, err
		}
		count, readErr := s.ops.read(file, buffer)
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return documentRead{}, err
		}
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
			size += int64(count)
			remaining := MaxSettingsBytes + 1 - len(data)
			if remaining > 0 {
				if count > remaining {
					count = remaining
				}
				data = append(data, buffer[:count]...)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return documentRead{}, fmt.Errorf("userconfig: read settings: %w", readErr)
		}
	}
	if err := file.Close(); err != nil {
		return documentRead{}, fmt.Errorf("userconfig: close settings: %w", err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	return documentRead{data: data, token: documentToken(sum, size, info, identity)}, nil
}

func (s *Store) decodeLoaded(document documentRead) (Loaded, error) {
	if document.token.size > MaxSettingsBytes {
		return Loaded{}, fmt.Errorf("settings file exceeds %d bytes", MaxSettingsBytes)
	}
	if !utf8.Valid(document.data) {
		return Loaded{}, errors.New("settings file is not valid UTF-8")
	}
	if err := rejectDuplicateObjectKeys(document.data); err != nil {
		return Loaded{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(document.data))
	decoder.DisallowUnknownFields()
	var settings Settings
	if err := decoder.Decode(&settings); err != nil {
		return Loaded{}, fmt.Errorf("decode settings: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Loaded{}, errors.New("unexpected trailing JSON value")
		}
		return Loaded{}, fmt.Errorf("decode trailing settings: %w", err)
	}
	if settings.SchemaVersion != SchemaVersion {
		return Loaded{}, errors.New("unsupported schema version")
	}
	if settings.Revision == 0 {
		return Loaded{}, errors.New("revision must be positive")
	}
	normalized, err := canonicalCandidate(candidateFromSettings(settings))
	if err != nil {
		return Loaded{}, err
	}
	settings.Avatar = normalized.Avatar
	settings.Plugins = normalized.Plugins
	settings.Processing = normalized.Processing
	settings.OSC = normalized.OSC
	return Loaded{Settings: settingsPointer(settings.Clone()), Defaults: s.defaults(), Token: document.token}, nil
}

func candidateFromSettings(settings Settings) Candidate {
	return Candidate{Avatar: settings.Avatar, Plugins: settings.Plugins, Processing: settings.Processing, OSC: settings.OSC}.Clone()
}

func settingsPointer(settings Settings) *Settings { return &settings }

func cloneLoaded(loaded Loaded) Loaded {
	clone := loaded
	clone.Defaults = loaded.Defaults.Clone()
	if loaded.Settings != nil {
		clone.Settings = settingsPointer(loaded.Settings.Clone())
	}
	return clone
}

func documentToken(hash [sha256.Size]byte, size int64, info os.FileInfo, identity documentIdentity) DocumentToken {
	return DocumentToken{exists: true, hash: hash, size: size, modified: info.ModTime().UnixNano(), identity: identity}
}

func encodeSettingsDocument(settings Settings) ([]byte, error) {
	// JSON null is never a valid v1 value. Keep all required repeated fields
	// explicit on disk even when their in-memory zero value is nil.
	wire := settings.Clone()
	if wire.Plugins.DevRoots == nil {
		wire.Plugins.DevRoots = []string{}
	}
	if wire.Processing.Overrides == nil {
		wire.Processing.Overrides = []ProcessingOverride{}
	}
	if wire.Processing.MutualExclusion == nil {
		wire.Processing.MutualExclusion = [][]string{}
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("userconfig: encode settings: %w", err)
	}
	if len(data) > MaxSettingsBytes {
		return nil, fmt.Errorf("userconfig: encoded settings exceed %d bytes", MaxSettingsBytes)
	}
	return data, nil
}

func rejectDuplicateObjectKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON token %v", token)
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("null values are not permitted")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func (s *Store) writeTemporary(ctx context.Context, data []byte, pattern string) (preparedTemporary, error) {
	temporary, path, cleanup, err := s.newTemporary(ctx, pattern)
	if err != nil {
		return preparedTemporary{}, err
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	if err := writeAll(ctx, temporary, data); err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: write temporary settings: %w", err)
	}
	hash := sha256.Sum256(data)
	prepared, err := s.finishTemporary(ctx, temporary, path, hash, int64(len(data)))
	if err != nil {
		return preparedTemporary{}, err
	}
	keep = true
	return prepared, nil
}

func (s *Store) newTemporary(ctx context.Context, pattern string) (storeFile, string, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
	}
	if err := s.ops.mkdirAll(s.paths.SettingsDir, 0o700); err != nil {
		return nil, "", nil, fmt.Errorf("userconfig: create settings directory: %w", err)
	}
	if err := s.ops.chmod(s.paths.SettingsDir, 0o700); err != nil {
		return nil, "", nil, fmt.Errorf("userconfig: secure settings directory: %w", err)
	}
	temporary, err := s.ops.createTemp(s.paths.SettingsDir, pattern)
	if err != nil {
		return nil, "", nil, fmt.Errorf("userconfig: create temporary settings: %w", err)
	}
	path := temporary.Name()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		_ = s.ops.remove(path)
		return nil, "", nil, fmt.Errorf("userconfig: secure temporary settings: %w", err)
	}
	return temporary, path, func() {
		_ = temporary.Close()
		_ = s.ops.remove(path)
	}, nil
}

func (s *Store) finishTemporary(ctx context.Context, file storeFile, path string, hash [sha256.Size]byte, size int64) (preparedTemporary, error) {
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return preparedTemporary{}, err
	}
	if err := file.Sync(); err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: sync temporary settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return preparedTemporary{}, err
	}
	info, err := s.ops.stat(file)
	if err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: stat temporary settings: %w", err)
	}
	identity, err := documentIdentityForFile(file, info)
	if err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: identify temporary settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return preparedTemporary{}, err
	}
	if err := file.Close(); err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: close temporary settings: %w", err)
	}
	return preparedTemporary{path: path, token: documentToken(hash, size, info, identity)}, nil
}

func writeAll(ctx context.Context, file io.Writer, data []byte) error {
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := data
		if len(chunk) > 32<<10 {
			chunk = chunk[:32<<10]
		}
		count, err := file.Write(chunk)
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if err != nil {
			return err
		}
		if count <= 0 {
			return io.ErrShortWrite
		}
		data = data[count:]
	}
	return nil
}

func (s *Store) copyCurrentToTemporary(ctx context.Context, expected DocumentToken, pattern string) (preparedTemporary, error) {
	temporary, path, cleanup, err := s.newTemporary(ctx, pattern)
	if err != nil {
		return preparedTemporary{}, err
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	if err := ctx.Err(); err != nil {
		return preparedTemporary{}, err
	}
	source, err := s.ops.open(s.paths.SettingsFile)
	if err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: open invalid settings for backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = source.Close()
		return preparedTemporary{}, err
	}
	info, err := s.ops.stat(source)
	if err != nil {
		_ = source.Close()
		return preparedTemporary{}, fmt.Errorf("userconfig: stat invalid settings for backup: %w", err)
	}
	identity, err := documentIdentityForFile(source, info)
	if err != nil {
		_ = source.Close()
		return preparedTemporary{}, fmt.Errorf("userconfig: identify invalid settings for backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = source.Close()
		return preparedTemporary{}, err
	}
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			_ = source.Close()
			return preparedTemporary{}, err
		}
		count, readErr := s.ops.read(source, buffer)
		if err := ctx.Err(); err != nil {
			_ = source.Close()
			return preparedTemporary{}, err
		}
		if count > 0 {
			chunk := buffer[:count]
			_, _ = hash.Write(chunk)
			size += int64(count)
			if err := writeAll(ctx, temporary, chunk); err != nil {
				_ = source.Close()
				return preparedTemporary{}, fmt.Errorf("userconfig: write invalid backup: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = source.Close()
			return preparedTemporary{}, fmt.Errorf("userconfig: read invalid settings for backup: %w", readErr)
		}
	}
	if err := source.Close(); err != nil {
		return preparedTemporary{}, fmt.Errorf("userconfig: close invalid settings for backup: %w", err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	if token := documentToken(sum, size, info, identity); token != expected {
		return preparedTemporary{}, ErrConflict
	}
	prepared, err := s.finishTemporary(ctx, temporary, path, sum, size)
	if err != nil {
		return preparedTemporary{}, err
	}
	keep = true
	return prepared, nil
}

func (s *Store) replaceTemporary(ctx context.Context, temporary, destination string, expected DocumentToken) error {
	defer s.ops.remove(temporary)
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := s.readCurrent(ctx)
	if errors.Is(err, os.ErrNotExist) && !expected.exists {
		// First-run creation is allowed only while the destination still does not exist.
	} else if err != nil {
		return fmt.Errorf("userconfig: reread before replacement: %w", err)
	} else if current.token != expected {
		return ErrConflict
	}
	if err := s.ops.replace(temporary, destination); err != nil {
		return fmt.Errorf("userconfig: replace settings: %w", err)
	}
	// Directory fsync is unavailable or unsupported on some Windows filesystems;
	// replacement has already succeeded, so this remains deliberately best effort.
	_ = s.ops.syncDirectory(filepath.Dir(destination))
	return nil
}

func (s *Store) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.lock:
		return nil
	}
}

func (s *Store) release() { s.lock <- struct{}{} }
