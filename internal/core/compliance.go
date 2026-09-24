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
//
// The json tags are PREVENTIVE: `adb compliance scaffold` has no --json today, so
// nothing marshals this type yet. They are here because this struct is field-for-field
// identical to PackScaffoldEntry, which shipped untagged and therefore emitted PascalCase
// `Name`/`Dest`/`Action` from `adb program scaffold --json` while its five sibling
// subcommands emitted snake_case. Tagging now means adding --json here is a one-line
// change that cannot reintroduce that inconsistency.
type ComplianceScaffoldEntry struct {
	Name   string               `json:"name"`
	Dest   string               `json:"dest"`
	Action HarnessInstallAction `json:"action"` // installed | unchanged | skipped (shared write semantics)
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
		// A plain conversion, now that this type carries the same json tags as
		// PackScaffoldEntry — identical tags are part of Go's struct-convertibility
		// rule, so the field-by-field literal this replaced only existed because the
		// tags differed.
		res.Entries = append(res.Entries, ComplianceScaffoldEntry(e))
	}
	return res, nil
}
