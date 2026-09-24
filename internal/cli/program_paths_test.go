package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file guards the PRINTED PATH contract of `adb program`.
//
// It exists because of a real, shipped defect: `adb program next` printed its
// `from:` value as a PACK-relative template path, which does not resolve when
// copy-pasted from the workspace root where `write:` and `reads:` are anchored.
// It was fixed in 9134a4f with no regression test — this is that test, plus the
// generalisation: every path any subcommand prints is classified and checked.
//
// The rule being pinned: a path printed by a program subcommand is either
//   (a) resolvable from the workspace root right now, or
//   (b) an absolute path, or
//   (c) a DESTINATION that does not exist yet by definition (write:, output:).
// A pack-relative path is none of those, and is what regressed before.

// scaffoldProgramInto provisions a pack into the workspace at programs/<id>/ —
// the layout `adb init project` produces and the one `next`'s `from:` path is
// anchored to. Returns the pack root on disk.
func scaffoldProgramInto(t *testing.T, id string) string {
	t.Helper()
	if err := runADB(t, "program", "scaffold", id); err != nil {
		t.Fatalf("program scaffold %s: %v", id, err)
	}
	return filepath.Join(App.BasePath, "programs", id)
}

// fieldValues pulls the values of a "  <label>: <value>" field out of rendered
// CLI output, in order. Used to read back the exact strings a user would copy.
func fieldValues(out, label string) []string {
	var vals []string
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		prefix := label + ":"
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		if v := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)); v != "" {
			vals = append(vals, v)
		}
	}
	return vals
}

// TestProgramCLI_NextFromPathResolvesFromWorkspaceRoot is the regression test for
// the defect fixed in 9134a4f: the `from:` path `adb program next` prints must
// resolve from the WORKSPACE ROOT, not only from inside the pack.
func TestProgramCLI_NextFromPathResolvesFromWorkspaceRoot(t *testing.T) {
	tmp := withProgramApp(t)
	scaffoldProgramInto(t, "technical-design")

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "next", "technical-design"); err != nil {
			t.Fatalf("program next: %v", err)
		}
	})

	froms := fieldValues(out, "from")
	if len(froms) == 0 {
		t.Fatalf("program next printed no from: path in a fresh provisioned workspace:\n%s", out)
	}
	for _, from := range froms {
		if filepath.IsAbs(from) {
			t.Errorf("from: %q is absolute; it should be workspace-relative and copy-pasteable", from)
			continue
		}
		// The whole point: join it to the workspace root and it must be there.
		resolved := filepath.Join(tmp, filepath.FromSlash(from))
		info, err := os.Stat(resolved)
		if err != nil {
			t.Errorf("from: %q does not resolve from the workspace root (%s): %v", from, resolved, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("from: %q resolves to a directory, want the template file", from)
		}
		// And it must be the provisioned pack path, not the bare pack-relative one
		// that regressed (which would have started with "templates/").
		if !strings.HasPrefix(from, "programs/") {
			t.Errorf("from: %q should be anchored under programs/<id>/, got a pack-relative path", from)
		}
	}
}

