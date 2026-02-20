package core

import (
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestFormatBranchName(t *testing.T) {
	tests := []struct {
		name        string
		pattern     string
		taskType    models.TaskType
		taskID      string
		description string
		want        string
	}{
		{
			name:        "empty pattern returns description as-is",
			pattern:     "",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "feat/my-branch",
			want:        "feat/my-branch",
		},
		{
			name:        "standard pattern",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "add-auth",
			want:        "feat/TASK-00001-add-auth",
		},
		{
			name:        "pattern with spaces in description",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeBug,
			taskID:      "CCAAS-42",
			description: "fix login timeout",
			want:        "bug/CCAAS-42-fix-login-timeout",
		},
		{
			name:        "pattern with special chars in description",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeSpike,
			taskID:      "PROJ-001",
			description: "evaluate: Redis vs. Memcached!",
			want:        "spike/PROJ-001-evaluate-redis-vs.-memcached",
		},
		{
			name:        "pattern without description",
			pattern:     "{type}/{id}",
			taskType:    models.TaskTypeRefactor,
			taskID:      "TASK-00005",
			description: "cleanup",
			want:        "refactor/TASK-00005",
		},
		{
			name:        "custom JIRA-style prefix no padding",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "CCAAS-1",
			description: "add auth",
			want:        "feat/CCAAS-1-add-auth",
		},
		{
			name:        "description with consecutive special chars",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "fix---multiple---dashes",
			want:        "feat/TASK-00001-fix-multiple-dashes",
		},
		{
			name:        "description with leading and trailing special chars",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "---trimmed---",
			want:        "feat/TASK-00001-trimmed",
		},
		{
			name:        "description with uppercase",
			pattern:     "{type}/{id}-{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "Add User Auth",
			want:        "feat/TASK-00001-add-user-auth",
		},
		{
			name:        "repo placeholder with path-based task ID",
			pattern:     "{type}/{repo}/{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "github.com/org/finance/new-feature",
			description: "new-feature",
			want:        "feat/finance/new-feature",
		},
		{
			name:        "prefix placeholder with path-based task ID",
			pattern:     "{type}/{prefix}/{description}",
			taskType:    models.TaskTypeBug,
			taskID:      "github.com/org/repo/fix-crash",
			description: "fix-crash",
			want:        "bug/github.com/org/repo/fix-crash",
		},
		{
			name:        "repo placeholder with short prefix task ID",
			pattern:     "{type}/{repo}/{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "finance/add-auth",
			description: "add-auth",
			want:        "feat/finance/add-auth",
		},
		{
			name:        "repo and prefix empty for legacy task ID",
			pattern:     "{type}/{repo}/{description}",
			taskType:    models.TaskTypeFeat,
			taskID:      "TASK-00001",
			description: "add-auth",
			want:        "feat//add-auth",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatBranchName(tc.pattern, tc.taskType, tc.taskID, tc.description)
			if got != tc.want {
				t.Errorf("FormatBranchName(%q, %q, %q, %q) = %q, want %q",
					tc.pattern, tc.taskType, tc.taskID, tc.description, got, tc.want)
			}
		})
	}
}

func TestSanitizeBranchSegment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple lowercase", "hello-world", "hello-world"},
		{"uppercase", "Hello-World", "hello-world"},
		{"spaces to dashes", "hello world", "hello-world"},
		{"special chars", "fix: crash!", "fix-crash"},
		{"consecutive dashes", "a---b", "a-b"},
		{"leading trailing dashes", "---abc---", "abc"},
		{"dots preserved", "v1.2.3", "v1.2.3"},
		{"slashes preserved", "feat/sub", "feat/sub"},
		{"empty string", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeBranchSegment(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeBranchSegment(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
