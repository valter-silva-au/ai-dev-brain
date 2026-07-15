package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileActionRunner is the default ActionRunner. It runs `exec` actions as real
// subprocesses rooted at the workspace, and records `skill` actions as durable
// request files under automation/requests/ (the record-intent model: adb cannot
// run a Claude skill itself, so a firing leaves a request a human/agent picks
// up). It refuses to shell out for skills — no `claude` dependency, deterministic
// in CI.
type FileActionRunner struct {
	basePath string
	now      func() time.Time
}

// NewFileActionRunner returns a FileActionRunner rooted at basePath.
func NewFileActionRunner(basePath string) *FileActionRunner {
	return &FileActionRunner{basePath: basePath, now: func() time.Time { return time.Now().UTC() }}
}

// RunExec runs args[0] with args[1:] in the workspace directory and returns the
// combined output. An empty arg list is a configuration error.
func (r *FileActionRunner) RunExec(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("exec action has no command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = r.basePath
	// Mark the child so adb hooks / recursion guards can see they run under an
	// automation action, mirroring the ADB_HOOK_ACTIVE convention.
	cmd.Env = append(os.Environ(), "ADB_AUTOMATION_ACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// RecordSkillRequest writes a request file describing the skill to run and the
// payload it should run against, then returns a one-line summary naming the file.
func (r *FileActionRunner) RecordSkillRequest(rule, skill string, payload map[string]string) (string, error) {
	ts := r.now()
	dir := filepath.Join(r.basePath, "automation", "requests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create requests dir: %w", err)
	}
	// A collision-resistant, human-sortable filename: timestamp + rule slug.
	name := fmt.Sprintf("%s-%s.md", ts.Format("20060102T150405.000000000"), sanitizeFileSlug(rule))
	path := filepath.Join(dir, name)

	var b strings.Builder
	fmt.Fprintf(&b, "# Skill request: %s\n\n", skill)
	fmt.Fprintf(&b, "- rule: %s\n", rule)
	fmt.Fprintf(&b, "- skill: %s\n", skill)
	fmt.Fprintf(&b, "- requested: %s\n", ts.Format(time.RFC3339))
	if len(payload) > 0 {
		b.WriteString("- payload:\n")
		keys := make([]string, 0, len(payload))
		for k := range payload {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  - %s: %s\n", k, payload[k])
		}
	}
	b.WriteString("\nRun this skill against the payload above, then remove this file.\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write skill request: %w", err)
	}
	rel, err := filepath.Rel(r.basePath, path)
	if err != nil {
		rel = path
	}
	return fmt.Sprintf("recorded skill request for %q at %s", skill, filepath.ToSlash(rel)), nil
}

// sanitizeFileSlug reduces a rule name to a filesystem-safe slug.
func sanitizeFileSlug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "rule"
	}
	return slug
}

// FileArtifactWriter is the default ArtifactWriter. It writes rule-output
// artifacts under the workspace, refusing any relPath that escapes it (the same
// defence-in-depth the stage-gate uses for evidence paths).
type FileArtifactWriter struct {
	basePath string
}

// NewFileArtifactWriter returns a FileArtifactWriter rooted at basePath.
func NewFileArtifactWriter(basePath string) *FileArtifactWriter {
	return &FileArtifactWriter{basePath: basePath}
}

// WriteArtifact writes content to relPath under the workspace, creating parent
// directories. relPath must stay within the workspace.
func (w *FileArtifactWriter) WriteArtifact(relPath, content string) error {
	if strings.TrimSpace(relPath) == "" {
		return fmt.Errorf("artifact path is empty")
	}
	root := filepath.Clean(w.basePath)
	p := filepath.Clean(filepath.Join(root, relPath))
	if p != root && !strings.HasPrefix(p, root+string(filepath.Separator)) {
		return fmt.Errorf("artifact path %q escapes the workspace", relPath)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	return nil
}
