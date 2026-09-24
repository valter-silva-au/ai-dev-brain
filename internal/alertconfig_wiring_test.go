package internal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// Alert thresholds, end to end through the App seam.
//
// internal/observability owns the parse (alertconfig_test.go covers every value
// shape); these tests cover the seam ResolveAlertConfig is wired at — that the
// evaluator `adb alerts` reads actually carries the workspace's configured
// thresholds. Before this, internal/app.go passed nil and no config file could
// change a threshold.
//
// Every test uses NewAppIsolated: a t.TempDir() basePath pins only the REPO tier,
// while the global tier would otherwise resolve to the developer's real
// $HOME/.taskconfig — which could set an alert_* key and make these assertions
// depend on the machine. NewAppIsolated repoints the global tier at
// <basePath>/.taskconfig, which is exactly what the precedence test below needs.

// writeConfigFile writes one config tier file under dir.
func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestApp_AlertThresholds_UnconfiguredEqualsDefaults is the defaults-do-not-move
// guard at the seam: an App built over a workspace with no alert config must
// evaluate against exactly DefaultAlertConfig().
func TestApp_AlertThresholds_UnconfiguredEqualsDefaults(t *testing.T) {
	t.Parallel()

	app, err := NewAppIsolated(t.TempDir())
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	got := app.AlertEvaluator.Config()
	if got == nil {
		t.Fatal("AlertEvaluator has no config")
	}
	for _, want := range observability.DefaultAlertConfig().Thresholds {
		th := got.GetThreshold(want.Type)
		if th == nil || *th != want {
			t.Errorf("%s = %+v, want the default %+v", want.Type, th, want)
		}
	}
	if len(got.Thresholds) != len(observability.DefaultAlertConfig().Thresholds) {
		t.Errorf("resolved %d thresholds, want %d",
			len(got.Thresholds), len(observability.DefaultAlertConfig().Thresholds))
	}
}

// TestApp_AlertThresholds_FromRepoConfig proves each of the four keys reaches the
// evaluator from a repo .taskrc — the shortest path a user actually takes.
func TestApp_AlertThresholds_FromRepoConfig(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	writeConfigFile(t, filepath.Join(tmp, ".taskrc"), strings.Join([]string{
		"custom_settings:",
		"  alert_task_blocked_too_long: 4h",
		"  alert_task_stale: 10d",
		"  alert_review_too_long: 36h",
		"  alert_backlog_too_large: \"42\"",
		"",
	}, "\n"))

	app, err := NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	config := app.AlertEvaluator.Config()
	checks := []struct {
		alertType    observability.AlertType
		wantDuration time.Duration
		wantCount    int
	}{
		{alertType: observability.AlertTaskBlockedTooLong, wantDuration: 4 * time.Hour},
		{alertType: observability.AlertTaskStale, wantDuration: 10 * 24 * time.Hour},
		{alertType: observability.AlertReviewTooLong, wantDuration: 36 * time.Hour},
		{alertType: observability.AlertBacklogTooLarge, wantCount: 42},
	}
	for _, c := range checks {
		th := config.GetThreshold(c.alertType)
		if th == nil {
			t.Errorf("%s threshold missing", c.alertType)
			continue
		}
		if th.Duration != c.wantDuration {
			t.Errorf("%s duration = %v, want %v", c.alertType, th.Duration, c.wantDuration)
		}
		if th.Count != c.wantCount {
			t.Errorf("%s count = %d, want %d", c.alertType, th.Count, c.wantCount)
		}
	}
}

// TestApp_AlertThresholds_UnquotedYAMLIntegerWorks — custom_settings is a
// map[string]string, so an unquoted `alert_backlog_too_large: 42` is a YAML *int*
// on the way in. It must still land: core's config layer stringifies a scalar
// leaf, so both spellings work and the docs can say so.
func TestApp_AlertThresholds_UnquotedYAMLIntegerWorks(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	writeConfigFile(t, filepath.Join(tmp, ".taskrc"),
		"custom_settings:\n  alert_backlog_too_large: 42\n")

	app, err := NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	if th := app.AlertEvaluator.Config().GetThreshold(observability.AlertBacklogTooLarge); th == nil || th.Count != 42 {
		t.Fatalf("backlog_too_large = %+v, want count 42 from an unquoted YAML integer", th)
	}
}

