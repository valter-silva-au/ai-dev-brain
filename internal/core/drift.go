package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// DriftChecker flags entities that have drifted from their template or catalog
// expectations (#128). It is the deterministic logic behind the scheduled
// conformance-drift rule: a D7 rule (`on schedule run exec: adb conformance
// check`) is the first real consumer of the #119 rule engine, but the checking
// itself is plain Go adb can run directly (no Claude skill needed).
//
// Two families of drift are detected:
//   - Template drift, from the copier/cruft manifest (#128 cap 3): the workspace
//     scaffolded from an older template version, or a scaffolded file now missing.
//   - Catalog drift, from the #109 graph registries (#128 cap 2): dangling
//     references — an initiative → unknown org, a ticket/metric → unknown
//     initiative.
type DriftChecker interface {
	// Check inspects the workspace and returns every drift finding (empty when it
	// conforms). It never fails on a missing template manifest — a workspace not
	// scaffolded by adb simply has no template expectations to drift from.
	Check() (*models.DriftReport, error)
}

type driftChecker struct {
	basePath        string
	catalog         CatalogSource
	templateVersion string // the current (shipping) template-set version
}

// NewDriftChecker builds a DriftChecker for the workspace at basePath. catalog
// supplies the registries for reference-integrity checks; templateVersion is the
// version the current template set ships (compared against the manifest's).
func NewDriftChecker(basePath string, catalog CatalogSource, templateVersion string) DriftChecker {
	return &driftChecker{basePath: basePath, catalog: catalog, templateVersion: templateVersion}
}

func (d *driftChecker) Check() (*models.DriftReport, error) {
	report := &models.DriftReport{Findings: []models.DriftFinding{}}

	if err := d.checkTemplateDrift(report); err != nil {
		return nil, err
	}
	if err := d.checkCatalogDrift(report); err != nil {
		return nil, err
	}

	// Deterministic order: by kind, then entity.
	sort.SliceStable(report.Findings, func(i, j int) bool {
		a, b := report.Findings[i], report.Findings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Entity < b.Entity
	})
	return report, nil
}

// checkTemplateDrift compares the recorded template manifest against the current
// template version and verifies every scaffolded file still exists. A workspace
// with no manifest is simply skipped (it was not scaffolded by adb).
func (d *driftChecker) checkTemplateDrift(report *models.DriftReport) error {
	manifest, err := readManifest(d.basePath)
	if err != nil {
		// No manifest → nothing to check (not scaffolded by adb). Other read/parse
		// errors are real: surface them.
		if errors.Is(err, ErrNoManifest) {
			return nil
		}
		return err
	}

	if d.templateVersion != "" && manifest.TemplateVersion != "" && manifest.TemplateVersion != d.templateVersion {
		report.Findings = append(report.Findings, models.DriftFinding{
			Entity: "workspace",
			Kind:   models.DriftStaleTemplate,
			Detail: fmt.Sprintf("scaffolded from template version %s; current is %s (run `adb init update`)", manifest.TemplateVersion, d.templateVersion),
		})
	}

	for rel := range manifest.Files {
		full := filepath.Join(d.basePath, filepath.FromSlash(rel))
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			report.Findings = append(report.Findings, models.DriftFinding{
				Entity: rel,
				Kind:   models.DriftMissingFile,
				Detail: "scaffolded file is missing from the workspace",
			})
		}
	}
	return nil
}

// checkCatalogDrift verifies reference integrity across the registries: every
// initiative's org, and every ticket's / metric's initiative, must resolve to a
// registered entity.
func (d *driftChecker) checkCatalogDrift(report *models.DriftReport) error {
	if d.catalog == nil {
		return nil
	}
	orgs, err := d.catalog.Organizations()
	if err != nil {
		return fmt.Errorf("drift: load organizations: %w", err)
	}
	inits, err := d.catalog.Initiatives()
	if err != nil {
		return fmt.Errorf("drift: load initiatives: %w", err)
	}
	tickets, err := d.catalog.Tickets()
	if err != nil {
		return fmt.Errorf("drift: load tickets: %w", err)
	}
	metrics, err := d.catalog.Metrics()
	if err != nil {
		return fmt.Errorf("drift: load metrics: %w", err)
	}

	orgIDs := map[string]bool{}
	for _, o := range orgs {
		orgIDs[o.ID] = true
	}
	initIDs := map[string]bool{}
	for _, in := range inits {
		initIDs[in.ID] = true
	}

	for _, in := range inits {
		if in.OrgID != "" && !orgIDs[in.OrgID] {
			report.Findings = append(report.Findings, models.DriftFinding{
				Entity: in.ID,
				Kind:   models.DriftDanglingOrg,
				Detail: fmt.Sprintf("initiative references unknown org %q", in.OrgID),
			})
		}
	}
	for _, t := range tickets {
		if t.Initiative != "" && !initIDs[t.Initiative] {
			report.Findings = append(report.Findings, models.DriftFinding{
				Entity: t.ID,
				Kind:   models.DriftDanglingInitiative,
				Detail: fmt.Sprintf("ticket references unknown initiative %q", t.Initiative),
			})
		}
	}
	for _, m := range metrics {
		if m.Initiative != "" && !initIDs[m.Initiative] {
			report.Findings = append(report.Findings, models.DriftFinding{
				Entity: m.GraphID(),
				Kind:   models.DriftDanglingInitiative,
				Detail: fmt.Sprintf("metric references unknown initiative %q", m.Initiative),
			})
		}
	}
	return nil
}
