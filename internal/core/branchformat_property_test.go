package core

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// gitSafeChars matches only characters that are safe in git branch names.
var gitSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._/-]*$`)

// Feature: ai-dev-brain, Property: Branch Format Contains Task Type
// When the default pattern {type}/{repo}/{description} is used, the output always starts with the task type.
func TestProperty_BranchFormatContainsTaskType(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskType := taskTypeGenerator().Draw(rt, "taskType")
		description := rapid.StringMatching(`[a-z ]{3,30}`).Draw(rt, "description")
		repoName := rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "repoName")

		taskID := "github.com/org/" + repoName + "/" + description

		pattern := "{type}/{repo}/{description}"
		result := FormatBranchName(pattern, taskType, taskID, description)

		if !strings.HasPrefix(result, string(taskType)+"/") {
			t.Fatalf("formatted branch %q must start with task type %q/", result, taskType)
		}
	})
}

// Feature: ai-dev-brain, Property: Sanitized Output Contains Only Safe Chars
// The output of sanitizeBranchSegment only contains git-safe characters.
func TestProperty_SanitizedOutputContainsSafeChars(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		input := rapid.String().Draw(rt, "input")
		result := sanitizeBranchSegment(input)

		if !gitSafeChars.MatchString(result) {
			t.Fatalf("sanitizeBranchSegment(%q) = %q contains unsafe characters", input, result)
		}

		// Must not contain consecutive dashes.
		if strings.Contains(result, "--") {
			t.Fatalf("sanitizeBranchSegment(%q) = %q contains consecutive dashes", input, result)
		}

		// Must not start or end with a dash.
		if strings.HasPrefix(result, "-") || strings.HasSuffix(result, "-") {
			t.Fatalf("sanitizeBranchSegment(%q) = %q starts or ends with a dash", input, result)
		}
	})
}
