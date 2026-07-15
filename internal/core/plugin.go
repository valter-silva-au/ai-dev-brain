package core

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file graduates the adb harness (the embedded agents + skills enumerated by
// HarnessManifest) into a distributable Claude Code PLUGIN + MARKETPLACE (#139
// step 20, D12 phase 2). BuildPlugin emits a directory that is both an installable
// plugin (`.claude-plugin/plugin.json` + agents/ + skills/ + `.mcp.json`) and a
// single-plugin marketplace (`.claude-plugin/marketplace.json`).

const (
	// PluginName is the plugin's id in plugin.json / marketplace.json.
	PluginName = "adb-founder-playbook"
	// DefaultPluginVersion is used when no --version is supplied.
	DefaultPluginVersion = "0.1.0"
	pluginDescription    = "The adb founder-playbook harness: the devil's-advocate agent, the stage-gate + ingestion skills, and the adb MCP server (graph/knowledge tools)."
	pluginAuthorName     = "valter-silva-au"
	marketplaceName      = "adb"
)

// PluginManifestFor returns the plugin identity manifest at the given version
// (empty → DefaultPluginVersion). Shared by BuildPlugin and `adb plugin manifest`.
func PluginManifestFor(version string) models.PluginManifest {
	if version == "" {
		version = DefaultPluginVersion
	}
	return models.PluginManifest{
		Name:        PluginName,
		Version:     version,
		Description: pluginDescription,
		Author:      models.PluginAuthor{Name: pluginAuthorName},
	}
}

// PluginBuildOptions configures BuildPlugin.
type PluginBuildOptions struct {
	Version string
	DryRun  bool
	Force   bool
}

// PluginBuildEntry is one file written (or planned) by BuildPlugin.
type PluginBuildEntry struct {
	Path   string // plugin-relative, forward-slashed
	Action HarnessInstallAction
}

// PluginBuildResult summarises a BuildPlugin call.
type PluginBuildResult struct {
	DestDir string
	Plugin  models.PluginManifest
	Entries []PluginBuildEntry
	DryRun  bool
}

// BuildPlugin emits the Claude Code plugin + marketplace for the harness in fsys
// into destDir: the embedded agents/skills (reusing HarnessManifest), the plugin
// manifest, an .mcp.json registering the adb MCP server, and the marketplace
// manifest. Writes are idempotent + clobber-safe (installHarnessFile: matching =
// unchanged, differing = skipped unless Force, DryRun plans without writing).
func BuildPlugin(fsys fs.FS, destDir string, opts PluginBuildOptions) (PluginBuildResult, error) {
	if destDir == "" {
		return PluginBuildResult{}, fmt.Errorf("destination directory not resolved")
	}
	manifest := PluginManifestFor(opts.Version)
	hopts := HarnessInstallOptions{DryRun: opts.DryRun, Force: opts.Force}
	res := PluginBuildResult{DestDir: destDir, Plugin: manifest, DryRun: opts.DryRun}

	writeFile := func(rel string, content []byte) error {
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		action, err := installHarnessFile(dest, content, hopts)
		if err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
		res.Entries = append(res.Entries, PluginBuildEntry{Path: rel, Action: action})
		return nil
	}
	writeJSON := func(rel string, v any) error {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal %s: %w", rel, err)
		}
		return writeFile(rel, append(data, '\n'))
	}

	// Copy the embedded harness (agents/** and skills/**) into the plugin.
	files, err := HarnessManifest(fsys)
	if err != nil {
		return PluginBuildResult{}, fmt.Errorf("enumerate harness for plugin: %w", err)
	}
	for _, f := range files {
		sub := "agents"
		if f.Kind == HarnessSkill {
			sub = "skills"
		}
		if err := writeFile(sub+"/"+f.RelPath, f.Content); err != nil {
			return PluginBuildResult{}, err
		}
	}

	// Plugin identity.
	if err := writeJSON(".claude-plugin/plugin.json", manifest); err != nil {
		return PluginBuildResult{}, err
	}
	// Ship the adb MCP server so installing the plugin wires the graph/knowledge tools.
	mcp := models.MCPConfig{MCPServers: map[string]models.MCPServerSpec{
		"adb": {Command: "adb", Args: []string{"mcp", "serve"}},
	}}
	if err := writeJSON(".mcp.json", mcp); err != nil {
		return PluginBuildResult{}, err
	}
	// Single-plugin marketplace: the emitted dir is installable via
	// `/plugin marketplace add <dir>` (source "./" — the plugin is at the root).
	market := models.MarketplaceManifest{
		Name:  marketplaceName,
		Owner: models.MarketplaceOwner{Name: pluginAuthorName},
		Plugins: []models.MarketplacePlugin{
			{Name: PluginName, Source: "./", Description: pluginDescription},
		},
	}
	if err := writeJSON(".claude-plugin/marketplace.json", market); err != nil {
		return PluginBuildResult{}, err
	}

	return res, nil
}
