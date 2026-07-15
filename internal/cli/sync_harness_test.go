package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestSyncClaudeUser_InstallsHarness drives `adb sync claude-user` through the
// real root command and asserts it installs the embedded harness into the Claude
// config dir (resolved from CLAUDE_CONFIG_DIR, pointed at a temp dir so the real
// ~/.claude is never touched): a fresh install lands the files, --dry-run writes
// nothing, and --force overwrites a locally edited file while a plain re-run does
// not clobber it.
func TestSyncClaudeUser_InstallsHarness(t *testing.T) {
	ws := t.TempDir()
	app, err := internal.NewApp(ws)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer app.Cleanup()
	oldApp := App
	App = app
	defer func() { App = oldApp }()

	agentRel := filepath.Join("agents", "devils-advocate.md")
	skillRel := filepath.Join("skills", "stage-gate", "SKILL.md")

	t.Run("installs harness into CLAUDE_CONFIG_DIR", func(t *testing.T) {
		cdir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cdir)

		if err := runADB(t, "sync", "claude-user"); err != nil {
			t.Fatalf("sync claude-user: %v", err)
		}
		for _, rel := range []string{agentRel, skillRel} {
			info, err := os.Stat(filepath.Join(cdir, rel))
			if err != nil || info.Size() == 0 {
				t.Errorf("expected non-empty installed %s (err=%v)", rel, err)
			}
		}
	})

	t.Run("dry-run writes nothing", func(t *testing.T) {
		cdir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cdir)

		if err := runADB(t, "sync", "claude-user", "--dry-run"); err != nil {
			t.Fatalf("sync claude-user --dry-run: %v", err)
		}
		if _, err := os.Stat(filepath.Join(cdir, "agents")); !os.IsNotExist(err) {
			t.Errorf("dry-run created the agents dir (err=%v)", err)
		}
	})

	t.Run("force overwrites an edited file; plain re-run does not", func(t *testing.T) {
		cdir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", cdir)
		agent := filepath.Join(cdir, agentRel)

		if err := runADB(t, "sync", "claude-user"); err != nil {
			t.Fatalf("initial install: %v", err)
		}
		embedded, err := os.ReadFile(agent)
		if err != nil {
			t.Fatalf("read installed agent: %v", err)
		}

		// Local edit + plain re-run must NOT clobber it.
		if err := os.WriteFile(agent, []byte("LOCAL EDIT"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := runADB(t, "sync", "claude-user"); err != nil {
			t.Fatalf("re-run: %v", err)
		}
		if b, _ := os.ReadFile(agent); string(b) != "LOCAL EDIT" {
			t.Errorf("plain re-run clobbered a local edit: %q", b)
		}

		// --force restores the embedded content.
		if err := runADB(t, "sync", "claude-user", "--force"); err != nil {
			t.Fatalf("--force re-run: %v", err)
		}
		if b, _ := os.ReadFile(agent); string(b) != string(embedded) {
			t.Errorf("--force did not restore embedded content: %q", b)
		}
	})
}
