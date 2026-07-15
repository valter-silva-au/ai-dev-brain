package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectLanguagesFromCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   []string
	}{
		{
			name:   "Go-heavy",
			counts: map[string]int{".go": 40, ".sh": 2},
			want:   []string{"go", "bash"},
		},
		{
			name:   "TS-heavy (ts + tsx fold to typescript)",
			counts: map[string]int{".ts": 10, ".tsx": 15, ".go": 1},
			want:   []string{"typescript", "go"}, // 25 ts vs 1 go
		},
		{
			name:   "mixed ordered by prevalence (go=9, python=5, bash=3)",
			counts: map[string]int{".py": 5, ".go": 9, ".sh": 3},
			want:   []string{"go", "python", "bash"},
		},
		{
			name:   "none recognised falls back",
			counts: map[string]int{".md": 12, ".txt": 3, ".png": 1},
			want:   []string{FallbackLanguage},
		},
		{
			name:   "empty falls back",
			counts: map[string]int{},
			want:   []string{FallbackLanguage},
		},
		{
			name:   "unknown extensions ignored",
			counts: map[string]int{".go": 2, ".rb": 99, ".exe": 5},
			want:   []string{"go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLanguagesFromCounts(tt.counts)
			if len(got) == 0 {
				t.Fatalf("DetectLanguagesFromCounts returned empty slice (must never be empty)")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DetectLanguagesFromCounts(%v) = %v, want %v", tt.counts, got, tt.want)
			}
		})
	}
}

func TestDetectLanguagesFromCounts_TieBreakIsAlphabetical(t *testing.T) {
	// go and python tie at 3 files each → alphabetical order (go before python).
	got := DetectLanguagesFromCounts(map[string]int{".go": 3, ".py": 3})
	want := []string{"go", "python"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tie-break: got %v, want %v", got, want)
	}
}

func TestDetectWorktreeLanguages_Walk(t *testing.T) {
	root := t.TempDir()
	// A Go-heavy tree with a shell script and some noise, plus vendored/VCS
	// dirs that must NOT skew the histogram.
	writeFiles(t, root, map[string]string{
		"main.go":                 "package main",
		"internal/app.go":         "package internal",
		"internal/util.go":        "package internal",
		"scripts/build.sh":        "#!/bin/sh",
		"README.md":               "# docs",
		"node_modules/x/index.ts": "export const x = 1", // pruned
		"vendor/y/y.go":           "package y",          // pruned
		".git/config":             "[core]",             // pruned
	})

	got, err := DetectWorktreeLanguages(root)
	if err != nil {
		t.Fatalf("DetectWorktreeLanguages failed: %v", err)
	}
	// 3 real .go vs 1 real .sh; the node_modules .ts and vendor .go are pruned.
	want := []string{"go", "bash"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DetectWorktreeLanguages = %v, want %v", got, want)
	}
}

func TestDetectWorktreeLanguages_NoCodeFallsBack(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"notes.md":  "# notes",
		"data.json": "{}",
	})

	got, err := DetectWorktreeLanguages(root)
	if err != nil {
		t.Fatalf("DetectWorktreeLanguages failed: %v", err)
	}
	if !reflect.DeepEqual(got, []string{FallbackLanguage}) {
		t.Errorf("DetectWorktreeLanguages (no code) = %v, want [%s]", got, FallbackLanguage)
	}
}

// writeFiles creates each relative path (with its parent dirs) under root.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}
