package cli

import (
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
)

// Observability service instances, set during app initialization in app.go.
var (
	EventLog    observability.EventLog
	AlertEngine observability.AlertEngine
	MetricsCalc observability.MetricsCalculator
	Notifier    observability.Notifier
)

// BranchPattern is the branch name format pattern from configuration.
// Set during app initialization in app.go.
var BranchPattern string

// SessionCapture provides access to the workspace session store.
// Set during app initialization in app.go.
var SessionCapture core.SessionCapturer

// HookEngine processes Claude Code hook events and updates adb artifacts.
// Set during app initialization in app.go.
var HookEngine core.HookEngine

// VersionChecker provides Claude Code version detection and feature gating.
// Set during app initialization in app.go.
var VersionChecker integration.ClaudeCodeVersionChecker
