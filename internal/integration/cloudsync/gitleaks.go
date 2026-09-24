package cloudsync

import (
	"fmt"
	"os/exec"
)

// LeakRunner is the runner seam so ScanForSecrets is unit-testable
// without gitleaks installed. It returns (stdout+stderr, exit code, err).
// err is populated when the process could not be started at all (missing
// binary, PATH lookup failure) — that condition MUST fail-CLOSED.
type LeakRunner func(args ...string) ([]byte, int, error)

// ScanForSecrets runs the default gitleaks binary against staging and
// returns (clean, report, err). Fail-CLOSED on scanner error / missing
// binary / unexpected exit — the caller MUST abort in those cases.
func ScanForSecrets(staging string) (bool, string, error) {
	return ScanForSecretsWith(defaultLeakRunner, staging)
}

// ScanForSecretsWith is the injectable variant used by tests. Semantics:
//   - exit 0  → clean=true, err=nil
//   - exit 1  → clean=false, err=nil (finding, caller must abort)
//   - anything else (missing binary, exit 2+, non-nil err) → err returned
//     (SECURITY: never treat as clean)
func ScanForSecretsWith(run LeakRunner, staging string) (bool, string, error) {
	stdout, code, err := run(
		"detect",
		"--no-git",
		"--source", staging,
		"--redact",
		"--no-banner",
	)
	report := string(stdout)
	if err != nil {
		// Runner could not start the process — treat as security failure.
		return false, report, fmt.Errorf("gitleaks scanner failed to start (fail-closed): %w", err)
	}
	switch code {
	case 0:
		return true, report, nil
	case 1:
		return false, report, nil
	default:
		return false, report, fmt.Errorf("gitleaks unexpected exit code %d (fail-closed): %s", code, report)
	}
}

// DefaultLeakRunner shells out to `gitleaks` on PATH. Exported so the CLI
// layer can wire it into cloudsync.Config.Leak without redeclaring it.
func DefaultLeakRunner(args ...string) ([]byte, int, error) {
	return defaultLeakRunner(args...)
}

// defaultLeakRunner shells out to `gitleaks` on PATH.
// Kept separate so ScanForSecrets can be routed through it in production
// while tests inject a table-driven fake.
func defaultLeakRunner(args ...string) ([]byte, int, error) {
	cmd := exec.Command("gitleaks", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return out, exitErr.ExitCode(), nil
		}
		// Startup / lookup failure — surface the error unchanged.
		return out, -1, err
	}
	return out, 0, nil
}
