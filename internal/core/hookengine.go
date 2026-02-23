package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/hooks"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// HookEngine processes Claude Code hook events and updates adb artifacts.
type HookEngine interface {
	// HandlePreToolUse validates before a tool executes. Returns error to block.
	HandlePreToolUse(input hooks.PreToolUseInput) error

	// HandlePostToolUse reacts after a tool executes. Non-blocking.
	HandlePostToolUse(input hooks.PostToolUseInput) error

	// HandleStop runs advisory checks when a session stops.
	HandleStop(input hooks.StopInput) error

	// HandleTaskCompleted validates task completion. Returns error to block.
	HandleTaskCompleted(input hooks.TaskCompletedInput) error

	// HandleSessionEnd handles session end context updates. Non-blocking.
	HandleSessionEnd(input hooks.SessionEndInput) error
}

type hookEngine struct {
	basePath   string
	config     models.HookConfig
	tracker    *hooks.ChangeTracker
	knowledgeX KnowledgeExtractor // optional, for Phase 2
	conflictDt ConflictDetector   // optional, for Phase 3
}

// NewHookEngine creates a HookEngine with the given configuration.
// knowledgeX and conflictDt may be nil (Phase 2/3 features disabled).
func NewHookEngine(
	basePath string,
	config models.HookConfig,
	knowledgeX KnowledgeExtractor,
	conflictDt ConflictDetector,
) HookEngine {
	return &hookEngine{
		basePath:   basePath,
		config:     config,
		tracker:    hooks.NewChangeTracker(basePath),
		knowledgeX: knowledgeX,
		conflictDt: conflictDt,
	}
}

// HandlePreToolUse validates before a tool executes.
// Returns error with exit code 2 semantics to block the tool use.
func (e *hookEngine) HandlePreToolUse(input hooks.PreToolUseInput) error {
	if !e.config.Enabled || !e.config.PreToolUse.Enabled {
		return nil
	}

	fp := input.FilePath()
	if fp == "" {
		return nil
	}

	// Phase 1: vendor guard
	if e.config.PreToolUse.BlockVendor && isVendorPath(fp) {
		return fmt.Errorf("BLOCKED: editing vendor/ files is not allowed. Run 'go mod vendor' instead")
	}

	// Phase 1: go.sum guard
	if e.config.PreToolUse.BlockGoSum && isGoSumPath(fp) {
		return fmt.Errorf("BLOCKED: direct editing of go.sum is not allowed. Run 'go mod tidy' instead")
	}

	// Phase 2: architecture guard - block core/ importing storage/ or integration/
	if e.config.PreToolUse.ArchitectureGuard {
		if err := e.checkArchitectureGuard(fp); err != nil {
			return err
		}
	}

	// Phase 3: ADR conflict check
	if e.config.PreToolUse.ADRConflictCheck && e.conflictDt != nil {
		e.checkADRConflicts(fp)
	}

	return nil
}

// HandlePostToolUse reacts after a tool executes.
// Always returns nil (non-blocking).
func (e *hookEngine) HandlePostToolUse(input hooks.PostToolUseInput) error {
	if !e.config.Enabled || !e.config.PostToolUse.Enabled {
		return nil
	}

	fp := input.FilePath()
	if fp == "" {
		return nil
	}

	// Phase 1: Go format
	if e.config.PostToolUse.GoFormat && strings.HasSuffix(fp, ".go") {
		if _, err := os.Stat(fp); err == nil {
			cmd := exec.Command("gofmt", "-s", "-w", fp)
			_ = cmd.Run() // Non-fatal.
		}
	}

	// Phase 1: Change tracking
	if e.config.PostToolUse.ChangeTracking {
		tool := input.ToolName
		if tool == "" {
			tool = "unknown"
		}
		_ = e.tracker.Append(models.SessionChangeEntry{
			Tool:     tool,
			FilePath: fp,
		})
	}

	// Phase 2: Dependency detection
	if e.config.PostToolUse.DependencyDetection && filepath.Base(fp) == "go.mod" {
		e.detectDependencyChanges(fp)
	}

	return nil
}

// HandleStop runs advisory checks when a session stops.
// Always returns nil (non-blocking, advisory only).
func (e *hookEngine) HandleStop(input hooks.StopInput) error {
	if !e.config.Enabled || !e.config.Stop.Enabled {
		return nil
	}

	// Advisory: check for uncommitted changes.
	if e.config.Stop.UncommittedCheck {
		output, err := runCommand("git", "status", "--porcelain")
		if err == nil && strings.TrimSpace(output) != "" {
			fmt.Fprintln(os.Stderr, "Warning: uncommitted changes detected")
		}
	}

	// Advisory: build check.
	if e.config.Stop.BuildCheck {
		if _, err := runCommand("go", "build", "./..."); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: build failed")
		}
	}

	// Advisory: vet check.
	if e.config.Stop.VetCheck {
		if _, err := runCommand("go", "vet", "./..."); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: vet check failed")
		}
	}

	// Update context.md with session changes.
	if e.config.Stop.ContextUpdate {
		e.updateContextFromChanges()
	}

	// Update status.yaml timestamp.
	if e.config.Stop.StatusTimestamp {
		e.updateStatusTimestamp()
	}

	// Cleanup change tracker.
	_ = e.tracker.Cleanup()

	return nil
}

