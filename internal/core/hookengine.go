package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
)

// EvidenceGateConfig opts the HookEngine into the evidence-read gate.
// When Enabled is true, the gate:
//   - records Read tool calls whose file_path matches any ReadPattern
//     to an append-only tracker at basePath/.adb/evidence_reads,
//   - blocks Write/Edit tool calls whose file_path matches any WritePath
//     unless a matching ReadPattern entry already exists in the tracker.
//
// Paths and patterns use filepath.Match semantics (*, ?, [range]); they
// do not support recursive `**`. Matching is done on the forward-slash
// form of the input path, so both `screenshots/foo.png` and
// `screenshots\foo.png` match `screenshots/*.png`.
type EvidenceGateConfig struct {
	Enabled      bool
	WritePaths   []string
	ReadPatterns []string
}

// OperatorConfig opts into operator-in-the-loop controls over long-running
// agents: a kill-switch (KillSwitchFile present at basePath → block all
// PreToolUse), and mid-run steering (SteerFile at basePath → surface its
// contents on stderr once and consume the file). These correspond to the
// kill-switch.sh and steer.sh patterns from
// anthropics/cwc-long-running-agents.
//
// Empty file names mean the feature is disabled, even when the containing
// struct's zero value is otherwise in play. NewHookEngine installs the
// conventional names (AGENT_STOP, STEER.md) on callers who opt in but do
// not override them.
type OperatorConfig struct {
	KillSwitchEnabled bool
	KillSwitchFile    string
	SteerEnabled      bool
	SteerFile         string
}

// MemoryIndexer is the minimal surface the HookEngine needs from the
// vector-memory subsystem. It's an interface (not a concrete
// *memory.SQLiteStore) so tests can inject a fake, and so HookEngine
// doesn't import internal/memory (which would create a long import
// chain through SQLite into core). The real wiring happens in app.go
// where both packages are already stitched together.
type MemoryIndexer interface {
	Upsert(ctx context.Context, ns, key, content string, meta map[string]string) error
}

// MemoryHookConfig configures how the HookEngine uses the memory
// subsystem. Enabled == false means no auto-indexing and no similar-
// work hints, regardless of any non-nil Indexer (so disabling at config
// level guarantees zero writes).
type MemoryHookConfig struct {
	Enabled bool
	Indexer MemoryIndexer
}

// SpecGateConfig opts the HookEngine into the spec-gate: a block on Write/Edit
// to guarded WritePaths until a required architecture decision/spec exists. It
// reuses the evidence-gate shape (guarded write paths + a precondition) but the
// precondition is "an accepted ADR exists", checked via the injected HasSpec
// closure so core needn't import storage. A nil HasSpec while Enabled fails
// SAFE-AND-LOUD: the gate treats "no checker" as "no spec" and blocks, surfacing
// the misconfiguration rather than silently letting writes through.
type SpecGateConfig struct {
	Enabled    bool
	WritePaths []string
	HasSpec    func() (bool, error)
}

// HookEngineOptions carries opt-in behaviours for HookEngine. Zero value
// is the legacy behaviour: no cwc-long-running-agents features active.
type HookEngineOptions struct {
	Evidence EvidenceGateConfig
	Operator OperatorConfig
	Memory   MemoryHookConfig
	SpecGate SpecGateConfig
}

// operatorWithDefaults fills unset file names with the conventional
// defaults when the feature is enabled. Lets callers opt-in with a
// single Enabled: true.
func operatorWithDefaults(op OperatorConfig) OperatorConfig {
	if op.KillSwitchEnabled && op.KillSwitchFile == "" {
		op.KillSwitchFile = "AGENT_STOP"
	}
	if op.SteerEnabled && op.SteerFile == "" {
		op.SteerFile = "STEER.md"
	}
	return op
}

// HookEngine processes Claude Code hook events with hybrid shell/Go architecture
type HookEngine struct {
	basePath string
	tracker  *hooks.ChangeTracker
	evidence *hooks.EvidenceTracker
	opts     HookEngineOptions
}

