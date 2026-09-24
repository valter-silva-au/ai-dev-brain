package foundation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
	"github.com/valter-silva-au/ai-dev-brain/internal/journal"
	"github.com/valter-silva-au/ai-dev-brain/internal/workspace"
)

const approvedRoleEscapeRemediation = "Preserve and review the role's existing content, then replace " +
	"the symbolic link with a real directory beneath the workspace."

func TestDoctorHealthyWorkspaceIsReadOnly(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize healthy workspace: %v", err)
	}

	before := snapshotFiles(t, root)
	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor healthy workspace: %v", err)
	}
	if result.Outcome != capability.OutcomeHealthy {
		t.Fatalf("outcome = %q, want healthy", result.Outcome)
	}
	if result.Data.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace id = %q, want workspace-1", result.Data.WorkspaceID)
	}
	if len(result.Data.Findings) != 0 {
		t.Fatalf("healthy findings = %#v", result.Data.Findings)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("doctor mutated workspace\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestDoctorReportsMissingBoundaryWithoutCreatingIt(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "missing")
	service := newTestService(t, nil)
	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor missing workspace: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention {
		t.Fatalf("outcome = %q, want needs_attention", result.Outcome)
	}
	assertFinding(t, result.Data.Findings, "workspace.boundary.missing", "error")
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("doctor created missing workspace: %v", err)
	}
}

func TestDoctorReportsLexicallyEscapedManifestRole(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create control directory: %v", err)
	}
	manifest := workspace.NewManifest(
		"workspace-1",
		"AWS",
		time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	)
	manifest.Roles.Events = "../outside"
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode invalid manifest: %v", err)
	}
	if err := os.WriteFile(layout.ManifestPath(), encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}

	service := newTestService(t, nil)
	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor invalid manifest: %v", err)
	}
	assertFinding(t, result.Data.Findings, "workspace.role.escaped", "error")
	assertNoFinding(t, result.Data.Findings, "workspace.manifest.invalid")
}

