package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/drapaimern/ai-dev-brain/internal/core"
	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// fakeChannelAdapter implements core.ChannelAdapter for testing.
type fakeChannelAdapter struct {
	name      string
	chanType  models.ChannelType
	items     []models.ChannelItem
	fetchErr  error
	sendErr   error
	sentItems []models.OutputItem
}

func (f *fakeChannelAdapter) Name() string                 { return f.name }
func (f *fakeChannelAdapter) Type() models.ChannelType     { return f.chanType }
func (f *fakeChannelAdapter) MarkProcessed(_ string) error { return nil }

func (f *fakeChannelAdapter) Fetch() ([]models.ChannelItem, error) {
	return f.items, f.fetchErr
}

func (f *fakeChannelAdapter) Send(item models.OutputItem) error {
	f.sentItems = append(f.sentItems, item)
	return f.sendErr
}

// testChannelRegistry implements core.ChannelRegistry for testing.
type testChannelRegistry struct {
	adapters map[string]*fakeChannelAdapter
	order    []*fakeChannelAdapter
}

func newTestChannelRegistry() *testChannelRegistry {
	return &testChannelRegistry{
		adapters: make(map[string]*fakeChannelAdapter),
	}
}

func (r *testChannelRegistry) addAdapter(a *fakeChannelAdapter) {
	r.adapters[a.name] = a
	r.order = append(r.order, a)
}

func (r *testChannelRegistry) Register(_ core.ChannelAdapter) error {
	return nil
}

func (r *testChannelRegistry) GetAdapter(name string) (core.ChannelAdapter, error) {
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("getting channel adapter: adapter %q not found", name)
	}
	return a, nil
}

func (r *testChannelRegistry) ListAdapters() []core.ChannelAdapter {
	result := make([]core.ChannelAdapter, len(r.order))
	for i, a := range r.order {
		result[i] = a
	}
	return result
}

func (r *testChannelRegistry) FetchAll() ([]models.ChannelItem, error) {
	var all []models.ChannelItem
	for _, a := range r.order {
		items, err := a.Fetch()
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// captureStdout captures stdout output during fn execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = origStdout

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading pipe: %v", err)
	}
	return string(out)
}

// --- channelListCmd tests ---

func TestChannelListCmd_NilRegistry(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()
	ChannelReg = nil

	err := channelListCmd.RunE(channelListCmd, nil)
	if err == nil {
		t.Fatal("expected error when ChannelReg is nil")
	}
	if !strings.Contains(err.Error(), "channel registry not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChannelListCmd_NoAdapters(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	ChannelReg = newTestChannelRegistry()

	output := captureStdout(t, func() {
		err := channelListCmd.RunE(channelListCmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No channel adapters registered") {
		t.Errorf("expected 'No channel adapters registered' message, got: %q", output)
	}
}

func TestChannelListCmd_MultipleAdapters(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{name: "email-main", chanType: models.ChannelEmail})
	reg.addAdapter(&fakeChannelAdapter{name: "slack-team", chanType: models.ChannelSlack})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelListCmd.RunE(channelListCmd, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "NAME") || !strings.Contains(output, "TYPE") {
		t.Errorf("expected table header with NAME and TYPE, got: %q", output)
	}
	if !strings.Contains(output, "email-main") {
		t.Errorf("expected adapter 'email-main' in output, got: %q", output)
	}
	if !strings.Contains(output, "slack-team") {
		t.Errorf("expected adapter 'slack-team' in output, got: %q", output)
	}
	if !strings.Contains(output, "email") {
		t.Errorf("expected channel type 'email' in output, got: %q", output)
	}
	if !strings.Contains(output, "slack") {
		t.Errorf("expected channel type 'slack' in output, got: %q", output)
	}
}

// --- channelInboxCmd tests ---

func TestChannelInboxCmd_NilRegistry(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()
	ChannelReg = nil

	err := channelInboxCmd.RunE(channelInboxCmd, nil)
	if err == nil {
		t.Fatal("expected error when ChannelReg is nil")
	}
	if !strings.Contains(err.Error(), "channel registry not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChannelInboxCmd_SpecificAdapter(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "email-main",
		chanType: models.ChannelEmail,
		items: []models.ChannelItem{
			{
				ID:       "msg-001",
				Channel:  models.ChannelEmail,
				From:     "alice@example.com",
				Subject:  "Design review notes",
				Priority: models.ChannelPriorityHigh,
			},
		},
	})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelInboxCmd.RunE(channelInboxCmd, []string{"email-main"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "msg-001") {
		t.Errorf("expected item ID in output, got: %q", output)
	}
	if !strings.Contains(output, "alice@example.com") {
		t.Errorf("expected sender in output, got: %q", output)
	}
	if !strings.Contains(output, "Design review notes") {
		t.Errorf("expected subject in output, got: %q", output)
	}
}

func TestChannelInboxCmd_AllAdapters(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "email-main",
		chanType: models.ChannelEmail,
		items: []models.ChannelItem{
			{
				ID:       "email-001",
				Channel:  models.ChannelEmail,
				From:     "alice@test.com",
				Subject:  "Email item",
				Priority: models.ChannelPriorityMedium,
			},
		},
	})
	reg.addAdapter(&fakeChannelAdapter{
		name:     "slack-team",
		chanType: models.ChannelSlack,
		items: []models.ChannelItem{
			{
				ID:       "slack-001",
				Channel:  models.ChannelSlack,
				From:     "bob",
				Subject:  "Slack item",
				Priority: models.ChannelPriorityLow,
			},
		},
	})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelInboxCmd.RunE(channelInboxCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "email-001") {
		t.Errorf("expected email item ID in output, got: %q", output)
	}
	if !strings.Contains(output, "slack-001") {
		t.Errorf("expected slack item ID in output, got: %q", output)
	}
}

