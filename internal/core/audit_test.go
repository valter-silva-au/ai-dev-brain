package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// findingsByControl indexes a report's findings by control id for assertions.
func findingsByControl(r *models.AuditReport) map[string]models.AuditFinding {
	m := map[string]models.AuditFinding{}
	for _, f := range r.Findings {
		m[f.Control] = f
	}
	return m
}

func TestSecurityAuditor_EmptyWorkspaceFails(t *testing.T) {
	dir := t.TempDir()
	report, err := NewSecurityAuditor(dir).Audit("")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	by := findingsByControl(report)

	// Missing secret-scanner config + un-ignored .env are hard fails.
	if by["secret-scanning"].Status != models.AuditFail {
		t.Errorf("secret-scanning = %s, want fail", by["secret-scanning"].Status)
	}
	if by["env-gitignored"].Status != models.AuditFail {
		t.Errorf("env-gitignored = %s, want fail", by["env-gitignored"].Status)
	}
	// No .env present is a pass; missing pre-commit + no SLOs are soft warns.
	if by["no-plaintext-env"].Status != models.AuditPass {
		t.Errorf("no-plaintext-env = %s, want pass", by["no-plaintext-env"].Status)
	}
	if by["slo-defined"].Status != models.AuditWarn {
		t.Errorf("slo-defined = %s, want warn", by["slo-defined"].Status)
	}
	// Manual controls are informational.
	if by["access-review"].Status != models.AuditManual {
		t.Errorf("access-review = %s, want manual", by["access-review"].Status)
	}
	if !report.HasFailures() {
		t.Error("empty workspace should have failures")
	}
}

func TestSecurityAuditor_ConfiguredWorkspacePasses(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitleaks.toml", "title = \"gitleaks\"\n")
	write(".gitignore", "work/\n.env\n.env.*\n")
	write(".pre-commit-config.yaml", "repos: []\n")
	write("slo/index.yaml", "slos:\n  - name: api\n    objective: 99.9\n")

	report, err := NewSecurityAuditor(dir).Audit("")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	by := findingsByControl(report)
	for _, id := range []string{"secret-scanning", "env-gitignored", "no-plaintext-env", "precommit-config", "slo-defined"} {
		if by[id].Status != models.AuditPass {
			t.Errorf("%s = %s (%s), want pass", id, by[id].Status, by[id].Detail)
		}
	}
	if report.HasFailures() {
		t.Errorf("configured workspace should not fail: %+v", report.Summary)
	}
}

func TestSecurityAuditor_PlaintextEnvWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, _ := NewSecurityAuditor(dir).Audit("")
	if findingsByControl(report)["no-plaintext-env"].Status != models.AuditWarn {
		t.Error("a plaintext .env at root should warn")
	}
}

func TestSecurityAuditor_EnvGitignore_NoPrefixFalsePositive(t *testing.T) {
	cases := []struct {
		name       string
		gitignore  string
		wantStatus models.AuditStatus
	}{
		// These match git's actual `git check-ignore .env` behaviour (#160): only
		// a bare `.env`, an anchored `/.env`, or a `.env*` glob ignore the FILE
		// `.env`. `.env.*` / `.env.local` / `.env/` / `.envrc` do NOT — so the
		// control must FAIL for them, or it reports a false all-clear on an
		// exposed secrets file.
		{"exact .env", ".env\n", models.AuditPass},
		{"glob .env*", ".env*\n", models.AuditPass},
		{"anchored /.env", "/.env\n", models.AuditPass},
		{"glob .env.* does NOT cover .env", ".env.*\n", models.AuditFail},
		{"specific .env.local does NOT cover .env", ".env.local\n", models.AuditFail},
		{"dir pattern .env/ does NOT cover the file .env", ".env/\n", models.AuditFail},
		{"only .envrc must NOT satisfy .env", ".envrc\n", models.AuditFail},
		{"no .gitignore", "", models.AuditFail},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.gitignore != "" {
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.gitignore), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			report, _ := NewSecurityAuditor(dir).Audit("")
			if got := findingsByControl(report)["env-gitignored"].Status; got != tc.wantStatus {
				t.Errorf("env-gitignored with %q = %s, want %s", tc.gitignore, got, tc.wantStatus)
			}
		})
	}
}

func TestSecurityAuditor_FrameworkFilter(t *testing.T) {
	report, err := NewSecurityAuditor(t.TempDir()).Audit("soc2")
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	by := findingsByControl(report)
	// General controls always run.
	if _, ok := by["secret-scanning"]; !ok {
		t.Error("general control secret-scanning should run under a framework filter")
	}
	// soc2 manual controls run; gdpr/hipaa ones do not.
	if _, ok := by["access-review"]; !ok {
		t.Error("soc2 control access-review should run under --framework soc2")
	}
	if _, ok := by["data-retention"]; ok {
		t.Error("gdpr control data-retention should NOT run under --framework soc2")
	}
	if _, ok := by["risk-analysis"]; ok {
		t.Error("hipaa control risk-analysis should NOT run under --framework soc2")
	}
}
