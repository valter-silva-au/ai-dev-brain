package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// This file guards ONE property, and it is the reason it exists at all:
//
//	every `adb …` spelling that `adb init` writes into a USER-OWNED file must
//	name a command that currently exists AND is advertised.
//
// `adb init workspace`, `adb init claude` and `adb init project` do not print
// documentation — they WRITE it, into the workspace's own README.md / CLAUDE.md /
// .claude/project_context.md. Those files then outlive the release that produced
// them: a rename lands, the docs in this repo get fixed in the same commit, and
// every workspace created from that build still ships a README naming a command
// whose visible spelling no longer exists. A grep over docs/ cannot catch that,
// because the stale text is a Go string literal and an embedded template, not a
// document.
//
// Two deliberate strictnesses, both of which caught something real:
//
//   - A HIDDEN command does not count. `adb task status` survives the batch-2c
//     rename as a hidden deprecated alias (spec §2), so "does this command
//     resolve?" would have passed on the old spelling forever. Generated docs must
//     name the canonical, advertised spelling.
//   - A cobra ALIAS does not count either, for the same reason: it is a secondary
//     spelling, and a scaffolded README should teach the primary one.
//
// The assertions read the files OFF DISK after running the real commands rather
// than reaching into the string literals, because what the user gets is the file —
// and for `init project` the content is an embedded template, which has no literal
// to reach into.

// ── the command index ────────────────────────────────────────────────────────

// cmdIndex is the whole command tree flattened to space-joined paths.
// `all` holds every path including hidden ones and cobra aliases (so a failure can
// say *why* a spelling is wrong); `advertised` holds only paths whose command and
// every ancestor are non-hidden, and which are the command's primary Name().
type cmdIndex struct {
	all        map[string]bool
	advertised map[string]bool
}

func indexCommands(root *cobra.Command) cmdIndex {
	idx := cmdIndex{all: map[string]bool{}, advertised: map[string]bool{}}

	var walk func(parent *cobra.Command, path []string, hiddenAncestor bool)
	walk = func(parent *cobra.Command, path []string, hiddenAncestor bool) {
		for _, sub := range parent.Commands() {
			name := sub.Name()
			// cobra generates these; they are not part of adb's surface.
			if name == "help" || name == "completion" {
				continue
			}
			childPath := append(append([]string{}, path...), name)
			key := strings.Join(childPath, " ")
			hidden := hiddenAncestor || sub.Hidden

			idx.all[key] = true
			if !hidden {
				idx.advertised[key] = true
			}
			for _, alias := range sub.Aliases {
				aliasPath := append(append([]string{}, path...), alias)
				idx.all[strings.Join(aliasPath, " ")] = true
			}
			walk(sub, childPath, hidden)
		}
	}
	walk(root, nil, false)
	return idx
}

// longestPrefix returns the longest leading run of toks that names a command in
// set, and how many tokens that consumed.
func longestPrefix(set map[string]bool, toks []string) (string, int) {
	best, bestN := "", 0
	for n := 1; n <= len(toks); n++ {
		candidate := strings.Join(toks[:n], " ")
		if set[candidate] {
			best, bestN = candidate, n
		}
	}
	return best, bestN
}

// ── extracting invocations from prose ────────────────────────────────────────

var cmdToken = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// terminators end an invocation when they are glued to the last token: a closing
// backtick is the important one, because "`adb task create` to create new tasks"
// would otherwise swallow the English that follows into the command path.
const invocationTerminators = "`\"'.,;:)"

// extractADBInvocations finds every `adb …` occurrence in text and returns the
// command path each one names, as a space-joined string.
//
// Tokenizing stops at the first field that cannot be a command name (a flag, a
// `<placeholder>`, a `#` comment, an English word after a closing backtick), so a
// documentation line reads as exactly the command path it means.
func extractADBInvocations(text string) []string {
	var found []string
	rest := text
	for {
		i := strings.Index(rest, "adb ")
		if i < 0 {
			return found
		}
		after := rest[i+len("adb "):]
		if i > 0 && isWordByte(rest[i-1]) {
			// part of a longer word (e.g. "…adb …" inside an identifier)
			rest = after
			continue
		}

		var toks []string
		for _, field := range strings.Fields(after) {
			trimmed := strings.Trim(field, invocationTerminators)
			if !cmdToken.MatchString(trimmed) {
				break
			}
			toks = append(toks, trimmed)
			if trimmed != field {
				break // the token carried a terminator — the invocation ends here
			}
		}
		if len(toks) > 0 {
			found = append(found, strings.Join(toks, " "))
		}
		rest = after
	}
}

