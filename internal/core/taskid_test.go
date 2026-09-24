package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestNewFileTaskIDGenerator(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")
	prefix := "TASK"

	gen := NewFileTaskIDGenerator(counterFile, prefix)
	if gen == nil {
		t.Fatal("NewFileTaskIDGenerator returned nil")
	}
	if gen.counterFile != counterFile {
		t.Errorf("Expected counterFile %s, got %s", counterFile, gen.counterFile)
	}
	if gen.prefix != prefix {
		t.Errorf("Expected prefix %s, got %s", prefix, gen.prefix)
	}
}

func TestGenerateTaskID_FirstID(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")
	taskID, err := gen.GenerateTaskID()

	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}
	if taskID != "TASK-00001" {
		t.Errorf("Expected first task ID to be TASK-00001, got %s", taskID)
	}

	// Verify counter file was created
	if _, err := os.Stat(counterFile); os.IsNotExist(err) {
		t.Fatal("Counter file was not created")
	}

	// Verify counter file content
	content, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("Failed to read counter file: %v", err)
	}
	if strings.TrimSpace(string(content)) != "1" {
		t.Errorf("Expected counter file content to be '1', got '%s'", strings.TrimSpace(string(content)))
	}
}

func TestGenerateTaskID_Sequential(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")

	// Generate multiple IDs sequentially
	expectedIDs := []string{
		"TASK-00001",
		"TASK-00002",
		"TASK-00003",
		"TASK-00004",
		"TASK-00005",
	}

	for _, expectedID := range expectedIDs {
		taskID, err := gen.GenerateTaskID()
		if err != nil {
			t.Fatalf("GenerateTaskID() failed: %v", err)
		}
		if taskID != expectedID {
			t.Errorf("Expected task ID %s, got %s", expectedID, taskID)
		}
	}
}

func TestGenerateTaskID_CustomPrefix(t *testing.T) {
	tempDir := t.TempDir()

	testCases := []struct {
		prefix   string
		expected string
	}{
		{"BUG", "BUG-00001"},
		{"FEAT", "FEAT-00001"},
		{"TEST", "TEST-00001"},
		{"STORY", "STORY-00001"},
		{"", "-00001"}, // empty prefix
	}

	for _, tc := range testCases {
		t.Run(tc.prefix, func(t *testing.T) {
			// Use separate counter file for each test case
			cf := filepath.Join(tempDir, fmt.Sprintf(".counter_%s", tc.prefix))
			gen := NewFileTaskIDGenerator(cf, tc.prefix)

			taskID, err := gen.GenerateTaskID()
			if err != nil {
				t.Fatalf("GenerateTaskID() failed: %v", err)
			}
			if taskID != tc.expected {
				t.Errorf("Expected task ID %s, got %s", tc.expected, taskID)
			}
		})
	}
}

// seedCounter writes v into the counter file using exactly the byte
// representation writeCounter produces, so a seeded counter is
// indistinguishable from one the generator itself advanced to v. The
// format-at-width tests below depend on that equivalence, and every one of
// them re-asserts it by reading the file back after a generate.
func seedCounter(t *testing.T, counterFile string, v int) {
	t.Helper()
	if err := os.WriteFile(counterFile, []byte(fmt.Sprintf("%d\n", v)), 0o644); err != nil {
		t.Fatalf("seed counter file: %v", err)
	}
}

// readCounterFile returns the persisted counter value, trimmed.
func readCounterFile(t *testing.T, counterFile string) string {
	t.Helper()
	content, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read counter file: %v", err)
	}
	return strings.TrimSpace(string(content))
}

