package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewFileContextManager(t *testing.T) {
	tempDir := t.TempDir()

	fcm := NewFileContextManager(tempDir)
	if fcm == nil {
		t.Fatal("NewFileContextManager returned nil")
	}
	if fcm.baseDir != tempDir {
		t.Errorf("Expected baseDir %s, got %s", tempDir, fcm.baseDir)
	}
}

func TestFileContextManager_ReadContext_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() should not error on non-existent file: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string for non-existent file, got: %s", content)
	}
}

func TestFileContextManager_WriteContext(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	testContent := "# Task Context\n\nThis is the task context."
	err := fcm.WriteContext("TASK-001", testContent)
	if err != nil {
		t.Fatalf("WriteContext() failed: %v", err)
	}

	// Verify file was created
	contextPath := filepath.Join(tempDir, "TASK-001", "context.md")
	if _, err := os.Stat(contextPath); os.IsNotExist(err) {
		t.Fatal("Context file was not created")
	}

	// Verify context file is present + owner-read/writable.
	assertOwnerReadWritableFile(t, contextPath)

	// Verify content
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if content != testContent {
		t.Errorf("Expected content %q, got %q", testContent, content)
	}
}

func TestFileContextManager_WriteContext_Overwrite(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Write initial content
	initialContent := "Initial content"
	err := fcm.WriteContext("TASK-001", initialContent)
	if err != nil {
		t.Fatalf("WriteContext() failed: %v", err)
	}

	// Overwrite with new content
	newContent := "New content"
	err = fcm.WriteContext("TASK-001", newContent)
	if err != nil {
		t.Fatalf("WriteContext() failed on overwrite: %v", err)
	}

	// Verify new content
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if content != newContent {
		t.Errorf("Expected content %q, got %q", newContent, content)
	}
}

func TestFileContextManager_AppendContext(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Append to non-existent file (should create it)
	section1 := "# Section 1\n\nFirst section.\n"
	err := fcm.AppendContext("TASK-001", section1)
	if err != nil {
		t.Fatalf("AppendContext() failed: %v", err)
	}

	// Verify first section
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if content != section1 {
		t.Errorf("Expected content %q, got %q", section1, content)
	}

	// Append second section
	section2 := "\n# Section 2\n\nSecond section.\n"
	err = fcm.AppendContext("TASK-001", section2)
	if err != nil {
		t.Fatalf("AppendContext() failed on second append: %v", err)
	}

	// Verify combined content
	content, err = fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	expectedContent := section1 + section2
	if content != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, content)
	}
}

func TestFileContextManager_AppendContext_MultipleAppends(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Append multiple sections
	sections := []string{
		"Section 1\n",
		"Section 2\n",
		"Section 3\n",
		"Section 4\n",
	}

	for _, section := range sections {
		err := fcm.AppendContext("TASK-002", section)
		if err != nil {
			t.Fatalf("AppendContext() failed: %v", err)
		}
	}

	// Verify all sections are present
	content, err := fcm.ReadContext("TASK-002")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}

	expectedContent := strings.Join(sections, "")
	if content != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, content)
	}
}

func TestFileContextManager_ReadNotes_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	content, err := fcm.ReadNotes("TASK-001")
	if err != nil {
		t.Fatalf("ReadNotes() should not error on non-existent file: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string for non-existent file, got: %s", content)
	}
}

func TestFileContextManager_WriteNotes(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	testContent := "# Notes\n\n- Note 1\n- Note 2"
	err := fcm.WriteNotes("TASK-001", testContent)
	if err != nil {
		t.Fatalf("WriteNotes() failed: %v", err)
	}

	// Verify file was created
	notesPath := filepath.Join(tempDir, "TASK-001", "notes.md")
	if _, err := os.Stat(notesPath); os.IsNotExist(err) {
		t.Fatal("Notes file was not created")
	}

	// Verify notes file is present + owner-read/writable.
	assertOwnerReadWritableFile(t, notesPath)

	// Verify content
	content, err := fcm.ReadNotes("TASK-001")
	if err != nil {
		t.Fatalf("ReadNotes() failed: %v", err)
	}
	if content != testContent {
		t.Errorf("Expected content %q, got %q", testContent, content)
	}
}

