package internal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
)

// legacyStateFile pairs a legacy root-level state basename with its target
// basename inside .adb/. The .adb_/leading-dot prefix is dropped in the target
// because .adb/ already namespaces the file (#186). The target column is the
// statedir.File* consts, not string literals: the writers routed through
// App.StatePath (#189/#190) reference the same consts, so "the target basenames
// must not drift" is enforced by the compiler rather than by this comment. (The
// VS Code extension re-encodes them in feedpath.ts as it cannot import Go.)
type legacyStateFile struct {
	legacy string // basename at the workspace root, e.g. ".adb_scheduler.log"
	target string // basename inside .adb/, a statedir.File* const, e.g. "scheduler.log"
}

// legacyStateFiles is the full adb-owned set relocated into .adb/ (#186). It
// covers the scheduler triad, the automation cursor, the hook session/evidence
// trackers, the counters, the context-state file, the two event logs, and the
// SQLite memory store plus its -shm/-wal siblings. $HOME-level files
// (~/.adb_terminal_*.json) are deliberately out of scope — they are not under
// basePath. The list is ordered deterministically for stable behaviour.
//
// A file whose feature no longer exists does NOT belong here: relocating dead
// data only moves the confusion into .adb/. Such a name goes in
// defunctStateFiles below, which deletes it.
var legacyStateFiles = []legacyStateFile{
	{".adb_scheduler.log", statedir.FileSchedulerLog},
	{".adb_scheduler.pid", statedir.FileSchedulerPID},
	{".adb_scheduler_state.yaml", statedir.FileSchedulerState},
	{".adb_automation_cursor", statedir.FileAutomationCursor},
	{".adb_session_changes", statedir.FileSessionChanges},
	{".adb_evidence_reads", statedir.FileEvidenceReads},
	{".task_counter", statedir.FileTaskCounter},
	{".session_counter", statedir.FileSessionCounter},
	{".context_state.yaml", statedir.FileContextState},
	{".events.jsonl", statedir.FileEventsLog},
	{".governance.jsonl", statedir.FileGovernanceLog},
	// SQLite memory store trio — moved as a set. Each moves independently under
	// the same move-if-absent rule; -shm/-wal may be absent (clean close), so
	// gating on them would be wrong. Seeding all three and running once moves
	// all three, keeping the store openable with its embedder metadata intact.
	// The -shm/-wal siblings are derived from the store const so the whole trio
	// stays pinned to one source of truth.
	{".adb_memory.sqlite", statedir.FileMemoryDB},
	{".adb_memory.sqlite-shm", statedir.FileMemoryDB + "-shm"},
	{".adb_memory.sqlite-wal", statedir.FileMemoryDB + "-wal"},
}

// defunctStateFiles are workspace-relative paths to adb-owned state files whose
// FEATURE no longer exists. They are deleted, not relocated: a cache belonging to
// a removed writer is dead data, and moving it into .adb/ would only preserve the
// question "what reads this?" in a tidier location. Give a newly-obsolete state
// file a row here (in both spellings it can have on disk) and it stops being
// somebody's future archaeology.
//
// Two rules make the blast radius provable, because these entries DELETE USER
// FILES on the next `adb` invocation of any kind:
//
//  1. Exact, literal paths naming one basename each — no globs, no directory
//     walks, no prefix matching. One entry can remove at most one file, so the
//     blast radius is auditable by reading the list. A pattern would not be.
//  2. Regular files only (checked with os.Lstat, which does not follow symlinks).
//     A directory at one of these paths is somebody else's data that happens to
//     collide with a retired name; a symlink there could point anywhere, and
//     os.Remove on the link plus a future writer following it is not a risk worth
//     taking for a cleanup. Both are reported and skipped — see removeDefunctState.
//
// Current membership — the MCP health-check TTL cache, in both spellings:
//   - ".adb_mcp_cache.json" is the pre-#186 workspace-root spelling, the one the
//     removed writer actually wrote (commit cdb16bc).
//   - ".adb/mcp_cache.json" is where a #186 migration would have put it before
//     that row left legacyStateFiles, so an already-migrated workspace has it here.
//
// The basename is a literal on purpose: statedir.FileMCPCache is gone, and
// reintroducing a const for a file nothing reads is what made it an orphan in the
// first place. See the "Retired names" note in internal/statedir/statedir.go.
var defunctStateFiles = []string{
	".adb_mcp_cache.json",
	filepath.Join(statedir.Name, "mcp_cache.json"),
}

