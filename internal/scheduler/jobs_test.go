package scheduler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateIfLarge_BelowThreshold(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(p, []byte("tiny\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rotated, err := rotateIfLarge(p, 1024, 3)
	if err != nil {
		t.Fatalf("rotateIfLarge: %v", err)
	}
	if rotated {
		t.Fatal("expected no rotation for tiny file")
	}
}

func TestRotateIfLarge_AboveThresholdKeepsHistory(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "events.jsonl")

	// Seed main file above threshold.
	if err := os.WriteFile(p, []byte(strings.Repeat("a", 2048)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// First rotation: creates .1.
	rotated, err := rotateIfLarge(p, 1024, 3)
	if err != nil || !rotated {
		t.Fatalf("first rotation: rotated=%v err=%v", rotated, err)
	}
	if _, err := os.Stat(p + ".1"); err != nil {
		t.Fatalf("expected .1 to exist: %v", err)
	}

	// Make main file large again, rotate twice more.
	for i := 0; i < 2; i++ {
		if err := os.WriteFile(p, []byte(strings.Repeat("b", 2048)), 0o644); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		if _, err := rotateIfLarge(p, 1024, 3); err != nil {
			t.Fatalf("rotate iter %d: %v", i, err)
		}
	}
	// After three rotations we expect .1, .2, .3 present.
	for _, suffix := range []string{".1", ".2", ".3"} {
		if _, err := os.Stat(p + suffix); err != nil {
			t.Fatalf("expected %s to exist: %v", p+suffix, err)
		}
	}
	// A fourth rotation must drop .3.
	if err := os.WriteFile(p, []byte(strings.Repeat("c", 2048)), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if _, err := rotateIfLarge(p, 1024, 3); err != nil {
		t.Fatalf("fourth rotate: %v", err)
	}
	if _, err := os.Stat(p + ".4"); err == nil {
		t.Fatalf(".4 should not be created (keep=3)")
	}
}

func TestRotateIfLarge_Missing(t *testing.T) {
	dir := t.TempDir()
	rotated, err := rotateIfLarge(filepath.Join(dir, "nope"), 1024, 3)
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if rotated {
		t.Fatal("expected no rotation for missing file")
	}
}
