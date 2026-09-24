package controlplane

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenBootstrapsAndReopensCurrentSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open fresh control plane: %v", err)
	}

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}

	var applicationID int
	if err := store.db.QueryRowContext(
		ctx,
		"PRAGMA application_id",
	).Scan(&applicationID); err != nil {
		t.Fatalf("read application id: %v", err)
	}
	if applicationID != ApplicationID {
		t.Fatalf("application id = %#x, want %#x", applicationID, ApplicationID)
	}

	var migrationCount int
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != CurrentSchemaVersion {
		t.Fatalf(
			"migration count = %d, want %d",
			migrationCount,
			CurrentSchemaVersion,
		)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close fresh control plane: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.Close()
	})

	if err := reopened.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations after reopen: %v", err)
	}
	if migrationCount != CurrentSchemaVersion {
		t.Fatalf(
			"migration count after reopen = %d, want %d",
			migrationCount,
			CurrentSchemaVersion,
		)
	}
}

func TestOpenMigratesExistingV1ControlPlaneToV2(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	v1, err := openWithMigrations(ctx, path, currentMigrations[:1])
	if err != nil {
		t.Fatalf("open schema-v1 control plane: %v", err)
	}
	if err := v1.Close(); err != nil {
		t.Fatalf("close schema-v1 control plane: %v", err)
	}

	migrated, err := openWithMigrations(ctx, path, currentMigrations[:2])
	if err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = migrated.Close()
	})

	version, err := migrated.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("read migrated schema version: %v", err)
	}
	if version != 2 {
		t.Fatalf("migrated schema version = %d, want 2", version)
	}

	for _, table := range []string{
		"organization_projection",
		"organization_aliases",
		"repository_projection",
		"repository_aliases",
	} {
		t.Run("creates_"+table, func(t *testing.T) {
			var count int
			if err := migrated.db.QueryRowContext(
				ctx,
				`SELECT COUNT(*) FROM sqlite_master
				 WHERE type = 'table' AND name = ?`,
				table,
			).Scan(&count); err != nil {
				t.Fatalf("inspect migrated table: %v", err)
			}
			if count != 1 {
				t.Fatalf("migrated table count = %d, want 1", count)
			}
		})
	}
}

func TestOpenMigratesExistingV2ControlPlaneToV3(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	v2, err := openWithMigrations(ctx, path, currentMigrations[:2])
	if err != nil {
		t.Fatalf("open schema-v2 control plane: %v", err)
	}
	now := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	if err := v2.ObserveOrganization(ctx, OrganizationProjection{
		ID:           "organization-1",
		Slug:         "amazon",
		Path:         "/workspace/organizations/amazon",
		DisplayName:  "Amazon",
		ManifestHash: "organization-v2",
		Status:       EntityStatusActive,
		ObservedAt:   now,
	}); err != nil {
		t.Fatalf("seed schema-v2 organization: %v", err)
	}
	if err := v2.ObserveRepository(ctx, RepositoryProjection{
		ID:              "repository-1",
		OrganizationID:  "organization-1",
		Host:            "github.com",
		Owner:           "valter-silva-au",
		Name:            "ai-dev-brain",
		Path:            "/workspace/organizations/amazon/repos/github.com/valter-silva-au/ai-dev-brain",
		ManifestHash:    "repository-v2",
		CanonicalRemote: "https://github.com/valter-silva-au/ai-dev-brain.git",
		Status:          EntityStatusActive,
		ObservedAt:      now,
	}); err != nil {
		t.Fatalf("seed schema-v2 repository: %v", err)
	}
	if err := v2.Close(); err != nil {
		t.Fatalf("close schema-v2 control plane: %v", err)
	}

	migrated, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrate control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = migrated.Close()
	})

	version, err := migrated.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("read migrated schema version: %v", err)
	}
	if version != 3 {
		t.Fatalf("migrated schema version = %d, want 3", version)
	}

	for _, table := range []string{
		"ticket_projection",
		"ticket_aliases",
		"ticket_artifacts",
		"source_observations",
		"source_dependencies",
	} {
		t.Run("creates_"+table, func(t *testing.T) {
			var count int
			if err := migrated.db.QueryRowContext(
				ctx,
				`SELECT COUNT(*) FROM sqlite_master
				 WHERE type = 'table' AND name = ?`,
				table,
			).Scan(&count); err != nil {
				t.Fatalf("inspect migrated table: %v", err)
			}
			if count != 1 {
				t.Fatalf("migrated table count = %d, want 1", count)
			}
		})
	}

	organization, err := migrated.Organization(ctx, "organization-1")
	if err != nil {
		t.Fatalf("read migrated organization: %v", err)
	}
	if organization.ManifestHash != "organization-v2" {
		t.Fatalf("migrated organization = %#v", organization)
	}
	repository, err := migrated.Repository(
		ctx,
		"organization-1",
		"repository-1",
	)
	if err != nil {
		t.Fatalf("read migrated repository: %v", err)
	}
	if repository.ManifestHash != "repository-v2" {
		t.Fatalf("migrated repository = %#v", repository)
	}
}

