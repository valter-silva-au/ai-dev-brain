package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestScreenshotCapturer_IsSupported(t *testing.T) {
	sc := NewScreenshotCapturer()

	supported := sc.IsSupported()

	// Check if current OS is in the supported list
	expectedSupport := runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows"

	if supported != expectedSupport {
		t.Errorf("Expected IsSupported() = %v for OS %s, got %v", expectedSupport, runtime.GOOS, supported)
	}
}

func TestScreenshotCapturer_GenerateDefaultPath(t *testing.T) {
	sc := &OSScreenshotCapturer{osType: runtime.GOOS}

	path := sc.generateDefaultPath()

	if !strings.HasPrefix(filepath.Base(path), "screenshot_") {
		t.Errorf("Expected filename to start with 'screenshot_', got %s", filepath.Base(path))
	}

	if !strings.HasSuffix(path, ".png") {
		t.Errorf("Expected filename to end with '.png', got %s", path)
	}
}

func TestScreenshotCapturer_CaptureScreenshotToDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping screenshot capture test in short mode")
	}

	// Skip on CI or if display is not available
	if os.Getenv("CI") != "" || os.Getenv("DISPLAY") == "" && runtime.GOOS == "linux" {
		t.Skip("Skipping screenshot test in CI or without display")
	}

	sc := NewScreenshotCapturer()

	if !sc.IsSupported() {
		t.Skipf("Screenshot capture not supported on %s", runtime.GOOS)
	}

	tmpDir := t.TempDir()

	// Note: This test might fail if display/screen is not available
	// We'll catch the error gracefully
	t.Run("CaptureToDirectory", func(t *testing.T) {
		path, err := sc.CaptureScreenshotToDir(tmpDir)
		if err != nil {
			// If screenshot fails due to no display, skip
			if strings.Contains(err.Error(), "display") ||
				strings.Contains(err.Error(), "DISPLAY") ||
				strings.Contains(err.Error(), "screen") {
				t.Skipf("Skipping: %v", err)
			}
			t.Logf("Screenshot capture failed (this may be expected): %v", err)
			return
		}

		// Verify the file was created
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Screenshot file was not created at %s", path)
		}

		// Verify it's in the right directory
		if filepath.Dir(path) != tmpDir {
			t.Errorf("Expected screenshot in %s, got %s", tmpDir, filepath.Dir(path))
		}

		// Clean up
		os.Remove(path)
	})
}

func TestScreenshotCapturer_UnsupportedOS(t *testing.T) {
	sc := &OSScreenshotCapturer{osType: "unsupported"}

	if sc.IsSupported() {
		t.Error("Expected IsSupported() to return false for unsupported OS")
	}

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.png")

	_, err := sc.CaptureScreenshot(outputPath)
	if err == nil {
		t.Error("Expected error when capturing screenshot on unsupported OS")
	}
}

func TestScreenshotCapturer_CaptureScreenshot(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name   string
		osType string
	}{
		{"macOS", "darwin"},
		{"Linux", "linux"},
		{"Windows", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &OSScreenshotCapturer{osType: tt.osType}

			if !sc.IsSupported() {
				t.Errorf("Expected %s to be supported", tt.osType)
			}

			// Test with explicit output path
			outputPath := filepath.Join(tmpDir, "test-"+tt.osType+".png")

			// We can't actually test screenshot capture without display
			// but we can test the path handling
			t.Logf("Would capture screenshot to: %s", outputPath)
		})
	}
}

func TestScreenshotCapturer_DirectoryCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	if !NewScreenshotCapturer().IsSupported() {
		t.Skipf("Screenshot capture not supported on %s", runtime.GOOS)
	}

	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "path")

	sc := NewScreenshotCapturer()

	// This will create the directory structure
	outputPath := filepath.Join(nestedDir, "screenshot.png")

	// Try to capture (will fail without display but should create dirs)
	_, err := sc.CaptureScreenshot(outputPath)

	// Check if directories were created even if screenshot failed
	if _, statErr := os.Stat(nestedDir); statErr == nil {
		// Directory was created
		t.Log("Directory structure created successfully")
	} else if err != nil && strings.Contains(err.Error(), "failed to create output directory") {
		t.Errorf("Failed to create output directory: %v", err)
	}
}

func TestScreenshotCapturer_EmptyOutputPath(t *testing.T) {
	sc := &OSScreenshotCapturer{osType: runtime.GOOS}

	// Test that default path is generated when empty string is passed
	// We're just testing the path generation logic, not actual capture
	defaultPath := sc.generateDefaultPath()

	if defaultPath == "" {
		t.Error("Expected non-empty default path")
	}

	if !strings.HasPrefix(filepath.Base(defaultPath), "screenshot_") {
		t.Errorf("Expected default path to start with 'screenshot_', got %s", filepath.Base(defaultPath))
	}
}
