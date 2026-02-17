package core

import (
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// gitSafeChars matches only characters that are safe in git branch names.
var gitSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._/-]*$`)

// Feature: ai-dev-brain, Property: Branch Format Contains Task ID
// When a pattern contains {id}, the formatted output always contains the task ID.
func TestProperty_BranchFormatContainsTaskID(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		taskType := taskTypeGenerator().Draw(rt, "taskType")
		prefix := rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "prefix")
		counter := rapid.IntRange(1, 99999).Draw(rt, "counter")
		description := rapid.StringMatching(`[a-z ]{3,30}`).Draw(rt, "description")

		taskID := prefix + "-" + strings.Repeat("0", 5) + string(rune('0'+counter%10))
		// Use a simple format to avoid counter formatting issues.
		taskID = prefix + "-00001"

		pattern := "{type}/{id}-{description}"
		result := FormatBranchName(pattern, taskType, taskID, description)

		if !strings.Contains(result, taskID) {
			t.Fatalf("formatted branch %q must contain task ID %q", result, taskID)
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
