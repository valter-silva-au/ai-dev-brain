package core

import (
	"os"
	"path/filepath"
	"testing"
)

// mkTicketDir creates tickets/<sub>/ under root and returns its absolute path.
func mkTicketDir(t *testing.T, root, sub string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(sub))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sub, err)
	}
	return full
}

// buildTicketsTree lays out a representative tickets/ tree spanning every
// layout the resolver must cope with: nested platform/org/repo, _local,
// legacy flat, and an _archived copy.
func buildTicketsTree(t *testing.T) (ticketsDir string, want map[string]string) {
	t.Helper()
	ticketsDir = t.TempDir()
	want = map[string]string{
		"TASK-00001": mkTicketDir(t, ticketsDir, "github.com/awslabs/mcp/TASK-00001-fix-auth-timeout"),
		"TASK-00002": mkTicketDir(t, ticketsDir, "_local/TASK-00002-kb-note"),
		"TASK-00003": mkTicketDir(t, ticketsDir, "TASK-00003"), // legacy flat
	}
	// A decoy whose id shares a numeric prefix with TASK-1 — prefix safety.
	mkTicketDir(t, ticketsDir, "github.com/x/y/TASK-00010-decoy")
	return ticketsDir, want
}

func TestResolveTicketDir(t *testing.T) {
	ticketsDir, want := buildTicketsTree(t)

	for id, dir := range want {
		got, err := ResolveTicketDir(ticketsDir, id)
		if err != nil {
			t.Errorf("ResolveTicketDir(%s) unexpected error: %v", id, err)
			continue
		}
		if got != dir {
			t.Errorf("ResolveTicketDir(%s) = %q, want %q", id, got, dir)
		}
	}
}

func TestResolveTicketDir_NotFound(t *testing.T) {
	ticketsDir, _ := buildTicketsTree(t)
	if got, err := ResolveTicketDir(ticketsDir, "TASK-99999"); err == nil {
		t.Errorf("ResolveTicketDir(TASK-99999) = %q, want error", got)
	}
}

// TestResolveTicketDir_PrefixSafety ensures a short id does not fuzzy-match a
// longer id that begins with the same digits (TASK-1 must not resolve to
// TASK-00010-decoy or any TASK-1x dir).
func TestResolveTicketDir_PrefixSafety(t *testing.T) {
	ticketsDir := t.TempDir()
	mkTicketDir(t, ticketsDir, "github.com/x/y/TASK-00010-decoy")
	// Neither an exact "TASK-0001" nor a "TASK-0001-*" dir exists.
	if got, err := ResolveTicketDir(ticketsDir, "TASK-0001"); err == nil {
		t.Errorf("ResolveTicketDir(TASK-0001) = %q, want error (must not prefix-match TASK-00010-decoy)", got)
	}
}

// TestResolveTicketDir_PrefersActiveOverArchived: when the same id exists both
// live and under _archived/, the live (non-archived) copy wins.
func TestResolveTicketDir_PrefersActiveOverArchived(t *testing.T) {
	ticketsDir := t.TempDir()
	active := mkTicketDir(t, ticketsDir, "github.com/awslabs/mcp/TASK-00004-live")
	mkTicketDir(t, ticketsDir, "_archived/github.com/awslabs/mcp/TASK-00004-old")

	got, err := ResolveTicketDir(ticketsDir, "TASK-00004")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != active {
		t.Errorf("ResolveTicketDir(TASK-00004) = %q, want the active copy %q", got, active)
	}
}

// TestResolveTicketDir_ArchivedOnly: an id that exists only under _archived/
// still resolves (to the archived dir).
func TestResolveTicketDir_ArchivedOnly(t *testing.T) {
	ticketsDir := t.TempDir()
	archived := mkTicketDir(t, ticketsDir, "_archived/github.com/x/y/TASK-00005-gone")

	got, err := ResolveTicketDir(ticketsDir, "TASK-00005")
	if err != nil {
		t.Fatalf("unexpected error resolving archived-only id: %v", err)
	}
	if got != archived {
		t.Errorf("ResolveTicketDir(TASK-00005) = %q, want %q", got, archived)
	}
}
