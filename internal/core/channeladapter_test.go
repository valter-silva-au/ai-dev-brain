package core

import (
	"fmt"
	"testing"

	"github.com/drapaimern/ai-dev-brain/pkg/models"
)

// fakeChannelAdapter is a test double for ChannelAdapter.
type fakeChannelAdapter struct {
	name      string
	typ       models.ChannelType
	items     []models.ChannelItem
	fetchErr  error
	sendErr   error
	markErr   error
	sentItems []models.OutputItem
	marked    []string
}

func (f *fakeChannelAdapter) Name() string             { return f.name }
func (f *fakeChannelAdapter) Type() models.ChannelType { return f.typ }
func (f *fakeChannelAdapter) Fetch() ([]models.ChannelItem, error) {
	return f.items, f.fetchErr
}
func (f *fakeChannelAdapter) Send(item models.OutputItem) error {
	f.sentItems = append(f.sentItems, item)
	return f.sendErr
}
func (f *fakeChannelAdapter) MarkProcessed(itemID string) error {
	f.marked = append(f.marked, itemID)
	return f.markErr
}

func TestChannelRegistry_Register(t *testing.T) {
	t.Run("registers adapter successfully", func(t *testing.T) {
		reg := NewChannelRegistry()
		adapter := &fakeChannelAdapter{name: "test", typ: models.ChannelFile}

		err := reg.Register(adapter)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := reg.GetAdapter("test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name() != "test" {
			t.Errorf("got name %q, want %q", got.Name(), "test")
		}
	})

	t.Run("rejects nil adapter", func(t *testing.T) {
		reg := NewChannelRegistry()
		err := reg.Register(nil)
		if err == nil {
			t.Fatal("expected error for nil adapter")
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		reg := NewChannelRegistry()
		adapter := &fakeChannelAdapter{name: "", typ: models.ChannelFile}
		err := reg.Register(adapter)
		if err == nil {
			t.Fatal("expected error for empty name")
		}
	})

	t.Run("rejects duplicate registration", func(t *testing.T) {
		reg := NewChannelRegistry()
		adapter := &fakeChannelAdapter{name: "dup", typ: models.ChannelFile}
		_ = reg.Register(adapter)

		err := reg.Register(adapter)
		if err == nil {
			t.Fatal("expected error for duplicate registration")
		}
	})
}

func TestChannelRegistry_GetAdapter(t *testing.T) {
	t.Run("returns error for unknown adapter", func(t *testing.T) {
		reg := NewChannelRegistry()
		_, err := reg.GetAdapter("nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown adapter")
		}
	})
}

func TestChannelRegistry_ListAdapters(t *testing.T) {
	t.Run("returns empty list when no adapters registered", func(t *testing.T) {
		reg := NewChannelRegistry()
		adapters := reg.ListAdapters()
		if len(adapters) != 0 {
			t.Errorf("got %d adapters, want 0", len(adapters))
		}
	})

	t.Run("returns all registered adapters", func(t *testing.T) {
		reg := NewChannelRegistry()
		_ = reg.Register(&fakeChannelAdapter{name: "a", typ: models.ChannelFile})
		_ = reg.Register(&fakeChannelAdapter{name: "b", typ: models.ChannelEmail})

		adapters := reg.ListAdapters()
		if len(adapters) != 2 {
			t.Errorf("got %d adapters, want 2", len(adapters))
		}
	})
}

func TestChannelRegistry_FetchAll(t *testing.T) {
	t.Run("aggregates items from all adapters", func(t *testing.T) {
		reg := NewChannelRegistry()
		_ = reg.Register(&fakeChannelAdapter{
			name: "a",
			typ:  models.ChannelFile,
			items: []models.ChannelItem{
				{ID: "item-1", Subject: "First"},
			},
		})
		_ = reg.Register(&fakeChannelAdapter{
			name: "b",
			typ:  models.ChannelEmail,
			items: []models.ChannelItem{
				{ID: "item-2", Subject: "Second"},
				{ID: "item-3", Subject: "Third"},
			},
		})

		items, err := reg.FetchAll()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(items) != 3 {
			t.Errorf("got %d items, want 3", len(items))
		}
	})

	t.Run("returns error when adapter fetch fails", func(t *testing.T) {
		reg := NewChannelRegistry()
		_ = reg.Register(&fakeChannelAdapter{
			name:     "bad",
			typ:      models.ChannelFile,
			fetchErr: errForTest,
		})

		_, err := reg.FetchAll()
		if err == nil {
			t.Fatal("expected error from failing adapter")
		}
	})

	t.Run("returns nil when no adapters registered", func(t *testing.T) {
		reg := NewChannelRegistry()
		items, err := reg.FetchAll()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if items != nil {
			t.Errorf("got %v, want nil", items)
		}
	})
}

var errForTest = fmt.Errorf("test error")
