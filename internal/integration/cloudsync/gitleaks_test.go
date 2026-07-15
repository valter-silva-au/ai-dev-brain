package cloudsync

import (
	"errors"
	"strings"
	"testing"
)

// TestScanForSecrets_Clean: exit 0 → clean=true, no error. This is the
// happy path — no secrets found in the staged upload set.
func TestScanForSecrets_Clean(t *testing.T) {
	runner := func(args ...string) ([]byte, int, error) {
		return []byte("no leaks found\n"), 0, nil
	}
	clean, report, err := ScanForSecretsWith(runner, "/tmp/staging")
	if err != nil {
		t.Fatalf("ScanForSecretsWith: %v", err)
	}
	if !clean {
		t.Errorf("clean = %v, want true", clean)
	}
	if report != "no leaks found\n" {
		t.Errorf("report = %q", report)
	}
}

// TestScanForSecrets_Leak: gitleaks exit 1 → clean=false, no err (finding
// is not a Go error — the caller MUST inspect clean and abort).
func TestScanForSecrets_Leak(t *testing.T) {
	runner := func(args ...string) ([]byte, int, error) {
		return []byte(`{"finding":"AKIA…"}`), 1, nil
	}
	clean, report, err := ScanForSecretsWith(runner, "/tmp/staging")
	if err != nil {
		t.Fatalf("scanner err: %v", err)
	}
	if clean {
		t.Errorf("clean = %v, want false on non-zero exit", clean)
	}
	if !strings.Contains(report, "AKIA") {
		t.Errorf("report should carry finding: %q", report)
	}
}

// TestScanForSecrets_MissingBinary: runner returns an error (e.g.
// gitleaks not on PATH) → the function MUST return err. Fail-CLOSED:
// the orchestrator must abort, not silently skip the scan.
func TestScanForSecrets_MissingBinary(t *testing.T) {
	runner := func(args ...string) ([]byte, int, error) {
		return nil, -1, errors.New("exec: gitleaks: not found in $PATH")
	}
	clean, _, err := ScanForSecretsWith(runner, "/tmp/staging")
	if err == nil {
		t.Fatalf("want error on missing binary, got nil (clean=%v) — fail-open is a security bug", clean)
	}
	if clean {
		t.Errorf("clean = true on scanner error — fail-open is a security bug")
	}
}

// TestScanForSecrets_UnexpectedExitCode: any non-{0,1} exit is treated
// as a scanner malfunction (fail-CLOSED). gitleaks exits 2+ for its
// own errors; do not mistake that for "clean".
func TestScanForSecrets_UnexpectedExitCode(t *testing.T) {
	runner := func(args ...string) ([]byte, int, error) {
		return []byte("gitleaks internal error\n"), 2, nil
	}
	clean, _, err := ScanForSecretsWith(runner, "/tmp/staging")
	if err == nil {
		t.Fatalf("want error on unexpected exit code, got nil (clean=%v)", clean)
	}
	if clean {
		t.Errorf("clean = true on unexpected exit — fail-open is a security bug")
	}
}
