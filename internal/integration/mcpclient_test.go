package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPClient_CheckServers(t *testing.T) {
	// Set up a test HTTP server.
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	t.Run("checks HTTP servers", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		config := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"healthy": map[string]interface{}{
					"type": "http",
					"url":  healthyServer.URL,
				},
				"unhealthy": map[string]interface{}{
					"type": "http",
					"url":  unhealthyServer.URL,
				},
			},
		}

		data, _ := json.Marshal(config)
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			t.Fatal(err)
		}

		client := NewMCPClient()
		result, err := client.CheckServers(configPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Servers) != 2 {
			t.Fatalf("expected 2 servers, got %d", len(result.Servers))
		}

		for _, s := range result.Servers {
			if s.Name == "healthy" && !s.Healthy {
				t.Error("expected healthy server to be healthy")
			}
			if s.Name == "unhealthy" && s.Healthy {
				t.Error("expected unhealthy server to be unhealthy")
			}
		}
	})

	t.Run("checks stdio servers", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		// Override lookPath for testing.
		origLookPath := lookPath
		lookPath = func(file string) (string, error) {
			if file == "existing-cmd" {
				return "/usr/bin/existing-cmd", nil
			}
			return "", fmt.Errorf("not found")
		}
		defer func() { lookPath = origLookPath }()

		config := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"found": map[string]interface{}{
					"type":    "stdio",
					"command": "existing-cmd",
				},
				"missing": map[string]interface{}{
					"type":    "stdio",
					"command": "nonexistent-cmd",
				},
			},
		}

		data, _ := json.Marshal(config)
		if err := os.WriteFile(configPath, data, 0o644); err != nil {
			t.Fatal(err)
		}

		client := NewMCPClient()
		result, err := client.CheckServers(configPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, s := range result.Servers {
			if s.Name == "found" && !s.Healthy {
				t.Error("expected found server to be healthy")
			}
			if s.Name == "missing" && s.Healthy {
				t.Error("expected missing server to be unhealthy")
			}
		}
	})

	t.Run("handles missing config file", func(t *testing.T) {
		client := NewMCPClient()
		_, err := client.CheckServers("/nonexistent/path/.mcp.json")
		if err == nil {
			t.Fatal("expected error for missing config")
		}
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")
		if err := os.WriteFile(configPath, []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}

		client := NewMCPClient()
		_, err := client.CheckServers(configPath)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestMCPClient_Cache(t *testing.T) {
	t.Run("saves and loads cache", func(t *testing.T) {
		dir := t.TempDir()
		client := NewMCPClient()

		result := &MCPCheckResult{
			CheckedAt: time.Now().UTC(),
			Servers: []MCPServerStatus{
				{Name: "test", Type: "http", Healthy: true},
			},
		}

		if err := client.SaveCache(dir, result, 5*time.Minute); err != nil {
			t.Fatalf("save cache error: %v", err)
		}

		loaded := client.LoadCache(dir)
		if loaded == nil {
			t.Fatal("expected cached result, got nil")
		}
		if len(loaded.Servers) != 1 {
			t.Fatalf("expected 1 server, got %d", len(loaded.Servers))
		}
		if loaded.Servers[0].Name != "test" {
			t.Errorf("expected server name 'test', got %q", loaded.Servers[0].Name)
		}
	})

	t.Run("returns nil for expired cache", func(t *testing.T) {
		dir := t.TempDir()
		client := NewMCPClient()

		result := &MCPCheckResult{
			CheckedAt: time.Now().UTC(),
		}

		// Save with 0 TTL (immediately expired).
		if err := client.SaveCache(dir, result, -1*time.Second); err != nil {
			t.Fatalf("save cache error: %v", err)
		}

		loaded := client.LoadCache(dir)
		if loaded != nil {
			t.Fatal("expected nil for expired cache")
		}
	})

	t.Run("returns nil for missing cache", func(t *testing.T) {
		dir := t.TempDir()
		client := NewMCPClient()

		loaded := client.LoadCache(dir)
		if loaded != nil {
			t.Fatal("expected nil for missing cache")
		}
	})
}
