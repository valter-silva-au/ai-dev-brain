package integration

import (
	"fmt"
	"net"
	"time"
)

// ConnectivityChecker checks network connectivity
type ConnectivityChecker interface {
	// CheckTCP checks if a TCP connection can be established to the given address
	// address format: "host:port" (e.g., "google.com:80", "8.8.8.8:53")
	CheckTCP(address string, timeout time.Duration) error
}

// DefaultConnectivityChecker implements ConnectivityChecker
type DefaultConnectivityChecker struct{}

// NewConnectivityChecker creates a new connectivity checker
func NewConnectivityChecker() ConnectivityChecker {
	return &DefaultConnectivityChecker{}
}

// CheckTCP checks if a TCP connection can be established to the given address
// Returns nil if connection successful, error otherwise
func (c *DefaultConnectivityChecker) CheckTCP(address string, timeout time.Duration) error {
	if address == "" {
		return fmt.Errorf("address cannot be empty")
	}

	if timeout <= 0 {
		timeout = 5 * time.Second // Default timeout
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", address, err)
	}

	// Close the connection immediately
	if err := conn.Close(); err != nil {
		// Log but don't return error, as the connection was successful
		return nil
	}

	return nil
}

// IsOnline checks connectivity to multiple well-known addresses
// Returns true if at least one connection succeeds
func IsOnline(timeout time.Duration) bool {
	checker := NewConnectivityChecker()

	// Try multiple well-known addresses
	addresses := []string{
		"8.8.8.8:53",    // Google DNS
		"1.1.1.1:53",    // Cloudflare DNS
		"google.com:80", // Google HTTP
	}

	for _, addr := range addresses {
		if err := checker.CheckTCP(addr, timeout); err == nil {
			return true
		}
	}

	return false
}
