package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_FounderPlaybookIsReachableFromAFreshWorkspace is the regression test
// for a defect that only the real binary could show.
//
// `adb initiative` reads the playbook registry (orgs/index.yaml). The composed
// v3 root registers the legacy command tree with IncludeOrg:false, because the
// v3 `adb org` supersedes the legacy one — but the v3 `adb org` writes the
// .aidb trust-scope registry, a different store. The command that wrote
// orgs/index.yaml went with the legacy tree, so on a fresh workspace:
//
//	$ adb initiative create "Widget" --org acme
//	Error: failed to create initiative: organization "acme" not found
//
// and no public command could create that org. `adb initiative`, `adb stage`,
// and the whole Idea→MVP→Launch→Scale gate system were unreachable.
//
// The in-process cli test for this path passes either way, because it builds
// the LEGACY root directly (IncludeOrg:true). Only the shipped binary composes
// the root the user actually gets, so the test has to be here.
func TestE2E_FounderPlaybookIsReachableFromAFreshWorkspace(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	mustRunADB(t, workspace, "init", "workspace", workspace)

	// 1. The org must be creatable at all — this is the step that was missing.
	create := mustRunADB(
		t, workspace,
		"org", "create", "Acme Robotics", "--git-host", "github.com", "--json",
	)
	var org struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		GitHost string `json:"git_host"`
	}
	if err := json.Unmarshal([]byte(create.stdout), &org); err != nil {
		t.Fatalf("decode org create JSON: %v\nstdout:\n%s", err, create.stdout)
	}
	if org.ID == "" {
		t.Fatalf("org create returned no id:\n%s", create.stdout)
	}

	// 2. The org must be visible to a later, separate invocation — the registry
	//    is what `initiative` reads, so an in-memory-only create would not do.
	//
	//    Note this is `catalog show`, NOT `org list`: in the composed root
	//    `adb org list` is the v3 trust-scope reader and will never show a
	//    playbook org. `adb catalog show --kind orgs` is the listing path for
	//    these, which is why the `adb org` help points at it.
	catalog := mustRunADB(t, workspace, "catalog", "show", "--kind", "orgs")
	if !strings.Contains(catalog.combined(), org.ID) {
		t.Fatalf(
			"catalog show --kind orgs omits %q:\n%s",
			org.ID,
			catalog.combined(),
		)
	}

	// 3. The step that used to fail.
	mustRunADB(
		t, workspace,
		"initiative", "create", "Widget Launcher", "--org", org.ID,
	)

	show := mustRunADB(t, workspace, "initiative", "show", "widget-launcher")
	if !strings.Contains(show.combined(), "Idea") {
		t.Fatalf("new initiative is not at stage Idea:\n%s", show.combined())
	}

	// 4. The gate system must be reachable, not merely the registry. A fresh
	//    initiative has no evidence, so Idea→MVP is expected to be REFUSED —
	//    that refusal is the proof the gate ran. A missing-org error here would
	//    mean the playbook is still unreachable.
	advance := runADB(t, workspace, "stage", "advance", "widget-launcher")
	combined := advance.combined()
	if strings.Contains(combined, "not found") {
		t.Fatalf("stage advance cannot resolve the initiative:\n%s", combined)
	}
	if advance.err == nil {
		t.Fatalf(
			"stage advance succeeded with no evidence; the gate did not run:\n%s",
			combined,
		)
	}
}

// TestE2E_OrgCreateAndOrgInitWriteDistinctRegistries pins the reason both verbs
// exist on one noun. They are not duplicates: `init` registers a v3 .aidb trust
// scope, `create` registers a playbook organization. Collapsing them is a
// deliberate decision to take later, not something to do by accident — and if
// someone does unify them, this test should be the thing that objects.
func TestE2E_OrgCreateAndOrgInitWriteDistinctRegistries(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	workspace := filepath.Join(parent, "AWS")

	// Both boundaries, in one workspace: a real .aidb boundary (so the v3 org
	// surface genuinely works and cannot pass this test by erroring out) and the
	// task workspace the playbook registry lives in.
	initializeV3Workspace(t, parent, workspace)
	mustRunADB(t, workspace, "init", "workspace", workspace)

	mustRunADB(t, workspace, "org", "create", "Acme", "--json")
	mustRunADB(t, workspace, "initiative", "create", "Widget", "--org", "acme")

	// The v3 trust-scope reader must succeed here — a skipped assertion is the
	// failure mode this guards against — and must not report the playbook org.
	list := mustRunADB(t, workspace, "org", "list", "--workspace", workspace)
	if strings.Contains(list.stdout, "acme") {
		t.Fatalf(
			"v3 org list reported the playbook org as a trust scope:\n%s",
			list.combined(),
		)
	}

	// Symmetrically, a v3 trust scope is not a playbook org: an initiative
	// cannot reference one, so the two stores stay legibly separate.
	initializeV3Organization(t, parent, workspace, "zeta", "Zeta")
	res := runADB(
		t, workspace,
		"initiative", "create", "Gadget", "--org", "zeta",
	)
	if res.err == nil {
		t.Fatalf(
			"initiative accepted a v3 trust scope as its org:\n%s",
			res.combined(),
		)
	}
	if !strings.Contains(res.combined(), "not found") {
		t.Fatalf(
			"initiative rejected the v3 trust scope for an unexpected reason:\n%s",
			res.combined(),
		)
	}
}
