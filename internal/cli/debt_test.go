package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// withDebtApp wires a real, ISOLATED App over a fresh temp workspace and returns
// its root. Isolation matters here for the same reason it does in
// program_test.go: an un-isolated App merges the developer's real ~/.taskconfig,
// and `adb debt` writes into the workspace it resolves — a test must never be
// able to append to a real debt/index.yaml.
func withDebtApp(t *testing.T) string {
	t.Helper()
	return withAppAt(t, t.TempDir()).BasePath
}

// addDebt records one item and returns nothing but a green assertion — the
// helper exists so the ordering tests read as a fixture rather than six
// near-identical runADB calls.
func addDebt(t *testing.T, args ...string) {
	t.Helper()
	if err := runADB(t, append([]string{"debt", "add"}, args...)...); err != nil {
		t.Fatalf("debt add %v: %v", args, err)
	}
}

// TestDebtCLI_Add covers the three shapes of `adb debt add`: bare, with the
// optional metadata flags, and --json.
func TestDebtCLI_Add(t *testing.T) {
	withDebtApp(t)

	out := captureStdout(t, func() { addDebt(t, "panics on nil graph") })
	if !strings.Contains(out, "DEBT-0001") || !strings.Contains(out, "panics on nil graph") {
		t.Errorf("add output = %q", out)
	}
	// The default priority is P2 and is reported, so the recorded triage rank is
	// never a silent default.
	if !strings.Contains(out, "[P2]") {
		t.Errorf("add output should report the priority, got %q", out)
	}
	// The graph node id is named at creation — the same contract `adb adr new`
	// honours by printing the ADR's file path.
	if !strings.Contains(out, "debt:DEBT-0001") {
		t.Errorf("add output should name the graph node, got %q", out)
	}

	out = captureStdout(t, func() {
		addDebt(t, "flat knowledge scan", "--area", "core", "--note", "walks flat ticket paths", "--priority", "P1")
	})
	if !strings.Contains(out, "DEBT-0002") || !strings.Contains(out, "[P1]") {
		t.Errorf("add output = %q", out)
	}

	jsonOut := captureStdout(t, func() {
		addDebt(t, "json shaped", "--area", "cli", "--note", "n", "--json")
	})
	var item models.DebtItem
	if err := json.Unmarshal([]byte(jsonOut), &item); err != nil {
		t.Fatalf("debt add --json is not a JSON object: %v\n%s", err, jsonOut)
	}
	if item.ID != "DEBT-0003" || item.Title != "json shaped" || item.Area != "cli" ||
		item.Note != "n" || item.Priority != models.PriorityP2 || item.Status != models.DebtOpen {
		t.Errorf("add --json item = %+v", item)
	}
	if item.Created.IsZero() {
		t.Error("add --json should carry a created timestamp")
	}
}

// TestDebtCLI_Add_RejectsInvalidPriority is the CLI half of #158: the manager
// validates, and the command must surface that as a non-zero exit rather than
// storing a mis-triaging value.
func TestDebtCLI_Add_RejectsInvalidPriority(t *testing.T) {
	withDebtApp(t)

	for _, bad := range []string{"p0", "P4", "critical"} {
		err := runADB(t, "debt", "add", "critical thing", "--priority", bad)
		if err == nil {
			t.Errorf("--priority %q should be rejected", bad)
			continue
		}
		if !strings.Contains(err.Error(), "invalid priority") {
			t.Errorf("--priority %q error = %v, want it to name the invalid priority", bad, err)
		}
	}
	// Nothing was recorded.
	out := captureStdout(t, func() {
		if err := runADB(t, "debt", "list"); err != nil {
			t.Fatalf("debt list: %v", err)
		}
	})
	if !strings.Contains(out, "No tech-debt items") {
		t.Errorf("rejected adds should leave the registry empty, got %q", out)
	}
}

