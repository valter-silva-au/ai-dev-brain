package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/valter-silva-au/ai-dev-brain/internal"
	"github.com/valter-silva-au/ai-dev-brain/internal/observability"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

func TestBuildRuleFromFlags_TimeSkill(t *testing.T) {
	r, err := buildRuleFromFlags("nightly", "15m", "", "", "", "repos-pull", "", nil, nil, "", false)
	if err != nil {
		t.Fatalf("buildRuleFromFlags error = %v", err)
	}
	if r.Name != "nightly" || r.On.Schedule != "15m" || r.Run.Skill != "repos-pull" {
		t.Fatalf("unexpected rule: %+v", r)
	}
	if !r.IsEnabled() {
		t.Fatal("rule should be enabled by default")
	}
}

func TestBuildRuleFromFlags_EventConditionEdgeOutput(t *testing.T) {
	r, err := buildRuleFromFlags(
		"flag-blocked", "", "task.status_changed",
		"{{.task_id}}", "depends_on",
		"triage", "",
		[]string{"relates_to:INIT-1"}, []string{"reports/{{.task_id}}.md"},
		"{{.task_id}}", false,
	)
	if err != nil {
		t.Fatalf("buildRuleFromFlags error = %v", err)
	}
	if r.On.Event != "task.status_changed" {
		t.Fatalf("event = %q", r.On.Event)
	}
	if r.If == nil || r.If.HasEdge != models.EdgeDependsOn {
		t.Fatalf("condition = %+v", r.If)
	}
	if len(r.Write) != 2 {
		t.Fatalf("want 2 outputs, got %d", len(r.Write))
	}
	// StringArray order: artifacts appended before edges in buildRuleFromFlags.
	if r.Write[0].Artifact != "reports/{{.task_id}}.md" {
		t.Fatalf("output[0] = %+v", r.Write[0])
	}
	if r.Write[1].Edge == nil || r.Write[1].Edge.Type != models.EdgeRelatesTo || r.Write[1].Edge.Target != "INIT-1" {
		t.Fatalf("output[1] = %+v", r.Write[1])
	}
	if r.Write[1].EdgeFrom != "{{.task_id}}" {
		t.Fatalf("edge_from = %q", r.Write[1].EdgeFrom)
	}
}

func TestBuildRuleFromFlags_Disabled(t *testing.T) {
	r, err := buildRuleFromFlags("parked", "1h", "", "", "", "s", "", nil, nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.IsEnabled() {
		t.Fatal("--disabled should park the rule")
	}
}

func TestBuildRuleFromFlags_Rejections(t *testing.T) {
	cases := []struct {
		name                                                      string
		rname, every, event, ifEnt, ifEdge, skill, exec, edgeFrom string
		edges, arts                                               []string
	}{
		{name: "no trigger", rname: "r", skill: "s"},
		{name: "both triggers", rname: "r", every: "1m", event: "task.created", skill: "s"},
		{name: "no action", rname: "r", every: "1m"},
		{name: "both actions", rname: "r", every: "1m", skill: "s", exec: "echo hi"},
		{name: "bad edge spec", rname: "r", every: "1m", skill: "s", edges: []string{"noseparator"}},
		// `--write-edge` and `--if-edge` are the only public surface that lets a
		// human name an edge type, so they are where the closed #109 vocabulary
		// gets enforced — at authoring time, not at 3am when the rule fires.
		{name: "write-edge type not canonical", rname: "r", every: "1m", skill: "s", edges: []string{"mentions:TASK-1"}},
		{name: "write-edge type near-miss typo", rname: "r", every: "1m", skill: "s", edges: []string{"depends-on:TASK-1"}},
		{name: "if-edge type not canonical", rname: "r", every: "1m", skill: "s", ifEnt: "TASK-1", ifEdge: "mentions"},
		// `--on-event` is the same defect class one layer over: the trigger is
		// matched literally against the dispatched event name, so an unknown type
		// does not error at fire time — nothing ever dispatches that name and the
		// rule silently never fires. Gate it where the author can see it.
		{name: "on-event type unknown", rname: "r", event: "bogus.type", skill: "s"},
		{name: "on-event type near-miss typo", rname: "r", event: "task.status-changed", skill: "s"},
		{name: "on-event type wrong case", rname: "r", event: "Task.Status_Changed", skill: "s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildRuleFromFlags(tc.rname, tc.every, tc.event, tc.ifEnt, tc.ifEdge, tc.skill, tc.exec, tc.edges, tc.arts, tc.edgeFrom, false)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.name)
			}
		})
	}
}

