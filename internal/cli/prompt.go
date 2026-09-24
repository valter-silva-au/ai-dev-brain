package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// promptTaskStatus holds the minimal fields from status.yaml
type promptTaskStatus struct {
	TaskID   string
	Title    string
	Status   string
	Priority string
}

// NewPromptCmd creates the 'adb prompt' command.
// This command is designed to be called from PROMPT_COMMAND in bash
// and must be extremely fast (<10ms). It avoids loading the full App
// struct and reads only the minimal files needed.
func NewPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "prompt",
		Short:  "Output shell prompt prefix with task context",
		Long:   `Output a colored PS1 prefix based on the current working directory context. Designed to be called from PROMPT_COMMAND.`,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrompt()
		},
	}

	return cmd
}

// runPrompt writes the prompt prefix, or nothing.
//
// Every failure path deliberately returns nil. This runs inside the user's
// PROMPT_COMMAND on every shell prompt: an error here would print a diagnostic
// into their prompt line and hand them a non-zero exit status for something they
// did not run. Emitting no prefix is the correct degradation, and is why the
// nilerr suppressions below are intentional rather than overlooked.
func runPrompt() error {
	cwd, err := os.Getwd()
	if err != nil {
		//nolint:nilerr // an unknown cwd means no context to show, not a prompt failure
		return nil
	}

	adbHome := os.Getenv("ADB_HOME")
	if adbHome == "" {
		return nil // Not in an ADB environment
	}

	workDir := filepath.Join(adbHome, "work")

	// Check if CWD is inside a worktree: $ADB_HOME/work/<TASK-ID>/...
	if strings.HasPrefix(cwd, workDir+"/") {
		rel, err := filepath.Rel(workDir, cwd)
		if err != nil {
			//nolint:nilerr // same contract: no relative path ⇒ no prefix, never a prompt error
			return nil
		}
		// Extract task ID (first path component)
		taskID := strings.SplitN(rel, "/", 2)[0]
		if taskID == "" {
			return nil
		}

		// Read status.yaml from the ticket directory
		statusPath := filepath.Join(adbHome, "tickets", taskID, "status.yaml")
		prefix := formatTaskPrompt(taskID, statusPath)
		fmt.Print(prefix)
		return nil
	}

	// Check if CWD is inside ADB_HOME (but not a worktree)
	if strings.HasPrefix(cwd, adbHome) {
		prefix := formatPortfolioPrompt(adbHome)
		if prefix != "" {
			fmt.Print(prefix)
		}
		return nil
	}

	// Outside ADB context — output nothing
	return nil
}

// unquoteYAMLScalar reverses the quoting the generator's emitter (yamlq /
// YAMLScalar) may have applied to a frontmatter value. Double-quoted: strip
// the pair and unescape \" and \\; single-quoted: strip the pair (YAML's
// single-quote form has no backslash escapes). Unquoted values pass through.
// Not a full YAML scalar parser — parseStatusFile only needs the lossless
// inverse of what the generator emits.
func unquoteYAMLScalar(value string) string {
	if len(value) >= 2 {
		switch {
		case value[0] == '"' && value[len(value)-1] == '"':
			inner := value[1 : len(value)-1]
			return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(inner)
		case value[0] == '\'' && value[len(value)-1] == '\'':
			return value[1 : len(value)-1]
		}
	}
	return value
}

// formatTaskPrompt reads status.yaml and formats the task prompt prefix.
// Uses line-based parsing instead of YAML because status.yaml titles
// contain unquoted [type] prefixes which YAML interprets as flow sequences.
func formatTaskPrompt(taskID, statusPath string) string {
	status := parseStatusFile(statusPath)

	taskType := parseTypeFromTitle(status.Title)

	priority := status.Priority
	if priority == "" {
		priority = "P2"
	}

	icon := statusIcon(status.Status)
	colorCode := priorityColor(priority)

	return fmt.Sprintf("\\[\\033[%sm\\][%s %s %s %s]\\[\\033[0m\\]", colorCode, taskID, taskType, priority, icon)
}

// parseStatusFile reads a status.yaml using line-based parsing.
// Values are unquoted after the Cut: the generator emitter-quotes frontmatter
// scalars that need it (TASK-00049), so a colon-bearing title arrives as
// title: "Learn pipeline: ...". Lossless for what YAMLScalar can emit:
// strip one pair of "..." or '...' and unescape the double-quote form's \\\\
// and \\" sequences.
func parseStatusFile(path string) promptTaskStatus {
	// gosec flags the env-derived path as tainted. It is built by runPrompt as
	// $ADB_HOME/tickets/<id>/status.yaml, where <id> is the FIRST component of a
	// path already proven to sit under $ADB_HOME/work — so it is one path
	// element, cannot be "..", and both halves come from the invoking user's own
	// environment and cwd. The open is read-only, at that user's own privileges,
	// with no boundary to cross.
	f, err := os.Open(path) //nolint:gosec // path is $ADB_HOME + one cwd component, read-only, same-user
	if err != nil {
		return promptTaskStatus{}
	}
	defer f.Close()

	var s promptTaskStatus
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		value = unquoteYAMLScalar(strings.TrimSpace(value))
		switch key {
		case "task_id":
			s.TaskID = value
		case "title":
			s.Title = value
		case "status":
			s.Status = value
		case "priority":
			s.Priority = value
		}
	}
	return s
}

// formatPortfolioPrompt outputs a portfolio summary when inside ADB_HOME.
// Uses line-based parsing to count task statuses from backlog.yaml.
func formatPortfolioPrompt(adbHome string) string {
	backlogPath := filepath.Join(adbHome, "backlog.yaml")
	// The only variable part is $ADB_HOME — the invoking user's own workspace
	// root, whose backlog this read-only open is for.
	f, err := os.Open(backlogPath) //nolint:gosec // fixed basename under the caller's own $ADB_HOME, read-only
	if err != nil {
		return ""
	}
	defer f.Close()

	var b, a, x int // backlog, active (in_progress), blocked/review
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "status:") {
			status := strings.TrimSpace(strings.TrimPrefix(line, "status:"))
			switch status {
			case "backlog":
				b++
			case "in_progress":
				a++
			case "blocked", "review":
				x++
			}
		}
	}

	if b+a+x == 0 {
		return ""
	}

	return fmt.Sprintf("\\[\\033[0;36m\\][adb %dB/%dA/%dX]\\[\\033[0m\\]", b, a, x)
}

// parseTypeFromTitle extracts the task type from "[type] description" pattern.
func parseTypeFromTitle(title string) string {
	if len(title) < 3 || title[0] != '[' {
		return "?"
	}
	end := strings.Index(title, "]")
	if end < 0 {
		return "?"
	}
	return title[1:end]
}

// statusIcon returns a single-character icon for the task status.
func statusIcon(status string) string {
	switch status {
	case "in_progress":
		return "*"
	case "blocked":
		return "!"
	case "review":
		return "?"
	case "done":
		return "+"
	case "backlog":
		return "."
	default:
		return "-"
	}
}

// priorityColor returns an ANSI color code for the priority level.
func priorityColor(priority string) string {
	switch strings.ToUpper(priority) {
	case "P0":
		return "1;37;41" // Bold white on red
	case "P1":
		return "1;31" // Bold red
	case "P2":
		return "1;36" // Bold cyan
	case "P3":
		return "0;37" // Dim white
	default:
		return "1;36" // Default cyan
	}
}
