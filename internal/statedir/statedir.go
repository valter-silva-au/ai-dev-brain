// Package statedir defines the single convention for where adb keeps its
// private per-workspace state: a .adb/ directory under the workspace root
// (#186). It is a stdlib-only leaf so every layer — package internal (via
// App.StatePath), the CLI, core, and hooks — can route its state paths through
// one place without an import cycle. Adding a new state file anywhere means
// calling Path here, so the workspace root never re-accretes .adb_* clutter.
package statedir

import (
	"os"
	"path/filepath"
)

// Name is the fixed state-directory basename under the workspace root. It is
// already a denied segment in the cloud-sync allowlist, so grouping state here
// keeps it out of the archive by the existing rule.
const Name = ".adb"

// The state-file basenames adb keeps under .adb/. Each is the bare name inside
// .adb/ (no leading dot / .adb_ prefix — .adb/ already namespaces the file).
// These are the single source of truth: the one-shot migration table
// (internal.legacyStateFiles) maps each legacy root name to the matching const,
// and every writer routes through Path with the same const. Referencing one
// const on both sides makes the "target basenames must not drift" contract a
// compile-time fact rather than a comment.
const (
	FileTaskCounter      = "task_counter"         // sequential task-ID counter
	FileSessionCounter   = "session_counter"      // sequential session counter
	FileContextState     = "context_state.yaml"   // AIContextGenerator section hashes
	FileEventsLog        = "events.jsonl"         // append-only dev event log
	FileGovernanceLog    = "governance.jsonl"     // append-only governance stream (#137)
	FileSchedulerLog     = "scheduler.log"        // scheduler daemon log
	FileSchedulerPID     = "scheduler.pid"        // scheduler daemon PID file
	FileSchedulerState   = "scheduler_state.yaml" // scheduler persisted state
	FileAutomationCursor = "automation_cursor"    // event-log cursor for event rules
	FileSessionChanges   = "session_changes"      // hook change tracker
	FileEvidenceReads    = "evidence_reads"       // hook evidence tracker
	FileMCPCache         = "mcp_cache.json"       // MCP health-check TTL cache
	FileMemoryDB         = "memory.sqlite"        // vector-memory SQLite store
)

// Dir returns the absolute path of the .adb/ state directory under basePath:
// <basePath>/.adb. Path(basePath, name) is always Dir(basePath)/name, so
// ensuring Dir exists is enough for any Path under it to be writable.
func Dir(basePath string) string {
	return filepath.Join(basePath, Name)
}

// Path returns the absolute path of an adb-owned state file named `name` under
// basePath's .adb/ directory: <basePath>/.adb/<name>. `name` is the bare
// basename inside .adb/ (e.g. FileSchedulerLog, FileTaskCounter), with no
// leading dot — .adb/ already namespaces the file.
func Path(basePath, name string) string {
	return filepath.Join(Dir(basePath), name)
}

// Ensure creates the .adb/ state directory under basePath (0o755) if it does
// not already exist, so a subsequent write to Path(basePath, name) finds its
// parent. It collapses the repeated os.MkdirAll(filepath.Dir(Path(...)), 0o755)
// idiom the state writers used. MkdirAll is a no-op when the directory already
// exists, so Ensure is idempotent; it fails loudly (returns the error) when the
// .adb path is occupied by a regular file rather than silently proceeding.
func Ensure(basePath string) error {
	return os.MkdirAll(Dir(basePath), 0o755)
}
