package core

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// This file enumerates the Claude Code HARNESS embedded under templates/claude/:
// the adversarial devils-advocate agent (agents/**) and the founder-playbook
// skills (skills/**). Enumeration is data-driven — the set is whatever files
// exist under those trees — so adding a skill or agent is a matter of dropping a
// file into the embedded filesystem, with no Go change (the same principle as the
// pluggable projectinit scaffolding). `adb sync claude-user` consumes this
// manifest to install the harness into the user's Claude Code config.

// HarnessKind classifies a harness artifact by the embedded tree it lives in and,
// downstream, where it installs.
type HarnessKind string

const (
	// HarnessAgent is a subagent definition (agents/**), installed under the user's
	// Claude agents directory.
	HarnessAgent HarnessKind = "agent"
	// HarnessSkill is a skill (skills/**, typically a per-skill dir holding SKILL.md),
	// installed under the user's Claude skills directory.
	HarnessSkill HarnessKind = "skill"
)

// harnessRoots maps each kind to its root directory inside the embedded template
// filesystem. embed.FS paths always use forward slashes. Agents are listed before
// skills so the manifest is deterministic.
var harnessRoots = []struct {
	kind HarnessKind
	root string
}{
	{HarnessAgent, "agents"},
	{HarnessSkill, "skills"},
}

// HarnessFile is one embedded harness artifact: its kind, its path relative to the
// kind's root (forward-slashed — e.g. "devils-advocate.md" or "stage-gate/SKILL.md"),
// and its raw content.
type HarnessFile struct {
	Kind    HarnessKind
	RelPath string
	Content []byte
}

// HarnessManifest enumerates every embedded agent and skill file in fsys, grouped
// by kind (agents then skills) and lexically ordered within each kind (fs.WalkDir
// walks in lexical order), so the result is deterministic. A kind whose root
// directory is absent contributes no files rather than erroring, so callers may
// pass any fs.FS (a synthetic one in tests, claude.FS in production).
func HarnessManifest(fsys fs.FS) ([]HarnessFile, error) {
	var files []HarnessFile
	for _, hr := range harnessRoots {
		if err := fs.WalkDir(fsys, hr.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				// A missing root just means this kind ships no files — not an error.
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			content, rerr := fs.ReadFile(fsys, p)
			if rerr != nil {
				return fmt.Errorf("reading harness file %s: %w", p, rerr)
			}
			files = append(files, HarnessFile{
				Kind:    hr.kind,
				RelPath: strings.TrimPrefix(p, hr.root+"/"),
				Content: content,
			})
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walking harness %s tree: %w", hr.kind, err)
		}
	}
	return files, nil
}