func TestFileContextManager_WriteNotes_Overwrite(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Write initial notes
	initialContent := "Initial notes"
	err := fcm.WriteNotes("TASK-001", initialContent)
	if err != nil {
		t.Fatalf("WriteNotes() failed: %v", err)
	}

	// Overwrite with new notes
	newContent := "New notes"
	err = fcm.WriteNotes("TASK-001", newContent)
	if err != nil {
		t.Fatalf("WriteNotes() failed on overwrite: %v", err)
	}

	// Verify new content
	content, err := fcm.ReadNotes("TASK-001")
	if err != nil {
		t.Fatalf("ReadNotes() failed: %v", err)
	}
	if content != newContent {
		t.Errorf("Expected content %q, got %q", newContent, content)
	}
}

func TestFileContextManager_ContextAndNotes_Independent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Write context and notes
	contextContent := "Context content"
	notesContent := "Notes content"

	err := fcm.WriteContext("TASK-001", contextContent)
	if err != nil {
		t.Fatalf("WriteContext() failed: %v", err)
	}

	err = fcm.WriteNotes("TASK-001", notesContent)
	if err != nil {
		t.Fatalf("WriteNotes() failed: %v", err)
	}

	// Verify both files are independent
	readContext, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if readContext != contextContent {
		t.Errorf("Context content mismatch: expected %q, got %q", contextContent, readContext)
	}

	readNotes, err := fcm.ReadNotes("TASK-001")
	if err != nil {
		t.Fatalf("ReadNotes() failed: %v", err)
	}
	if readNotes != notesContent {
		t.Errorf("Notes content mismatch: expected %q, got %q", notesContent, readNotes)
	}
}

func TestFileContextManager_MultipleTasksIndependent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Create context for multiple tasks
	tasks := map[string]string{
		"TASK-001": "Context for task 1",
		"TASK-002": "Context for task 2",
		"TASK-003": "Context for task 3",
	}

	for taskID, content := range tasks {
		err := fcm.WriteContext(taskID, content)
		if err != nil {
			t.Fatalf("WriteContext() failed for %s: %v", taskID, err)
		}
	}

	// Verify each task has its own independent context
	for taskID, expectedContent := range tasks {
		content, err := fcm.ReadContext(taskID)
		if err != nil {
			t.Fatalf("ReadContext() failed for %s: %v", taskID, err)
		}
		if content != expectedContent {
			t.Errorf("Task %s: expected content %q, got %q", taskID, expectedContent, content)
		}
	}

	// Verify task directories exist
	for taskID := range tasks {
		taskDir := filepath.Join(tempDir, taskID)
		if _, err := os.Stat(taskDir); os.IsNotExist(err) {
			t.Errorf("Task directory for %s was not created", taskID)
		}
	}
}

func TestFileContextManager_TaskDirectoryCreation(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Write context should create task directory
	err := fcm.WriteContext("TASK-001", "test content")
	if err != nil {
		t.Fatalf("WriteContext() failed: %v", err)
	}

	// Verify directory was created with correct permissions
	taskDir := filepath.Join(tempDir, "TASK-001")
	// Verify task directory is present + owner-accessible.
	assertOwnerAccessibleDir(t, taskDir)
}

func TestFileContextManager_EmptyContent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Write empty context
	err := fcm.WriteContext("TASK-001", "")
	if err != nil {
		t.Fatalf("WriteContext() failed with empty content: %v", err)
	}

	// Read empty context
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty content, got %q", content)
	}

	// Verify file exists even with empty content
	contextPath := filepath.Join(tempDir, "TASK-001", "context.md")
	if _, err := os.Stat(contextPath); os.IsNotExist(err) {
		t.Error("Empty context file was not created")
	}
}

