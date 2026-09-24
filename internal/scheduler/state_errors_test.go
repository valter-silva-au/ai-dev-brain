package scheduler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStateStore_UpdateReportsWriteFailure covers the storage-side bug class:
// stateStore.update discarded writeLocked's error, so per-job state that failed
// to reach disk was silently lost. `adb scheduler list` then showed stale or
// empty state, and RunOnStart re-ran every job after a restart — with no
// indication anywhere that a write had failed.
func TestStateStore_UpdateReportsWriteFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory where the state file should be: os.WriteFile on <path>.tmp
	// succeeds, but the rename onto a non-empty directory fails.
	statePath := filepath.Join(dir, "state.yaml")
	if err := os.MkdirAll(filepath.Join(statePath, "occupied"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s := newStateStore(statePath)
	if err := s.update("alpha", func(st *State) { st.Runs = 1 }); err == nil {
		t.Fatal("update must report a persistence failure; state was silently lost")
	}
}

// TestStateStore_UpdateSucceedsNormally is the counterpart: the happy path still
// returns nil and still persists, so the new return value has not turned a
// working write into a reported failure.
func TestStateStore_UpdateSucceedsNormally(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yaml")
	s := newStateStore(statePath)
	if err := s.update("alpha", func(st *State) { st.Runs = 7 }); err != nil {
		t.Fatalf("update on a writable path: %v", err)
	}
	got, err := LoadStates(statePath)
	if err != nil {
		t.Fatalf("LoadStates: %v", err)
	}
	if len(got) != 1 || got[0].Runs != 7 {
		t.Fatalf("state not persisted: %+v", got)
	}
}

// TestRun_LogsUnreadableStateFile: load() already returns nil for a MISSING
// file, so the only errors reaching Run are a read failure or a parse failure —
// exactly the ones an operator needs, and exactly the ones the old
// `_ = states.load() // ignore errors on first start` hid. A corrupt state file
// makes every job look never-run and fire at once; the log line is what explains
// that.
func TestRun_LogsUnreadableStateFile(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.WriteFile(statePath, []byte("\tthis: is not\n  valid: yaml\n\t\t- ["), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	var logBuf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_ = Run(ctx, RunOptions{
		Jobs: []Job{{
			Name:            "noop",
			DefaultInterval: time.Hour,
			Run:             func(context.Context) error { return nil },
		}},
		StateFile:  statePath,
		Logger:     &logBuf,
		RunOnStart: false,
	})

	logged := logBuf.String()
	if !strings.Contains(logged, "scheduler state") {
		t.Errorf("Run must log that it could not load the state file; log was:\n%s", logged)
	}
	if !strings.Contains(logged, statePath) {
		t.Errorf("the log line should name the state file; log was:\n%s", logged)
	}
}

// TestRun_MissingStateFileIsSilent pins the other half: a first start has no
// state file, and that is normal — it must not produce a scary log line.
func TestRun_MissingStateFileIsSilent(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "absent.yaml")

	var logBuf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_ = Run(ctx, RunOptions{
		Jobs: []Job{{
			Name:            "noop",
			DefaultInterval: time.Hour,
			Run:             func(context.Context) error { return nil },
		}},
		StateFile:  statePath,
		Logger:     &logBuf,
		RunOnStart: false,
	})

	if strings.Contains(logBuf.String(), "scheduler state") {
		t.Errorf("a missing state file is the normal first-start case and must not be logged as a problem:\n%s", logBuf.String())
	}
}
