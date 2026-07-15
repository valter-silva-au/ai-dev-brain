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
// trackers, the MCP cache, the counters, the context-state file, the two event
// logs, and the SQLite memory store plus its -shm/-wal siblings. $HOME-level
// files (~/.adb_terminal_*.json) are deliberately out of scope — they are not
// under basePath. The list is ordered deterministically for stable behaviour.
var legacyStateFiles = []legacyStateFile{
	{".adb_scheduler.log", statedir.FileSchedulerLog},
	{".adb_scheduler.pid", statedir.FileSchedulerPID},
	{".adb_scheduler_state.yaml", statedir.FileSchedulerState},
	{".adb_automation_cursor", statedir.FileAutomationCursor},
	{".adb_session_changes", statedir.FileSessionChanges},
	{".adb_evidence_reads", statedir.FileEvidenceReads},
	{".adb_mcp_cache.json", statedir.FileMCPCache},
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
