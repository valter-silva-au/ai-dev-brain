package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestBuildSystemPrompt_ContainsAllSections asserts the composed prompt carries
// the ADB-orchestrator preamble and every ChatContext section — this is the
// contract F4 (chat wiring) relies on.
func TestBuildSystemPrompt_ContainsAllSections(t *testing.T) {
	c := ChatContext{
		Tasks:   "TASK-1: hello [in_progress]",
		Metrics: "TasksCreated=3 TasksCompleted=1",
	}

	got := BuildSystemPrompt(c)

	wantSubstrings := []string{
		"ADB",     // orchestrator identity
		"Tasks",   // tasks section header
		"Metrics", // metrics section header
		c.Tasks,   // rendered tasks payload
		c.Metrics, // rendered metrics payload
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("BuildSystemPrompt() missing %q\n---prompt---\n%s\n------------", want, got)
		}
	}
}

// TestBuildSystemPrompt_EmptyContext produces a well-formed prompt even when
// Tasks/Metrics are empty — F4 may render before task metrics have been
// gathered.
func TestBuildSystemPrompt_EmptyContext(t *testing.T) {
	got := BuildSystemPrompt(ChatContext{})
	if !strings.Contains(got, "ADB") {
		t.Errorf("empty-context prompt should still name the orchestrator; got:\n%s", got)
	}
}

// TestChat_UsesInjectedRunner proves Chat routes through the runner seam
// (no live claude call in tests). The runner receives the composed prompt +
// model and returns whatever it wants.
func TestChat_UsesInjectedRunner(t *testing.T) {
	var (
		gotName  string
		gotArgs  []string
		gotStdin string
	)
	fake := ChatRunnerFunc(func(ctx context.Context, name string, args []string) (string, error) {
		gotName = name
		gotArgs = args
		return "hello from fake claude", nil
	})

	out, err := Chat(context.Background(), fake, ChatContext{Tasks: "T"}, "hi")
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if out != "hello from fake claude" {
		t.Errorf("Chat = %q; want %q", out, "hello from fake claude")
	}
	if gotName != "claude" {
		t.Errorf("runner invoked with name %q; want %q", gotName, "claude")
	}
	// argv should be `-p <prompt> --model <default>` — check flag positions.
	if len(gotArgs) < 4 || gotArgs[0] != "-p" || gotArgs[2] != "--model" {
		t.Errorf("runner argv = %v; want [-p, <prompt>, --model, <model>]", gotArgs)
	}
	if !strings.Contains(gotArgs[1], "hi") || !strings.Contains(gotArgs[1], "T") {
		t.Errorf("prompt argv[1] missing user message or context; got %q", gotArgs[1])
	}
	if gotArgs[3] != DefaultChatModel {
		t.Errorf("model argv = %q; want %q", gotArgs[3], DefaultChatModel)
	}
	_ = gotStdin
}

// TestChat_TrimsWhitespace mirrors the salvaged behaviour: shell output has
// trailing newlines that must be stripped before F4 parses actions.
func TestChat_TrimsWhitespace(t *testing.T) {
	fake := ChatRunnerFunc(func(ctx context.Context, name string, args []string) (string, error) {
		return "  \nreal answer\n\n", nil
	})
	out, err := Chat(context.Background(), fake, ChatContext{}, "q")
	if err != nil {
		t.Fatal(err)
	}
	if out != "real answer" {
		t.Errorf("Chat = %q; want trimmed %q", out, "real answer")
	}
}

// TestChat_PropagatesRunnerError ensures runner failures surface as errors —
// not silently swallowed. The salvaged code returned an error including
// combined output; we preserve that.
func TestChat_PropagatesRunnerError(t *testing.T) {
	fake := ChatRunnerFunc(func(ctx context.Context, name string, args []string) (string, error) {
		return "usage: claude ...", errors.New("exit status 2")
	})
	_, err := Chat(context.Background(), fake, ChatContext{}, "q")
	if err == nil {
		t.Fatal("Chat should have returned an error when runner failed")
	}
	if !strings.Contains(err.Error(), "exit status 2") {
		t.Errorf("error should carry runner error; got %v", err)
	}
}

// TestChat_NilRunnerRejected — a nil runner is a programmer error; Chat must
// not attempt to spawn anything on its own.
func TestChat_NilRunnerRejected(t *testing.T) {
	_, err := Chat(context.Background(), nil, ChatContext{}, "q")
	if err == nil {
		t.Fatal("Chat with nil runner must return an error")
	}
}

// TestExecChatRunner_UnknownBinaryFails guards the exec-seam constructor:
// pointing it at a non-existent binary must surface a clear error, not panic
// and not hang. This is the ONLY test that exercises real exec, and it only
// asserts failure — never a live LLM call.
func TestExecChatRunner_UnknownBinaryFails(t *testing.T) {
	runner := ExecChatRunner()
	// A binary that cannot exist on any test host.
	_, err := runner.Run(context.Background(), "adb-nonexistent-binary-xyz-123", []string{"-p", "hi"})
	if err == nil {
		t.Fatal("ExecChatRunner should have failed for a nonexistent binary")
	}
}
