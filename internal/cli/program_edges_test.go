package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// CLI-level edge cases: how a BROKEN pack surfaces, how search paths are
// resolved, and the render helpers' fallback branches. The theme is that a
// misconfiguration must be visible in the output rather than silently changing
// what the command reports.

// withAppAt wires a real, ISOLATED App over dir (which must already contain any
// .taskrc the test needs) and restores the previous App afterwards. It is the one
// App-wiring helper in this package's program tests; withProgramApp is it plus a
// fresh t.TempDir().
//
// It used to be a PAIR — withAppAt (isolate $HOME, then wire) and wireAppAt (wire
// only) — because the env-based isolation repointed $HOME at a fresh empty dir, so
// a test that wanted to populate the GLOBAL tier had to opt out of the isolation
// entirely to keep the file it had just written visible. NewAppIsolated makes the
// global tier <dir>/.taskconfig, i.e. a file INSIDE the workspace the test already
// controls, so writing the global tier and being isolated are no longer in
// conflict and one helper covers both cases. See
// TestProgramCLI_SearchPathTierPrecedence, the test that forced the split.
func withAppAt(t *testing.T, dir string) *internal.App {
	t.Helper()
	app, err := internal.NewAppIsolated(dir)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	t.Cleanup(func() { _ = app.Cleanup() })
	old := App
	App = app
	t.Cleanup(func() { App = old })
	return app
}

