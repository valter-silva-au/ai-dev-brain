package observability

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultChatModel is the Claude model passed to the `claude` CLI for the
// dashboard chat. F4 (webview chat/steer) reuses this constant unless the
// caller overrides it.
const DefaultChatModel = "sonnet"

// DefaultChatTimeout bounds a single Chat call. Matches the salvaged
// dashboard behaviour (60s) — long enough for a real claude -p reply, short
// enough to keep the extension responsive.
const DefaultChatTimeout = 60 * time.Second

// ChatContext is the pre-rendered workspace snapshot fed into the ADB
// orchestrator system prompt. Callers render tasks/metrics into strings — this
// package does not depend on internal/core or pkg/models, so it stays pure and
// unit-testable without the full app.
type ChatContext struct {
	Tasks   string // pre-rendered task summary (e.g. "TASK-1: hello [in_progress]")
	Metrics string // pre-rendered metrics summary
}

// ChatRunner runs the LLM CLI (typically `claude -p …`) and returns its
// combined stdout+stderr. The interface exists purely to make Chat testable
// without shelling out. Callers use ExecChatRunner() in production and a
// stub in tests.
type ChatRunner interface {
	// Run invokes name with args and returns combined output. Implementations
	// MUST honor ctx cancellation.
	Run(ctx context.Context, name string, args []string) (string, error)
}

// ChatRunnerFunc adapts a function to the ChatRunner interface.
type ChatRunnerFunc func(ctx context.Context, name string, args []string) (string, error)

// Run implements ChatRunner.
func (f ChatRunnerFunc) Run(ctx context.Context, name string, args []string) (string, error) {
	return f(ctx, name, args)
}

// ExecChatRunner returns a ChatRunner that shells out via os/exec.
// This is the production seam — tests inject a fake instead.
func ExecChatRunner() ChatRunner {
	return ChatRunnerFunc(func(ctx context.Context, name string, args []string) (string, error) {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if err != nil {
			return string(out), err
		}
		return string(out), nil
	})
}

// BuildSystemPrompt composes the ADB-orchestrator system prompt from a
// ChatContext. Salvaged from internal/server/llm.go's Server.buildSystemPrompt,
// but decoupled from the retired Server type — tasks/metrics are pre-rendered
// by the caller instead of being gathered from live agents/hive.
func BuildSystemPrompt(c ChatContext) string {
	var sb strings.Builder
	sb.WriteString("You are ADB — the AI Dev Brain orchestrator. ")
	sb.WriteString("You are the central nervous system of your development ecosystem.\n\n")
	sb.WriteString("## Your Personality\n")
	sb.WriteString("- Friendly, warm, slightly nerdy project manager\n")
	sb.WriteString("- Genuinely curious about what agents are building\n")
	sb.WriteString("- Proactively helpful; concise but thorough\n\n")
	sb.WriteString("## Your Capabilities\n")
	sb.WriteString("- Real-time visibility into tasks and workspace metrics\n")
	sb.WriteString("- Answer questions about the ecosystem, propose actions, provide status updates\n\n")
	sb.WriteString("## Current System State (LIVE)\n\n")
	sb.WriteString("### Tasks\n")
	if c.Tasks == "" {
		sb.WriteString("(none)\n")
	} else {
		sb.WriteString(c.Tasks)
		if !strings.HasSuffix(c.Tasks, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n### Metrics\n")
	if c.Metrics == "" {
		sb.WriteString("(none)\n")
	} else {
		sb.WriteString(c.Metrics)
		if !strings.HasSuffix(c.Metrics, "\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n## Instructions\n")
	sb.WriteString("Respond to the user's message. Be helpful and specific. ")
	sb.WriteString("Use the live system state above to inform your answers. ")
	sb.WriteString("Format your response as plain text with markdown — it will be rendered in a chat panel. ")
	sb.WriteString("Keep responses concise (2-5 sentences for simple queries; more for status reports).\n")
	return sb.String()
}

// Chat runs one turn of the ADB orchestrator conversation: it composes the
// system prompt from ctx, appends the user message, and delegates to runner.
// The returned string is trimmed of surrounding whitespace so downstream
// parsers (F4's parseSteerActions) don't have to.
//
// runner MUST be non-nil — Chat deliberately does not default to a live exec
// seam so tests can never accidentally shell out.
func Chat(ctx context.Context, runner ChatRunner, cctx ChatContext, userMessage string) (string, error) {
	if runner == nil {
		return "", fmt.Errorf("observability.Chat: runner is nil (inject ExecChatRunner() in production)")
	}

	prompt := fmt.Sprintf("%s\n\nUser message: %s", BuildSystemPrompt(cctx), userMessage)

	callCtx, cancel := context.WithTimeout(ctx, DefaultChatTimeout)
	defer cancel()

	out, err := runner.Run(callCtx, "claude", []string{"-p", prompt, "--model", DefaultChatModel})
	if err != nil {
		return "", fmt.Errorf("claude call failed: %w: %s", err, strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}