// TestProgramCLI_NextFromPathPrecedence walks all three branches of the `from:`
// resolution the fix introduced. The printed path used to be anchored at
// programs/<id>/ unconditionally, which is only correct for a PROVISIONED pack —
// so an external pack on a search path, and an embedded pack nobody had scaffolded
// yet, both got a path that resolved nowhere with nothing said about it.
//
// Each case names the branch, so a failure says which of the three moved.
func TestProgramCLI_NextFromPathPrecedence(t *testing.T) {
	// externalWorkspace wires a workspace whose repo tier points at a search path
	// holding the house-style pack, and returns the workspace and the pack root.
	externalWorkspace := func(t *testing.T) (ws, packRoot string) {
		t.Helper()
		ws = t.TempDir()
		packs := filepath.Join(ws, "private-packs")
		packRoot = filepath.Join(packs, "house-style")
		writePack(t, packRoot)
		writeTaskrc(t, ws, "programs_search_paths", packs)
		withAppAt(t, ws)
		return ws, packRoot
	}

	tests := []struct {
		name string
		// setup wires the App and returns the workspace root plus the program to ask
		// `next` about.
		setup func(t *testing.T) (ws, program string)
		// check asserts on the single from: value the command printed.
		check func(t *testing.T, ws, from string)
	}{
		{
			name: "provisioned: the workspace-relative path, verified on disk",
			setup: func(t *testing.T) (string, string) {
				ws := withProgramApp(t)
				scaffoldProgramInto(t, "technical-design")
				return ws, "technical-design"
			},
			check: func(t *testing.T, ws, from string) {
				if filepath.IsAbs(from) {
					t.Fatalf("from: %q should be workspace-relative once provisioned", from)
				}
				if !strings.HasPrefix(from, "programs/technical-design/") {
					t.Errorf("from: %q should be anchored under programs/<id>/", from)
				}
				if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(from))); err != nil {
					t.Errorf("from: %q does not resolve from the workspace root: %v", from, err)
				}
			},
		},
		{
			name: "external and unprovisioned: the absolute search-path location",
			setup: func(t *testing.T) (string, string) {
				ws, _ := externalWorkspace(t)
				return ws, "house-style"
			},
			check: func(t *testing.T, ws, from string) {
				// A search path need not live inside the workspace, so there is no
				// relative form to print — only the absolute location the template is
				// actually at.
				if !filepath.IsAbs(from) {
					t.Fatalf("from: %q should be absolute for an unprovisioned external pack", from)
				}
				if _, err := os.Stat(from); err != nil {
					t.Errorf("from: %q does not exist: %v", from, err)
				}
				want := filepath.Join(ws, "private-packs", "house-style", "templates", "draft", "charter.md")
				if from != want {
					t.Errorf("from: %q, want the pack's real template %q", from, want)
				}
			},
		},
		{
			name: "external and then provisioned: the provisioned copy wins",
			setup: func(t *testing.T) (string, string) {
				ws, _ := externalWorkspace(t)
				scaffoldProgramInto(t, "house-style")
				return ws, "house-style"
			},
			check: func(t *testing.T, ws, from string) {
				if want := "programs/house-style/templates/draft/charter.md"; from != want {
					t.Errorf("from: %q, want the provisioned copy %q", from, want)
				}
				if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(from))); err != nil {
					t.Errorf("from: %q does not resolve from the workspace root: %v", from, err)
				}
			},
		},
		{
			name: "embedded and unprovisioned: the path plus how to get it there",
			setup: func(t *testing.T) (string, string) {
				// No scaffold: an embedded pack's templates live inside the binary
				// until one happens, so nothing under programs/ exists yet.
				return withProgramApp(t), "technical-design"
			},
			check: func(t *testing.T, ws, from string) {
				// The path is still shown — it is where the template WILL be — but the
				// line has to say it is not there and what fixes that, or it is the
				// unresolvable path this whole fix is about.
				if !strings.HasPrefix(from, "programs/technical-design/") {
					t.Errorf("from: %q should still name the provisioned path", from)
				}
				if !strings.Contains(from, "not provisioned") {
					t.Errorf("from: %q must say the template is not on disk yet", from)
				}
				if !strings.Contains(from, "adb program scaffold technical-design") {
					t.Errorf("from: %q must name the command that provisions it", from)
				}
				// It must say WHAT WAS CHECKED, not just "not provisioned": only
				// programs/<id>/ is looked at, so a pack installed with
				// `scaffold <id> <custom-dest>` also lands here despite being
				// provisioned. Naming the directory keeps the message true in that case.
				if !strings.Contains(from, "not provisioned at programs/technical-design/") {
					t.Errorf("from: %q must name the directory that was checked, since a "+
						"custom scaffold dest is provisioned but not found there", from)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws, program := tc.setup(t)
			out := captureStdout(t, func() {
				if err := runADB(t, "program", "next", program); err != nil {
					t.Fatalf("program next %s: %v", program, err)
				}
			})
			froms := fieldValues(out, "from")
			if len(froms) == 0 {
				t.Fatalf("program next printed no from: path:\n%s", out)
			}
			for _, from := range froms {
				tc.check(t, ws, from)
			}
		})
	}
}

