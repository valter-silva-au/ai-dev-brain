package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRemoveTicketProjectionDeletesExactScopedAggregate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTicketTestStore(t, ctx)
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	observeTicketTestOrganization(
		t,
		ctx,
		store,
		"organization-1",
		"amazon",
		now,
	)
	observeTicketTestRepository(
		t,
		ctx,
		store,
		"repository-1",
		"organization-1",
		"one",
		now,
	)
	observeTicketTestRepository(
		t,
		ctx,
		store,
		"repository-2",
		"organization-1",
		"two",
		now,
	)

	targetScope := TicketScope{
		OrganizationID: "organization-1",
		RepositoryID:   "repository-1",
	}
	otherRepositoryScope := TicketScope{
		OrganizationID: "organization-1",
		RepositoryID:   "repository-2",
	}
	organizationScope := TicketScope{OrganizationID: "organization-1"}

	target := ticketProjectionWithChildren(
		"ticket-target",
		targetScope,
		"ADB-39",
		"/workspace/repository-1-tickets/ADB-39",
		now,
	)
	otherRepository := ticketProjectionWithChildren(
		"ticket-other-repository",
		otherRepositoryScope,
		"ADB-39",
		"/workspace/repository-2-tickets/ADB-39",
		now,
	)
	organizationTicket := ticketProjectionWithChildren(
		"ticket-organization",
		organizationScope,
		"ADB-39",
		"/workspace/organization-tickets/ADB-39",
		now,
	)
	for _, projection := range []TicketProjection{
		target,
		otherRepository,
		organizationTicket,
	} {
		if err := store.ObserveTicket(ctx, projection); err != nil {
			t.Fatalf("observe ticket %q: %v", projection.ID, err)
		}
	}

	if err := store.RemoveTicketProjection(
		ctx,
		targetScope,
		target.Aliases[0],
	); err != nil {
		t.Fatalf("remove ticket projection: %v", err)
	}

	if _, err := store.Ticket(ctx, targetScope, target.ID); !errors.Is(
		err,
		sql.ErrNoRows,
	) {
		t.Fatalf("read removed ticket error = %v, want sql.ErrNoRows", err)
	}
	for _, table := range []string{
		"ticket_aliases",
		"ticket_artifacts",
		"source_observations",
		"source_dependencies",
	} {
		t.Run("removes_"+table, func(t *testing.T) {
			var count int
			if err := store.db.QueryRowContext(
				ctx,
				"SELECT COUNT(*) FROM "+table+" WHERE ticket_id = ?",
				target.ID,
			).Scan(&count); err != nil {
				t.Fatalf("count removed ticket rows: %v", err)
			}
			if count != 0 {
				t.Fatalf("removed ticket rows = %d, want 0", count)
			}
		})
	}

	for _, item := range []struct {
		name  string
		scope TicketScope
		id    string
	}{
		{
			name:  "same alias in another repository",
			scope: otherRepositoryScope,
			id:    otherRepository.ID,
		},
		{
			name:  "same alias at organization scope",
			scope: organizationScope,
			id:    organizationTicket.ID,
		},
	} {
		t.Run(item.name, func(t *testing.T) {
			got, err := store.Ticket(ctx, item.scope, target.Aliases[0])
			if err != nil {
				t.Fatalf("read ticket in other scope: %v", err)
			}
			if got.ID != item.id {
				t.Fatalf("ticket in other scope = %q, want %q", got.ID, item.id)
			}
		})
	}
}

func TestRemoveTicketProjectionReturnsScopedNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		scope    func(TicketScope) TicketScope
		selector func(TicketProjection) string
	}{
		{
			name: "repository ticket is absent from organization scope",
			scope: func(scope TicketScope) TicketScope {
				return TicketScope{OrganizationID: scope.OrganizationID}
			},
			selector: func(projection TicketProjection) string {
				return projection.ID
			},
		},
		{
			name: "missing selector is absent from exact scope",
			scope: func(scope TicketScope) TicketScope {
				return scope
			},
			selector: func(TicketProjection) string {
				return "missing-ticket"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openTicketTestStore(t, ctx)
			scope := observeTicketTestScope(t, ctx, store)
			projection := testTicketProjection(
				"ticket-1",
				scope,
				"ADB-39",
				now,
			)
			if err := store.ObserveTicket(ctx, projection); err != nil {
				t.Fatalf("observe ticket projection: %v", err)
			}

			err := store.RemoveTicketProjection(
				ctx,
				test.scope(scope),
				test.selector(projection),
			)
			if !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("removal error = %v, want sql.ErrNoRows", err)
			}
			if !strings.Contains(err.Error(), "remove ticket projection") {
				t.Fatalf("removal error = %q, want clear context", err)
			}
			if _, readErr := store.Ticket(
				ctx,
				scope,
				projection.ID,
			); readErr != nil {
				t.Fatalf("failed removal changed ticket: %v", readErr)
			}
		})
	}
}

func TestRemoveTicketProjectionRollsBackWhenChildrenRemain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTicketTestStore(t, ctx)
	scope := observeTicketTestScope(t, ctx, store)
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	projection := ticketProjectionWithChildren(
		"ticket-1",
		scope,
		"ADB-39",
		"/workspace/tickets/ADB-39",
		now,
	)
	if err := store.ObserveTicket(ctx, projection); err != nil {
		t.Fatalf("observe ticket projection: %v", err)
	}

	store.db.SetMaxOpenConns(1)
	if _, err := store.db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	})

	err := store.RemoveTicketProjection(ctx, scope, projection.ID)
	if err == nil || !strings.Contains(err.Error(), "cleanup incomplete") {
		t.Fatalf("removal error = %v, want incomplete cleanup error", err)
	}
	got, readErr := store.Ticket(ctx, scope, projection.ID)
	if readErr != nil {
		t.Fatalf("read ticket after rolled-back removal: %v", readErr)
	}
	if got.ID != projection.ID ||
		len(got.Aliases) != len(projection.Aliases) ||
		len(got.Artifacts) != len(projection.Artifacts) ||
		len(got.Sources) != len(projection.Sources) ||
		len(got.Dependencies) != len(projection.Dependencies) {
		t.Fatalf("rolled-back ticket aggregate = %#v", got)
	}
}

func ticketProjectionWithChildren(
	id string,
	scope TicketScope,
	visibleKey string,
	path string,
	observedAt time.Time,
) TicketProjection {
	projection := testTicketProjection(id, scope, visibleKey, observedAt)
	projection.Path = path
	projection.Aliases = []string{"legacy-39"}
	projection.Artifacts = []ArtifactProjection{{
		Role:         "context",
		Path:         "context.md",
		Authority:    "authored",
		SearchPolicy: "lexical-semantic",
		SourceHash:   "context-v1",
	}}
	projection.Sources = []SourceObservation{
		{
			CanonicalSource: "ticket://" + id + "/context.md",
			AuthorityClass:  SourceAuthoritySource,
			ContentHash:     "context-v1",
			ObservedAt:      observedAt,
			RefreshPolicy:   "explicit",
			State:           SourceStateCurrent,
		},
		{
			CanonicalSource: "ticket://" + id + "/status.yaml",
			AuthorityClass:  SourceAuthorityProjection,
			ContentHash:     "status-v1",
			ObservedAt:      observedAt,
			RefreshPolicy:   "command-preflight",
			State:           SourceStateCurrent,
		},
	}
	projection.Dependencies = []SourceDependency{{
		CanonicalSource: "ticket://" + id + "/status.yaml",
		DependsOnSource: "ticket://" + id + "/context.md",
		Type:            "renders",
	}}
	return projection
}