func TestOpenRejectsMigrationDriftBeforeApplyingNextMigration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	v2, err := openWithMigrations(ctx, path, currentMigrations[:2])
	if err != nil {
		t.Fatalf("open schema-v2 control plane: %v", err)
	}
	if _, err := v2.db.ExecContext(
		ctx,
		`UPDATE schema_migrations
		    SET checksum = 'drifted'
		  WHERE version = 2`,
	); err != nil {
		t.Fatalf("corrupt schema-v2 migration checksum: %v", err)
	}
	if err := v2.Close(); err != nil {
		t.Fatalf("close drifted schema-v2 control plane: %v", err)
	}

	store, err := Open(ctx, path)
	if store != nil {
		_ = store.Close()
	}
	if !errors.Is(err, ErrMigrationDrift) {
		t.Fatalf("open drifted control plane error = %v, want ErrMigrationDrift", err)
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open drifted control plane directly: %v", err)
	}
	t.Cleanup(func() {
		_ = raw.Close()
	})

	var version int
	if err := raw.QueryRowContext(
		ctx,
		"PRAGMA user_version",
	).Scan(&version); err != nil {
		t.Fatalf("read drifted schema version: %v", err)
	}
	if version != 2 {
		t.Fatalf("drifted schema version = %d, want 2", version)
	}

	var ticketTables int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		  WHERE type = 'table' AND name = 'ticket_projection'`,
	).Scan(&ticketTables); err != nil {
		t.Fatalf("inspect drifted ticket projection table: %v", err)
	}
	if ticketTables != 0 {
		t.Fatalf("ticket projection table count = %d, want 0", ticketTables)
	}
}

func TestOrganizationProjectionPreservesIdentityAcrossMove(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	observedAt := time.Date(
		2026,
		time.September,
		10,
		10,
		0,
		0,
		0,
		time.UTC,
	)
	projection := OrganizationProjection{
		ID:           "organization-1",
		Slug:         "amazon",
		Path:         "/workspace/organizations/amazon",
		DisplayName:  "Amazon",
		ManifestHash: "manifest-v1",
		Status:       EntityStatusActive,
		Aliases:      []string{},
		ObservedAt:   observedAt,
	}
	if err := store.ObserveOrganization(ctx, projection); err != nil {
		t.Fatalf("observe organization: %v", err)
	}

	projection.Slug = "amazon-web-services"
	projection.Path = "/workspace/organizations/amazon-web-services"
	projection.ManifestHash = "manifest-v2"
	projection.Aliases = []string{
		"amazon",
		"/workspace/organizations/amazon",
	}
	projection.ObservedAt = observedAt.Add(time.Minute)
	if err := store.ObserveOrganization(ctx, projection); err != nil {
		t.Fatalf("move organization projection: %v", err)
	}

	for _, test := range []struct {
		name     string
		selector string
	}{
		{name: "immutable_id", selector: "organization-1"},
		{name: "current_slug", selector: "amazon-web-services"},
		{name: "previous_slug_alias", selector: "amazon"},
		{
			name:     "previous_path_alias",
			selector: "/workspace/organizations/amazon",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := store.Organization(ctx, test.selector)
			if err != nil {
				t.Fatalf("read organization: %v", err)
			}
			if got.ID != "organization-1" ||
				got.Slug != "amazon-web-services" ||
				got.Path != projection.Path {
				t.Fatalf("organization = %#v", got)
			}
			if !reflect.DeepEqual(got.Aliases, projection.Aliases) {
				t.Fatalf(
					"organization aliases = %v, want %v",
					got.Aliases,
					projection.Aliases,
				)
			}
		})
	}

	organizations, err := store.Organizations(ctx)
	if err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	if len(organizations) != 1 ||
		organizations[0].ID != "organization-1" {
		t.Fatalf("organizations = %#v", organizations)
	}
}

func TestOrganizationProjectionEnforcesParentAndNameUniqueness(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	parent := OrganizationProjection{
		ID:           "organization-parent",
		Slug:         "parent",
		Path:         "/workspace/organizations/parent",
		DisplayName:  "Parent",
		ManifestHash: "parent-hash",
		Status:       EntityStatusActive,
		ObservedAt:   now,
	}
	if err := store.ObserveOrganization(ctx, parent); err != nil {
		t.Fatalf("observe parent: %v", err)
	}
	child := OrganizationProjection{
		ID:           "organization-child",
		Slug:         "child",
		Path:         "/workspace/organizations/child",
		DisplayName:  "Child",
		ParentID:     parent.ID,
		ManifestHash: "child-hash",
		Status:       EntityStatusActive,
		Aliases:      []string{"child-old"},
		ObservedAt:   now,
	}
	if err := store.ObserveOrganization(ctx, child); err != nil {
		t.Fatalf("observe child: %v", err)
	}

	collision := OrganizationProjection{
		ID:           "organization-collision",
		Slug:         "child-old",
		Path:         "/workspace/organizations/collision",
		DisplayName:  "Collision",
		ManifestHash: "collision-hash",
		Status:       EntityStatusActive,
		ObservedAt:   now,
	}
	if err := store.ObserveOrganization(ctx, collision); err == nil {
		t.Fatal("organization slug colliding with alias was accepted")
	}

	orphan := OrganizationProjection{
		ID:           "organization-orphan",
		Slug:         "orphan",
		Path:         "/workspace/organizations/orphan",
		DisplayName:  "Orphan",
		ParentID:     "organization-missing",
		ManifestHash: "orphan-hash",
		Status:       EntityStatusActive,
		ObservedAt:   now,
	}
	if err := store.ObserveOrganization(ctx, orphan); err == nil {
		t.Fatal("organization with missing parent was accepted")
	}
}

func TestRepositoryProjectionRoundTripsAliasesAndOwner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	if err := store.ObserveOrganization(ctx, OrganizationProjection{
		ID:           "organization-1",
		Slug:         "amazon",
		Path:         "/workspace/organizations/amazon",
		DisplayName:  "Amazon",
		ManifestHash: "organization-hash",
		Status:       EntityStatusActive,
		ObservedAt:   now,
	}); err != nil {
		t.Fatalf("observe owning organization: %v", err)
	}

	projection := RepositoryProjection{
		ID:              "repository-1",
		OrganizationID:  "organization-1",
		Host:            "github.com",
		Owner:           "valter-silva-au",
		Name:            "ai-dev-brain",
		Path:            "/workspace/organizations/amazon/repos/github.com/valter-silva-au/ai-dev-brain",
		ManifestHash:    "repository-hash",
		CanonicalRemote: "git@github.com:valter-silva-au/ai-dev-brain.git",
		Status:          EntityStatusActive,
		Aliases:         []string{"github.com/old/ai-dev-brain"},
		ObservedAt:      now,
	}
	if err := store.ObserveRepository(ctx, projection); err != nil {
		t.Fatalf("observe repository: %v", err)
	}

	for _, test := range []struct {
		name     string
		selector string
	}{
		{name: "immutable_id", selector: "repository-1"},
		{
			name:     "current_host_owner_name",
			selector: "github.com/valter-silva-au/ai-dev-brain",
		},
		{
			name:     "previous_host_owner_name_alias",
			selector: "github.com/old/ai-dev-brain",
		},
		{name: "current_path", selector: projection.Path},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := store.Repository(
				ctx,
				"organization-1",
				test.selector,
			)
			if err != nil {
				t.Fatalf("read repository: %v", err)
			}
			if got.ID != projection.ID ||
				got.OrganizationID != projection.OrganizationID ||
				got.CanonicalRemote != projection.CanonicalRemote {
				t.Fatalf("repository = %#v", got)
			}
			if !reflect.DeepEqual(got.Aliases, projection.Aliases) {
				t.Fatalf(
					"repository aliases = %v, want %v",
					got.Aliases,
					projection.Aliases,
				)
			}
		})
	}

	repositories, err := store.Repositories(ctx, "organization-1")
	if err != nil {
		t.Fatalf("list repositories: %v", err)
	}
	if len(repositories) != 1 || repositories[0].ID != projection.ID {
		t.Fatalf("repositories = %#v", repositories)
	}
}

func TestTicketProjectionRoundTripsAndReplacesChildren(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTicketTestStore(t, ctx)
	scope := observeTicketTestScope(t, ctx, store)
	awst := time.FixedZone("AWST", 8*60*60)
	observedAt := time.Date(
		2026,
		time.September,
		10,
		17,
		30,
		45,
		123456789,
		awst,
	)
	sourceUpdatedAt := observedAt.Add(-2 * time.Hour)
	lastSuccessAt := observedAt.Add(-time.Minute)
	projection := testTicketProjection(
		"ticket-1",
		scope,
		"ADB-39",
		observedAt,
	)
	projection.Aliases = []string{"github:#39", "ADB-0039"}
	projection.Artifacts = []ArtifactProjection{
		{
			Role:         "notes",
			Path:         "notes.md",
			Authority:    "authored",
			SearchPolicy: "lexical-semantic",
			RenderedHash: "",
			SourceHash:   "notes-source-v1",
		},
		{
			Role:         "context",
			Path:         "context.md",
			Authority:    "generated",
			SearchPolicy: "lexical-semantic",
			RenderedHash: "context-rendered-v1",
			SourceHash:   "context-source-v1",
		},
	}
	projection.Sources = []SourceObservation{
		{
			CanonicalSource: "ticket://ticket-1/status.yaml",
			AuthorityClass:  SourceAuthorityProjection,
			ContentHash:     "status-v1",
			ObservedAt:      observedAt,
			TTL:             15 * time.Minute,
			RefreshPolicy:   "command-preflight",
			State:           SourceStateStale,
			LastError:       "remote unavailable",
		},
		{
			CanonicalSource: "ticket://ticket-1/context.md",
			AuthorityClass:  SourceAuthoritySource,
			ContentHash:     "context-v1",
			ObservedAt:      observedAt.Add(-time.Second),
			SourceUpdatedAt: sourceUpdatedAt,
			TTL:             time.Hour,
			RefreshPolicy:   "dependency-or-ttl",
			State:           SourceStateCurrent,
			LastSuccessAt:   lastSuccessAt,
		},
	}
	projection.Dependencies = []SourceDependency{
		{
			CanonicalSource: "ticket://ticket-1/status.yaml",
			DependsOnSource: "ticket://ticket-1/context.md",
			Type:            "renders",
		},
	}

	if err := store.ObserveTicket(ctx, projection); err != nil {
		t.Fatalf("observe ticket projection: %v", err)
	}

	for _, test := range []struct {
		name     string
		selector string
	}{
		{name: "immutable_id", selector: projection.ID},
		{name: "visible_key", selector: projection.VisibleKey},
		{name: "path", selector: projection.Path},
		{name: "alias", selector: projection.Aliases[0]},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := store.Ticket(ctx, scope, test.selector)
			if err != nil {
				t.Fatalf("read ticket: %v", err)
			}
			if got.ID != projection.ID ||
				got.OrganizationID != scope.OrganizationID ||
				got.RepositoryID != scope.RepositoryID ||
				got.Status != projection.Status ||
				got.Type != projection.Type ||
				got.Priority != projection.Priority ||
				got.ProfileID != projection.ProfileID ||
				got.ProfileVersion != projection.ProfileVersion ||
				got.ManifestHash != projection.ManifestHash ||
				got.ArchiveState != projection.ArchiveState {
				t.Fatalf("ticket = %#v", got)
			}
			if !got.ObservedAt.Equal(observedAt) ||
				got.ObservedAt.Location() != time.UTC {
				t.Fatalf(
					"ticket observed_at = %v (%v), want %v UTC",
					got.ObservedAt,
					got.ObservedAt.Location(),
					observedAt,
				)
			}
			if !reflect.DeepEqual(got.Aliases, projection.Aliases) {
				t.Fatalf(
					"ticket aliases = %v, want %v",
					got.Aliases,
					projection.Aliases,
				)
			}
			if len(got.Artifacts) != 2 ||
				got.Artifacts[0].Role != "context" ||
				got.Artifacts[1].Role != "notes" {
				t.Fatalf("ticket artifacts = %#v", got.Artifacts)
			}
			if got.Artifacts[0].TicketID != projection.ID {
				t.Fatalf(
					"artifact ticket id = %q, want %q",
					got.Artifacts[0].TicketID,
					projection.ID,
				)
			}
			if got.Artifacts[0].Authority != "generated" ||
				got.Artifacts[0].SearchPolicy != "lexical-semantic" ||
				got.Artifacts[0].RenderedHash != "context-rendered-v1" ||
				got.Artifacts[0].SourceHash != "context-source-v1" {
				t.Fatalf(
					"context artifact = %#v",
					got.Artifacts[0],
				)
			}
			if len(got.Sources) != 2 ||
				got.Sources[0].CanonicalSource !=
					"ticket://ticket-1/context.md" ||
				got.Sources[1].CanonicalSource !=
					"ticket://ticket-1/status.yaml" {
				t.Fatalf("ticket sources = %#v", got.Sources)
			}
			if !got.Sources[0].SourceUpdatedAt.Equal(sourceUpdatedAt) ||
				!got.Sources[0].LastSuccessAt.Equal(lastSuccessAt) ||
				got.Sources[0].SourceUpdatedAt.Location() != time.UTC ||
				got.Sources[0].LastSuccessAt.Location() != time.UTC {
				t.Fatalf("source timestamps = %#v", got.Sources[0])
			}
			if got.Sources[0].AuthorityClass != SourceAuthoritySource ||
				got.Sources[0].ContentHash != "context-v1" ||
				got.Sources[0].TTL != time.Hour ||
				got.Sources[0].RefreshPolicy != "dependency-or-ttl" ||
				got.Sources[0].State != SourceStateCurrent ||
				got.Sources[0].LastError != "" ||
				got.Sources[1].LastError != "remote unavailable" {
				t.Fatalf("source fields = %#v", got.Sources)
			}
			if len(got.Dependencies) != 1 ||
				got.Dependencies[0].TicketID != projection.ID ||
				got.Dependencies[0].Type != "renders" {
				t.Fatalf(
					"ticket dependencies = %#v",
					got.Dependencies,
				)
			}
		})
	}

	replacement := projection
	replacement.VisibleKey = "ADB-40"
	replacement.Path = "/workspace/tickets/ADB-40"
	replacement.Status = "done"
	replacement.ProfileVersion = "v2"
	replacement.ManifestHash = "ticket-manifest-v2"
	replacement.ArchiveState = TicketArchiveStateArchived
	replacement.ObservedAt = observedAt.Add(time.Minute)
	replacement.Aliases = []string{"ADB-39", "github:#39"}
	replacement.Artifacts = []ArtifactProjection{
		{
			Role:         "context",
			Path:         "context.md",
			Authority:    "authored",
			SearchPolicy: "lexical",
			SourceHash:   "context-source-v2",
		},
	}
	replacement.Sources = []SourceObservation{
		{
			CanonicalSource: "ticket://ticket-1/context.md",
			AuthorityClass:  SourceAuthoritySource,
			ContentHash:     "context-v2",
			ObservedAt:      observedAt.Add(time.Minute),
			TTL:             2 * time.Hour,
			RefreshPolicy:   "explicit",
			State:           SourceStateCurrent,
			LastSuccessAt:   observedAt.Add(time.Minute),
		},
	}
	replacement.Dependencies = nil

	if err := store.ObserveTicket(ctx, replacement); err != nil {
		t.Fatalf("replace ticket projection: %v", err)
	}
	got, err := store.Ticket(ctx, scope, "ADB-39")
	if err != nil {
		t.Fatalf("read replaced ticket by old key: %v", err)
	}
	if got.ID != projection.ID ||
		got.VisibleKey != replacement.VisibleKey ||
		got.Path != replacement.Path ||
		got.ArchiveState != TicketArchiveStateArchived {
		t.Fatalf("replaced ticket = %#v", got)
	}
	if !reflect.DeepEqual(got.Aliases, replacement.Aliases) ||
		len(got.Artifacts) != 1 ||
		got.Artifacts[0].Role != "context" ||
		len(got.Sources) != 1 ||
		len(got.Dependencies) != 0 {
		t.Fatalf("replacement children were not exact: %#v", got)
	}
}

func TestTicketProjectionEnforcesOwnershipAndScopeUniqueSelectors(
	t *testing.T,
) {
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
	observeTicketTestOrganization(
		t,
		ctx,
		store,
		"organization-2",
		"other",
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
	observeTicketTestRepository(
		t,
		ctx,
		store,
		"repository-3",
		"organization-2",
		"three",
		now,
	)

	scope := TicketScope{
		OrganizationID: "organization-1",
		RepositoryID:   "repository-1",
	}
	original := testTicketProjection("ticket-1", scope, "ADB-39", now)
	original.Aliases = []string{"legacy-39"}
	if err := store.ObserveTicket(ctx, original); err != nil {
		t.Fatalf("observe original ticket: %v", err)
	}

	moved := original
	moved.OrganizationID = "organization-2"
	moved.RepositoryID = "repository-3"
	if err := store.ObserveTicket(ctx, moved); err == nil {
		t.Fatal("ticket replacement crossed ownership boundary")
	}
	got, err := store.Ticket(ctx, scope, original.ID)
	if err != nil {
		t.Fatalf("read original ticket after refused move: %v", err)
	}
	if got.OrganizationID != original.OrganizationID ||
		got.RepositoryID != original.RepositoryID {
		t.Fatalf("refused move changed ownership: %#v", got)
	}

	wrongOwner := testTicketProjection(
		"ticket-wrong-owner",
		TicketScope{
			OrganizationID: "organization-2",
			RepositoryID:   "repository-1",
		},
		"ADB-40",
		now,
	)
	if err := store.ObserveTicket(ctx, wrongOwner); err == nil {
		t.Fatal("ticket accepted repository from another organization")
	}

	organizationScoped := testTicketProjection(
		"ticket-organization",
		TicketScope{OrganizationID: "organization-1"},
		original.VisibleKey,
		now,
	)
	organizationScoped.Path = "/workspace/organization-tickets/ADB-39"
	if err := store.ObserveTicket(ctx, organizationScoped); err != nil {
		t.Fatalf("observe organization-scoped ticket: %v", err)
	}
	if organizationScoped.RepositoryID != "" {
		t.Fatalf(
			"organization-scoped repository = %q, want empty",
			organizationScoped.RepositoryID,
		)
	}

	otherRepository := testTicketProjection(
		"ticket-other-repository",
		TicketScope{
			OrganizationID: "organization-1",
			RepositoryID:   "repository-2",
		},
		original.VisibleKey,
		now,
	)
	otherRepository.Path = "/workspace/repository-2-tickets/ADB-39"
	if err := store.ObserveTicket(ctx, otherRepository); err != nil {
		t.Fatalf("observe same key in another repository: %v", err)
	}

	duplicateKey := testTicketProjection(
		"ticket-duplicate",
		scope,
		original.VisibleKey,
		now,
	)
	if err := store.ObserveTicket(ctx, duplicateKey); err == nil {
		t.Fatal("duplicate visible key in one scope was accepted")
	}

	aliasCollision := testTicketProjection(
		"ticket-alias-collision",
		scope,
		"ADB-41",
		now,
	)
	aliasCollision.Aliases = []string{original.VisibleKey}
	if err := store.ObserveTicket(ctx, aliasCollision); err == nil {
		t.Fatal("alias colliding with a visible key was accepted")
	}

	keyCollision := testTicketProjection(
		"ticket-key-collision",
		scope,
		original.Aliases[0],
		now,
	)
	if err := store.ObserveTicket(ctx, keyCollision); err == nil {
		t.Fatal("visible key colliding with an alias was accepted")
	}
}

func TestTicketProjectionSelectorsCannotCollideWithImmutableIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate func(TicketProjection, TicketScope, time.Time) TicketProjection
	}{
		{
			name: "visible key matches existing id",
			candidate: func(
				original TicketProjection,
				scope TicketScope,
				now time.Time,
			) TicketProjection {
				return testTicketProjection(
					"ticket-2",
					scope,
					original.ID,
					now,
				)
			},
		},
		{
			name: "immutable id matches existing visible key",
			candidate: func(
				original TicketProjection,
				scope TicketScope,
				now time.Time,
			) TicketProjection {
				return testTicketProjection(
					original.VisibleKey,
					scope,
					"ADB-40",
					now,
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := openTicketTestStore(t, ctx)
			scope := observeTicketTestScope(t, ctx, store)
			now := time.Date(
				2026,
				time.September,
				10,
				10,
				0,
				0,
				0,
				time.UTC,
			)
			original := testTicketProjection(
				"ticket-1",
				scope,
				"ADB-39",
				now,
			)
			if err := store.ObserveTicket(ctx, original); err != nil {
				t.Fatalf("observe original ticket: %v", err)
			}

			candidate := test.candidate(original, scope, now)
			if err := store.ObserveTicket(ctx, candidate); err == nil {
				t.Fatalf(
					"ticket selector collision was accepted: %#v",
					candidate,
				)
			}
		})
	}
}

func TestTicketsFiltersAndOrdersWithinExactScope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTicketTestStore(t, ctx)
	scope := observeTicketTestScope(t, ctx, store)
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)

	for _, projection := range []TicketProjection{
		testTicketProjection("ticket-z", scope, "ADB-30", now),
		testTicketProjection("ticket-a", scope, "ADB-10", now),
		testTicketProjection("ticket-b", scope, "ADB-20", now),
		testTicketProjection(
			"ticket-organization",
			TicketScope{OrganizationID: scope.OrganizationID},
			"ADB-00",
			now,
		),
	} {
		if projection.ID == "ticket-b" {
			projection.Status = "done"
			projection.Type = "fix"
			projection.Priority = "low"
			projection.ArchiveState = TicketArchiveStateArchived
		}
		if err := store.ObserveTicket(ctx, projection); err != nil {
			t.Fatalf("observe ticket %q: %v", projection.ID, err)
		}
	}

	tests := []struct {
		name    string
		filter  TicketFilter
		wantIDs []string
	}{
		{
			name:    "repository scope orders by visible key",
			filter:  TicketFilter{Scope: scope},
			wantIDs: []string{"ticket-a", "ticket-b", "ticket-z"},
		},
		{
			name: "combined filters select exact ticket",
			filter: TicketFilter{
				Scope:        scope,
				Status:       "done",
				Type:         "fix",
				Priority:     "low",
				ArchiveState: TicketArchiveStateArchived,
			},
			wantIDs: []string{"ticket-b"},
		},
		{
			name: "organization scope excludes repository tickets",
			filter: TicketFilter{
				Scope: TicketScope{
					OrganizationID: scope.OrganizationID,
				},
			},
			wantIDs: []string{"ticket-organization"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projections, err := store.Tickets(ctx, test.filter)
			if err != nil {
				t.Fatalf("list tickets: %v", err)
			}
			gotIDs := make([]string, 0, len(projections))
			for _, projection := range projections {
				gotIDs = append(gotIDs, projection.ID)
			}
			if !reflect.DeepEqual(gotIDs, test.wantIDs) {
				t.Fatalf("ticket ids = %v, want %v", gotIDs, test.wantIDs)
			}
		})
	}
}

func TestObserveTicketValidatesAggregateBeforeReplacement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	validArtifact := func(role string, path string) ArtifactProjection {
		return ArtifactProjection{
			Role:         role,
			Path:         path,
			Authority:    "authored",
			SearchPolicy: "lexical",
		}
	}
	validSource := func(canonicalSource string) SourceObservation {
		return SourceObservation{
			CanonicalSource: canonicalSource,
			AuthorityClass:  SourceAuthoritySource,
			ContentHash:     "content-hash",
			ObservedAt:      now,
			RefreshPolicy:   "explicit",
			State:           SourceStateCurrent,
		}
	}
	validSources := func() []SourceObservation {
		return []SourceObservation{
			validSource("ticket://ticket-1/context.md"),
			validSource("ticket://ticket-1/status.yaml"),
		}
	}
	validDependency := func() SourceDependency {
		return SourceDependency{
			CanonicalSource: "ticket://ticket-1/status.yaml",
			DependsOnSource: "ticket://ticket-1/context.md",
			Type:            "renders",
		}
	}

	tests := []struct {
		name      string
		mutate    func(*TicketProjection)
		wantError string
	}{
		{
			name: "ticket id is required",
			mutate: func(projection *TicketProjection) {
				projection.ID = ""
			},
			wantError: "ticket projection id is required",
		},
		{
			name: "ticket organization is required",
			mutate: func(projection *TicketProjection) {
				projection.OrganizationID = ""
			},
			wantError: "ticket projection organization id is required",
		},
		{
			name: "ticket visible key is required",
			mutate: func(projection *TicketProjection) {
				projection.VisibleKey = ""
			},
			wantError: "ticket projection visible key is required",
		},
		{
			name: "ticket path is required",
			mutate: func(projection *TicketProjection) {
				projection.Path = ""
			},
			wantError: "ticket projection path is required",
		},
		{
			name: "ticket status is required",
			mutate: func(projection *TicketProjection) {
				projection.Status = ""
			},
			wantError: "ticket projection status is required",
		},
		{
			name: "ticket type is required",
			mutate: func(projection *TicketProjection) {
				projection.Type = ""
			},
			wantError: "ticket projection type is required",
		},
		{
			name: "ticket priority is required",
			mutate: func(projection *TicketProjection) {
				projection.Priority = ""
			},
			wantError: "ticket projection priority is required",
		},
		{
			name: "ticket profile id is required",
			mutate: func(projection *TicketProjection) {
				projection.ProfileID = ""
			},
			wantError: "ticket projection profile id is required",
		},
		{
			name: "ticket profile version is required",
			mutate: func(projection *TicketProjection) {
				projection.ProfileVersion = ""
			},
			wantError: "ticket projection profile version is required",
		},
		{
			name: "ticket manifest hash is required",
			mutate: func(projection *TicketProjection) {
				projection.ManifestHash = ""
			},
			wantError: "ticket projection manifest hash is required",
		},
		{
			name: "ticket archive state is supported",
			mutate: func(projection *TicketProjection) {
				projection.ArchiveState = TicketArchiveState("deleted")
			},
			wantError: "unsupported ticket archive state",
		},
		{
			name: "ticket observed at is required",
			mutate: func(projection *TicketProjection) {
				projection.ObservedAt = time.Time{}
			},
			wantError: "ticket projection observed_at is required",
		},
		{
			name: "ticket visible key and path differ",
			mutate: func(projection *TicketProjection) {
				projection.Path = projection.VisibleKey
			},
			wantError: "ticket visible key and path must differ",
		},
		{
			name: "ticket alias cannot repeat current identity",
			mutate: func(projection *TicketProjection) {
				projection.Aliases = []string{projection.VisibleKey}
			},
			wantError: "duplicates the current identity",
		},
		{
			name: "ticket aliases are unique",
			mutate: func(projection *TicketProjection) {
				projection.Aliases = []string{"old", "old"}
			},
			wantError: "duplicate projection alias",
		},
		{
			name: "artifact ticket mismatch",
			mutate: func(projection *TicketProjection) {
				artifact := validArtifact("context", "context.md")
				artifact.TicketID = "ticket-other"
				projection.Artifacts = []ArtifactProjection{artifact}
			},
			wantError: "does not match ticket",
		},
		{
			name: "artifact role is required",
			mutate: func(projection *TicketProjection) {
				projection.Artifacts = []ArtifactProjection{
					validArtifact("", "context.md"),
				}
			},
			wantError: "artifact semantic role is required",
		},
		{
			name: "artifact path is required",
			mutate: func(projection *TicketProjection) {
				projection.Artifacts = []ArtifactProjection{
					validArtifact("context", ""),
				}
			},
			wantError: "artifact \"context\" path is required",
		},
		{
			name: "artifact authority is required",
			mutate: func(projection *TicketProjection) {
				artifact := validArtifact("context", "context.md")
				artifact.Authority = ""
				projection.Artifacts = []ArtifactProjection{artifact}
			},
			wantError: "artifact \"context\" authority is required",
		},
		{
			name: "artifact search policy is required",
			mutate: func(projection *TicketProjection) {
				artifact := validArtifact("context", "context.md")
				artifact.SearchPolicy = ""
				projection.Artifacts = []ArtifactProjection{artifact}
			},
			wantError: "artifact \"context\" search policy is required",
		},
		{
			name: "artifact roles are unique",
			mutate: func(projection *TicketProjection) {
				projection.Artifacts = []ArtifactProjection{
					validArtifact("context", "context.md"),
					validArtifact("context", "other.md"),
				}
			},
			wantError: "duplicate artifact semantic role",
		},
		{
			name: "artifact paths are unique",
			mutate: func(projection *TicketProjection) {
				projection.Artifacts = []ArtifactProjection{
					validArtifact("context", "shared.md"),
					validArtifact("notes", "shared.md"),
				}
			},
			wantError: "duplicate artifact path",
		},
		{
			name: "source ticket mismatch",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.TicketID = "ticket-other"
				projection.Sources = []SourceObservation{source}
			},
			wantError: "source ticket id",
		},
		{
			name: "source canonical source is required",
			mutate: func(projection *TicketProjection) {
				projection.Sources = []SourceObservation{validSource("")}
			},
			wantError: "canonical source is required",
		},
		{
			name: "source authority class is supported",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.AuthorityClass = SourceAuthorityClass("unknown")
				projection.Sources = []SourceObservation{source}
			},
			wantError: "unsupported source authority class",
		},
		{
			name: "source content hash is required",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.ContentHash = ""
				projection.Sources = []SourceObservation{source}
			},
			wantError: "content hash is required",
		},
		{
			name: "source observed at is required",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.ObservedAt = time.Time{}
				projection.Sources = []SourceObservation{source}
			},
			wantError: "observed_at is required",
		},
		{
			name: "negative source ttl",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.TTL = -time.Second
				projection.Sources = []SourceObservation{source}
			},
			wantError: "ttl cannot be negative",
		},
		{
			name: "source refresh policy is required",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.RefreshPolicy = ""
				projection.Sources = []SourceObservation{source}
			},
			wantError: "refresh policy is required",
		},
		{
			name: "source state is supported",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				source.State = SourceState("unknown")
				projection.Sources = []SourceObservation{source}
			},
			wantError: "unsupported source state",
		},
		{
			name: "canonical sources are unique",
			mutate: func(projection *TicketProjection) {
				source := validSource("ticket://ticket-1/context.md")
				projection.Sources = []SourceObservation{source, source}
			},
			wantError: "duplicate canonical source",
		},
		{
			name: "dependency ticket mismatch",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				dependency.TicketID = "ticket-other"
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{dependency}
			},
			wantError: "dependency ticket id",
		},
		{
			name: "dependency canonical source is required",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				dependency.CanonicalSource = ""
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{dependency}
			},
			wantError: "dependency canonical source is required",
		},
		{
			name: "dependency target is required",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				dependency.DependsOnSource = ""
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{dependency}
			},
			wantError: "dependency source target is required",
		},
		{
			name: "source cannot depend on itself",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				dependency.DependsOnSource = dependency.CanonicalSource
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{dependency}
			},
			wantError: "cannot depend on itself",
		},
		{
			name: "dependency type is required",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				dependency.Type = ""
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{dependency}
			},
			wantError: "type is required",
		},
		{
			name: "dependency source must be observed",
			mutate: func(projection *TicketProjection) {
				projection.Sources = []SourceObservation{
					validSource("ticket://ticket-1/context.md"),
				}
				projection.Dependencies = []SourceDependency{
					validDependency(),
				}
			},
			wantError: "dependency source",
		},
		{
			name: "dependency target must be observed",
			mutate: func(projection *TicketProjection) {
				projection.Sources = []SourceObservation{
					validSource("ticket://ticket-1/status.yaml"),
				}
				projection.Dependencies = []SourceDependency{
					validDependency(),
				}
			},
			wantError: "dependency target",
		},
		{
			name: "dependencies are unique",
			mutate: func(projection *TicketProjection) {
				dependency := validDependency()
				projection.Sources = validSources()
				projection.Dependencies = []SourceDependency{
					dependency,
					dependency,
				}
			},
			wantError: "duplicate dependency",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openTicketTestStore(t, ctx)
			scope := observeTicketTestScope(t, ctx, store)
			original := testTicketProjection("ticket-1", scope, "ADB-39", now)
			if err := store.ObserveTicket(ctx, original); err != nil {
				t.Fatalf("observe original ticket: %v", err)
			}

			replacement := original
			replacement.VisibleKey = "ADB-invalid"
			replacement.Path = "/workspace/tickets/ADB-invalid"
			test.mutate(&replacement)
			err := store.ObserveTicket(ctx, replacement)
			if err == nil {
				t.Fatal("invalid ticket replacement was accepted")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf(
					"validation error = %q, want substring %q",
					err,
					test.wantError,
				)
			}
			got, err := store.Ticket(ctx, scope, original.ID)
			if err != nil {
				t.Fatalf("read original ticket: %v", err)
			}
			if got.VisibleKey != original.VisibleKey ||
				got.Path != original.Path {
				t.Fatalf("invalid replacement changed ticket: %#v", got)
			}
		})
	}
}

func TestObserveTicketRollsBackAfterChildReplacementFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := openTicketTestStore(t, ctx)
	scope := observeTicketTestScope(t, ctx, store)
	now := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	original := testTicketProjection("ticket-1", scope, "ADB-39", now)
	original.Artifacts = []ArtifactProjection{{
		Role:         "context",
		Path:         "context.md",
		Authority:    "authored",
		SearchPolicy: "lexical",
		SourceHash:   "original",
	}}
	if err := store.ObserveTicket(ctx, original); err != nil {
		t.Fatalf("observe original ticket: %v", err)
	}
	if _, err := store.db.ExecContext(
		ctx,
		`CREATE TRIGGER fail_ticket_artifact_insert
		 BEFORE INSERT ON ticket_artifacts
		 WHEN NEW.source_hash = 'force-database-failure'
		 BEGIN
		   SELECT RAISE(ABORT, 'forced ticket artifact failure');
		 END`,
	); err != nil {
		t.Fatalf("create ticket artifact failure trigger: %v", err)
	}

	replacement := original
	replacement.Status = "done"
	replacement.ManifestHash = "ticket-manifest-v2"
	replacement.Artifacts = []ArtifactProjection{{
		Role:         "context",
		Path:         "context.md",
		Authority:    "generated",
		SearchPolicy: "lexical-semantic",
		RenderedHash: "rendered-v2",
		SourceHash:   "force-database-failure",
	}}
	if err := store.ObserveTicket(ctx, replacement); err == nil {
		t.Fatal("ticket replacement succeeded despite forced database failure")
	}

	got, err := store.Ticket(ctx, scope, original.ID)
	if err != nil {
		t.Fatalf("read ticket after failed replacement: %v", err)
	}
	wantArtifacts := []ArtifactProjection{{
		TicketID:     original.ID,
		Role:         "context",
		Path:         "context.md",
		Authority:    "authored",
		SearchPolicy: "lexical",
		SourceHash:   "original",
	}}
	if got.Status != original.Status ||
		got.ManifestHash != original.ManifestHash ||
		!reflect.DeepEqual(got.Artifacts, wantArtifacts) {
		t.Fatalf("failed replacement changed ticket aggregate: %#v", got)
	}
}

func TestMigrationFailureRollsBackSchemaAndVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	broken := []migration{{
		Version: 1,
		Name:    "broken",
		SQL: `