func TestDoctorReportsEscapedRolesInStableOrderWithoutFollowingThem(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	external := filepath.Join(t.TempDir(), "external")
	if err := os.MkdirAll(filepath.Join(external, "events"), 0o755); err != nil {
		t.Fatalf("create external events: %v", err)
	}
	corruptOperation := filepath.Join(external, "events", "operation-corrupt")
	if err := os.MkdirAll(corruptOperation, 0o755); err != nil {
		t.Fatalf("create corrupt external operation: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(corruptOperation, "plan.json"),
		[]byte("{not-json"),
		0o644,
	); err != nil {
		t.Fatalf("write corrupt external journal plan: %v", err)
	}
	for path, content := range map[string]string{
		"config.yaml":  "external: true\n",
		"state.sqlite": "not sqlite",
		"marker.txt":   "preserve me",
	} {
		target := filepath.Join(external, filepath.FromSlash(path))
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatalf("write external fixture %q: %v", target, err)
		}
	}
	if err := os.Symlink(external, filepath.Join(root, "managed")); err != nil {
		t.Fatalf("create escaped managed role: %v", err)
	}

	roles := workspace.Roles{
		Config:        "managed/config.yaml",
		State:         "managed/state.sqlite",
		Events:        "managed/events",
		Cache:         "managed/cache",
		Organizations: "managed/organizations",
	}
	rewriteWorkspaceManifest(t, root, func(manifest *workspace.Manifest) {
		manifest.Roles = roles
	})

	beforeWorkspace := snapshotTreeWithoutFollowingLinks(t, root)
	beforeExternal := snapshotTreeWithoutFollowingLinks(t, external)
	result, err := service.Doctor(
		context.Background(),
		DoctorRequest{Root: root},
	)
	if err != nil {
		t.Fatalf("doctor escaped workspace: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention {
		t.Fatalf("outcome = %q, want needs_attention", result.Outcome)
	}
	if !result.Recovery.Required {
		t.Fatal("escaped role must require recovery")
	}

	wantRoles := []struct {
		name string
		path string
	}{
		{name: "config", path: roles.Config},
		{name: "state", path: roles.State},
		{name: "events", path: roles.Events},
		{name: "cache", path: roles.Cache},
		{name: "organizations", path: roles.Organizations},
	}
	if len(result.Data.Findings) != len(wantRoles) {
		t.Fatalf("findings = %#v, want one escaped finding per role", result.Data.Findings)
	}
	for index, want := range wantRoles {
		finding := result.Data.Findings[index]
		if finding.ID != "workspace.role.escaped" ||
			finding.Severity != "error" {
			t.Fatalf("finding[%d] = %#v, want workspace.role.escaped/error", index, finding)
		}
		evidence := strings.Join(finding.Evidence, "\n")
		for _, expected := range []string{
			"role=" + want.name,
			"declared=" + want.path,
			"component=managed",
			"reason=symlink",
		} {
			if !strings.Contains(evidence, expected) {
				t.Fatalf("finding[%d] evidence missing %q: %#v", index, expected, finding.Evidence)
			}
		}
		if finding.Remediation != approvedRoleEscapeRemediation {
			t.Fatalf(
				"finding[%d] remediation = %q, want %q",
				index,
				finding.Remediation,
				approvedRoleEscapeRemediation,
			)
		}
	}
	for _, finding := range result.Data.Findings {
		if strings.HasPrefix(finding.ID, "operation.journal.") {
			t.Fatalf("doctor followed escaped events role: %#v", finding)
		}
	}
	afterWorkspace := snapshotTreeWithoutFollowingLinks(t, root)
	if !reflect.DeepEqual(afterWorkspace, beforeWorkspace) {
		t.Fatalf(
			"doctor mutated escaped workspace\nbefore=%#v\nafter=%#v",
			beforeWorkspace,
			afterWorkspace,
		)
	}
	afterExternal := snapshotTreeWithoutFollowingLinks(t, external)
	if !reflect.DeepEqual(afterExternal, beforeExternal) {
		t.Fatalf(
			"doctor mutated escaped target\nbefore=%#v\nafter=%#v",
			beforeExternal,
			afterExternal,
		)
	}
}

func TestDoctorReportsUnsafeRoleReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		role    string
		reason  string
		prepare func(*testing.T, string)
	}{
		{
			name:   "in-root symlink",
			role:   "linked/organizations",
			reason: "symlink",
			prepare: func(t *testing.T, root string) {
				t.Helper()
				if err := os.Symlink(
					filepath.Join(root, ".aidb"),
					filepath.Join(root, "linked"),
				); err != nil {
					t.Fatalf("create in-root role symlink: %v", err)
				}
			},
		},
		{
			name:    "lexical escape",
			role:    "../outside",
			reason:  "lexical_escape",
			prepare: func(*testing.T, string) {},
		},
		{
			name:   "uninspectable component",
			role:   "blocked/organizations",
			reason: "uninspectable",
			prepare: func(t *testing.T, root string) {
				t.Helper()
				if err := os.WriteFile(
					filepath.Join(root, "blocked"),
					[]byte("not a directory"),
					0o644,
				); err != nil {
					t.Fatalf("write blocking component: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(t.TempDir(), "AWS")
			service := newTestService(t, nil)
			if _, err := service.Initialize(
				context.Background(),
				InitializeRequest{Root: root, Name: "AWS", Apply: true},
			); err != nil {
				t.Fatalf("initialize workspace: %v", err)
			}
			test.prepare(t, root)
			rewriteWorkspaceManifest(t, root, func(manifest *workspace.Manifest) {
				manifest.Roles.Organizations = test.role
			})

			result, err := service.Doctor(
				context.Background(),
				DoctorRequest{Root: root},
			)
			if err != nil {
				t.Fatalf("doctor unsafe role: %v", err)
			}
			finding := findFinding(t, result.Data.Findings, "workspace.role.escaped")
			evidence := strings.Join(finding.Evidence, "\n")
			for _, expected := range []string{
				"role=organizations",
				"declared=" + test.role,
				"reason=" + test.reason,
			} {
				if !strings.Contains(evidence, expected) {
					t.Fatalf("evidence missing %q: %#v", expected, finding.Evidence)
				}
			}
			assertNoFinding(t, result.Data.Findings, "workspace.manifest.invalid")
		})
	}
}

func TestDoctorUsesDeclaredRolePaths(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	roles := workspace.Roles{
		Config:        "portable/config.yaml",
		State:         "derived/state.sqlite",
		Events:        "history/events",
		Cache:         "derived/cache",
		Organizations: "catalog/organizations",
	}
	for source, role := range map[string]string{
		layout.ConfigPath():       roles.Config,
		layout.StatePath():        roles.State,
		layout.EventsDir():        roles.Events,
		layout.CacheDir():         roles.Cache,
		layout.OrganizationsDir(): roles.Organizations,
	} {
		target := filepath.Join(root, filepath.FromSlash(role))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("create declared role parent: %v", err)
		}
		if err := os.Rename(source, target); err != nil {
			t.Fatalf("move %q to declared role %q: %v", source, target, err)
		}
	}
	rewriteWorkspaceManifest(t, root, func(manifest *workspace.Manifest) {
		manifest.Roles = roles
	})

	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor custom roles: %v", err)
	}
	assertFinding(t, result.Data.Findings, "control_plane.projection.stale", "warning")
	for _, id := range []string{
		"workspace.role.escaped",
		"workspace.config.missing",
		"workspace.config.unreadable",
		"control_plane.missing",
		"control_plane.invalid",
		"operation.journal.missing",
		"operation.journal.unreadable",
	} {
		assertNoFinding(t, result.Data.Findings, id)
	}
}

