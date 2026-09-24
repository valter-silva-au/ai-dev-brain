package ticket

import (
	"context"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
	"github.com/valter-silva-au/ai-dev-brain/internal/repository"
)

var (
	CreateDescriptor = capability.Descriptor{
		Capability: "ticket.create",
		Version:    "v1",
		Command:    "create",
		Tool:       "adb_ticket_create",
		Summary:    "Plan or create a portable ticket.",
		Mutating:   true,
	}
	ListDescriptor = capability.Descriptor{
		Capability: "ticket.list",
		Version:    "v1",
		Command:    "list",
		Tool:       "adb_ticket_list",
		Summary:    "List portable tickets deterministically.",
		Mutating:   false,
	}
	ShowDescriptor = capability.Descriptor{
		Capability: "ticket.show",
		Version:    "v1",
		Command:    "show",
		Tool:       "adb_ticket_show",
		Summary:    "Show one portable ticket.",
		Mutating:   false,
	}
	UpdateDescriptor = capability.Descriptor{
		Capability: "ticket.update",
		Version:    "v1",
		Command:    "update",
		Tool:       "adb_ticket_update",
		Summary:    "Plan or update a portable ticket.",
		Mutating:   true,
	}
	CloseDescriptor = capability.Descriptor{
		Capability: "ticket.close",
		Version:    "v1",
		Command:    "close",
		Tool:       "adb_ticket_close",
		Summary:    "Plan or close a ticket through its completion gate.",
		Mutating:   true,
	}
	ArchiveDescriptor = capability.Descriptor{
		Capability: "ticket.archive",
		Version:    "v1",
		Command:    "archive",
		Tool:       "adb_ticket_archive",
		Summary:    "Plan or change ticket archive state.",
		Mutating:   true,
	}
	RemoveDescriptor = capability.Descriptor{
		Capability: "ticket.remove",
		Version:    "v1",
		Command:    "remove",
		Tool:       "adb_ticket_remove",
		Summary:    "Plan or exactly remove an archived ticket.",
		Mutating:   true,
	}
)

type Options struct {
	Clock                func() time.Time
	IDGenerator          func() string
	TemplateResolver     profile.TemplateResolver
	ProfileResolver      ProfileResolver
	CompletionGate       CompletionGate
	Git                  repository.Git
	AfterPlan            func() error
	BeforeProjection     func(context.Context, Layout, Manifest) error
	AfterProjection      func(context.Context, Layout, Manifest) error
	AfterFilesystemStage func(string) error
	BeforeMutationLock   func() error
}

type ProfileResolver interface {
	ResolveProfile(profile.ProfileReference) (profile.Profile, error)
}

type ProfileResolverFunc func(
	profile.ProfileReference,
) (profile.Profile, error)

func (function ProfileResolverFunc) ResolveProfile(
	reference profile.ProfileReference,
) (profile.Profile, error) {
	return function(reference)
}

type CompletionGate interface {
	Check(context.Context, TicketData) ([]Finding, error)
}

type CompletionGateFunc func(context.Context, TicketData) ([]Finding, error)

func (function CompletionGateFunc) Check(
	ctx context.Context,
	ticket TicketData,
) ([]Finding, error) {
	return function(ctx, ticket)
}

type Finding struct {
	Code       string            `json:"code" yaml:"code"`
	Severity   string            `json:"severity" yaml:"severity"`
	Summary    string            `json:"summary" yaml:"summary"`
	Evidence   []string          `json:"evidence" yaml:"evidence"`
	NextAction capability.Action `json:"next_action" yaml:"next_action"`
}

type TicketData struct {
	Manifest Manifest `json:"manifest" yaml:"manifest"`
	Layout   Layout   `json:"-" yaml:"-"`
	Path     string   `json:"path" yaml:"path"`
}

type MutationData struct {
	Ticket       TicketData `json:"ticket" yaml:"ticket"`
	OperationID  string     `json:"operation_id,omitempty" yaml:"operation_id,omitempty"`
	PreviousPath string     `json:"previous_path,omitempty" yaml:"previous_path,omitempty"`
	RecoveryPath string     `json:"recovery_path,omitempty" yaml:"recovery_path,omitempty"`
	Findings     []Finding  `json:"findings" yaml:"findings"`
}

type ListRequest struct {
	Scope           ScopeLayout `json:"-" yaml:"-"`
	IncludeArchived bool        `json:"include_archived" yaml:"include_archived"`
}

type ListData struct {
	Tickets []TicketData `json:"tickets" yaml:"tickets"`
}

type ShowRequest struct {
	Scope    ScopeLayout `json:"-" yaml:"-"`
	Selector string      `json:"selector" yaml:"selector"`
}

type ShowData struct {
	Ticket TicketData `json:"ticket" yaml:"ticket"`
}

type CreateRequest struct {
	Scope       ScopeLayout
	Title       string
	PathSlug    string
	BranchSlug  string
	Type        string
	Priority    Priority
	Owner       string
	Tags        []string
	Profile     profile.Profile
	KeyPolicy   KeyPolicy
	ActorType   string
	ActorID     string
	Tool        string
	OperationID string
	Apply       bool
}

type UpdateRequest struct {
	Scope       ScopeLayout
	Selector    string
	Profile     profile.Profile
	NextProfile *profile.Profile
	Title       *string
	PathSlug    *string
	BranchSlug  *string
	Type        *string
	Status      *Status
	Priority    *Priority
	Owner       *string
	Tags        *[]string
	ActorType   string
	ActorID     string
	Tool        string
	Apply       bool
}

type CloseRequest struct {
	Scope     ScopeLayout
	Selector  string
	Profile   profile.Profile
	ActorType string
	ActorID   string
	Tool      string
	Apply     bool
}

type ArchiveRequest struct {
	Scope     ScopeLayout
	Selector  string
	Profile   profile.Profile
	Restore   bool
	ActorType string
	ActorID   string
	Tool      string
	Apply     bool
}

type RemoveRequest struct {
	Scope      ScopeLayout
	Selector   string
	ExpectedID string
	Profile    profile.Profile
	ActorType  string
	ActorID    string
	Tool       string
	Apply      bool
}
