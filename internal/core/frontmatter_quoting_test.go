package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/templates"
	"gopkg.in/yaml.v3"
)

// TASK-00049: every ticket template's YAML frontmatter must survive hostile,
// user-supplied values — chiefly titles containing ": " (a plain YAML scalar
// cannot hold one; a raw interpolation renders a nested mapping that breaks
// VS Code Markdown preview and every strict reader). These goldens render the
// real embedded templates through BOTH managers, strict-parse the frontmatter
// with yaml.v3, and assert value round-trip equality.
//
// To revert-guard (verify the tests bite): drop .Funcs(ticketFuncMap) from
// EmbedTemplateManager.loadTemplate or LayeredTemplateManager.parse, or swap
// {{yamlq .Title}} back to {{.Title}} in the templates — every case here must
// then FAIL to parse or round-trip.

// hostileTitles covers the YAML-plain-scalar failure modes: colon+space
// (nested mapping), leading indicator characters, flow-sequence lookalikes,
// quotes/backslashes, multi-line, and YAML 1.1 boolean lookalikes.
var hostileTitles = []string{
	"Learn pipeline: channel-catalog ingestion + per-niche stats table",
	"[feat] Whatever: the colon strikes back",
	"# leading hash",
	"- leading dash",
	"& ampersand lead",
	"* star lead",
	"? question lead",
	"[bracket] sequence lookalike",
	"{brace} mapping lookalike",
	"| pipe",
	"> gt",
	"% pct",
	"@ at",
	"` tick",
	"quote \" and backslash \\ inside",
	"' leading single quote",
	"trailing space ",
	"multi\nline title",
	"y", // YAML 1.1 boolean lookalike — yaml.v3 must quote it
	"no: more colons: than allowed: surely",
}

// hostileExtras exercise the other user-supplied frontmatter slots
// (status.yaml assignee/tags).
var hostileExtras = []string{
	"valter: owner of things",
	"",
	"quotes \"in\" the value",
	"' leading quote",
}

// fmManagers returns both production render paths; the FuncMap must be
// attached in each.
func fmManagers(t *testing.T) []TemplateManager {
	t.Helper()
	embed, err := NewEmbedTemplateManager(templates.FS)
	if err != nil {
		t.Fatalf("NewEmbedTemplateManager: %v", err)
	}
	layered, err := NewLayeredTemplateManager("", templates.FS)
	if err != nil {
		t.Fatalf("NewLayeredTemplateManager: %v", err)
	}
	return []TemplateManager{embed, layered}
}

// extractFrontmatter returns the leading `---`-delimited YAML block of a
// rendered ticket file (content between the first and second `---` lines),
// or "" when the file has no frontmatter.
func extractFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n")
		}
	}
	return ""
}

// mustParseFrontmatter strict-unmarshals a frontmatter block into a map.
func mustParseFrontmatter(t *testing.T, typ TemplateType, fm string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		t.Fatalf("frontmatter of %s is not valid YAML:\n%s\nerror: %v", typ, fm, err)
	}
	return parsed
}

func TestFrontmatterTitleRoundTrip(t *testing.T) {
	// Markdown-ticket templates: the title rides in a `---` frontmatter block.
	docTemplates := []TemplateType{TemplateTypeContext, TemplateTypeNotes, TemplateTypeDesign, TemplateTypeHandoff}
	for _, title := range hostileTitles {
		for _, tm := range fmManagers(t) {
			for _, typ := range docTemplates {
				data := map[string]interface{}{
					"TemplateVersion": "abc12345",
					"Title":           title,
					"TaskID":          "TASK-00049",
					"CreatedAt":       "2026-09-08T12:41:57+08:00",
					"UpdatedAt":       "2026-09-08T12:41:57+08:00",
				}
				out, err := tm.Render(typ, data)
				if err != nil {
					t.Fatalf("Render(%s, %q): %v", typ, title, err)
				}
				fm := extractFrontmatter(out)
				if fm == "" {
					t.Fatalf("Render(%s): no frontmatter in output:\n%s", typ, out)
				}
				parsed := mustParseFrontmatter(t, typ, fm)
				if got, ok := parsed["title"].(string); !ok || got != title {
					t.Errorf("%s: title did not round-trip:\n want %q\n got  %#v", typ, title, parsed["title"])
				}
			}
		}
	}
}