CREATE TABLE partial_state (id INTEGER PRIMARY KEY);
THIS IS NOT VALID SQL;
`,
	}}

	store, err := openWithMigrations(ctx, path, broken)
	if err == nil {
		_ = store.Close()
		t.Fatal("expected broken migration to fail")
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	t.Cleanup(func() {
		_ = raw.Close()
	})

	var version int
	if err := raw.QueryRowContext(
		ctx,
		"PRAGMA user_version",
	).Scan(&version); err != nil {
		t.Fatalf("read rolled-back version: %v", err)
	}
	if version != 0 {
		t.Fatalf("version after rollback = %d, want 0", version)
	}

	var partialTables int
	if err := raw.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type = 'table' AND name = 'partial_state'`,
	).Scan(&partialTables); err != nil {
		t.Fatalf("count partial tables: %v", err)
	}
	if partialTables != 0 {
		t.Fatalf("partial table survived rollback")
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	if _, err := raw.ExecContext(
		ctx,
		"PRAGMA application_id = 0x41494442; PRAGMA user_version = 99;",
	); err != nil {
		t.Fatalf("seed newer schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	store, err := Open(ctx, path)
	if store != nil {
		_ = store.Close()
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("open error = %v, want ErrSchemaTooNew", err)
	}
}

func TestOpenRejectsForeignDatabaseWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "foreign.sqlite")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open foreign database: %v", err)
	}
	if _, err := raw.ExecContext(
		ctx,
		`PRAGMA application_id = 0x12345678;
		 CREATE TABLE foreign_data (id INTEGER PRIMARY KEY, value TEXT);
		 INSERT INTO foreign_data (value) VALUES ('preserve me');`,
	); err != nil {
		t.Fatalf("seed foreign database: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close foreign database: %v", err)
	}

	before := snapshotDirectory(t, directory)
	store, err := Open(ctx, path)
	if store != nil {
		_ = store.Close()
	}
	if !errors.Is(err, ErrForeignDatabase) {
		t.Fatalf("open foreign database error = %v, want ErrForeignDatabase", err)
	}
	after := snapshotDirectory(t, directory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"foreign database was mutated\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func TestCheckReportsIntegrityAndIncompleteOperations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	if _, err := store.db.ExecContext(
		ctx,
		`INSERT INTO operations (
			operation_id, idempotency_key, kind, plan_hash, status,
			started_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"operation-1",
		"workspace.initialize:workspace-1",
		"workspace.initialize",
		"plan-hash",
		"applying",
		now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed incomplete operation: %v", err)
	}

	check, err := store.Check(ctx)
	if err != nil {
		t.Fatalf("check control plane: %v", err)
	}
	if check.Integrity != "ok" {
		t.Fatalf("integrity = %q, want ok", check.Integrity)
	}
	if check.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf(
			"schema version = %d, want %d",
			check.SchemaVersion,
			CurrentSchemaVersion,
		)
	}
	if len(check.IncompleteOperations) != 1 {
		t.Fatalf(
			"incomplete operations = %d, want 1",
			len(check.IncompleteOperations),
		)
	}
	if check.IncompleteOperations[0].ID != "operation-1" {
		t.Fatalf(
			"incomplete operation id = %q",
			check.IncompleteOperations[0].ID,
		)
	}
}

func TestOpenReadOnlyObservesCommittedTicketUpdates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	writer := openTicketTestStoreAtPath(t, ctx, path)
	scope := observeTicketTestScope(t, ctx, writer)
	now := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	projection := testTicketProjection("ticket-1", scope, "ADB-39", now)
	if err := writer.ObserveTicket(ctx, projection); err != nil {
		t.Fatalf("observe initial ticket: %v", err)
	}

	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("open read-only control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
	})
	initial, err := reader.Ticket(ctx, scope, projection.ID)
	if err != nil {
		t.Fatalf("read initial ticket: %v", err)
	}
	if initial.ManifestHash != "ticket-manifest-v1" {
		t.Fatalf("initial manifest hash = %q", initial.ManifestHash)
	}

	projection.ManifestHash = "ticket-manifest-v2"
	projection.ObservedAt = now.Add(time.Minute)
	if err := writer.ObserveTicket(ctx, projection); err != nil {
		t.Fatalf("observe updated ticket: %v", err)
	}

	updated, err := reader.Ticket(ctx, scope, projection.ID)
	if err != nil {
		t.Fatalf("read updated ticket: %v", err)
	}
	if updated.ManifestHash != projection.ManifestHash {
		t.Fatalf(
			"updated manifest hash = %q, want %q",
			updated.ManifestHash,
			projection.ManifestHash,
		)
	}
}

func TestTicketReadsOneAggregateRevisionDuringConcurrentReplacement(
	t *testing.T,
) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	reader := openTicketTestStoreAtPath(t, ctx, path)
	scope := observeTicketTestScope(t, ctx, reader)
	writer, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open concurrent writer: %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
	})

	now := time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC)
	revision := func(version string, observedAt time.Time) TicketProjection {
		projection := testTicketProjection(
			"ticket-1",
			scope,
			"ADB-39",
			observedAt,
		)
		projection.ManifestHash = version
		projection.Aliases = make([]string, 128)
		projection.Artifacts = make([]ArtifactProjection, 128)
		for index := range projection.Aliases {
			projection.Aliases[index] = fmt.Sprintf(
				"%s-alias-%03d",
				version,
				index,
			)
			projection.Artifacts[index] = ArtifactProjection{
				Role:         fmt.Sprintf("artifact-%03d", index),
				Path:         fmt.Sprintf("artifacts/%03d.md", index),
				Authority:    "authored",
				SearchPolicy: "lexical",
				SourceHash:   version,
			}
		}
		return projection
	}
	revisions := []TicketProjection{
		revision("revision-1", now),
		revision("revision-2", now.Add(time.Second)),
	}
	if err := writer.ObserveTicket(ctx, revisions[0]); err != nil {
		t.Fatalf("observe initial revision: %v", err)
	}

	writerErrors := make(chan error, 1)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for iteration := 0; iteration < 300; iteration++ {
			if err := writer.ObserveTicket(
				ctx,
				revisions[iteration%len(revisions)],
			); err != nil {
				writerErrors <- err
				return
			}
		}
	}()

	for iteration := 0; iteration < 300; iteration++ {
		got, err := reader.Ticket(ctx, scope, "ticket-1")
		if err != nil {
			t.Fatalf("read concurrent ticket revision: %v", err)
		}
		for _, alias := range got.Aliases {
			if !strings.HasPrefix(alias, got.ManifestHash+"-alias-") {
				t.Fatalf(
					"mixed ticket revision: manifest=%q alias=%q",
					got.ManifestHash,
					alias,
				)
			}
		}
		for _, artifact := range got.Artifacts {
			if artifact.SourceHash != got.ManifestHash {
				t.Fatalf(
					"mixed ticket revision: manifest=%q artifact=%#v",
					got.ManifestHash,
					artifact,
				)
			}
		}
	}
	<-writerDone
	select {
	case err := <-writerErrors:
		t.Fatalf("replace concurrent ticket revision: %v", err)
	default:
	}
}

func TestOpenReadOnlyDoesNotMutateDatabaseFiles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	directory := t.TempDir()
	path := filepath.Join(directory, "state.sqlite")

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	if err := store.ObserveWorkspace(ctx, WorkspaceProjection{
		ID:           "workspace-1",
		Root:         "/workspace",
		ManifestHash: "manifest-hash",
		ObservedAt:   time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("observe workspace: %v", err)
	}
	if err := store.ObserveOrganization(ctx, OrganizationProjection{
		ID:           "organization-1",
		Slug:         "amazon",
		Path:         "/workspace/organizations/amazon",
		DisplayName:  "Amazon",
		ManifestHash: "organization-hash",
		Status:       EntityStatusActive,
		ObservedAt:   time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("observe organization: %v", err)
	}
	if err := store.ObserveTicket(ctx, testTicketProjection(
		"ticket-1",
		TicketScope{OrganizationID: "organization-1"},
		"ADB-39",
		time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	)); err != nil {
		t.Fatalf("observe ticket: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close writable control plane: %v", err)
	}

	before := snapshotDirectory(t, directory)
	readOnly, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatalf("open read-only control plane: %v", err)
	}
	if _, err := readOnly.Check(ctx); err != nil {
		t.Fatalf("check read-only control plane: %v", err)
	}
	projection, err := readOnly.Workspace(ctx)
	if err != nil {
		t.Fatalf("read workspace projection: %v", err)
	}
	if projection.ID != "workspace-1" {
		t.Fatalf("workspace id = %q, want workspace-1", projection.ID)
	}
	organization, err := readOnly.Organization(ctx, "amazon")
	if err != nil {
		t.Fatalf("read organization projection: %v", err)
	}
	if organization.ID != "organization-1" {
		t.Fatalf("organization id = %q, want organization-1", organization.ID)
	}
	ticket, err := readOnly.Ticket(
		ctx,
		TicketScope{OrganizationID: "organization-1"},
		"ADB-39",
	)
	if err != nil {
		t.Fatalf("read ticket projection: %v", err)
	}
	if ticket.ID != "ticket-1" {
		t.Fatalf("ticket id = %q, want ticket-1", ticket.ID)
	}
	tickets, err := readOnly.Tickets(ctx, TicketFilter{
		Scope: TicketScope{OrganizationID: "organization-1"},
	})
	if err != nil {
		t.Fatalf("list ticket projections: %v", err)
	}
	if len(tickets) != 1 || tickets[0].ID != "ticket-1" {
		t.Fatalf("ticket projections = %#v", tickets)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatalf("close read-only control plane: %v", err)
	}

	after := snapshotDirectory(t, directory)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf(
			"read-only access mutated database files\nbefore=%v\nafter=%v",
			before,
			after,
		)
	}
}

func openTicketTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()

	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func observeTicketTestScope(
	t *testing.T,
	ctx context.Context,
	store *Store,
) TicketScope {
	t.Helper()

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
		"ai-dev-brain",
		now,
	)
	return TicketScope{
		OrganizationID: "organization-1",
		RepositoryID:   "repository-1",
	}
}

func observeTicketTestOrganization(
	t *testing.T,
	ctx context.Context,
	store *Store,
	id string,
	slug string,
	observedAt time.Time,
) {
	t.Helper()

	if err := store.ObserveOrganization(ctx, OrganizationProjection{
		ID:           id,
		Slug:         slug,
		Path:         "/workspace/organizations/" + slug,
		DisplayName:  slug,
		ManifestHash: slug + "-manifest",
		Status:       EntityStatusActive,
		ObservedAt:   observedAt,
	}); err != nil {
		t.Fatalf("observe organization %q: %v", id, err)
	}
}

func observeTicketTestRepository(
	t *testing.T,
	ctx context.Context,
	store *Store,
	id string,
	organizationID string,
	name string,
	observedAt time.Time,
) {
	t.Helper()

	if err := store.ObserveRepository(ctx, RepositoryProjection{
		ID:              id,
		OrganizationID:  organizationID,
		Host:            "github.com",
		Owner:           organizationID,
		Name:            name,
		Path:            "/workspace/repositories/" + id,
		ManifestHash:    id + "-manifest",
		CanonicalRemote: "https://github.com/" + organizationID + "/" + name,
		Status:          EntityStatusActive,
		ObservedAt:      observedAt,
	}); err != nil {
		t.Fatalf("observe repository %q: %v", id, err)
	}
}

func testTicketProjection(
	id string,
	scope TicketScope,
	visibleKey string,
	observedAt time.Time,
) TicketProjection {
	return TicketProjection{
		ID:             id,
		OrganizationID: scope.OrganizationID,
		RepositoryID:   scope.RepositoryID,
		VisibleKey:     visibleKey,
		Path:           "/workspace/tickets/" + visibleKey,
		Status:         "in_progress",
		Type:           "feat",
		Priority:       "high",
		ProfileID:      "engineering",
		ProfileVersion: "v1",
		ManifestHash:   "ticket-manifest-v1",
		ArchiveState:   TicketArchiveStateActive,
		ObservedAt:     observedAt,
	}
}

func snapshotDirectory(t *testing.T, directory string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read snapshot directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatalf("read snapshot file %q: %v", entry.Name(), err)
		}
		snapshot[entry.Name()] = fmt.Sprintf(
			"size=%d sha256=%x",
			len(content),
			sha256.Sum256(content),
		)
	}
	return snapshot
}

func openTicketTestStoreAtPath(
	t *testing.T,
	ctx context.Context,
	path string,
) *Store {
	t.Helper()

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open ticket test control plane: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
