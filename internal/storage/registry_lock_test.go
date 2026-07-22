package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// Cross-process contention proof for the founder-playbook YAML registries
// (stagestore + adrstore) — backend-readiness contract #2 for the cockpit spike
// (TASK-00070). It extends the backlog lock proof (backlog_lock_test.go) to the
// registries this PR put behind the same cross-process lock + atomic replace.
//
// Unlike the backlog test — which uses separate manager instances in ONE process
// (distinct in-process mutexes coordinating only through the OS file lock) —
// these tests spawn REAL separate OS processes via the standard Go re-exec
// pattern: the parent re-runs THIS test binary with helper env vars set; the
// helper-process branch performs N create cycles through the store; the parent
// spawns several such processes against the SAME registry and afterwards asserts
// ZERO lost updates (every process's writes survive). Separate processes cannot
// share an in-process mutex, so the ONLY thing that can serialise their
// load-modify-save cycles is the cross-process file lock. Without it, concurrent
// appends clobber each other and the final registry holds fewer records than
// expected — flip acquireRegistryLock to a no-op and these tests fail.

const (
	// lockHelperDirEnv, when set, switches this test binary into a contention
	// worker process operating on the registry rooted at its value.
	lockHelperDirEnv = "ADB_REGISTRY_LOCK_HELPER_DIR"
	// lockHelperWorkerEnv carries the worker's index so it writes a disjoint set
	// of records (unique IDs / ADR numbers), isolating the lost-update signal to
	// concurrent appends rather than key collisions.
	lockHelperWorkerEnv = "ADB_REGISTRY_LOCK_HELPER_WORKER"

	lockContentionWorkers   = 8
	lockContentionPerWorker = 12
)

// spawnLockContentionWorkers re-execs this test binary once per worker — each in
// its own OS process, all pointed at dir — and fails t if any worker process
// exits non-zero. runName is the ^Name$-anchored helper test the child runs.
func spawnLockContentionWorkers(t *testing.T, runName, dir string) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, lockContentionWorkers)
	for w := 0; w < lockContentionWorkers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run", runName)
			cmd.Env = append(os.Environ(),
				lockHelperDirEnv+"="+dir,
				lockHelperWorkerEnv+"="+strconv.Itoa(worker),
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				errs <- fmt.Errorf("worker %d process failed: %v\n%s", worker, err, out)
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestFileStageStore_CrossProcessConcurrentWriters proves CreateOrganization is
// safe across processes: N processes each append lockContentionPerWorker orgs to
// the shared orgs/index.yaml, and every one must survive.
func TestFileStageStore_CrossProcessConcurrentWriters(t *testing.T) {
	if os.Getenv(lockHelperDirEnv) != "" {
		t.Skip("worker-process invocation: the parent test does not run in a worker")
	}
	dir := t.TempDir()

	spawnLockContentionWorkers(t, "^TestStageStoreLockContentionHelperProcess$", dir)

	// The registry must still be valid YAML after concurrent cross-process writers.
	data, err := os.ReadFile(filepath.Join(dir, "orgs", "index.yaml"))
	if err != nil {
		t.Fatalf("reading orgs index after concurrent writers: %v", err)
	}
	var index OrgIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		t.Fatalf("orgs/index.yaml is not valid YAML after concurrent writers: %v", err)
	}

	// Every org from every worker must have survived — no lost updates.
	want := lockContentionWorkers * lockContentionPerWorker
	if len(index.Organizations) != want {
		t.Fatalf("expected %d orgs after concurrent writers, got %d (lost-update race)", want, len(index.Organizations))
	}
	seen := make(map[string]int, want)
	for _, org := range index.Organizations {
		seen[org.ID]++
	}
	for w := 0; w < lockContentionWorkers; w++ {
		for j := 0; j < lockContentionPerWorker; j++ {
			id := fmt.Sprintf("org-%02d-%02d", w, j)
			if seen[id] != 1 {
				t.Errorf("org %s present %d times, want 1", id, seen[id])
			}
		}
	}
}

// TestStageStoreLockContentionHelperProcess is the worker-process branch of
// TestFileStageStore_CrossProcessConcurrentWriters. It runs ONLY when re-exec'd
// with the helper env vars set; a normal `go test` run skips it.
func TestStageStoreLockContentionHelperProcess(t *testing.T) {
	dir := os.Getenv(lockHelperDirEnv)
	if dir == "" {
		t.Skip("not a lock-contention worker process")
	}
	worker, err := strconv.Atoi(os.Getenv(lockHelperWorkerEnv))
	if err != nil {
		t.Fatalf("bad worker index %q: %v", os.Getenv(lockHelperWorkerEnv), err)
	}
	store := NewFileStageStore(dir)
	for j := 0; j < lockContentionPerWorker; j++ {
		id := fmt.Sprintf("org-%02d-%02d", worker, j)
		if err := store.CreateOrganization(models.Organization{ID: id, Name: id}); err != nil {
			t.Fatalf("worker %d CreateOrganization(%s): %v", worker, id, err)
		}
	}
}

