package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file is the deliberate STATE-SPACE table for the presence-based gate:
//
//	{generated, ready, blocked, pending}
//	  × trigger    {phase, manual, event}
//	  × mode       {all, any}
//	  × |requires| {0, 1, many}
//
// program_test.go covers the same engine case-by-case; this walks the product of
// the dimensions instead, so a classification rule that regresses for one
// combination fails with the combination named. The builder below keeps each case
// to the shape it is actually about (ids, triggers, edges) rather than a manifest.

// gateTemplate is a compact template spec for the gate table: everything a case
// needs to say, and nothing else. Output is derived as docs/<id>.md.
type gateTemplate struct {
	id       string
	phase    string
	trigger  models.ProgramTrigger
	requires []string
	mode     models.RequiresMode
}

// gateOutput is the workspace-relative artifact path a gateTemplate id produces.
// One rule, used by both the builder and the expectation helpers, so a case can
// never disagree with the program it built.
func gateOutput(id string) string { return "docs/" + id + ".md" }

// buildGateProgram assembles a VALID program from phase ids and template specs.
// It validates on the way out: a table case that describes a malformed manifest
// is a bug in the case, and should fail as one rather than exercising the gate on
// a program LoadProgram would have rejected.
func buildGateProgram(t *testing.T, phases []string, specs ...gateTemplate) *models.Program {
	t.Helper()
	p := &models.Program{
		Program: models.ProgramMeta{ID: "gate", Name: "Gate", Lineage: []string{"a public source"}},
	}
	for _, id := range phases {
		p.Phases = append(p.Phases, models.ProgramPhase{ID: id, Name: strings.ToUpper(id)})
	}
	for _, s := range specs {
		phase := s.phase
		if phase == "" {
			phase = phases[0]
		}
		p.Templates = append(p.Templates, models.ProgramTemplate{
			ID:           s.id,
			Name:         strings.ToUpper(s.id),
			Phase:        phase,
			Path:         "templates/" + s.id + ".md",
			Output:       gateOutput(s.id),
			Requires:     s.requires,
			RequiresMode: s.mode,
			Trigger:      s.trigger,
			HumanReview:  &models.HumanReview{Required: true, Note: "check " + s.id, Risk: "skipping " + s.id + " costs"},
		})
	}
	if err := ValidateProgram(p); err != nil {
		t.Fatalf("table case built an invalid program: %v", err)
	}
	return p
}

