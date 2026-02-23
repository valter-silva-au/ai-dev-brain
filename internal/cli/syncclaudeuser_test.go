package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAdbMCPServerConfig(t *testing.T) {
	config := adbMCPServerConfig()

	if config["type"] != "stdio" {
		t.Errorf("expected type=stdio, got %v", config["type"])
	}
	if config["command"] != "adb" {
		t.Errorf("expected command=adb, got %v", config["command"])
	}

	args, ok := config["args"].([]interface{})
	if !ok {
		t.Fatalf("expected args to be []interface{}, got %T", config["args"])
	}
	if len(args) != 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Errorf("expected args=[mcp, serve], got %v", args)
	}
}

func TestSyncAdbMCPServer_CreatesNewFile(t *testing.T) {
	home := t.TempDir()

	count, err := syncAdbMCPServer(home, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}

	// Verify file contents
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("reading .claude.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing .claude.json: %v", err)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcpServers to be a map, got %T", config["mcpServers"])
	}

	adbServer, ok := servers["adb"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected adb server entry, got %v", servers)
	}

	if adbServer["type"] != "stdio" {
		t.Errorf("expected type=stdio, got %v", adbServer["type"])
	}
	if adbServer["command"] != "adb" {
		t.Errorf("expected command=adb, got %v", adbServer["command"])
	}
}

func TestSyncAdbMCPServer_MergesIntoExisting(t *testing.T) {
	home := t.TempDir()

	// Write an existing .claude.json with other keys and servers
	existing := map[string]interface{}{
		"someOtherKey": "preserved",
		"mcpServers": map[string]interface{}{
			"custom-server": map[string]interface{}{
				"type": "http",
				"url":  "https://example.com",
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o644); err != nil {
		t.Fatalf("writing existing .claude.json: %v", err)
	}

	count, err := syncAdbMCPServer(home, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}

	// Verify merged contents
	result, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("reading .claude.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("parsing .claude.json: %v", err)
	}

	// Other keys preserved
	if config["someOtherKey"] != "preserved" {
		t.Errorf("expected someOtherKey=preserved, got %v", config["someOtherKey"])
	}

	servers := config["mcpServers"].(map[string]interface{})

	// Custom server preserved
	if _, ok := servers["custom-server"]; !ok {
		t.Error("expected custom-server to be preserved")
	}

	// adb server added
	if _, ok := servers["adb"]; !ok {
		t.Error("expected adb server to be present")
	}
}

func TestSyncAdbMCPServer_DryRun(t *testing.T) {
	home := t.TempDir()

	count, err := syncAdbMCPServer(home, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}

	// File should NOT be created in dry-run
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Error("expected .claude.json to not exist in dry-run mode")
	}
}

func TestSyncThirdPartyMCPServers_ExcludesAdb(t *testing.T) {
	home := t.TempDir()

	// syncThirdPartyMCPServers reads from embedded FS and excludes the adb entry.
	// The embedded template contains adb + aws-docs + aws-knowledge + context7.
	count, err := syncThirdPartyMCPServers(home, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should sync only third-party servers (adb excluded)
	if count < 1 {
		t.Errorf("expected at least 1 third-party server, got %d", count)
	}

	// Verify file contents
	result, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("reading .claude.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("parsing .claude.json: %v", err)
	}

	servers := config["mcpServers"].(map[string]interface{})

	// adb should NOT be present (excluded)
	if _, ok := servers["adb"]; ok {
		t.Error("expected adb server to be excluded from third-party sync")
	}

	// At least one third-party server should be present
	if len(servers) == 0 {
		t.Error("expected at least one third-party server to be present")
	}
}

func TestSyncAdbThenThirdParty_BothPresent(t *testing.T) {
	home := t.TempDir()

	// Step 1: sync adb (always)
	adbCount, err := syncAdbMCPServer(home, false)
	if err != nil {
		t.Fatalf("syncing adb: %v", err)
	}
	if adbCount != 1 {
		t.Errorf("expected adb count=1, got %d", adbCount)
	}

	// Step 2: sync third-party (with --mcp)
	thirdPartyCount, err := syncThirdPartyMCPServers(home, false)
	if err != nil {
		t.Fatalf("syncing third-party: %v", err)
	}
	if thirdPartyCount < 1 {
		t.Errorf("expected at least 1 third-party server, got %d", thirdPartyCount)
	}

	// Verify both are present
	result, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("reading .claude.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("parsing .claude.json: %v", err)
	}

	servers := config["mcpServers"].(map[string]interface{})

	if _, ok := servers["adb"]; !ok {
		t.Error("expected adb server to be present")
	}

	// At least one third-party server should coexist
	if len(servers) < 2 {
		t.Errorf("expected at least 2 servers (adb + third-party), got %d", len(servers))
	}
}

func TestMergeMCPServers_NoExistingMcpServersKey(t *testing.T) {
	home := t.TempDir()

	// Write a .claude.json without mcpServers key
	existing := map[string]interface{}{
		"someKey": "someValue",
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), data, 0o644); err != nil {
		t.Fatalf("writing existing .claude.json: %v", err)
	}

	servers := map[string]interface{}{
		"test-server": map[string]interface{}{
			"type": "http",
			"url":  "https://example.com",
		},
	}

	count, err := mergeMCPServers(servers, home, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}

	result, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("reading .claude.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("parsing .claude.json: %v", err)
	}

	// Original key preserved
	if config["someKey"] != "someValue" {
		t.Errorf("expected someKey=someValue, got %v", config["someKey"])
	}

	// Server added
	servers2 := config["mcpServers"].(map[string]interface{})
	if _, ok := servers2["test-server"]; !ok {
		t.Error("expected test-server to be present")
	}
}

func TestSyncThirdPartyMCPServers_DryRunNoFileCreated(t *testing.T) {
	home := t.TempDir()

	count, err := syncThirdPartyMCPServers(home, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 server in dry-run, got %d", count)
	}

	// File should NOT be created in dry-run
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); !os.IsNotExist(err) {
		t.Error("expected .claude.json to not exist in dry-run mode")
	}
}
