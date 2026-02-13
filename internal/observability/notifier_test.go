package observability

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlackNotifier_NoAlerts(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	err := n.Notify(nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called {
		t.Fatal("expected no HTTP request for empty alerts")
	}

	err = n.Notify([]Alert{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if called {
		t.Fatal("expected no HTTP request for empty alerts slice")
	}
}

func TestSlackNotifier_SendsAlerts(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	alerts := []Alert{
		{
			ID:          "blocked-TASK-00001",
			Condition:   "task_blocked_too_long",
			Severity:    SeverityHigh,
			Message:     "task TASK-00001 has been blocked for more than 24 hours",
			TriggeredAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			ID:          "stale-TASK-00002",
			Condition:   "task_stale",
			Severity:    SeverityMedium,
			Message:     "task TASK-00002 has had no activity for more than 3 days",
			TriggeredAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		},
	}

	err := n.Notify(alerts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}

	var msg slackMessage
	if err := json.Unmarshal(receivedBody, &msg); err != nil {
		t.Fatalf("unmarshaling request body: %v", err)
	}

	// Expect: header + section(alert1) + divider + section(alert2) = 4 blocks
	if len(msg.Blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(msg.Blocks))
	}

	if msg.Blocks[0].Type != "header" {
		t.Errorf("expected first block type header, got %s", msg.Blocks[0].Type)
	}
	if msg.Blocks[0].Text == nil || msg.Blocks[0].Text.Text != "adb Alert Summary" {
		t.Errorf("expected header text 'adb Alert Summary', got %v", msg.Blocks[0].Text)
	}

	if msg.Blocks[1].Type != "section" {
		t.Errorf("expected second block type section, got %s", msg.Blocks[1].Type)
	}

	if msg.Blocks[2].Type != "divider" {
		t.Errorf("expected third block type divider, got %s", msg.Blocks[2].Type)
	}

	if msg.Blocks[3].Type != "section" {
		t.Errorf("expected fourth block type section, got %s", msg.Blocks[3].Type)
	}

	// Verify alert content is present in the section blocks
	body := string(receivedBody)
	if !contains(body, "TASK-00001") {
		t.Error("expected body to contain TASK-00001")
	}
	if !contains(body, "TASK-00002") {
		t.Error("expected body to contain TASK-00002")
	}
	if !contains(body, "2025-01-15 10:30 UTC") {
		t.Error("expected body to contain triggered time")
	}
}

func TestSlackNotifier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	alerts := []Alert{
		{
			ID:          "test-alert",
			Condition:   "task_blocked_too_long",
			Severity:    SeverityHigh,
			Message:     "test alert",
			TriggeredAt: time.Now().UTC(),
		},
	}

	err := n.Notify(alerts)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code 500, got: %s", err.Error())
	}
}

func TestSlackNotifier_SeverityEmojis(t *testing.T) {
	tests := []struct {
		severity AlertSeverity
		emoji    string
	}{
		{SeverityHigh, "\U0001f534"},
		{SeverityMedium, "\U0001f7e1"},
		{SeverityLow, "\U0001f535"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			var receivedBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				receivedBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("reading request body: %v", err)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			n := NewSlackNotifier(srv.URL)
			alerts := []Alert{
				{
					ID:          "emoji-test",
					Condition:   "test",
					Severity:    tt.severity,
					Message:     "test message",
					TriggeredAt: time.Now().UTC(),
				},
			}

			err := n.Notify(alerts)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			body := string(receivedBody)
			if !contains(body, tt.emoji) {
				t.Errorf("expected body to contain emoji %s for severity %s", tt.emoji, tt.severity)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
