package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewMCPClient(t *testing.T) {
	// Test with positive TTL
	client := NewMCPClient(30 * time.Second)
	if client == nil {
		t.Error("expected non-nil client")
	}

	// Test with zero TTL (should use default)
	client = NewMCPClient(0)
	if client == nil {
		t.Error("expected non-nil client with default TTL")
	}

	// Test with negative TTL (should use default)
	client = NewMCPClient(-1 * time.Second)
	if client == nil {
		t.Error("expected non-nil client with default TTL")
	}
}

func TestCheckHealth(t *testing.T) {
	// Test empty URL
	client := NewMCPClient(30 * time.Second)
	_, err := client.CheckHealth("")
	if err == nil {
		t.Error("expected error for empty URL")
	}

	// Test successful health check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	result, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !result.Healthy {
		t.Error("expected healthy result")
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
	if result.ServerURL != server.URL {
		t.Errorf("expected server URL %s, got %s", server.URL, result.ServerURL)
	}

	// Test unhealthy status code
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	result, err = client.CheckHealth(unhealthyServer.URL)
	if err == nil {
		t.Error("expected error for unhealthy status")
	}
	if result.Healthy {
		t.Error("expected unhealthy result")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status code 500, got %d", result.StatusCode)
	}

	// Test 3xx redirect (should be considered healthy)
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer redirectServer.Close()

	result, err = client.CheckHealth(redirectServer.URL)
	if err != nil {
		t.Errorf("unexpected error for redirect: %v", err)
	}
	if !result.Healthy {
		t.Error("expected healthy result for 3xx status")
	}
}

func TestCheckHealthCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewMCPClient(1 * time.Second)

	// First call - should hit the server
	result1, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call - should use cache
	result2, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call (cached), got %d", callCount)
	}

	// Results should be the same
	if result1.ServerURL != result2.ServerURL {
		t.Error("cached result differs from original")
	}

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Third call - cache expired, should hit server again
	_, err = client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache expiry, got %d", callCount)
	}
}

func TestClearCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewMCPClient(10 * time.Second)

	// First call
	_, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Clear cache
	client.ClearCache()

	// Second call - should hit server again after cache clear
	_, err = client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache clear, got %d", callCount)
	}
}

func TestSetTTL(t *testing.T) {
	client := NewMCPClient(1 * time.Second)

	// Set new TTL
	client.SetTTL(5 * time.Second)

	// Test that invalid TTL is ignored
	client.SetTTL(0)
	client.SetTTL(-1 * time.Second)

	// If we get here without panic, the test passes
}

func TestCheckHealthInvalidURL(t *testing.T) {
	client := NewMCPClient(30 * time.Second)

	// Test invalid URL
	result, err := client.CheckHealth("http://invalid-host-that-does-not-exist.local:9999")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
	if result.Healthy {
		t.Error("expected unhealthy result for invalid URL")
	}
	if result.Error == nil {
		t.Error("expected error to be stored in result")
	}
}
