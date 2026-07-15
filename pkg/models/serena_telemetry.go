package models

// SerenaRecord is one agent self-report of how effective Serena's code-nav was
// in a session (#203). It is carried as the payload of a
// serena.effectiveness_recorded observability event; there is no separate store.
type SerenaRecord struct {
	// Verdict is one of ValidSerenaVerdicts: helped | neutral | hindered | unused.
	Verdict string `json:"verdict"`
	// Score is a 1..5 self-rating (0 = unset).
	Score int `json:"score"`
	// UsedFor names what Serena was used for (e.g. "find_symbol on the CLI").
	UsedFor string `json:"used_for,omitempty"`
	// Beat is what Serena beat / replaced (e.g. "grep across 40 files").
	Beat string `json:"beat,omitempty"`
	// Friction is any friction encountered (e.g. "slow first activation").
	Friction string `json:"friction,omitempty"`
	// TaskID optionally ties the report to a ticket.
	TaskID string `json:"task_id,omitempty"`
}

// SerenaRollup summarises recorded Serena-effectiveness reports for the operator
// (#203): totals, a per-verdict breakdown, the mean score, and the most recent
// entries.
type SerenaRollup struct {
	Total        int            `json:"total"`
	ByVerdict    map[string]int `json:"by_verdict"`
	AverageScore float64        `json:"average_score"`
	Recent       []SerenaRecord `json:"recent"`
}

// ValidSerenaVerdicts is the closed set of effectiveness verdicts. "unused" is a
// first-class, honest outcome — Serena wasn't needed — not an error.
var ValidSerenaVerdicts = []string{"helped", "neutral", "hindered", "unused"}

// IsValidSerenaVerdict reports whether v is an accepted verdict.
func IsValidSerenaVerdict(v string) bool {
	for _, valid := range ValidSerenaVerdicts {
		if v == valid {
			return true
		}
	}
	return false
}
