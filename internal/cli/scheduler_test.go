package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// seedLiveSchedulerPIDFile points the scheduler's PID file at THIS process, so
// readPIDFile's liveness probe succeeds and schedulerDaemonStop gets as far as
// signalling. The signal itself is always faked — a real SIGTERM here would kill
// the test binary.
func seedLiveSchedulerPIDFile(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("ADB_HOME", tmp)
	App = nil
	if err := os.MkdirAll(filepath.Join(tmp, ".adb"), 0o755); err != nil {
		t.Fatalf("mkdir .adb: %v", err)
	}
	if err := os.WriteFile(schedulerPIDPath(), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatalf("seed PID file: %v", err)
	}
}

// TestSchedulerStop_SurfacesASignalFailure is the bug the nilerr finding named:
// `adb scheduler stop` treated EVERY signal failure as a stop, printing
// "✓ Scheduler stopped" and exiting 0. A daemon we were not permitted to signal
// is still running, and a script that checks the exit status was told otherwise.
func TestSchedulerStop_SurfacesASignalFailure(t *testing.T) {
	seedLiveSchedulerPIDFile(t)
	restore := swapSignalStop(func(*os.Process) error { return os.ErrPermission })
	defer restore()

	err := schedulerDaemonStop()
	if err == nil {
		t.Fatal("stop must fail when the daemon could not be signalled, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("stop must name the underlying signal failure, got %v", err)
	}
}

// TestSchedulerStop_AlreadyGoneIsSuccess is the other half, and the reason the
// branch exists: the daemon can exit between the liveness probe and the signal,
// which os.Process.Signal reports as ErrProcessDone. That IS a stop — it must
// stay a success, and the stale PID file must still be cleared.
func TestSchedulerStop_AlreadyGoneIsSuccess(t *testing.T) {
	seedLiveSchedulerPIDFile(t)
	restore := swapSignalStop(func(*os.Process) error { return os.ErrProcessDone })
	defer restore()

	if err := schedulerDaemonStop(); err != nil {
		t.Fatalf("a daemon that already exited is a successful stop, got %v", err)
	}
	if _, err := os.Stat(schedulerPIDPath()); !os.IsNotExist(err) {
		t.Errorf("stop must remove the stale PID file, stat err = %v", err)
	}
}

// TestSchedulerRestart_AbortsWhenStopFails pins that restart does not start a
// second daemon over the top of one it failed to stop.
func TestSchedulerRestart_AbortsWhenStopFails(t *testing.T) {
	seedLiveSchedulerPIDFile(t)
	restore := swapSignalStop(func(*os.Process) error { return os.ErrPermission })
	defer restore()

	err := newSchedulerRestartCmd().Execute()
	if err == nil {
		t.Fatal("restart must abort when the stop half fails, got nil")
	}
	if !strings.Contains(err.Error(), "restart") {
		t.Errorf("restart must say which half failed, got %v", err)
	}
}

// TestSchedulerList_ShowsRuleJobs guards #178: `adb scheduler list` must show
// every job that has persisted state — including the daemon's `rule:<name>` and
// `automation-dispatch` jobs — not just the three hardcoded built-ins. A rule
// job with recorded failures was previously invisible.
func TestSchedulerList_ShowsRuleJobs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ADB_HOME", tmp)
	// App must be nil so schedulerBase() falls back to ADB_HOME.
	App = nil

	// Seed a state file the way the daemon persists it: a stateFile{jobs: [...]}
	// carrying a built-in plus a rule job and automation-dispatch with runs.
	stateYAML := `jobs:
  - name: repos-pull
    runs: 2
    failures: 0
  - name: "rule:fastrule"
    runs: 3
    failures: 1
    last_error: "boom"
  - name: automation-dispatch
    runs: 5
`
	// State now lives under .adb/ (#186), the path schedulerStatePath() resolves.
	if err := os.MkdirAll(filepath.Join(tmp, ".adb"), 0o755); err != nil {
		t.Fatalf("mkdir .adb: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".adb", "scheduler_state.yaml"), []byte(stateYAML), 0o644); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	cmd := newSchedulerListCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scheduler list: %v", err)
	}
	got := out.String()

	// The built-ins still appear.
	for _, want := range []string{"repos-pull", "alerts-tick", "events-rotate"} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing built-in %q:\n%s", want, got)
		}
	}
	// The rule job + automation-dispatch (persisted state) now appear too.
	if !strings.Contains(got, "rule:fastrule") {
		t.Errorf("list must show the rule:fastrule job (#178):\n%s", got)
	}
	if !strings.Contains(got, "automation-dispatch") {
		t.Errorf("list must show the automation-dispatch job (#178):\n%s", got)
	}
	// Its recorded failure/error is surfaced (the whole point — visibility).
	if !strings.Contains(got, "boom") {
		t.Errorf("list must surface the rule job's last_error:\n%s", got)
	}
}

// TestSchedulerPaths_UnderADB pins that the scheduler's pid/log/state/cursor
// paths resolve under <base>/.adb/ (#186, #189) with their dot-free basenames.
// schedulerBase() still resolves the workspace root, so status/stop keep working
// after the migration. App is nil here so base falls back to ADB_HOME.
func TestSchedulerPaths_UnderADB(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ADB_HOME", tmp)
	App = nil

	cases := map[string]string{
		schedulerPIDPath():     filepath.Join(tmp, ".adb", "scheduler.pid"),
		schedulerLogPath():     filepath.Join(tmp, ".adb", "scheduler.log"),
		schedulerStatePath():   filepath.Join(tmp, ".adb", "scheduler_state.yaml"),
		automationCursorPath(): filepath.Join(tmp, ".adb", "automation_cursor"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("scheduler path = %q, want %q", got, want)
		}
	}
}
