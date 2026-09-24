package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// Session transcripts are the ONE file class adb writes that routinely contains
// whatever the coding agent read — an agent session reads .env files, prints
// `aws sts` output, and has secrets pasted into it. So this store is the single
// documented exception to CLAUDE.md's 0o644 file mandate: every file it writes is
// 0o600 (see CLAUDE.md's "Go coding standards", and the G306 note in
// .golangci.yml, which names this store).
//
// The directories stay 0o755. A 0o700 sessions dir would be a separate decision
// with a migration attached — the dirs already exist in real workspaces, created
// by the ticket bootstrap — whereas no transcript file has ever been written
// (TaskManager.sessionCapturer is assigned and never read), so the file mode is
// free to tighten today and will not be free later.
const wantSessionFileMode fs.FileMode = 0o600

func TestFileSessionStore_WritesTranscriptsPrivate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileSessionStoreManager(dir)

	session := &models.CapturedSession{
		ID:      "SESSION-00001",
		TaskID:  "TASK-00001",
		Summary: "a summary that could quote anything the agent read",
		Turns: []models.SessionTurn{
			{Index: 0, Role: "user", Content: "pretend this pasted an AWS key"},
			{Index: 1, Role: "assistant", Content: "and this echoed a .env file"},
		},
	}

	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	// Every file the store wrote, not just the ones we remembered to name: walk
	// the tree so a FIFTH file added later is covered without editing this test.
	var checked int
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		checked++
		if got := info.Mode().Perm(); got != wantSessionFileMode {
			rel, _ := filepath.Rel(dir, path)
			t.Errorf(
				"%s has mode %#o, want %#o — session transcripts may contain "+
					"whatever the agent read",
				rel, got, wantSessionFileMode,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the session store: %v", err)
	}

	// Anti-vacuity: a walk that found nothing would pass every assertion above.
	// SaveSession writes index.yaml, session.yaml, turns.yaml and summary.md.
	if checked < 4 {
		t.Fatalf("only %d file(s) inspected, want at least 4 — the walk found "+
			"almost nothing, so this test proves almost nothing", checked)
	}
}

// TestFileSessionStore_KeepsDirectoriesTraversable pins the other half of the
// decision: the MODE CHANGE IS FILES ONLY. If someone later tightens the dirs to
// 0o700 as well, that is a separate call with a migration attached, and this test
// is what makes them notice they are making it.
func TestFileSessionStore_KeepsDirectoriesTraversable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewFileSessionStoreManager(dir)

	if err := store.SaveSession(&models.CapturedSession{ID: "SESSION-00002", TaskID: "TASK-00002"}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "SESSION-00002"))
	if err != nil {
		t.Fatalf("stat the session dir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("session dir has mode %#o, want 0o755", got)
	}
}
