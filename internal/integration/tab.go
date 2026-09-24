package integration

import (
	"fmt"
	"io"
	"os"
)

// TabManager manages terminal tab names via ANSI escape sequences
type TabManager interface {
	// SetTabName sets the terminal tab name using ANSI OSC 0 sequence
	SetTabName(name string) error

	// SetTabNameWithWriter sets the terminal tab name and writes to a custom writer
	SetTabNameWithWriter(name string, writer io.Writer) error
}

// DefaultTabManager implements TabManager
type DefaultTabManager struct {
	writer io.Writer
}

// NewTabManager creates a new TabManager
// If writer is nil, os.Stdout is used
func NewTabManager(writer io.Writer) TabManager {
	if writer == nil {
		writer = os.Stdout
	}
	return &DefaultTabManager{
		writer: writer,
	}
}

// SetTabName sets the terminal tab name using ANSI OSC 0 sequence
// OSC 0 ; text BEL - Sets both icon name and window title
// Format: \033]0;text\007
func (m *DefaultTabManager) SetTabName(name string) error {
	return m.SetTabNameWithWriter(name, m.writer)
}

// SetTabNameWithWriter sets the terminal tab name and writes to a custom writer
func (m *DefaultTabManager) SetTabNameWithWriter(name string, writer io.Writer) error {
	if name == "" {
		return fmt.Errorf("tab name cannot be empty")
	}
	if writer == nil {
		writer = m.writer
	}

	// ANSI OSC 0 sequence: \033]0;text\007
	// \033 = ESC
	// ]0; = OSC 0 (set icon name and window title)
	// \007 = BEL (bell character, terminates OSC sequence)
	sequence := fmt.Sprintf("\033]0;%s\007", name)

	_, err := writer.Write([]byte(sequence))
	return err
}
