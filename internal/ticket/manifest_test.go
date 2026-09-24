package ticket

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/profile"
)

func TestManifestRoundTripAndStrictValidation(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	createdAt := time.Date(
		2026,
		time.September,
		10,
		13,
		0,
		0,
		0,
		time.UTC,
	)
	manifest, err := NewManifest(
		"550e8400-e29b-41d4-a716-446655440000",
		scope,
		"TASK-00039",
		"Design AI Dev Brain v3 — self-maintaining workspace",
		"design-ai-dev-brain-v3",
		"design-ai-dev-brain-v3",
		"feat",
		activeProfile,
		createdAt,
		Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			ActorID:     "local-user",
			Tool:        "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("new ticket manifest: %v", err)
	}
	manifest.Owner = "platform"
	manifest.Tags = []string{"architecture", "v3"}
	manifest.Relationships = []Relationship{{
		Type:     RelationshipDependsOn,
		TargetID: "TASK-00027",
	}}
	manifest.RemoteReferences = []RemoteReference{{
		Provider: "github",
		Resource: "issue",
		ID:       "valter-silva-au/ai-dev-brain#39",
		URL:      "https://github.com/valter-silva-au/ai-dev-brain/issues/39",
	}}

	if err := manifest.Validate(layout, activeProfile); err != nil {
		t.Fatalf("validate ticket manifest: %v", err)
	}

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode ticket manifest: %v", err)
	}
	decoded, err := DecodeManifest(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode ticket manifest: %v", err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip\n got: %#v\nwant: %#v", decoded, manifest)
	}

	unknown := strings.Replace(
		encoded.String(),
		"kind: Ticket\n",
		"kind: Ticket\nunexpected: true\n",
		1,
	)
	if _, err := DecodeManifest(strings.NewReader(unknown)); err == nil {
		t.Fatal("unknown ticket manifest field was accepted")
	}
	if _, err := DecodeManifest(
		strings.NewReader(encoded.String() + "---\n{}\n"),
	); err == nil {
		t.Fatal("multiple ticket manifest documents were accepted")
	}
}

func TestManifestRejectsScopeIdentityAndSelectorCollisions(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	manifest := validManifest(t, scope, activeProfile)

	tests := map[string]func(*Manifest){
		"wrong organization": func(value *Manifest) {
			value.OrganizationID = "another-organization"
		},
		"unexpected repository": func(value *Manifest) {
			value.RepositoryID = "repository-1"
		},
		"visible key alias": func(value *Manifest) {
			value.Aliases = []string{value.VisibleKey}
		},
		"local key alias": func(value *Manifest) {
			value.Aliases = []string{value.LocalKey}
		},
		"path alias": func(value *Manifest) {
			value.Aliases = []string{layout.RelativePath()}
		},
		"duplicate folded alias": func(value *Manifest) {
			value.Aliases = []string{"TASK-00001-old", "task-00001-old"}
		},
		"profile mismatch": func(value *Manifest) {
			value.Profile.ID = "another-profile"
		},
		"branch mismatch": func(value *Manifest) {
			value.Branch.Intent = "feat/TASK-00001-wrong"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := manifest
			mutate(&candidate)
			if err := candidate.Validate(layout, activeProfile); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

func TestManifestRejectsInvalidMetadataAndArchiveShape(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	manifest := validManifest(t, scope, activeProfile)
	archivedAt := manifest.UpdatedAt.Add(time.Minute)

	tests := map[string]func(*Manifest){
		"empty title": func(value *Manifest) {
			value.Title = ""
		},
		"invalid status": func(value *Manifest) {
			value.Status = "closed"
		},
		"invalid priority": func(value *Manifest) {
			value.Priority = "urgent"
		},
		"duplicate tag": func(value *Manifest) {
			value.Tags = []string{"v3", "V3"}
		},
		"self relationship": func(value *Manifest) {
			value.Relationships = []Relationship{{
				Type:     RelationshipDependsOn,
				TargetID: value.VisibleKey,
			}}
		},
		"credentialed remote URL": func(value *Manifest) {
			value.RemoteReferences = []RemoteReference{{
				Provider: "github",
				Resource: "issue",
				ID:       "org/repo#1",
				URL:      "https://token@github.com/org/repo/issues/1",
			}}
		},
		"done without closed time": func(value *Manifest) {
			value.Status = StatusDone
		},
		"active with archived time": func(value *Manifest) {
			value.ArchivedAt = &archivedAt
		},
		"archived without archived time": func(value *Manifest) {
			value.ArchiveState = ArchiveStateArchived
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := manifest
			mutate(&candidate)
			if err := candidate.Validate(layout, activeProfile); err == nil {
				t.Fatalf("%s was accepted", name)
			}
		})
	}
}

func TestManifestAllowsRemoteVisibleKeyWithoutChangingLocalPathOrBranch(
	t *testing.T,
) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	manifest := validManifest(t, scope, activeProfile)
	manifest.VisibleKey = "github:valter-silva-au/ai-dev-brain#39"
	manifest.RemoteReferences = []RemoteReference{{
		Provider: "github",
		Resource: "issue",
		ID:       "valter-silva-au/ai-dev-brain#39",
		Primary:  true,
	}}

	if err := manifest.Validate(layout, activeProfile); err != nil {
		t.Fatalf("validate remote-visible manifest: %v", err)
	}
	if layout.RelativePath() !=
		"TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf("remote visible key changed local path: %q", layout.RelativePath())
	}
	if manifest.Branch.Intent !=
		"feat/TASK-00039-design-ai-dev-brain-v3" {
		t.Fatalf(
			"remote visible key changed branch intent: %q",
			manifest.Branch.Intent,
		)
	}

	manifest.RemoteReferences[0].Primary = false
	if err := manifest.Validate(layout, activeProfile); err == nil {
		t.Fatal("remote visible key without matching primary reference was accepted")
	}
}

func TestDecodeManifestRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	reader := io.LimitReader(
		strings.NewReader(strings.Repeat("x", int(maxManifestBytes)+2)),
		maxManifestBytes+2,
	)
	if _, err := DecodeManifest(reader); err == nil {
		t.Fatal("oversized ticket manifest was accepted")
	}
}

func TestPrepareMutationPreservesImmutableIdentityAndAliases(t *testing.T) {
	t.Parallel()

	scope, oldLayout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	previous := validManifest(t, scope, activeProfile)
	previous.VisibleKey = "github:valter-silva-au/ai-dev-brain#39"
	previous.RemoteReferences = []RemoteReference{{
		Provider: "github",
		Resource: "issue",
		ID:       "valter-silva-au/ai-dev-brain#39",
		Primary:  true,
	}}

	nextLayout, err := scope.Ticket(previous.LocalKey, "renamed-ticket")
	if err != nil {
		t.Fatalf("new next layout: %v", err)
	}
	next := previous
	next.VisibleKey = "github:valter-silva-au/ai-dev-brain#400"
	next.RemoteReferences = []RemoteReference{{
		Provider: "github",
		Resource: "issue",
		ID:       "valter-silva-au/ai-dev-brain#400",
		Primary:  true,
	}}
	next.Title = "A freely changed title: still independent"
	next.Aliases = []string{"legacy-selector"}
	next.PathSlug = "renamed-ticket"
	next.Branch.Slug = "independent-branch-slug"
	next.Status = StatusInProgress
	next.UpdatedAt = next.UpdatedAt.Add(time.Minute)
	next.LastMutation = Provenance{
		OperationID: "operation-2",
		ActorType:   "agent",
		ActorID:     "codex",
		Tool:        "adb-cli",
	}
	next.Branch.Intent, err = BranchIntent(
		next.Type,
		next.LocalKey,
		next.Branch.Slug,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("next branch intent: %v", err)
	}

	prepared, err := PrepareMutation(
		previous,
		next,
		oldLayout,
		nextLayout,
		activeProfile,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("prepare mutation: %v", err)
	}
	for _, want := range []string{
		previous.VisibleKey,
		oldLayout.OwnerRelativePath(),
		"legacy-selector",
	} {
		if !contains(prepared.Aliases, want) {
			t.Fatalf("aliases = %#v, missing %q", prepared.Aliases, want)
		}
	}
	if prepared.ID != previous.ID ||
		prepared.OrganizationID != previous.OrganizationID ||
		prepared.RepositoryID != previous.RepositoryID ||
		!prepared.CreatedAt.Equal(previous.CreatedAt) ||
		!reflect.DeepEqual(prepared.Provenance, previous.Provenance) {
		t.Fatal("prepare mutation changed immutable ticket identity")
	}

	crossScope := next
	crossScope.OrganizationID = "another-organization"
	if _, err := PrepareMutation(
		previous,
		crossScope,
		oldLayout,
		nextLayout,
		activeProfile,
		activeProfile,
	); err == nil {
		t.Fatal("cross-scope ticket mutation was accepted")
	}
	changedLocalKey := next
	changedLocalKey.LocalKey = "TASK-00400"
	if _, err := PrepareMutation(
		previous,
		changedLocalKey,
		oldLayout,
		nextLayout,
		activeProfile,
		activeProfile,
	); err == nil {
		t.Fatal("local key mutation was accepted")
	}
}

