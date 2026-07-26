package plugins

import (
	"errors"
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
		return errors.New("plugin manifest has unsupported schema version")
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
		return fmt.Errorf("plugin manifest identity is invalid: %w", err)
	}
	if len(m.Name) > maxManifestNameBytes {
		return errors.New("plugin manifest name exceeds size limit")
	}
	if strings.TrimSpace(m.Description) == "" {
		return errors.New("plugin manifest description must be nonblank")
	}
	if len(m.Description) > maxManifestDescriptionBytes {
		return errors.New("plugin manifest description exceeds size limit")
	}
	if m.ProtocolMin > protocol.Version || m.ProtocolMax < protocol.Version {
		return errors.New("plugin manifest protocol range does not include host protocol")
	}
	if err := validateEntrypoint(m.Entrypoint); err != nil {
		return err
	}
	return nil
}

func validateEntrypoint(entrypoint string) error {
	if entrypoint == "" {
		return errors.New("plugin manifest entrypoint must be nonempty")
	}
	if strings.IndexByte(entrypoint, 0) >= 0 {
		return errors.New("plugin manifest entrypoint contains NUL")
	}
	if filepath.IsAbs(entrypoint) || filepath.VolumeName(entrypoint) != "" ||
		strings.HasPrefix(entrypoint, `\\`) || strings.HasPrefix(entrypoint, "//") {
		return errors.New("plugin manifest entrypoint must be a relative path")
	}
	for _, component := range strings.FieldsFunc(entrypoint, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == ".." {
			return errors.New("plugin manifest entrypoint must not traverse parent directories")
		}
	}
	return nil
}

func resolveEntrypoint(rootDir, entrypoint string) (string, error) {
	if err := validateEntrypoint(entrypoint); err != nil {
		return "", err
	}
	absoluteRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", errors.New("plugin root path is invalid")
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", errors.New("plugin root cannot be resolved")
	}
	rootInfo, err := os.Stat(canonicalRoot)
	if err != nil || !rootInfo.IsDir() {
		return "", errors.New("plugin root is not a directory")
	}

	candidate := filepath.Join(canonicalRoot, entrypoint)
	canonicalExecutable, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", errors.New("plugin executable cannot be resolved")
	}
	canonicalExecutable, err = filepath.Abs(canonicalExecutable)
	if err != nil {
		return "", errors.New("plugin executable path is invalid")
	}
	contained, err := isContainedPath(canonicalRoot, canonicalExecutable)
	if err != nil || !contained {
		return "", errors.New("plugin executable escapes its root directory")
	}
	info, err := os.Stat(canonicalExecutable)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("plugin executable is not a regular file")
	}
	return canonicalExecutable, nil
}

func isContainedPath(root, target string) (bool, error) {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}
