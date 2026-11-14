package plugins

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxTroubleshootingSteps     = 8
	maxTroubleshootingStepBytes = 512
)

// Troubleshooting is optional plugin-authored plain text. Both application
// locales must be present together so the UI never mixes languages.
type Troubleshooting struct {
	EnUS []string `json:"en-US"`
	ZhCN []string `json:"zh-CN"`
}

func (guide Troubleshooting) Validate() error {
	for _, locale := range []struct {
		name  string
		steps []string
	}{{"en-US", guide.EnUS}, {"zh-CN", guide.ZhCN}} {
		if len(locale.steps) == 0 || len(locale.steps) > maxTroubleshootingSteps {
			return fmt.Errorf("%w: troubleshooting %s must have 1 to %d steps", ErrInvalidManifest, locale.name, maxTroubleshootingSteps)
		}
		for _, step := range locale.steps {
			if !utf8.ValidString(step) || strings.TrimSpace(step) == "" || len(step) > maxTroubleshootingStepBytes {
				return fmt.Errorf("%w: troubleshooting %s step is blank or exceeds %d bytes", ErrInvalidManifest, locale.name, maxTroubleshootingStepBytes)
			}
		}
	}
	return nil
}

func (guide Troubleshooting) clone() Troubleshooting {
	return Troubleshooting{
		EnUS: append([]string(nil), guide.EnUS...),
		ZhCN: append([]string(nil), guide.ZhCN...),
	}
}