// TestProgramStatus_GateMatrix walks the gate's state space. Each case names the
// templates it builds, which artifacts exist on disk, and the state every
// template must land in — so a failure identifies the exact combination.
func TestProgramStatus_GateMatrix(t *testing.T) {
	tests := []struct {
		name string
		// phases in manifest order; defaults to a single "one" phase.
		phases []string
		specs  []gateTemplate
		// generated lists template ids whose output artifact is written first.
		generated []string
		// want maps every template id to its expected status. Every template in
		// the program must appear, so a case cannot silently ignore one.
		want map[string]models.TemplateStatus
		// wantBlockedBy maps a template id to the requirement ids its BlockedBy
		// must name (order-independent). Absent ⇒ BlockedBy must be empty.
		wantBlockedBy map[string][]string
	}{
		// --- |requires| = 0, across all three triggers ----------------------
		{
			name:  "no requires, phase trigger, is ready",
			specs: []gateTemplate{{id: "a"}},
			want:  map[string]models.TemplateStatus{"a": models.TemplateReady},
		},
		{
			name:  "no requires, manual trigger, is pending not ready",
			specs: []gateTemplate{{id: "a", trigger: models.TriggerManual}},
			want:  map[string]models.TemplateStatus{"a": models.TemplatePending},
		},
		{
			name:  "no requires, event trigger, is pending not ready",
			specs: []gateTemplate{{id: "a", trigger: models.TriggerEvent}},
			want:  map[string]models.TemplateStatus{"a": models.TemplatePending},
		},
		{
			name:      "no requires, phase trigger, generated once its output exists",
			specs:     []gateTemplate{{id: "a"}},
			generated: []string{"a"},
			want:      map[string]models.TemplateStatus{"a": models.TemplateGenerated},
		},

		// --- generated beats every trigger ----------------------------------
		// Presence is checked FIRST, so an existing artifact reports generated
		// even for a template that would otherwise be pending forever.
		{
			name:      "generated beats manual trigger",
			specs:     []gateTemplate{{id: "a", trigger: models.TriggerManual}},
			generated: []string{"a"},
			want:      map[string]models.TemplateStatus{"a": models.TemplateGenerated},
		},
		{
			name:      "generated beats event trigger",
			specs:     []gateTemplate{{id: "a", trigger: models.TriggerEvent}},
			generated: []string{"a"},
			want:      map[string]models.TemplateStatus{"a": models.TemplateGenerated},
		},
		{
			name: "generated beats unsatisfied requires",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}},
			},
			generated: []string{"b"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady,
				"b": models.TemplateGenerated,
			},
		},

		// --- |requires| = 1, mode all --------------------------------------
		{
			name: "one requires all, unsatisfied, blocked naming it",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}, mode: models.RequiresAll},
			},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady,
				"b": models.TemplateBlocked,
			},
			wantBlockedBy: map[string][]string{"b": {"a"}},
		},
		{
			name: "one requires all, satisfied, ready",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}, mode: models.RequiresAll},
			},
			generated: []string{"a"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateGenerated,
				"b": models.TemplateReady,
			},
		},
		{
			name: "one requires, satisfied, but manual trigger stays pending",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}, trigger: models.TriggerManual},
			},
			generated: []string{"a"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateGenerated,
				"b": models.TemplatePending,
			},
		},
		{
			name: "one requires, unsatisfied, event trigger is pending not blocked",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}, trigger: models.TriggerEvent},
			},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady,
				"b": models.TemplatePending,
			},
		},

		// --- |requires| = many, mode all -----------------------------------
		{
			name: "three requires all, none satisfied, blocked_by names every one",
			specs: []gateTemplate{
				{id: "a"}, {id: "b"}, {id: "c"},
				{id: "d", requires: []string{"a", "b", "c"}, mode: models.RequiresAll},
			},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady, "b": models.TemplateReady, "c": models.TemplateReady,
				"d": models.TemplateBlocked,
			},
			wantBlockedBy: map[string][]string{"d": {"a", "b", "c"}},
		},
		{
			name: "three requires all, two satisfied, still blocked on the third only",
			specs: []gateTemplate{
				{id: "a"}, {id: "b"}, {id: "c"},
				{id: "d", requires: []string{"a", "b", "c"}, mode: models.RequiresAll},
			},
			generated: []string{"a", "b"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateGenerated, "b": models.TemplateGenerated,
				"c": models.TemplateReady, "d": models.TemplateBlocked,
			},
			wantBlockedBy: map[string][]string{"d": {"c"}},
		},
		{
			name: "three requires all, every one satisfied, ready",
			specs: []gateTemplate{
				{id: "a"}, {id: "b"}, {id: "c"},
				{id: "d", requires: []string{"a", "b", "c"}, mode: models.RequiresAll},
			},
			generated: []string{"a", "b", "c"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateGenerated, "b": models.TemplateGenerated,
				"c": models.TemplateGenerated, "d": models.TemplateReady,
			},
		},

		// --- |requires| = many, mode any -----------------------------------
		{
			name: "three requires any, none satisfied, blocked_by still names every one",
			specs: []gateTemplate{
				{id: "a"}, {id: "b"}, {id: "c"},
				{id: "d", requires: []string{"a", "b", "c"}, mode: models.RequiresAny},
			},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady, "b": models.TemplateReady, "c": models.TemplateReady,
				"d": models.TemplateBlocked,
			},
			// Under `any` a caller needs the full choice of upstreams to produce,
			// so every missing requirement is reported even though one would do.
			wantBlockedBy: map[string][]string{"d": {"a", "b", "c"}},
		},
		{
			name: "three requires any, exactly one satisfied, ready",
			specs: []gateTemplate{
				{id: "a"}, {id: "b"}, {id: "c"},
				{id: "d", requires: []string{"a", "b", "c"}, mode: models.RequiresAny},
			},
			generated: []string{"b"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady, "b": models.TemplateGenerated,
				"c": models.TemplateReady, "d": models.TemplateReady,
			},
		},
		{
			name: "one requires any behaves like all",
			specs: []gateTemplate{
				{id: "a"},
				{id: "b", requires: []string{"a"}, mode: models.RequiresAny},
			},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady,
				"b": models.TemplateBlocked,
			},
			wantBlockedBy: map[string][]string{"b": {"a"}},
		},
		{
			name: "empty requires with mode any is satisfied, not blocked",
			specs: []gateTemplate{
				{id: "a", requires: nil, mode: models.RequiresAny},
			},
			want: map[string]models.TemplateStatus{"a": models.TemplateReady},
		},

		// --- transitive chains ----------------------------------------------
		{
			name:   "chain unblocks one hop at a time",
			phases: []string{"one", "two", "three"},
			specs: []gateTemplate{
				{id: "a", phase: "one"},
				{id: "b", phase: "two", requires: []string{"a"}},
				{id: "c", phase: "three", requires: []string{"b"}},
			},
			generated: []string{"a"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateGenerated,
				"b": models.TemplateReady,
				// c stays blocked: satisfying a does NOT transitively satisfy b.
				"c": models.TemplateBlocked,
			},
			wantBlockedBy: map[string][]string{"c": {"b"}},
		},
		{
			name:   "a generated middle unblocks the tail even with the head missing",
			phases: []string{"one", "two", "three"},
			specs: []gateTemplate{
				{id: "a", phase: "one"},
				{id: "b", phase: "two", requires: []string{"a"}},
				{id: "c", phase: "three", requires: []string{"b"}},
			},
			generated: []string{"b"},
			want: map[string]models.TemplateStatus{
				"a": models.TemplateReady,
				"b": models.TemplateGenerated,
				"c": models.TemplateReady,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			phases := tc.phases
			if len(phases) == 0 {
				phases = []string{"one"}
			}
			p := buildGateProgram(t, phases, tc.specs...)
			ws := t.TempDir()
			for _, id := range tc.generated {
				writeArtifact(t, ws, gateOutput(id), "# "+id+"\n")
			}

			states, err := ProgramStatus(p, ws)
			if err != nil {
				t.Fatalf("ProgramStatus: %v", err)
			}
			if len(states) != len(tc.want) {
				t.Fatalf("got %d states, want an expectation for each of %d templates", len(states), len(tc.want))
			}

			for _, st := range states {
				id := st.Template.ID
				want, ok := tc.want[id]
				if !ok {
					t.Errorf("template %q has no expected status in the case", id)
					continue
				}
				if st.Status != want {
					t.Errorf("template %q = %q, want %q (detail: %s)", id, st.Status, want, st.Detail)
				}
				assertBlockedBy(t, st, tc.wantBlockedBy[id])
			}

			// `next` is by definition the ready subset, in the same order — assert
			// that here too so the two views can never drift apart.
			next, err := NextTemplates(p, ws)
			if err != nil {
				t.Fatalf("NextTemplates: %v", err)
			}
			var wantReady, gotReady []string
			for _, st := range states {
				if st.Status == models.TemplateReady {
					wantReady = append(wantReady, st.Template.ID)
				}
			}
			for _, st := range next {
				gotReady = append(gotReady, st.Template.ID)
			}
			if strings.Join(gotReady, ",") != strings.Join(wantReady, ",") {
				t.Errorf("NextTemplates = %v, want the ready subset in status order %v", gotReady, wantReady)
			}
		})
	}
}

