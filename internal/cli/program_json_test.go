package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// This file pins the `--json` CONTRACT of every `adb program` subcommand: the
// top-level shape (array vs object), the required keys, and each key's type. A
// scripted consumer reads these shapes, so a field rename is a breaking change
// and should fail here rather than in someone's pipeline.
//
// It also asserts the shapes are INTERNALLY CONSISTENT across the subcommands.
// `adb` elsewhere ships two different `--json` conventions (`events query` is a
// single array, `events tail` is JSONL) — the program commands are all single
// JSON values, and that is worth holding still.

// jsonKind is the expected top-level JSON shape of a subcommand's --json output.
type jsonKind int

const (
	jsonArray  jsonKind = iota // a single JSON array
	jsonObject                 // a single JSON object
)

// fieldSpec is one required key and the JSON type it must carry.
type fieldSpec struct {
	key string
	// typ is the reflect.Kind of the decoded value: string, float64 (any JSON
	// number), bool, slice, or map.
	typ reflect.Kind
}

// TestProgramCLI_JSONContract walks every subcommand's --json output and checks
// the shape and required keys. Each case names the subcommand, so a failure says
// exactly which contract moved.
func TestProgramCLI_JSONContract(t *testing.T) {
	tests := []struct {
		name string
		args []string
		kind jsonKind
		// required are the keys every element (array) or the object itself must
		// carry, with their types.
		required []fieldSpec
		// setup runs before the command.
		setup func(t *testing.T, ws string)
	}{
		{
			name: "list rows",
			args: []string{"program", "list", "--json"},
			kind: jsonArray,
			required: []fieldSpec{
				{"id", reflect.String},
				{"phases", reflect.Float64},
				{"templates", reflect.Float64},
				{"source", reflect.String},
			},
		},
		{
			name: "show manifest",
			args: []string{"program", "show", "technical-design", "--json"},
			kind: jsonObject,
			required: []fieldSpec{
				{"program", reflect.Map},
				{"phases", reflect.Slice},
				{"templates", reflect.Slice},
			},
		},
		{
			name: "status rows",
			args: []string{"program", "status", "technical-design", "--json"},
			kind: jsonArray,
			// The template_* trio is the read-side mirror of output/output_path: where
			// to READ the template, as opposed to where to WRITE the artifact. Before
			// they existed the row carried no template location at all, so the resolved
			// `from:` fix was invisible to --json — the shape an agent reads.
			required: []fieldSpec{
				{"id", reflect.String},
				{"name", reflect.String},
				{"phase", reflect.String},
				{"state", reflect.String},
				{"output", reflect.String},
				{"output_path", reflect.String},
				{"template_path", reflect.String},
				{"template_from", reflect.String},
				{"template_source", reflect.String},
				{"trigger", reflect.String},
			},
		},
		{
			name: "next rows share the status row schema",
			args: []string{"program", "next", "technical-design", "--json"},
			kind: jsonArray,
			required: []fieldSpec{
				{"id", reflect.String},
				{"name", reflect.String},
				{"phase", reflect.String},
				{"state", reflect.String},
				{"output", reflect.String},
				{"output_path", reflect.String},
				{"template_path", reflect.String},
				{"template_from", reflect.String},
				{"template_source", reflect.String},
				{"trigger", reflect.String},
			},
		},
		{
			name: "trace report",
			args: []string{"program", "trace", "technical-design", "business-requirements", "--json"},
			kind: jsonObject,
			required: []fieldSpec{
				{"template_id", reflect.String},
				{"artifact", reflect.String},
			},
			setup: func(t *testing.T, ws string) {
				writeArtifact(t, ws, "docs/technical-design/requirements/business-requirements.md",
					"---\nsources:\n  - project-doc/vision.md\n---\n# BRD\n")
			},
		},
		{
			name: "scaffold entries",
			args: []string{"program", "scaffold", "change-adoption", "--json", "--dry-run"},
			kind: jsonArray,
			// snake_case like every other shape here, plus dry_run — the marker that
			// lets a consumer tell a planned install from a real one, since both
			// report action "installed".
			required: []fieldSpec{
				{"name", reflect.String},
				{"dest", reflect.String},
				{"action", reflect.String},
				{"dry_run", reflect.Bool},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := withProgramApp(t)
			if tc.setup != nil {
				tc.setup(t, ws)
			}

			out := captureStdout(t, func() {
				if err := runADB(t, tc.args...); err != nil {
					t.Fatalf("%v: %v", tc.args, err)
				}
			})

			// Every program --json output is ONE JSON value, never JSONL.
			assertSingleJSONValue(t, out)

			switch tc.kind {
			case jsonArray:
				var rows []map[string]any
				if err := json.Unmarshal([]byte(out), &rows); err != nil {
					t.Fatalf("not a JSON array: %v\n%s", err, out)
				}
				if len(rows) == 0 {
					t.Fatalf("expected at least one row:\n%s", out)
				}
				for i, row := range rows {
					assertFields(t, tc.required, row, tc.name+" row "+strconv.Itoa(i))
				}
			case jsonObject:
				var obj map[string]any
				if err := json.Unmarshal([]byte(out), &obj); err != nil {
					t.Fatalf("not a JSON object: %v\n%s", err, out)
				}
				assertFields(t, tc.required, obj, tc.name)
			}
		})
	}
}

