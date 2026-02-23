package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// mockAIContextGenerator implements core.AIContextGenerator for testing.
type mockAIContextGenerator struct {
	syncContextFn func() error
}

func (m *mockAIContextGenerator) GenerateContextFile(aiType core.AIType) (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) RegenerateSection(section core.ContextSection) error {
	return nil
}

func (m *mockAIContextGenerator) SyncContext() error {
	if m.syncContextFn != nil {
		return m.syncContextFn()
	}
	return nil
}

func (m *mockAIContextGenerator) AssembleProjectOverview() (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) AssembleDirectoryStructure() (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) AssembleConventions() (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) AssembleGlossary() (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) AssembleActiveTaskSummaries() (string, error) {
	return "", nil
}

func (m *mockAIContextGenerator) AssembleDecisionsSummary() (string, error) {
	return "", nil
}

func TestSyncContextCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "sync-context" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'sync-context' command to be registered")
	}
}

func TestSyncContextCommand_NilGenerator(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()
	AICtxGen = nil

	var buf bytes.Buffer
	syncContextCmd.SetOut(&buf)
	syncContextCmd.SetErr(&buf)

	err := syncContextCmd.RunE(syncContextCmd, []string{})
	if err == nil {
		t.Fatal("expected error when AICtxGen is nil")
	}
	if !strings.Contains(err.Error(), "AI context generator not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSyncContextCommand_Success(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()

	syncCalled := false
	AICtxGen = &mockAIContextGenerator{
		syncContextFn: func() error {
			syncCalled = true
			return nil
		},
	}

	err := syncContextCmd.RunE(syncContextCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !syncCalled {
		t.Error("SyncContext was not called")
	}
}

func TestSyncContextCommand_SyncError(t *testing.T) {
	origGen := AICtxGen
	defer func() { AICtxGen = origGen }()

	AICtxGen = &mockAIContextGenerator{
		syncContextFn: func() error {
			return fmt.Errorf("failed to read docs/")
		},
	}

	err := syncContextCmd.RunE(syncContextCmd, []string{})
	if err == nil {
		t.Fatal("expected error from SyncContext")
	}
	if !strings.Contains(err.Error(), "failed to read docs/") {
		t.Errorf("unexpected error: %v", err)
	}
}
