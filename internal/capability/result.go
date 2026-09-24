package capability

type Outcome string

const (
	OutcomePlanned   Outcome = "planned"
	OutcomeApplied   Outcome = "applied"
	OutcomeUnchanged Outcome = "unchanged"
	OutcomeConflict  Outcome = "conflict"
	OutcomeHealthy   Outcome = "healthy"
	OutcomeAttention Outcome = "needs_attention"
	OutcomeFailed    Outcome = "failed"
)

type EffectStatus string

const (
	EffectPlanned EffectStatus = "planned"
	EffectApplied EffectStatus = "applied"
	EffectSkipped EffectStatus = "skipped"
	EffectFailed  EffectStatus = "failed"
)

type Effect struct {
	Action string       `json:"action" yaml:"action"`
	Target string       `json:"target" yaml:"target"`
	Status EffectStatus `json:"status" yaml:"status"`
}

type Notice struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
}

type Action struct {
	Code    string `json:"code" yaml:"code"`
	Message string `json:"message" yaml:"message"`
}

type Recovery struct {
	Required bool     `json:"required" yaml:"required"`
	Guidance []string `json:"guidance" yaml:"guidance"`
}

type Result[T any] struct {
	Capability  string   `json:"capability" yaml:"capability"`
	Version     string   `json:"version" yaml:"version"`
	Outcome     Outcome  `json:"outcome" yaml:"outcome"`
	Data        T        `json:"data" yaml:"data"`
	Effects     []Effect `json:"effects" yaml:"effects"`
	Warnings    []Notice `json:"warnings" yaml:"warnings"`
	NextActions []Action `json:"next_actions" yaml:"next_actions"`
	Recovery    Recovery `json:"recovery" yaml:"recovery"`
}