// assertFields checks each required key is present with the expected JSON type.
func assertFields(t *testing.T, required []fieldSpec, obj map[string]any, where string) {
	t.Helper()
	for _, f := range required {
		v, ok := obj[f.key]
		if !ok {
			t.Errorf("%s: missing required key %q (keys: %v)", where, f.key, sortedKeys(obj))
			continue
		}
		if v == nil {
			t.Errorf("%s: key %q is null, want %s", where, f.key, f.typ)
			continue
		}
		if got := reflect.ValueOf(v).Kind(); got != f.typ {
			t.Errorf("%s: key %q has type %s, want %s", where, f.key, got, f.typ)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// assertSingleJSONValue proves the output is one JSON value rather than JSONL —
// the distinction that bites consumers of `adb events tail --json`.
func assertSingleJSONValue(t *testing.T, out string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var first any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("output is not decodable JSON: %v\n%s", err, out)
	}
	var second any
	if err := dec.Decode(&second); err == nil {
		t.Errorf("output decoded a SECOND JSON value — this shape is JSONL, not a single value:\n%s", out)
	}
}

// TestProgramCLI_StatusAndNextRowsAreTheSameSchema asserts `next` is a strict
// subset projection of `status`: the same key set, so a consumer can read either
// with one struct. `next` being the ready subset of `status` is the documented
// contract, and a shared row schema is what makes that usable.
func TestProgramCLI_StatusAndNextRowsAreTheSameSchema(t *testing.T) {
	withProgramApp(t)

	statusRows := rawJSONRows(t, "program", "status", "technical-design", "--json")
	nextRows := rawJSONRows(t, "program", "next", "technical-design", "--json")
	if len(nextRows) == 0 {
		t.Fatal("next returned no rows in a fresh workspace")
	}

	// Find the status row for each next row and compare key sets and values.
	for _, nextRow := range nextRows {
		id, _ := nextRow["id"].(string)
		var match map[string]any
		for _, s := range statusRows {
			if s["id"] == id {
				match = s
				break
			}
		}
		if match == nil {
			t.Errorf("next row %q has no corresponding status row", id)
			continue
		}
		if got, want := sortedKeys(nextRow), sortedKeys(match); !reflect.DeepEqual(got, want) {
			t.Errorf("row %q: next keys %v differ from status keys %v", id, got, want)
		}
		if !reflect.DeepEqual(nextRow, match) {
			t.Errorf("row %q: next and status disagree\nnext:   %v\nstatus: %v", id, nextRow, match)
		}
		if nextRow["state"] != "ready" {
			t.Errorf("next row %q has state %v, want ready", id, nextRow["state"])
		}
	}

	// The template location has to be in BOTH shapes. An equal-key-set assertion on
	// its own would stay green if `status` and `next` dropped it together, which is
	// exactly the state the surface was in: `next` printed a resolved `from:` while
	// neither --json row carried a template location at all.
	for _, key := range []string{"template_path", "template_from", "template_source"} {
		for what, rows := range map[string][]map[string]any{"status": statusRows, "next": nextRows} {
			if v, ok := rows[0][key].(string); !ok || v == "" {
				t.Errorf("%s rows must carry a non-empty %q; got %v (keys: %v)",
					what, key, rows[0][key], sortedKeys(rows[0]))
			}
		}
	}
}

// TestProgramCLI_JSONKeyCasingIsConsistent asserts ONE naming convention across
// the whole `adb program --json` surface: snake_case, in every subcommand, at
// every nesting depth.
//
// It has teeth because the surface did not always have that property. `scaffold`
// used to marshal core.PackScaffoldEntry directly, and that type declared no json
// tags, so encoding/json fell back to Go field names and emitted PascalCase
// `Name`/`Dest`/`Action` — a consumer needed two conventions to read one command
// group. The type carries tags now, and this is what keeps it that way.
func TestProgramCLI_JSONKeyCasingIsConsistent(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		setup func(t *testing.T, ws string)
	}{
		{name: "list", args: []string{"program", "list", "--json"}},
		{name: "show", args: []string{"program", "show", "technical-design", "--json"}},
		{name: "status", args: []string{"program", "status", "technical-design", "--json"}},
		{name: "next", args: []string{"program", "next", "technical-design", "--json"}},
		{
			name: "trace",
			args: []string{"program", "trace", "technical-design", "business-requirements", "--json"},
			setup: func(t *testing.T, ws string) {
				writeArtifact(t, ws, "docs/technical-design/requirements/business-requirements.md",
					"---\nsources:\n  - path: project-doc/vision.md\n    note: framing\n---\n# BRD\n")
			},
		},
		{name: "scaffold", args: []string{"program", "scaffold", "change-adoption", "--json", "--dry-run"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := withProgramApp(t)
			if tc.setup != nil {
				tc.setup(t, ws)
			}
			out := captureStdout(t, func() {
				if err := runADB(t, tc.args...); err != nil {
					t.Fatalf("%v: %v", tc.args, err)
				}
			})
			var decoded any
			if err := json.Unmarshal([]byte(out), &decoded); err != nil {
				t.Fatalf("%v: not decodable JSON: %v\n%s", tc.args, err, out)
			}
			keys := jsonKeys(decoded)
			if len(keys) == 0 {
				t.Fatalf("%s emitted no object keys at all, so the casing rule was not "+
					"actually exercised:\n%s", tc.name, out)
			}
			for _, k := range keys {
				if !snakeCaseKey.MatchString(k) {
					t.Errorf("%s: key %q is not snake_case — one command group must not "+
						"need two naming conventions", tc.name, k)
				}
			}
		})
	}
}

// snakeCaseKey is the shape every program --json key must have: lowercase, digits,
// and underscores only, starting with a letter.
var snakeCaseKey = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// jsonKeys collects every object key in a decoded JSON value, at every depth, so
// the casing rule covers nested shapes (show's manifest, status's human_review)
// and not just the top level.
func jsonKeys(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			out = append(out, k)
			out = append(out, jsonKeys(child)...)
		}
	case []any:
		for _, child := range t {
			out = append(out, jsonKeys(child)...)
		}
	}
	sort.Strings(out)
	return out
}

