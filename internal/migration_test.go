package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// seedRootFile writes a legacy state file at the workspace root with known
// content so a move can be asserted by reading it back at the new path.
func seedRootFile(t *testing.T, base, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0o644); err != nil {
		t.Fatalf("seed %s: %v", name, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// TestMigrateStateToADB_MovesLegacyWhenTargetAbsent is the core acceptance:
// a legacy root file with no .adb/ target is relocated and readable at the new
// path, and the root copy is gone.
func TestMigrateStateToADB_MovesLegacyWhenTargetAbsent(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".task_counter", "42")

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB: %v", err)
	}

	target := filepath.Join(base, ".adb", "task_counter")
	if got := readFile(t, target); got != "42" {
		t.Errorf("target content = %q, want %q", got, "42")
	}
	if exists(filepath.Join(base, ".task_counter")) {
		t.Error("legacy .task_counter still present after migration")
	}
}

// TestMigrateStateToADB_PresentTargetNotOverwritten guards user story 5: if the
// .adb/ target already exists, the legacy file is left untouched and the target
// keeps its content — never a silent overwrite.
func TestMigrateStateToADB_PresentTargetNotOverwritten(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, ".adb"), 0o755); err != nil {
		t.Fatalf("mkdir .adb: %v", err)
	}
	seedRootFile(t, base, ".task_counter", "legacy")
	if err := os.WriteFile(filepath.Join(base, ".adb", "task_counter"), []byte("target"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB: %v", err)
	}

	if got := readFile(t, filepath.Join(base, ".adb", "task_counter")); got != "target" {
		t.Errorf("target overwritten: got %q, want %q", got, "target")
	}
	if !exists(filepath.Join(base, ".task_counter")) {
		t.Error("legacy file removed even though target was present (data loss)")
	}
}

// TestMigrateStateToADB_NoLegacyIsNoop verifies a fresh workspace (no legacy
// files) migrates nothing and does not error.
func TestMigrateStateToADB_NoLegacyIsNoop(t *testing.T) {
	base := t.TempDir()

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB on empty workspace: %v", err)
	}

	// No .adb/ children were created for absent legacy files.
	entries, _ := os.ReadDir(filepath.Join(base, ".adb"))
	if len(entries) != 0 {
		t.Errorf("migration created %d files on an empty workspace, want 0", len(entries))
	}
}

// TestMigrateStateToADB_Idempotent verifies a second run after a successful
// migration changes nothing (user story 4).
func TestMigrateStateToADB_Idempotent(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".events.jsonl", `{"e":1}`)

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Second run must be a clean no-op.
	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("second migrate (idempotent): %v", err)
	}

	if got := readFile(t, filepath.Join(base, ".adb", "events.jsonl")); got != `{"e":1}` {
		t.Errorf("content changed after idempotent re-run: %q", got)
	}
	if exists(filepath.Join(base, ".events.jsonl")) {
		t.Error("legacy .events.jsonl reappeared at root")
	}
}

// TestMigrateStateToADB_SQLiteTrioMovedTogether verifies the memory-store trio
// (.sqlite + -shm + -wal) all relocate in a single run (user story 9).
func TestMigrateStateToADB_SQLiteTrioMovedTogether(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".adb_memory.sqlite", "db")
	seedRootFile(t, base, ".adb_memory.sqlite-shm", "shm")
	seedRootFile(t, base, ".adb_memory.sqlite-wal", "wal")

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB: %v", err)
	}

	for name, want := range map[string]string{
		"memory.sqlite":     "db",
		"memory.sqlite-shm": "shm",
		"memory.sqlite-wal": "wal",
	} {
		if got := readFile(t, filepath.Join(base, ".adb", name)); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if exists(filepath.Join(base, ".adb_memory.sqlite")) {
		t.Error("legacy .adb_memory.sqlite still at root")
	}
}