func TestStatusYAMLRoundTrip(t *testing.T) {
	// status.yaml is a whole-file YAML document, not a `---` block.
	for _, title := range hostileTitles {
		data := map[string]interface{}{
			"TaskID":    "TASK-00049",
			"Title":     title,
			"Status":    "in_progress",
			"CreatedAt": "2026-09-08T12:41:57+08:00",
			"UpdatedAt": "2026-09-08T12:41:57+08:00",
		}
		for _, tm := range fmManagers(t) {
			out, err := tm.Render(TemplateTypeStatus, data)
			if err != nil {
				t.Fatalf("Render(status, %q): %v", title, err)
			}
			var parsed map[string]any
			if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
				t.Fatalf("status.yaml not valid YAML for title %q:\n%s\nerror: %v", title, out, err)
			}
			if got, ok := parsed["title"].(string); !ok || got != title {
				t.Errorf("status.yaml: title did not round-trip:\n want %q\n got  %#v", title, parsed["title"])
			}
		}
	}
}

func TestStatusUserFieldsRoundTrip(t *testing.T) {
	data := map[string]interface{}{
		"TaskID":    "TASK-00049",
		"Title":     "title: with colon",
		"Status":    "in_progress",
		"Assignee":  hostileExtras[0],
		"Tags":      hostileExtras,
		"CreatedAt": "2026-09-08T12:41:57+08:00",
		"UpdatedAt": "2026-09-08T12:41:57+08:00",
	}
	for _, tm := range fmManagers(t) {
		out, err := tm.Render(TemplateTypeStatus, data)
		if err != nil {
			t.Fatalf("Render(status): %v", err)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("status.yaml not valid YAML:\n%s\nerror: %v", out, err)
		}
		if got := parsed["assignee"]; got != hostileExtras[0] {
			t.Errorf("assignee round-trip: want %#v, got %#v", hostileExtras[0], got)
		}
		gotTags, ok := parsed["tags"].([]interface{})
		if !ok || len(gotTags) != len(hostileExtras) {
			t.Fatalf("tags: want list of %d, got %#v", len(hostileExtras), parsed["tags"])
		}
		for i, want := range hostileExtras {
			if gotTags[i] != want {
				t.Errorf("tags[%d]: want %#v, got %#v", i, want, gotTags[i])
			}
		}
	}
}

func TestPlainScalarsStayPlain(t *testing.T) {
	// Machine-generated values must not gain quote noise: yamlq emits a plain
	// scalar wherever the YAML grammar allows it, so the diff churn on
	// existing generated files stays near-zero.
	data := map[string]interface{}{
		"TaskID":    "TASK-00049",
		"Title":     "no colon in sight",
		"Status":    "in_progress",
		"CreatedAt": "2026-09-08T12:41:57+08:00",
		"UpdatedAt": "2026-09-08T12:41:57+08:00",
	}
	for _, tm := range fmManagers(t) {
		out, err := tm.Render(TemplateTypeStatus, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		for _, line := range strings.Split(out, "\n") {
			for _, key := range []string{"task_id", "status", "created_at", "updated_at"} {
				if strings.HasPrefix(line, key+":") && strings.Contains(line, `"`) {
					t.Errorf("%s gained quote noise: %q", key, line)
				}
			}
		}
		if !strings.Contains(out, "title: no colon in sight") {
			t.Errorf("plain title should render as an unquoted plain scalar, got:\n%s", out)
		}
	}
}

func TestMachineKeysStayUnquotedForDriftRegex(t *testing.T) {
	// ticketdrift.go reads template/template_version via a tolerant regex
	// expecting unquoted values; they must never be emitter-quoted.
	data := map[string]interface{}{
		"TemplateVersion": "abc12345",
		"Title":           "title: with colon",
		"TaskID":          "TASK-00049",
		"CreatedAt":       "2026-09-08T12:41:57+08:00",
	}
	for _, tm := range fmManagers(t) {
		out, err := tm.Render(TemplateTypeContext, data)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		fm := mustParseFrontmatter(t, TemplateTypeContext, extractFrontmatter(out))
		if fmt.Sprint(parsedString(fm, "template")) != "ticket/context" {
			t.Errorf("template key wrong: %#v", fm["template"])
		}
		if parsedString(fm, "template_version") != "abc12345" {
			t.Errorf("template_version wrong: %#v", fm["template_version"])
		}
		for _, line := range strings.Split(extractFrontmatter(out), "\n") {
			if strings.HasPrefix(line, "template:") && strings.Contains(line, `"`) {
				t.Errorf("template key must stay unquoted (drift regex), got %q", line)
			}
			if strings.HasPrefix(line, "template_version:") && strings.Contains(line, `"`) {
				t.Errorf("template_version must stay unquoted (drift regex), got %q", line)
			}
		}
	}
}

func parsedString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