// TestFileADRStore_CrossProcessConcurrentWriters proves the lower-level Create is
// safe across processes: N processes each append lockContentionPerWorker ADRs
// with disjoint pre-assigned numbers, isolating the shared-index APPEND lock.
// The allocation race (concurrent number assignment) is proven separately by
// TestFileADRStore_CrossProcessConcurrentAllocation (registry_alloc_test.go),
// which drives the PRODUCTION ADRManager.New → CreateNext path across processes.
func TestFileADRStore_CrossProcessConcurrentWriters(t *testing.T) {
	if os.Getenv(lockHelperDirEnv) != "" {
		t.Skip("worker-process invocation: the parent test does not run in a worker")
	}
	dir := t.TempDir()

	spawnLockContentionWorkers(t, "^TestADRStoreLockContentionHelperProcess$", dir)

	data, err := os.ReadFile(filepath.Join(dir, "adr", "index.yaml"))
	if err != nil {
		t.Fatalf("reading adr index after concurrent writers: %v", err)
	}
	var index models.ADRIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		t.Fatalf("adr/index.yaml is not valid YAML after concurrent writers: %v", err)
	}

	want := lockContentionWorkers * lockContentionPerWorker
	if len(index.ADRs) != want {
		t.Fatalf("expected %d ADRs after concurrent writers, got %d (lost-update race)", want, len(index.ADRs))
	}
	seen := make(map[int]int, want)
	for _, adr := range index.ADRs {
		seen[adr.Number]++
	}
	for w := 0; w < lockContentionWorkers; w++ {
		for j := 0; j < lockContentionPerWorker; j++ {
			number := adrNumberFor(w, j)
			if seen[number] != 1 {
				t.Errorf("adr %d present %d times, want 1", number, seen[number])
			}
		}
	}
}

// TestADRStoreLockContentionHelperProcess is the worker-process branch of
// TestFileADRStore_CrossProcessConcurrentWriters.
func TestADRStoreLockContentionHelperProcess(t *testing.T) {
	dir := os.Getenv(lockHelperDirEnv)
	if dir == "" {
		t.Skip("not a lock-contention worker process")
	}
	worker, err := strconv.Atoi(os.Getenv(lockHelperWorkerEnv))
	if err != nil {
		t.Fatalf("bad worker index %q: %v", os.Getenv(lockHelperWorkerEnv), err)
	}
	store := NewFileADRStore(dir)
	for j := 0; j < lockContentionPerWorker; j++ {
		number := adrNumberFor(worker, j)
		slug := fmt.Sprintf("adr-%02d-%02d", worker, j)
		adr := models.ADR{Number: number, Title: slug, Status: models.ADRProposed, Slug: slug}
		if err := store.Create(adr, "# "+slug+"\n"); err != nil {
			t.Fatalf("worker %d Create(%d): %v", worker, number, err)
		}
	}
}

// adrNumberFor gives each (worker, index) pair a globally unique ADR number in a
// disjoint per-worker range, so concurrent Create calls never collide on number
// (which would fail on the duplicate check rather than exercise the lock).
func adrNumberFor(worker, j int) int { return (worker+1)*1000 + j }

// The PRODUCTION allocation path (ADRManager.New → FileADRStore.CreateNext) is
// proven race-free by TestFileADRStore_CrossProcessConcurrentAllocation, which
// lives in registry_alloc_test.go (external package storage_test) because it
// imports internal/core, and core imports internal/storage — a white-box test
// here importing core would be an import cycle.

// TestFileStageStore_LockDisabledLosesUpdates_ManualSanityCheck is the DURABLE
// record of the lock's negative control. It is skipped in CI (reproducing the
// failure needs a deliberate, temporary production edit — we do NOT ship a
// runtime toggle), but it documents exactly how to reproduce the lost-update
// failure and the result observed with the lock removed.
//
// PROCEDURE (run by a maintainer, reverted afterwards):
//  1. In internal/storage/registrylock.go, make acquireRegistryLock a no-op that
//     returns func(){} (and closes the file) WITHOUT calling lockfile.Lock.
//  2. Re-run TestFileStageStore_CrossProcessConcurrentWriters.
//  3. Observe it FAIL — the 8×12=96 expected orgs come back short because the
//     concurrent load-modify-save cycles clobber each other.
//  4. Revert the edit; confirm the test passes again (96/96).
//
// OBSERVED (2026-07-22, this Windows box): with the lock disabled the run
// produced 13 of 96 orgs — "expected 96 orgs after concurrent writers, got 13
// (lost-update race)". With the lock enabled the same test is 96/96. That delta
// is the proof the lock is load-bearing and the contention tests genuinely detect
// the race rather than passing vacuously.
func TestFileStageStore_LockDisabledLosesUpdates_ManualSanityCheck(t *testing.T) {
	t.Skip("documentation-only negative control; see the doc comment for the manual procedure and the observed 13/96 result")
}
