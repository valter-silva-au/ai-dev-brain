package storage_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/storage"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// Cross-process proof that the PRODUCTION ADR allocation path is race-free —
// backend-readiness contract #2 for the cockpit spike (TASK-00070). Unlike the
// store-level append proof in registry_lock_test.go, the workers here drive the
// real manager entrypoint core.NewADRManager(store).New(...) (which calls
// FileADRStore.CreateNext under the hood), so a regression that reintroduced a
// NextNumber+Create race in ADRManager.New — not just in the store — is caught.
//
// It lives in the EXTERNAL package storage_test because it imports internal/core,
// and core imports internal/storage; a white-box (package storage) test importing
// core would be an "import cycle not allowed in test". The re-exec helper still
// runs in the same test binary, so -test.run targets it by name across packages.
const (
	// allocHelperDirEnv, when set, switches this test binary into an allocation
	// worker operating on the ADR registry rooted at its value.
	allocHelperDirEnv = "ADB_ADR_ALLOC_HELPER_DIR"
	// allocHelperWorkerEnv carries the worker index (only for distinct titles /
	// diagnostics; the STORE, not the worker, allocates each number).
	allocHelperWorkerEnv = "ADB_ADR_ALLOC_HELPER_WORKER"

	allocWorkers   = 8
	allocPerWorker = 12
)

// TestFileADRStore_CrossProcessConcurrentAllocation spawns allocWorkers real OS
// processes, each allocating allocPerWorker ADRs through ADRManager.New WITHOUT
// pre-assigning numbers, so the store picks every number. Separate processes
// share no in-process mutex, so only the cross-process lock inside CreateNext can
// stop two workers reading the same max and allocating a duplicate. The registry
// must therefore end with exactly N*M ADRs numbered 1..N*M — unique and
// sequential-dense, no gaps. A non-atomic NextNumber-then-Create would hand two
// workers the same number: one Create wins, the other fails the duplicate check
// (a non-zero worker exit) AND leaves a gap.
func TestFileADRStore_CrossProcessConcurrentAllocation(t *testing.T) {
	if os.Getenv(allocHelperDirEnv) != "" {
		t.Skip("worker-process invocation: the parent test does not run in a worker")
	}
	dir := t.TempDir()

	var wg sync.WaitGroup
	errs := make(chan error, allocWorkers)
	for w := 0; w < allocWorkers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run", "^TestADRManagerAllocationHelperProcess$")
			cmd.Env = append(os.Environ(),
				allocHelperDirEnv+"="+dir,
				allocHelperWorkerEnv+"="+strconv.Itoa(worker),
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

	data, err := os.ReadFile(filepath.Join(dir, "adr", "index.yaml"))
	if err != nil {
		t.Fatalf("reading adr index after concurrent allocators: %v", err)
	}
	var index models.ADRIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		t.Fatalf("adr/index.yaml is not valid YAML after concurrent allocators: %v", err)
	}

	want := allocWorkers * allocPerWorker
	if len(index.ADRs) != want {
		t.Fatalf("expected %d ADRs after concurrent allocation, got %d (lost update or duplicate-number failure)", want, len(index.ADRs))
	}
	// Numbers must be unique AND sequential-dense 1..want — the property a
	// non-atomic allocate-then-create would violate.
	seen := make(map[int]bool, want)
	for _, adr := range index.ADRs {
		if adr.Number < 1 || adr.Number > want {
			t.Errorf("adr number %d out of range [1,%d]", adr.Number, want)
		}
		if seen[adr.Number] {
			t.Errorf("duplicate adr number %d", adr.Number)
		}
		seen[adr.Number] = true
	}
	for n := 1; n <= want; n++ {
		if !seen[n] {
			t.Errorf("missing adr number %d (allocation not sequential-dense)", n)
		}
	}

	// The persisted markdown for every ADR must be consistent with its ALLOCATED
	// number. The workers use titles that slugify to empty, so each slug is the
	// number-derived fallback adr-NNNN and each filename/heading is a pure function
	// of the number the store allocated. Because CreateNext runs build with that
	// number and REJECTS a callback returning a different one, this proves the
	// number in the index matches the number baked into the file on disk (guards
	// the build-vs-forced-number desync hazard).
	for _, adr := range index.ADRs {
		wantSlug := fmt.Sprintf("adr-%04d", adr.Number)
		if adr.Slug != wantSlug {
			t.Errorf("adr %d slug = %q, want number-derived fallback %q", adr.Number, adr.Slug, wantSlug)
		}
		body, err := os.ReadFile(filepath.Join(dir, "docs", "adr", adr.Filename()))
		if err != nil {
			t.Errorf("adr %d markdown missing at %s: %v", adr.Number, adr.Filename(), err)
			continue
		}
		wantHeading := fmt.Sprintf("# %d. %s", adr.Number, adr.Title)
		if !strings.HasPrefix(string(body), wantHeading) {
			t.Errorf("adr %d markdown does not open with %q (body/number desync)", adr.Number, wantHeading)
		}
	}
}

// TestADRManagerAllocationHelperProcess is the worker-process branch of
// TestFileADRStore_CrossProcessConcurrentAllocation. It drives the real
// production allocation path: the manager's New calls store.CreateNext.
func TestADRManagerAllocationHelperProcess(t *testing.T) {
	dir := os.Getenv(allocHelperDirEnv)
	if dir == "" {
		t.Skip("not an adr-allocation worker process")
	}
	worker, err := strconv.Atoi(os.Getenv(allocHelperWorkerEnv))
	if err != nil {
		t.Fatalf("bad worker index %q: %v", os.Getenv(allocHelperWorkerEnv), err)
	}
	mgr := core.NewADRManager(storage.NewFileADRStore(dir))
	for j := 0; j < allocPerWorker; j++ {
		// A title made only of non-[a-z0-9] runes slugifies to empty, forcing the
		// manager's number-derived fallback slug (adr-NNNN). That makes each
		// persisted filename a pure function of the ALLOCATED number, so the parent
		// can assert filename/heading consistency without knowing which process won
		// which number. It still satisfies New's non-empty-title guard.
		title := strings.Repeat("#", worker+1) + "." + strings.Repeat("#", j+1)
		if _, err := mgr.New(title, nil); err != nil {
			t.Fatalf("worker %d New #%d: %v", worker, j, err)
		}
	}
}
