package models

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already kebab", "my-feature", "my-feature"},
		{"spaces to dashes", "Add nested layout", "add-nested-layout"},
		{"underscores to dashes", "fix_bug_in_x", "fix-bug-in-x"},
		{"mixed case", "PlatonicG0Probe", "platonicg0probe"},
		{"strip non-alnum", "feat: nested!", "feat-nested"},
		{"collapse runs", "a---b___c   d", "a-b-c-d"},
		{"trim leading/trailing dashes", "--hello--", "hello"},
		{"empty stays empty", "", ""},
		{"all-symbols collapses to empty", "!!!???", ""},
		{"unicode stripped", "olá-world", "ol-world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slugify(tt.input); got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConventionalType(t *testing.T) {
	tests := []struct {
		taskType TaskType
		want     string
	}{
		{TaskTypeFeat, "feat"},
		{TaskTypeBug, "fix"},     // legacy mapping preserved: bug -> fix
		{TaskTypeSpike, "chore"}, // ADR mapping: spike -> chore
		{TaskTypeRefactor, "refactor"},
		// The Conventional set added in WS-B maps 1:1 to its own prefix.
		{TaskTypeFix, "fix"},
		{TaskTypeDocs, "docs"},
		{TaskTypeChore, "chore"},
		{TaskTypeTest, "test"},
		{TaskTypePerf, "perf"},
		{TaskTypePrototype, "chore"},     // D10: prototype -> chore, like spike
		{TaskTypeWork, "work"},           // no case — work never branches, falls through
		{TaskType("mystery"), "mystery"}, // genuinely unknown falls through
	}
	for _, tt := range tests {
		t.Run(string(tt.taskType), func(t *testing.T) {
			if got := ConventionalType(tt.taskType); got != tt.want {
				t.Errorf("ConventionalType(%q) = %q, want %q", tt.taskType, got, tt.want)
			}
		})
	}
}

func TestTaskType_IsValid(t *testing.T) {
	valid := []TaskType{
		TaskTypeFeat, TaskTypeFix, TaskTypeRefactor, TaskTypeDocs,
		TaskTypeChore, TaskTypeTest, TaskTypePerf, TaskTypeSpike,
		TaskTypeWork, TaskTypePrototype,
	}
	for _, tt := range valid {
		if !tt.IsValid() {
			t.Errorf("IsValid(%q) = false, want true", tt)
		}
	}
	invalid := []TaskType{
		TaskTypeBug, // legacy alias — rejected by create/MCP in favour of fix
		TaskType(""),
		TaskType("feature"),
		TaskType("FEAT"),
	}
	for _, tt := range invalid {
		if tt.IsValid() {
			t.Errorf("IsValid(%q) = true, want false", tt)
		}
	}
}

func TestValidTaskTypes_Membership(t *testing.T) {
	// The eight Conventional-Commits CODE types plus the two NON-CODE types
	// (work, prototype) added in D10.
	want := map[TaskType]bool{
		TaskTypeFeat: true, TaskTypeFix: true, TaskTypeRefactor: true,
		TaskTypeDocs: true, TaskTypeChore: true, TaskTypeTest: true,
		TaskTypePerf: true, TaskTypeSpike: true,
		TaskTypeWork: true, TaskTypePrototype: true,
	}
	if len(ValidTaskTypes) != len(want) {
		t.Fatalf("ValidTaskTypes has %d entries, want %d", len(ValidTaskTypes), len(want))
	}
	for _, tt := range ValidTaskTypes {
		if !want[tt] {
			t.Errorf("ValidTaskTypes contains unexpected type %q", tt)
		}
		if tt == TaskTypeBug {
			t.Errorf("ValidTaskTypes must not contain the legacy bug type")
		}
	}
}

func TestBranchName(t *testing.T) {
	tests := []struct {
		name     string
		taskType TaskType
		slug     string
		id       string
		want     string
	}{
		{
			// The exact ADR example: a spike task gets a chore/<slug> branch,
			// not a spike/<slug> branch.
			name:     "spike maps to chore",
			taskType: TaskTypeSpike,
			slug:     "platonic-g0-insurability-probe",
			id:       "TASK-00050",
			want:     "chore/platonic-g0-insurability-probe",
		},
		{
			name:     "bug maps to fix",
			taskType: TaskTypeBug,
			slug:     "stickler-optional-field",
			id:       "TASK-00029",
			want:     "fix/stickler-optional-field",
		},
		{
			name:     "feat passes through",
			taskType: TaskTypeFeat,
			slug:     "nested-correlation-layout",
			id:       "TASK-00049",
			want:     "feat/nested-correlation-layout",
		},
		{
			name:     "refactor passes through",
			taskType: TaskTypeRefactor,
			slug:     "extract-resolver",
			id:       "TASK-00099",
			want:     "refactor/extract-resolver",
		},
		{
			// Defensive: empty slug should never happen in production
			// (Create always Slugifies the branch arg) but if it does,
			// fall back to the lowercased task ID rather than producing
			// a bare "feat/" with a trailing slash.
			name:     "empty slug falls back to id",
			taskType: TaskTypeFeat,
			slug:     "",
			id:       "TASK-00001",
			want:     "feat/task-00001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BranchName(tt.taskType, tt.slug, tt.id); got != tt.want {
				t.Errorf("BranchName(%q, %q, %q) = %q, want %q", tt.taskType, tt.slug, tt.id, got, tt.want)
			}
		})
	}
}