// TestGenerateTaskID_Format pins the ID format at every decimal width the
// counter can reach, including across the 5-digit boundary where the padded
// width has to grow rather than truncate.
//
// It reaches those widths by seeding the counter file rather than by counting
// up to them one call at a time. That is sound because the counter is
// persisted as a decimal integer and re-read from disk on every call:
// formatting is a per-call function of the value on disk and carries no
// in-memory state across calls, so an ID's shape cannot depend on how the
// counter arrived at its value. Counting to 100000 cost ~100k open/truncate/
// write/fsync/close cycles (~479s wall on an APFS laptop, ~4.8ms each, ~90%
// of it blocked on fsync) and still only asserted 7 of the 100k IDs it
// produced. The contiguous-run test below covers "the counter never skips or
// repeats" directly, and asserts every ID rather than the last of a batch.
func TestGenerateTaskID_Format(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{"one digit", 1, "TASK-00001"},
		{"two digits", 10, "TASK-00010"},
		{"three digits", 100, "TASK-00100"},
		{"four digits", 1000, "TASK-01000"},
		{"five digits fill the pad exactly", 10000, "TASK-10000"},
		{"last five-digit id", 99999, "TASK-99999"},
		{"first six-digit id widens, never truncates", 100000, "TASK-100000"},
		{"seven digits", 1234567, "TASK-1234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			counterFile := filepath.Join(t.TempDir(), ".task_counter")
			if tt.count > 1 {
				seedCounter(t, counterFile, tt.count-1)
			}

			gen := NewFileTaskIDGenerator(counterFile, "TASK")
			got, err := gen.GenerateTaskID()
			if err != nil {
				t.Fatalf("GenerateTaskID() failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("counter %d: expected task ID %s, got %s", tt.count, tt.want, got)
			}

			// The generator's own persisted representation must match the one
			// seedCounter writes — otherwise seeding would be exercising a
			// different read path than a real run does.
			if persisted := readCounterFile(t, counterFile); persisted != strconv.Itoa(tt.count) {
				t.Errorf("counter file holds %q, want %q", persisted, strconv.Itoa(tt.count))
			}
		})
	}
}

// TestGenerateTaskID_ContiguousRun asserts that a run of consecutive calls
// yields consecutive IDs with no skip, no repeat, and no width glitch at a
// decimal boundary — checking *every* ID in the run. The count-to-100000 test
// this replaces only ever compared the last ID of each batch, so
// TASK-00002..TASK-00009 (and ~99993 others) were generated and never
// asserted; a skip that landed inside a batch and was compensated for later
// would have gone unnoticed.
func TestGenerateTaskID_ContiguousRun(t *testing.T) {
	tests := []struct {
		name string
		from int // counter value seeded before the run (0 = fresh file)
		n    int // number of consecutive IDs to generate and assert
	}{
		{"from a fresh counter across the 1-2-3 digit widths", 0, 105},
		{"across the four-to-five digit boundary", 9998, 5},
		{"across the five-to-six digit boundary", 99997, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			counterFile := filepath.Join(t.TempDir(), ".task_counter")
			if tt.from > 0 {
				seedCounter(t, counterFile, tt.from)
			}

			gen := NewFileTaskIDGenerator(counterFile, "TASK")
			for i := 0; i < tt.n; i++ {
				want := fmt.Sprintf("TASK-%05d", tt.from+1+i)
				got, err := gen.GenerateTaskID()
				if err != nil {
					t.Fatalf("call %d: GenerateTaskID() failed: %v", i+1, err)
				}
				if got != want {
					t.Fatalf("call %d: expected task ID %s, got %s", i+1, want, got)
				}
			}

			// The run must also be durable: the file records where it stopped.
			if persisted, want := readCounterFile(t, counterFile), strconv.Itoa(tt.from+tt.n); persisted != want {
				t.Errorf("counter file holds %q after the run, want %q", persisted, want)
			}
		})
	}
}

func TestGenerateTaskID_PersistentCounter(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	// Create first generator and generate some IDs
	gen1 := NewFileTaskIDGenerator(counterFile, "TASK")
	for i := 0; i < 5; i++ {
		_, err := gen1.GenerateTaskID()
		if err != nil {
			t.Fatalf("GenerateTaskID() failed: %v", err)
		}
	}

	// Create second generator with same counter file
	gen2 := NewFileTaskIDGenerator(counterFile, "TASK")
	taskID, err := gen2.GenerateTaskID()
	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}

	// Should continue from where gen1 left off
	if taskID != "TASK-00006" {
		t.Errorf("Expected task ID to be TASK-00006 (continuing from previous), got %s", taskID)
	}
}

func TestGenerateTaskID_ConcurrentGeneration(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")

	// Number of concurrent goroutines
	numGoroutines := 100
	numIDsPerGoroutine := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel to collect generated IDs
	idChan := make(chan string, numGoroutines*numIDsPerGoroutine)

	// Generate IDs concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIDsPerGoroutine; j++ {
				taskID, err := gen.GenerateTaskID()
				if err != nil {
					t.Errorf("GenerateTaskID() failed: %v", err)
					return
				}
				idChan <- taskID
			}
		}()
	}

	wg.Wait()
	close(idChan)

	// Collect all generated IDs
	generatedIDs := make(map[string]bool)
	for id := range idChan {
		if generatedIDs[id] {
			t.Errorf("Duplicate task ID generated: %s", id)
		}
		generatedIDs[id] = true
	}

	// Verify we got the expected number of unique IDs
	expectedCount := numGoroutines * numIDsPerGoroutine
	if len(generatedIDs) != expectedCount {
		t.Errorf("Expected %d unique IDs, got %d", expectedCount, len(generatedIDs))
	}

	// Verify IDs are in expected range
	for i := 1; i <= expectedCount; i++ {
		expectedID := fmt.Sprintf("TASK-%05d", i)
		if !generatedIDs[expectedID] {
			t.Errorf("Expected ID %s not found in generated IDs", expectedID)
		}
	}
}