func TestParseDataFlags(t *testing.T) {
	got, err := parseDataFlags([]string{"task_id=TASK-1", "note=has=equals"})
	if err != nil {
		t.Fatal(err)
	}
	if got["task_id"] != "TASK-1" {
		t.Fatalf("task_id = %q", got["task_id"])
	}
	if got["note"] != "has=equals" {
		t.Fatalf("note = %q (should split on first = only)", got["note"])
	}
	if _, err := parseDataFlags([]string{"noequals"}); err == nil {
		t.Fatal("expected error for a flag without =")
	}
	if got := mustNil(t, []string{}); got != nil {
		t.Fatalf("empty data should be nil, got %+v", got)
	}
}

func TestStringifyEventValue(t *testing.T) {
	cases := []struct {
		in   interface{}
		want string
	}{
		{"TASK-42", "TASK-42"},                    // strings pass through
		{float64(42), "42"},                       // whole JSON numbers: no ".0"
		{float64(42.5), "42.5"},                   // fractional numbers keep precision
		{float64(1e21), "1000000000000000000000"}, // no scientific notation
		{true, "true"},                            // other types fall back to %v
	}
	for _, c := range cases {
		if got := stringifyEventValue(c.in); got != c.want {
			t.Fatalf("stringifyEventValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func mustNil(t *testing.T, data []string) map[string]string {
	t.Helper()
	m, err := parseDataFlags(data)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestEdgeTypeHint_DerivedFromVocabulary keeps the flag help honest: it is
// rendered from models.CanonicalEdgeTypes, so the set `adb schedule add --help`
// advertises is the set EdgeType.Validate enforces. Hard-coding the list in the
// help string is how a sixth edge type ends up rejected by a flag that says it
// is allowed — or advertised by a flag that rejects it.
func TestEdgeTypeHint_DerivedFromVocabulary(t *testing.T) {
	got := edgeTypeHint()
	for _, et := range models.CanonicalEdgeTypes {
		if !strings.Contains(got, string(et)) {
			t.Errorf("edgeTypeHint() = %q, missing canonical type %q", got, et)
		}
	}
	if n := strings.Count(got, "|") + 1; n != len(models.CanonicalEdgeTypes) {
		t.Errorf("edgeTypeHint() = %q lists %d types, want %d", got, n, len(models.CanonicalEdgeTypes))
	}
}

// TestBuildRuleFromFlags_UnknownEventTypeNamesTheSet is the message half of the
// gate. Rejecting is not enough on its own: an author who reaches for
// `--on-event task.status-changed` cannot fix it from "unknown event type"
// alone, and until this test the error pointed at `adb events` — a command whose
// three subcommands (digest/query/tail) enumerate nothing, so the pointer bottomed
// out in "go read internal/observability/schema.go". The error names the whole
// vocabulary, the way EdgeType.Validate names the edge one.
func TestBuildRuleFromFlags_UnknownEventTypeNamesTheSet(t *testing.T) {
	_, err := buildRuleFromFlags("r", "", "bogus.type", "", "", "triage", "", nil, nil, "", false)
	if err == nil {
		t.Fatal("expected --on-event bogus.type to be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bogus.type") {
		t.Errorf("error must quote the offending type, got %q", msg)
	}
	for _, e := range observability.KnownEventTypes {
		if !strings.Contains(msg, string(e)) {
			t.Errorf("error must name the valid set; %q is missing from %q", e, msg)
		}
	}
}

// TestBuildRuleFromFlags_AcceptsEveryKnownEventType pins the call on the
// vocabulary's edges: the accepted set is *exactly* observability.KnownEventTypes,
// with no hand-maintained subset in this package.
//
// That includes the two RESERVED types — task.completed and knowledge.extracted —
// which nothing currently emits, so a rule triggered on one can never fire from
// the event log. They are accepted anyway, for three reasons. (1) Emitted-ness is
// not recorded anywhere a program can read; excluding them means hard-coding a
// denylist in this file that restates a fact living only in a schema_test.go
// comment, and goes stale in the DANGEROUS direction — worktree.created was
// reserved until #206 graduated it, and a stale denylist would reject a type that
// now fires. (2) The event log is not the only dispatcher: `adb schedule dispatch
// --event knowledge.extracted` fires such a rule today, as can a hook. (3) The
// error would be a false statement — "unknown event type" about a type that is in
// the documented schema.
func TestBuildRuleFromFlags_AcceptsEveryKnownEventType(t *testing.T) {
	for _, e := range observability.KnownEventTypes {
		t.Run(string(e), func(t *testing.T) {
			r, err := buildRuleFromFlags("r", "", string(e), "", "", "triage", "", nil, nil, "", false)
			if err != nil {
				t.Fatalf("--on-event %s must be accepted (it is in KnownEventTypes): %v", e, err)
			}
			if r.On.Event != string(e) {
				t.Fatalf("event = %q, want %q", r.On.Event, e)
			}
		})
	}
	// Named explicitly so deleting the loop cannot quietly drop the reserved case.
	for _, reserved := range []observability.EventType{
		observability.EventTaskCompleted, observability.EventKnowledgeExtracted,
	} {
		if _, err := buildRuleFromFlags("r", "", string(reserved), "", "", "s", "", nil, nil, "", false); err != nil {
			t.Errorf("reserved-but-unemitted %q must still be accepted: %v", reserved, err)
		}
	}
}

// TestBuildRuleFromFlags_EventTypeWhitespaceAndCase pins that --on-event handles
// its argument exactly as --write-edge handles an edge type: surrounding
// whitespace is trimmed (a shell-quoting slip is forgiven), but case is NOT
// folded. Event types are literal keys compared for equality by the dispatcher
// and by cli/events.go's filters, so accepting "Task.Created" and storing it
// would author a rule that matches nothing — the very failure being fixed.
func TestBuildRuleFromFlags_EventTypeWhitespaceAndCase(t *testing.T) {
	r, err := buildRuleFromFlags("r", "", "  task.created  ", "", "", "s", "", nil, nil, "", false)
	if err != nil {
		t.Fatalf("surrounding whitespace should be trimmed, got %v", err)
	}
	if r.On.Event != "task.created" {
		t.Errorf("event = %q, want the trimmed %q", r.On.Event, "task.created")
	}
	if _, err := buildRuleFromFlags("r", "", "TASK.CREATED", "", "", "s", "", nil, nil, "", false); err == nil {
		t.Error("case must not be folded: TASK.CREATED would never match a dispatched event")
	}
}

// TestEventTypeHint_DerivedFromVocabulary is the --on-event twin of
// TestEdgeTypeHint_DerivedFromVocabulary, with one difference forced by scale:
// there are 19 event types against 5 edge types, so the full pipe-joined list
// edgeTypeHint() produces would be a ~330-character wall of text in flag help,
// dwarfing every other flag on `adb schedule add --help`. The hint therefore
// renders the derived GROUP PREFIXES plus the count, and the full set goes in the
// rejection error — read once, by someone who is stuck, which is where the 330
// characters earn their place.
//
// Both halves must be derived, so what the help advertises cannot drift from what
// the gate enforces: a new type in a new group appears as a new prefix, and a new
// type in an existing group bumps the count.
func TestEventTypeHint_DerivedFromVocabulary(t *testing.T) {
	hint := eventTypeHint()

	// Every distinct prefix in the vocabulary is advertised...
	seen := map[string]bool{}
	for _, e := range observability.KnownEventTypes {
		prefix, _, ok := strings.Cut(string(e), ".")
		if !ok {
			t.Fatalf("event type %q has no '.' — the prefix hint assumes group.name", e)
		}
		if seen[prefix] {
			continue
		}
		seen[prefix] = true
		if !strings.Contains(hint, prefix+".*") {
			t.Errorf("eventTypeHint() = %q, missing group %q", hint, prefix+".*")
		}
	}
	// ...and nothing beyond them, so the hint is a rendering rather than prose that
	// happens to contain the right words.
	if n := strings.Count(hint, ".*"); n != len(seen) {
		t.Errorf("eventTypeHint() = %q lists %d groups, want %d", hint, n, len(seen))
	}
	// The count is derived too — hard-coding "19" is how the help goes stale.
	if !strings.Contains(hint, fmt.Sprintf("%d", len(observability.KnownEventTypes))) {
		t.Errorf("eventTypeHint() = %q must state the derived count %d",
			hint, len(observability.KnownEventTypes))
	}

	// The full list, used by the rejection error, names every type verbatim.
	full := eventTypeList()
	for _, e := range observability.KnownEventTypes {
		if !strings.Contains(full, string(e)) {
			t.Errorf("eventTypeList() = %q, missing %q", full, e)
		}
	}
}

// TestScheduleAddCmd_OnEventFlagHelpIsDerived reads the flag's registered usage
// string, because that — not the helper — is what a user actually sees. A derived
// helper wired into a flag whose help still says "e.g. task.status_changed" is the
// drift this is meant to catch.
func TestScheduleAddCmd_OnEventFlagHelpIsDerived(t *testing.T) {
	flag := newScheduleAddCmd().Flags().Lookup("on-event")
	if flag == nil {
		t.Fatal("--on-event flag not registered")
	}
	usage := flag.Usage
	if !strings.Contains(usage, eventTypeHint()) {
		t.Errorf("--on-event usage = %q, want it to embed eventTypeHint() = %q", usage, eventTypeHint())
	}
	// A type the test did not hardcode: the LAST entry in the vocabulary, whose
	// group must show up. A hardcoded help string mentioning only task.* passes a
	// naive "contains an event type" check and fails this one.
	last := observability.KnownEventTypes[len(observability.KnownEventTypes)-1]
	prefix, _, _ := strings.Cut(string(last), ".")
	if !strings.Contains(usage, prefix+".*") {
		t.Errorf("--on-event usage = %q must cover the last vocabulary entry %q (group %q)",
			usage, last, prefix+".*")
	}
}

// TestSchedule_TolerantOfLegacyUnknownEventType is the READ half of the split, and
// the reason the gate lives at the write surface rather than in Rule.Validate. A
// workspace whose automation/rules.yaml already carries an unknown event type —
// hand-edited, or authored by a build predating the gate — must stay fully usable.
//
// It asserts three things, and the second two are the ones with teeth. Listing
// alone is NOT a sufficient guard: FileRuleStore.Load does not call Validate at
// all, so `schedule list` keeps working even if Rule.Validate rejects unknown
// types — an earlier version of this test checked only listing and was proven
// vacuous by exactly that control. The breakage a Validate-side check causes is on
// SAVE: RuleSet.Validate runs over every rule in the set, so one legacy rule makes
// the whole file unwritable, and the user can then neither author new automation
// nor delete the offending rule.
func TestSchedule_TolerantOfLegacyUnknownEventType(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "automation"), 0o755); err != nil {
		t.Fatalf("mkdir automation: %v", err)
	}
	// `legacy` carries an unknown event type AND a non-canonical edge type; read
	// tolerates both. `keeper` is well-formed, and exists so that removing it leaves
	// `legacy` in the set being saved — otherwise Save would validate an empty set
	// and prove nothing.
	rules := "rules:\n" +
		"  - name: legacy\n" +
		"    on:\n" +
		"      event: bogus.type\n" +
		"    if:\n" +
		"      entity: TASK-1\n" +
		"      has_edge: mentions\n" +
		"    run:\n" +
		"      skill: triage\n" +
		"  - name: keeper\n" +
		"    on:\n" +
		"      schedule: 1h\n" +
		"    run:\n" +
		"      skill: repos-pull\n"
	rulesPath := filepath.Join(tmp, "automation", "rules.yaml")
	if err := os.WriteFile(rulesPath, []byte(rules), 0o644); err != nil {
		t.Fatalf("write rules.yaml: %v", err)
	}

	app, err := internal.NewAppIsolated(tmp)
	if err != nil {
		t.Fatalf("NewAppIsolated: %v", err)
	}
	oldApp := App
	App = app
	defer func() {
		App = oldApp
		app.Cleanup()
	}()

	run := func(t *testing.T, cmd *cobra.Command, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s %v: %v\noutput:\n%s", cmd.Name(), args, err, out.String())
		}
		return out.String()
	}

	// 1. It still LOADS and LISTS, with the unknown type shown verbatim — listing it
	//    is how you find the rule you need to fix.
	got := run(t, newScheduleListCmd())
	if !strings.Contains(got, "legacy") || !strings.Contains(got, "bogus.type") {
		t.Errorf("the legacy rule must still be listed verbatim; output was:\n%s", got)
	}

	// 2. A NEW rule can still be authored beside it. This rewrites the whole file,
	//    legacy rule included, so a Validate-side known-type check fails here.
	run(t, newScheduleAddCmd(), "--name", "fresh", "--on-event", "task.created", "--run-skill", "triage")

	// 3. And another rule can still be REMOVED, which likewise re-saves the set with
	//    the legacy rule in it. Without this, a user cannot even clean up.
	run(t, newScheduleRemoveCmd(), "keeper")

	// 4. And it is still FIREABLE by name. `schedule dispatch --event` is
	//    deliberately NOT gated against the vocabulary: it is the firing surface, not
	//    an authoring one, and dispatching the unknown name is the only way to
	//    exercise a legacy rule at all. A future contributor "finishing the job" by
	//    gating dispatch too would make such a rule permanently unfireable.
	fired := run(t, newScheduleDispatchCmd(), "--event", "bogus.type")
	if !strings.Contains(fired, "legacy") {
		t.Errorf("dispatch must still fire a legacy rule by its unknown event name; output was:\n%s", fired)
	}

	// The legacy rule survived both writes untouched, rather than being dropped or
	// rewritten on the way through.
	after, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("read back rules.yaml: %v", err)
	}
	if !strings.Contains(string(after), "bogus.type") {
		t.Errorf("the legacy rule must survive a save verbatim; rules.yaml is now:\n%s", after)
	}
}
