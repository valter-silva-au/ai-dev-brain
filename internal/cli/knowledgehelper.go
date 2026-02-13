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
