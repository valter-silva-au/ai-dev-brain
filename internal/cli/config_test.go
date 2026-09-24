package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestConfigTier_WiredThroughNewApp proves the org tier is resolved end-to-end
// through the App constructor: a .taskrc names an org, orgs/<id>/config.yaml
// supplies a value, and the repo tier overrides another — with precedence
// Repo > Org > Global surfaced via App.MergedConfig.SettingSource (what
// `adb config get` uses).
func TestConfigTier_WiredThroughNewApp(t *testing.T) {
	// Load-bearing: this test's fixture IS the org selector, so both ambient
	// inputs to tier resolution must be out of the picture.
	//   - $ADB_ORG outranks .taskrc's `org:` field for an un-isolated App
	//     (core/config.go LoadConfig reads it first), so an exported ADB_ORG
	//     selects a DIFFERENT org and the assertion below fails on a missing
	//     orgs/<that-org>/config.yaml: `ADB_ORG=hostile … → expected org tier
	//     acme wired through NewApp, got <nil>`.
	//   - $HOME/.taskconfig is the GLOBAL tier for an un-isolated App, so a real
	//     one there contributes custom_settings the deploy_target/team precedence
	//     assertions must not see.
	//
	// NewAppIsolated drops $ADB_ORG and repoints the global tier at
	// <tmp>/.taskconfig (absent here). Crucially it does NOT drop .taskrc's `org:`
	// field — that is workspace data, not ambient state — which is precisely what
	// this test asserts: the org tier below resolves from the fixture.
	tmp := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmp, ".taskrc"),
		[]byte("repo_name: acme-web\norg: acme\ncustom_settings:\n  deploy_target: repo-staging\n"), 0o644); err != nil {
		t.Fatalf("write .taskrc: %v", err)
	}
	orgDir := filepath.Join(tmp, "orgs", "acme")
	if err := os.MkdirAll(orgDir, 0o755); err != nil {
		t.Fatalf("mkdir org: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orgDir, "config.yaml"),
		[]byte("custom_settings:\n  deploy_target: org-prod\n  team: platform\n"), 0o644); err != nil {
		t.Fatalf("write org config: %v", err)
	}

	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	defer app.Cleanup()

	mc := app.MergedConfig
	if mc.Org == nil || mc.Org.OrgID != "acme" {
		t.Fatalf("expected org tier acme wired through NewApp, got %+v", mc.Org)
	}
	// Repo overrides org for the same key.
	if v, tier, ok := mc.SettingSource("deploy_target"); !ok || v != "repo-staging" || tier != "repo" {
		t.Errorf("deploy_target = (%q,%q,%v), want (repo-staging,repo,true)", v, tier, ok)
	}
	// Org-only key resolves from the org tier.
	if v, tier, ok := mc.SettingSource("team"); !ok || v != "platform" || tier != "org" {
		t.Errorf("team = (%q,%q,%v), want (platform,org,true)", v, tier, ok)
	}
}
