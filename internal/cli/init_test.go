package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/core"
)

// mockProjectInitializer implements core.ProjectInitializer for testing.
type mockProjectInitializer struct {
	initFn     func(config core.InitConfig) (*core.InitResult, error)
	lastConfig core.InitConfig
}

func (m *mockProjectInitializer) Init(config core.InitConfig) (*core.InitResult, error) {
	m.lastConfig = config
	if m.initFn != nil {
		return m.initFn(config)
	}
	return &core.InitResult{
		Created: []string{config.BasePath + "/tickets"},
	}, nil
}

func TestInitCommand_Registration(t *testing.T) {
	subcommands := rootCmd.Commands()
	found := false
	for _, cmd := range subcommands {
		if cmd.Name() == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'init' command to be registered")
	}
}

func TestInitCommand_NilProjectInitializer(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()
	ProjectInit = nil

	var buf bytes.Buffer
	initCmd.SetOut(&buf)
	initCmd.SetErr(&buf)

	err := initCmd.RunE(initCmd, []string{})
	if err == nil {
		t.Fatal("expected error when ProjectInit is nil")
	}
	if !strings.Contains(err.Error(), "project initializer not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInitCommand_Success(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()

	mock := &mockProjectInitializer{
		initFn: func(config core.InitConfig) (*core.InitResult, error) {
			return &core.InitResult{
				Created: []string{config.BasePath + "/tickets", config.BasePath + "/work"},
			}, nil
		},
	}
	ProjectInit = mock

	err := initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitCommand_CustomPath(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()

	mock := &mockProjectInitializer{}
	ProjectInit = mock

	err := initCmd.RunE(initCmd, []string{"/tmp/test-project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// filepath.Abs resolves the path relative to the current drive on Windows.
	expectedPath, _ := filepath.Abs("/tmp/test-project")
	if mock.lastConfig.BasePath != expectedPath {
		t.Errorf("expected basePath %s, got %s", expectedPath, mock.lastConfig.BasePath)
	}
}

func TestInitCommand_DefaultFlagValues(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()

	mock := &mockProjectInitializer{}
	ProjectInit = mock

	// Reset flags to defaults before test.
	_ = initCmd.Flags().Set("name", "")
	_ = initCmd.Flags().Set("ai", "claude")
	_ = initCmd.Flags().Set("prefix", "TASK")

	err := initCmd.RunE(initCmd, []string{"/tmp/test-defaults"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastConfig.AI != "claude" {
		t.Errorf("expected default AI 'claude', got %q", mock.lastConfig.AI)
	}
	if mock.lastConfig.Prefix != "TASK" {
		t.Errorf("expected default prefix 'TASK', got %q", mock.lastConfig.Prefix)
	}
	// Name defaults to basename of path.
	if mock.lastConfig.Name != "test-defaults" {
		t.Errorf("expected default name 'test-defaults', got %q", mock.lastConfig.Name)
	}
}

func TestInitCommand_CustomFlags(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()

	mock := &mockProjectInitializer{}
	ProjectInit = mock

	_ = initCmd.Flags().Set("name", "my-project")
	_ = initCmd.Flags().Set("ai", "kiro")
	_ = initCmd.Flags().Set("prefix", "PRJ")

	err := initCmd.RunE(initCmd, []string{"/tmp/test-custom"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.lastConfig.Name != "my-project" {
		t.Errorf("expected name 'my-project', got %q", mock.lastConfig.Name)
	}
	if mock.lastConfig.AI != "kiro" {
		t.Errorf("expected AI 'kiro', got %q", mock.lastConfig.AI)
	}
	if mock.lastConfig.Prefix != "PRJ" {
		t.Errorf("expected prefix 'PRJ', got %q", mock.lastConfig.Prefix)
	}

	// Reset flags after test.
	_ = initCmd.Flags().Set("name", "")
	_ = initCmd.Flags().Set("ai", "claude")
	_ = initCmd.Flags().Set("prefix", "TASK")
}

func TestInitCommand_InitError(t *testing.T) {
	origInit := ProjectInit
	defer func() { ProjectInit = origInit }()

	ProjectInit = &mockProjectInitializer{
		initFn: func(config core.InitConfig) (*core.InitResult, error) {
			return nil, fmt.Errorf("disk full")
		},
	}

	err := initCmd.RunE(initCmd, []string{})
	if err == nil {
		t.Fatal("expected error from Init")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("unexpected error: %v", err)
	}
}
