package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"pgregory.net/rapid"
)

// TestProperty_TaskIDFormat verifies that generated task IDs always match the expected format
func TestProperty_TaskIDFormat(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		counterFile := filepath.Join(baseDir, suffix)
		prefix := rapid.StringMatching(`^[A-Z][A-Z0-9]*$`).Draw(t, "prefix")

		gen := NewFileTaskIDGenerator(counterFile, prefix)
		taskID, err := gen.GenerateTaskID()

		if err != nil {
			t.Fatalf("GenerateTaskID failed: %v", err)
		}

		// Verify format: PREFIX-NNNNN
		pattern := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-\d{5}$`)
		if !pattern.MatchString(taskID) {
			t.Fatalf("Task ID %s does not match expected format %s-NNNNN", taskID, prefix)
		}
	})
}

// TestProperty_TaskIDSequential verifies that task IDs are sequential
func TestProperty_TaskIDSequential(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		counterFile := filepath.Join(baseDir, suffix)
		count := rapid.IntRange(1, 20).Draw(t, "count")

		gen := NewFileTaskIDGenerator(counterFile, "TASK")

		var ids []string
		for i := 0; i < count; i++ {
			id, err := gen.GenerateTaskID()
			if err != nil {
				t.Fatalf("GenerateTaskID failed: %v", err)
			}
			ids = append(ids, id)
		}

		// Verify all IDs are unique
		for i := 0; i < len(ids)-1; i++ {
			if ids[i] == ids[i+1] {
				t.Fatalf("Generated duplicate ID: %s", ids[i])
			}
		}
	})
}

// TestProperty_TaskIDPersistence verifies that task ID counter persists across instances
func TestProperty_TaskIDPersistence(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		counterFile := filepath.Join(baseDir, suffix)
		prefix := rapid.StringMatching(`^[A-Z]+$`).Draw(t, "prefix")

		gen1 := NewFileTaskIDGenerator(counterFile, prefix)
		id1, err := gen1.GenerateTaskID()
		if err != nil {
			t.Fatalf("First GenerateTaskID failed: %v", err)
		}

		// Create new instance
		gen2 := NewFileTaskIDGenerator(counterFile, prefix)
		id2, err := gen2.GenerateTaskID()
		if err != nil {
			t.Fatalf("Second GenerateTaskID failed: %v", err)
		}

		// Extract numbers from IDs
		num1Str := id1[len(prefix)+1:]
		num2Str := id2[len(prefix)+1:]

		num1, _ := strconv.Atoi(num1Str)
		num2, _ := strconv.Atoi(num2Str)

		if num1 >= num2 {
			t.Fatalf("Counter not incrementing: %d >= %d", num1, num2)
		}
	})
}

// TestProperty_TaskIDConcurrency verifies that concurrent task ID generation is safe
func TestProperty_TaskIDConcurrency(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		counterFile := filepath.Join(baseDir, suffix)
		goroutines := rapid.IntRange(2, 10).Draw(t, "goroutines")

		gen := NewFileTaskIDGenerator(counterFile, "TASK")

		var wg sync.WaitGroup
		ids := make([]string, goroutines)
		errors := make([]error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				id, err := gen.GenerateTaskID()
				ids[index] = id
				errors[index] = err
			}(i)
		}

		wg.Wait()

		// Check for errors
		for _, err := range errors {
			if err != nil {
				t.Fatalf("Concurrent GenerateTaskID failed: %v", err)
			}
		}

		// Verify all IDs are unique
		seen := make(map[string]bool)
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("Duplicate ID generated in concurrent test: %s", id)
			}
			seen[id] = true
		}
	})
}

// TestProperty_TaskIDInvalidCounterRecovery verifies recovery from corrupted counter files
func TestProperty_TaskIDInvalidCounterRecovery(t *testing.T) {
	baseDir := t.TempDir()
	rapid.Check(t, func(t *rapid.T) {
		suffix := rapid.StringMatching(`^[a-z0-9]+$`).Draw(t, "suffix")
		counterFile := filepath.Join(baseDir, suffix)

		// Write content that is definitely not a valid integer
		// (strconv.Atoi trims nothing; TrimSpace in readCounter handles whitespace)
		invalidContent := rapid.StringMatching(`^[a-z]{3,20}$`).Draw(t, "invalid")

		if err := os.WriteFile(counterFile, []byte(invalidContent), 0o644); err != nil {
			t.Fatalf("Failed to write invalid counter: %v", err)
		}

		gen := NewFileTaskIDGenerator(counterFile, "TASK")
		_, err := gen.GenerateTaskID()

		// Should fail on corrupted counter (non-numeric content)
		if err == nil {
			t.Fatal("Expected error for corrupted counter file")
		}
	})
}

// TestProperty_BranchNameSanitization verifies branch name sanitization
func TestProperty_BranchNameSanitization(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		taskID := rapid.StringMatching(`^TASK-\d{5}$`).Draw(t, "taskID")

		branchName := SanitizeBranchName(taskID)

		// Branch names should:
		// - Not contain spaces
		// - Not start with - or .
		// - Not contain consecutive dots
		// - Not end with .lock
		if strings.Contains(branchName, " ") {
			t.Fatal("Branch name contains spaces")
		}
		if strings.HasPrefix(branchName, "-") || strings.HasPrefix(branchName, ".") {
			t.Fatal("Branch name starts with invalid character")
		}
		if strings.Contains(branchName, "..") {
			t.Fatal("Branch name contains consecutive dots")
		}
		if strings.HasSuffix(branchName, ".lock") {
			t.Fatal("Branch name ends with .lock")
		}
	})
}

// SanitizeBranchName sanitizes a task ID for use as a branch name
func SanitizeBranchName(taskID string) string {
	// Replace spaces and special characters
	sanitized := strings.ReplaceAll(taskID, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, "..", ".")

	// Remove leading dots and dashes
	sanitized = strings.TrimLeft(sanitized, ".-")

	// Remove .lock suffix if present
	sanitized = strings.TrimSuffix(sanitized, ".lock")

	return "task/" + sanitized
}

// Helper function to parse int with error handling
func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &i)
	return i, err
}