// TestProgramCLI_CustomScaffoldDestWordingIsHonest covers the one case branch 3's
// wording used to get wrong: `adb program scaffold <id> <custom-dest>` installs the
// pack somewhere else, nothing records where, and only programs/<id>/ is checked —
// so `next` reported the pack as simply "not provisioned" when it demonstrably was.
//
// The invariant is intact either way (the path is marked as not-there and the
// suggested remedy works), so this is about the message being TRUE: it must say
// which directory was checked.
func TestProgramCLI_CustomScaffoldDestWordingIsHonest(t *testing.T) {
	tmp := withProgramApp(t)

	elsewhere := filepath.Join(tmp, "vendor-docs", "td")
	if err := runADB(t, "program", "scaffold", "technical-design", elsewhere); err != nil {
		t.Fatalf("scaffold to a custom dest: %v", err)
	}
	// The pack really is provisioned — just not where `next` looks.
	if _, err := os.Stat(filepath.Join(elsewhere, "program.yaml")); err != nil {
		t.Fatalf("fixture is wrong, the custom dest should hold the pack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "programs", "technical-design")); !os.IsNotExist(err) {
		t.Fatalf("a custom dest must not also populate programs/<id>/ (err=%v)", err)
	}

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "next", "technical-design"); err != nil {
			t.Fatalf("program next: %v", err)
		}
	})
	froms := fieldValues(out, "from")
	if len(froms) == 0 {
		t.Fatalf("program next printed no from: path:\n%s", out)
	}
	for _, from := range froms {
		// The claim has to be scoped to the path that was actually stat'd.
		if !strings.Contains(from, "not provisioned at programs/technical-design/") {
			t.Errorf("from: %q claims the pack is not provisioned without saying where it "+
				"looked, but it IS provisioned at %s", from, elsewhere)
		}
	}
}

// TestProgramCLI_TemplateLocationJSONMatchesHumanFrom pins the machine half of the
// `from:` fix: `status`/`next` --json must carry the SAME resolved template location
// the human view prints. It did not — the rows had no template path at all, so the
// fix was invisible to `--json`, which is what an agent reads.
//
// One case per resolution branch, each also asserting what a consumer needs to act:
// whether template_from can be read right now.
func TestProgramCLI_TemplateLocationJSONMatchesHumanFrom(t *testing.T) {
	tests := []struct {
		name string
		// setup wires the App and returns the workspace root plus the program id.
		setup func(t *testing.T) (ws, program string)
		// wantSource is the template_source branch marker.
		wantSource string
		// wantAbs says template_from is an absolute path rather than workspace-relative.
		wantAbs bool
		// wantReadable says template_from must exist on disk right now.
		wantReadable bool
	}{
		{
			name: "provisioned: workspace-relative and readable",
			setup: func(t *testing.T) (string, string) {
				ws := withProgramApp(t)
				scaffoldProgramInto(t, "technical-design")
				return ws, "technical-design"
			},
			wantSource:   "provisioned",
			wantReadable: true,
		},
		{
			name: "external search path: absolute and readable",
			setup: func(t *testing.T) (string, string) {
				ws := t.TempDir()
				packs := filepath.Join(ws, "private-packs")
				writePack(t, filepath.Join(packs, "house-style"))
				writeTaskrc(t, ws, "programs_search_paths", packs)
				withAppAt(t, ws)
				return ws, "house-style"
			},
			wantSource:   "search_path",
			wantAbs:      true,
			wantReadable: true,
		},
		{
			name: "embedded and unprovisioned: the destination, explicitly NOT readable",
			setup: func(t *testing.T) (string, string) {
				return withProgramApp(t), "technical-design"
			},
			wantSource:   "unprovisioned",
			wantReadable: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws, program := tc.setup(t)

			human := captureStdout(t, func() {
				if err := runADB(t, "program", "next", program); err != nil {
					t.Fatalf("program next %s: %v", program, err)
				}
			})
			froms := fieldValues(human, "from")
			rows := programStatusJSON(t, program)

			// Index the human from: values by the template id they follow, so the two
			// views are compared per template rather than positionally.
			readyRows := make([]programTemplateJSON, 0, len(rows))
			for _, r := range rows {
				if r.State == "ready" {
					readyRows = append(readyRows, r)
				}
			}
			if len(readyRows) == 0 || len(froms) != len(readyRows) {
				t.Fatalf("fixture mismatch: %d ready rows, %d from: lines:\n%s", len(readyRows), len(froms), human)
			}

			for i, row := range readyRows {
				if row.TemplateSource != templateLocationKind(tc.wantSource) {
					t.Errorf("%s: template_source = %q, want %q", row.ID, row.TemplateSource, tc.wantSource)
				}
				// template_path is the manifest's own pack-relative spelling — the same
				// value `show --json` reports as templates[].path.
				if row.TemplatePath == "" || filepath.IsAbs(row.TemplatePath) ||
					strings.HasPrefix(row.TemplatePath, "programs/") {
					t.Errorf("%s: template_path = %q, want the raw pack-relative path", row.ID, row.TemplatePath)
				}
				// The human line is template_from plus, at most, the not-provisioned
				// marker — so the machine view never disagrees with the human one.
				if !strings.HasPrefix(froms[i], row.TemplateFrom) {
					t.Errorf("%s: human from: %q does not start with template_from %q",
						row.ID, froms[i], row.TemplateFrom)
				}
				if got := filepath.IsAbs(row.TemplateFrom); got != tc.wantAbs {
					t.Errorf("%s: template_from %q absolute=%v, want %v", row.ID, row.TemplateFrom, got, tc.wantAbs)
				}

				resolved := row.TemplateFrom
				if !tc.wantAbs {
					resolved = filepath.Join(ws, filepath.FromSlash(row.TemplateFrom))
				}
				_, err := os.Stat(resolved)
				if tc.wantReadable && err != nil {
					t.Errorf("%s: template_from %q should be readable now: %v", row.ID, row.TemplateFrom, err)
				}
				if !tc.wantReadable {
					if err == nil {
						t.Errorf("%s: template_from %q exists, so template_source should not be %q",
							row.ID, row.TemplateFrom, row.TemplateSource)
					}
					// And the human view must say so, or the two views disagree about
					// whether the path can be opened.
					if !strings.Contains(froms[i], "not provisioned") {
						t.Errorf("%s: human from: %q must mark the path as not on disk", row.ID, froms[i])
					}
				}
			}
		})
	}
}

