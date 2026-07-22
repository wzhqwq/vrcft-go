package projectstatus

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrMalformedFrontMatter = errors.New("malformed spec front matter")
	ErrInvalidSpec          = errors.New("invalid project spec")
	ErrMissingSection       = errors.New("missing spec section")
)

var requiredSections = []string{
	"Purpose",
	"Responsibilities",
	"Non-responsibilities",
	"Current implementation",
	"Public/internal interfaces",
	"Owned data",
	"Dependencies",
	"Concurrency and lifecycle",
	"Error handling",
	"Performance constraints",
	"Security boundaries",
	"Required tests",
	"Known gaps",
	"Completion definition",
}

func ParseSpec(sourcePath string, content []byte) (Spec, error) {
	lines := bytes.Split(content, []byte("\n"))
	if len(lines) < 3 || strings.TrimSpace(string(lines[0])) != "---" {
		return Spec{}, fmt.Errorf("%s: %w", sourcePath, ErrMalformedFrontMatter)
	}
	closing := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(string(lines[index])) == "---" {
			closing = index
			break
		}
	}
	if closing < 0 {
		return Spec{}, fmt.Errorf("%s: %w: closing delimiter", sourcePath, ErrMalformedFrontMatter)
	}

	var spec Spec
	decoder := yaml.NewDecoder(bytes.NewReader(bytes.Join(lines[1:closing], []byte("\n"))))
	decoder.KnownFields(true)
	if err := decoder.Decode(&spec); err != nil {
		return Spec{}, fmt.Errorf("%s: %w: %v", sourcePath, ErrInvalidSpec, err)
	}
	spec.SourcePath = filepath.ToSlash(sourcePath)
	spec.Body = strings.TrimSpace(string(bytes.Join(lines[closing+1:], []byte("\n")))) + "\n"
	if err := ValidateSpec(spec); err != nil {
		return Spec{}, fmt.Errorf("%s: %w", sourcePath, err)
	}
	if err := validateSections(spec.Body); err != nil {
		return Spec{}, fmt.Errorf("%s: %w", sourcePath, err)
	}
	return spec, nil
}

func ValidateSpec(spec Spec) error {
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.Milestone) == "" {
		return fmt.Errorf("%w: id and milestone are required", ErrInvalidSpec)
	}
	switch spec.Kind {
	case KindGoPackage, KindFrontend, KindParameterSpec, KindBuildRelease, KindEndToEnd:
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidSpec, spec.Kind)
	}
	normalized := strings.ReplaceAll(spec.Path, "\\", "/")
	cleaned := path.Clean(normalized)
	if normalized == "" || filepath.IsAbs(spec.Path) || path.IsAbs(normalized) || filepath.VolumeName(spec.Path) != "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: unsafe path %q", ErrInvalidSpec, spec.Path)
	}
	if cleaned == "." && spec.ID != "root" {
		return fmt.Errorf("%w: only root may use path .", ErrInvalidSpec)
	}
	if len(spec.Checks) == 0 {
		return fmt.Errorf("%w: at least one check is required", ErrInvalidSpec)
	}
	checkIDs := make(map[string]struct{}, len(spec.Checks))
	required := false
	for _, check := range spec.Checks {
		if strings.TrimSpace(check.ID) == "" || strings.TrimSpace(check.Description) == "" || check.Weight <= 0 {
			return fmt.Errorf("%w: check id, description, and positive weight are required", ErrInvalidSpec)
		}
		if _, exists := checkIDs[check.ID]; exists {
			return fmt.Errorf("%w: duplicate check %q", ErrInvalidSpec, check.ID)
		}
		checkIDs[check.ID] = struct{}{}
		switch check.Type {
		case CheckCommand, CheckFile, CheckSymbol, CheckNotPlaceholder, CheckGeneratedClean, CheckDependsComplete, CheckAggregate:
		default:
			return fmt.Errorf("%w: unknown check type %q", ErrInvalidSpec, check.Type)
		}
		if check.Required {
			required = true
		}
	}
	if !required {
		return fmt.Errorf("%w: at least one required check is needed", ErrInvalidSpec)
	}
	for _, blocker := range spec.Blockers {
		if _, exists := checkIDs[blocker.Check]; !exists {
			return fmt.Errorf("%w: blocker references unknown check %q", ErrInvalidSpec, blocker.Check)
		}
	}
	return nil
}

func validateSections(body string) error {
	position := 0
	for _, section := range requiredSections {
		heading := "## " + section
		relative := strings.Index(body[position:], heading)
		if relative < 0 {
			return fmt.Errorf("%w: %s", ErrMissingSection, section)
		}
		position += relative + len(heading)
	}
	return nil
}

func sortSpecMetadata(spec *Spec) {
	sort.Strings(spec.DependsOn)
	for index := range spec.Checks {
		sort.Strings(spec.Checks[index].Members)
		sort.Strings(spec.Checks[index].Patterns)
	}
}
