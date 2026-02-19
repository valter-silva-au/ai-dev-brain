package cli

import (
	"os"
	"path/filepath"
	"strings"
)

const knowledgeSectionHeader = "## Accumulated Project Knowledge"

// appendKnowledgeToTaskContext appends a knowledge summary section to the
// task-context.md file in the worktree. This is non-fatal: any errors are
// silently ignored so that task creation/resume is never blocked.
func appendKnowledgeToTaskContext(worktreePath string) {
	if KnowledgeMgr == nil {
		return
	}

	summary, err := KnowledgeMgr.AssembleKnowledgeSummary(10)
	if err != nil || summary == "" {
		return
	}

	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")

	existing, err := os.ReadFile(taskContextPath)
	if err != nil {
		return
	}

	content := string(existing)

	// Remove any existing knowledge section before appending fresh content.
	content = removeKnowledgeSection(content)

	section := "\n" + knowledgeSectionHeader + "\n\n" + summary
	content = strings.TrimRight(content, "\n") + "\n" + section

	_ = os.WriteFile(taskContextPath, []byte(content), 0o644)
}

// removeKnowledgeSection strips the "## Accumulated Project Knowledge" section
// and everything after it from the content. This allows refreshing the section
// on resume without duplicating it.
func removeKnowledgeSection(content string) string {
	idx := strings.Index(content, knowledgeSectionHeader)
	if idx < 0 {
		return content
	}
	return strings.TrimRight(content[:idx], "\n") + "\n"
}

// refreshTaskContextMetadata reads task-context.md in a worktree and updates
// the Status line to match the provided status. This is non-fatal: errors are
// silently ignored so that resume is never blocked.
func refreshTaskContextMetadata(worktreePath, status string) {
	taskContextPath := filepath.Join(worktreePath, ".claude", "rules", "task-context.md")

	data, err := os.ReadFile(taskContextPath)
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	updated := false
	for i, line := range lines {
		if strings.HasPrefix(line, "- **Status**: ") {
			lines[i] = "- **Status**: " + status
			updated = true
			break
		}
	}

	if !updated {
		return
	}

	_ = os.WriteFile(taskContextPath, []byte(strings.Join(lines, "\n")), 0o644)
}
