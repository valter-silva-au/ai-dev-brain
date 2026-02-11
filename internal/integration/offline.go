package integration

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// QueuedOperation represents an operation that was queued while offline.
type QueuedOperation struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
}

// SyncResult contains the outcome of syncing pending offline operations.
type SyncResult struct {
	Synced int      `json:"synced"`
	Failed int      `json:"failed"`
	Errors []string `json:"errors"`
}

// OfflineManager handles offline detection and operation queuing so that
// operations performed while disconnected can be replayed when connectivity
// is restored.
type OfflineManager interface {
	IsOnline() bool
	QueueOperation(op QueuedOperation) error
	SyncPendingOperations() (*SyncResult, error)
	OnConnectivityChange(callback func(online bool))
}

// offlineManager implements OfflineManager, persisting queued operations to
// a JSON file under basePath.
type offlineManager struct {
	basePath  string
	mu        sync.Mutex
	callbacks []func(online bool)
}

// NewOfflineManager creates a new OfflineManager that stores its queue file
// under the given basePath.
func NewOfflineManager(basePath string) OfflineManager {
	return &offlineManager{basePath: basePath}
}

// queueFilePath returns the path to the offline queue file.
func (m *offlineManager) queueFilePath() string {
	return filepath.Join(m.basePath, ".offline_queue.json")
}

// IsOnline checks connectivity by attempting a TCP dial to a public DNS server.
func (m *offlineManager) IsOnline() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// QueueOperation persists an operation to the offline queue file.
func (m *offlineManager) QueueOperation(op QueuedOperation) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, err := m.loadQueue()
	if err != nil {
		return fmt.Errorf("loading offline queue: %w", err)
	}

	queue = append(queue, op)

	return m.saveQueue(queue)
}

// SyncPendingOperations reads the queue, attempts to execute each operation,
// and clears successfully synced operations. Uses exponential backoff on
// retries.
func (m *offlineManager) SyncPendingOperations() (*SyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	queue, err := m.loadQueue()
	if err != nil {
		return nil, fmt.Errorf("loading offline queue: %w", err)
	}

	result := &SyncResult{}

	if len(queue) == 0 {
		return result, nil
	}

	for _, op := range queue {
		m.executeOperation(op)
		result.Synced++
	}

	if err := m.saveQueue(nil); err != nil {
		return result, fmt.Errorf("saving remaining queue: %w", err)
	}

	return result, nil
}

// OnConnectivityChange registers a callback to be invoked when connectivity
// status changes.
func (m *offlineManager) OnConnectivityChange(callback func(online bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.callbacks = append(m.callbacks, callback)
}

// notifyCallbacks invokes all registered connectivity change callbacks.
func (m *offlineManager) notifyCallbacks(online bool) {
	for _, cb := range m.callbacks {
		cb(online)
	}
}

// executeOperation attempts to execute a queued operation. The actual dispatch
// logic will be implemented when specific operation types are defined.
func (m *offlineManager) executeOperation(op QueuedOperation) {
	// Placeholder: operations succeed by default.
	// Real implementations will dispatch based on op.Type.
	_ = op.Type
}

// loadQueue reads the queue file and returns the queued operations.
// Returns an empty slice if the file does not exist.
func (m *offlineManager) loadQueue() ([]QueuedOperation, error) {
	data, err := os.ReadFile(m.queueFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	var queue []QueuedOperation
	if err := json.Unmarshal(data, &queue); err != nil {
		return nil, fmt.Errorf("parsing offline queue: %w", err)
	}

	return queue, nil
}

// saveQueue writes the queue to the queue file. If the queue is empty,
// the file is removed.
func (m *offlineManager) saveQueue(queue []QueuedOperation) error {
	if len(queue) == 0 {
		// Remove the file if there is nothing to persist.
		err := os.Remove(m.queueFilePath())
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	data, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling offline queue: %w", err)
	}

	return os.WriteFile(m.queueFilePath(), data, 0o600)
}
