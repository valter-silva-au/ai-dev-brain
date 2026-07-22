package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path atomically. It creates the parent
// directory if needed (0o755), writes to a temp file IN THE SAME directory, then
// renames it over path. Because the committing step is a rename within one
// directory, a reader concurrent with the write sees either the whole previous
// file or the whole new one — never a half-written file, unlike os.WriteFile,
// which truncates the target and then writes in place. On Unix this is rename(2);
// on Windows os.Rename issues MoveFileEx with MOVEFILE_REPLACE_EXISTING, so the
// replace is atomic on both platforms.
//
// It is the shared save primitive for the mutable YAML registries; callers pair
// it with a cross-process lock (see acquireRegistryLock) held around the whole
// load-modify-save cycle so writers never interleave.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// A dotfile temp in the same directory keeps the rename on one filesystem and
	// keeps the partial file out of any registry glob.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// If anything below fails before the rename lands, remove the temp file so a
	// failed write never litters the directory.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file into place: %w", err)
	}
	committed = true
	return nil
}