// HandleTaskCompleted validates task completion with a two-phase approach.
// Phase A (blocking): tests, lint, uncommitted check.
// Phase B (non-blocking): knowledge extraction, context update.
func (e *hookEngine) HandleTaskCompleted(input hooks.TaskCompletedInput) error {
	if !e.config.Enabled || !e.config.TaskCompleted.Enabled {
		return nil
	}

	// --- Phase A: Blocking quality gates ---

	if e.config.TaskCompleted.CheckUncommitted {
		if err := e.checkUncommittedGoFiles(); err != nil {
			return err
		}
	}

	if e.config.TaskCompleted.RunTests {
		testCmd := e.config.TaskCompleted.TestCommand
		if testCmd == "" {
			testCmd = "go test ./..."
		}
		parts := strings.Fields(testCmd)
		if output, err := runCommand(parts[0], parts[1:]...); err != nil {
			return fmt.Errorf("BLOCKED: tests failed:\n%s", output)
		}
	}

	if e.config.TaskCompleted.RunLint {
		lintCmd := e.config.TaskCompleted.LintCommand
		if lintCmd == "" {
			lintCmd = "golangci-lint run"
		}
		parts := strings.Fields(lintCmd)
		if output, err := runCommand(parts[0], parts[1:]...); err != nil {
			return fmt.Errorf("BLOCKED: lint failed:\n%s", output)
		}
	}

	// --- Phase B: Non-blocking knowledge and context ---

	// Phase 2: Knowledge extraction.
	if e.config.TaskCompleted.ExtractKnowledge && e.knowledgeX != nil {
		e.extractAndApplyKnowledge()
	}

	// Phase 2: Wiki updates.
	if e.config.TaskCompleted.UpdateWiki && e.knowledgeX != nil {
		e.updateWikiFromKnowledge()
	}

	// Phase 2: ADR generation.
	if e.config.TaskCompleted.GenerateADRs && e.knowledgeX != nil {
		e.generateADRsFromKnowledge()
	}

	// Phase 1: Context update.
	if e.config.TaskCompleted.UpdateContext {
		e.updateContextFromChanges()
		e.appendCompletionSummary()
	}

	return nil
}

// HandleSessionEnd handles session end context updates.
// Session capture is handled separately by the CLI layer.
func (e *hookEngine) HandleSessionEnd(input hooks.SessionEndInput) error {
	if !e.config.Enabled || !e.config.SessionEnd.Enabled {
		return nil
	}

	// Update context.md with session changes.
	if e.config.SessionEnd.UpdateContext {
		e.updateContextFromChanges()
	}

	return nil
}

// --- Helper methods ---

