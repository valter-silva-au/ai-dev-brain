package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
)

func TestBuildPlugin_EmitsManifestsAndHarness(t *testing.T) {
	dir := t.TempDir()
	res, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{Version: "1.2.3"})
	if err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	if res.Plugin.Version != "1.2.3" || res.Plugin.Name != PluginName {
		t.Errorf("plugin manifest = %+v", res.Plugin)
	}

	// plugin.json is valid and carries the identity.
	var pm models.PluginManifest
	readJSON(t, filepath.Join(dir, ".claude-plugin", "plugin.json"), &pm)
	if pm.Name != PluginName || pm.Version != "1.2.3" || pm.Author.Name == "" || pm.Description == "" {
		t.Errorf("plugin.json = %+v", pm)
	}

	// marketplace.json lists the plugin at source "./".
	var mk models.MarketplaceManifest
	readJSON(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"), &mk)
	if len(mk.Plugins) != 1 || mk.Plugins[0].Name != PluginName || mk.Plugins[0].Source != "./" {
		t.Errorf("marketplace.json = %+v", mk)
	}

	// .mcp.json registers the adb MCP server.
	var mcp models.MCPConfig
	readJSON(t, filepath.Join(dir, ".mcp.json"), &mcp)
	adb, ok := mcp.MCPServers["adb"]
	if !ok || adb.Command != "adb" || len(adb.Args) != 2 || adb.Args[0] != "mcp" || adb.Args[1] != "serve" {
		t.Errorf(".mcp.json = %+v", mcp)
	}

	// Every embedded harness file was copied under agents/ or skills/.
	harness, err := HarnessManifest(claude.FS)
	if err != nil {
		t.Fatalf("HarnessManifest: %v", err)
	}
	if len(harness) == 0 {
		t.Fatal("expected embedded harness files")
	}
	for _, f := range harness {
		sub := "agents"
		if f.Kind == HarnessSkill {
			sub = "skills"
		}
		p := filepath.Join(dir, sub, filepath.FromSlash(f.RelPath))
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Errorf("harness file not copied: %s (%v)", p, rerr)
			continue
		}
		if string(data) != string(f.Content) {
			t.Errorf("copied harness file %s content mismatch", p)
		}
	}

	// The devils-advocate agent specifically landed.
	if _, err := os.Stat(filepath.Join(dir, "agents", "devils-advocate.md")); err != nil {
		t.Errorf("expected agents/devils-advocate.md: %v", err)
	}
}

func TestBuildPlugin_DefaultVersionAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	res, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{}) // no version
	if err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	if res.Plugin.Version != DefaultPluginVersion {
		t.Errorf("default version = %q, want %q", res.Plugin.Version, DefaultPluginVersion)
	}

	// Re-build is idempotent: every entry unchanged.
	res2, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{})
	if err != nil {
		t.Fatalf("re-build: %v", err)
	}
	for _, e := range res2.Entries {
		if e.Action != HarnessUnchanged {
			t.Errorf("re-build %s = %s, want unchanged", e.Path, e.Action)
		}
	}
}

func TestBuildPlugin_ForceOverwritesEditedFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{}); err != nil {
		t.Fatalf("BuildPlugin: %v", err)
	}
	pj := filepath.Join(dir, ".claude-plugin", "plugin.json")
	if err := os.WriteFile(pj, []byte(`{"name":"tampered"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without --force, a user-edited file is skipped (not clobbered).
	res, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{})
	if err != nil {
		t.Fatalf("re-build: %v", err)
	}
	if pluginActionFor(res, ".claude-plugin/plugin.json") != HarnessSkipped {
		t.Errorf("edited plugin.json should be skipped without --force, got %s", pluginActionFor(res, ".claude-plugin/plugin.json"))
	}

	// With --force, it is overwritten back to the canonical manifest.
	res, err = BuildPlugin(claude.FS, dir, PluginBuildOptions{Force: true})
	if err != nil {
		t.Fatalf("force re-build: %v", err)
	}
	if pluginActionFor(res, ".claude-plugin/plugin.json") != HarnessInstalled {
		t.Errorf("--force should overwrite plugin.json, got %s", pluginActionFor(res, ".claude-plugin/plugin.json"))
	}
	var pm models.PluginManifest
	readJSON(t, pj, &pm)
	if pm.Name != PluginName {
		t.Errorf("plugin.json not restored: name=%q", pm.Name)
	}
}

// pluginActionFor returns the recorded action for a plugin-relative path.
func pluginActionFor(res PluginBuildResult, rel string) HarnessInstallAction {
	for _, e := range res.Entries {
		if e.Path == rel {
			return e.Action
		}
	}
	return ""
}

func TestBuildPlugin_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	if _, err := BuildPlugin(claude.FS, dir, PluginBuildOptions{DryRun: true}); err != nil {
		t.Fatalf("BuildPlugin dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "plugin.json")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not write plugin.json (stat err=%v)", err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
