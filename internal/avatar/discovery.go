package avatar

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Source uint8

const (
	SourceAvatarConfig Source = iota + 1
	SourceFallback
	SourceNone
)

type resolvedConfig struct {
	path           string
	source         Source
	requireIDMatch bool
}

type configCandidate struct {
	path    string
	modTime int64
}

func validateAvatarID(avatarID string) error {
	if avatarID == "" || len(avatarID) > maxAvatarIDBytes || avatarID == "." || avatarID == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidAvatarID, avatarID)
	}
	if strings.ContainsAny(avatarID, "/\\:\x00*?[]") {
		return fmt.Errorf("%w: unsafe character in %q", ErrInvalidAvatarID, avatarID)
	}
	return nil
}

func resolveConfig(oscRoot, fallbackPath, avatarID string) (resolvedConfig, error) {
	if err := validateAvatarID(avatarID); err != nil {
		return resolvedConfig{}, err
	}

	root, err := absoluteCleanPath(oscRoot)
	if err != nil {
		return resolvedConfig{}, fmt.Errorf("%w: OSC root %q: %v", ErrInvalidConfigPath, oscRoot, err)
	}
	pattern := filepath.Join(root, "*", "Avatars", avatarID+".json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return resolvedConfig{}, fmt.Errorf("%w: glob %q: %v", ErrInvalidConfigPath, pattern, err)
	}
	if len(matches) == 0 {
		return resolveFallback(fallbackPath)
	}

	candidates := make([]configCandidate, 0, len(matches))
	for _, match := range matches {
		candidate, err := absoluteCleanPath(match)
		if err != nil {
			return resolvedConfig{}, fmt.Errorf("%w: candidate %q: %v", ErrInvalidConfigPath, match, err)
		}
		if !isWithinRoot(root, candidate) {
			return resolvedConfig{}, fmt.Errorf("%w: candidate %q escapes OSC root %q", ErrInvalidConfigPath, candidate, root)
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return resolvedConfig{}, fmt.Errorf("%w: inspect candidate %q: %v", ErrInvalidConfigPath, candidate, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return resolvedConfig{}, fmt.Errorf("%w: candidate %q is not a regular non-link file", ErrInvalidConfigPath, candidate)
		}
		candidates = append(candidates, configCandidate{path: candidate, modTime: info.ModTime().UnixNano()})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime != candidates[j].modTime {
			return candidates[i].modTime > candidates[j].modTime
		}
		return candidates[i].path < candidates[j].path
	})
	return resolvedConfig{
		path:           candidates[0].path,
		source:         SourceAvatarConfig,
		requireIDMatch: true,
	}, nil
}

func resolveFallback(fallbackPath string) (resolvedConfig, error) {
	if fallbackPath == "" {
		return resolvedConfig{}, ErrConfigNotFound
	}
	path, err := absoluteCleanPath(fallbackPath)
	if err != nil {
		return resolvedConfig{}, fmt.Errorf("%w: fallback path %q: %v", ErrInvalidConfigPath, fallbackPath, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return resolvedConfig{}, fmt.Errorf("%w: fallback %q", ErrConfigNotFound, path)
		}
		return resolvedConfig{}, fmt.Errorf("%w: inspect fallback %q: %v", ErrInvalidConfigPath, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return resolvedConfig{}, fmt.Errorf("%w: fallback %q is not a regular non-link file", ErrInvalidConfigPath, path)
	}
	return resolvedConfig{path: path, source: SourceFallback}, nil
}

func absoluteCleanPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func isWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