func TestDoctorReportsMalformedOrUnsupportedManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed",
			content: "schema_version: [",
		},
		{
			name: "unsupported schema",
			content: `schema_version: aidb.workspace/v99
kind: Workspace
id: workspace-1
name: AWS
created_at: 2026-09-10T08:00:00Z
roles:
  config: .aidb/config.yaml
  state: .aidb/state.sqlite
  events: .aidb/events
  cache: .aidb/cache
  organizations: organizations
`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(t.TempDir(), "AWS")
			layout, err := workspace.NewLayout(root)
			if err != nil {
				t.Fatalf("new layout: %v", err)
			}
			if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
				t.Fatalf("create control directory: %v", err)
			}
			if err := os.WriteFile(
				layout.ManifestPath(),
				[]byte(test.content),
				0o644,
			); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			service := newTestService(t, nil)
			result, err := service.Doctor(
				context.Background(),
				DoctorRequest{Root: root},
			)
			if err != nil {
				t.Fatalf("doctor invalid manifest: %v", err)
			}
			assertFinding(
				t,
				result.Data.Findings,
				"workspace.manifest.invalid",
				"error",
			)
		})
	}
}

func TestDoctorReportsCorruptControlPlane(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	if err := os.MkdirAll(layout.ControlDir(), 0o755); err != nil {
		t.Fatalf("create control directory: %v", err)
	}
	manifest := workspace.NewManifest(
		"workspace-1",
		"AWS",
		time.Date(2026, time.September, 10, 8, 0, 0, 0, time.UTC),
	)
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(layout.ManifestPath(), encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(layout.StatePath(), []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt control plane: %v", err)
	}

	service := newTestService(t, nil)
	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor corrupt control plane: %v", err)
	}
	assertFinding(t, result.Data.Findings, "control_plane.invalid", "error")
}

func TestDoctorReportsMissingControlPlane(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	if err := os.Remove(layout.StatePath()); err != nil {
		t.Fatalf("remove control plane: %v", err)
	}

	result, err := service.Doctor(
		context.Background(),
		DoctorRequest{Root: root},
	)
	if err != nil {
		t.Fatalf("doctor missing control plane: %v", err)
	}
	assertFinding(t, result.Data.Findings, "control_plane.missing", "error")
}

func TestDoctorReportsControlPlaneSchemaMismatch(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	database, err := sql.Open("sqlite", layout.StatePath())
	if err != nil {
		t.Fatalf("open control plane: %v", err)
	}
	if _, err := database.Exec("PRAGMA user_version = 99"); err != nil {
		_ = database.Close()
		t.Fatalf("change control-plane schema version: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close control plane: %v", err)
	}

	result, err := service.Doctor(
		context.Background(),
		DoctorRequest{Root: root},
	)
	if err != nil {
		t.Fatalf("doctor schema mismatch: %v", err)
	}
	assertFinding(t, result.Data.Findings, "control_plane.invalid", "error")
}

func TestDoctorReportsIncompleteJournalOperation(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	journalStore, err := journal.NewStore(
		layout.EventsDir(),
		func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				9,
				0,
				0,
				0,
				time.UTC,
			)
		},
		func() string { return "later-event" },
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	if _, err := journalStore.Begin(journal.Plan{
		OperationID:    "operation-incomplete",
		IdempotencyKey: "test:incomplete",
		Kind:           "test",
	}); err != nil {
		t.Fatalf("begin incomplete operation: %v", err)
	}
	if _, err := journalStore.Append(
		"operation-incomplete",
		journal.EventInput{Phase: journal.PhaseApplying},
	); err != nil {
		t.Fatalf("append incomplete operation: %v", err)
	}

	result, err := service.Doctor(context.Background(), DoctorRequest{Root: root})
	if err != nil {
		t.Fatalf("doctor incomplete operation: %v", err)
	}
	if result.Outcome != capability.OutcomeAttention {
		t.Fatalf("outcome = %q, want needs_attention", result.Outcome)
	}
	assertFinding(
		t,
		result.Data.Findings,
		"operation.journal.incomplete",
		"warning",
	)
}

