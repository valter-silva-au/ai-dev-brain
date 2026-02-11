package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Screenshot holds raw captured screenshot data.
type Screenshot struct {
	Data      []byte
	Timestamp time.Time
	Source    string
}

// ContentCategory classifies the content extracted from a screenshot.
type ContentCategory string

const (
	CategoryCommunication ContentCategory = "communication"
	CategoryWiki          ContentCategory = "wiki"
	CategoryRequirement   ContentCategory = "requirement"
	CategoryCode          ContentCategory = "code"
	CategoryOther         ContentCategory = "other"
)

// ProcessedContent represents the result of OCR and classification on a
// screenshot.
type ProcessedContent struct {
	ExtractedText  string
	Category       ContentCategory
	Summary        string
	RelevantToTask bool
	SuggestedPath  string
}

// ScreenshotPipeline captures screenshots, processes them through OCR, and
// files the extracted content under the appropriate task directory.
type ScreenshotPipeline interface {
	Capture() (*Screenshot, error)
	ProcessScreenshot(screenshot *Screenshot) (*ProcessedContent, error)
	FileContent(content *ProcessedContent, taskID string) (string, error)
}

// screenshotPipeline implements ScreenshotPipeline.
type screenshotPipeline struct {
	basePath string
}

// NewScreenshotPipeline creates a new ScreenshotPipeline that stores files
// under the given basePath.
func NewScreenshotPipeline(basePath string) ScreenshotPipeline {
	return &screenshotPipeline{basePath: basePath}
}

// Capture takes a screenshot using the OS-specific capture tool.
// On macOS it uses screencapture, on Linux it uses import (ImageMagick),
// and on Windows it uses the Snipping Tool.
func (p *screenshotPipeline) Capture() (*Screenshot, error) {
	tmpFile, err := os.CreateTemp("", "adb-screenshot-*.png")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("screencapture", "-i", tmpPath)
	case "linux":
		cmd = exec.Command("import", tmpPath)
	case "windows":
		cmd = exec.Command("snippingtool", "/clip")
	default:
		return nil, fmt.Errorf("unsupported OS for screenshot capture: %s", runtime.GOOS)
	}

	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screenshot capture failed: %s: %w", string(output), err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading screenshot file: %w", err)
	}

	return &Screenshot{
		Data:      data,
		Timestamp: time.Now(),
		Source:    runtime.GOOS,
	}, nil
}

// ProcessScreenshot is a placeholder that returns an error indicating OCR
// is not yet configured. When an OCR provider is integrated, this method
// will extract text and classify the content.
func (p *screenshotPipeline) ProcessScreenshot(screenshot *Screenshot) (*ProcessedContent, error) {
	if screenshot == nil {
		return nil, fmt.Errorf("screenshot must not be nil")
	}
	return nil, fmt.Errorf("OCR not yet configured")
}

// FileContent writes processed content to the appropriate directory based on
// the content category. Returns the path of the written file.
func (p *screenshotPipeline) FileContent(content *ProcessedContent, taskID string) (string, error) {
	if content == nil {
		return "", fmt.Errorf("content must not be nil")
	}
	if taskID == "" {
		return "", fmt.Errorf("taskID must not be empty")
	}

	subDir := categorySubDir(content.Category)
	dir := filepath.Join(p.basePath, "tasks", taskID, subDir)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", dir, err)
	}

	filename := fmt.Sprintf("screenshot-%d.md", time.Now().UnixMilli())
	filePath := filepath.Join(dir, filename)

	body := fmt.Sprintf("# Screenshot Content\n\n**Category:** %s\n**Summary:** %s\n\n%s\n",
		content.Category, content.Summary, content.ExtractedText)

	if err := os.WriteFile(filePath, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("writing content file: %w", err)
	}

	return filePath, nil
}

// categorySubDir maps a ContentCategory to a subdirectory name.
func categorySubDir(cat ContentCategory) string {
	switch cat {
	case CategoryCommunication:
		return "communications"
	case CategoryWiki:
		return "wiki"
	case CategoryRequirement:
		return "requirements"
	case CategoryCode:
		return "code"
	case CategoryOther:
		return "other"
	default:
		return "other"
	}
}
