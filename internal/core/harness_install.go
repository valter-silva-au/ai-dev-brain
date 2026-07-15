package core

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// This file installs the embedded harness (enumerated by HarnessManifest) into a
// Claude Code config directory (~/.claude): agents under agents/, skills under
// skills/. It is what `adb sync claude-user` calls. The install is idempotent and
// refuses to silently clobber a file that a user has edited (see the action set).

// HarnessInstallAction records what happened (or, in a dry run, would happen) to
// one harness file.
type HarnessInstallAction string

const (
	// HarnessInstalled means the file was written — either new, or overwritten
	// with Force because it differed from the embedded content.
	HarnessInstalled HarnessInstallAction = "installed"
	// HarnessUnchanged means the destination already matched the embedded content.
	HarnessUnchanged HarnessInstallAction = "unchanged"
	// HarnessSkipped means the destination exists and DIFFERS from the embedded
	// content, and Force was not set — so it was left untouched (no silent clobber
	// of a user edit). Re-run with Force to overwrite.
	HarnessSkipped HarnessInstallAction = "skipped"
)

// HarnessInstallOptions modulates InstallHarness.
type HarnessInstallOptions struct {
	DryRun bool // plan only; write nothing
	Force  bool // overwrite a destination that exists but differs from the embedded content
}

// HarnessInstallEntry is the outcome for one harness file.
type HarnessInstallEntry struct {
	Kind   HarnessKind
	Dest   string // the path the file installs to (under claudeDir)
	Action HarnessInstallAction
}

// HarnessInstallResult is the summary of an InstallHarness call.
type HarnessInstallResult struct {
	ClaudeDir string
	DryRun    bool
	Entries   []HarnessInstallEntry
}

// Count returns how many entries had the given action.
func (r HarnessInstallResult) Count(action HarnessInstallAction) int {
	n := 0
	for _, e := range r.Entries {
		if e.Action == action {
			n++
		}
	}
	return n
}

// harnessDestSubdir maps a harness kind to its subdirectory under the Claude
// config dir. Claude Code loads user agents from ~/.claude/agents and user skills
// from ~/.claude/skills.
func harnessDestSubdir(kind HarnessKind) string {
	switch kind {
	case HarnessAgent:
		return "agents"
	case HarnessSkill:
		return "skills"
	default:
		return string(kind)
	}
}

// InstallHarness installs the embedded harness (HarnessManifest(fsys)) into
// claudeDir. Each file goes to <claudeDir>/<agents|skills>/<relpath>. The install
// is idempotent: a destination already equal to the embedded content is
// "unchanged"; a destination that exists but DIFFERS is "skipped" (never silently
// clobbered) unless opts.Force overwrites it; a missing destination is
// "installed". opts.DryRun plans without writing. Dirs are created 0o755, files
// written 0o644.
func InstallHarness(fsys fs.FS, claudeDir string, opts HarnessInstallOptions) (HarnessInstallResult, error) {
	if claudeDir == "" {
		return HarnessInstallResult{}, fmt.Errorf("claude config dir not resolved")
	}
	files, err := HarnessManifest(fsys)
	if err != nil {
		return HarnessInstallResult{}, err
	}
	res := HarnessInstallResult{ClaudeDir: claudeDir, DryRun: opts.DryRun}
	for _, f := range files {
		dest := filepath.Join(claudeDir, harnessDestSubdir(f.Kind), filepath.FromSlash(f.RelPath))
		action, err := installHarnessFile(dest, f.Content, opts)
		if err != nil {
			return HarnessInstallResult{}, err
		}
		res.Entries = append(res.Entries, HarnessInstallEntry{Kind: f.Kind, Dest: dest, Action: action})
	}
	return res, nil
}

// installHarnessFile decides and (unless DryRun) performs the write for one file.
func installHarnessFile(dest string, content []byte, opts HarnessInstallOptions) (HarnessInstallAction, error) {
	existing, err := os.ReadFile(dest)
	switch {
	case err == nil:
		if bytes.Equal(existing, content) {
			return HarnessUnchanged, nil
		}
		if !opts.Force {
			return HarnessSkipped, nil // exists & differs, no --force → don't clobber
		}
		// differs + Force → fall through and overwrite
	case errors.Is(err, fs.ErrNotExist):
		// new file → fall through and install
	default:
		return "", fmt.Errorf("reading %s: %w", dest, err)
	}

	if opts.DryRun {
		return HarnessInstalled, nil // planned, not written
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("creating dir for %s: %w", dest, err)
	}
	if err := os.WriteFile(dest, content, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", dest, err)
	}
	return HarnessInstalled, nil
}
