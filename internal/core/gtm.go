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
type GTMScaffoldEntry struct {
	Name   string
	Dest   string
	Action HarnessInstallAction
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
		res.Entries = append(res.Entries, GTMScaffoldEntry{Name: e.Name, Dest: e.Dest, Action: e.Action})
	}
	return res, nil
}
