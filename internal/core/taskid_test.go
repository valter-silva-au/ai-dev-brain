package core

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestGenerateTaskID_Format(t *testing.T) {
	tempDir := t.TempDir()
	counterFile := filepath.Join(tempDir, ".task_counter")

	gen := NewFileTaskIDGenerator(counterFile, "TASK")

	// Test that counter is zero-padded to 5 digits
	testCases := []struct {
		count    int
		expected string
	}{
		{1, "TASK-00001"},
		{10, "TASK-00010"},
		{100, "TASK-00100"},
		{1000, "TASK-01000"},
		{10000, "TASK-10000"},
		{99999, "TASK-99999"},
		{100000, "TASK-100000"}, // overflow beyond 5 digits
	}

	for i, tc := range testCases {
		// Generate IDs until we reach the expected count
		numToGenerate := tc.count
		if i > 0 {
			numToGenerate = tc.count - testCases[i-1].count
		}

		var taskID string
		var err error
		for j := 0; j < numToGenerate; j++ {
			taskID, err = gen.GenerateTaskID()
			if err != nil {
				t.Fatalf("GenerateTaskID() failed: %v", err)
			}
		}

		if taskID != tc.expected {
			t.Errorf("Expected task ID %s for count %d, got %s", tc.expected, tc.count, taskID)
		}
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
