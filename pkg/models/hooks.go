package models

// HookConfig holds hook configuration from .taskconfig.
type HookConfig struct {
	Enabled       bool                `yaml:"enabled" mapstructure:"enabled"`
	PreToolUse    PreToolUseConfig    `yaml:"pre_tool_use" mapstructure:"pre_tool_use"`
	PostToolUse   PostToolUseConfig   `yaml:"post_tool_use" mapstructure:"post_tool_use"`
	Stop          StopConfig          `yaml:"stop" mapstructure:"stop"`
	TaskCompleted TaskCompletedConfig `yaml:"task_completed" mapstructure:"task_completed"`
	SessionEnd    SessionEndConfig    `yaml:"session_end" mapstructure:"session_end"`
}

// DefaultHookConfig returns sensible defaults for hook configuration.
// Phase 1 features are enabled by default, Phase 2/3 are disabled.
func DefaultHookConfig() HookConfig {
	return HookConfig{
		Enabled: true,
		PreToolUse: PreToolUseConfig{
			Enabled:           true,
			BlockVendor:       true,
			BlockGoSum:        true,
			ArchitectureGuard: false, // Phase 2
			ADRConflictCheck:  false, // Phase 3
		},
		PostToolUse: PostToolUseConfig{
			Enabled:             true,
			GoFormat:            true,
			ChangeTracking:      true,
			DependencyDetection: false, // Phase 2
		},
		Stop: StopConfig{
			Enabled:          true,
			UncommittedCheck: true,
			BuildCheck:       true,
			VetCheck:         true,
			ContextUpdate:    true,
			StatusTimestamp:  true,
		},
		TaskCompleted: TaskCompletedConfig{
			Enabled:          true,
			CheckUncommitted: true,
			RunTests:         true,
			RunLint:          true,
			TestCommand:      "go test ./...",
			LintCommand:      "golangci-lint run",
			ExtractKnowledge: false, // Phase 2
			UpdateWiki:       false, // Phase 2
			GenerateADRs:     false, // Phase 3
			UpdateContext:    true,
		},
		SessionEnd: SessionEndConfig{
			Enabled:           true,
			CaptureSession:    true,
			MinTurnsCapture:   3,
			UpdateContext:     true,
			ExtractKnowledge:  false, // Phase 2
			LogCommunications: false, // Phase 3
		},
	}
}

// PreToolUseConfig controls checks run before Edit/Write tool calls.
type PreToolUseConfig struct {
	Enabled           bool `yaml:"enabled" mapstructure:"enabled"`
	BlockVendor       bool `yaml:"block_vendor" mapstructure:"block_vendor"`
	BlockGoSum        bool `yaml:"block_go_sum" mapstructure:"block_go_sum"`
	ArchitectureGuard bool `yaml:"architecture_guard" mapstructure:"architecture_guard"`
	ADRConflictCheck  bool `yaml:"adr_conflict_check" mapstructure:"adr_conflict_check"`
}

// PostToolUseConfig controls actions run after Edit/Write tool calls.
type PostToolUseConfig struct {
	Enabled             bool `yaml:"enabled" mapstructure:"enabled"`
	GoFormat            bool `yaml:"go_format" mapstructure:"go_format"`
	ChangeTracking      bool `yaml:"change_tracking" mapstructure:"change_tracking"`
	DependencyDetection bool `yaml:"dependency_detection" mapstructure:"dependency_detection"`
}

// StopConfig controls checks run when a conversation is stopped.
type StopConfig struct {
	Enabled          bool `yaml:"enabled" mapstructure:"enabled"`
	UncommittedCheck bool `yaml:"uncommitted_check" mapstructure:"uncommitted_check"`
	BuildCheck       bool `yaml:"build_check" mapstructure:"build_check"`
	VetCheck         bool `yaml:"vet_check" mapstructure:"vet_check"`
	ContextUpdate    bool `yaml:"context_update" mapstructure:"context_update"`
	StatusTimestamp  bool `yaml:"status_timestamp" mapstructure:"status_timestamp"`
}

// TaskCompletedConfig controls checks and actions run when a task is marked completed.
type TaskCompletedConfig struct {
	Enabled          bool   `yaml:"enabled" mapstructure:"enabled"`
	CheckUncommitted bool   `yaml:"check_uncommitted" mapstructure:"check_uncommitted"`
	RunTests         bool   `yaml:"run_tests" mapstructure:"run_tests"`
	RunLint          bool   `yaml:"run_lint" mapstructure:"run_lint"`
	TestCommand      string `yaml:"test_command" mapstructure:"test_command"`
	LintCommand      string `yaml:"lint_command" mapstructure:"lint_command"`
	ExtractKnowledge bool   `yaml:"extract_knowledge" mapstructure:"extract_knowledge"`
	UpdateWiki       bool   `yaml:"update_wiki" mapstructure:"update_wiki"`
	GenerateADRs     bool   `yaml:"generate_adrs" mapstructure:"generate_adrs"`
	UpdateContext    bool   `yaml:"update_context" mapstructure:"update_context"`
}

// SessionEndConfig controls actions run when an AI coding session ends.
type SessionEndConfig struct {
	Enabled           bool `yaml:"enabled" mapstructure:"enabled"`
	CaptureSession    bool `yaml:"capture_session" mapstructure:"capture_session"`
	MinTurnsCapture   int  `yaml:"min_turns_capture" mapstructure:"min_turns_capture"`
	UpdateContext     bool `yaml:"update_context" mapstructure:"update_context"`
	ExtractKnowledge  bool `yaml:"extract_knowledge" mapstructure:"extract_knowledge"`
	LogCommunications bool `yaml:"log_communications" mapstructure:"log_communications"`
}

// SessionChangeEntry represents a tracked file modification during a session.
type SessionChangeEntry struct {
	Timestamp int64  `yaml:"timestamp"`
	Tool      string `yaml:"tool"`
	FilePath  string `yaml:"file_path"`
}
