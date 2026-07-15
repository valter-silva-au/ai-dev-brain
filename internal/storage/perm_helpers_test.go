package storage

import (
	"os"
	"testing"
)

// assertOwnerReadWritableFile checks the portable contract for adb
// files: exists, is regular (not symlink/dir), and is readable+
// writable by the current user. Replaces exact mode-bit matching
// (0o644) which fails on Windows where os.Stat always reports 0o666.
func assertOwnerReadWritableFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("%q should be a regular file, got mode %v", path, info.Mode())
	}
	if info.Mode().Perm()&0o600 != 0o600 {
		t.Errorf("%q must be readable+writable by owner, got mode %o", path, info.Mode().Perm())
	}
	// Prove writability with an open-for-append (guards against
	// Windows mode-bit-lies where Stat claims rw but the ACL denies).
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Errorf("open %q for read+append: %v", path, err)
		return
	}
	_ = f.Close()
}

// assertOwnerAccessibleDir checks the portable contract for adb
// directories: exists, is a directory, and is readable+writable+
// traversable by the owner (rwx = 0o700 bit set).
func assertOwnerAccessibleDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if !info.IsDir() {
		t.Errorf("%q should be a directory, got mode %v", path, info.Mode())
	}
	if info.Mode().Perm()&0o700 != 0o700 {
		t.Errorf("%q must be rwx by owner, got mode %o", path, info.Mode().Perm())
	}
}
