package core

import (
	"io/fs"
)

// This file enumerates and scaffolds the SOC2/GDPR/HIPAA compliance control
// packs (#131 step 17). It is a thin wrapper over the generic pack scaffolder
// (packs.go): the set of frameworks and their docs is whatever exists under
// compliance/ in the embedded FS, so adding a framework or a control doc is a
// matter of dropping files in — no Go change.

const complianceRoot = "compliance"

// ComplianceFrameworks lists the framework ids available in fsys (the immediate
// subdirectories of compliance/), sorted.
func ComplianceFrameworks(fsys fs.FS) ([]string, error) {
	return listPackDirs(fsys, complianceRoot)
}

// ComplianceScaffoldEntry is the outcome for one scaffolded control doc.
type ComplianceScaffoldEntry struct {
	Name   string
	Dest   string
	Action HarnessInstallAction // installed | unchanged | skipped (shared write semantics)
}

// ComplianceScaffoldResult summarises a ScaffoldComplianceFramework call.
type ComplianceScaffoldResult struct {
	Framework string
	DestDir   string
	DryRun    bool
	Entries   []ComplianceScaffoldEntry
}

// ScaffoldComplianceFramework writes every control doc for framework into destDir
// with the same idempotent, clobber-safe semantics as the validation pack: a
// matching file is "unchanged", a differing file is "skipped" unless opts.Force,
// and DryRun plans without writing. It errors if the framework is unknown.
func ScaffoldComplianceFramework(fsys fs.FS, framework, destDir string, opts HarnessInstallOptions) (ComplianceScaffoldResult, error) {
	packEntries, err := scaffoldPack(fsys, complianceRoot, framework, destDir, opts)
	if err != nil {
		return ComplianceScaffoldResult{}, err
	}
	res := ComplianceScaffoldResult{Framework: framework, DestDir: destDir, DryRun: opts.DryRun}
	for _, e := range packEntries {
		res.Entries = append(res.Entries, ComplianceScaffoldEntry{Name: e.Name, Dest: e.Dest, Action: e.Action})
	}
	return res, nil
}
