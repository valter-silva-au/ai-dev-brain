package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// =============================================================================
// Unit tests: QueueOperation and loadQueue/saveQueue
// =============================================================================

func TestQueueOperation_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir)

	op := QueuedOperation{
		ID:        "op-1",
		Type:      "git_push",
		Payload:   map[string]string{"repo": "test"},
		Timestamp: time.Now(),
	}

	if err := mgr.QueueOperation(op); err != nil {
		t.Fatalf("QueueOperation failed: %v", err)
	}

	// Verify file exists.
	queueFile := filepath.Join(dir, ".offline_queue.json")
	if _, err := os.Stat(queueFile); os.IsNotExist(err) {
		t.Fatal("expected queue file to exist")
	}
}

func TestQueueOperation_MultipleOperations(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir)

	for i := 0; i < 3; i++ {
		op := QueuedOperation{
			ID:        fmt.Sprintf("op-%d", i),
			Type:      "test_op",
			Timestamp: time.Now(),
		}
		if err := mgr.QueueOperation(op); err != nil {
			t.Fatalf("QueueOperation %d failed: %v", i, err)
		}
	}

	// Sync should process all 3.
	result, err := mgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("SyncPendingOperations failed: %v", err)
	}
	if result.Synced != 3 {
		t.Errorf("Synced = %d, want 3", result.Synced)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
}

// =============================================================================
// Unit tests: SyncPendingOperations
// =============================================================================

func TestSyncPendingOperations_EmptyQueue(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir)

	result, err := mgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Synced != 0 {
		t.Errorf("Synced = %d, want 0", result.Synced)
	}
	if result.Failed != 0 {
		t.Errorf("Failed = %d, want 0", result.Failed)
	}
}

func TestSyncPendingOperations_ClearsQueueOnSuccess(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir)

	op := QueuedOperation{
		ID:        "op-1",
		Type:      "test_op",
		Timestamp: time.Now(),
	}
	if err := mgr.QueueOperation(op); err != nil {
		t.Fatalf("QueueOperation failed: %v", err)
	}

	_, err := mgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("SyncPendingOperations failed: %v", err)
	}

	// Queue file should be removed.
	queueFile := filepath.Join(dir, ".offline_queue.json")
	if _, err := os.Stat(queueFile); !os.IsNotExist(err) {
		t.Error("expected queue file to be removed after successful sync")
	}

	// Second sync should show 0 operations.
	result, err := mgr.SyncPendingOperations()
	if err != nil {
		t.Fatalf("second SyncPendingOperations failed: %v", err)
	}
	if result.Synced != 0 {
		t.Errorf("Synced = %d after cleared queue, want 0", result.Synced)
	}
}

// =============================================================================
// Unit tests: OnConnectivityChange
// =============================================================================

func TestOnConnectivityChange_RegistersCallback(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir).(*offlineManager)

	called := false
	mgr.OnConnectivityChange(func(online bool) {
		called = true
	})

	if len(mgr.callbacks) != 1 {
		t.Errorf("callbacks count = %d, want 1", len(mgr.callbacks))
	}

	mgr.notifyCallbacks(true)
	if !called {
		t.Error("callback was not invoked")
	}
}

func TestOnConnectivityChange_MultipleCallbacks(t *testing.T) {
	dir := t.TempDir()
	mgr := NewOfflineManager(dir).(*offlineManager)

	count := 0
	mgr.OnConnectivityChange(func(online bool) { count++ })
	mgr.OnConnectivityChange(func(online bool) { count++ })

	mgr.notifyCallbacks(false)
	if count != 2 {
		t.Errorf("callback count = %d, want 2", count)
	}
}

// =============================================================================
// Unit tests: queue file handling
// =============================================================================

func TestLoadQueue_MissingFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := &offlineManager{basePath: dir}

	queue, err := mgr.loadQueue()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queue) != 0 {
		t.Errorf("expected empty queue, got %d items", len(queue))
	}
}

func TestLoadQueue_InvalidJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".offline_queue.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := &offlineManager{basePath: dir}
	_, err := mgr.loadQueue()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// =============================================================================
// Property 11: Offline Operation Queue and Sync
// =============================================================================

// Feature: ai-dev-brain, Property 11: Offline Operation Queue and Sync
// *For any* operation queued while offline, when connectivity is restored,
// the operation SHALL be executed and the queue SHALL be empty after successful
// sync.
//
// **Validates: Requirements 10.3, 10.5**
func TestProperty11_OfflineOperationQueueAndSync(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		dir := t.TempDir()
		mgr := NewOfflineManager(dir)

		// Generate a random number of operations.
		numOps := rapid.IntRange(1, 20).Draw(rt, "numOps")
		opTypes := []string{"git_push", "git_pull", "status_update", "context_sync"}

		for i := 0; i < numOps; i++ {
			op := QueuedOperation{
				ID:        fmt.Sprintf("op-%d", i),
				Type:      opTypes[rapid.IntRange(0, len(opTypes)-1).Draw(rt, fmt.Sprintf("opType_%d", i))],
				Payload:   map[string]int{"index": i},
				Timestamp: time.Now(),
			}
			if err := mgr.QueueOperation(op); err != nil {
				rt.Fatalf("QueueOperation %d failed: %v", i, err)
			}
		}

		// Queue file should exist.
		queueFile := filepath.Join(dir, ".offline_queue.json")
		if _, err := os.Stat(queueFile); os.IsNotExist(err) {
			rt.Fatal("queue file should exist after queuing operations")
		}

		// Sync all pending operations.
		result, err := mgr.SyncPendingOperations()
		if err != nil {
			rt.Fatalf("SyncPendingOperations failed: %v", err)
		}

		// All operations should be synced (placeholder always succeeds).
		if result.Synced != numOps {
			rt.Errorf("Synced = %d, want %d", result.Synced, numOps)
		}
		if result.Failed != 0 {
			rt.Errorf("Failed = %d, want 0", result.Failed)
		}
		if len(result.Errors) != 0 {
			rt.Errorf("Errors = %v, want empty", result.Errors)
		}

		// Queue should be empty after sync.
		if _, err := os.Stat(queueFile); !os.IsNotExist(err) {
			rt.Error("queue file should be removed after successful sync")
		}

		// A second sync should show 0 operations.
		result2, err := mgr.SyncPendingOperations()
		if err != nil {
			rt.Fatalf("second SyncPendingOperations failed: %v", err)
		}
		if result2.Synced != 0 {
			rt.Errorf("second sync Synced = %d, want 0", result2.Synced)
		}
	})
}
