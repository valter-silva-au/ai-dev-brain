package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewScreenshotPipeline_ReturnsNonNil(t *testing.T) {
	p := NewScreenshotPipeline("/tmp/test")
	if p == nil {
		t.Fatal("expected non-nil ScreenshotPipeline")
	}
}

func TestProcessScreenshot_NilScreenshot_ReturnsError(t *testing.T) {
	p := NewScreenshotPipeline(t.TempDir())
	_, err := p.ProcessScreenshot(nil)
	if err == nil {
		t.Fatal("expected error for nil screenshot")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("error = %q, want to contain 'must not be nil'", err.Error())
	}
}

func TestProcessScreenshot_OCRNotConfigured(t *testing.T) {
	p := NewScreenshotPipeline(t.TempDir())
	_, err := p.ProcessScreenshot(&Screenshot{
		Data:   []byte("fake image data"),
		Source: "test",
	})
	if err == nil {
		t.Fatal("expected error for OCR not configured")
	}
	if !strings.Contains(err.Error(), "OCR not yet configured") {
		t.Errorf("error = %q, want to contain 'OCR not yet configured'", err.Error())
	}
}

func TestFileContent_NilContent_ReturnsError(t *testing.T) {
	p := NewScreenshotPipeline(t.TempDir())
	_, err := p.FileContent(nil, "TASK-00001")
	if err == nil {
		t.Fatal("expected error for nil content")
	}
	if !strings.Contains(err.Error(), "content must not be nil") {
		t.Errorf("error = %q, want to contain 'content must not be nil'", err.Error())
	}
}

func TestFileContent_EmptyTaskID_ReturnsError(t *testing.T) {
	p := NewScreenshotPipeline(t.TempDir())
	_, err := p.FileContent(&ProcessedContent{
		ExtractedText: "text",
		Category:      CategoryOther,
	}, "")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
	if !strings.Contains(err.Error(), "taskID must not be empty") {
		t.Errorf("error = %q, want to contain 'taskID must not be empty'", err.Error())
	}
}

func TestFileContent_WritesFile(t *testing.T) {
	basePath := t.TempDir()
	p := NewScreenshotPipeline(basePath)

	content := &ProcessedContent{
		ExtractedText: "Some extracted text",
		Category:      CategoryCommunication,
		Summary:       "A summary",
	}

	filePath, err := p.FileContent(content, "TASK-00001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the file was created.
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Fatal("expected file to exist")
	}

	// Verify the path is under the correct category subdirectory.
	if !strings.Contains(filePath, "communications") {
		t.Errorf("file path = %q, expected to contain 'communications'", filePath)
	}

	// Verify the content.
	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		t.Fatalf("failed to read file: %v", readErr)
	}
	body := string(data)
	if !strings.Contains(body, "Screenshot Content") {
		t.Error("file missing 'Screenshot Content' heading")
	}
	if !strings.Contains(body, "communication") {
		t.Error("file missing category")
	}
	if !strings.Contains(body, "A summary") {
		t.Error("file missing summary")
	}
	if !strings.Contains(body, "Some extracted text") {
		t.Error("file missing extracted text")
	}
}

func TestFileContent_AllCategories(t *testing.T) {
	tests := []struct {
		category ContentCategory
		subDir   string
	}{
		{CategoryCommunication, "communications"},
		{CategoryWiki, "wiki"},
		{CategoryRequirement, "requirements"},
		{CategoryCode, "code"},
		{CategoryOther, "other"},
		{"unknown_category", "other"},
	}

	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			basePath := t.TempDir()
			p := NewScreenshotPipeline(basePath)

			content := &ProcessedContent{
				ExtractedText: "text",
				Category:      tc.category,
				Summary:       "summary",
			}

			filePath, err := p.FileContent(content, "TASK-00001")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(filePath, tc.subDir) {
				t.Errorf("file path = %q, expected to contain %q", filePath, tc.subDir)
			}
		})
	}
}

func TestFileContent_MkdirAllFails_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	basePath := t.TempDir()
	// Create the parent directory as read-only to prevent MkdirAll.
	readOnlyDir := filepath.Join(basePath, "tasks")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(readOnlyDir, 0o755) }()

	p := NewScreenshotPipeline(basePath)
	content := &ProcessedContent{
		ExtractedText: "text",
		Category:      CategoryOther,
		Summary:       "summary",
	}

	_, err := p.FileContent(content, "TASK-00001")
	if err == nil {
		t.Fatal("expected error for read-only directory")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("error = %q, want to contain 'creating directory'", err.Error())
	}
}

func TestFileContent_WriteFileFails_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	basePath := t.TempDir()
	// Create the target directory but make it read-only after creation
	// so MkdirAll succeeds but WriteFile fails.
	targetDir := filepath.Join(basePath, "tasks", "TASK-00001", "other")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(targetDir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(targetDir, 0o755) }()

	p := NewScreenshotPipeline(basePath)
	content := &ProcessedContent{
		ExtractedText: "text",
		Category:      CategoryOther,
		Summary:       "summary",
	}

	_, err := p.FileContent(content, "TASK-00001")
	if err == nil {
		t.Fatal("expected error when writing file fails")
	}
	if !strings.Contains(err.Error(), "writing content file") {
		t.Errorf("error = %q, want to contain 'writing content file'", err.Error())
	}
}

func TestCategorySubDir_AllCategories(t *testing.T) {
	tests := []struct {
		category ContentCategory
		want     string
	}{
		{CategoryCommunication, "communications"},
		{CategoryWiki, "wiki"},
		{CategoryRequirement, "requirements"},
		{CategoryCode, "code"},
		{CategoryOther, "other"},
		{"nonexistent", "other"},
	}

	for _, tc := range tests {
		t.Run(string(tc.category), func(t *testing.T) {
			got := categorySubDir(tc.category)
			if got != tc.want {
				t.Errorf("categorySubDir(%q) = %q, want %q", tc.category, got, tc.want)
			}
		})
	}
}