func TestDoctorReportsAmbiguousJournalOperation(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	service := newTestService(t, nil)
	if _, err := service.Initialize(context.Background(), InitializeRequest{
		Root:  root,
		Name:  "AWS",
		Apply: true,
	}); err != nil {
		t.Fatalf("initialize workspace: %v", err)
	}

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	target := filepath.Join(root, "ambiguous.txt")
	if err := os.WriteFile(target, []byte("unexpected"), 0o644); err != nil {
		t.Fatalf("write ambiguous target: %v", err)
	}
	journalStore, err := journal.NewStore(
		layout.EventsDir(),
		func() time.Time {
			return time.Date(
				2026,
				time.September,
				10,
				9,
				0,
				0,
				0,
				time.UTC,
			)
		},
		func() string { return "ambiguous-event" },
	)
	if err != nil {
		t.Fatalf("new journal store: %v", err)
	}
	if _, err := journalStore.Begin(journal.Plan{
		OperationID:    "operation-ambiguous",
		IdempotencyKey: "test:ambiguous",
		Kind:           "test",
		Steps: []journal.Step{{
			Ordinal:    1,
			Action:     "replace",
			Target:     target,
			BeforeHash: journal.Digest([]byte("before")),
			AfterHash:  journal.Digest([]byte("after")),
		}},
	}); err != nil {
		t.Fatalf("begin ambiguous operation: %v", err)
	}
	if _, err := journalStore.Append(
		"operation-ambiguous",
		journal.EventInput{Phase: journal.PhaseApplying},
	); err != nil {
		t.Fatalf("append applying event: %v", err)
	}
	if _, err := journalStore.Append(
		"operation-ambiguous",
		journal.EventInput{
			Phase: journal.PhaseStepApplying,
			Step:  1,
		},
	); err != nil {
		t.Fatalf("append step applying event: %v", err)
	}

	result, err := service.Doctor(
		context.Background(),
		DoctorRequest{Root: root},
	)
	if err != nil {
		t.Fatalf("doctor ambiguous journal: %v", err)
	}
	assertFinding(
		t,
		result.Data.Findings,
		"operation.journal.attention",
		"error",
	)
}

func assertFinding(
	t *testing.T,
	findings []Finding,
	id string,
	severity string,
) {
	t.Helper()

	for _, finding := range findings {
		if finding.ID != id {
			continue
		}
		if finding.Severity != severity {
			t.Fatalf(
				"finding %q severity = %q, want %q",
				id,
				finding.Severity,
				severity,
			)
		}
		if finding.Summary == "" ||
			len(finding.Evidence) == 0 ||
			finding.Remediation == "" {
			t.Fatalf("finding %q is incomplete: %#v", id, finding)
		}
		return
	}
	t.Fatalf("finding %q not present in %#v", id, findings)
}

func findFinding(
	t *testing.T,
	findings []Finding,
	id string,
) Finding {
	t.Helper()

	for _, finding := range findings {
		if finding.ID == id {
			return finding
		}
	}
	t.Fatalf("finding %q not present in %#v", id, findings)
	return Finding{}
}

func assertNoFinding(
	t *testing.T,
	findings []Finding,
	id string,
) {
	t.Helper()

	for _, finding := range findings {
		if finding.ID == id {
			t.Fatalf("unexpected finding %q in %#v", id, findings)
		}
	}
}

func rewriteWorkspaceManifest(
	t *testing.T,
	root string,
	mutate func(*workspace.Manifest),
) {
	t.Helper()

	layout, err := workspace.NewLayout(root)
	if err != nil {
		t.Fatalf("new layout: %v", err)
	}
	manifest, err := workspace.ReadManifest(layout.ManifestPath())
	if err != nil {
		t.Fatalf("read workspace manifest: %v", err)
	}
	mutate(&manifest)
	var encoded bytes.Buffer
	if err := workspace.EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode workspace manifest: %v", err)
	}
	if err := os.WriteFile(layout.ManifestPath(), encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("rewrite workspace manifest: %v", err)
	}
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(content)
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			snapshot[relative] = hex.EncodeToString(digest[:])
			return nil
		},
	)
	if err != nil {
		t.Fatalf("snapshot workspace files: %v", err)
	}
	return snapshot
}

func snapshotTreeWithoutFollowingLinks(
	t *testing.T,
	root string,
) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)
	if err := filepath.WalkDir(
		root,
		func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				snapshot[relative] = "symlink->" + target
				return nil
			}
			if entry.IsDir() {
				snapshot[relative+"/"] = "directory"
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(content)
			snapshot[relative] = hex.EncodeToString(digest[:])
			return nil
		},
	); err != nil {
		t.Fatalf("snapshot tree without following links: %v", err)
	}
	return snapshot
}
