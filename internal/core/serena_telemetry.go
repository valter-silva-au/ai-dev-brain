package core

import "github.com/valter-silva-au/ai-dev-brain/pkg/models"

// SerenaTelemetry is the Serena effectiveness-telemetry service behind
// `adb serena record` / `adb serena report` (#203). Record self-reports a
// scorecard as a serena.effectiveness_recorded event; Report rolls the recorded
// history up from the append-only event log — there is no separate store. The
// concrete implementation is wired in app.go over the observability EventLog.
type SerenaTelemetry interface {
	// Record emits exactly one serena.effectiveness_recorded event for rec.
	Record(rec models.SerenaRecord) error
	// Report reads the recorded history and rolls it up for the operator.
	Report() (models.SerenaRollup, error)
}
