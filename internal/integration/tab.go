package integration

import (
	"fmt"
	"os"
)

// TabEnvironment represents the detected development environment.
type TabEnvironment string

const (
	EnvVSCode   TabEnvironment = "vscode"
	EnvKiro     TabEnvironment = "kiro"
	EnvTerminal TabEnvironment = "terminal"
	EnvUnknown  TabEnvironment = "unknown"
)

// TabManager provides operations for renaming terminal/editor tabs to reflect
// the active task.
type TabManager interface {
	RenameTab(taskID string, branchName string) error
	RestoreTab() error
	DetectEnvironment() TabEnvironment
}

// tabManager implements TabManager using ANSI escape sequences.
type tabManager struct{}

// NewTabManager creates a new TabManager.
func NewTabManager() TabManager {
	return &tabManager{}
}

// RenameTab sets the terminal tab title to "TASK-XXXXX (branch-name)" using
// an ANSI escape sequence (OSC 0). Returns an error if writing fails but
// never panics.
func (m *tabManager) RenameTab(taskID string, branchName string) error {
	if taskID == "" {
		return fmt.Errorf("taskID must not be empty")
	}
	if branchName == "" {
		return fmt.Errorf("branchName must not be empty")
	}

	title := fmt.Sprintf("%s (%s)", taskID, branchName)
	// OSC 0 (Set Window Title): ESC ] 0 ; <title> BEL
	seq := fmt.Sprintf("\033]0;%s\007", title)

	_, err := fmt.Fprint(os.Stdout, seq)
	if err != nil {
		return fmt.Errorf("setting tab title: %w", err)
	}

	return nil
}

// RestoreTab resets the terminal tab title by sending an empty title
// ANSI escape sequence.
func (m *tabManager) RestoreTab() error {
	seq := "\033]0;\007"

	_, err := fmt.Fprint(os.Stdout, seq)
	if err != nil {
		return fmt.Errorf("restoring tab title: %w", err)
	}

	return nil
}

// DetectEnvironment inspects environment variables to determine which
// development environment the CLI is running in.
func (m *tabManager) DetectEnvironment() TabEnvironment {
	if os.Getenv("KIRO_PID") != "" {
		return EnvKiro
	}
	if os.Getenv("VSCODE_PID") != "" {
		return EnvVSCode
	}
	if os.Getenv("TERM_PROGRAM") != "" {
		return EnvTerminal
	}
	return EnvUnknown
}
