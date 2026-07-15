package hooks

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/statedir"
)

// EvidenceTracker records evidence-file reads during a session. It is the
// append-only companion to ChangeTracker, used by the evidence-read gate
// in HookEngine.ProcessPreToolUse. Each line is
// `timestamp|normalised-path` separated by '|'.
//
// The file lives under the workspace's .adb/ state dir (#186), at
// .adb/evidence_reads. It is consumed by the gate on subsequent Write/Edit tool
// calls and cleared at session-end by HookEngine.ProcessSessionEnd.
type EvidenceTracker struct {
	basePath    string // workspace root; statedir.Ensure targets its .adb/
	sessionFile string
}

// NewEvidenceTracker creates a tracker rooted at basePath.
func NewEvidenceTracker(basePath string) *EvidenceTracker {
	return &EvidenceTracker{
		basePath:    basePath,
		sessionFile: statedir.Path(basePath, statedir.FileEvidenceReads),
	}
}

// Record appends an evidence-read entry for path.
func (et *EvidenceTracker) Record(path string) error {
	// Ensure the .adb/ parent exists (#186) — the file now lives under it.
	if err := statedir.Ensure(et.basePath); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	f, err := os.OpenFile(et.sessionFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open evidence file: %w", err)
	}
	defer f.Close()

	line := fmt.Sprintf("%s|%s\n", time.Now().UTC().Format(time.RFC3339Nano), path)
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to write evidence record: %w", err)
	}
	return nil
}

// Reads returns all evidence-path entries tracked so far, in insertion
// order. If the tracker file does not exist the result is empty with no
// error.
func (et *EvidenceTracker) Reads() ([]string, error) {
	f, err := os.Open(et.sessionFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open evidence file: %w", err)
	}
	defer f.Close()

	var paths []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "|", 2)
		if len(parts) == 2 {
			paths = append(paths, parts[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read evidence file: %w", err)
	}
	return paths, nil
}

// Clear removes the tracker file. Safe to call when the file does not
// exist.
func (et *EvidenceTracker) Clear() error {
	if err := os.Remove(et.sessionFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear evidence file: %w", err)
	}
	return nil
}
