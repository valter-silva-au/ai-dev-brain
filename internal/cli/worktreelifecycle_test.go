package cli

import (
	"bytes"
	"testing"
)

func TestWorktreeLifecycleCommands(t *testing.T) {
	t.Run("worktree-lifecycle has all subcommands", func(t *testing.T) {
		subs := worktreeLifecycleCmd.Commands()
		names := make(map[string]bool)
		for _, s := range subs {
			names[s.Name()] = true
		}

		expected := []string{"pre-create", "post-create", "pre-remove", "post-remove"}
		for _, name := range expected {
			if !names[name] {
				t.Errorf("expected subcommand %q not found", name)
			}
		}
	})

	t.Run("post-create exits gracefully without env vars", func(t *testing.T) {
		// Unset env vars to simulate no task context.
		t.Setenv("ADB_TASK_ID", "")
		t.Setenv("ADB_WORKTREE_PATH", "")

		var buf bytes.Buffer
		worktreePostCreateCmd.SetOut(&buf)
		err := worktreePostCreateCmd.RunE(worktreePostCreateCmd, nil)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("pre-remove exits gracefully without env vars", func(t *testing.T) {
		t.Setenv("ADB_WORKTREE_PATH", "")

		err := worktreePreRemoveCmd.RunE(worktreePreRemoveCmd, nil)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("post-remove exits gracefully without env vars", func(t *testing.T) {
		t.Setenv("ADB_TASK_ID", "")
		t.Setenv("ADB_TICKET_PATH", "")

		err := worktreePostRemoveCmd.RunE(worktreePostRemoveCmd, nil)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}
