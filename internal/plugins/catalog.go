package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const defaultMaxManifestBytes int64 = 64 * 1024

type Catalog interface {
	Scan(context.Context) ([]InstalledPlugin, error)
}

type DirectoryCatalogConfig struct {
	BuiltinRoot      string
	DevRoots         []string
	MaxManifestBytes int64
}

type directoryCatalog struct {
	builtinRoot      string
	devRoots         []string
	maxManifestBytes int64
}

func NewDirectoryCatalog(config DirectoryCatalogConfig) (Catalog, error) {
	if config.BuiltinRoot == "" {
		return nil, errors.New("plugin catalog requires a builtin root")
	}
	builtinRoot, err := filepath.Abs(config.BuiltinRoot)
	if err != nil {
		return nil, errors.New("plugin catalog builtin root is invalid")
	}
	devRoots := make([]string, 0, len(config.DevRoots))
	for _, root := range config.DevRoots {
		if root == "" {
			return nil, errors.New("plugin catalog development root is invalid")
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, errors.New("plugin catalog development root is invalid")
		}
		devRoots = append(devRoots, absoluteRoot)
	}
	maxManifestBytes := config.MaxManifestBytes
	if maxManifestBytes == 0 {
		maxManifestBytes = defaultMaxManifestBytes
	}
	if maxManifestBytes < 1 {
		return nil, errors.New("plugin catalog manifest size limit must be positive")
	}
	return &directoryCatalog{
		builtinRoot:      builtinRoot,
		devRoots:         devRoots,
		maxManifestBytes: maxManifestBytes,
	}, nil
}

func (c *directoryCatalog) Scan(ctx context.Context) ([]InstalledPlugin, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plugins := make([]InstalledPlugin, 0)
	seenIDs := make(map[string]struct{})
	if err := c.scanRoot(ctx, c.builtinRoot, SourceBuiltin, false, seenIDs, &plugins); err != nil {
		return nil, err
	}
	for _, root := range c.devRoots {
		if err := c.scanRoot(ctx, root, SourceDev, true, seenIDs, &plugins); err != nil {
			return nil, err
		}
	}
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Manifest.ID < plugins[j].Manifest.ID
	})
	return plugins, nil
}

func (c *directoryCatalog) scanRoot(ctx context.Context, root string, source Source, optional bool, seenIDs map[string]struct{}, plugins *[]InstalledPlugin) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if optional && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return errors.New("plugin catalog root cannot be resolved")
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return errors.New("plugin catalog root is not a directory")
	}
	entries, err := os.ReadDir(canonicalRoot)
	if err != nil {
		return errors.New("plugin catalog root cannot be read")
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		pluginRoot := filepath.Join(canonicalRoot, entry.Name())
		entryInfo, err := os.Lstat(pluginRoot)
		if err != nil {
			return errors.New("plugin catalog entry cannot be inspected")
		}
		if entryInfo.Mode()&os.ModeSymlink != 0 || entryInfo.Mode()&os.ModeIrregular != 0 {
			return errors.New("plugin catalog does not permit linked plugin directories")
		}
		if !entryInfo.IsDir() {
			continue
		}
		manifest, err := c.readManifest(pluginRoot)
		if err != nil {
			return fmt.Errorf("plugin catalog contains an invalid manifest: %w", err)
		}
		if _, exists := seenIDs[manifest.ID]; exists {
			return fmt.Errorf("%w", ErrDuplicatePluginID)
		}
		executable, err := resolveEntrypoint(pluginRoot, manifest.Entrypoint)
		if err != nil {
			return fmt.Errorf("plugin catalog contains an invalid executable: %w", err)
		}
		seenIDs[manifest.ID] = struct{}{}
		*plugins = append(*plugins, InstalledPlugin{
			Manifest:   manifest,
			RootDir:    pluginRoot,
			Executable: executable,
			Source:     source,
		})
	}
	return nil
}

func (c *directoryCatalog) readManifest(pluginRoot string) (Manifest, error) {
	file, err := os.Open(filepath.Join(pluginRoot, "manifest.json"))
	if err != nil {
		return Manifest{}, errors.New("manifest cannot be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Manifest{}, errors.New("manifest cannot be inspected")
	}
	if info.Size() > c.maxManifestBytes {
		return Manifest{}, errors.New("manifest exceeds size limit")
	}
	decoder := json.NewDecoder(io.LimitReader(file, c.maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, errors.New("manifest JSON is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Manifest{}, errors.New("manifest JSON has trailing content")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
