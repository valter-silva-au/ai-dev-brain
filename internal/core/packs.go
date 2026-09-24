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
//
// The json tags are load-bearing, not decoration: this type is what
// `adb program scaffold --json` used to marshal directly, and without tags
// encoding/json falls back to Go field names — so that one subcommand emitted
// PascalCase `Name`/`Dest`/`Action` while every other `adb program` subcommand
// emitted snake_case. Tagging it snake_case makes the whole command group read
// with one convention.
type PackScaffoldEntry struct {
	Name   string               `json:"name"`
	Dest   string               `json:"dest"`
	Action HarnessInstallAction `json:"action"` // installed | unchanged | skipped
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

// scaffoldPackTree is the RECURSIVE sibling of scaffoldPack: it walks root/<pack>/
// with fs.WalkDir and preserves each file's relative subpath into destDir, so a
// pack can be a tree (a document program keeps its manifest at the pack root and
// its templates under templates/<phase>/). The install semantics are identical —
// it reuses installHarnessFile, so matching content is "unchanged", differing
// content is "skipped" unless opts.Force, and DryRun plans without writing.
//
// Entry.Name is the SLASH-relative subpath within the pack (e.g.
// "templates/design/design-doc.md") so callers can print a stable, OS-independent
// name; Entry.Dest is the OS-specific on-disk path.
func scaffoldPackTree(fsys fs.FS, root, pack, destDir string, opts HarnessInstallOptions) ([]PackScaffoldEntry, error) {
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
	var out []PackScaffoldEntry
	err = fs.WalkDir(fsys, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", p, err)
		}
		// p is an embed-FS path (always forward-slash); the destination is
		// OS-specific, so convert the relative subpath before joining.
		content, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		action, err := installHarnessFile(dest, content, opts)
		if err != nil {
			return err
		}
		out = append(out, PackScaffoldEntry{Name: filepath.ToSlash(rel), Dest: dest, Action: action})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scaffold pack %q: %w", pack, err)
	}
	return out, nil
}
