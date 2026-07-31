package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
	"github.com/wzhqwq/vrcft-go/pkg/trackingmodel"
)

const (
	manifestSchemaVersion       = 1
	maxManifestNameBytes        = 256
	maxManifestDescriptionBytes = 4096
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceDev     Source = "dev"
)

type Manifest struct {
	SchemaVersion int                      `json:"schemaVersion"`
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	Description   string                   `json:"description"`
	ProtocolMin   uint16                   `json:"protocolMin"`
	ProtocolMax   uint16                   `json:"protocolMax"`
	Entrypoint    string                   `json:"entrypoint"`
	Capabilities  trackingmodel.Capability `json:"capabilities"`
}

type InstalledPlugin struct {
	Manifest   Manifest
	RootDir    string
	Executable string
	Source     Source
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != manifestSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version", ErrInvalidManifest)
	}

	descriptor := pluginapi.Descriptor{
		APIVersion:   pluginapi.APIVersion,
		ID:           m.ID,
		Name:         m.Name,
		Version:      m.Version,
		Description:  m.Description,
		Capabilities: m.Capabilities,
	}
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("%w: identity is invalid: %w", ErrInvalidManifest, err)
	}
	if len(m.Name) > maxManifestNameBytes {
		return fmt.Errorf("%w: name exceeds size limit", ErrInvalidManifest)
	}
	if strings.TrimSpace(m.Description) == "" {
		return fmt.Errorf("%w: description must be nonblank", ErrInvalidManifest)
	}
	if len(m.Description) > maxManifestDescriptionBytes {
		return fmt.Errorf("%w: description exceeds size limit", ErrInvalidManifest)
	}
	if m.ProtocolMin > protocol.Version || m.ProtocolMax < protocol.Version {
		return fmt.Errorf("%w: protocol range does not include host protocol", ErrInvalidManifest)
	}
	if err := validateEntrypoint(m.Entrypoint); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	return nil
}

func validateEntrypoint(entrypoint string) error {
	if entrypoint == "" {
		return fmt.Errorf("%w: must be nonempty", ErrInvalidEntrypoint)
	}
	if strings.IndexByte(entrypoint, 0) >= 0 {
		return fmt.Errorf("%w: contains NUL", ErrInvalidEntrypoint)
	}
	if filepath.IsAbs(entrypoint) || filepath.VolumeName(entrypoint) != "" ||
		strings.HasPrefix(entrypoint, `\\`) || strings.HasPrefix(entrypoint, "//") {
		return fmt.Errorf("%w: must be a relative path", ErrInvalidEntrypoint)
	}
	for _, component := range strings.FieldsFunc(entrypoint, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return fmt.Errorf("%w: must not traverse parent directories", ErrInvalidEntrypoint)
		}
	}
	return nil
}

func resolveEntrypoint(rootDir, entrypoint string) (string, error) {
	_, executable, err := resolveLaunchPaths(rootDir, entrypoint)
	return executable, err
}

func resolveLaunchPaths(rootDir, entrypoint string) (string, string, error) {
	if err := validateEntrypoint(entrypoint); err != nil {
		return "", "", err
	}
	absoluteRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", "", fmt.Errorf("%w: root path is invalid", ErrInvalidEntrypoint)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", "", fmt.Errorf("%w: root cannot be resolved", ErrInvalidEntrypoint)
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return "", "", fmt.Errorf("%w: root path is invalid", ErrInvalidEntrypoint)
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return "", "", fmt.Errorf("%w: root is not a directory", ErrInvalidEntrypoint)
	}

	candidate := filepath.Join(canonicalRoot, entrypoint)
	canonicalExecutable, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", "", fmt.Errorf("%w: executable cannot be resolved", ErrInvalidEntrypoint)
	}
	canonicalExecutable, err = filepath.Abs(canonicalExecutable)
	if err != nil {
		return "", "", fmt.Errorf("%w: executable path is invalid", ErrInvalidEntrypoint)
	}
	contained, err := isContainedPath(canonicalRoot, canonicalExecutable)
	if err != nil || !contained {
		return "", "", fmt.Errorf("%w: executable escapes its root directory", ErrInvalidEntrypoint)
	}
	info, err := os.Stat(canonicalExecutable)
	if err != nil || !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("%w: executable is not a regular file", ErrInvalidEntrypoint)
	}
	return canonicalRoot, canonicalExecutable, nil
}

func isContainedPath(root, target string) (bool, error) {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}