// NewHookEngine creates a new hook engine with legacy options.
// Equivalent to NewHookEngineWithOptions(basePath, HookEngineOptions{}).
func NewHookEngine(basePath string) *HookEngine {
	return NewHookEngineWithOptions(basePath, HookEngineOptions{})
}

// NewHookEngineWithOptions creates a hook engine with explicit options.
func NewHookEngineWithOptions(basePath string, opts HookEngineOptions) *HookEngine {
	opts.Operator = operatorWithDefaults(opts.Operator)
	return &HookEngine{
		basePath: basePath,
		tracker:  hooks.NewChangeTracker(basePath),
		evidence: hooks.NewEvidenceTracker(basePath),
		opts:     opts,
	}
}

// PreventRecursion checks if we're in a recursive hook invocation
func (he *HookEngine) PreventRecursion() bool {
	return os.Getenv("ADB_HOOK_ACTIVE") == "1"
}

// normalisePath returns a cleaned, forward-slash form of path so that
// subsequent string matching is consistent across operating systems.
// Windows Claude Code hook events can carry backslash-separated paths
// (e.g. `C:\Users\me\repo\vendor\foo.go`); without normalisation the
// vendor/go.sum guard silently no-ops on Windows.
//
// The explicit backslash→slash replacement runs on *every* OS: filepath.Clean
// only treats `\` as a separator on Windows, so a Windows-style path processed
// on Linux (e.g. a hook event replayed in CI) would otherwise pass through
// unchanged and defeat the guard. ToSlash then guarantees a forward-slash
// result regardless of the host separator.
func normalisePath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(strings.ReplaceAll(path, `\`, "/")))
}

// ProcessPreToolUse handles PreToolUse hooks - blocking validation
func (he *HookEngine) ProcessPreToolUse(event *hooks.PreToolUseEvent) error {
	// Kill-switch runs *before* the recursion guard: an operator halt
	// must win even when a hook invokes itself. A visible message tells
	// the operator exactly which file to remove to resume.
	if he.opts.Operator.KillSwitchEnabled {
		path := filepath.Join(he.basePath, he.opts.Operator.KillSwitchFile)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("blocked: operator kill-switch active (remove %q to resume)", path)
		}
	}

	if he.PreventRecursion() {
		return nil
	}

	// Operator steering: surface a one-shot message from STEER.md on
	// stderr, then consume the file via os.Rename so concurrent
	// invocations don't double-read or double-delete. Skipped inside
	// recursion to avoid duplicate prints in the same logical tool use.
	if he.opts.Operator.SteerEnabled {
		he.consumeSteerFile()
	}

	filePath, hasFilePath := event.Parameters["file_path"].(string)
	normalised := ""
	if hasFilePath {
		normalised = normalisePath(filePath)
	}

	// Block edits to vendor/ and go.sum
	if event.ToolName == "Edit" || event.ToolName == "Write" {
		if hasFilePath {
			if strings.Contains(normalised, "/vendor/") || strings.HasSuffix(normalised, "/go.sum") || normalised == "go.sum" {
				return fmt.Errorf("blocked: modifications to vendor/ and go.sum are not allowed")
			}
		}
	}

	// Evidence-read gate: record matching reads; block writes to guarded
	// paths that have no matching read on record.
	if he.opts.Evidence.Enabled && hasFilePath {
		if event.ToolName == "Read" {
			if matchesAny(normalised, he.opts.Evidence.ReadPatterns) {
				if err := he.evidence.Record(normalised); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to record evidence read: %v\n", err)
				}
			}
		}
		if event.ToolName == "Edit" || event.ToolName == "Write" {
			if matchesAny(normalised, he.opts.Evidence.WritePaths) {
				if !he.hasMatchingEvidence() {
					return fmt.Errorf("blocked: writing to %q requires a prior Read of a file matching one of %v (evidence-gate)", filePath, he.opts.Evidence.ReadPatterns)
				}
			}
		}
	}

	// Spec-gate: block writes to guarded paths until a required architecture
	// decision/spec exists (an accepted ADR).
	if he.opts.SpecGate.Enabled && hasFilePath && (event.ToolName == "Edit" || event.ToolName == "Write") {
		if matchesAny(normalised, he.opts.SpecGate.WritePaths) {
			ok := false
			if he.opts.SpecGate.HasSpec != nil {
				has, err := he.opts.SpecGate.HasSpec()
				if err != nil {
					return fmt.Errorf("spec-gate check failed for %q: %w", filePath, err)
				}
				ok = has
			}
			if !ok {
				return fmt.Errorf("blocked: writing to %q requires an accepted architecture decision (spec-gate) — record one with `adb adr new` and `adb adr set-status <n> accepted`", filePath)
			}
		}
	}

	return nil
}

// matchesAny reports whether path matches any of the filepath.Match
// patterns. Invalid patterns are treated as non-matches (they surface
// in tests).
func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if matched, err := filepath.Match(p, path); err == nil && matched {
			return true
		}
		// Also try matching just the basename, so callers can write
		// `*.png` without worrying about leading directories.
		if matched, err := filepath.Match(p, filepath.Base(path)); err == nil && matched {
			return true
		}
	}
	return false
}

// consumeSteerFile looks for the configured SteerFile at basePath and,
// if present, atomically renames it to `<name>.consumed` before reading
// and printing its contents. The rename is the concurrency primitive:
// on all three supported OSes it is atomic, so at most one concurrent
// hook invocation observes the rename succeeding. The other gets
// os.ErrNotExist from the subsequent ReadFile and silently returns.
// Failures are non-fatal; they log to stderr and do not block the tool
// call.
func (he *HookEngine) consumeSteerFile() {
	src := filepath.Join(he.basePath, he.opts.Operator.SteerFile)
	dst := src + ".consumed"

	// os.Rename with a non-existent src returns os.ErrNotExist.
	if err := os.Rename(src, dst); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: failed to consume steer file: %v\n", err)
		}
		return
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read consumed steer file: %v\n", err)
		return
	}
	msg := strings.TrimRight(string(data), "\r\n \t")
	if msg == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "OPERATOR STEERING: %s\n", msg)
}

// hasMatchingEvidence reports whether any previously-recorded evidence
// read matches one of the configured ReadPatterns. Used by the
// evidence-read gate.
func (he *HookEngine) hasMatchingEvidence() bool {
	reads, err := he.evidence.Reads()
	if err != nil || len(reads) == 0 {
		return false
	}
	for _, r := range reads {
		if matchesAny(r, he.opts.Evidence.ReadPatterns) {
			return true
		}
	}
	return false
}

// ProcessPostToolUse handles PostToolUse hooks - non-blocking actions
func (he *HookEngine) ProcessPostToolUse(event *hooks.PostToolUseEvent) error {
	if he.PreventRecursion() {
		return nil
	}

	// Auto-format Go files after Edit/Write
	if event.ToolName == "Edit" || event.ToolName == "Write" {
		if filePath, ok := event.Parameters["file_path"].(string); ok {
			if strings.HasSuffix(filePath, ".go") {
				if err := he.formatGoFile(filePath); err != nil {
					// Non-blocking: log warning but don't fail
					fmt.Fprintf(os.Stderr, "Warning: failed to format %s: %v\n", filePath, err)
				}
			}

			// Track the change
			operation := "modified"
			if event.ToolName == "Write" {
				operation = "created"
			}
			if err := he.tracker.TrackChange(filePath, operation); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to track change: %v\n", err)
			}
		}
	}

	return nil
}

// ProcessStop handles Stop hooks - advisory checks
func (he *HookEngine) ProcessStop() error {
	if he.PreventRecursion() {
		return nil
	}

	warnings := []string{}

	// Check for uncommitted changes
	if hasUncommitted, err := he.hasUncommittedChanges(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to check git status: %v\n", err)
	} else if hasUncommitted {
		warnings = append(warnings, "Uncommitted changes detected")
	}

	// Check build status
	if err := he.checkBuild(); err != nil {
		warnings = append(warnings, fmt.Sprintf("Build check failed: %v", err))
	}

	// Check go vet
	if err := he.checkVet(); err != nil {
		warnings = append(warnings, fmt.Sprintf("go vet found issues: %v", err))
	}

	// Update context.md with session summary
	if err := he.updateContextOnStop(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update context: %v\n", err)
	}

	// Print warnings (advisory only)
	if len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠️  Advisory warnings:\n")
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  - %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "\n")
	}

	return nil
}

// ProcessTaskCompleted handles TaskCompleted hooks - two-phase approach
func (he *HookEngine) ProcessTaskCompleted(event *hooks.TaskCompletedEvent) error {
	if he.PreventRecursion() {
		return nil
	}

	// Phase A: Blocking quality gates
	if err := he.phaseAQualityGates(event); err != nil {
		return fmt.Errorf("quality gate failed: %w", err)
	}

	// Phase B: Non-blocking knowledge extraction
	if err := he.phaseBKnowledgeExtraction(event); err != nil {
		// Non-blocking: log but continue
		fmt.Fprintf(os.Stderr, "Warning: knowledge extraction failed: %v\n", err)
	}

	// Auto-index the task's knowledge.yaml + notes into vector memory.
	// Non-blocking: any failure logs and continues.
	he.indexTaskIntoMemory(event.TaskID)

	return nil
}

// indexSessionIntoMemory upserts a captured transcript into the
// vector-memory namespace sessions/<session-id>. No-op when memory is
// disabled, indexer is nil, or the session id / transcript is empty.
func (he *HookEngine) indexSessionIntoMemory(sessionID, transcript string) {
	if !he.opts.Memory.Enabled || he.opts.Memory.Indexer == nil {
		return
	}
	if sessionID == "" || transcript == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ns := "sessions/" + sessionID
	meta := map[string]string{"source": "session-end"}
	if err := he.opts.Memory.Indexer.Upsert(ctx, ns, "transcript", transcript, meta); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: memory auto-index %s/transcript: %v\n", ns, err)
	}
}

// indexTaskIntoMemory reads knowledge.yaml and notes.md from the ticket
// directory and upserts them into the vector-memory namespace
// tickets/<task-id>. No-op when memory is disabled or the indexer is
// nil. All errors are logged (non-blocking) so quality-gate success is
// never conflated with memory health.
func (he *HookEngine) indexTaskIntoMemory(taskID string) {
	if !he.opts.Memory.Enabled || he.opts.Memory.Indexer == nil || taskID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ns := "tickets/" + taskID
	meta := map[string]string{"source": "task-completed"}

	taskDir := filepath.Join(he.basePath, "tickets", taskID)
	for _, fname := range []string{"knowledge.yaml", "notes.md", "context.md"} {
		path := filepath.Join(taskDir, fname)
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		key := fname
		if err := he.opts.Memory.Indexer.Upsert(ctx, ns, key, string(data), meta); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: memory auto-index %s/%s: %v\n", ns, key, err)
		}
	}
}

// ProcessSessionEnd handles SessionEnd hooks
func (he *HookEngine) ProcessSessionEnd(event *hooks.SessionEndEvent) error {
	if he.PreventRecursion() {
		return nil
	}

	// Capture transcript if available
	var transcriptStr string
	if transcript, ok := event.Metadata["transcript"].(string); ok {
		transcriptStr = transcript
		taskID := he.getCurrentTaskID()
		if taskID != "" {
			taskDir := filepath.Join(he.basePath, "tickets", taskID)
			if err := hooks.CaptureTranscript(taskDir, event.SessionID, transcript); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to capture transcript: %v\n", err)
			}
		}
	}

	// Auto-index the transcript into vector memory under
	// sessions/<session-id>. Non-blocking: failures log and continue.
	he.indexSessionIntoMemory(event.SessionID, transcriptStr)

	// Update context.md with session summary
	if err := he.updateContextOnSessionEnd(event); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update context: %v\n", err)
	}

	// Clear the evidence-read tracker at session boundary. Safe even
	// when the evidence gate is disabled; Clear is a no-op if the file
	// does not exist.
	if err := he.evidence.Clear(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to clear evidence tracker: %v\n", err)
	}

	return nil
}

// formatGoFile runs gofmt on a Go file. If the file does not exist at
// the given path (e.g. a Git-Bash-style `/tmp/...` path fed on Windows,
// which Go's os.* resolves natively), log a clear warning and return
// nil rather than letting gofmt fail with an unhelpful exit-2 error.
// Real Claude Code hook events carry Windows-form paths where this
// code path works as-is; the guard just makes the test/script failure
// mode honest on mismatched-path-convention inputs.
func (he *HookEngine) formatGoFile(filePath string) error {
	resolved, err := resolveFilePath(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr,
				"Warning: skipping gofmt: file_path %q does not resolve on this platform\n",
				filePath)
			return nil
		}
		return fmt.Errorf("stat %q before gofmt: %w", filePath, err)
	}
	cmd := exec.Command("gofmt", "-w", resolved)
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	return cmd.Run()
}

// resolveFilePath os.Stat's filePath and returns a usable, native path.
// On Windows it also tries translating Git-Bash mount-style paths
// (/c/Users/..., /tmp/...) into their native `C:\Users\...` form when
// the direct path doesn't resolve — Claude Code's own Windows tool
// events carry Windows-form paths, but scripts / tests / Git Bash
// pipelines sometimes pass the mount form. Returns the original err
// (usually os.ErrNotExist) if no translation works.
func resolveFilePath(filePath string) (string, error) {
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	// On non-Windows, Git-Bash-style paths ARE native — no translation
	// to try.
	if runtime.GOOS != "windows" {
		return "", os.ErrNotExist
	}
	translated, ok := translateGitBashPath(filePath)
	if !ok {
		return "", os.ErrNotExist
	}
	if _, err := os.Stat(translated); err == nil {
		return translated, nil
	}
	return "", os.ErrNotExist
}

// translateGitBashPath maps Git Bash mount paths to Windows native form.
// Handles two common shapes:
//
//   - `/c/Users/...`, `/d/Projects/...` → `C:\Users\...`, `D:\Projects\...`
//     (Git Bash mounts drive letters as top-level single-letter dirs.)
//   - `/tmp/...` → `%TEMP%\...` via the TEMP env var (or USERPROFILE\AppData
//     \Local\Temp on Windows by convention).
//
// Returns (nativePath, true) on successful translation, ("", false)
// when the input doesn't look like a mount path. Best-effort; a false
// return just means "I don't know how to translate this; let the caller
// fall back".
func translateGitBashPath(p string) (string, bool) {
	if p == "" || !strings.HasPrefix(p, "/") {
		return "", false
	}
	// This helper always produces a *Windows* native (backslash-separated)
	// path — Git-Bash mount forms only exist to reach Windows paths, and the
	// sole caller (resolveFilePath) invokes it only on Windows. We build the
	// result with explicit backslash joins rather than path/filepath, whose
	// separator is keyed to the *host* OS, so the output is deterministic and
	// testable off-Windows (a Linux CI run must see the same `\` form).
	toWin := func(rest string) string { // forward-slash remainder → backslash tail
		return strings.ReplaceAll(strings.TrimPrefix(rest, "/"), "/", `\`)
	}
	// `/tmp/...` → `${TMP}\...`
	if strings.HasPrefix(p, "/tmp/") || p == "/tmp" {
		tmp := os.Getenv("TEMP")
		if tmp == "" {
			tmp = os.Getenv("TMP")
		}
		if tmp == "" {
			return "", false
		}
		rest := strings.TrimPrefix(p, "/tmp")
		if rest == "" {
			return tmp, true
		}
		return strings.TrimRight(tmp, `\`) + `\` + toWin(rest), true
	}
	// `/c/Users/...` → `C:\Users\...`. Drive letter is the first segment;
	// must be exactly 1 alpha char to avoid matching `/cmd/...` style
	// legitimate Unix paths.
	parts := strings.SplitN(p, "/", 3) // ["", "c", "Users/..."]
	if len(parts) >= 2 && len(parts[1]) == 1 && isAsciiAlpha(parts[1][0]) {
		drive := strings.ToUpper(parts[1]) + `:\`
		rest := ""
		if len(parts) == 3 {
			rest = parts[2]
		}
		return drive + toWin(rest), true
	}
	return "", false
}

func isAsciiAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// hasUncommittedChanges checks for uncommitted git changes
func (he *HookEngine) hasUncommittedChanges() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(output) > 0, nil
}

// checkBuild runs go build to verify the code compiles
func (he *HookEngine) checkBuild() error {
	cmd := exec.Command("go", "build", "./...")
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	cmd.Dir = he.basePath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// checkVet runs go vet
func (he *HookEngine) checkVet() error {
	cmd := exec.Command("go", "vet", "./...")
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	cmd.Dir = he.basePath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", output)
	}
	return nil
}

// phaseAQualityGates performs blocking quality checks
func (he *HookEngine) phaseAQualityGates(event *hooks.TaskCompletedEvent) error {
	// Check tests pass
	cmd := exec.Command("go", "test", "./...", "-count=1")
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	cmd.Dir = he.basePath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tests failed: %s", output)
	}

	// Check build
	if err := he.checkBuild(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Check vet
	if err := he.checkVet(); err != nil {
		return fmt.Errorf("go vet failed: %w", err)
	}

	return nil
}

// phaseBKnowledgeExtraction performs non-blocking knowledge work
func (he *HookEngine) phaseBKnowledgeExtraction(event *hooks.TaskCompletedEvent) error {
	taskDir := filepath.Join(he.basePath, "tickets", event.TaskID)

	// Get tracked changes
	changes, err := he.tracker.GetChanges()
	if err != nil {
		return err
	}

	// Build summary
	summary := fmt.Sprintf("Task %s completed with %d file changes", event.TaskID, len(changes))
	if len(changes) > 0 {
		summary += "\n\nModified files:\n"
		for _, change := range changes {
			summary += fmt.Sprintf("- %s (%s)\n", change.FilePath, change.Operation)
		}
	}

	// Update context.md
	if err := hooks.UpdateContextFile(taskDir, summary); err != nil {
		return err
	}

	// Extract knowledge using KnowledgeExtractor
	extractor := NewKnowledgeExtractor(he.basePath)
	if err := extractor.ExtractAndSave(event.TaskID); err != nil {
		// Log warning but don't fail - knowledge extraction is advisory
		fmt.Fprintf(os.Stderr, "Warning: failed to extract knowledge: %v\n", err)
	}

	// Clear tracked changes
	if err := he.tracker.Clear(); err != nil {
		return err
	}

	return nil
}

// updateContextOnStop updates context.md when stopping
func (he *HookEngine) updateContextOnStop() error {
	taskID := he.getCurrentTaskID()
	if taskID == "" {
		return nil
	}

	taskDir := filepath.Join(he.basePath, "tickets", taskID)
	changes, err := he.tracker.GetChanges()
	if err != nil {
		return err
	}

	if len(changes) == 0 {
		return nil
	}

	summary := fmt.Sprintf("Session paused with %d file changes", len(changes))
	return hooks.UpdateContextFile(taskDir, summary)
}

// updateContextOnSessionEnd updates context.md when session ends
func (he *HookEngine) updateContextOnSessionEnd(event *hooks.SessionEndEvent) error {
	taskID := he.getCurrentTaskID()
	if taskID == "" {
		return nil
	}

	taskDir := filepath.Join(he.basePath, "tickets", taskID)
	summary := fmt.Sprintf("Session %s ended (duration: %.2fs)", event.SessionID, event.Duration)
	return hooks.UpdateContextFile(taskDir, summary)
}

// getCurrentTaskID gets the current task ID from environment or git branch
func (he *HookEngine) getCurrentTaskID() string {
	// Try environment variable first
	if taskID := os.Getenv("ADB_TASK_ID"); taskID != "" {
		return taskID
	}

	// Try to extract from git branch (e.g., task/TASK-001)
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Env = append(os.Environ(), "ADB_HOOK_ACTIVE=1")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(output))
	if strings.HasPrefix(branch, "task/") {
		return strings.TrimPrefix(branch, "task/")
	}

	return ""
}
