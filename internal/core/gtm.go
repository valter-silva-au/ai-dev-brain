package core

import (
	"io/fs"
)

// This file enumerates and scaffolds the go-to-market packs (#135 step 18) —
// positioning/messaging + moat-narrative — as a thin wrapper over the generic
// pack scaffolder (packs.go). Packs are the subdirectories of gtm/ in the
// embedded FS.

const gtmRoot = "gtm"

// GTMPacks lists the GTM pack ids available in fsys (subdirectories of gtm/).
func GTMPacks(fsys fs.FS) ([]string, error) {
	return listPackDirs(fsys, gtmRoot)
}

// GTMScaffoldEntry is the outcome for one scaffolded GTM doc.
//
// The json tags are PREVENTIVE: `adb gtm scaffold` has no --json today, so nothing
// marshals this type yet. They are here because this struct is field-for-field identical
// to PackScaffoldEntry, which shipped untagged and therefore emitted PascalCase
// `Name`/`Dest`/`Action` from `adb program scaffold --json` while its five sibling
// subcommands emitted snake_case. Tagging now means adding --json here is a one-line
// change that cannot reintroduce that inconsistency.
type GTMScaffoldEntry struct {
	Name   string               `json:"name"`
	Dest   string               `json:"dest"`
	Action HarnessInstallAction `json:"action"`
}

// GTMScaffoldResult summarises a ScaffoldGTMPack call.
type GTMScaffoldResult struct {
	Pack    string
	DestDir string
	DryRun  bool
	Entries []GTMScaffoldEntry
}

// ScaffoldGTMPack writes every doc for pack into destDir with the idempotent,
// clobber-safe harness-install semantics. It errors if the pack is unknown.
func ScaffoldGTMPack(fsys fs.FS, pack, destDir string, opts HarnessInstallOptions) (GTMScaffoldResult, error) {
	entries, err := scaffoldPack(fsys, gtmRoot, pack, destDir, opts)
	if err != nil {
		return GTMScaffoldResult{}, err
	}
	res := GTMScaffoldResult{Pack: pack, DestDir: destDir, DryRun: opts.DryRun}
	for _, e := range entries {
		// A plain conversion, now that this type carries the same json tags as
		// PackScaffoldEntry — identical tags are part of Go's struct-convertibility
		// rule, so the field-by-field literal this replaced only existed because the
		// tags differed.
		res.Entries = append(res.Entries, GTMScaffoldEntry(e))
	}
	return res, nil
}
