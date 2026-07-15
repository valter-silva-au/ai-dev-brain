package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
)

// TestConfigTier_WiredThroughNewApp proves the org tier is resolved end-to-end
// through internal.NewApp: a .taskrc names an org, orgs/<id>/config.yaml supplies
// a value, and the repo tier overrides another — with precedence Repo > Org >
// Global surfaced via App.MergedConfig.SettingSource (what `adb config get` uses).
func TestConfigTier_WiredThroughNewApp(t *testing.T) {
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

	app, err := internal.NewApp(tmp)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
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
