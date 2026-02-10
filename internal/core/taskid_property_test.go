package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ai-dev-brain, Property 1: Task ID Uniqueness
// Every call to GenerateTaskID must produce a unique ID.
func TestProperty_TaskIDUniqueness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(2, 100).Draw(rt, "n")
		prefix := rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "prefix")

		dir, err := os.MkdirTemp("", "taskid-property-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(dir)

		gen := NewTaskIDGenerator(dir, prefix)

		seen := make(map[string]struct{}, n)
		for i := 0; i < n; i++ {
			id, err := gen.GenerateTaskID()
			if err != nil {
				t.Fatalf("GenerateTaskID failed on call %d: %v", i+1, err)
			}
			if _, exists := seen[id]; exists {
				t.Fatalf("duplicate task ID %q on call %d", id, i+1)
			}
			seen[id] = struct{}{}
		}

		// Verify counter file has correct final value.
		data, err := os.ReadFile(filepath.Join(dir, ".task_counter"))
		if err != nil {
			t.Fatalf("failed to read counter file: %v", err)
		}
		expected := fmt.Sprintf("%d", n)
		if string(data) != expected {
			t.Fatalf("expected counter file to contain %s, got %s", expected, string(data))
		}
	})
}
