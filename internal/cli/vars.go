package cli

import "github.com/drapaimern/ai-dev-brain/internal/observability"

// Observability service instances, set during app initialization in app.go.
var (
	EventLog    observability.EventLog
	AlertEngine observability.AlertEngine
	MetricsCalc observability.MetricsCalculator
	Notifier    observability.Notifier
)