func (e *hookEngine) resolveTicketPath() string {
	taskID := os.Getenv("ADB_TASK_ID")
	if taskID == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(e.basePath, "tickets", taskID),
		filepath.Join(e.basePath, "tickets", "_archived", taskID),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (e *hookEngine) updateContextFromChanges() {
	entries, err := e.tracker.Read()
	if err != nil || len(entries) == 0 {
		return
	}
	ticketPath := e.resolveTicketPath()
	if ticketPath == "" {
		return
	}
	grouped := hooks.GroupChangesByDirectory(entries)
	summary := hooks.FormatSessionSummary(grouped)
	if summary != "" {
		_ = hooks.AppendToContext(ticketPath, summary)
	}
}

func (e *hookEngine) updateStatusTimestamp() {
	ticketPath := e.resolveTicketPath()
	if ticketPath == "" {
		return
	}
	_ = hooks.UpdateStatusTimestamp(ticketPath)
}

func (e *hookEngine) checkUncommittedGoFiles() error {
	// Check both unstaged and staged Go files.
	unstaged, _ := runCommand("git", "diff", "--name-only", "--", "*.go")
	staged, _ := runCommand("git", "diff", "--cached", "--name-only", "--", "*.go")

	var goFiles []string
	goFiles = append(goFiles, filterGoFiles(unstaged)...)
	goFiles = append(goFiles, filterGoFiles(staged)...)

	if len(goFiles) > 0 {
		return fmt.Errorf("BLOCKED: uncommitted Go changes:\n%s\nCommit or stash before completing the task", strings.Join(goFiles, "\n"))
	}
	return nil
}

func (e *hookEngine) appendCompletionSummary() {
	ticketPath := e.resolveTicketPath()
	if ticketPath == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	summary := fmt.Sprintf("### Task Completed %s\n\nQuality gates passed: tests, lint, uncommitted check.", now)
	_ = hooks.AppendToContext(ticketPath, summary)
}

// Phase 2: Architecture guard - blocks core/ from importing storage/ or integration/.
func (e *hookEngine) checkArchitectureGuard(fp string) error {
	// Only check Go files in internal/core/.
	if !strings.Contains(fp, "internal/core/") || !strings.HasSuffix(fp, ".go") {
		return nil
	}
	data, err := os.ReadFile(fp) //nolint:gosec // G304: path from trusted hook input
	if err != nil {
		return nil // Can't read, skip check.
	}
	content := string(data)
	if strings.Contains(content, `"github.com/valter-silva-au/ai-dev-brain/internal/storage"`) {
		return fmt.Errorf("BLOCKED: core/ must not import storage/ directly. Define a local interface instead")
	}
	if strings.Contains(content, `"github.com/valter-silva-au/ai-dev-brain/internal/integration"`) {
		return fmt.Errorf("BLOCKED: core/ must not import integration/ directly. Define a local interface instead")
	}
	return nil
}

// Phase 3: ADR conflict check - warns (does not block) if edit conflicts with ADRs.
func (e *hookEngine) checkADRConflicts(fp string) {
	if e.conflictDt == nil {
		return
	}
	conflicts, err := e.conflictDt.CheckForConflicts(ConflictContext{
		AffectedFiles: []string{fp},
	})
	if err != nil || len(conflicts) == 0 {
		return
	}
	for _, c := range conflicts {
		fmt.Fprintf(os.Stderr, "ADR conflict warning [%s]: %s (source: %s)\n", c.Severity, c.Description, c.Source)
	}
}

// Phase 2: Detect dependency changes in go.mod.
func (e *hookEngine) detectDependencyChanges(fp string) {
	ticketPath := e.resolveTicketPath()
	if ticketPath == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	section := fmt.Sprintf("### Dependency Change %s\n- go.mod was modified. Run `go mod tidy` to verify.", now)
	_ = hooks.AppendToContext(ticketPath, section)
}

// Phase 2: Extract knowledge from the current task.
func (e *hookEngine) extractAndApplyKnowledge() {
	taskID := os.Getenv("ADB_TASK_ID")
	if taskID == "" || e.knowledgeX == nil {
		return
	}
	knowledge, err := e.knowledgeX.ExtractFromTask(taskID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Knowledge extraction failed (non-blocking): %s\n", err)
		return
	}
	ticketPath := e.resolveTicketPath()
	if ticketPath == "" || knowledge == nil {
		return
	}
	// Log extraction results to context.
	var sb strings.Builder
	now := time.Now().UTC().Format(time.RFC3339)
	sb.WriteString(fmt.Sprintf("### Knowledge Extracted %s\n", now))
	if len(knowledge.Learnings) > 0 {
		sb.WriteString(fmt.Sprintf("- %d learning(s) extracted\n", len(knowledge.Learnings)))
	}
	if len(knowledge.Decisions) > 0 {
		sb.WriteString(fmt.Sprintf("- %d decision(s) extracted\n", len(knowledge.Decisions)))
	}
	if len(knowledge.Gotchas) > 0 {
		sb.WriteString(fmt.Sprintf("- %d gotcha(s) extracted\n", len(knowledge.Gotchas)))
	}
	if sb.Len() > 0 {
		_ = hooks.AppendToContext(ticketPath, sb.String())
	}
}

// Phase 2: Update wiki from extracted knowledge.
func (e *hookEngine) updateWikiFromKnowledge() {
	taskID := os.Getenv("ADB_TASK_ID")
	if taskID == "" || e.knowledgeX == nil {
		return
	}
	knowledge, err := e.knowledgeX.ExtractFromTask(taskID)
	if err != nil || knowledge == nil {
		return
	}
	if err := e.knowledgeX.UpdateWiki(knowledge); err != nil {
		fmt.Fprintf(os.Stderr, "Wiki update failed (non-blocking): %s\n", err)
	}
}

// Phase 2: Generate ADR drafts from extracted decisions.
func (e *hookEngine) generateADRsFromKnowledge() {
	taskID := os.Getenv("ADB_TASK_ID")
	if taskID == "" || e.knowledgeX == nil {
		return
	}
	knowledge, err := e.knowledgeX.ExtractFromTask(taskID)
	if err != nil || knowledge == nil {
		return
	}
	for _, d := range knowledge.Decisions {
		if _, err := e.knowledgeX.CreateADR(d, taskID); err != nil {
			fmt.Fprintf(os.Stderr, "ADR generation failed for '%s' (non-blocking): %s\n", d.Decision, err)
		}
	}
}

// --- Package-level helpers ---

func isVendorPath(fp string) bool {
	return strings.HasPrefix(fp, "vendor/") || strings.Contains(fp, "/vendor/")
}

func isGoSumPath(fp string) bool {
	return filepath.Base(fp) == "go.sum"
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func filterGoFiles(output string) []string {
	var goFiles []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.HasSuffix(line, ".go") {
			goFiles = append(goFiles, line)
		}
	}
	return goFiles
}