// TestProgramCLI_PrintedPathsResolve generalises the regression above into a walk
// over every subcommand that prints a path. Each case says which field it reads
// and how that field's value is required to behave.
func TestProgramCLI_PrintedPathsResolve(t *testing.T) {
	// pathKind is how a printed path is allowed to behave.
	type pathKind int
	const (
		mustResolve pathKind = iota // relative to the workspace root, exists now
		mustBeAbs                   // an absolute on-disk path that exists now
		destination                 // workspace-relative, need NOT exist yet
	)

	tests := []struct {
		name string
		// args is the subcommand to run.
		args []string
		// label is the "  <label>: <value>" field to read (empty ⇒ whole-line rule).
		label string
		kind  pathKind
		// setup runs before the command, e.g. to generate an artifact.
		setup func(t *testing.T, ws string)
	}{
		{
			name:  "next from: is the provisioned template and resolves",
			args:  []string{"program", "next", "technical-design"},
			label: "from",
			kind:  mustResolve,
		},
		{
			name:  "next write: is a workspace-relative destination",
			args:  []string{"program", "next", "technical-design"},
			label: "write",
			kind:  destination,
		},
		{
			name:  "show output: is a workspace-relative destination",
			args:  []string{"program", "show", "technical-design"},
			label: "output",
			kind:  destination,
		},
		{
			name:  "trace artifact path is absolute and exists",
			args:  []string{"program", "trace", "technical-design", "business-requirements"},
			label: "", // trace heads with "<id> → <absolute artifact>"
			kind:  mustBeAbs,
			setup: func(t *testing.T, ws string) {
				writeArtifact(t, ws, "docs/technical-design/requirements/business-requirements.md",
					"---\nsources: []\n---\n# BRD\n")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := withProgramApp(t)
			scaffoldProgramInto(t, "technical-design")
			if tc.setup != nil {
				tc.setup(t, tmp)
			}

			out := captureStdout(t, func() {
				if err := runADB(t, tc.args...); err != nil {
					t.Fatalf("%v: %v", tc.args, err)
				}
			})

			var vals []string
			if tc.label != "" {
				vals = fieldValues(out, tc.label)
			} else {
				// The trace heading: "<template-id> → <path>".
				for _, line := range strings.Split(out, "\n") {
					if _, p, found := strings.Cut(line, " → "); found {
						vals = append(vals, strings.TrimSpace(p))
					}
				}
			}
			if len(vals) == 0 {
				t.Fatalf("no %q values in output:\n%s", tc.label, out)
			}

			for _, v := range vals {
				switch tc.kind {
				case mustResolve:
					if filepath.IsAbs(v) {
						t.Errorf("%q should be workspace-relative, got an absolute path", v)
						continue
					}
					if _, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(v))); err != nil {
						t.Errorf("%q does not resolve from the workspace root: %v", v, err)
					}
				case mustBeAbs:
					if !filepath.IsAbs(v) {
						t.Errorf("%q should be an absolute path", v)
						continue
					}
					if _, err := os.Stat(v); err != nil {
						t.Errorf("absolute path %q does not exist: %v", v, err)
					}
				case destination:
					if filepath.IsAbs(v) {
						t.Errorf("destination %q should be workspace-relative, got an absolute path", v)
					}
					// A destination need not exist — but it must not escape the
					// workspace when joined, or a printed path would be unusable.
					resolved := filepath.Clean(filepath.Join(tmp, filepath.FromSlash(v)))
					if !strings.HasPrefix(resolved, filepath.Clean(tmp)+string(os.PathSeparator)) {
						t.Errorf("destination %q escapes the workspace root when resolved (%s)", v, resolved)
					}
				}
			}
		})
	}
}

