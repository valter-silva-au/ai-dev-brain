package core

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// serenaLangByExt maps a lowercase file extension (leading dot) to the Serena
// language key whose language server handles it. Extensions absent from the map
// are ignored by the detector. Kept deliberately small and confident — Serena
// activates a language server per key, so a wrong mapping is worse than an
// omission. Extend as new stacks appear in worktrees (#201).
var serenaLangByExt = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".sh":   "bash",
	".bash": "bash",
	".py":   "python",
	".rs":   "rust",
	".java": "java",
}

// FallbackLanguage is the language key returned when a worktree has no
// recognised source files, so a provisioned .serena/project.yml is never empty
// or invalid (#201, consumed by #202). Bash is a safe neutral default: it is
// near-universal in repos and its language server tolerates arbitrary trees.
const FallbackLanguage = "bash"

// skippedDirs are directory names the worktree walk prunes so vendored/VCS
// trees don't skew the language histogram.
var skippedDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// DetectLanguagesFromCounts maps a file-extension histogram to the ordered set
// of Serena language keys, most-prevalent first. Ties break alphabetically so
// the result is deterministic. It never returns an empty slice: when nothing is
// recognised it yields [FallbackLanguage]. Pure — no IO — so it is trivially
// table-testable; DetectWorktreeLanguages is the thin filesystem walk over it.
func DetectLanguagesFromCounts(extCounts map[string]int) []string {
	byLang := make(map[string]int)
	for ext, n := range extCounts {
		if lang, ok := serenaLangByExt[strings.ToLower(ext)]; ok {
			byLang[lang] += n
		}
	}
	if len(byLang) == 0 {
		return []string{FallbackLanguage}
	}

	langs := make([]string, 0, len(byLang))
	for lang := range byLang {
		langs = append(langs, lang)
	}
	sort.Slice(langs, func(i, j int) bool {
		if byLang[langs[i]] != byLang[langs[j]] {
			return byLang[langs[i]] > byLang[langs[j]] // most files first
		}
		return langs[i] < langs[j] // stable, deterministic tie-break
	})
	return langs
}

// DetectWorktreeLanguages walks root read-only and returns the ordered Serena
// language keys present, most-prevalent first (see DetectLanguagesFromCounts).
// It performs no mutation, no network access, and no language-server invocation
// — just extension detection. VCS/vendor directories are pruned so they don't
// skew prevalence. A worktree with no recognised code yields [FallbackLanguage].
func DetectWorktreeLanguages(root string) ([]string, error) {
	extCounts := make(map[string]int)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Never prune the root itself, even if it happens to be named e.g.
			// "vendor"; only prune such directories when nested inside.
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		extCounts[strings.ToLower(filepath.Ext(d.Name()))]++
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan worktree %q: %w", root, err)
	}
	return DetectLanguagesFromCounts(extCounts), nil
}
