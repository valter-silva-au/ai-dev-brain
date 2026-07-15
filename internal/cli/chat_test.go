package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// setupChatTest wires a temp workspace + global App, seeds a couple of
// tasks so the "live chat context" has content, and installs a stub
// chatRunnerForTesting that never shells out to claude. Returns a cleanup
// that restores globals.
func setupChatTest(t *testing.T) (
	stubRuns *[]stubRunCall,
	cleanup func(),
) {
	t.Helper()
	// Mirror events_test.go's setup: NewApp on a TempDir, swap the global
	// App var, restore on cleanup. Keeps test isolation clean.
	oldApp := App
	oldRunner := chatRunnerForTesting

	// A minimal workspace: NewApp produces a real BacklogManager; we seed
	// two tasks via BacklogManager.Save so renderTasksSummary has content.
	appCleanup, seededTasks := newTestAppWithTasks(t)

	stub := &stubRunner{}
	chatRunnerForTesting = stub

	return &stub.calls, func() {
		chatRunnerForTesting = oldRunner
		App = oldApp
		appCleanup()
		_ = seededTasks
	}
}

type stubRunCall struct {
	Name string
	Args []string
}

// stubRunner captures the args the observability.Chat helper passes to the
// runner and returns a canned reply. Guarantees no real `claude` binary
// is ever invoked from these tests.
type stubRunner struct {
	calls []stubRunCall
	reply string
	err   error
}

func (s *stubRunner) Run(_ context.Context, name string, args []string) (string, error) {
	s.calls = append(s.calls, stubRunCall{Name: name, Args: append([]string(nil), args...)})
	if s.err != nil {
		return "", s.err
	}
	if s.reply == "" {
		return "STUB REPLY", nil
	}
	return s.reply, nil
}

// newTestAppWithTasks seeds the workspace with two tasks so renderTasksSummary
// isn't empty. Uses BacklogManager.Save directly (not the TaskManager) so
// the test doesn't accidentally exercise the git-worktree machinery.
func newTestAppWithTasks(t *testing.T) (cleanup func(), seeded []models.Task) {
	t.Helper()

	// Reuse the events-test scaffold: it already stands up NewApp on a
	// TempDir with EventLog + BacklogManager wired.
	app, done := setupEventsTest(t)

	// Seed two tasks. BacklogManager.Save persists atomically.
	backlog, err := app.BacklogManager.Load()
	if err != nil {
		t.Fatalf("load initial backlog: %v", err)
	}
	backlog.Tasks = append(backlog.Tasks,
		models.Task{
			ID:       "TASK-1",
			Title:    "hello",
			Type:     models.TaskTypeFeat,
			Status:   models.TaskStatusInProgress,
			Priority: models.PriorityP1,
		},
		models.Task{
			ID:       "TASK-2",
			Title:    "world",
			Type:     models.TaskTypeBug,
			Status:   models.TaskStatusBlocked,
			Priority: models.PriorityP2,
		},
	)
	if err := app.BacklogManager.Save(backlog); err != nil {
		t.Fatalf("save seeded backlog: %v", err)
	}
	return done, backlog.Tasks
}

// TestChat_RequiresMessage: --message is required and rejects empty/whitespace.
// This is the first line of defence — if the user (or the extension) forgets
// to pass a message, the command must fail loudly, not fire an empty prompt.
func TestChat_RequiresMessage(t *testing.T) {
	_, cleanup := setupChatTest(t)
	defer cleanup()

	cmd := NewChatCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// --message is MarkFlagRequired, so cobra rejects an empty invocation.
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected an error when --message is omitted; got: %s", out.String())
	}
}

func TestChat_RejectsWhitespaceOnlyMessage(t *testing.T) {
	_, cleanup := setupChatTest(t)
	defer cleanup()

	cmd := NewChatCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--message", "   \t\n  "})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for whitespace-only message")
	}
	if !strings.Contains(err.Error(), "required") && !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("expected a 'required/non-empty' error; got: %v", err)
	}
}