func TestPrepareMutationIsolatesArchiveAndRestoreMetadata(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	previous := validManifest(t, scope, activeProfile)
	archivedAt := previous.UpdatedAt.Add(time.Minute)
	archive := previous
	archive.ArchiveState = ArchiveStateArchived
	archive.ArchivedAt = &archivedAt
	archive.UpdatedAt = archivedAt
	archive.LastMutation = Provenance{
		OperationID: "archive-operation",
		ActorType:   "human",
		Tool:        "adb-cli",
	}

	archived, err := PrepareMutation(
		previous,
		archive,
		layout,
		layout,
		activeProfile,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("prepare archive: %v", err)
	}
	if archived.Status != previous.Status {
		t.Fatal("archive changed lifecycle status")
	}

	archiveWithEdit := archive
	archiveWithEdit.Title = "Unrelated title edit"
	if _, err := PrepareMutation(
		previous,
		archiveWithEdit,
		layout,
		layout,
		activeProfile,
		activeProfile,
	); err == nil {
		t.Fatal("archive bundled with ordinary metadata mutation was accepted")
	}

	restore := archived
	restore.ArchiveState = ArchiveStateActive
	restore.ArchivedAt = nil
	restore.UpdatedAt = restore.UpdatedAt.Add(time.Minute)
	restore.LastMutation = Provenance{
		OperationID: "restore-operation",
		ActorType:   "human",
		Tool:        "adb-cli",
	}
	if _, err := PrepareMutation(
		archived,
		restore,
		layout,
		layout,
		activeProfile,
		activeProfile,
	); err != nil {
		t.Fatalf("prepare restore: %v", err)
	}
}

func TestPrepareMutationSupportsExplicitProfileUpgrade(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	previousProfile := testBuiltinProfile(t)
	nextProfile := previousProfile
	nextProfile.Version = "v2"
	previous := validManifest(t, scope, previousProfile)
	next := previous
	next.Profile.Version = nextProfile.Version
	next.UpdatedAt = next.UpdatedAt.Add(time.Minute)
	next.LastMutation = Provenance{
		OperationID: "profile-upgrade",
		ActorType:   "agent",
		Tool:        "adb-cli",
	}

	prepared, err := PrepareMutation(
		previous,
		next,
		layout,
		layout,
		previousProfile,
		nextProfile,
	)
	if err != nil {
		t.Fatalf("prepare profile upgrade: %v", err)
	}
	if prepared.Profile.Version != "v2" {
		t.Fatalf("upgraded profile = %#v", prepared.Profile)
	}
}

func TestRoleRelocationPreservesPortableAndCanonicalPathAliases(t *testing.T) {
	t.Parallel()

	scope, oldLayout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	previous := validManifest(t, scope, activeProfile)
	newScope := scope
	newScope.ticketsRoot = filepath.Join(
		newScope.OwnerRoot(),
		"relocated-tickets",
	)
	newLayout, err := newScope.Ticket(
		previous.LocalKey,
		previous.PathSlug,
	)
	if err != nil {
		t.Fatalf("new relocated layout: %v", err)
	}
	next := previous
	next.UpdatedAt = next.UpdatedAt.Add(time.Minute)
	next.LastMutation = Provenance{
		OperationID: "role-relocation",
		ActorType:   "human",
		Tool:        "adb-cli",
	}

	prepared, err := PrepareMutation(
		previous,
		next,
		oldLayout,
		newLayout,
		activeProfile,
		activeProfile,
	)
	if err != nil {
		t.Fatalf("prepare role relocation: %v", err)
	}
	if !contains(prepared.Aliases, oldLayout.OwnerRelativePath()) {
		t.Fatalf(
			"portable aliases = %#v, missing %q",
			prepared.Aliases,
			oldLayout.OwnerRelativePath(),
		)
	}
	projection, err := BuildProjection(
		prepared,
		newLayout,
		activeProfile,
		"manifest-sha256",
		next.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("build relocated projection: %v", err)
	}
	if !contains(projection.Aliases, oldLayout.Root()) {
		t.Fatalf(
			"projection aliases = %#v, missing old path %q",
			projection.Aliases,
			oldLayout.Root(),
		)
	}
}

