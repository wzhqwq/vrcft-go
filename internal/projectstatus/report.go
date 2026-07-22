package projectstatus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func RenderMarkdown(status Status) ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintln(&output, "# Project Status")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Generated: %s\n", status.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&output, "- Commit: `%s`\n", status.Commit)
	fmt.Fprintf(&output, "- Source fingerprint: `%s`\n", status.SourceFingerprint)
	fmt.Fprintf(&output, "- Dirty: `%t`\n", status.Dirty)
	fmt.Fprintf(&output, "- State: `%s`\n", status.State)
	fmt.Fprintf(&output, "- Progress: %.1f%% (%d/%d weight)\n", status.Progress, status.PassedWeight, status.TotalWeight)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Milestones")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Milestone | State | Progress |")
	fmt.Fprintln(&output, "|---|---|---:|")
	for _, milestone := range status.Milestones {
		fmt.Fprintf(&output, "| %s | %s | %.1f%% (%d/%d) |\n", milestone.ID, milestone.State, milestone.Progress, milestone.PassedWeight, milestone.TotalWeight)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Packages and Subsystems")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Milestone | Spec | Path | State | Progress |")
	fmt.Fprintln(&output, "|---|---|---|---|---:|")
	for _, spec := range status.Specs {
		fmt.Fprintf(&output, "| %s | %s | %s | %s | %.1f%% (%d/%d) |\n", spec.Milestone, spec.ID, strings.ReplaceAll(spec.Path, "\\", "/"), spec.State, spec.Progress, spec.PassedWeight, spec.TotalWeight)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Failed Required Checks")
	fmt.Fprintln(&output)
	if len(status.FailedRequired) == 0 {
		fmt.Fprintln(&output, "None.")
	} else {
		for _, check := range status.FailedRequired {
			fmt.Fprintf(&output, "- `%s/%s` (%s): %s\n", check.SpecID, check.CheckID, check.State, compactEvidence(check.Evidence))
		}
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "## Next Action")
	fmt.Fprintln(&output)
	if status.NextAction == nil {
		fmt.Fprintln(&output, "All required checks pass.")
	} else {
		fmt.Fprintf(&output, "Address `%s/%s`: %s\n", status.NextAction.SpecID, status.NextAction.CheckID, compactEvidence(status.NextAction.Evidence))
	}
	return output.Bytes(), nil
}

func RenderJSON(status Status) ([]byte, error) {
	content, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func NormalizeMarkdown(content []byte) []byte {
	var output bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "- Generated:") || strings.HasPrefix(line, "- Commit:") || strings.HasPrefix(line, "- Dirty:") {
			continue
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	return output.Bytes()
}

func compactEvidence(evidence string) string {
	evidence = strings.Join(strings.Fields(evidence), " ")
	if evidence == "" {
		return "no evidence"
	}
	if len(evidence) > 240 {
		return evidence[:240] + "…"
	}
	return evidence
}
