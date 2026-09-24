package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	t.Parallel()
	base := t.TempDir()
	seedRootFile(t, base, ".task_counter", "7")
	seedRootFile(t, base, ".events.jsonl", `{"e":2}`)

	if _, err := NewAppIsolated(base); err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
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

// ---------------------------------------------------------------------------
// Defunct-state cleanup (removeDefunctState)
//
// The two paths below are spelled out as LITERALS rather than read from
// defunctStateFiles on purpose: a test that derives its inputs from the
// production list can only ever confirm the code agrees with itself, and would
// stay green if the list gained a third, wider entry. TestDefunctStateFiles_List
// pins the list itself.
// ---------------------------------------------------------------------------

const defunctCacheAtRoot = ".adb_mcp_cache.json"

var defunctCacheInADB = filepath.Join(".adb", "mcp_cache.json")

// bystanders are seeded into every cleanup case and asserted untouched
// afterwards. They are the blast-radius proof: one lives beside the relocated
// cache INSIDE .adb/, the other is a root dotfile of the kind a glob or a
// prefix match would sweep up.
var cleanupBystanders = map[string]string{
	filepath.Join(".adb", "events.jsonl"): `{"e":1}`,
	".taskrc":                             "org: acme\n",
}

// seedFile writes content at base/rel, creating parents. Used for both the
// defunct files under test and the bystanders.
func seedFile(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed %s: %v", rel, err)
	}
}

func seedBystanders(t *testing.T, base string) {
	t.Helper()
	for rel, content := range cleanupBystanders {
		seedFile(t, base, rel, content)
	}
}

func assertBystandersIntact(t *testing.T, base string) {
	t.Helper()
	for rel, want := range cleanupBystanders {
		if got := readFile(t, filepath.Join(base, rel)); got != want {
			t.Errorf("bystander %s = %q, want %q — cleanup blast radius is too wide", rel, got, want)
		}
	}
}

// runCleanup calls removeDefunctState with a captured warning sink.
func runCleanup(t *testing.T, base string) string {
	t.Helper()
	var warnings bytes.Buffer
	removeDefunctState(base, &warnings)
	return warnings.String()
}

// TestDefunctStateFiles_List pins the deletion list. A new entry here deletes a
// user file on the next `adb` invocation of ANY command, so it should be a
// deliberate, reviewed edit rather than something a refactor can slip in.
func TestDefunctStateFiles_List(t *testing.T) {
	t.Parallel()
	want := []string{defunctCacheAtRoot, defunctCacheInADB}
	if len(defunctStateFiles) != len(want) {
		t.Fatalf("defunctStateFiles = %q, want exactly %q", defunctStateFiles, want)
	}
	for i, w := range want {
		if defunctStateFiles[i] != w {
			t.Errorf("defunctStateFiles[%d] = %q, want %q", i, defunctStateFiles[i], w)
		}
	}
	// Exact basenames only — a pattern would make the blast radius unprovable.
	for _, rel := range defunctStateFiles {
		if strings.ContainsAny(rel, "*?[") {
			t.Errorf("defunctStateFiles entry %q looks like a glob; exact paths only", rel)
		}
		if filepath.IsAbs(rel) {
			t.Errorf("defunctStateFiles entry %q is absolute; must be workspace-relative", rel)
		}
	}
}

// TestRemoveDefunctState covers the removal matrix: each spelling alone, both at
// once, and absence. Every case also asserts the bystanders survived.
func TestRemoveDefunctState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		seed  []string // defunct paths to plant
		quiet bool     // no warning expected
	}{
		{name: "legacy root spelling removed", seed: []string{defunctCacheAtRoot}, quiet: true},
		{name: "relocated .adb spelling removed", seed: []string{defunctCacheInADB}, quiet: true},
		{name: "both spellings removed in one pass", seed: []string{defunctCacheAtRoot, defunctCacheInADB}, quiet: true},
		{name: "absent is a clean no-op", seed: nil, quiet: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			seedBystanders(t, base)
			for _, rel := range tt.seed {
				seedFile(t, base, rel, "stale cache")
			}

			warnings := runCleanup(t, base)

			if tt.quiet && warnings != "" {
				t.Errorf("unexpected warnings: %q", warnings)
			}
			// Both spellings must be gone regardless of which were planted:
			// absence is not an error, so the post-state is the same.
			for _, rel := range defunctStateFiles {
				if exists(filepath.Join(base, rel)) {
					t.Errorf("%s still present after cleanup", rel)
				}
			}
			assertBystandersIntact(t, base)
		})
	}
}

// TestRemoveDefunctState_Idempotent verifies the second run is a silent no-op —
// the steady state after the first cleanup, hit on every subsequent command.
func TestRemoveDefunctState_Idempotent(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	seedBystanders(t, base)
	seedFile(t, base, defunctCacheAtRoot, "stale")
	seedFile(t, base, defunctCacheInADB, "stale")

	if warnings := runCleanup(t, base); warnings != "" {
		t.Fatalf("first run warned: %q", warnings)
	}
	if warnings := runCleanup(t, base); warnings != "" {
		t.Errorf("second run is not a silent no-op: %q", warnings)
	}
	assertBystandersIntact(t, base)
}