// TestProgramCLI_ScaffoldJSONDryRunMarker asserts a scripted consumer can tell a
// PLAN from a write. `--dry-run` rows report action "installed" exactly like a real
// install does — only the human view says "Would scaffold" — so the JSON needs its
// own marker or a consumer cannot distinguish the two at all.
func TestProgramCLI_ScaffoldJSONDryRunMarker(t *testing.T) {
	const pack = "change-adoption"

	tests := []struct {
		name string
		// before are commands run first, to set up the on-disk state.
		before [][]string
		// mutate runs after before, to dirty what was installed. It must touch EVERY
		// file in the pack, because each case asserts one action across all rows.
		mutate func(t *testing.T, ws string)
		args   []string
		// wantDryRun is the dry_run marker every row must carry.
		wantDryRun bool
		// wantAction is the action every row must report.
		wantAction string
		// wantOnDisk says whether the pack must exist after the command.
		wantOnDisk bool
	}{
		{
			name:       "--dry-run marks the rows as a plan and writes nothing",
			args:       []string{"program", "scaffold", pack, "--json", "--dry-run"},
			wantDryRun: true,
			wantAction: "installed",
			wantOnDisk: false,
		},
		{
			name:       "a real install is not marked dry-run",
			args:       []string{"program", "scaffold", pack, "--json"},
			wantDryRun: false,
			wantAction: "installed",
			wantOnDisk: true,
		},
		{
			name:       "a re-install is unchanged and still not marked dry-run",
			before:     [][]string{{"program", "scaffold", pack}},
			args:       []string{"program", "scaffold", pack, "--json"},
			wantDryRun: false,
			wantAction: "unchanged",
			wantOnDisk: true,
		},
		{
			name:       "a dry run over an installed pack reports unchanged AND dry_run",
			before:     [][]string{{"program", "scaffold", pack}},
			args:       []string{"program", "scaffold", pack, "--json", "--dry-run"},
			wantDryRun: true,
			wantAction: "unchanged",
			wantOnDisk: true,
		},
		{
			// The third and last HarnessInstallAction: a destination that exists and
			// DIFFERS is skipped rather than clobbered, and only --force overrides it.
			// Without this row the JSON contract only ever saw installed/unchanged, so
			// "skipped" could have been renamed or mis-tagged unnoticed — and it is the
			// one action that means "your edit was preserved", the clobber-safety
			// promise a scripted consumer has to be able to read.
			name:       "a locally-edited file is skipped, not clobbered",
			before:     [][]string{{"program", "scaffold", pack}},
			mutate:     editScaffoldedPack,
			args:       []string{"program", "scaffold", pack, "--json"},
			wantDryRun: false,
			wantAction: "skipped",
			wantOnDisk: true,
		},
		{
			name:       "a dry run over a locally-edited pack plans the skip, not a write",
			before:     [][]string{{"program", "scaffold", pack}},
			mutate:     editScaffoldedPack,
			args:       []string{"program", "scaffold", pack, "--json", "--dry-run"},
			wantDryRun: true,
			wantAction: "skipped",
			wantOnDisk: true,
		},
		{
			name:       "--force overwrites a locally-edited file and reports installed",
			before:     [][]string{{"program", "scaffold", pack}},
			mutate:     editScaffoldedPack,
			args:       []string{"program", "scaffold", pack, "--json", "--force"},
			wantDryRun: false,
			wantAction: "installed",
			wantOnDisk: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := withProgramApp(t)
			for _, pre := range tc.before {
				if err := runADB(t, pre...); err != nil {
					t.Fatalf("setup %v: %v", pre, err)
				}
			}
			if tc.mutate != nil {
				tc.mutate(t, ws)
			}

			rows := rawJSONRows(t, tc.args...)
			if len(rows) == 0 {
				t.Fatal("scaffold --json returned no rows")
			}
			for _, row := range rows {
				if got, ok := row["dry_run"].(bool); !ok || got != tc.wantDryRun {
					t.Errorf("row %v: dry_run = %v, want %v", row["name"], row["dry_run"], tc.wantDryRun)
				}
				if got := row["action"]; got != tc.wantAction {
					t.Errorf("row %v: action = %v, want %v", row["name"], got, tc.wantAction)
				}
			}

			_, err := os.Stat(filepath.Join(ws, "programs", pack, "program.yaml"))
			if tc.wantOnDisk && err != nil {
				t.Errorf("the pack should be on disk: %v", err)
			}
			if !tc.wantOnDisk && err == nil {
				t.Error("a dry run wrote the pack to disk")
			}
		})
	}
}