// TestProgramCLI_StatusBlockedByPathsAreWorkspaceRelative asserts the `needs:`
// annotation on a blocked template names the file it waits on as a
// workspace-relative path — the same path the gate stats, so a user can create
// exactly that file and watch the template unblock.
func TestProgramCLI_StatusBlockedByPathsAreWorkspaceRelative(t *testing.T) {
	tmp := withProgramApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "program", "status", "technical-design"); err != nil {
			t.Fatalf("program status: %v", err)
		}
	})

	var checked int
	for _, line := range strings.Split(out, "\n") {
		_, needs, found := strings.Cut(line, "(needs: ")
		if !found {
			continue
		}
		needs = strings.TrimSuffix(strings.TrimSpace(needs), ")")
		for _, entry := range strings.Split(needs, ", ") {
			// Each entry reads "<template-id> → <output path>".
			_, outputPath, ok := strings.Cut(entry, " → ")
			if !ok {
				t.Errorf("needs entry %q should read \"<id> → <output>\"", entry)
				continue
			}
			outputPath = strings.TrimSpace(outputPath)
			if filepath.IsAbs(outputPath) {
				t.Errorf("needs path %q should be workspace-relative", outputPath)
				continue
			}
			checked++
			// Creating exactly that file must flip the gate — the path the CLI
			// prints and the path the gate stats have to be the same one.
			writeArtifact(t, tmp, outputPath, "# generated\n")
		}
	}
	if checked == 0 {
		t.Fatalf("no blocked template printed a needs: annotation:\n%s", out)
	}

	after := programStatusJSON(t, "technical-design")
	for _, row := range after {
		if row.State == "blocked" {
			t.Errorf("template %s is still blocked after creating every path the CLI named: %v",
				row.ID, row.BlockedBy)
		}
	}
}

// TestProgramCLI_OutputPathJSONMatchesTheGate asserts the `output_path` a scripted
// consumer reads is the absolute path the gate actually stats, and that `output`
// is its workspace-relative form. A consumer that writes to `output_path` must be
// able to expect the next `status` call to report it generated.
func TestProgramCLI_OutputPathJSONMatchesTheGate(t *testing.T) {
	tmp := withProgramApp(t)

	rows := programStatusJSON(t, "technical-design")
	if len(rows) == 0 {
		t.Fatal("no status rows")
	}
	for _, row := range rows {
		if !filepath.IsAbs(row.OutputPath) {
			t.Errorf("%s: output_path %q should be absolute", row.ID, row.OutputPath)
		}
		if filepath.IsAbs(row.Output) {
			t.Errorf("%s: output %q should be workspace-relative", row.ID, row.Output)
		}
		if want := filepath.Join(tmp, filepath.FromSlash(row.Output)); row.OutputPath != want {
			t.Errorf("%s: output_path = %q, want workspace root joined with output (%q)", row.ID, row.OutputPath, want)
		}
	}

	// Write to the advertised output_path of a ready template → it reports generated.
	var target string
	for _, row := range rows {
		if row.State == "ready" {
			target = row.OutputPath
			break
		}
	}
	if target == "" {
		t.Fatal("no ready template to write to")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("# written to the advertised output_path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, row := range programStatusJSON(t, "technical-design") {
		if row.OutputPath == target && row.State != "generated" {
			t.Errorf("writing to the advertised output_path left the template %q, want generated", row.State)
		}
	}
}
