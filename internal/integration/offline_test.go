package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLoadQueue_EmptyFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".offline_queue.json"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := &offlineManager{basePath: dir}
	queue, err := mgr.loadQueue()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queue) != 0 {
		t.Errorf("expected empty queue, got %d items", len(queue))
	}
}

func TestLoadQueue_UnreadableFile_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	dir := t.TempDir()
	queueFile := filepath.Join(dir, ".offline_queue.json")
	if err := os.WriteFile(queueFile, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make file unreadable.
	if err := os.Chmod(queueFile, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(queueFile, 0o644) }()

	mgr := &offlineManager{basePath: dir}
	_, err := mgr.loadQueue()
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestSaveQueue_Empty_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	queueFile := filepath.Join(dir, ".offline_queue.json")

	// Create queue file first.
	if err := os.WriteFile(queueFile, []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := &offlineManager{basePath: dir}
	if err := mgr.saveQueue(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should be removed.
	if _, err := os.Stat(queueFile); !os.IsNotExist(err) {
		t.Error("expected queue file to be removed")
	}
}

func TestSaveQueue_Empty_NonexistentFile_NoError(t *testing.T) {
	dir := t.TempDir()
	mgr := &offlineManager{basePath: dir}

	// Saving an empty queue when the file doesn't exist should not error.
	if err := mgr.saveQueue(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSaveQueue_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	mgr := &offlineManager{basePath: dir}

	queue := []QueuedOperation{
		{ID: "op-1", Type: "test", Timestamp: time.Now()},
	}

	if err := mgr.saveQueue(queue); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back and verify.
	loaded, err := mgr.loadQueue()
	if err != nil {
		t.Fatalf("unexpected error reading back: %v", err)
	}
	if len(loaded) != 1 {
		t.Errorf("expected 1 operation, got %d", len(loaded))
	}
	if loaded[0].ID != "op-1" {
		t.Errorf("loaded[0].ID = %q, want %q", loaded[0].ID, "op-1")
	}
}

func TestQueueOperation_LoadError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	queueFile := filepath.Join(dir, ".offline_queue.json")
	// Write invalid JSON so loadQueue fails.
	if err := os.WriteFile(queueFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewOfflineManager(dir)
	err := mgr.QueueOperation(QueuedOperation{
		ID:   "op-1",
		Type: "test",
	})
	if err == nil {
		t.Fatal("expected error when loadQueue fails")
	}
}

func TestSyncPendingOperations_LoadError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	queueFile := filepath.Join(dir, ".offline_queue.json")
	if err := os.WriteFile(queueFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewOfflineManager(dir)
	_, err := mgr.SyncPendingOperations()
	if err == nil {
		t.Fatal("expected error when loadQueue fails")
	}
}

func TestIsOnline_ReturnsBool(t *testing.T) {
	// IsOnline does a real network dial, so we just verify it returns
	// without panicking. The result depends on actual connectivity.
	mgr := NewOfflineManager(t.TempDir())
	_ = mgr.IsOnline()
}

func TestSaveQueue_ReadOnlyDir_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	dir := t.TempDir()
	// Make directory read-only so WriteFile fails.
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	mgr := &offlineManager{basePath: dir}
	err := mgr.saveQueue([]QueuedOperation{
		{ID: "op-1", Type: "test", Timestamp: time.Now()},
	})
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestSaveQueue_RemoveError(t *testing.T) {
	// Test that saveQueue with an empty queue returns error if os.Remove fails
	// for a reason other than file-not-exist.
	dir := t.TempDir()

	// Create the queue file as a directory so os.Remove fails.
	queuePath := filepath.Join(dir, ".offline_queue.json")
	if err := os.MkdirAll(queuePath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a file inside so rmdir fails.
	if err := os.WriteFile(filepath.Join(queuePath, "dummy"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := &offlineManager{basePath: dir}
	err := mgr.saveQueue(nil) // empty queue -> tries os.Remove
	if err == nil {
		t.Fatal("expected error when os.Remove fails on a directory")
	}
}

func TestSaveQueue_MarshalError(t *testing.T) {
	// Test the marshal error path in saveQueue by providing an unmarshalable payload.
	// JSON marshal fails for channels, functions, etc.
	dir := t.TempDir()
	mgr := &offlineManager{basePath: dir}

	// Create an operation with a payload that can't be marshaled (a channel).
	queue := []QueuedOperation{
		{
			ID:        "op-1",
			Type:      "test",
			Payload:   make(chan int), // channels cannot be marshaled to JSON
			Timestamp: time.Now(),
		},
	}

	err := mgr.saveQueue(queue)
	if err == nil {
		t.Fatal("expected error when marshalling fails")
	}
	if !strings.Contains(err.Error(), "marshalling offline queue") {
		t.Errorf("error = %q, want to contain 'marshalling offline queue'", err.Error())
	}
}

func TestSyncPendingOperations_SaveQueueErrorAfterSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not available on Windows")
	}
	// When SyncPendingOperations processes all operations and then tries to
	// save an empty queue (via saveQueue(nil)), if that save fails, it should
	// still return the result with the synced count but also an error.
	dir := t.TempDir()

	// Manually create the offlineManager and write a valid queue file.
	mgr := &offlineManager{basePath: dir}
	queue := []QueuedOperation{
		{ID: "op-1", Type: "test", Timestamp: time.Now()},
	}
	if err := mgr.saveQueue(queue); err != nil {
		t.Fatal(err)
	}

	queuePath := filepath.Join(dir, ".offline_queue.json")

	// Now replace the queue file with a directory so os.Remove fails.
	if err := os.Remove(queuePath); err != nil {
		t.Fatal(err)
	}
	// Re-write the queue data to a temporary file so loadQueue works.
	// We need loadQueue to succeed but saveQueue(nil) to fail.
	// Strategy: write a valid queue file, then lock it by replacing dir perms.

	// Actually, the simplest approach: the queue file needs to be readable
	// for loadQueue, but the directory needs to prevent os.Remove for saveQueue.
	// This is tricky because os.Remove requires write permissions on the parent.

	// Write valid JSON to the queue file first.
	if err := mgr.saveQueue(queue); err != nil {
		t.Fatal(err)
	}

	// Make the directory read-only so os.Remove fails in saveQueue(nil).
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	result, err := mgr.SyncPendingOperations()
	if err == nil {
		t.Fatal("expected error when saveQueue fails during sync")
	}
	// The operations should still have been synced even though save failed.
	if result.Synced != 1 {
		t.Errorf("Synced = %d, want 1", result.Synced)
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
