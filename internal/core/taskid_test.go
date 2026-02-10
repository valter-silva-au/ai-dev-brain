package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateTaskID_FirstID(t *testing.T) {
	dir := t.TempDir()
	gen := NewTaskIDGenerator(dir, "TASK")

	id, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", id)
	}
}

func TestGenerateTaskID_IncrementsCounter(t *testing.T) {
	dir := t.TempDir()
	gen := NewTaskIDGenerator(dir, "TASK")

	id1, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if id1 != "TASK-00001" {
		t.Errorf("expected TASK-00001, got %s", id1)
	}

	id2, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if id2 != "TASK-00002" {
		t.Errorf("expected TASK-00002, got %s", id2)
	}
}

func TestGenerateTaskID_CustomPrefix(t *testing.T) {
	dir := t.TempDir()
	gen := NewTaskIDGenerator(dir, "PROJ")

	id, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "PROJ-00001" {
		t.Errorf("expected PROJ-00001, got %s", id)
	}
}

func TestGenerateTaskID_ReadsExistingCounter(t *testing.T) {
	dir := t.TempDir()

	// Pre-seed the counter file with 42.
	counterPath := filepath.Join(dir, ".task_counter")
	if err := os.WriteFile(counterPath, []byte("42"), 0o644); err != nil {
		t.Fatalf("failed to write counter file: %v", err)
	}

	gen := NewTaskIDGenerator(dir, "TASK")

	id, err := gen.GenerateTaskID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "TASK-00043" {
		t.Errorf("expected TASK-00043, got %s", id)
	}
}
