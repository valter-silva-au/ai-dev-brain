package observability

import (
	"strings"
	"testing"
	"time"
)

// ev is a tiny helper to build an Event with a task_id, an optional activity,
// and an age relative to now.
func ev(typ EventType, taskID, activity string, ago time.Duration) Event {
	data := map[string]interface{}{"task_id": taskID}
	if activity != "" {
		data["activity"] = activity
	}
	return Event{
		Timestamp: time.Now().UTC().Add(-ago),
		Type:      typ,
		Data:      data,
	}
}

func TestSessionDigest(t *testing.T) {
	tests := []struct {
		name    string
		events  []Event
		self    string
		since   time.Duration
		cap     int
		wantN   int      // number of digest lines
		wantHas []string // substrings each expected line-set must contain
		wantNot []string // substrings that must NOT appear
	}{
		{
			name: "latest event per other task, self excluded",
			events: []Event{
				ev(EventAgentSessionStarted, "TASK-00081", "", 30*time.Minute),
				ev(EventAgentSessionActive, "TASK-00081", "editing", 12*time.Minute),
				ev(EventAgentSessionStarted, "TASK-00088", "", 5*time.Minute), // self
			},
			self:    "TASK-00088",
			since:   8 * time.Hour,
			cap:     5,
			wantN:   1,
			wantHas: []string{"TASK-00081", "editing", "ago"},
			wantNot: []string{"TASK-00088"},
		},
		{
			name: "multiple other tasks",
			events: []Event{
				ev(EventAgentSessionActive, "TASK-00001", "editing", 3*time.Minute),
				ev(EventAgentSessionActive, "TASK-00002", "running tests", 9*time.Minute),
			},
			self:    "TASK-00099",
			since:   8 * time.Hour,
			cap:     5,
			wantN:   2,
			wantHas: []string{"TASK-00001", "TASK-00002"},
		},
		{
			name: "since-window cutoff drops stale events",
			events: []Event{
				ev(EventAgentSessionActive, "TASK-00001", "editing", 30*time.Minute),
				ev(EventAgentSessionActive, "TASK-00002", "editing", 10*time.Hour),
			},
			self:    "TASK-00099",
			since:   8 * time.Hour,
			cap:     5,
			wantN:   1,
			wantHas: []string{"TASK-00001"},
			wantNot: []string{"TASK-00002"},
		},
		{
			name: "cap limits number of lines",
			events: []Event{
				ev(EventAgentSessionActive, "TASK-00001", "editing", 1*time.Minute),
				ev(EventAgentSessionActive, "TASK-00002", "editing", 2*time.Minute),
				ev(EventAgentSessionActive, "TASK-00003", "editing", 3*time.Minute),
				ev(EventAgentSessionActive, "TASK-00004", "editing", 4*time.Minute),
			},
			self:  "TASK-00099",
			since: 8 * time.Hour,
			cap:   2,
			wantN: 2,
		},
		{
			name:   "empty case",
			events: nil,
			self:   "TASK-00099",
			since:  8 * time.Hour,
			cap:    5,
			wantN:  0,
		},
		{
			name: "only self present -> empty",
			events: []Event{
				ev(EventAgentSessionActive, "TASK-00099", "editing", 1*time.Minute),
			},
			self:  "TASK-00099",
			since: 8 * time.Hour,
			cap:   5,
			wantN: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := newTestEventLog(t)
			for _, e := range tt.events {
				appendRawEvent(t, log, e)
			}

			d, err := BuildSessionDigest(log, SessionDigestOptions{
				Self:  tt.self,
				Since: tt.since,
				Cap:   tt.cap,
			})
			if err != nil {
				t.Fatalf("BuildSessionDigest: %v", err)
			}

			if len(d.Lines) != tt.wantN {
				t.Fatalf("got %d digest lines, want %d: %#v", len(d.Lines), tt.wantN, d.Lines)
			}

			joined := strings.Join(renderLines(d), "\n")
			for _, want := range tt.wantHas {
				if !strings.Contains(joined, want) {
					t.Errorf("expected digest to contain %q, got:\n%s", want, joined)
				}
			}
			for _, notWant := range tt.wantNot {
				if strings.Contains(joined, notWant) {
					t.Errorf("digest should NOT contain %q, got:\n%s", notWant, joined)
				}
			}
		})
	}
}

// TestSessionDigest_LatestWins verifies the newest event per task_id is the
// one that shapes the line (activity + age), not an earlier one.
func TestSessionDigest_LatestWins(t *testing.T) {
	log := newTestEventLog(t)
	appendRawEvent(t, log, ev(EventAgentSessionStarted, "TASK-00081", "starting", 40*time.Minute))
	appendRawEvent(t, log, ev(EventAgentSessionActive, "TASK-00081", "editing", 2*time.Minute))

	d, err := BuildSessionDigest(log, SessionDigestOptions{Self: "TASK-X", Since: 8 * time.Hour, Cap: 5})
	if err != nil {
		t.Fatalf("BuildSessionDigest: %v", err)
	}
	if len(d.Lines) != 1 {
		t.Fatalf("want 1 line, got %d", len(d.Lines))
	}
	if d.Lines[0].Activity != "editing" {
		t.Errorf("want latest activity 'editing', got %q", d.Lines[0].Activity)
	}
}

// renderLines is a small test helper turning the digest into human strings.
func renderLines(d SessionDigest) []string {
	out := make([]string, 0, len(d.Lines))
	for _, l := range d.Lines {
		out = append(out, l.Render())
	}
	return out
}

// newTestEventLog builds an EventLog backed by a temp file.
func newTestEventLog(t *testing.T) *EventLog {
	t.Helper()
	return NewEventLog(t.TempDir() + "/.events.jsonl")
}

// appendRawEvent writes a fully-formed Event (with a chosen Timestamp) directly
// through the log, bypassing Log()'s time.Now() stamping so tests control age.
func appendRawEvent(t *testing.T, log *EventLog, e Event) {
	t.Helper()
	if err := log.appendEvent(e); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
}
