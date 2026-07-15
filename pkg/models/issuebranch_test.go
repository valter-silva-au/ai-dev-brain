package models

import "testing"

func TestIssueBranchName(t *testing.T) {
	cases := []struct {
		name  string
		typ   TaskType
		slug  string
		id    string
		issue int
		want  string
	}{
		{"linked feat encodes issue", TaskTypeFeat, "my-slug", "TASK-00042", 210, "feat/210-my-slug"},
		{"unlinked (0) falls back", TaskTypeFeat, "my-slug", "TASK-00042", 0, "feat/my-slug"},
		{"negative falls back", TaskTypeFeat, "my-slug", "TASK-00042", -1, "feat/my-slug"},
		{"spike maps to chore + issue", TaskTypeSpike, "probe", "TASK-1", 7, "chore/7-probe"},
		{"empty slug uses id", TaskTypeFeat, "", "TASK-9", 5, "feat/5-task-9"},
	}
	for _, tc := range cases {
		if got := IssueBranchName(tc.typ, tc.slug, tc.id, tc.issue); got != tc.want {
			t.Errorf("%s: IssueBranchName(%v,%q,%q,%d) = %q, want %q", tc.name, tc.typ, tc.slug, tc.id, tc.issue, got, tc.want)
		}
	}
}