// TestApp_AlertThresholds_RepoBeatsGlobal pins the precedence the layered config
// promises — and is the reason four flat keys were chosen over one packed value:
// the repo tier overrides ONE threshold while the other keeps resolving from the
// global tier.
func TestApp_AlertThresholds_RepoBeatsGlobal(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	// NewAppIsolated points the GLOBAL tier at <basePath>/.taskconfig.
	writeConfigFile(t, filepath.Join(tmp, ".taskconfig"),
		"custom_settings:\n  alert_task_stale: 20d\n  alert_review_too_long: 9h\n")
	writeConfigFile(t, filepath.Join(tmp, ".taskrc"),
		"custom_settings:\n  alert_task_stale: 2h\n")

	app, err := NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	config := app.AlertEvaluator.Config()
	if th := config.GetThreshold(observability.AlertTaskStale); th == nil || th.Duration != 2*time.Hour {
		t.Errorf("task_stale = %+v, want the repo tier's 2h", th)
	}
	if th := config.GetThreshold(observability.AlertReviewTooLong); th == nil || th.Duration != 9*time.Hour {
		t.Errorf("review_too_long = %+v, want the global tier's 9h", th)
	}
	// Untouched by either tier.
	if th := config.GetThreshold(observability.AlertBacklogTooLarge); th == nil || th.Count != 10 {
		t.Errorf("backlog_too_large = %+v, want the default count 10", th)
	}
}

// TestApp_AlertThresholds_MalformedWarnsOnStderr is the non-fatal contract at the
// seam. A bad value must (a) not fail App construction, (b) keep the default, and
// (c) warn on STDERR with STDOUT untouched — a config diagnostic on stdout would
// corrupt every `--json` pipeline in the workspace.
func TestApp_AlertThresholds_MalformedWarnsOnStderr(t *testing.T) {
	// No t.Parallel: this test swaps the process-wide os.Stderr/os.Stdout.
	tmp := t.TempDir()
	writeConfigFile(t, filepath.Join(tmp, ".taskrc"),
		"custom_settings:\n  alert_task_stale: 3 weeks\n")

	stderr, stdout, app, err := captureAppStreams(t, tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated must not fail on an unparseable alert threshold: %v", err)
	}
	defer app.Cleanup()

	if stdout != "" {
		t.Errorf("App construction wrote to stdout: %q", stdout)
	}
	for _, want := range []string{"Warning:", "alert_task_stale", "repo", "3 weeks"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q does not mention %q", stderr, want)
		}
	}
	if th := app.AlertEvaluator.Config().GetThreshold(observability.AlertTaskStale); th == nil || th.Duration != 3*24*time.Hour {
		t.Errorf("task_stale = %+v, want the default 72h retained", th)
	}
}

// captureAppStreams builds an isolated App with os.Stderr/os.Stdout redirected to
// pipes, returning what each received. Both are restored before it returns.
func captureAppStreams(t *testing.T, basePath string) (stderr, stdout string, app *App, err error) {
	t.Helper()

	errR, errW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	outR, outW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	origErr, origOut := os.Stderr, os.Stdout
	os.Stderr, os.Stdout = errW, outW

	app, err = NewAppIsolated(basePath)

	os.Stderr, os.Stdout = origErr, origOut
	if closeErr := errW.Close(); closeErr != nil {
		t.Fatalf("close stderr pipe: %v", closeErr)
	}
	if closeErr := outW.Close(); closeErr != nil {
		t.Fatalf("close stdout pipe: %v", closeErr)
	}

	stderr = drain(t, errR)
	stdout = drain(t, outR)
	return stderr, stdout, app, err
}

func drain(t *testing.T, r *os.File) string {
	t.Helper()
	var buf bytes.Buffer
	if _, copyErr := io.Copy(&buf, r); copyErr != nil {
		t.Fatalf("read pipe: %v", copyErr)
	}
	if closeErr := r.Close(); closeErr != nil {
		t.Fatalf("close pipe: %v", closeErr)
	}
	return buf.String()
}