// editScaffoldedPack rewrites EVERY file under the workspace's programs/ tree so
// the next install sees a destination that exists and DIFFERS — the state that
// produces the `skipped` action. It walks the tree rather than naming a pack so it
// stays correct for whichever pack the case scaffolded, and every file is touched
// because each case asserts one action across all rows.
func editScaffoldedPack(t *testing.T, ws string) {
	t.Helper()
	var edited int
	err := filepath.WalkDir(filepath.Join(ws, "programs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if err := os.WriteFile(p, []byte("EDITED\n"), 0o644); err != nil {
			return err
		}
		edited++
		return nil
	})
	if err != nil {
		t.Fatalf("edit the scaffolded pack: %v", err)
	}
	if edited == 0 {
		t.Fatal("no scaffolded file was edited, so the skipped path was never set up")
	}
}

// rawJSONRows runs a subcommand and decodes its --json output as an array of
// untyped objects, so a test can inspect the raw key set rather than a struct
// that would silently tolerate a rename.
func rawJSONRows(t *testing.T, args ...string) []map[string]any {
	t.Helper()
	out := captureStdout(t, func() {
		if err := runADB(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	})
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("%v: not a JSON array: %v\n%s", args, err, out)
	}
	return rows
}

// TestProgramCLI_ShowJSONRoundTripsEveryShippedPack asserts `show --json` is a
// faithful, complete rendering of each shipped manifest — not just the one pack
// the older tests happen to cover.
func TestProgramCLI_ShowJSONRoundTripsEveryShippedPack(t *testing.T) {
	withProgramApp(t)

	for _, id := range []string{
		"product-discovery", "technical-design", "delivery-readiness",
		"operations", "change-adoption",
	} {
		t.Run(id, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := runADB(t, "program", "show", id, "--json"); err != nil {
					t.Fatalf("show %s --json: %v", id, err)
				}
			})
			var shown struct {
				Program struct {
					ID      string   `json:"id"`
					Name    string   `json:"name"`
					Lineage []string `json:"lineage"`
				} `json:"program"`
				Phases []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"phases"`
				Templates []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Phase       string `json:"phase"`
					Path        string `json:"path"`
					Output      string `json:"output"`
					HumanReview *struct {
						Required bool   `json:"required"`
						Note     string `json:"note"`
						Risk     string `json:"risk"`
					} `json:"human_review"`
				} `json:"templates"`
			}
			if err := json.Unmarshal([]byte(out), &shown); err != nil {
				t.Fatalf("show %s --json is not valid JSON: %v", id, err)
			}
			if shown.Program.ID != id {
				t.Errorf("program.id = %q, want %q", shown.Program.ID, id)
			}
			if shown.Program.Name == "" || len(shown.Program.Lineage) == 0 {
				t.Errorf("%s: identity is incomplete: %+v", id, shown.Program)
			}
			if len(shown.Phases) == 0 || len(shown.Templates) == 0 {
				t.Fatalf("%s: phases=%d templates=%d, want both non-zero", id, len(shown.Phases), len(shown.Templates))
			}
			phases := map[string]bool{}
			for _, ph := range shown.Phases {
				if ph.ID == "" || ph.Name == "" {
					t.Errorf("%s: phase %+v is incomplete", id, ph)
				}
				phases[ph.ID] = true
			}
			for _, tpl := range shown.Templates {
				if tpl.ID == "" || tpl.Name == "" || tpl.Path == "" || tpl.Output == "" {
					t.Errorf("%s: template %+v is incomplete", id, tpl)
				}
				if !phases[tpl.Phase] {
					t.Errorf("%s: template %q names phase %q which is not in phases", id, tpl.ID, tpl.Phase)
				}
				// human_review is mandatory on every template, and for a SHIPPED
				// pack both the note and the risk must be real (L600 §7).
				if tpl.HumanReview == nil {
					t.Errorf("%s: template %q has no human_review", id, tpl.ID)
					continue
				}
				if strings.TrimSpace(tpl.HumanReview.Note) == "" || strings.TrimSpace(tpl.HumanReview.Risk) == "" {
					t.Errorf("%s: template %q human_review needs both a note and a risk: %+v",
						id, tpl.ID, *tpl.HumanReview)
				}
			}
		})
	}
}
