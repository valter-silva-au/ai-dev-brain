package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type organizationView struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Path        string   `json:"path"`
	Aliases     []string `json:"aliases"`
}

type organizationResult struct {
	Capability string `json:"capability"`
	Version    string `json:"version"`
	Outcome    string `json:"outcome"`
	Data       struct {
		OrganizationID string             `json:"organization_id"`
		OperationID    string             `json:"operation_id"`
		Slug           string             `json:"slug"`
		Path           string             `json:"path"`
		Organization   organizationView   `json:"organization"`
		Organizations  []organizationView `json:"organizations"`
		PreviousPath   string             `json:"previous_path"`
	} `json:"data"`
}

func TestE2E_V3OrganizationLifecycleAndLegacyCompatibility(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "AWS")
	initializeV3Workspace(t, parent, root)

	planned := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"init",
			"amazon",
			"--workspace",
			root,
			"--name",
			"Amazon",
			"--format",
			"json",
		).stdout,
	)
	if planned.Capability != "organization.initialize" ||
		planned.Version != "v1" ||
		planned.Outcome != "planned" {
		t.Fatalf("unexpected organization plan: %#v", planned)
	}
	if _, err := os.Stat(planned.Data.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("organization plan created %q: %v", planned.Data.Path, err)
	}

	applied := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"init",
			"amazon",
			"--workspace",
			root,
			"--name",
			"Amazon",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if applied.Outcome != "applied" ||
		applied.Data.OrganizationID == "" ||
		applied.Data.OperationID == "" {
		t.Fatalf("unexpected organization apply: %#v", applied)
	}
	organizationID := applied.Data.OrganizationID

	beforeReads := snapshotTreeHashes(t, root)
	listed := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"list",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	shown := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"show",
			"amazon",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	afterReads := snapshotTreeHashes(t, root)
	if !reflect.DeepEqual(beforeReads, afterReads) {
		t.Fatalf(
			"organization list/show mutated workspace\nbefore=%v\nafter=%v",
			beforeReads,
			afterReads,
		)
	}
	if len(listed.Data.Organizations) != 1 ||
		listed.Data.Organizations[0].ID != organizationID ||
		shown.Data.Organization.ID != organizationID {
		t.Fatalf("organization reads lost identity: list=%#v show=%#v", listed, shown)
	}

	movePlan := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"move",
			"amazon",
			"--workspace",
			root,
			"--slug",
			"amazon-web-services",
			"--format",
			"json",
		).stdout,
	)
	if movePlan.Outcome != "planned" {
		t.Fatalf("organization move plan = %#v", movePlan)
	}
	moved := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"move",
			"amazon",
			"--workspace",
			root,
			"--slug",
			"amazon-web-services",
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if moved.Outcome != "applied" ||
		moved.Data.Organization.ID != organizationID ||
		moved.Data.Organization.Slug != "amazon-web-services" {
		t.Fatalf("organization move lost identity: %#v", moved)
	}
	aliasView := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"show",
			"amazon",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	if aliasView.Data.Organization.ID != organizationID {
		t.Fatalf("old organization selector did not resolve: %#v", aliasView)
	}

	archivePlan := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"archive",
			"amazon-web-services",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	if archivePlan.Outcome != "planned" {
		t.Fatalf("organization archive plan = %#v", archivePlan)
	}
	archived := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"archive",
			"amazon-web-services",
			"--workspace",
			root,
			"--apply",
			"--format",
			"json",
		).stdout,
	)
	if archived.Outcome != "applied" ||
		archived.Data.Organization.ID != organizationID ||
		archived.Data.Organization.Status != "archived" {
		t.Fatalf("organization archive lost identity: %#v", archived)
	}
	activeOnly := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"list",
			"--workspace",
			root,
			"--format",
			"json",
		).stdout,
	)
	if len(activeOnly.Data.Organizations) != 0 {
		t.Fatalf("active organization list includes archived entry: %#v", activeOnly)
	}
	withArchived := decodeSingleJSON[organizationResult](
		t,
		mustRunADB(
			t,
			parent,
			"org",
			"list",
			"--workspace",
			root,
			"--include-archived",
			"--format",
			"json",
		).stdout,
	)
	if len(withArchived.Data.Organizations) != 1 ||
		withArchived.Data.Organizations[0].ID != organizationID {
		t.Fatalf("archived organization is not queryable: %#v", withArchived)
	}

	legacy := mustRunADB(t, parent, "task", "status", "--json")
	if legacy.stdout != "[]\n" {
		t.Fatalf("legacy task status output = %q, want empty JSON array", legacy.stdout)
	}
}
