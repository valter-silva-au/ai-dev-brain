package models

// This file models the Claude Code PLUGIN + MARKETPLACE manifests (#139 step 20,
// D12 phase 2). A plugin directory carries `.claude-plugin/plugin.json`
// (identity) alongside its agents/skills/commands and an optional `.mcp.json`; a
// marketplace directory carries `.claude-plugin/marketplace.json` listing one or
// more plugins. The adb harness graduates from "installed files" (D12 phase 1)
// into this distributable form.

// PluginAuthor identifies a plugin's author in plugin.json.
type PluginAuthor struct {
	Name string `json:"name" yaml:"name"`
}

// PluginManifest is `.claude-plugin/plugin.json` — the plugin's identity.
type PluginManifest struct {
	Name        string       `json:"name" yaml:"name"`
	Version     string       `json:"version" yaml:"version"`
	Description string       `json:"description" yaml:"description"`
	Author      PluginAuthor `json:"author" yaml:"author"`
}

// MCPServerSpec is one server entry in a plugin's `.mcp.json`.
type MCPServerSpec struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args,omitempty" yaml:"args,omitempty"`
}

// MCPConfig is a plugin's `.mcp.json` (server registrations shipped with the
// plugin). Installing the plugin wires these servers.
type MCPConfig struct {
	MCPServers map[string]MCPServerSpec `json:"mcpServers" yaml:"mcpServers"`
}

// MarketplaceOwner identifies a marketplace's owner in marketplace.json.
type MarketplaceOwner struct {
	Name string `json:"name" yaml:"name"`
}

// MarketplacePlugin is one entry in a marketplace's plugin list. Source is a
// path relative to the marketplace root ("./" means the plugin lives at the
// marketplace root — the single-plugin case).
type MarketplacePlugin struct {
	Name        string `json:"name" yaml:"name"`
	Source      string `json:"source" yaml:"source"`
	Description string `json:"description" yaml:"description"`
}

// MarketplaceManifest is `.claude-plugin/marketplace.json`.
type MarketplaceManifest struct {
	Name    string              `json:"name" yaml:"name"`
	Owner   MarketplaceOwner    `json:"owner" yaml:"owner"`
	Plugins []MarketplacePlugin `json:"plugins" yaml:"plugins"`
}
