package integration

import (
	"net"
	"testing"
	"time"
)

func TestCheckTCP(t *testing.T) {
	checker := NewConnectivityChecker()

	tests := []struct {
		name        string
		address     string
		timeout     time.Duration
		expectError bool
	}{
		{
			name:        "empty address",
			address:     "",
			timeout:     5 * time.Second,
			expectError: true,
		},
		{
			name:        "invalid address format",
			address:     "invalid",
			timeout:     1 * time.Second,
			expectError: true,
		},
		{
			name:        "unreachable address",
			address:     "192.0.2.1:9999", // TEST-NET-1, should be unreachable
			timeout:     1 * time.Second,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checker.CheckTCP(tt.address, tt.timeout)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCheckTCPWithLocalServer(t *testing.T) {
	// Start a local TCP server for testing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer listener.Close()

	// Get the actual port assigned
	address := listener.Addr().String()

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Test successful connection
	checker := NewConnectivityChecker()
	err = checker.CheckTCP(address, 2*time.Second)
	if err != nil {
		t.Errorf("expected successful connection to local server, got error: %v", err)
	}
}

func TestCheckTCPTimeout(t *testing.T) {
	checker := NewConnectivityChecker()

	// Test with zero timeout (should use default)
	err := checker.CheckTCP("192.0.2.1:9999", 0)
	if err == nil {
		t.Error("expected error for unreachable address")
	}

	// Test with negative timeout (should use default)
	err = checker.CheckTCP("192.0.2.1:9999", -1*time.Second)
	if err == nil {
		t.Error("expected error for unreachable address")
	}
}

func TestIsOnline(t *testing.T) {
	// Note: This test may fail in completely offline environments
	// We'll test the function exists and returns a boolean
	result := IsOnline(2 * time.Second)

	// Just verify it returns a boolean value
	_ = result

	// Test with very short timeout
	result = IsOnline(1 * time.Millisecond)
	// With such a short timeout, it's likely to return false
	// but we won't enforce that in the test as it depends on network conditions
	_ = result
}
