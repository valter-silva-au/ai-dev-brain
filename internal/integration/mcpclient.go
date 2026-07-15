package integration

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MCPHealthCheck represents a cached health check result
type MCPHealthCheck struct {
	ServerURL  string
	Healthy    bool
	StatusCode int
	CheckedAt  time.Time
	Error      error
}

// MCPClient checks MCP server health with caching
type MCPClient interface {
	// CheckHealth performs an HTTP GET health check on an MCP server
	// Returns cached result if within TTL, otherwise performs new check
	CheckHealth(serverURL string) (MCPHealthCheck, error)

	// ClearCache clears all cached health check results
	ClearCache()

	// SetTTL sets the cache TTL duration
	SetTTL(ttl time.Duration)
}

// DefaultMCPClient implements MCPClient with TTL caching
type DefaultMCPClient struct {
	httpClient *http.Client
	cache      map[string]MCPHealthCheck
	cacheMux   sync.RWMutex
	ttl        time.Duration
}

// NewMCPClient creates a new MCP client with the specified TTL
func NewMCPClient(ttl time.Duration) MCPClient {
	if ttl <= 0 {
		ttl = 30 * time.Second // Default TTL
	}

	return &DefaultMCPClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]MCPHealthCheck),
		ttl:   ttl,
	}
}

// CheckHealth performs an HTTP GET health check on an MCP server
func (c *DefaultMCPClient) CheckHealth(serverURL string) (MCPHealthCheck, error) {
	if serverURL == "" {
		return MCPHealthCheck{}, fmt.Errorf("serverURL cannot be empty")
	}

	// Check cache first
	c.cacheMux.RLock()
	cached, exists := c.cache[serverURL]
	c.cacheMux.RUnlock()

	// Return cached result if within TTL
	if exists && time.Since(cached.CheckedAt) < c.ttl {
		return cached, nil
	}

	// Perform new health check
	result := MCPHealthCheck{
		ServerURL: serverURL,
		CheckedAt: time.Now(),
	}

	resp, err := c.httpClient.Get(serverURL)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Errorf("failed to connect to MCP server: %w", err)
	} else {
		defer resp.Body.Close()
		result.StatusCode = resp.StatusCode
		// Consider 2xx and 3xx status codes as healthy
		result.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 400
		if !result.Healthy {
			result.Error = fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
		}
	}

	// Update cache
	c.cacheMux.Lock()
	c.cache[serverURL] = result
	c.cacheMux.Unlock()

	return result, result.Error
}

// ClearCache clears all cached health check results
func (c *DefaultMCPClient) ClearCache() {
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	c.cache = make(map[string]MCPHealthCheck)
}

// SetTTL sets the cache TTL duration
func (c *DefaultMCPClient) SetTTL(ttl time.Duration) {
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	if ttl > 0 {
		c.ttl = ttl
	}
}
