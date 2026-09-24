package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ScreenshotCapturer captures screenshots in an OS-specific manner
type ScreenshotCapturer interface {
	// CaptureScreenshot captures a screenshot and saves it to the specified path
	// If path is empty, generates a default path in the current directory
	CaptureScreenshot(outputPath string) (string, error)

	// CaptureScreenshotToDir captures a screenshot to a directory with auto-generated filename
	CaptureScreenshotToDir(outputDir string) (string, error)

	// IsSupported returns true if screenshot capture is supported on the current OS
	IsSupported() bool
}

// OSScreenshotCapturer implements ScreenshotCapturer with OS-specific commands
type OSScreenshotCapturer struct {
	osType string // darwin, linux, windows
}

// NewScreenshotCapturer creates a new screenshot capturer for the current OS
func NewScreenshotCapturer() ScreenshotCapturer {
	return &OSScreenshotCapturer{
		osType: runtime.GOOS,
	}
}

// IsSupported returns true if screenshot capture is supported on the current OS
func (sc *OSScreenshotCapturer) IsSupported() bool {
	switch sc.osType {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

// CaptureScreenshot captures a screenshot and saves it to the specified path
func (sc *OSScreenshotCapturer) CaptureScreenshot(outputPath string) (string, error) {
	if !sc.IsSupported() {
		return "", fmt.Errorf("screenshot capture not supported on %s", sc.osType)
	}

	// If no output path specified, generate one
	if outputPath == "" {
		outputPath = sc.generateDefaultPath()
	}

	// Ensure the directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Execute OS-specific screenshot command
	var cmd *exec.Cmd
	switch sc.osType {
	case "darwin":
		// macOS: screencapture -x (no sound) -i (interactive)
		// For non-interactive full screen capture, use:
		cmd = exec.Command("screencapture", "-x", outputPath)

	case "linux":
		// Linux: use import from ImageMagick
		// Check if import is available, fallback to scrot or gnome-screenshot
		if _, err := exec.LookPath("import"); err == nil {
			cmd = exec.Command("import", "-window", "root", outputPath)
		} else if _, err := exec.LookPath("scrot"); err == nil {
			cmd = exec.Command("scrot", outputPath)
		} else if _, err := exec.LookPath("gnome-screenshot"); err == nil {
			cmd = exec.Command("gnome-screenshot", "-f", outputPath)
		} else {
			return "", fmt.Errorf("no screenshot tool found (install imagemagick, scrot, or gnome-screenshot)")
		}

	case "windows":
		// Windows: use snippingtool or powershell
		// PowerShell approach using Windows Forms
		psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bitmap = New-Object System.Drawing.Bitmap $screen.Width, $screen.Height
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.CopyFromScreen($screen.Location, [System.Drawing.Point]::Empty, $screen.Size)
$bitmap.Save('%s')
$graphics.Dispose()
$bitmap.Dispose()
`, outputPath)
		cmd = exec.Command("powershell", "-Command", psScript)

	default:
		return "", fmt.Errorf("unsupported OS: %s", sc.osType)
	}

	// Execute the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("screenshot command failed: %w: %s", err, string(output))
	}

	// Verify the file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot file was not created")
	}

	return outputPath, nil
}

// CaptureScreenshotToDir captures a screenshot to a directory with auto-generated filename
func (sc *OSScreenshotCapturer) CaptureScreenshotToDir(outputDir string) (string, error) {
	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("screenshot_%s.png", timestamp)
	outputPath := filepath.Join(outputDir, filename)

	return sc.CaptureScreenshot(outputPath)
}

// generateDefaultPath generates a default screenshot path
func (sc *OSScreenshotCapturer) generateDefaultPath() string {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("screenshot_%s.png", timestamp)
	return filepath.Join(".", filename)
}

// CaptureInteractive captures a screenshot with user interaction (select region)
// Only supported on macOS with screencapture -i
func (sc *OSScreenshotCapturer) CaptureInteractive(outputPath string) (string, error) {
	if sc.osType != "darwin" {
		return "", fmt.Errorf("interactive screenshot only supported on macOS")
	}

	if outputPath == "" {
		outputPath = sc.generateDefaultPath()
	}

	// Ensure the directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// screencapture with -i flag for interactive selection
	cmd := exec.Command("screencapture", "-i", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("screenshot command failed: %w: %s", err, string(output))
	}

	// Verify the file was created (user might have cancelled)
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("screenshot was cancelled or file was not created")
	}

	return outputPath, nil
}
