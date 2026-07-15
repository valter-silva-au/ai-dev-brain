package core

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
)

// This file holds the generic "template pack" scaffolder shared by the
// compliance packs (#133) and the GTM packs (#135): a pack is a subdirectory of
// an embedded root (compliance/<framework>/, gtm/<pack>/) whose files scaffold
// into the workspace idempotently. The set of packs is whatever exists under the
// root — pluggable by dropping files in, no Go change.

// PackScaffoldEntry is the outcome for one scaffolded pack file.
type PackScaffoldEntry struct {
	Name   string
	Dest   string
	Action HarnessInstallAction // installed | unchanged | skipped
}

// listPackDirs returns the sorted subdirectory names under root in fsys — the
// available packs that can be scaffolded.
func listPackDirs(fsys fs.FS, root string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("read %s root: %w", root, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// scaffoldPack writes every file under root/<pack>/ into destDir with the
// idempotent, clobber-safe harness-install semantics (matching = unchanged,
// differing = skipped unless opts.Force, DryRun plans). It errors if pack is not
// a known subdirectory of root.
func scaffoldPack(fsys fs.FS, root, pack, destDir string, opts HarnessInstallOptions) ([]PackScaffoldEntry, error) {
	if destDir == "" {
		return nil, fmt.Errorf("destination directory not resolved")
	}
	packs, err := listPackDirs(fsys, root)
	if err != nil {
		return nil, err
	}
	known := false
	for _, p := range packs {
		if p == pack {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown %s pack %q (available: %v)", root, pack, packs)
	}

	src := path.Join(root, pack)
	entries, err := fs.ReadDir(fsys, src)
	if err != nil {
		return nil, fmt.Errorf("read pack %q: %w", pack, err)
	}
	var out []PackScaffoldEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// path.Join for the embed FS source (always forward-slash); filepath.Join
		// for the on-disk destination (OS-specific).
		content, err := fs.ReadFile(fsys, path.Join(src, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		dest := filepath.Join(destDir, e.Name())
		action, err := installHarnessFile(dest, content, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, PackScaffoldEntry{Name: e.Name(), Dest: dest, Action: action})
	}
	return out, nil
}
