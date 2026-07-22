//go:build windows

package storage

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// TestAtomicWriteFile_ReadOnlyTargetFailsFast proves the read-only-target fast-fail:
// on Windows the read-only attribute blocks MoveFileEx permanently, so the bounded
// retry must NOT spend its budget on it. Counting os.Rename attempts through the
// testHookRenameAttempt seam, a write over a read-only target must make EXACTLY ONE
// attempt and return an error, and must leave the target untouched (all-or-nothing).
//
// The complementary half — a transient sharing violation on a WRITABLE target must
// RETRY (>= 2 attempts) — is proven by TestAtomicWriteFile_ConcurrentOpenReader.
// Together the two tests pin both branches of the retry classification.
func TestAtomicWriteFile_ReadOnlyTargetFailsFast(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(target, []byte("old: true\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	// Mark the target read-only so MoveFileEx returns ACCESS_DENIED permanently.
	if err := os.Chmod(target, 0o444); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	defer func() { _ = os.Chmod(target, 0o644) }() // restore for TempDir cleanup

	var attempts atomic.Int32
	testHookRenameAttempt = func(_ int, _ error) { attempts.Add(1) }
	defer func() { testHookRenameAttempt = nil }()

	err := atomicWriteFile(target, []byte("new: true\n"), 0o644)
	if err == nil {
		t.Fatal("atomicWriteFile over a read-only target should fail, got nil")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("read-only target should fail fast in exactly 1 attempt, got %d (the retry burned its budget on a permanent condition)", got)
	}

	// A failed replace must not mutate the target — it stays the original content.
	got, rerr := os.ReadFile(target)
	if rerr != nil {
		t.Fatalf("read target after failed write: %v", rerr)
	}
	if string(got) != "old: true\n" {
		t.Fatalf("target content = %q, want the original (a failed replace must not touch it)", got)
	}
}

// TestTargetIsReadOnly_LstatNotStat proves targetIsReadOnly classifies a symlink by
// the LINK's own attribute, not the referent's. A writable symlink pointing at a
// read-only file must NOT be treated as a permanent (read-only) target — MoveFileEx
// replaces the link (the reparse point), whose own read-only attribute is what
// governs whether the rename is permanently blocked. os.Stat would follow the link
// to the read-only referent and misclassify a transient denial as permanent, failing
// fast on a rename that would otherwise have retried and succeeded; os.Lstat does not.
//
// Symlink creation on Windows needs privilege (elevation or Developer Mode); when it
// is unavailable the test skips rather than failing.
func TestTargetIsReadOnly_LstatNotStat(t *testing.T) {
	dir := t.TempDir()
	referent := filepath.Join(dir, "referent.yaml")
	if err := os.WriteFile(referent, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("seed referent: %v", err)
	}
	if err := os.Chmod(referent, 0o444); err != nil {
		t.Fatalf("chmod referent read-only: %v", err)
	}
	defer func() { _ = os.Chmod(referent, 0o644) }()

	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(referent, link); err != nil {
		t.Skipf("cannot create symlink (needs privilege/Developer Mode): %v", err)
	}

	// The link itself is writable; only its referent is read-only. Lstat-based
	// classification must therefore report false (not a permanent read-only target).
	if targetIsReadOnly(link) {
		t.Fatal("targetIsReadOnly followed the symlink to its read-only referent; it must inspect the link itself (os.Lstat)")
	}
}