// TestDebtCLI_List_HeaderAndTriageOrder pins the tabwriter header (every other
// listing in the CLI has one) and the documented triage order: open before
// resolved, then priority P0→P3, then id.
func TestDebtCLI_List_HeaderAndTriageOrder(t *testing.T) {
	withDebtApp(t)

	addDebt(t, "middling", "--priority", "P2")           // DEBT-0001
	addDebt(t, "urgent", "--priority", "P0")             // DEBT-0002
	addDebt(t, "also urgent", "--priority", "P0")        // DEBT-0003
	addDebt(t, "will be resolved", "--priority", "P0")   // DEBT-0004
	addDebt(t, "low", "--priority", "P3", "--area", "x") // DEBT-0005
	if err := runADB(t, "debt", "resolve", "DEBT-0004"); err != nil {
		t.Fatalf("debt resolve: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runADB(t, "debt", "list"); err != nil {
			t.Fatalf("debt list: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("want a header + 5 rows, got %d lines:\n%s", len(lines), out)
	}
	// The header is column-aligned by tabwriter, so match on fields not spacing.
	if got := strings.Fields(lines[0]); len(got) != 5 ||
		got[0] != "ID" || got[1] != "PRI" || got[2] != "STATUS" || got[3] != "AREA" || got[4] != "TITLE" {
		t.Errorf("header = %q, want ID PRI STATUS AREA TITLE", lines[0])
	}
	// Triage order: the two open P0s by id, then P2, then P3, then the resolved P0.
	wantIDs := []string{"DEBT-0002", "DEBT-0003", "DEBT-0001", "DEBT-0005", "DEBT-0004"}
	for i, want := range wantIDs {
		if !strings.HasPrefix(lines[i+1], want) {
			t.Errorf("row %d = %q, want it to start with %s", i, lines[i+1], want)
		}
	}
	// A resolved row still reports its status, and an area-less row renders "-"
	// rather than a ragged blank column.
	if !strings.Contains(lines[5], string(models.DebtResolved)) {
		t.Errorf("resolved row = %q", lines[5])
	}
	if !strings.Contains(lines[1], " - ") {
		t.Errorf("area-less row should render a dash placeholder, got %q", lines[1])
	}
}

// TestDebtCLI_List_OpenFilter asserts --open drops resolved items from both
// shapes of the listing.
func TestDebtCLI_List_OpenFilter(t *testing.T) {
	withDebtApp(t)

	addDebt(t, "still open")
	addDebt(t, "already done")
	if err := runADB(t, "debt", "resolve", "DEBT-0002"); err != nil {
		t.Fatalf("debt resolve: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runADB(t, "debt", "list", "--open"); err != nil {
			t.Fatalf("debt list --open: %v", err)
		}
	})
	if !strings.Contains(out, "DEBT-0001") {
		t.Errorf("--open should keep the open item: %q", out)
	}
	if strings.Contains(out, "DEBT-0002") {
		t.Errorf("--open should drop the resolved item: %q", out)
	}

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "debt", "list", "--open", "--json"); err != nil {
			t.Fatalf("debt list --open --json: %v", err)
		}
	})
	var items []models.DebtItem
	if err := json.Unmarshal([]byte(jsonOut), &items); err != nil {
		t.Fatalf("debt list --json is not a JSON array: %v\n%s", err, jsonOut)
	}
	if len(items) != 1 || items[0].ID != "DEBT-0001" {
		t.Errorf("--open --json items = %+v", items)
	}
}

// TestDebtCLI_List_Empty asserts the empty case is a hint, not a bare header.
func TestDebtCLI_List_Empty(t *testing.T) {
	withDebtApp(t)

	out := captureStdout(t, func() {
		if err := runADB(t, "debt", "list"); err != nil {
			t.Fatalf("debt list: %v", err)
		}
	})
	if !strings.Contains(out, "No tech-debt items") || !strings.Contains(out, "adb debt add") {
		t.Errorf("empty listing should name the command that fixes it, got %q", out)
	}

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "debt", "list", "--json"); err != nil {
			t.Fatalf("debt list --json: %v", err)
		}
	})
	if got := strings.TrimSpace(jsonOut); got != "[]" && got != "null" {
		t.Errorf("empty --json = %q, want an empty array", got)
	}
}

