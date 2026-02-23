package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// MCPServerStatus represents the health check result for an MCP server.
type MCPServerStatus struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"`
	URL          string        `json:"url,omitempty"`
	Command      string        `json:"command,omitempty"`
	Healthy      bool          `json:"healthy"`
	ResponseTime time.Duration `json:"response_time_ms"`
	Error        string        `json:"error,omitempty"`
}

// MCPServerConfig represents a configured MCP server from .mcp.json.
type MCPServerConfig struct {
	Type    string   `json:"type"`
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// MCPCheckResult holds the results of an MCP server health check.
type MCPCheckResult struct {
	Servers   []MCPServerStatus `json:"servers"`
	CheckedAt time.Time         `json:"checked_at"`
}

// MCPCacheEntry represents a cached MCP check result.
type MCPCacheEntry struct {
	Result    MCPCheckResult `json:"result"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// MCPClient provides MCP server validation and discovery caching.
type MCPClient interface {
	// CheckServers validates all configured MCP servers and returns their status.
	CheckServers(configPath string) (*MCPCheckResult, error)

	// LoadCache loads cached MCP check results. Returns nil if cache is stale or missing.
	LoadCache(basePath string) *MCPCheckResult

	// SaveCache saves MCP check results with a TTL.
	SaveCache(basePath string, result *MCPCheckResult, ttl time.Duration) error
}

type mcpClient struct {
	httpClient *http.Client
}

// NewMCPClient creates a new MCP client for server validation.
func NewMCPClient() MCPClient {
	return &mcpClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *mcpClient) CheckServers(configPath string) (*MCPCheckResult, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading MCP config %s: %w", configPath, err)
	}

	var config struct {
		MCPServers map[string]MCPServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing MCP config: %w", err)
	}

	result := &MCPCheckResult{
		CheckedAt: time.Now().UTC(),
	}

	for name, server := range config.MCPServers {
		status := MCPServerStatus{
			Name:    name,
			Type:    server.Type,
			URL:     server.URL,
			Command: server.Command,
		}

		start := time.Now()

		switch server.Type {
		case "http":
			if server.URL != "" {
				resp, err := c.httpClient.Get(server.URL)
				if err != nil {
					status.Error = err.Error()
				} else {
					_ = resp.Body.Close()
					// Any response (even 4xx) means the server is reachable.
					status.Healthy = resp.StatusCode < 500
					if resp.StatusCode >= 500 {
						status.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
					}
				}
			} else {
				status.Error = "no URL configured"
			}
		case "stdio":
			// For stdio servers, we can only check if the command exists.
			if server.Command != "" {
				_, lookErr := lookPath(server.Command)
				if lookErr != nil {
					status.Error = fmt.Sprintf("command not found: %s", server.Command)
				} else {
					status.Healthy = true
				}
			} else {
				status.Error = "no command configured"
			}
		default:
			status.Error = fmt.Sprintf("unknown server type: %s", server.Type)
		}

		status.ResponseTime = time.Since(start)
		result.Servers = append(result.Servers, status)
	}

	return result, nil
}

func (c *mcpClient) LoadCache(basePath string) *MCPCheckResult {
	cachePath := filepath.Join(basePath, ".adb_mcp_cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var entry MCPCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil // Cache expired.
	}

	return &entry.Result
}

func (c *mcpClient) SaveCache(basePath string, result *MCPCheckResult, ttl time.Duration) error {
	entry := MCPCacheEntry{
		Result:    *result,
		ExpiresAt: time.Now().Add(ttl),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP cache: %w", err)
	}

	cachePath := filepath.Join(basePath, ".adb_mcp_cache.json")
	return os.WriteFile(cachePath, data, 0o644)
}

// lookPath wraps exec.LookPath for testability.
var lookPath = exec.LookPath
