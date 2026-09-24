package ticket

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func inspectAppendOnly(
	manifest Manifest,
	layout Layout,
	activeProfile profile.Profile,
) ([]AppendOnlyCheckpoint, []Finding, error) {
	checkpoints := make(
		map[string]AppendOnlyCheckpoint,
		len(manifest.AppendOnlyCheckpoints),
	)
	for _, checkpoint := range manifest.AppendOnlyCheckpoints {
		checkpoints[checkpoint.Role] = checkpoint
	}

	next := make([]AppendOnlyCheckpoint, 0)
	findings := make([]Finding, 0)
	for _, artifact := range activeProfile.Artifacts {
		if !artifact.AppendOnly {
			continue
		}
		checkpoint, exists := checkpoints[artifact.Role]
		if !exists {
			findings = append(findings, Finding{
				Code:     "ticket.append_only.checkpoint_missing",
				Severity: "error",
				Summary: fmt.Sprintf(
					"Append-only role %q has no accepted checkpoint.",
					artifact.Role,
				),
				Evidence: []string{artifact.Path},
				NextAction: capability.Action{
					Code: "review_append_only_baseline",
					Message: "Review the authored file and explicitly establish " +
						"an accepted append-only baseline.",
				},
			})
			continue
		}
		path := filepath.Join(
			layout.Root(),
			filepath.FromSlash(artifact.Path),
		)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"read append-only role %q: %w",
				artifact.Role,
				err,
			)
		}
		if int64(len(content)) < checkpoint.Length {
			findings = append(findings, Finding{
				Code:     "ticket.append_only.truncated",
				Severity: "error",
				Summary: fmt.Sprintf(
					"Append-only role %q was truncated.",
					artifact.Role,
				),
				Evidence: []string{
					fmt.Sprintf(
						"accepted length=%d current length=%d",
						checkpoint.Length,
						len(content),
					),
				},
				NextAction: capability.Action{
					Code:    "restore_append_only_prefix",
					Message: "Restore the accepted prefix before continuing.",
				},
			})
			continue
		}
		prefix := content[:checkpoint.Length]
		if profile.HashContent(prefix) != checkpoint.Hash {
			findings = append(findings, Finding{
				Code:     "ticket.append_only.prefix_changed",
				Severity: "error",
				Summary: fmt.Sprintf(
					"Append-only role %q changed inside its accepted prefix.",
					artifact.Role,
				),
				Evidence: []string{artifact.Path},
				NextAction: capability.Action{
					Code:    "restore_append_only_prefix",
					Message: "Restore the accepted prefix; only suffix appends are safe.",
				},
			})
			continue
		}
		next = append(next, AppendOnlyCheckpoint{
			Role:   artifact.Role,
			Path:   artifact.Path,
			Length: int64(len(content)),
			Hash:   profile.HashContent(content),
		})
	}
	sort.Slice(next, func(left int, right int) bool {
		return next[left].Role < next[right].Role
	})
	return next, findings, nil
}

func findingsNextActions(findings []Finding) []capability.Action {
	actions := make([]capability.Action, 0, len(findings))
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		if finding.NextAction.Code == "" {
			continue
		}
		if _, exists := seen[finding.NextAction.Code]; exists {
			continue
		}
		seen[finding.NextAction.Code] = struct{}{}
		actions = append(actions, finding.NextAction)
	}
	return actions
}