// TestDebtCLI_ListJSON_KeysAreSnakeCase keeps the --json contract byte-compatible
// for a consumer: snake_case keys throughout (the `adb program` convention), and
// the optional fields omitted rather than emitted empty.
func TestDebtCLI_ListJSON_KeysAreSnakeCase(t *testing.T) {
	withDebtApp(t)

	addDebt(t, "bare item")
	addDebt(t, "annotated item", "--area", "core", "--note", "detail", "--relates-to", "TASK-1")

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "debt", "list", "--json"); err != nil {
			t.Fatalf("debt list --json: %v", err)
		}
	})
	var rows []map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, jsonOut)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		byID[id] = r
	}

	bare, ok := byID["DEBT-0001"]
	if !ok {
		t.Fatalf("DEBT-0001 missing from %v", rows)
	}
	for _, want := range []string{"id", "title", "priority", "status", "created"} {
		if _, present := bare[want]; !present {
			t.Errorf("bare row missing key %q: %v", want, bare)
		}
	}
	for _, absent := range []string{"area", "note", "links", "resolved"} {
		if _, present := bare[absent]; present {
			t.Errorf("bare row should omit empty key %q: %v", absent, bare)
		}
	}

	annotated := byID["DEBT-0002"]
	for _, want := range []string{"area", "note", "links"} {
		if _, present := annotated[want]; !present {
			t.Errorf("annotated row missing key %q: %v", want, annotated)
		}
	}
	links, _ := annotated["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("annotated row links = %v", annotated["links"])
	}
	link, _ := links[0].(map[string]any)
	if link["type"] != string(models.EdgeRelatesTo) || link["target"] != "TASK-1" {
		t.Errorf("link = %v", link)
	}
	// Every key in every row is snake_case (no camelCase creep).
	for _, r := range rows {
		for k := range r {
			if strings.ToLower(k) != k {
				t.Errorf("key %q is not snake_case", k)
			}
		}
	}
}

// TestDebtCLI_Resolve covers the transition, its idempotency, and the unknown-id
// error.
func TestDebtCLI_Resolve(t *testing.T) {
	withDebtApp(t)

	addDebt(t, "fix me")

	out := captureStdout(t, func() {
		if err := runADB(t, "debt", "resolve", "DEBT-0001"); err != nil {
			t.Fatalf("debt resolve: %v", err)
		}
	})
	if !strings.Contains(out, "DEBT-0001 resolved") {
		t.Errorf("resolve output = %q", out)
	}

	// Idempotent: resolving again is a no-op, not an error.
	if err := runADB(t, "debt", "resolve", "DEBT-0001"); err != nil {
		t.Errorf("second resolve should be a no-op, got %v", err)
	}

	// The registry records it once, resolved, with a resolved timestamp.
	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "debt", "list", "--json"); err != nil {
			t.Fatalf("debt list --json: %v", err)
		}
	})
	var items []models.DebtItem
	if err := json.Unmarshal([]byte(jsonOut), &items); err != nil {
		t.Fatalf("not a JSON array: %v\n%s", err, jsonOut)
	}
	if len(items) != 1 || items[0].Status != models.DebtResolved {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Resolved == nil || items[0].Resolved.IsZero() {
		t.Error("a resolved item should carry a resolved timestamp")
	}

	if err := runADB(t, "debt", "resolve", "DEBT-9999"); err == nil {
		t.Error("resolving an unknown id should error")
	}
}