// TestRemoveDefunctState_LeavesDirectoryAlone verifies a DIRECTORY occupying a
// defunct path is reported and kept. A directory there is somebody else's data
// that happens to collide with a retired name; deleting a tree is never what a
// one-file cleanup should do.
func TestRemoveDefunctState_LeavesDirectoryAlone(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{defunctCacheAtRoot, defunctCacheInADB} {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			seedBystanders(t, base)
			dir := filepath.Join(base, rel)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", rel, err)
			}
			seedFile(t, base, filepath.Join(rel, "child.txt"), "keep me")

			warnings := runCleanup(t, base)

			if !strings.Contains(warnings, "not a regular file (a directory)") {
				t.Errorf("warnings = %q, want a not-a-regular-file (a directory) warning", warnings)
			}
			info, err := os.Lstat(dir)
			if err != nil {
				t.Fatalf("directory at %s was removed: %v", rel, err)
			}
			if !info.IsDir() {
				t.Fatalf("%s is no longer a directory", rel)
			}
			if got := readFile(t, filepath.Join(dir, "child.txt")); got != "keep me" {
				t.Errorf("child.txt = %q, want %q", got, "keep me")
			}
			assertBystandersIntact(t, base)
		})
	}
}

// TestRemoveDefunctState_DoesNotFollowSymlink verifies a symlink at a defunct
// path is neither removed nor followed. os.Lstat describes the link itself, so
// the mode check refuses it — os.Stat would have resolved it to its target and
// happily called it "a regular file", which is how a cleanup ends up deleting
// something outside the workspace.
func TestRemoveDefunctState_DoesNotFollowSymlink(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{defunctCacheAtRoot, defunctCacheInADB} {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			seedBystanders(t, base)

			// The symlink target stands in for anything a link could point at —
			// here inside the temp dir so the test is self-contained.
			seedFile(t, base, filepath.Join("elsewhere", "precious.txt"), "do not delete")
			target := filepath.Join(base, "elsewhere", "precious.txt")
			link := filepath.Join(base, rel)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatalf("mkdir for link: %v", err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable on this platform: %v", err)
			}

			warnings := runCleanup(t, base)

			if !strings.Contains(warnings, "not a regular file (a symlink)") {
				t.Errorf("warnings = %q, want a not-a-regular-file (a symlink) warning", warnings)
			}
			if _, err := os.Lstat(link); err != nil {
				t.Errorf("symlink at %s was removed: %v", rel, err)
			}
			if got := readFile(t, target); got != "do not delete" {
				t.Errorf("symlink target = %q, want %q — the link was followed", got, "do not delete")
			}
			assertBystandersIntact(t, base)
		})
	}
}

// TestNewApp_RemovesDefunctStateOnInit is the end-to-end wiring check: the
// cleanup runs in the body all three constructors share, so building an App over
// a workspace carrying either cache spelling clears both — while the legacy
// migration and the bystanders are unaffected.
func TestNewApp_RemovesDefunctStateOnInit(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	seedBystanders(t, base)
	seedFile(t, base, defunctCacheAtRoot, "stale root cache")
	seedFile(t, base, defunctCacheInADB, "stale relocated cache")
	seedRootFile(t, base, ".task_counter", "9")

	if _, err := NewAppIsolated(base); err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}

	for _, rel := range defunctStateFiles {
		if exists(filepath.Join(base, rel)) {
			t.Errorf("%s survived App construction", rel)
		}
	}
	// The relocation still happened, and nothing else was touched.
	if got := readFile(t, filepath.Join(base, ".adb", "task_counter")); got != "9" {
		t.Errorf(".adb/task_counter = %q, want 9", got)
	}
	assertBystandersIntact(t, base)
}

// TestMigrateStateToADB_DoesNotRelocateDefunctCache guards the division of
// labour: the retired MCP cache must not reappear in the migration table. If it
// did, a legacy root copy would be moved into .adb/ instead of deleted, and the
// orphan would be resurrected in a tidier place.
func TestMigrateStateToADB_DoesNotRelocateDefunctCache(t *testing.T) {
	t.Parallel()
	for _, f := range legacyStateFiles {
		if f.legacy == defunctCacheAtRoot {
			t.Errorf("legacyStateFiles still relocates %q; it belongs in defunctStateFiles", f.legacy)
		}
		if f.target == "mcp_cache.json" {
			t.Errorf("legacyStateFiles still targets %q inside .adb/", f.target)
		}
	}

	base := t.TempDir()
	seedFile(t, base, defunctCacheAtRoot, "stale")
	if err := migrateStateToADB(base); err != nil {
		t.Fatalf("migrateStateToADB: %v", err)
	}
	if exists(filepath.Join(base, defunctCacheInADB)) {
		t.Error("migration relocated the defunct cache into .adb/")
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