// migrateStateToADB is the one-shot migration that relocates existing
// root-level state files into .adb/ so upgrading a workspace does not appear
// "reset" (#186, ticket #188). For each known legacy path: if the legacy file
// exists and its .adb/ target does not, it is moved (rename, falling back to
// copy+remove across devices). It is idempotent (absent legacy or present
// target → skip), never overwrites a present target (no data loss), and moves
// only the files actually present (a partial legacy set migrates partially).
//
// Errors are aggregated and returned rather than swallowed, but the caller
// (NewApp) treats a migration error as non-fatal so a read-only command still
// runs — the migration is a convenience, not a precondition.
func migrateStateToADB(basePath string) error {
	var errs []error
	for _, f := range legacyStateFiles {
		legacy := filepath.Join(basePath, f.legacy)
		target := filepath.Join(basePath, stateDirName, f.target)
		if err := moveIfAbsent(legacy, target); err != nil {
			errs = append(errs, fmt.Errorf("migrate %s -> %s/%s: %w", f.legacy, stateDirName, f.target, err))
		}
	}
	return errors.Join(errs...)
}

// removeDefunctState deletes the defunctStateFiles that are present under
// basePath — the counterpart to migrateStateToADB for state whose feature was
// removed rather than relocated (see defunctStateFiles for what qualifies).
//
// It reports rather than returns: warnings go to warn (os.Stderr in the App
// constructor, a buffer in tests; a nil warn discards them) and the function
// always returns. That matches how the caller already treats a migration error —
// `fmt.Fprintf(os.Stderr, "warning: …")` and carry on — and it is the only
// defensible shape here, because this runs on the way to building an App for
// EVERY command: a read-only filesystem, a root-owned leftover, or an
// unreadable directory must cost a warning, never `adb task list`.
//
// Properties it guarantees:
//   - idempotent: a removed file is absent on the next run, and absence is not an
//     error, so running twice (or a thousand times) is a no-op after the first.
//   - one path per entry: each entry is joined to basePath and acted on directly.
//     Nothing is globbed, walked, or pattern-matched.
//   - never a directory, never through a symlink: os.Lstat describes the path
//     ITSELF (unlike os.Stat, which resolves a symlink to its target and would let
//     an `.adb/mcp_cache.json -> ~/.ssh/id_rsa` decide what "is a regular file"
//     means). Anything that is not a regular file is reported and left alone, so
//     the worst case is a warning about a name collision rather than a deletion
//     somewhere outside the workspace.
func removeDefunctState(basePath string, warn io.Writer) {
	warnf := func(format string, args ...interface{}) {
		if warn == nil {
			return
		}
		fmt.Fprintf(warn, "warning: "+format+"\n", args...)
	}

	for _, rel := range defunctStateFiles {
		path := filepath.Join(basePath, rel)

		// Lstat, not Stat: describe the path itself so a symlink is caught here
		// rather than silently resolved to whatever it points at.
		info, err := os.Lstat(path)
		if err != nil {
			// Absent is the overwhelmingly common case (and the post-cleanup
			// steady state) — say nothing. Anything else is worth a line.
			if !os.IsNotExist(err) {
				warnf("could not inspect obsolete state file %s: %v", path, err)
			}
			continue
		}

		// Only a regular file is ours to delete. A directory, a symlink, a socket
		// or a device at this path is something else that happens to collide with
		// a retired name; naming the mode makes the warning actionable.
		if !info.Mode().IsRegular() {
			warnf("leaving obsolete state path %s alone: not a regular file (%s)", path, describeNonRegular(info.Mode()))
			continue
		}

		if err := os.Remove(path); err != nil {
			warnf("could not remove obsolete state file %s: %v", path, err)
		}
	}
}

// describeNonRegular names the two collisions that actually happen at a state
// path — a directory and a symlink — in words rather than as os.FileMode's
// dashed rendering ("d---------"), which reads as noise in a warning a user is
// meant to act on. Anything else falls back to the mode's type bits.
func describeNonRegular(mode os.FileMode) string {
	switch {
	case mode.IsDir():
		return "a directory"
	case mode&os.ModeSymlink != 0:
		return "a symlink"
	default:
		return "mode " + mode.Type().String()
	}
}

// moveIfAbsent moves src to dst only when src exists and dst does not. A missing
// src or a present dst is a no-op (nil), which is what makes migrateStateToADB
// idempotent and non-destructive: it never overwrites an existing target. The
// move is os.Rename, falling back to copy+remove when rename fails (e.g. src and
// dst on different filesystems, which surfaces as EXDEV) — kept portable by
// attempting the copy fallback on any rename error rather than matching errnos.
func moveIfAbsent(src, dst string) error {
	// Skip when the legacy source is absent — nothing to migrate.
	if _, err := os.Lstat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Never overwrite a present target (user story 5: no silent data loss).
	if _, err := os.Lstat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err != nil {
		// Cross-device rename fails (e.g. a bind-mounted /tmp); fall back to
		// copy+remove. A genuine permission error re-surfaces from the copy.
		if cerr := copyThenRemove(src, dst); cerr != nil {
			return fmt.Errorf("rename failed (%v); copy fallback: %w", err, cerr)
		}
	}
	return nil
}

// copyThenRemove copies src to dst then removes src — the cross-device fallback
// for moveIfAbsent. src is a regular state file; dst's parent already exists.
// The destination is created with the source's permission bits so a migrated
// file keeps its mode rather than picking up os.Create's 0o666&umask.
func copyThenRemove(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}
