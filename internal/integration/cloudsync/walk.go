package cloudsync

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// WalkUploadSet walks the workspace at root and returns the sorted list of
// workspace-relative paths eligible for cloud upload. It short-circuits
// (returns filepath.SkipDir) whenever it enters a denied directory, so we
// never descend into communications/, sessions/, work/, repos/, .omnictx/,
// .adb/ — matters for security (no accidental read of private trees) AND
// for performance (repos/ mirrors are huge). Symlinks are NOT followed
// (filepath.WalkDir uses os.Lstat by default).
func WalkUploadSet(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Tolerate errors on the root only; anywhere else, an error
			// deep in the tree can indicate a permission issue on a
			// dir we don't care about (e.g. repos/, which we prune
			// below anyway). Skip the entry, don't fail the whole walk.
			if path == root {
				return walkErr
			}
			// If the erroring path is (or is inside) a denied segment,
			// swallow silently. Otherwise surface the error.
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil {
				segs := splitClean(rel)
				if isDenied(segs) {
					if d != nil && d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		segs := splitClean(rel)

		// If ANY segment is denied, prune here.
		if isDenied(segs) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			// Directories themselves aren't uploaded; keep walking.
			return nil
		}
		// Skip symlinks (both file-symlinks and dir-symlinks). filepath.WalkDir
		// uses Lstat so a symlink APPEARS here as a regular DirEntry with the
		// ModeSymlink bit set; if we let it through, stageFile would open the
		// TARGET (out-of-tree) and copy its contents into the upload set.
		// Fail-CLOSED: symlinks are always skipped, even inside include roots.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if !ShouldUpload(rel) {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// splitClean returns the /-separated segments of a workspace-relative path.
// Shared by allowlist.go and walk.go so both apply the same normalisation.
func splitClean(rel string) []string {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return nil
	}
	raw := strings.Split(rel, "/")
	segs := raw[:0]
	for _, s := range raw {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}