func TestGenerateTaskID_MultipleGeneratorsConcurrent(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	// Create multiple generators sharing the same counter file
	numGenerators := 10
	numIDsPerGenerator := 20

	generators := make([]*FileTaskIDGenerator, numGenerators)
	for i := 0; i < numGenerators; i++ {
		generators[i] = NewFileTaskIDGenerator(counterFile, "TASK")
	}

	var wg sync.WaitGroup
	wg.Add(numGenerators)

	// Channel to collect generated IDs
	idChan := make(chan string, numGenerators*numIDsPerGenerator)

	// Generate IDs concurrently from multiple generators
	for i := 0; i < numGenerators; i++ {
		go func(gen *FileTaskIDGenerator) {
			defer wg.Done()
			for j := 0; j < numIDsPerGenerator; j++ {
				taskID, err := gen.GenerateTaskID()
				if err != nil {
					t.Errorf("GenerateTaskID() failed: %v", err)
					return
				}
				idChan <- taskID
			}
		}(generators[i])
	}

	wg.Wait()
	close(idChan)

	// Collect all generated IDs
	generatedIDs := make(map[string]bool)
	for id := range idChan {
		if generatedIDs[id] {
			t.Errorf("Duplicate task ID generated: %s", id)
		}
		generatedIDs[id] = true
	}

	// Verify we got the expected number of unique IDs
	expectedCount := numGenerators * numIDsPerGenerator
	if len(generatedIDs) != expectedCount {
		t.Errorf("Expected %d unique IDs, got %d", expectedCount, len(generatedIDs))
	}
}

func TestGenerateTaskID_DirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, "subdir", "nested", ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")
	taskID, err := gen.GenerateTaskID()

	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}
	if taskID != "TASK-00001" {
		t.Errorf("Expected first task ID to be TASK-00001, got %s", taskID)
	}

	// Verify directories were created
	dirPath := filepath.Dir(counterFile)
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Path is not a directory")
	}
}

func TestGenerateTaskID_FilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")
	_, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}

	// Assert the counter file exists and is readable+writable by the
	// current user. Exact Unix mode bits (0o644) aren't portable:
	// Windows synthesises 0o666 regardless of the mode supplied at
	// Open time. Testing for usability rather than bit-exactness keeps
	// the contract portable.
	info, err := os.Stat(counterFile)
	if err != nil {
		t.Fatalf("Failed to stat counter file: %v", err)
	}
	if info.Mode().Perm()&0o600 != 0o600 {
		t.Errorf("counter file %q must be readable+writable by owner; got mode %o", counterFile, info.Mode().Perm())
	}
	// A regular file, not a directory or symlink.
	if !info.Mode().IsRegular() {
		t.Errorf("counter file %q should be a regular file; mode = %v", counterFile, info.Mode())
	}
	// Sanity: we can actually read from and append to it (proves the
	// writability claim isn't just a mode-bit lie on Windows).
	f, err := os.OpenFile(counterFile, os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open for read+append: %v", err)
	}
	_ = f.Close()
}

func TestGenerateTaskID_EmptyCounterFile(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	// Create empty counter file
	if err := os.WriteFile(counterFile, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create empty counter file: %v", err)
	}

	gen := NewFileTaskIDGenerator(counterFile, "TASK")
	taskID, err := gen.GenerateTaskID()

	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}
	if taskID != "TASK-00001" {
		t.Errorf("Expected first task ID to be TASK-00001, got %s", taskID)
	}
}

func TestGenerateTaskID_CounterFileInCurrentDir(t *testing.T) {
	// Save current directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	// Create temp dir and change to it
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir) // Restore original directory

	// Use relative path in current directory
	counterFile := ".task_counter"

	gen := NewFileTaskIDGenerator(counterFile, "TASK")
	taskID, err := gen.GenerateTaskID()

	if err != nil {
		t.Fatalf("GenerateTaskID() failed: %v", err)
	}
	if taskID != "TASK-00001" {
		t.Errorf("Expected first task ID to be TASK-00001, got %s", taskID)
	}

	// Verify file was created in current directory
	if _, err := os.Stat(filepath.Join(tempDir, counterFile)); os.IsNotExist(err) {
		t.Fatal("Counter file was not created in current directory")
	}
}