func TestOwnerRelocationPreservesWorkspaceRelativeAndCanonicalPathAliases(
	t *testing.T,
) {
	t.Parallel()

	activeProfile := testBuiltinProfile(t)
	for _, testCase := range []struct {
		name string
		kind ScopeKind
	}{
		{name: "organization", kind: ScopeOrganization},
		{name: "repository", kind: ScopeRepository},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			trustRoot := workspaceRoot
			if testCase.kind == ScopeRepository {
				trustRoot = filepath.Join(workspaceRoot, "organizations", "amazon")
			}
			oldOwner := filepath.Join(trustRoot, "old-owner")
			newOwner := filepath.Join(trustRoot, "new-owner")
			for _, root := range []string{oldOwner, newOwner} {
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatalf("create owner root: %v", err)
				}
			}
			oldScope := ScopeLayout{
				kind:           testCase.kind,
				workspaceRoot:  workspaceRoot,
				organizationID: "organization-1",
				trustRoot:      trustRoot,
				ownerRoot:      oldOwner,
				ticketsRoot:    filepath.Join(oldOwner, "tickets"),
			}
			newScope := oldScope
			newScope.ownerRoot = newOwner
			newScope.ticketsRoot = filepath.Join(newOwner, "tickets")
			if testCase.kind == ScopeRepository {
				oldScope.repositoryID = "repository-1"
				newScope.repositoryID = "repository-1"
			}

			oldLayout, err := oldScope.Ticket(
				"TASK-00039",
				"owner-relocation",
			)
			if err != nil {
				t.Fatalf("old ticket layout: %v", err)
			}
			newLayout, err := newScope.Ticket(
				"TASK-00039",
				"owner-relocation",
			)
			if err != nil {
				t.Fatalf("new ticket layout: %v", err)
			}
			previous := validManifest(t, oldScope, activeProfile)
			previous.PathSlug = "owner-relocation"
			previous.Branch.Slug = "owner-relocation"
			previous.Branch.Intent, err = BranchIntent(
				previous.Type,
				previous.LocalKey,
				previous.Branch.Slug,
				activeProfile,
			)
			if err != nil {
				t.Fatalf("branch intent: %v", err)
			}
			next := previous
			next.UpdatedAt = next.UpdatedAt.Add(time.Minute)
			next.LastMutation = Provenance{
				OperationID: "owner-relocation",
				ActorType:   "human",
				Tool:        "adb-cli",
			}

			prepared, err := PrepareMutation(
				previous,
				next,
				oldLayout,
				newLayout,
				activeProfile,
				activeProfile,
			)
			if err != nil {
				t.Fatalf("prepare owner relocation: %v", err)
			}
			workspaceAlias := workspacePathAlias(
				oldLayout.WorkspaceRelativePath(),
			)
			if !contains(prepared.Aliases, workspaceAlias) {
				t.Fatalf(
					"portable aliases = %#v, missing %q",
					prepared.Aliases,
					workspaceAlias,
				)
			}
			projection, err := BuildProjection(
				prepared,
				newLayout,
				activeProfile,
				"manifest-sha256",
				next.UpdatedAt,
			)
			if err != nil {
				t.Fatalf("build relocated projection: %v", err)
			}
			if !contains(projection.Aliases, oldLayout.Root()) {
				t.Fatalf(
					"projection aliases = %#v, missing old path %q",
					projection.Aliases,
					oldLayout.Root(),
				)
			}
		})
	}
}