// writeTaskrc writes a repo-tier .taskrc carrying one custom setting.
func writeTaskrc(t *testing.T, dir, key, value string) {
	t.Helper()
	body := "repo_name: demo\ntask_id_prefix: TASK\ncustom_settings:\n  " + key + ": \"" + value + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".taskrc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestProgramCLI_BrokenExternalPackIsVisibleNotSilent asserts the deliberate
// design choice in newProgramListCmd: a directory that HAS a program.yaml but
// whose manifest will not load still gets a row, marked unreadable. A broken pack
// silently vanishing from `list` is the failure mode this prevents — the author
// would have no signal at all.
func TestProgramCLI_BrokenExternalPackIsVisibleNotSilent(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		// wantErrText is a substring the reported error must contain, so the row
		// tells the author what is actually wrong.
		wantErrText string
	}{
		{
			name:        "unparseable yaml",
			manifest:    "program: [not a mapping\n",
			wantErrText: "parse program manifest",
		},
		{
			name:        "valid yaml but invalid manifest",
			manifest:    "program:\n  id: broken\n  name: Broken\n",
			wantErrText: "lineage",
		},
		{
			name: "a dependency cycle",
			manifest: `program:
  id: broken
  name: Broken
  lineage: ["src"]
phases:
  - {id: one, name: One}
templates:
  - id: a
    name: A
    phase: one
    path: t/a.md
    output: docs/a.md
    requires: [b]
    human_review: {required: true, note: n, risk: r}
  - id: b
    name: B
    phase: one
    path: t/b.md
    output: docs/b.md
    requires: [a]
    human_review: {required: true, note: n, risk: r}
`,
			wantErrText: "dependency cycle",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			packs := filepath.Join(tmp, "packs")
			packDir := filepath.Join(packs, "broken")
			if err := os.MkdirAll(packDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(packDir, "program.yaml"), []byte(tc.manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			writeTaskrc(t, tmp, "programs_search_paths", packs)
			withAppAt(t, tmp)

			// The --json row carries the error, so a scripted consumer sees it too.
			out := captureStdout(t, func() {
				if err := runADB(t, "program", "list", "--json"); err != nil {
					t.Fatalf("list --json: %v", err)
				}
			})
			var rows []programListJSON
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatalf("list --json: %v\n%s", err, out)
			}
			var found bool
			for _, r := range rows {
				if r.ID != "broken" {
					continue
				}
				found = true
				if r.Error == "" {
					t.Errorf("the broken pack reported no error: %+v", r)
					continue
				}
				if !strings.Contains(r.Error, tc.wantErrText) {
					t.Errorf("error %q does not mention %q, so the author cannot tell what is wrong",
						r.Error, tc.wantErrText)
				}
			}
			if !found {
				t.Fatalf("a broken pack must still be LISTED, not silently dropped:\n%s", out)
			}

			// And the human view marks it unreadable rather than printing a blank row.
			human := captureStdout(t, func() {
				if err := runADB(t, "program", "list"); err != nil {
					t.Fatalf("list: %v", err)
				}
			})
			if !strings.Contains(human, "unreadable") {
				t.Errorf("the human view must flag the pack as unreadable:\n%s", human)
			}

			// Asking for it directly is a hard error naming the real problem — NOT
			// "unknown program", which would send the author looking in the wrong place.
			err := runADB(t, "program", "show", "broken")
			if err == nil {
				t.Fatal("show on a malformed pack should error")
			}
			if strings.Contains(err.Error(), "unknown program") {
				t.Errorf("a malformed pack must not be reported as missing: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantErrText) {
				t.Errorf("error %q does not mention %q", err, tc.wantErrText)
			}
		})
	}
}

// TestProgramCLI_SearchPathResolution covers how the config value is split and
// resolved: multiple directories, both separators, whitespace, and a RELATIVE
// entry (which must resolve against the workspace root so a checked-in .taskrc
// stays portable across machines).
func TestProgramCLI_SearchPathResolution(t *testing.T) {
	t.Run("a relative search path resolves against the workspace root", func(t *testing.T) {
		tmp := t.TempDir()
		writePack(t, filepath.Join(tmp, "local-packs", "house-style"))
		writeTaskrc(t, tmp, "programs_search_paths", "local-packs")
		withAppAt(t, tmp)

		got := programSearchPaths()
		want := filepath.Join(tmp, "local-packs")
		if len(got) != 1 || got[0] != want {
			t.Fatalf("programSearchPaths() = %v, want [%s]", got, want)
		}
		out := captureStdout(t, func() {
			if err := runADB(t, "program", "list"); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
		if !strings.Contains(out, "house-style") {
			t.Errorf("a relative search path did not resolve:\n%s", out)
		}
	})

	t.Run("multiple comma-separated paths, with whitespace", func(t *testing.T) {
		tmp := t.TempDir()
		a := filepath.Join(tmp, "a")
		b := filepath.Join(tmp, "b")
		writePack(t, filepath.Join(a, "house-style"))
		if err := os.MkdirAll(b, 0o755); err != nil {
			t.Fatal(err)
		}
		writeTaskrc(t, tmp, "programs_search_paths", a+" , "+b)
		withAppAt(t, tmp)

		got := programSearchPaths()
		if len(got) != 2 || got[0] != a || got[1] != b {
			t.Errorf("programSearchPaths() = %v, want [%s %s] (trimmed)", got, a, b)
		}
	})

	t.Run("an empty setting resolves to no paths", func(t *testing.T) {
		tmp := t.TempDir()
		writeTaskrc(t, tmp, "programs_search_paths", "")
		withAppAt(t, tmp)
		if got := programSearchPaths(); len(got) != 0 {
			t.Errorf("programSearchPaths() = %v, want none", got)
		}
	})

	t.Run("an embedded pack wins an id collision", func(t *testing.T) {
		tmp := t.TempDir()
		packs := filepath.Join(tmp, "packs")
		// An external pack reusing a SHIPPED id must not shadow it.
		shadow := filepath.Join(packs, "technical-design")
		if err := os.MkdirAll(shadow, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := `program:
  id: technical-design
  name: IMPOSTOR
  lineage: ["src"]
phases:
  - {id: one, name: One}
templates:
  - id: a
    name: A
    phase: one
    path: t/a.md
    output: docs/a.md
    human_review: {required: true, note: n, risk: r}
`
		if err := os.WriteFile(filepath.Join(shadow, "program.yaml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTaskrc(t, tmp, "programs_search_paths", packs)
		withAppAt(t, tmp)

		out := captureStdout(t, func() {
			if err := runADB(t, "program", "show", "technical-design"); err != nil {
				t.Fatalf("show: %v", err)
			}
		})
		if strings.Contains(out, "IMPOSTOR") {
			t.Errorf("an external pack shadowed a shipped id:\n%s", out)
		}
		if !strings.Contains(out, "Technical Design") {
			t.Errorf("the embedded pack should have won the collision:\n%s", out)
		}
		// It must also appear exactly once in the list.
		rows := rawJSONRows(t, "program", "list", "--json")
		var n int
		for _, r := range rows {
			if r["id"] == "technical-design" {
				n++
				if r["source"] != "embedded" {
					t.Errorf("source = %v, want embedded", r["source"])
				}
			}
		}
		if n != 1 {
			t.Errorf("technical-design appears %d times, want exactly once", n)
		}
	})
}

// TestProgramCLI_SearchPathTierPrecedence pins how the two accepted SPELLINGS of
// the search-paths setting combine with the three config tiers.
//
// The rule is TIER-MAJOR: most-specific tier first (Repo > Org > Global), and only
// WITHIN one tier does the canonical `programs_search_paths` outrank the dotted
// `programs.search_paths`. A spelling preference must never outrank a tier
// preference.
//
// This has teeth because the resolution used to be key-major — it looped over the
// spellings, resolving each across all three tiers via MergedConfig.SettingSource —
// so the first spelling won at whatever tier it happened to sit, and a canonical key
// in `.taskconfig` (global) silently beat a dotted key in `.taskrc` (repo). That was
// unreachable while the dotted spelling killed config load outright; the tier
// flattening in internal/core/config.go made it live, and reachable.
//
// Each (tier, spelling) pair gets its OWN search directory holding a pack named
// "<tier>-<spelling>", so the pack that resolves names exactly which one won.
func TestProgramCLI_SearchPathTierPrecedence(t *testing.T) {
	const (
		canonical = "programs_search_paths"
		dotted    = "programs.search_paths"
	)
	// orgID selects the middle tier via the repo config's `org:` field rather than
	// $ADB_ORG. An isolated App ignores $ADB_ORG but still honours `org:` — that
	// field is workspace data, not ambient state — so the fixture is what selects
	// the tier, on any machine.
	const orgID = "demo-org"

	spelling := func(key string) string {
		if key == canonical {
			return "canonical"
		}
		return "dotted"
	}

	tests := []struct {
		name string
		// global/org/repo list the spellings that tier writes; nil ⇒ that tier says
		// nothing. An org entry also brings the org tier into existence.
		global, org, repo []string
		// wantPack is the pack id that must resolve, i.e. "<tier>-<spelling>".
		wantPack string
		// wantTier is the tier programSearchPathsSetting must report as the winner.
		wantTier string
	}{
		{
			// THE REGRESSION: the less-specific tier used to win on spelling alone.
			name:     "a dotted key at the repo tier beats a canonical key at the global tier",
			global:   []string{canonical},
			repo:     []string{dotted},
			wantPack: "repo-dotted",
			wantTier: "repo",
		},
		{
			name:     "both tiers canonical: the repo tier wins",
			global:   []string{canonical},
			repo:     []string{canonical},
			wantPack: "repo-canonical",
			wantTier: "repo",
		},
		{
			name:     "both tiers dotted: the repo tier wins",
			global:   []string{dotted},
			repo:     []string{dotted},
			wantPack: "repo-dotted",
			wantTier: "repo",
		},
		{
			name:     "canonical and dotted in the SAME tier: canonical wins",
			repo:     []string{canonical, dotted},
			wantPack: "repo-canonical",
			wantTier: "repo",
		},
		{
			name:     "the org tier in the middle: it beats global and loses to repo",
			global:   []string{canonical},
			org:      []string{canonical},
			repo:     []string{dotted},
			wantPack: "repo-dotted",
			wantTier: "repo",
		},
		{
			name:     "the org tier wins when the repo tier says nothing",
			global:   []string{canonical},
			org:      []string{dotted},
			wantPack: "org-dotted",
			wantTier: "org",
		},
		{
			name:     "only the global tier speaks: its value is used",
			global:   []string{dotted},
			wantPack: "global-dotted",
			wantTier: "global",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// All three tiers are files inside ws: an isolated App's GLOBAL tier is
			// <ws>/.taskconfig, its ORG tier <ws>/orgs/<id>/config.yaml, its REPO
			// tier <ws>/.taskrc. Writing the global tier used to require opting out
			// of the isolation (it lived in a fake $HOME that the isolation then
			// repointed away from — this case is the one that caught it); with the
			// seam it is just another file in the fixture.
			ws := t.TempDir()

			// settingsBlock renders one tier's custom_settings, creating a distinct
			// search dir + pack per spelling so the winner is identifiable.
			settingsBlock := func(tier string, spellings []string) string {
				if len(spellings) == 0 {
					return ""
				}
				block := "custom_settings:\n"
				for _, key := range spellings {
					pack := tier + "-" + spelling(key)
					dir := filepath.Join(ws, "search-"+pack)
					writePackAs(t, filepath.Join(dir, pack), pack, pack)
					block += "  " + key + ": \"" + dir + "\"\n"
				}
				return block
			}

			if err := os.WriteFile(filepath.Join(ws, ".taskconfig"),
				[]byte("task_id_prefix: TASK\n"+settingsBlock("global", tc.global)), 0o644); err != nil {
				t.Fatal(err)
			}
			if len(tc.org) > 0 {
				orgDir := filepath.Join(ws, "orgs", orgID)
				if err := os.MkdirAll(orgDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(orgDir, "config.yaml"),
					[]byte("org_id: "+orgID+"\n"+settingsBlock("org", tc.org)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// The repo tier always exists: it is what selects the org tier, and an
			// absent .taskrc would also change which tiers are present at all.
			if err := os.WriteFile(filepath.Join(ws, ".taskrc"),
				[]byte("repo_name: demo\ntask_id_prefix: TASK\norg: "+orgID+"\n"+
					settingsBlock("repo", tc.repo)), 0o644); err != nil {
				t.Fatal(err)
			}

			withAppAt(t, ws)

			// The setting resolves to the winning tier…
			_, gotTier, ok := programSearchPathsSetting()
			if !ok {
				t.Fatal("programSearchPathsSetting() found nothing; the fixture wrote no tier")
			}
			if gotTier != tc.wantTier {
				t.Errorf("winning tier = %q, want %q — a spelling preference outranked a tier", gotTier, tc.wantTier)
			}
			// …and that is the ONE directory searched, so exactly one external pack lists.
			wantDir := filepath.Join(ws, "search-"+tc.wantPack)
			if got := programSearchPaths(); len(got) != 1 || got[0] != wantDir {
				t.Errorf("programSearchPaths() = %v, want [%s]", got, wantDir)
			}

			// End to end: only the winner's pack is discoverable.
			rows := rawJSONRows(t, "program", "list", "--json")
			listed := map[string]bool{}
			for _, r := range rows {
				id, _ := r["id"].(string)
				listed[id] = true
			}
			if !listed[tc.wantPack] {
				t.Errorf("the winning tier's pack %q did not list; listed: %v", tc.wantPack, sortedKeys(rows[0]))
			}
			for _, tier := range []string{"global", "org", "repo"} {
				for _, sp := range []string{"canonical", "dotted"} {
					pack := tier + "-" + sp
					if pack != tc.wantPack && listed[pack] {
						t.Errorf("pack %q from a losing (tier, spelling) also resolved", pack)
					}
				}
			}
		})
	}
}

// TestProgramCLI_PrintedIDsAreAddressable pins that every program id `adb program`
// prints inside a command the reader is told to RUN is the id that actually
// resolves — the pack DIRECTORY name — never the manifest's declared id.
//
// The two can differ for an external pack (nothing requires a directory to match
// the id its manifest declares, and resolution keys on the directory), and
// `next`'s empty-result hint used to quote the manifest id:
//
//	$ adb program next dirname-x
//	manifest-y: nothing is ready — run `adb program status manifest-y` …
//	$ adb program status manifest-y
//	Error: unknown program "manifest-y"        # exit 1
//
// That is the same "prints something that resolves nowhere" defect class the `from:`
// resolution exists to eliminate, so the check here is general: extract every
// backticked `adb …` command from the output and require it to run clean.
func TestProgramCLI_PrintedIDsAreAddressable(t *testing.T) {
	const (
		packDir    = "dirname-x"
		manifestID = "manifest-y"
	)

	tests := []struct {
		name string
		// setup wires the App and returns the workspace root plus the id to ask about.
		setup func(t *testing.T) (ws, program string)
		// wantHeadings are substrings the output must carry.
		wantHeadings []string
	}{
		{
			// The empty-result hint: reachable only once every template is generated.
			name: "next's nothing-is-ready hint names the directory id, not the manifest id",
			setup: func(t *testing.T) (string, string) {
				ws := t.TempDir()
				packs := filepath.Join(ws, "packs")
				writePackAs(t, filepath.Join(packs, packDir), manifestID, "Divergent Pack")
				writeTaskrc(t, ws, "programs_search_paths", packs)
				withAppAt(t, ws)
				// The pack's only template declares this output, so writing it leaves
				// nothing ready.
				writeArtifact(t, ws, "docs/"+manifestID+"/draft/charter.md", "# Charter\n")
				return ws, packDir
			},
			wantHeadings: []string{"nothing is ready", "adb program status " + packDir},
		},
		{
			// The unprovisioned-pack marker on `from:` — its remedy must also run.
			name: "next's not-provisioned marker names a scaffold command that works",
			setup: func(t *testing.T) (string, string) {
				return withProgramApp(t), "technical-design"
			},
			wantHeadings: []string{"not provisioned", "adb program scaffold technical-design"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, program := tc.setup(t)
			out := captureStdout(t, func() {
				if err := runADB(t, "program", "next", program); err != nil {
					t.Fatalf("program next %s: %v", program, err)
				}
			})
			for _, want := range tc.wantHeadings {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, manifestID) {
				t.Errorf("output quotes the manifest id %q, which resolves nowhere:\n%s", manifestID, out)
			}

			cmds := backtickedADBCommands(out)
			if len(cmds) == 0 {
				t.Fatalf("no backticked `adb …` command in the output, so nothing was checked:\n%s", out)
			}
			for _, cmd := range cmds {
				if err := runADB(t, cmd...); err != nil {
					t.Errorf("the output told the reader to run `adb %s`, which fails: %v",
						strings.Join(cmd, " "), err)
				}
			}
		})
	}
}

// backtickedADB matches an `adb …` command quoted in rendered output — the things a
// reader is told to type.
var backtickedADB = regexp.MustCompile("`adb ([^`]+)`")

// backtickedADBCommands returns each backticked `adb …` command in out as an argv
// slice with the leading "adb" stripped, ready for runADB.
func backtickedADBCommands(out string) [][]string {
	var cmds [][]string
	for _, m := range backtickedADB.FindAllStringSubmatch(out, -1) {
		if args := strings.Fields(m[1]); len(args) > 0 {
			cmds = append(cmds, args)
		}
	}
	return cmds
}

// TestProgramCLI_StatusHeadingNamesTheAddressableID is the status half of the id
// contract above: the heading identifies the pack by the id the reader typed, so it
// is not labelled with a name no other command accepts.
func TestProgramCLI_StatusHeadingNamesTheAddressableID(t *testing.T) {
	ws := t.TempDir()
	packs := filepath.Join(ws, "packs")
	writePackAs(t, filepath.Join(packs, "dirname-x"), "manifest-y", "Divergent Pack")
	writeTaskrc(t, ws, "programs_search_paths", packs)
	withAppAt(t, ws)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "status", "dirname-x"); err != nil {
			t.Fatalf("program status dirname-x: %v", err)
		}
	})
	if !strings.Contains(out, "dirname-x — Divergent Pack") {
		t.Errorf("status should head with the id that resolves:\n%s", out)
	}

	// `show` renders the MANIFEST, so its heading is the manifest's own id — but it
	// must then say which id addresses the pack, or the reader leaves with the wrong
	// one.
	shown := captureStdout(t, func() {
		if err := runADB(t, "program", "show", "dirname-x"); err != nil {
			t.Fatalf("program show dirname-x: %v", err)
		}
	})
	if !strings.Contains(shown, "manifest-y — Divergent Pack") {
		t.Errorf("show should render the manifest's declared id:\n%s", shown)
	}
	if !strings.Contains(shown, `addressed as "dirname-x"`) {
		t.Errorf("show must name the id that resolves when it differs from the manifest id:\n%s", shown)
	}
}

// TestProgramCLI_NextWithNothingReady asserts the empty-set message is helpful:
// `operations` is entirely manual/event triggered, so nothing is ever auto-ready,
// and the command must say what to run instead of printing nothing at all.
func TestProgramCLI_NextWithNothingReady(t *testing.T) {
	withProgramApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "next", "operations"); err != nil {
			t.Fatalf("next operations: %v", err)
		}
	})
	if !strings.Contains(out, "nothing is ready") {
		t.Errorf("next should say nothing is ready:\n%s", out)
	}
	if !strings.Contains(out, "adb program status operations") {
		t.Errorf("next should point at `status` to explain why:\n%s", out)
	}

	// --json is an empty array, not null — a consumer must be able to len() it.
	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "program", "next", "operations", "--json"); err != nil {
			t.Fatalf("next --json: %v", err)
		}
	})
	if strings.TrimSpace(jsonOut) == "null" {
		t.Errorf("next --json emitted null for an empty set; want []:\n%s", jsonOut)
	}
	var rows []programTemplateJSON
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil {
		t.Fatalf("next --json is not decodable: %v\n%s", err, jsonOut)
	}
	if len(rows) != 0 {
		t.Errorf("expected no ready templates for operations, got %d", len(rows))
	}
}

