package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// SecurityAuditor evaluates the workspace's security/compliance posture against
// a fixed catalog of controls and reports findings (#131 step 17). It mirrors
// the DriftChecker/DriftReport shape. Deterministic controls verify real
// workspace facts (secret-scanning config, .env hygiene, pre-commit, SLOs);
// framework controls that need human attestation report `manual` and point at
// the scaffolded compliance doc — they never fail the audit.
type SecurityAuditor interface {
	// Audit runs the controls and returns the report. framework filters to
	// "general" + that framework's controls; "" runs every control.
	Audit(framework string) (*models.AuditReport, error)
}

type securityAuditor struct {
	basePath string
}

// NewSecurityAuditor builds an auditor for the workspace at basePath.
func NewSecurityAuditor(basePath string) SecurityAuditor {
	return &securityAuditor{basePath: basePath}
}

// auditControl is one control in the catalog. check is nil for manual controls
// (they always report AuditManual).
type auditControl struct {
	id         string
	framework  string // "general" applies to every framework filter
	title      string
	check      func(basePath string) (models.AuditStatus, string)
	manualHint string // remediation pointer for manual controls
}

// controlCatalog is the fixed, ordered control set. Deterministic controls read
// the workspace; manual controls are attestations tracked in the scaffolded docs.
func controlCatalog() []auditControl {
	return []auditControl{
		{
			id: "secret-scanning", framework: "general", title: "Secret scanner configured",
			check: func(base string) (models.AuditStatus, string) {
				if fileExists(filepath.Join(base, ".gitleaks.toml")) {
					return models.AuditPass, ".gitleaks.toml present"
				}
				return models.AuditFail, "no .gitleaks.toml — configure a secret scanner (adb sync cloud's push gate uses gitleaks)"
			},
		},
		{
			id: "env-gitignored", framework: "general", title: "Secrets file is git-ignored",
			check: func(base string) (models.AuditStatus, string) {
				if gitignoreCovers(base, ".env") {
					return models.AuditPass, ".gitignore ignores .env"
				}
				return models.AuditFail, "add .env to .gitignore so local secrets are never committed"
			},
		},
		{
			id: "no-plaintext-env", framework: "general", title: "No plaintext .env at workspace root",
			check: func(base string) (models.AuditStatus, string) {
				if fileExists(filepath.Join(base, ".env")) {
					return models.AuditWarn, "a plaintext .env at the workspace root risks secret exposure — prefer a secrets manager"
				}
				return models.AuditPass, "no .env at the workspace root"
			},
		},
		{
			id: "precommit-config", framework: "general", title: "Pre-commit hooks configured",
			check: func(base string) (models.AuditStatus, string) {
				if fileExists(filepath.Join(base, ".pre-commit-config.yaml")) {
					return models.AuditPass, ".pre-commit-config.yaml present"
				}
				return models.AuditWarn, "no .pre-commit-config.yaml — consider pre-commit hooks for lint/secret checks"
			},
		},
		{
			id: "slo-defined", framework: "general", title: "Reliability objectives (SLOs) defined",
			check: func(base string) (models.AuditStatus, string) {
				if fileHasContent(filepath.Join(base, "slo", "index.yaml")) {
					return models.AuditPass, "SLOs recorded (slo/index.yaml)"
				}
				return models.AuditWarn, "no SLOs defined — set targets with `adb slo set`"
			},
		},
		// Manual attestation controls (tracked in the scaffolded compliance docs).
		{id: "access-review", framework: "soc2", title: "Periodic logical-access review", manualHint: "attest in compliance/soc2/ (adb compliance scaffold soc2)"},
		{id: "incident-response", framework: "soc2", title: "Documented, tested incident response", manualHint: "attest in compliance/soc2/"},
		{id: "data-subject-rights", framework: "gdpr", title: "Data-subject rights supported", manualHint: "attest in compliance/gdpr/ (adb compliance scaffold gdpr)"},
		{id: "data-retention", framework: "gdpr", title: "Data-retention policy enforced", manualHint: "attest in compliance/gdpr/"},
		{id: "risk-analysis", framework: "hipaa", title: "ePHI risk analysis", manualHint: "attest in compliance/hipaa/ (adb compliance scaffold hipaa)"},
		{id: "baa", framework: "hipaa", title: "Business Associate Agreements in place", manualHint: "attest in compliance/hipaa/"},
	}
}

func (a *securityAuditor) Audit(framework string) (*models.AuditReport, error) {
	framework = strings.ToLower(strings.TrimSpace(framework))
	report := &models.AuditReport{Findings: []models.AuditFinding{}}

	for _, c := range controlCatalog() {
		// Filter: "" runs all; otherwise general + the requested framework.
		if framework != "" && c.framework != "general" && c.framework != framework {
			continue
		}
		var status models.AuditStatus
		var detail string
		if c.check != nil {
			status, detail = c.check(a.basePath)
		} else {
			status, detail = models.AuditManual, c.manualHint
		}
		report.Findings = append(report.Findings, models.AuditFinding{
			Control:   c.id,
			Framework: c.framework,
			Title:     c.title,
			Status:    status,
			Detail:    detail,
		})
	}

	for _, f := range report.Findings {
		switch f.Status {
		case models.AuditPass:
			report.Summary.Pass++
		case models.AuditFail:
			report.Summary.Fail++
		case models.AuditWarn:
			report.Summary.Warn++
		case models.AuditManual:
			report.Summary.Manual++
		}
	}
	return report, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// fileHasContent reports whether path exists and is a non-empty regular file.
func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// gitignoreCovers reports whether base/.gitignore has an entry that actually
// ignores the FILE named name (e.g. ".env"), matching git's real semantics —
// verified with `git check-ignore`:
//
//	.env      -> ignored      /.env   -> ignored      .env*   -> ignored
//	.env.*    -> NOT ignored (needs a char after the dot)
//	.env.local / .env.production / .env/  -> NOT ignored (different path / a dir)
//	.envrc    -> NOT ignored (different file)
//
// So only a bare `name`, an anchored `/name`, or a `name*` glob (the `*` sits
// immediately after the name and matches the empty suffix) cover the file. A
// pattern like `.env.*` was previously (wrongly) treated as covering `.env`,
// which let `adb audit security` report pass on an exposed .env — the false
// all-clear this fixes (#160). Comments, whitespace, and trailing-slash dir
// patterns are handled; this is a focused matcher for the single bare-file
// case, not a full gitignore engine.
func gitignoreCovers(base, name string) bool {
	data, err := os.ReadFile(filepath.Join(base, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// A trailing `/` makes the pattern match a DIRECTORY named name, not the
		// file — `.env/` does not ignore the file `.env`.
		if strings.HasSuffix(line, "/") {
			continue
		}
		line = strings.TrimPrefix(line, "/") // an anchored pattern like /.env
		// Exact file match, or a `name*` glob (`*` matches the empty suffix).
		// Any other trailing character (`.env.` needs more, `.envrc` is a
		// different file) does NOT ignore the bare file — git treats them as
		// distinct paths.
		if line == name || line == name+"*" {
			return true
		}
	}
	return false
}
