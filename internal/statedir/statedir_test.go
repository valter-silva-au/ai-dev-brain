package statedir

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDir returns the .adb/ directory under basePath — the parent of every
// Path(basePath, name).
func TestDir(t *testing.T) {
	base := t.TempDir()
	want := filepath.Join(base, Name)
	if got := Dir(base); got != want {
		t.Errorf("Dir(%q) = %q, want %q", base, got, want)
	}
}

// TestPathIsInsideDir guards the invariant Ensure/Dir rely on: Path(base, name)
// is always Dir(base)/name, so ensuring Dir is enough for any Path to be writable.
func TestPathIsInsideDir(t *testing.T) {
	base := t.TempDir()
	got := Path(base, FileTaskCounter)
	want := filepath.Join(Dir(base), FileTaskCounter)
	if got != want {
		t.Errorf("Path(base, %q) = %q, want %q", FileTaskCounter, got, want)
	}
}

// TestEnsureCreatesStateDir is the core acceptance for the helper: Ensure makes
// the .adb/ directory (0o755) so a subsequent Path write finds its parent.
func TestEnsureCreatesStateDir(t *testing.T) {
	base := t.TempDir()

	if err := Ensure(base); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	info, err := os.Stat(Dir(base))
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", Dir(base))
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("state dir mode = %o, want 0o755", got)
	}
}

// TestEnsureIdempotent verifies a second Ensure on an existing dir is a clean
// no-op (MkdirAll semantics) — writers call it on every operation.
func TestEnsureIdempotent(t *testing.T) {
	base := t.TempDir()
	if err := Ensure(base); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	// Drop a file in; a second Ensure must not disturb it or error.
	marker := Path(base, FileTaskCounter)
	if err := os.WriteFile(marker, []byte("1"), 0o644); err != nil {
		t.Fatalf("seed marker: %v", err)
	}
	if err := Ensure(base); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker disturbed by idempotent Ensure: %v", err)
	}
}

// TestEnsureErrorsWhenStateDirPathIsFile verifies Ensure surfaces the error
// when the .adb path is occupied by a regular file (can't become a dir) rather
// than swallowing it — mirrors migrateStateToADB's fail-loud contract.
func TestEnsureErrorsWhenStateDirPathIsFile(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(Dir(base), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed .adb as file: %v", err)
	}
	if err := Ensure(base); err == nil {
		t.Fatal("expected an error when .adb is a regular file, got nil")
	}
}

// TestStateFileBasenames is the shared-contract guard: every exported state-file
// basename const must be a bare, dot-free, unique basename so it namespaces
// cleanly under .adb/ (the migration table drops the legacy leading dot / .adb_
// prefix precisely so these are clean).
func TestStateFileBasenames(t *testing.T) {
	names := map[string]string{
		"FileTaskCounter":      FileTaskCounter,
		"FileSessionCounter":   FileSessionCounter,
		"FileContextState":     FileContextState,
		"FileEventsLog":        FileEventsLog,
		"FileGovernanceLog":    FileGovernanceLog,
		"FileSchedulerLog":     FileSchedulerLog,
		"FileSchedulerPID":     FileSchedulerPID,
		"FileSchedulerState":   FileSchedulerState,
		"FileAutomationCursor": FileAutomationCursor,
		"FileSessionChanges":   FileSessionChanges,
		"FileEvidenceReads":    FileEvidenceReads,
		"FileMCPCache":         FileMCPCache,
		"FileMemoryDB":         FileMemoryDB,
	}
	seen := map[string]string{}
	for constName, value := range names {
		if value == "" {
			t.Errorf("%s is empty", constName)
		}
		if len(value) > 0 && value[0] == '.' {
			t.Errorf("%s = %q must not start with a dot (.adb/ already namespaces it)", constName, value)
		}
		if value != filepath.Base(value) {
			t.Errorf("%s = %q is not a bare basename", constName, value)
		}
		if prev, dup := seen[value]; dup {
			t.Errorf("%s and %s share the same value %q", constName, prev, value)
		}
		seen[value] = constName
	}
}

// TestEnsureThenWrite is an end-to-end shape check: Ensure(base) then writing to
// Path(base, name) succeeds, which is exactly the writer call pattern this helper
// replaces (os.MkdirAll(filepath.Dir(Path(...))) then open).
func TestEnsureThenWrite(t *testing.T) {
	base := t.TempDir()
	if err := Ensure(base); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	p := Path(base, FileEventsLog)
	if err := os.WriteFile(p, []byte(`{"e":1}`), 0o644); err != nil {
		t.Fatalf("write under ensured dir: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("stat written file: %v", err)
	}
}