func TestFileContextManager_LargeContent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Create large content (10KB)
	largeContent := strings.Repeat("This is a line of text.\n", 400)

	err := fcm.WriteContext("TASK-001", largeContent)
	if err != nil {
		t.Fatalf("WriteContext() failed with large content: %v", err)
	}

	// Verify content
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if content != largeContent {
		t.Error("Large content not preserved correctly")
	}
}

func TestFileContextManager_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent writes to different tasks
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			taskID := fmt.Sprintf("TASK-%03d", index)
			content := fmt.Sprintf("Content for task %d", index)

			if err := fcm.WriteContext(taskID, content); err != nil {
				t.Errorf("WriteContext() failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all tasks were written
	for i := 0; i < numGoroutines; i++ {
		taskID := fmt.Sprintf("TASK-%03d", i)
		content, err := fcm.ReadContext(taskID)
		if err != nil {
			t.Errorf("ReadContext() failed for %s: %v", taskID, err)
		}
		expectedContent := fmt.Sprintf("Content for task %d", i)
		if content != expectedContent {
			t.Errorf("Task %s: expected %q, got %q", taskID, expectedContent, content)
		}
	}
}

func TestFileContextManager_ConcurrentAppends(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent appends to same task
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			section := fmt.Sprintf("Section %d\n", index)
			if err := fcm.AppendContext("TASK-001", section); err != nil {
				t.Errorf("AppendContext() failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all sections were appended (order may vary due to concurrency)
	content, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}

	// Count occurrences of "Section"
	count := strings.Count(content, "Section")
	if count != numGoroutines {
		t.Errorf("Expected %d sections, found %d", numGoroutines, count)
	}
}

func TestFileContextManager_ConcurrentReadsAndWrites(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Initialize with content
	initialContent := "Initial content"
	err := fcm.WriteContext("TASK-001", initialContent)
	if err != nil {
		t.Fatalf("WriteContext() failed: %v", err)
	}

	numGoroutines := 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Mix of concurrent reads and writes
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()

			if index%2 == 0 {
				// Even: read
				_, err := fcm.ReadContext("TASK-001")
				if err != nil {
					t.Errorf("ReadContext() failed: %v", err)
				}
			} else {
				// Odd: write
				content := fmt.Sprintf("Updated content %d", index)
				err := fcm.WriteContext("TASK-001", content)
				if err != nil {
					t.Errorf("WriteContext() failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Just verify we can still read (content will be from last write)
	_, err = fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("Final ReadContext() failed: %v", err)
	}
}

func TestFileContextManager_SpecialCharactersInTaskID(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Test with task ID containing allowed special characters
	taskID := "TASK-001_v2"
	content := "Test content"

	err := fcm.WriteContext(taskID, content)
	if err != nil {
		t.Fatalf("WriteContext() failed with special characters in task ID: %v", err)
	}

	readContent, err := fcm.ReadContext(taskID)
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if readContent != content {
		t.Errorf("Expected content %q, got %q", content, readContent)
	}
}

func TestFileContextManager_UnicodeContent(t *testing.T) {
	tempDir := t.TempDir()
	fcm := NewFileContextManager(tempDir)

	// Test with Unicode content
	unicodeContent := "# Task Context\n\n日本語\nΕλληνικά\nРусский\n🚀 Emoji support"

	err := fcm.WriteContext("TASK-001", unicodeContent)
	if err != nil {
		t.Fatalf("WriteContext() failed with Unicode content: %v", err)
	}

	readContent, err := fcm.ReadContext("TASK-001")
	if err != nil {
		t.Fatalf("ReadContext() failed: %v", err)
	}
	if readContent != unicodeContent {
		t.Errorf("Unicode content not preserved correctly")
	}
}
