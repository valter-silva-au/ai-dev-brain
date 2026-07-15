package models

// AuditStatus is the outcome of one security/compliance control evaluation.
type AuditStatus string

const (
	// AuditPass: a deterministic control is satisfied.
	AuditPass AuditStatus = "pass"
	// AuditFail: a deterministic control is unmet — it fails the audit.
	AuditFail AuditStatus = "fail"
	// AuditWarn: a soft control is unmet — surfaced, but does not fail the audit.
	AuditWarn AuditStatus = "warn"
	// AuditManual: a control requires human attestation (adb cannot verify it) —
	// informational, points at the scaffolded compliance doc. Never fails.
	AuditManual AuditStatus = "manual"
)

// AuditFinding is one control's evaluation in a security-posture audit.
type AuditFinding struct {
	Control   string      `yaml:"control" json:"control"`     // stable control id
	Framework string      `yaml:"framework" json:"framework"` // general|soc2|gdpr|hipaa
	Title     string      `yaml:"title" json:"title"`
	Status    AuditStatus `yaml:"status" json:"status"`
	Detail    string      `yaml:"detail" json:"detail"` // what was found + remediation
}

// AuditReport is the result of `adb audit security`: per-control findings plus a
// status rollup.
type AuditReport struct {
	Findings []AuditFinding `yaml:"findings" json:"findings"`
	Summary  AuditSummary   `yaml:"summary" json:"summary"`
}

// AuditSummary counts findings by status.
type AuditSummary struct {
	Pass   int `yaml:"pass" json:"pass"`
	Fail   int `yaml:"fail" json:"fail"`
	Warn   int `yaml:"warn" json:"warn"`
	Manual int `yaml:"manual" json:"manual"`
}

// HasFailures reports whether any control failed (the gate signal for
// `adb audit security --exit-code`).
func (r AuditReport) HasFailures() bool { return r.Summary.Fail > 0 }