func TestPrepareMutationRequiresFreshAuditMetadata(t *testing.T) {
	t.Parallel()

	scope, layout := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	previous := validManifest(t, scope, activeProfile)

	staleTime := previous
	staleTime.Title = "Changed title"
	staleTime.LastMutation = Provenance{
		OperationID: "changed-operation",
		ActorType:   "human",
		Tool:        "adb-cli",
	}
	if _, err := PrepareMutation(
		previous,
		staleTime,
		layout,
		layout,
		activeProfile,
		activeProfile,
	); err == nil {
		t.Fatal("mutation with unchanged updated_at was accepted")
	}

	staleOperation := previous
	staleOperation.Title = "Changed title"
	staleOperation.UpdatedAt = staleOperation.UpdatedAt.Add(time.Minute)
	if _, err := PrepareMutation(
		previous,
		staleOperation,
		layout,
		layout,
		activeProfile,
		activeProfile,
	); err == nil {
		t.Fatal("mutation with unchanged last_mutation operation was accepted")
	}
}

func TestNewManifestReturnsConstructionErrors(t *testing.T) {
	t.Parallel()

	scope, _ := testOrganizationTicketLayout(t)
	activeProfile := testBuiltinProfile(t)
	if _, err := NewManifest(
		"not-a-uuid",
		scope,
		"TASK-00039",
		"Title",
		"valid-path",
		"valid-branch",
		"prototype",
		activeProfile,
		time.Now(),
		Provenance{
			OperationID: "operation",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	); err == nil {
		t.Fatal("invalid manifest construction returned no error")
	}
}

func TestStatusTransitionsAreExplicit(t *testing.T) {
	t.Parallel()

	for _, transition := range []struct {
		from Status
		to   Status
	}{
		{StatusBacklog, StatusInProgress},
		{StatusInProgress, StatusBlocked},
		{StatusBlocked, StatusInProgress},
		{StatusInProgress, StatusReview},
		{StatusReview, StatusBlocked},
	} {
		if err := ValidateStatusTransition(
			transition.from,
			transition.to,
		); err != nil {
			t.Fatalf(
				"valid transition %s -> %s rejected: %v",
				transition.from,
				transition.to,
				err,
			)
		}
	}
	for _, transition := range []struct {
		from Status
		to   Status
	}{
		{StatusBacklog, StatusReview},
		{StatusInProgress, StatusBacklog},
		{StatusReview, StatusDone},
		{StatusDone, StatusReview},
		{StatusDone, StatusInProgress},
		{StatusReview, StatusBacklog},
		{Status("unknown"), StatusDone},
	} {
		if err := ValidateStatusTransition(
			transition.from,
			transition.to,
		); err == nil {
			t.Fatalf(
				"invalid transition %s -> %s was accepted",
				transition.from,
				transition.to,
			)
		}
	}
	for _, from := range []Status{StatusInProgress, StatusReview} {
		if err := ValidateCloseTransition(from, StatusDone); err != nil {
			t.Fatalf("valid close transition %s -> done rejected: %v", from, err)
		}
	}
	if err := ValidateCloseTransition(StatusDone, StatusDone); err != nil {
		t.Fatalf("idempotent close transition rejected: %v", err)
	}
	for _, from := range []Status{StatusBacklog, StatusBlocked} {
		if err := ValidateCloseTransition(from, StatusDone); err == nil {
			t.Fatalf("invalid close transition %s -> done was accepted", from)
		}
	}
}

func validManifest(
	t *testing.T,
	scope ScopeLayout,
	activeProfile profile.Profile,
) Manifest {
	t.Helper()

	manifest, err := NewManifest(
		"550e8400-e29b-41d4-a716-446655440000",
		scope,
		"TASK-00039",
		"Design AI Dev Brain v3",
		"design-ai-dev-brain-v3",
		"design-ai-dev-brain-v3",
		"feat",
		activeProfile,
		time.Date(2026, time.September, 10, 13, 0, 0, 0, time.UTC),
		Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	if err != nil {
		t.Fatalf("new valid manifest: %v", err)
	}
	return manifest
}

func testOrganizationTicketLayout(t *testing.T) (ScopeLayout, Layout) {
	t.Helper()

	organizationLayout, organizationManifest := testOrganization(t)
	scope, err := NewOrganizationScope(
		organizationLayout,
		organizationManifest,
	)
	if err != nil {
		t.Fatalf("new organization ticket scope: %v", err)
	}
	layout, err := scope.Ticket("TASK-00039", "design-ai-dev-brain-v3")
	if err != nil {
		t.Fatalf("new ticket layout: %v", err)
	}
	return scope, layout
}

func testBuiltinProfile(t *testing.T) profile.Profile {
	t.Helper()

	value, err := profile.BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}
	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