// TestProgramCLI_RenderHelpers is a table over the pure render helpers, including
// the DEFENSIVE branches the normal flow cannot reach. Those branches exist so a
// status added to the model without updating the CLI shows up in the output
// instead of being silently mislabelled — which is only true if they work.
func TestProgramCLI_RenderHelpers(t *testing.T) {
	// Safe to parallelise: pure string helpers, no App and no captureStdout — the
	// two process-wide barriers that keep the rest of this package serial (see the
	// note in hook_options_test.go).
	t.Parallel()

	t.Run("stateGlyph", func(t *testing.T) {
		tests := []struct {
			status models.TemplateStatus
			want   string
		}{
			{models.TemplateGenerated, "✓"},
			{models.TemplateReady, "→"},
			{models.TemplateBlocked, "⨯"},
			{models.TemplatePending, "·"},
			// An unmodelled status must not silently borrow another glyph.
			{models.TemplateStatus("invented"), "?"},
			{models.TemplateStatus(""), "?"},
		}
		for _, tc := range tests {
			if got := stateGlyph(tc.status); got != tc.want {
				t.Errorf("stateGlyph(%q) = %q, want %q", tc.status, got, tc.want)
			}
		}
	})

	t.Run("blockedByLabel", func(t *testing.T) {
		tests := []struct{ in, want string }{
			{"prd (docs/prd.md)", "prd → docs/prd.md"},
			{"a (docs/nested/dir/a.md)", "a → docs/nested/dir/a.md"},
			// No " (" to split on: pass it through rather than mangling it.
			{"bare-id", "bare-id"},
			{"", ""},
			// A parenthesis inside the output path still cuts at the FIRST " (".
			{"x (docs/a (1).md)", "x → docs/a (1).md"},
		}
		for _, tc := range tests {
			if got := blockedByLabel(tc.in); got != tc.want {
				t.Errorf("blockedByLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})

	t.Run("humanReviewLine", func(t *testing.T) {
		tests := []struct {
			name string
			hr   *models.HumanReview
			want string
		}{
			{name: "nil is reported, not blank", hr: nil, want: "not declared"},
			{
				name: "required with a note",
				hr:   &models.HumanReview{Required: true, Note: "check the FRs"},
				want: "REQUIRED — check the FRs",
			},
			{
				name: "optional with no note",
				hr:   &models.HumanReview{Required: false},
				want: "optional",
			},
			{
				name: "a folded multi-line note collapses to one line",
				hr:   &models.HumanReview{Required: true, Note: "line one\nline  two\n"},
				want: "REQUIRED — line one line two",
			},
		}
		for _, tc := range tests {
			if got := humanReviewLine(tc.hr); got != tc.want {
				t.Errorf("%s: humanReviewLine() = %q, want %q", tc.name, got, tc.want)
			}
		}
	})

	t.Run("collapseSpace", func(t *testing.T) {
		tests := []struct{ in, want string }{
			{"one  two", "one two"},
			{"folded\nblock\ntext", "folded block text"},
			{"  leading and trailing  ", "leading and trailing"},
			{"", ""},
			{"\n\t ", ""},
		}
		for _, tc := range tests {
			if got := collapseSpace(tc.in); got != tc.want {
				t.Errorf("collapseSpace(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})
}

// TestProgramCLI_StatusSurfacesAnUnknownState drives the defensive branch in
// printProgramStatus through the real renderer: a state the switch does not model
// must appear in the output, not be folded into a neighbouring case.
func TestProgramCLI_StatusSurfacesAnUnknownState(t *testing.T) {
	p := &models.Program{
		Program: models.ProgramMeta{ID: "p", Name: "P", Lineage: []string{"src"}},
		Phases:  []models.ProgramPhase{{ID: "one", Name: "One"}},
		Templates: []models.ProgramTemplate{{
			ID: "a", Name: "A", Phase: "one", Path: "t/a.md", Output: "docs/a.md",
			HumanReview: &models.HumanReview{Required: true, Note: "n", Risk: "r"},
		}},
	}
	// printProgramStatus takes the states directly, so an unmodelled status can be
	// injected here without inventing one in the model.
	out := captureStdout(t, func() {
		printProgramStatus("p", p, []core.TemplateState{{
			Template: p.Templates[0],
			Phase:    p.Phases[0],
			Status:   models.TemplateStatus("invented"),
		}})
	})
	if !strings.Contains(out, "unknown status") {
		t.Errorf("an unmodelled status must be surfaced, not folded into another case:\n%s", out)
	}
	if !strings.Contains(out, "invented") {
		t.Errorf("the unmodelled status value should be named:\n%s", out)
	}
}

// TestProgramCLI_EmptyProgramIDIsRejected covers the reachable empty-id path:
// cobra's ExactArgs(1) is satisfied by an EMPTY string, so `adb program show ""`
// reaches pack resolution with nothing to resolve. It must say so rather than
// reporting an unknown program (which would imply the id was looked up).
func TestProgramCLI_EmptyProgramIDIsRejected(t *testing.T) {
	withProgramApp(t)

	for _, args := range [][]string{
		{"program", "show", ""},
		{"program", "status", ""},
		{"program", "next", ""},
		{"program", "scaffold", ""},
		{"program", "trace", "", "some-template"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			err := runADB(t, args...)
			if err == nil {
				t.Fatal("an empty program id should error")
			}
			if !strings.Contains(err.Error(), "program id is required") {
				t.Errorf("error %q should say the program id is required", err)
			}
		})
	}
}
