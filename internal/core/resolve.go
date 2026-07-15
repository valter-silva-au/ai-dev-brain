package core

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// ResolveTicketDir finds the on-disk ticket directory for a task ID at any
// nesting depth under ticketsDir. It is the read-time, id-only dual of
// bootstrap.resolveTaskDir (which builds the path at creation time from a
// loaded task model): callers that only have a TASK-id — and no backlog entry
// with a TicketPath — use this to locate the ticket dir regardless of whether
// it lives at the nested correlation path
// (tickets/<platform>/<org>/<repo>/TASK-id-slug), under tickets/_local/, in a
// legacy flat tickets/TASK-id, or archived under tickets/_archived/….
//
// Matching rule: a directory matches when its base name is exactly taskID or
// begins with taskID+"-". The trailing dash is required so a short id can't
// fuzzy-match a longer one (TASK-1 must not match TASK-10-foo).
//
// Precedence: when the same id resolves both live and under _archived/, the
// live (non-archived) copy wins — archived is only returned when it's the sole
// match. Among same-precedence matches the shallowest, lexically-first path is
// chosen so resolution is deterministic. Returns an error when nothing matches.
func ResolveTicketDir(ticketsDir, taskID string) (string, error) {
	prefix := taskID + "-"

	var active []string
	var archived []string

	walkErr := filepath.WalkDir(ticketsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable subtrees rather than aborting the whole walk;
			// a permission blip on one dir shouldn't blind the resolver to
			// the rest of the tree.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		base := d.Name()
		if base != taskID && !strings.HasPrefix(base, prefix) {
			return nil
		}
		// Matched. Don't descend into a matched ticket dir — its own subdirs
		// (sessions/, knowledge/) can't contain another ticket.
		if isArchivedPath(ticketsDir, path) {
			archived = append(archived, path)
		} else {
			active = append(active, path)
		}
		return fs.SkipDir
	})
	if walkErr != nil {
		return "", fmt.Errorf("walk tickets dir %q: %w", ticketsDir, walkErr)
	}

	if pick := shallowestFirst(active); pick != "" {
		return pick, nil
	}
	if pick := shallowestFirst(archived); pick != "" {
		return pick, nil
	}
	return "", fmt.Errorf("no ticket directory found for %s under %s", taskID, ticketsDir)
}

// isArchivedPath reports whether path lives under the ticketsDir/_archived
// subtree. It compares path components (not a substring) so an org or repo
// literally named "_archived" elsewhere wouldn't be misclassified — though the
// canonical layout only ever puts _archived directly under ticketsDir.
func isArchivedPath(ticketsDir, path string) bool {
	rel, err := filepath.Rel(ticketsDir, path)
	if err != nil {
		return false
	}
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		if seg == "_archived" {
			return true
		}
	}
	return false
}

// shallowestFirst returns the match with the fewest path separators, breaking
// ties lexically, so ResolveTicketDir is deterministic when a tree somehow
// holds more than one match at the same archived/active precedence. Returns ""
// for an empty slice.
func shallowestFirst(matches []string) string {
	best := ""
	bestDepth := -1
	for _, m := range matches {
		depth := strings.Count(m, string(filepath.Separator))
		if best == "" || depth < bestDepth || (depth == bestDepth && m < best) {
			best = m
			bestDepth = depth
		}
	}
	return best
}