// TestDebtCLI_IsAGraphNode is the integration this ticket exists for: a debt item
// is a first-class debt:DEBT-NNNN node in the typed graph, exactly as an ADR is
// an adr:NNNN node. No `adb graph rebuild` is needed — GraphManager.Neighbors
// always derives from the authoritative registries, and rebuild only materialises
// the cache.
func TestDebtCLI_IsAGraphNode(t *testing.T) {
	app := withAppAt(t, t.TempDir())

	addDebt(t, "linked debt", "--relates-to", "TASK-1")
	addDebt(t, "unlinked debt")

	edges, err := app.GraphManager.Neighbors("debt:DEBT-0001")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("debt:DEBT-0001 edges = %+v, want 1", edges)
	}
	if edges[0].From != "debt:DEBT-0001" || edges[0].Type != models.EdgeRelatesTo || edges[0].To != "TASK-1" {
		t.Errorf("edge = %+v", edges[0])
	}
	// The edge is visible from the other end too, so a ticket sees the debt owed
	// against it.
	taskEdges, err := app.GraphManager.Neighbors("TASK-1")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(taskEdges) != 1 || taskEdges[0].From != "debt:DEBT-0001" {
		t.Errorf("TASK-1 edges = %+v", taskEdges)
	}

	// A linkless item is a legitimate degree-0 node: it declares no edges, so it
	// contributes none. That is not a failure — the catalog still lists it.
	unlinked, err := app.GraphManager.Neighbors("debt:DEBT-0002")
	if err != nil {
		t.Fatalf("Neighbors: %v", err)
	}
	if len(unlinked) != 0 {
		t.Errorf("debt:DEBT-0002 should have no edges, got %+v", unlinked)
	}

	// Rebuild persists the same edge into the derived index cache.
	g, err := app.GraphManager.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if len(g.Index().Edges) != 1 || g.Index().Edges[0].From != "debt:DEBT-0001" {
		t.Errorf("rebuilt index = %+v", g.Index().Edges)
	}
}

// TestDebtCLI_CatalogSurfacesDebt is the other half of the integration: debt
// appears under `adb catalog --kind debt`, carries its graph degree like every
// other catalog entry, and is counted in the unfiltered summary.
func TestDebtCLI_CatalogSurfacesDebt(t *testing.T) {
	withDebtApp(t)

	addDebt(t, "linked debt", "--priority", "P0", "--area", "core", "--relates-to", "TASK-1")
	addDebt(t, "unlinked debt")

	jsonOut := captureStdout(t, func() {
		if err := runADB(t, "catalog", "show", "--kind", "debt", "--json"); err != nil {
			t.Fatalf("catalog show --kind debt --json: %v", err)
		}
	})
	var entries []models.CatalogDebt
	if err := json.Unmarshal([]byte(jsonOut), &entries); err != nil {
		t.Fatalf("--kind debt --json is not a JSON array: %v\n%s", err, jsonOut)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 catalog debt entries, got %+v", entries)
	}
	first := entries[0]
	if first.ID != "debt:DEBT-0001" {
		t.Errorf("entry id = %q, want the graph id debt:DEBT-0001", first.ID)
	}
	if first.Priority != string(models.PriorityP0) || first.Status != string(models.DebtOpen) ||
		first.Area != "core" || first.Title != "linked debt" {
		t.Errorf("entry = %+v", first)
	}
	if first.Edges != 1 {
		t.Errorf("entry edges = %d, want 1 (the relates_to edge)", first.Edges)
	}
	if entries[1].Edges != 0 {
		t.Errorf("unlinked entry edges = %d, want 0", entries[1].Edges)
	}

	// The human view and the unfiltered summary both account for debt.
	out := captureStdout(t, func() {
		if err := runADB(t, "catalog", "show"); err != nil {
			t.Fatalf("catalog show: %v", err)
		}
	})
	if !strings.Contains(out, "Tech debt (2):") {
		t.Errorf("catalog show should list a tech-debt section, got:\n%s", out)
	}
	if !strings.Contains(out, "2 debt") {
		t.Errorf("catalog summary should count debt, got:\n%s", out)
	}
	if !strings.Contains(out, "debt:DEBT-0001") {
		t.Errorf("catalog show should name the debt graph id, got:\n%s", out)
	}
}

// TestDebtCLI_CatalogKindIsValidated guards the class of defect the `--filter
// Done` bug was: an unrecognised selector must be rejected with the valid set,
// never silently return nothing.
func TestDebtCLI_CatalogKindIsValidated(t *testing.T) {
	withDebtApp(t)

	err := runADB(t, "catalog", "show", "--kind", "debts")
	if err == nil {
		t.Fatal("an unknown --kind should error")
	}
	if !strings.Contains(err.Error(), "debt") {
		t.Errorf("the error should list the valid kinds including debt, got %v", err)
	}

	// The accepted spelling is case-insensitive, like every other kind.
	if err := runADB(t, "catalog", "show", "--kind", "DEBT"); err != nil {
		t.Errorf("--kind DEBT should be accepted, got %v", err)
	}
}
