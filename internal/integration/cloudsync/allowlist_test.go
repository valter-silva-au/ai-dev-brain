package cloudsync

import "testing"

// TestShouldUpload pins the security boundary of the cloud-sync allowlist:
// deny-first, then a strict include-root allowlist. New top-level dirs are
// never uploaded by accident, and adb machinery / secrets / private ticket
// communications never leave the machine.
func TestShouldUpload(t *testing.T) {
	cases := []struct {
		name string
		rel  string // path relative to workspace root
		want bool
	}{
		// INCLUDE roots
		{"raw file", "raw/articles/x.md", true},
		{"scripts", "scripts/update-all.sh", true},
		{"skills", "skills/cohere/SKILL.md", true},
		{"wiki", "wiki/index.md", true},
		{"ticket context", "tickets/github.com/awslabs/mcp/TASK-1-x/context.md", true},
		{"ticket notes", "tickets/github.com/awslabs/mcp/TASK-1-x/notes.md", true},
		{"root config CLAUDE.md", "CLAUDE.md", true},
		{"root config Taskfile", "Taskfile.yaml", true},
		{"root config WIKI.md", "WIKI.md", true},

		// HARD DENY — never upload
		{"env", ".env", false},
		{"env variant", ".env.local", false},
		{"env staging", ".env.staging", false},
		{"omnictx", ".omnictx/corpus.enc", false},
		{"omnictx nested", ".omnictx/nested/x.jsonl", false},
		{"communications", "tickets/github.com/awslabs/mcp/TASK-1-x/communications/slack.md", false},
		{"backlog", "backlog.yaml", false},
		{"task counter", ".task_counter", false},
		{"session counter", ".session_counter", false},
		{"adb dir", ".adb/state", false},
		// Relocated state under .adb/ (#186) — denied by the blanket .adb rule.
		{"adb dir events", ".adb/events.jsonl", false},
		{"adb dir governance", ".adb/governance.jsonl", false},
		{"adb dir memory", ".adb/memory.sqlite", false},
		{"adb dir memory shm", ".adb/memory.sqlite-shm", false},
		{"adb dir memory wal", ".adb/memory.sqlite-wal", false},
		{"adb dir task counter", ".adb/task_counter", false},
		{"adb dir scheduler pid", ".adb/scheduler.pid", false},
		{"adb dir scheduler state", ".adb/scheduler_state.yaml", false},
		{"adb dir session changes", ".adb/session_changes", false},
		{"adb dir context state", ".adb/context_state.yaml", false},
		{"adb dir claude-user", ".adb/claude-user.md", false},
		// Legacy root-level names — still denied for un-migrated workspaces.
		{"legacy adb memory", ".adb_memory.sqlite", false},
		{"legacy events", ".events.jsonl", false},
		{"legacy task counter", ".task_counter", false},
		{"sessions dir", "tickets/x/sessions/2026.jsonl", false},
		{"work tree", "work/github.com/o/r/TASK-1/main.go", false},
		{"repos tree", "repos/github.com/o/r/README.md", false},

		// path-escape / relative traversal
		{"parent traversal", "../secret", false},
		{"embedded traversal to omnictx", "raw/../.omnictx/x", false},
		{"embedded traversal to env", "raw/../.env", false},
		{"deep traversal to work", "wiki/../work/steal.go", false},

		// NOT in any include root → deny by default
		{"unlisted top dir", "node_modules/x/index.js", false},
		{"dotfile at root", ".DS_Store", false},
		{"random top file", "randomfile.txt", false},
		{"empty rel", "", false},
		{"dot rel", ".", false},

		// absolute paths — deny (only workspace-relative accepted)
		{"absolute path", "/etc/passwd", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldUpload(tc.rel); got != tc.want {
				t.Errorf("ShouldUpload(%q) = %v, want %v", tc.rel, got, tc.want)
			}
		})
	}
}
