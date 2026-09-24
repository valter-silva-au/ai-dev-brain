package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	SchemaVersion      = "aidb.operation-plan/v1"
	EventSchemaVersion = "aidb.operation-event/v1"
)

type Phase string

const (
	PhaseApplying     Phase = "applying"
	PhaseStepApplying Phase = "step_applying"
	PhaseStepApplied  Phase = "step_applied"
	PhaseCommitted    Phase = "committed"
	PhaseFailed       Phase = "failed"
)

type Status string

const (
	StatusPlanned   Status = "planned"
	StatusApplying  Status = "applying"
	StatusCommitted Status = "committed"
	StatusFailed    Status = "failed"
	StatusAttention Status = "needs_attention"
)

type Step struct {
	Ordinal       int    `json:"ordinal"`
	Action        string `json:"action"`
	Target        string `json:"target"`
	TicketID      string `json:"ticket_id,omitempty"`
	AppliedTarget string `json:"applied_target,omitempty"`
	BeforeHash    string `json:"before_hash,omitempty"`
	AfterHash     string `json:"after_hash,omitempty"`
}

type Plan struct {
	SchemaVersion  string    `json:"schema_version"`
	OperationID    string    `json:"operation_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Kind           string    `json:"kind"`
	CreatedAt      time.Time `json:"created_at"`
	Steps          []Step    `json:"steps"`
	Hash           string    `json:"hash"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type EventInput struct {
	Phase Phase
	Step  int
	Error *ErrorInfo
}

type Event struct {
	SchemaVersion string     `json:"schema_version"`
	OperationID   string     `json:"operation_id"`
	Sequence      int        `json:"sequence"`
	ID            string     `json:"id"`
	Phase         Phase      `json:"phase"`
	Timestamp     time.Time  `json:"timestamp"`
	Step          int        `json:"step,omitempty"`
	Error         *ErrorInfo `json:"error,omitempty"`
}

type State struct {
	Status            Status
	LastSequence      int
	Reason            string
	AppliedUnrecorded bool
}

func Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