// TestMigrateStateToADB_PartialSetOnlyMovesPresent verifies that when only some
// legacy files exist, only those move — a clean-close SQLite (no -shm/-wal) is
// the realistic partial-trio case.
func TestMigrateStateToADB_PartialSetOnlyMovesPresent(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".adb_memory.sqlite", "db")
	seedRootFile(t, base, ".adb_scheduler.pid", "1234")
	// Deliberately no -shm/-wal, no other state.

	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB: %v", err)
	}

	if got := readFile(t, filepath.Join(base, ".adb", "memory.sqlite")); got != "db" {
		t.Errorf("memory.sqlite = %q, want db", got)
	}
	if got := readFile(t, filepath.Join(base, ".adb", "scheduler.pid")); got != "1234" {
		t.Errorf("scheduler.pid = %q, want 1234", got)
	}
	// Absent legacy files must not have been conjured under .adb/.
	if exists(filepath.Join(base, ".adb", "memory.sqlite-shm")) {
		t.Error("memory.sqlite-shm created despite absent legacy")
	}
	if exists(filepath.Join(base, ".adb", "events.jsonl")) {
		t.Error("events.jsonl created despite absent legacy")
	}
}

// TestMigrateStateToADB_ErrorReportedNotSwallowed verifies a migration failure
// is surfaced (user story 20). Seeding a legacy .adb_scheduler.pid while a
// *directory* occupies its target basename forces a non-overwrite path... but
// a present target is a no-op, so instead we make .adb itself a regular file so
// MkdirAll of the state dir fails — the error must propagate, not vanish.
func TestMigrateStateToADB_ErrorReportedNotSwallowed(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".task_counter", "42")
	// Occupy the .adb path with a regular file so creating the state dir fails.
	if err := os.WriteFile(filepath.Join(base, ".adb"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seed .adb as file: %v", err)
	}

	err := migrateStateToADB(base)
	if err == nil {
		t.Fatal("expected a migration error when .adb is a regular file, got nil")
	}
	// The legacy file must be left intact (the move never completed).
	if !exists(filepath.Join(base, ".task_counter")) {
		t.Error("legacy file lost after a failed migration")
	}
}

// TestMigrateStateToADB_TargetBasenamesMatchStatePath is a contract guard: every
// migration target must be the exact path App.StatePath produces, so the
// migrated file lands where the (soon-to-be-routed) writers will look for it.
func TestMigrateStateToADB_TargetBasenamesMatchStatePath(t *testing.T) {
	app := &App{BasePath: t.TempDir()}
	for _, f := range legacyStateFiles {
		want := app.StatePath(f.target)
		got := filepath.Join(app.BasePath, stateDirName, f.target)
		if got != want {
			t.Errorf("target %q: StatePath=%q, migration builds %q", f.target, want, got)
		}
		// The target must never keep the redundant leading dot / .adb_ prefix.
		if len(f.target) > 0 && f.target[0] == '.' {
			t.Errorf("target %q should not start with a dot inside .adb/", f.target)
		}
	}
}

// TestNewApp_MigratesLegacyStateOnInit verifies the end-to-end wiring: building
// an App over a workspace with legacy root state relocates that state into .adb/.
func TestNewApp_MigratesLegacyStateOnInit(t *testing.T) {
	base := t.TempDir()
	seedRootFile(t, base, ".task_counter", "7")
	seedRootFile(t, base, ".events.jsonl", `{"e":2}`)

	if _, err := NewApp(base); err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	if got := readFile(t, filepath.Join(base, ".adb", "task_counter")); got != "7" {
		t.Errorf(".adb/task_counter = %q, want 7", got)
	}
	if got := readFile(t, filepath.Join(base, ".adb", "events.jsonl")); got != `{"e":2}` {
		t.Errorf(".adb/events.jsonl = %q, want the seeded content", got)
	}
	if exists(filepath.Join(base, ".task_counter")) {
		t.Error("legacy .task_counter still at root after NewApp")
	}
}

// TestCopyThenRemove_PreservesSourceMode is the code-review follow-up: the
// cross-device migration fallback must preserve the source file's permission
// bits (CLAUDE.md "files 0o644"), not open the destination at os.Create's
// 0o666&umask. Exercised directly since forcing an EXDEV rename in a unit test
// isn't portable.
func TestCopyThenRemove_PreservesSourceMode(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	if err := copyThenRemove(src, dst); err != nil {
		t.Fatalf("copyThenRemove: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("dst mode = %o, want 0o600 (source mode preserved)", got)
	}
	if got := readFile(t, dst); got != "payload" {
		t.Errorf("dst content = %q, want payload", got)
	}
	if exists(src) {
		t.Error("src should be removed after copyThenRemove")
	}
}