// assertBlockedBy checks a state's BlockedBy names exactly wantIDs (in any
// order) and that each entry carries BOTH the requirement id and the output path
// it is waiting for — the property that makes the message actionable.
func assertBlockedBy(t *testing.T, st TemplateState, wantIDs []string) {
	t.Helper()
	if len(st.BlockedBy) != len(wantIDs) {
		t.Errorf("template %q BlockedBy = %v, want %d entry/entries for %v",
			st.Template.ID, st.BlockedBy, len(wantIDs), wantIDs)
		return
	}
	for _, id := range wantIDs {
		want := fmt.Sprintf("%s (%s)", id, gateOutput(id))
		var found bool
		for _, entry := range st.BlockedBy {
			if entry == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("template %q BlockedBy = %v, want an entry %q (the id AND the file it waits on)",
				st.Template.ID, st.BlockedBy, want)
		}
	}
}

// TestProgramStatus_Ordering pins the two-level ordering contract: phase order is
// MANIFEST order, and within a phase templates keep manifest order. Both matter —
// the status view and `next` are read top-to-bottom as a work queue.
func TestProgramStatus_Ordering(t *testing.T) {
	tests := []struct {
		name   string
		phases []string
		specs  []gateTemplate
		want   string // comma-joined expected id order
	}{
		{
			name:   "manifest order within one phase is preserved",
			phases: []string{"one"},
			specs:  []gateTemplate{{id: "c"}, {id: "a"}, {id: "b"}},
			want:   "c,a,b",
		},
		{
			name:   "phase order wins over manifest order",
			phases: []string{"first", "second"},
			specs: []gateTemplate{
				{id: "late", phase: "second"},
				{id: "early", phase: "first"},
			},
			want: "early,late",
		},
		{
			name:   "phase order is manifest order, not alphabetical",
			phases: []string{"zebra", "alpha"},
			specs: []gateTemplate{
				{id: "in-alpha", phase: "alpha"},
				{id: "in-zebra", phase: "zebra"},
			},
			want: "in-zebra,in-alpha",
		},
		{
			name:   "stable within a phase across three phases",
			phases: []string{"one", "two", "three"},
			specs: []gateTemplate{
				{id: "t1", phase: "three"},
				{id: "o1", phase: "one"},
				{id: "t2", phase: "three"},
				{id: "w1", phase: "two"},
				{id: "o2", phase: "one"},
			},
			want: "o1,o2,w1,t1,t2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := buildGateProgram(t, tc.phases, tc.specs...)
			states, err := ProgramStatus(p, t.TempDir())
			if err != nil {
				t.Fatalf("ProgramStatus: %v", err)
			}
			var ids []string
			for _, st := range states {
				ids = append(ids, st.Template.ID)
			}
			if got := strings.Join(ids, ","); got != tc.want {
				t.Errorf("order = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestProgramStatus_PhaseMetadataResolves asserts every state carries the phase
// RECORD (not just the id), because the CLI prints the phase name — a template
// whose phase failed to resolve would print an empty heading rather than error.
func TestProgramStatus_PhaseMetadataResolves(t *testing.T) {
	p := buildGateProgram(t, []string{"one", "two"},
		gateTemplate{id: "a", phase: "one"},
		gateTemplate{id: "b", phase: "two"},
	)
	states, err := ProgramStatus(p, t.TempDir())
	if err != nil {
		t.Fatalf("ProgramStatus: %v", err)
	}
	for _, st := range states {
		if st.Phase.ID != st.Template.Phase {
			t.Errorf("template %q resolved phase %q, want %q", st.Template.ID, st.Phase.ID, st.Template.Phase)
		}
		if st.Phase.Name == "" {
			t.Errorf("template %q resolved an empty phase name", st.Template.ID)
		}
	}
}

// TestArtifactExists_PresenceSemantics pins what "generated" means. Presence is
// the WHOLE gate (a considered trade-off, L600 §5), so the edges of "does this
// file exist" are the edges of the gate itself.
func TestArtifactExists_PresenceSemantics(t *testing.T) {
	t.Run("an empty file counts as generated", func(t *testing.T) {
		// Documented limitation, asserted deliberately: a scaffolded-but-unfilled
		// artifact unblocks everything downstream. If this ever changes it should
		// change with a failing test, not silently.
		p := buildGateProgram(t, []string{"one"}, gateTemplate{id: "a"})
		ws := t.TempDir()
		writeArtifact(t, ws, gateOutput("a"), "")

		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if states[0].Status != models.TemplateGenerated {
			t.Errorf("an empty artifact = %q, want generated (presence is the whole gate)", states[0].Status)
		}
	})

	t.Run("a directory at the output path does NOT count as generated", func(t *testing.T) {
		p := buildGateProgram(t, []string{"one"}, gateTemplate{id: "a"})
		ws := t.TempDir()
		if err := os.MkdirAll(filepath.Join(ws, filepath.FromSlash(gateOutput("a"))), 0o755); err != nil {
			t.Fatal(err)
		}
		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		if states[0].Status != models.TemplateReady {
			t.Errorf("a directory at the output path = %q, want ready (only a regular file is an artifact)", states[0].Status)
		}
	})

	t.Run("output path resolution uses OS separators", func(t *testing.T) {
		// The engine deliberately splits `path` (embed FS, forward slash) from
		// `filepath` (on disk). A manifest output is always forward-slash, so the
		// resolved OutputPath must carry the platform separator or the stat would
		// miss the file on Windows.
		p := buildGateProgram(t, []string{"one"}, gateTemplate{id: "a"})
		ws := t.TempDir()
		states, err := ProgramStatus(p, ws)
		if err != nil {
			t.Fatalf("ProgramStatus: %v", err)
		}
		want := filepath.Join(ws, "docs", "a.md")
		if states[0].OutputPath != want {
			t.Errorf("OutputPath = %q, want %q", states[0].OutputPath, want)
		}
		if runtime.GOOS == "windows" && strings.Contains(states[0].OutputPath, "/") {
			t.Errorf("OutputPath %q kept a forward slash on Windows", states[0].OutputPath)
		}
	})
}
