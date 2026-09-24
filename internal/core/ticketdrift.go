package core

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// This file is the TICKET DRIFT ENGINE (TASK-00031 phase 2): read-only
// inspection plus the one-time de-dup migration for tickets bootstrapped by
// older binaries.
//
// It is INSPECTION ONLY. The frontmatter *normalizer* that used to live at the
// bottom of this file (TASK-00049) went with `adb task normalize --frontmatter`
// when that command was removed in TASK-00039: repairing drift is a manual edit
// to the ticket file now, and `adb task validate` only reports it.
//
// Two drift kinds, both consequences of the retired duplicate-seed bug
// (bootstrap once fed the description into notes.md as well as context.md):
//
//   - duplicate_seed: notes.md still carries the verbatim description that
//     context.md already holds — the divergence anti-pattern that turned
//     notes.md files into everything-logs.
//   - stale_template: the ticket's frontmatter template_version differs from
//     the currently resolved template — the ticket was bootstrapped by an adb
//     binary whose embedded template has since moved on (exactly what happened
//     on this monorepo, where PATH adb ran 8 commits behind HEAD until the
//     2026-08-31 reinstall).

// TicketFinding is one drift warning about one ticket file.
type TicketFinding struct {
	TaskID string
	File   string // basename the finding is about ("notes.md")
	Kind   string // "duplicate_seed" | "stale_template" | "unparsable_template"
	Detail string
}

// ticketFrontmatter is the minimal machine layer parsed out of a rendered
// ticket file. Absent (legacy) frontmatter means both stamps are empty.
type ticketFrontmatter struct {
	Template        string
	TemplateVersion string
}

// frontmatterRe matches the YAML frontmatter block at the top of a ticket file.
var frontmatterLine = regexp.MustCompile(`^(template|template_version):\s*(\S.*)$`)

// parseTicketFrontmatter reads the leading `---` block (if any) and extracts
// the machine-layer keys. Everything else is ignored — the machine layer is
// read tolerantly, never strictly.
func parseTicketFrontmatter(content string) ticketFrontmatter {
	fm := ticketFrontmatter{}
	lines := strings.SplitN(content, "\n", 64)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fm
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if m := frontmatterLine.FindStringSubmatch(line); m != nil {
			switch m[1] {
			case "template":
				fm.Template = strings.TrimSpace(m[2])
			case "template_version":
				fm.TemplateVersion = strings.TrimSpace(m[2])
			}
		}
	}
	return fm
}

// ExtractSection returns the body of the named markdown section ("## Title")
// from content, up to the next heading of the same or higher level. Empty when
// the section is absent.
func ExtractSection(content, title string) string {
	heading := "## " + title
	var body []string
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				break // next same-level section starts
			}
			if strings.TrimSpace(line) == heading {
				inSection = true
				continue
			}
		} else if inSection && strings.HasPrefix(line, "#") {
			break
		}
		if inSection {
			body = append(body, line)
		}
	}
	return strings.Trim(strings.Join(body, "\n"), "\n")
}

// notesFile is the file the duplicate-seed check inspects (a named constant so
// InspectTicket and DedupTicketSeed agree on the filename).
const ticketNotesFile = "notes.md"

// InspectTicket runs the drift checks over one ticket directory. description
// is the task's description (from the backlog model — the ground truth the
// seed duplicated). Checks:
//
//  1. duplicate_seed — the description appears verbatim in notes.md
//     (context.md remains the sole carrier of the why).
//  2. stale_template — frontmatter template_version != the currently resolved
//     template's stamp (only when the file carries frontmatter; a
//     pre-frontmatter ticket simply has no stamp to compare).
//
// Missing files are not findings — the ticket may predate a given file.
func InspectTicket(ticketDir, description string, lm *LayeredTemplateManager) []TicketFinding {
	var findings []TicketFinding

	b, err := os.ReadFile(filepath.Join(ticketDir, ticketNotesFile))
	if err == nil {
		content := string(b)
		desc := strings.TrimSpace(description)
		if desc != "" && strings.Contains(content, desc) {
			findings = append(findings, TicketFinding{
				Kind:   "duplicate_seed",
				Detail: "notes.md still carries the verbatim seeded description — run `adb task normalize --dedup`",
			})
			return findings
		}
		if fm := parseTicketFrontmatter(content); fm.TemplateVersion != "" {
			if want, verr := lm.Version(TemplateTypeNotes); verr == nil && fm.TemplateVersion != want {
				findings = append(findings, TicketFinding{
					Kind:   "stale_template",
					Detail: fmt.Sprintf("template_version %s is older than the resolved template (%s)", fm.TemplateVersion, want),
				})
			}
		}
	}
	return findings
}