func isWordByte(b byte) bool {
	return b == '-' || b == '_' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// ── the assertion ────────────────────────────────────────────────────────────

// retiredSpellings are the exact substrings a generated artifact must never carry.
// These are cheap, blunt, and independent of the tree walk on purpose: they still
// fail if a future refactor accidentally re-advertises one of these namespaces.
var retiredSpellings = []string{
	"adb task status",
	"adb task delete",
	"adb task cleanup",
	"adb task priority",
	"adb task unarchive",
	"adb dashboard",
	"adb work ",
	"adb sync ",
	"adb chat",
	"adb session",
	"adb memory",
}

// assertGeneratedContent runs both halves of the guard over one artifact.
func assertGeneratedContent(t *testing.T, idx cmdIndex, label, content string) {
	t.Helper()

	if strings.TrimSpace(content) == "" {
		t.Fatalf("%s: generated content is empty — the artifact was not produced", label)
	}

	for _, bad := range retiredSpellings {
		if strings.Contains(content, bad) {
			t.Errorf("%s names the retired spelling %q:\n%s", label, bad, content)
		}
	}

	for _, invocation := range extractADBInvocations(content) {
		toks := strings.Fields(invocation)
		resolved, n := longestPrefix(idx.all, toks)
		switch {
		case n == 0:
			t.Errorf("%s: `adb %s` names no command at all", label, invocation)
		case n < len(toks):
			t.Errorf("%s: `adb %s` — %q resolves but %q does not name a subcommand of it",
				label, invocation, resolved, strings.Join(toks[n:], " "))
		case !idx.advertised[resolved]:
			t.Errorf("%s: `adb %s` resolves only to a hidden command or an alias; "+
				"a generated artifact must name the advertised spelling", label, invocation)
		}
	}
}

// TestGeneratedArtifacts_NameOnlyAdvertisedCommands is the guard itself: run the
// three init commands into throwaway workspaces, then read back every file they
// wrote that documents a command.
func TestGeneratedArtifacts_NameOnlyAdvertisedCommands(t *testing.T) {
	idx := indexCommands(NewRootCmd())
	if !idx.advertised["task create"] {
		t.Fatalf("command index looks broken: `task create` is not advertised (%d paths indexed)", len(idx.all))
	}

	// `init workspace` — README.md, plus its printed Next steps.
	wsDir := t.TempDir()
	withAppAt(t, wsDir)
	wsOut := captureStdout(t, func() {
		if err := runADB(t, "init", "workspace", wsDir); err != nil {
			t.Fatalf("init workspace: %v", err)
		}
	})
	assertGeneratedContent(t, idx, "init workspace README.md", readGenerated(t, wsDir, "README.md"))
	assertGeneratedContent(t, idx, "init workspace stdout", wsOut)

	// `init claude` — CLAUDE.md.
	if err := runADB(t, "init", "claude", wsDir); err != nil {
		t.Fatalf("init claude: %v", err)
	}
	assertGeneratedContent(t, idx, "init claude CLAUDE.md", readGenerated(t, wsDir, "CLAUDE.md"))

	// `init project` — the embedded .claude/ tree, plus its printed Next steps.
	projDir := filepath.Join(t.TempDir(), "proj")
	projOut := captureStdout(t, func() {
		if err := runADB(t, "init", "project", projDir); err != nil {
			t.Fatalf("init project: %v", err)
		}
	})
	assertGeneratedContent(t, idx, "init project stdout", projOut)
	assertGeneratedContent(t, idx, "init project .claude/project_context.md",
		readGenerated(t, projDir, filepath.Join(".claude", "project_context.md")))
}

// readGenerated reads one generated artifact, failing the test when it is absent —
// a missing file would otherwise make every assertion above vacuously pass.
func readGenerated(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read generated %s: %v", rel, err)
	}
	return string(data)
}

// TestExtractADBInvocations covers the tokenizer directly, because the guard above
// is only as strong as its parse: a tokenizer that quietly extracted nothing would
// make the whole file pass on any content at all.
func TestExtractADBInvocations(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"plain", "adb metrics", []string{"metrics"}},
		{"two words", "adb task list", []string{"task list"}},
		{"backtick stops the path", "- Use `adb task create` to create new tasks", []string{"task create"}},
		{"placeholder stops the path", "adb task resume <task-id>", []string{"task resume"}},
		{"dash description stops the path", "- `adb task list` - View all tasks", []string{"task list"}},
		{"hash comment stops the path", "   adb task list                  # See what is in flight", []string{"task list"}},
		{"flag stops the path", "adb task list --json --git", []string{"task list"}},
		{"several per text", "run `adb task list`, then `adb context build`", []string{"task list", "context build"}},
		{"bare adb yields nothing", "the `adb` binary", nil},
		{"no occurrence", "nothing to see", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractADBInvocations(tc.in)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("extractADBInvocations(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestIndexCommands_HiddenDoesNotCountAsAdvertised pins the strictness that makes
// the guard useful: a hidden deprecated alias is present in `all` (so a failure can
// explain itself) and absent from `advertised` (so a generated artifact may not
// name it). Without this, every renamed command would keep passing via its alias.
func TestIndexCommands_HiddenDoesNotCountAsAdvertised(t *testing.T) {
	root := &cobra.Command{Use: "adb"}
	visible := &cobra.Command{Use: "task"}
	visible.AddCommand(&cobra.Command{Use: "list"})
	visible.AddCommand(&cobra.Command{Use: "status", Hidden: true})
	aliased := &cobra.Command{Use: "context", Aliases: []string{"ctx"}}
	hiddenParent := &cobra.Command{Use: "work", Hidden: true}
	hiddenParent.AddCommand(&cobra.Command{Use: "list"})
	root.AddCommand(visible, aliased, hiddenParent)

	idx := indexCommands(root)

	for _, key := range []string{"task list", "context"} {
		if !idx.advertised[key] {
			t.Errorf("%q should be advertised", key)
		}
	}
	// A hidden command, a child of a hidden parent, and an alias are all *known*
	// but none of them is advertised.
	for _, key := range []string{"task status", "work", "work list", "ctx"} {
		if !idx.all[key] {
			t.Errorf("%q should be known", key)
		}
		if idx.advertised[key] {
			t.Errorf("%q must not be advertised", key)
		}
	}
}
