package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestDriftChecker_CleanWorkspaceHasNoDrift(t *testing.T) {
	dir := t.TempDir()
	src := &fakeCatalogSource{
		orgs:  []models.Organization{{ID: "acme"}},
		inits: []models.Initiative{{ID: "onboarding", OrgID: "acme"}},
		tasks: []models.Task{{ID: "TASK-1", Initiative: "onboarding"}},
		metrics: []models.Metric{
			{Initiative: "onboarding", Name: "sean-ellis", Value: 42},
		},
	}
	// No manifest on disk + version matches → no template drift; refs all resolve.
	report, err := NewDriftChecker(dir, src, "1").Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.HasDrift() {
		t.Errorf("expected no drift, got %+v", report.Findings)
	}
}

func TestDriftChecker_CatalogReferenceIntegrity(t *testing.T) {
	src := &fakeCatalogSource{
		orgs:  []models.Organization{{ID: "acme"}},
		inits: []models.Initiative{{ID: "onboarding", OrgID: "ghost-org"}}, // dangling org
		tasks: []models.Task{{ID: "TASK-1", Initiative: "ghost-init"}},     // dangling initiative
		metrics: []models.Metric{
			{Initiative: "ghost-init", Name: "x", Value: 1}, // dangling initiative
		},
	}
	report, err := NewDriftChecker(t.TempDir(), src, "1").Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	byKind := map[string][]models.DriftFinding{}
	for _, f := range report.Findings {
		byKind[f.Kind] = append(byKind[f.Kind], f)
	}
	if len(byKind[models.DriftDanglingOrg]) != 1 || byKind[models.DriftDanglingOrg][0].Entity != "onboarding" {
		t.Errorf("dangling-org findings = %+v", byKind[models.DriftDanglingOrg])
	}
	// The ticket and the metric both dangle to an unknown initiative.
	if len(byKind[models.DriftDanglingInitiative]) != 2 {
		t.Errorf("dangling-initiative findings = %+v", byKind[models.DriftDanglingInitiative])
	}
}

func TestDriftChecker_TemplateDrift(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "proj")
	// Scaffold at template version 1.
	if err := NewFileProjectInitializer(v1FS()).InitializeProject(dir, InitOptions{Name: "acme"}); err != nil {
		t.Fatalf("scaffold: %v", err)
	}

	// Delete a scaffolded file to force a missing-file finding.
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("remove README: %v", err)
	}

	// Current template version is now 2 → stale-template finding too.
	report, err := NewDriftChecker(dir, &fakeCatalogSource{}, "2").Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	var stale, missing bool
	for _, f := range report.Findings {
		switch f.Kind {
		case models.DriftStaleTemplate:
			stale = true
		case models.DriftMissingFile:
			if f.Entity == "README.md" {
				missing = true
			}
		}
	}
	if !stale {
		t.Errorf("expected a stale-template finding; got %+v", report.Findings)
	}
	if !missing {
		t.Errorf("expected a missing-file finding for README.md; got %+v", report.Findings)
	}
}

func TestConformanceDriftRule_FiresThroughRuleEngine(t *testing.T) {
	// A declarative scheduled D7 rule whose action runs the drift check — exactly
	// what `adb schedule add --every 24h --run-exec "adb conformance check"` writes.
	rule := models.Rule{
		Name: "conformance-drift",
		On:   models.RuleTrigger{Schedule: "24h"},
		Run:  models.RuleAction{Exec: []string{"adb", "conformance", "check"}},
	}
	if err := rule.Validate(); err != nil {
		t.Fatalf("rule invalid: %v", err)
	}

	runner := &fakeRunner{}
	engine := NewRuleEngine(&fakeRuleStore{set: models.RuleSet{Rules: []models.Rule{rule}}}, nil, runner, nil, nil)

	// It is an enabled time rule (the scheduler would turn it into a job).
	timeRules, err := engine.TimeRules()
	if err != nil {
		t.Fatalf("TimeRules: %v", err)
	}
	if len(timeRules) != 1 || timeRules[0].Name != "conformance-drift" {
		t.Fatalf("expected the drift rule as a time rule, got %+v", timeRules)
	}

	firing, err := engine.FireByName(context.Background(), "conformance-drift", nil)
	if err != nil {
		t.Fatalf("FireByName: %v", err)
	}
	if firing.Status != FiringFired {
		t.Fatalf("firing status = %q (%s), want fired", firing.Status, firing.Reason)
	}
	if len(runner.execs) != 1 {
		t.Fatalf("expected 1 exec, got %d", len(runner.execs))
	}
	args := runner.execs[0].args
	if len(args) != 3 || args[0] != "adb" || args[1] != "conformance" || args[2] != "check" {
		t.Errorf("rule did not exec the conformance check; got %v", args)
	}
}