// TestChat_CallsClaudeWithSystemPromptAndMessage: verifies that when the
// user sends a message the runner sees a claude -p invocation carrying (a)
// the ADB orchestrator system prompt (identifiable by "You are ADB") and
// (b) the user message text.
func TestChat_CallsClaudeWithSystemPromptAndMessage(t *testing.T) {
	stubCalls, cleanup := setupChatTest(t)
	defer cleanup()

	cmd := NewChatCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--message", "what's the status of TASK-1?"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	if len(*stubCalls) != 1 {
		t.Fatalf("expected exactly one runner call, got %d", len(*stubCalls))
	}
	call := (*stubCalls)[0]
	if call.Name != "claude" {
		t.Errorf("expected runner name 'claude', got %q", call.Name)
	}
	// Pin the argv shape observability.Chat produces so a regression here
	// is loud: "-p", "<prompt>", "--model", "<model>".
	if len(call.Args) < 4 {
		t.Fatalf("expected argv len >= 4, got %d: %v", len(call.Args), call.Args)
	}
	if call.Args[0] != "-p" {
		t.Errorf("expected first arg '-p', got %q", call.Args[0])
	}
	prompt := call.Args[1]
	if !strings.Contains(prompt, "You are ADB") {
		t.Errorf("prompt should carry ADB orchestrator system prompt; got: %q", prompt[:min(len(prompt), 200)])
	}
	if !strings.Contains(prompt, "what's the status of TASK-1?") {
		t.Errorf("prompt should include the user message; got: %q", prompt[:min(len(prompt), 200)])
	}
	// Live context must be present: the seeded task ids should show up in
	// the prompt via the tasks summary.
	if !strings.Contains(prompt, "TASK-1") {
		t.Errorf("prompt should carry live task context (TASK-1)")
	}
}

// TestChat_EmitsReplyOnStdout: the reply the LLM produces flows straight to
// stdout, trimmed of surrounding whitespace by observability.Chat. The
// webview reads this stdout and hands it to parseSteerActions.
func TestChat_EmitsReplyOnStdout(t *testing.T) {
	_, cleanup := setupChatTest(t)
	defer cleanup()
	// Reach in to set the stub reply before running.
	stub, ok := chatRunnerForTesting.(*stubRunner)
	if !ok {
		t.Fatalf("expected stubRunner")
	}
	stub.reply = "here you go\n\n```adb-action\n{\"verb\":\"task.update\",\"taskId\":\"TASK-1\",\"status\":\"blocked\"}\n```\n"

	cmd := NewChatCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--message", "block TASK-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}
	body := out.String()
	if !strings.Contains(body, "adb-action") {
		t.Errorf("expected reply body forwarded to stdout; got %q", body)
	}
	if !strings.Contains(body, "task.update") {
		t.Errorf("expected fenced action payload in stdout; got %q", body)
	}
}

// TestChat_PropagatesRunnerError: a claude -p failure surfaces as a
// non-zero cmd.Execute error. The webview relies on this to render an
// error state instead of a silent no-op.
func TestChat_PropagatesRunnerError(t *testing.T) {
	_, cleanup := setupChatTest(t)
	defer cleanup()
	stub, ok := chatRunnerForTesting.(*stubRunner)
	if !ok {
		t.Fatalf("expected stubRunner")
	}
	stub.err = fmt.Errorf("claude call failed: exit status 1")

	cmd := NewChatCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--message", "hi"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error, got: %s", out.String())
	}
}

// TestChat_RegisteredOnRoot: `adb chat` is discoverable via the root
// command so the extension can spawn `adb chat --message …` without any
// alternate entry point.
func TestChat_RegisteredOnRoot(t *testing.T) {
	root := NewRootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "chat" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'chat' subcommand on root")
	}
}

// TestBuildLiveChatContext_IncludesTasksAndMetrics: renderTasksSummary +
// renderMetricsSummary produce non-empty strings for a seeded workspace.
// This is the "the LLM has enough context to be useful" pin.
func TestBuildLiveChatContext_IncludesTasksAndMetrics(t *testing.T) {
	_, cleanup := setupChatTest(t)
	defer cleanup()

	cctx, err := buildLiveChatContext()
	if err != nil {
		t.Fatalf("buildLiveChatContext: %v", err)
	}
	if !strings.Contains(cctx.Tasks, "TASK-1") {
		t.Errorf("tasks summary missing TASK-1: %q", cctx.Tasks)
	}
	if !strings.Contains(cctx.Tasks, "TASK-2") {
		t.Errorf("tasks summary missing TASK-2: %q", cctx.Tasks)
	}
	// Metrics may be zero on a fresh log; render should still produce the
	// counter labels.
	if !strings.Contains(cctx.Metrics, "Tasks Created") {
		t.Errorf("metrics summary missing 'Tasks Created' label: %q", cctx.Metrics)
	}
}

// TestRenderMetrics_IsPure: given a Metrics struct, renderMetrics must be
// deterministic and self-contained (no App dependency). Pins the format
// the LLM sees so a regression in wording is loud.
func TestRenderMetrics_IsPure(t *testing.T) {
	m := &observability.Metrics{
		TasksCreated:   3,
		TasksCompleted: 1,
		AgentSessions:  2,
		TasksByStatus: map[string]int{
			"in_progress": 2,
			"done":        1,
		},
	}
	got := renderMetrics(m)
	for _, want := range []string{"Tasks Created: 3", "Tasks Completed: 1", "Agent Sessions: 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected substring %q in metrics render, got:\n%s", want, got)
		}
	}
}

// min is a tiny helper — Go 1.21+ has one built-in but we keep this local
// to stay compatible with older toolchains that show up in CI.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
