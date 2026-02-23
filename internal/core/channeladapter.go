package core

import (
	"fmt"
	"sync"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// ChannelAdapter defines the interface for a channel adapter.
// Each adapter handles a specific input/output channel type.
type ChannelAdapter interface {
	// Name returns the adapter's unique name.
	Name() string

	// Type returns the channel type this adapter handles.
	Type() models.ChannelType

	// Fetch retrieves pending items from the channel.
	Fetch() ([]models.ChannelItem, error)

	// Send delivers an output item to the channel.
	Send(item models.OutputItem) error

	// MarkProcessed marks an item as processed.
	MarkProcessed(itemID string) error
}

// ChannelRegistry manages registered channel adapters.
type ChannelRegistry interface {
	// Register adds a channel adapter to the registry.
	Register(adapter ChannelAdapter) error

	// GetAdapter returns the adapter with the given name.
	GetAdapter(name string) (ChannelAdapter, error)

	// ListAdapters returns all registered adapters.
	ListAdapters() []ChannelAdapter

	// FetchAll retrieves pending items from all enabled adapters.
	FetchAll() ([]models.ChannelItem, error)
}

type channelRegistry struct {
	mu       sync.RWMutex
	adapters map[string]ChannelAdapter
}

// NewChannelRegistry creates a ChannelRegistry.
func NewChannelRegistry() ChannelRegistry {
	return &channelRegistry{
		adapters: make(map[string]ChannelAdapter),
	}
}

func (r *channelRegistry) Register(adapter ChannelAdapter) error {
	if adapter == nil {
		return fmt.Errorf("registering channel adapter: adapter is nil")
	}
	name := adapter.Name()
	if name == "" {
		return fmt.Errorf("registering channel adapter: name is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("registering channel adapter: adapter %q already registered", name)
	}
	r.adapters[name] = adapter
	return nil
}

func (r *channelRegistry) GetAdapter(name string) (ChannelAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("getting channel adapter: adapter %q not found", name)
	}
	return adapter, nil
}

func (r *channelRegistry) ListAdapters() []ChannelAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ChannelAdapter, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		result = append(result, adapter)
	}
	return result
}

func (r *channelRegistry) FetchAll() ([]models.ChannelItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allItems []models.ChannelItem
	for name, adapter := range r.adapters {
		items, err := adapter.Fetch()
		if err != nil {
			return nil, fmt.Errorf("fetching from channel %q: %w", name, err)
		}
		allItems = append(allItems, items...)
	}
	return allItems, nil
}
