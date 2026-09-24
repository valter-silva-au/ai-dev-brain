package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// TestFileBacklogManager_CrossProcessConcurrentWriters stress-tests the
// cross-process file lock (#205, F1). It uses SEPARATE FileBacklogManager
// instances pointed at the SAME backlog.yaml — each instance has its own
// in-process sync.RWMutex, so the ONLY thing that can serialise their
// load→modify→save cycles is the cross-process OS file lock. Without that lock
// concurrent AddTask calls clobber each other (classic lost-update race) and
// the final file holds fewer than the expected number of tasks.
//
// A start barrier maximises the overlap so the race is reliably exercised.
func TestFileBacklogManager_CrossProcessConcurrentWriters(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "backlog.yaml")

	const (
		instances      = 10
		tasksPerWorker = 5
	)
	total := instances * tasksPerWorker

	var ready sync.WaitGroup
	var done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(instances)
	done.Add(instances)

	for i := 0; i < instances; i++ {
		go func(worker int) {
			defer done.Done()
			// A fresh manager per goroutine — distinct in-process locks, so
			// they can only coordinate through the cross-process file lock.
			fbm := NewFileBacklogManager(filePath)
			ready.Done()
			<-start
			for j := 0; j < tasksPerWorker; j++ {
				id := fmt.Sprintf("TASK-%02d%02d", worker, j)
				task := models.NewTask(id, fmt.Sprintf("task %d/%d", worker, j), models.TaskTypeFeat)
				if err := fbm.AddTask(*task); err != nil {
					t.Errorf("AddTask(%s) failed: %v", id, err)
				}
			}
		}(i)
	}

	ready.Wait()
	close(start)
	done.Wait()

	// The file must still be valid YAML.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("reading backlog after concurrent writers: %v", err)
	}
	var backlog models.Backlog
	if err := yaml.Unmarshal(data, &backlog); err != nil {
		t.Fatalf("backlog.yaml is not valid YAML after concurrent writers: %v", err)
	}

	// Every task must have survived — no lost updates.
	if len(backlog.Tasks) != total {
		t.Fatalf("expected %d tasks after concurrent writers, got %d (lost-update race)", total, len(backlog.Tasks))
	}

	// And every id must be present exactly once.
	seen := make(map[string]int, total)
	for _, task := range backlog.Tasks {
		seen[task.ID]++
	}
	for i := 0; i < instances; i++ {
		for j := 0; j < tasksPerWorker; j++ {
			id := fmt.Sprintf("TASK-%02d%02d", i, j)
			if seen[id] != 1 {
				t.Errorf("task %s present %d times, want 1", id, seen[id])
			}
		}
	}
}
