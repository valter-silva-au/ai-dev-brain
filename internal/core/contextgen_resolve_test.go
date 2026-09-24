package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug these tests pin (found in TASK-00039 Batch 4, by driving the binary
// after a gosec G703 finding pointed at the write):
//
// GenerateTaskContext built its target as filepath.Join(ticketsDir, taskID) — the
// FLAT layout. Tickets have not been flat since the correlation layout landed
// (L100 §5): a real ticket lives at tickets/<platform>/<org>/<repo>/TASK-id-slug/
// or tickets/_local/TASK-id-slug/. So for every non-legacy ticket the command
// printed "✓ Task context regenerated" while:
//
//  1. writing to tickets/TASK-id/context.md, a directory it INVENTED;
//  2. leaving the real ticket's context.md untouched and stale; and
//  3. littering tickets/ with a phantom dir whose base name is exactly the id —
//     which ResolveTicketDir then matches, and which is SHALLOWER than the real
//     one, so the phantom can win future resolution.
//
// The caller already knew better: internal/cli/sync.go resolves the id to decide
// whether it is known, then discarded the resolution and passed the raw id down.
// The fix routes core through ResolveTicketDir, which is also what makes the G703
// traversal unreachable at the core boundary rather than only at the CLI's.

// seedTicket creates a ticket directory at a nested path with a context.md whose
// content is recognisable, and returns the context.md path.
func seedTicket(t *testing.T, ticketsDir, relDir string) string {
	t.Helper()

	dir := filepath.Join(ticketsDir, relDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed ticket dir %s: %v", relDir, err)
	}
	path := filepath.Join(dir, "context.md")
	if err := os.WriteFile(path, []byte("ORIGINAL SEEDED CONTENT\n"), 0o644); err != nil {
		t.Fatalf("seed context.md: %v", err)
	}
	return path
}

func TestGenerateTaskContext_WritesIntoTheNestedTicketDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		relDir string
	}{
		// The two live layouts (L100 §5 cases 1 and 3).
		{"repo-backed", filepath.Join("github.com", "acme", "thing", "TASK-00002-probe")},
		{"repo-less _local", filepath.Join("_local", "TASK-00002-probe")},
		// The legacy flat layout must keep working — it is case 4, not a bug.
		{"legacy flat", "TASK-00002"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			ticketsDir := filepath.Join(base, "tickets")
			realContext := seedTicket(t, ticketsDir, tc.relDir)

			cg := NewContextGenerator(
				filepath.Join(base, "backlog.yaml"), ticketsDir, base, nil,
			)

			// hookMode=true takes the append path, which needs no template
			// manager — the same os.WriteFile the G703 finding named.
			if err := cg.GenerateTaskContext("TASK-00002", true); err != nil {
				t.Fatalf("GenerateTaskContext: %v", err)
			}

			// (1) The REAL ticket's context.md must have been updated.
			got, err := os.ReadFile(realContext)
			if err != nil {
				t.Fatalf("read the real context.md: %v", err)
			}
			if !strings.Contains(string(got), "ORIGINAL SEEDED CONTENT") {
				t.Errorf("the real context.md lost its existing content; got %q", got)
			}
			if !strings.Contains(string(got), "## Updated:") {
				t.Errorf(
					"the real ticket's context.md at %s was NOT regenerated; got %q",
					tc.relDir, got,
				)
			}

			// (2) No phantom flat directory may be invented. For the legacy-flat
			// case the ticket genuinely IS tickets/TASK-00002, so skip it there.
			if tc.relDir != "TASK-00002" {
				phantom := filepath.Join(ticketsDir, "TASK-00002")
				if _, err := os.Stat(phantom); err == nil {
					t.Errorf(
						"invented a phantom ticket dir at %s — it shadows the real "+
							"ticket in ResolveTicketDir, being shallower",
						phantom,
					)
				}
			}
		})
	}
}

// The next two tests deliberately use hookMode=FALSE, and that is the whole point
// of them. hookMode=true takes an early `return os.WriteFile(...)` with no
// MkdirAll, so a bad id fails on ENOENT and the test would pass against the
// unfixed code — vacuously. os.MkdirAll on the non-hook path is what actually
// INVENTS a directory, so it is the only path on which "does a bad id create a
// ticket dir?" is a real question. (A guard that passes against the bug it names is
// worse than no guard; Batch 2c shipped one and only a negative-control pass
// caught it.)

// TestGenerateTaskContext_RejectsAnUnknownID is the counterpart to resolving: a
// caller can no longer bring a ticket dir into existence just by naming an id. An
// id with no ticket on disk must be an error, not a silent "✓" over a freshly
// invented directory.
func TestGenerateTaskContext_RejectsAnUnknownID(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	ticketsDir := filepath.Join(base, "tickets")
	if err := os.MkdirAll(ticketsDir, 0o755); err != nil {
		t.Fatalf("mkdir tickets: %v", err)
	}

	cg := NewContextGenerator(
		filepath.Join(base, "backlog.yaml"), ticketsDir, base, NewMockTemplateManager(),
	)

	if err := cg.GenerateTaskContext("TASK-99999", false); err == nil {
		t.Fatal("GenerateTaskContext(unknown id) = nil error, want a rejection")
	}
	if _, err := os.Stat(filepath.Join(ticketsDir, "TASK-99999")); err == nil {
		t.Error("an unknown id created a ticket directory")
	}
}

// TestGenerateTaskContext_RefusesToTraverse is the G703 finding itself, pinned at
// the CORE boundary. internal/cli guards this today via taskIsKnown, but
// GenerateTaskContext is on the exported ContextGenerator interface, so a second
// caller would not inherit that guard.
func TestGenerateTaskContext_RefusesToTraverse(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		filepath.Join("..", "OUTSIDE"),
		filepath.Join("..", "..", "OUTSIDE"),
		"/etc/adb-should-never-write-here",
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			base := t.TempDir()
			ticketsDir := filepath.Join(base, "tickets")
			if err := os.MkdirAll(ticketsDir, 0o755); err != nil {
				t.Fatalf("mkdir tickets: %v", err)
			}
			outside := filepath.Join(base, "OUTSIDE")

			cg := NewContextGenerator(
				filepath.Join(base, "backlog.yaml"), ticketsDir, base,
				NewMockTemplateManager(),
			)
			if err := cg.GenerateTaskContext(id, false); err == nil {
				t.Errorf("GenerateTaskContext(%q) = nil error, want a rejection", id)
			}
			if _, err := os.Stat(outside); err == nil {
				t.Errorf("GenerateTaskContext(%q) wrote outside the tickets dir", id)
			}
		})
	}
}
