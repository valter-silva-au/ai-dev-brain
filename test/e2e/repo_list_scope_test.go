package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestE2E_RepoListOrgIsOptional pins the workspace-wide repository inventory
// against the real binary: a caller who does not yet know the organization
// names must still be able to ask what the workspace holds.
func TestE2E_RepoListOrgIsOptional(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	workspace := filepath.Join(parent, "AWS")
	initializeV3Workspace(t, parent, workspace)
	initializeV3Organization(t, parent, workspace, "amazon", "Amazon")
	initializeV3Organization(t, parent, workspace, "zeta-corp", "Zeta Corp")

	// No --org: must succeed rather than demand the flag.
	res := mustRunADB(
		t, workspace,
		"repo", "list", "--workspace", workspace, "--format", "json",
	)
	var result struct {
		Outcome string `json:"outcome"`
		Data    struct {
			OrganizationID string `json:"organization_id"`
			Repositories   []struct {
				OrganizationID   string `json:"organization_id"`
				OrganizationSlug string `json:"organization_slug"`
			} `json:"repositories"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &result); err != nil {
		t.Fatalf("decode repo list JSON: %v\nstdout:\n%s", err, res.stdout)
	}
	if result.Outcome != "healthy" {
		t.Fatalf("repo list outcome = %q, want healthy", result.Outcome)
	}
	// Two organizations, neither holding a repository: the answer is "none",
	// and the envelope must not claim it was scoped to one of them.
	if len(result.Data.Repositories) != 0 {
		t.Fatalf("repositories = %#v, want none", result.Data.Repositories)
	}
	if result.Data.OrganizationID != "" {
		t.Fatalf(
			"workspace-wide list reported scope %q",
			result.Data.OrganizationID,
		)
	}

	// Naming an org must still scope the read, and say so.
	scoped := mustRunADB(
		t, workspace,
		"repo", "list",
		"--workspace", workspace,
		"--org", "amazon",
		"--format", "json",
	)
	if err := json.Unmarshal([]byte(scoped.stdout), &result); err != nil {
		t.Fatalf("decode scoped repo list JSON: %v\n%s", err, scoped.stdout)
	}
	if result.Data.OrganizationID == "" {
		t.Fatal("scoped repo list lost its organization scope")
	}
}