func TestChannelInboxCmd_NoPendingItems(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "empty-channel",
		chanType: models.ChannelFile,
		items:    nil,
	})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelInboxCmd.RunE(channelInboxCmd, []string{"empty-channel"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "No pending items") {
		t.Errorf("expected 'No pending items' message, got: %q", output)
	}
}

func TestChannelInboxCmd_LongSubjectTruncated(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	longSubject := strings.Repeat("A", 60)
	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "test-ch",
		chanType: models.ChannelFile,
		items: []models.ChannelItem{
			{
				ID:       "trunc-001",
				Channel:  models.ChannelFile,
				From:     "sender",
				Subject:  longSubject,
				Priority: models.ChannelPriorityLow,
			},
		},
	})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelInboxCmd.RunE(channelInboxCmd, []string{"test-ch"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Subject > 50 chars should be truncated to 47 chars + "..."
	if strings.Contains(output, longSubject) {
		t.Errorf("expected long subject to be truncated, got full subject in output")
	}
	expected := strings.Repeat("A", 47) + "..."
	if !strings.Contains(output, expected) {
		t.Errorf("expected truncated subject %q in output, got: %q", expected, output)
	}
}

func TestChannelInboxCmd_LongIDAndFromTruncated(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	longID := strings.Repeat("x", 30)
	longFrom := strings.Repeat("y", 30)
	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "test-ch",
		chanType: models.ChannelFile,
		items: []models.ChannelItem{
			{
				ID:       longID,
				Channel:  models.ChannelFile,
				From:     longFrom,
				Subject:  "Short subject",
				Priority: models.ChannelPriorityLow,
			},
		},
	})
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelInboxCmd.RunE(channelInboxCmd, []string{"test-ch"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// truncate(s, 20) => s[:17] + "..."
	truncatedID := strings.Repeat("x", 17) + "..."
	truncatedFrom := strings.Repeat("y", 17) + "..."
	if !strings.Contains(output, truncatedID) {
		t.Errorf("expected truncated ID %q in output, got: %q", truncatedID, output)
	}
	if !strings.Contains(output, truncatedFrom) {
		t.Errorf("expected truncated From %q in output, got: %q", truncatedFrom, output)
	}
}

func TestChannelInboxCmd_AdapterNotFound(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	ChannelReg = newTestChannelRegistry()

	err := channelInboxCmd.RunE(channelInboxCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent adapter")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// --- channelSendCmd tests ---

func TestChannelSendCmd_NilRegistry(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()
	ChannelReg = nil

	err := channelSendCmd.RunE(channelSendCmd, []string{"adapter", "dest", "subj", "body"})
	if err == nil {
		t.Fatal("expected error when ChannelReg is nil")
	}
	if !strings.Contains(err.Error(), "channel registry not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChannelSendCmd_Success(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	adapter := &fakeChannelAdapter{
		name:     "email-main",
		chanType: models.ChannelEmail,
	}
	reg := newTestChannelRegistry()
	reg.addAdapter(adapter)
	ChannelReg = reg

	output := captureStdout(t, func() {
		err := channelSendCmd.RunE(channelSendCmd, []string{"email-main", "bob@test.com", "Hello", "Message body"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	if !strings.Contains(output, "Sent item") {
		t.Errorf("expected 'Sent item' confirmation, got: %q", output)
	}
	if !strings.Contains(output, "email-main") {
		t.Errorf("expected adapter name in confirmation, got: %q", output)
	}
	if len(adapter.sentItems) != 1 {
		t.Fatalf("expected 1 sent item, got %d", len(adapter.sentItems))
	}
	sent := adapter.sentItems[0]
	if sent.Destination != "bob@test.com" {
		t.Errorf("expected destination 'bob@test.com', got %q", sent.Destination)
	}
	if sent.Subject != "Hello" {
		t.Errorf("expected subject 'Hello', got %q", sent.Subject)
	}
	if sent.Content != "Message body" {
		t.Errorf("expected content 'Message body', got %q", sent.Content)
	}
	if sent.Channel != models.ChannelEmail {
		t.Errorf("expected channel email, got %q", sent.Channel)
	}
	if !strings.HasPrefix(sent.ID, "out-") {
		t.Errorf("expected ID starting with 'out-', got %q", sent.ID)
	}
}

func TestChannelSendCmd_AdapterNotFound(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	ChannelReg = newTestChannelRegistry()

	err := channelSendCmd.RunE(channelSendCmd, []string{"nonexistent", "dest", "subj", "body"})
	if err == nil {
		t.Fatal("expected error for nonexistent adapter")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestChannelSendCmd_SendError(t *testing.T) {
	origReg := ChannelReg
	defer func() { ChannelReg = origReg }()

	reg := newTestChannelRegistry()
	reg.addAdapter(&fakeChannelAdapter{
		name:     "broken-ch",
		chanType: models.ChannelSlack,
		sendErr:  fmt.Errorf("connection refused"),
	})
	ChannelReg = reg

	err := channelSendCmd.RunE(channelSendCmd, []string{"broken-ch", "dest", "subj", "body"})
	if err == nil {
		t.Fatal("expected error when send fails")
	}
	if !strings.Contains(err.Error(), "sending to channel broken-ch") {
		t.Errorf("expected wrapped error with adapter name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected original error in chain, got: %v", err)
	}
}
