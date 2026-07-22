package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// Crash-safety proof for the shared temp-file-plus-rename replace primitive
// (atomicWriteFile), exercised through a real store write path — backend-readiness
// contract #2 for the cockpit spike (TASK-00070), spec-concurrency.md
// "Crash-safety test":
//
//	kill a writer between temp-write and rename → registry remains the old,
//	valid YAML (no torn file).
//
// The failure is injected with the testHookAfterTempWrite seam in atomicwrite.go
// (nil in every normal process). The parent seeds a valid registry, then re-execs
// THIS test binary as a child that performs a real FileStageStore write and, via
// the seam, prints crashHookMarker and os.Exit(crashHookExitCode)s after the temp
// file is fully written but before the rename. The parent then proves the on-disk
// registry is byte-identical to the pre-crash original, still parses, still loads
// to exactly the original record, and left at most one temp dotfile behind.
// atomicWriteFile is the shared save primitive for every registry, so proving it
// through one store (stagestore) covers the pattern for all of them.
//
// SOUNDNESS: a "child exited non-zero" check alone would false-pass — a child that
// died in setup/lock/write (t.Fatalf → exit 1) never reaches the injected
// pre-rename crash yet still leaves the original intact and satisfies every
// assertion vacuously. So the crash hook is made unmistakable: it emits a marker
// line and exits with a dedicated code, and the parent fails unless BOTH are
// present — proving the failure was injected exactly between temp-write and rename.
const (
	// crashHelperDirEnv, when set, switches this test binary into the crash-helper
	// child: it writes to the FileStageStore rooted at its value and dies mid-write.
	crashHelperDirEnv = "ADB_ATOMICWRITE_CRASH_HELPER_DIR"
	// crashHookMarker is printed by the child immediately before it exits from the
	// pre-rename hook; the parent asserts it in the child's combined output.
	crashHookMarker = "CRASH_HOOK_REACHED"
	// crashHookExitCode is the dedicated exit code the pre-rename hook uses, so the
	// parent can distinguish "crashed at the injected point" from any other
	// non-zero exit (a setup/lock/write failure exits 1 via t.Fatalf).
	crashHookExitCode = 42
)

// TestFileStageStore_CrashBeforeRenameLeavesOriginal is the parent assertion.
func TestFileStageStore_CrashBeforeRenameLeavesOriginal(t *testing.T) {
	if os.Getenv(crashHelperDirEnv) != "" {
		t.Skip("crash-helper worker invocation: the parent assertion test does not run in a worker")
	}
	dir := t.TempDir()

	// Seed the registry with a valid original the crashing writer must not damage.
	store := NewFileStageStore(dir)
	if err := store.CreateOrganization(models.Organization{ID: "keep-me", Name: "Original"}); err != nil {
		t.Fatalf("seeding original org: %v", err)
	}
	orgsPath := filepath.Join(dir, "orgs", "index.yaml")
	original, err := os.ReadFile(orgsPath)
	if err != nil {
		t.Fatalf("reading seeded registry: %v", err)
	}

	// Re-exec a child that calls the real store write path but dies (os.Exit)
	// after the temp file is written and before the rename.
	cmd := exec.Command(os.Args[0], "-test.run", "^TestAtomicWriteCrashHelperProcess$")
	cmd.Env = append(os.Environ(), crashHelperDirEnv+"="+dir)
	out, err := cmd.CombinedOutput()

	// The child must have died AT THE INJECTED CRASH — not in setup/lock/write,
	// which would also exit non-zero but leave the original intact for the wrong
	// reason. Require the dedicated exit code AND the marker the hook prints just
	// before exiting; either being absent means the pre-rename point was never
	// reached and this proof would otherwise pass vacuously.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected the crash-helper child to exit non-zero via the crash hook; got err=%v\n%s", err, out)
	}
	if code := exitErr.ExitCode(); code != crashHookExitCode {
		t.Fatalf("crash-helper child exited %d, want the dedicated crash-hook code %d "+
			"(a different code means it died before the pre-rename hook, not at it):\n%s", code, crashHookExitCode, out)
	}
	if !bytes.Contains(out, []byte(crashHookMarker)) {
		t.Fatalf("crash-helper child never printed %q, so it did not reach the pre-rename hook:\n%s", crashHookMarker, out)
	}

	// The registry on disk must be byte-identical to the pre-crash original: the
	// rename never happened, so no partial write and no second record landed.
	got, err := os.ReadFile(orgsPath)
	if err != nil {
		t.Fatalf("reading registry after crash: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("registry changed after a crash before rename:\n got: %q\nwant: %q", got, original)
	}

	// It must still parse as valid YAML and still load to exactly the original org.
	var index OrgIndex
	if err := yaml.Unmarshal(got, &index); err != nil {
		t.Fatalf("registry is not valid YAML after crash: %v", err)
	}
	reopened := NewFileStageStore(dir)
	orgs, err := reopened.ListOrganizations()
	if err != nil {
		t.Fatalf("listing orgs after crash: %v", err)
	}
	if len(orgs) != 1 || orgs[0].ID != "keep-me" {
		t.Fatalf("expected exactly the original org after crash, got %+v", orgs)
	}

	// The only artifact the aborted write may leave is at most one temp dotfile
	// (os.Exit skips atomicWriteFile's cleanup defer); the registry lock sidecar
	// (index.yaml.lock) is expected and is not a temp file.
	entries, err := os.ReadDir(filepath.Join(dir, "orgs"))
	if err != nil {
		t.Fatalf("reading registry dir after crash: %v", err)
	}
	tmpLeftovers := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".index.yaml.tmp-") {
			tmpLeftovers++
		}
	}
	if tmpLeftovers > 1 {
		t.Fatalf("expected at most one leftover temp file, found %d", tmpLeftovers)
	}
}

// TestAtomicWriteCrashHelperProcess is the crash-helper child branch of
// TestFileStageStore_CrashBeforeRenameLeavesOriginal. It runs ONLY when re-exec'd
// with crashHelperDirEnv set; a normal `go test` run skips it (and so never
// assigns the testHookAfterTempWrite seam).
func TestAtomicWriteCrashHelperProcess(t *testing.T) {
	dir := os.Getenv(crashHelperDirEnv)
	if dir == "" {
		t.Skip("not an atomic-write crash-helper process")
	}
	// Simulate a crash: die after the temp file is written but before the rename.
	// This assignment happens ONLY in the re-exec'd child (guarded by the env
	// above); the parent process never reaches it, so the seam stays inert
	// everywhere except this deliberately-crashing child. The marker + dedicated
	// exit code let the parent prove the crash landed at THIS point (os.Stdout is
	// unbuffered, so the marker is flushed before os.Exit skips deferred cleanup).
	testHookAfterTempWrite = func() {
		fmt.Fprintln(os.Stdout, crashHookMarker)
		os.Exit(crashHookExitCode)
	}

	store := NewFileStageStore(dir)
	if err := store.CreateOrganization(models.Organization{ID: "crash-me", Name: "should never land"}); err != nil {
		t.Fatalf("unexpected error before the crash hook fired: %v", err)
	}
	t.Fatal("atomicWriteFile returned without the crash hook firing")
}
