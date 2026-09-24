package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file drives `adb program` and `adb init` end-to-end through the built
// binary, in temp directories. The in-process tests in internal/cli cover the
// same surface; what only this level can prove is that a SEPARATE PROCESS
// resolves its own workspace, reads its own config tiers, and finds the packs in
// its own embedded FS — and that the paths it prints are usable in the workspace
// it printed them for.

const (
	// defaultPack is the pack `adb init project` provisions when not told otherwise.
	defaultPack = "technical-design"
	// defaultPackFiles is the pack's file count: one manifest + eight templates.
	defaultPackFiles = 9
	// brdOutput is the first-phase artifact of defaultPack — the one template with
	// no requirements, so it is the pack's single entry point.
	brdOutput = "docs/technical-design/requirements/business-requirements.md"
)

// --- helpers ---------------------------------------------------------------

// newProject scaffolds a fresh project with `adb init project` and returns its
// root. extraArgs are appended to the init command (e.g. --no-programs).
func newProject(t *testing.T, extraArgs ...string) string {
	t.Helper()
	parent := t.TempDir()
	ws := filepath.Join(parent, "ws")
	args := append([]string{"init", "project", ws, "--name", "scratch"}, extraArgs...)
	// NOTE: the flag is --git, not --git-init.
	mustRunADB(t, parent, args...)
	if _, err := os.Stat(filepath.Join(ws, ".taskrc")); err != nil {
		t.Fatalf("init project did not scaffold a .taskrc: %v", err)
	}
	return ws
}

// readManifest loads a project's .adb/template-manifest.yaml.
func readManifest(t *testing.T, ws string) models.TemplateManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ws, ".adb", "template-manifest.yaml"))
	if err != nil {
		t.Fatalf("read template manifest: %v", err)
	}
	var m models.TemplateManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse template manifest: %v", err)
	}
	return m
}

// writeManifest persists a mutated manifest back to the project.
func writeManifest(t *testing.T, ws string, m models.TemplateManifest) {
	t.Helper()
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".adb", "template-manifest.yaml"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// updateGroups parses `adb init update` output into its four named groups. The
// output shape is "  <label> (<n>):" followed by one indented path per line.
func updateGroups(out string) map[string][]string {
	groups := map[string][]string{}
	current := ""
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if label, _, found := strings.Cut(trimmed, " ("); found && strings.HasSuffix(trimmed, "):") {
			switch label {
			case "added", "updated", "conflict", "unchanged":
				current = label
				if _, ok := groups[current]; !ok {
					groups[current] = nil
				}
				continue
			}
		}
		// A file line is indented further than its group header.
		if current != "" && strings.HasPrefix(line, "    ") {
			groups[current] = append(groups[current], trimmed)
			continue
		}
		current = ""
	}
	return groups
}

// packFilesIn returns the entries of group that belong to programs/<pack>/.
func packFilesIn(files []string, pack string) []string {
	var out []string
	prefix := "programs/" + pack + "/"
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// programListRow is the `--json` row of `adb program list`. `source` is "embedded"
// for a shipped pack, otherwise the search-path directory it was found in; `error`
// is set for a pack that lists but will not load.
type programListRow struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Error  string `json:"error"`
}

// statusRow is the flat `--json` row of `adb program status` / `next`.
type statusRow struct {
	ID         string   `json:"id"`
	Phase      string   `json:"phase"`
	State      string   `json:"state"`
	Output     string   `json:"output"`
	OutputPath string   `json:"output_path"`
	Trigger    string   `json:"trigger"`
	BlockedBy  []string `json:"blocked_by"`
}

// programStatus runs `adb program status --json` and decodes it.
func programStatus(t *testing.T, ws, pack string) []statusRow {
	t.Helper()
	res := mustRunADB(t, ws, "program", "status", pack, "--json")
	var rows []statusRow
	if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
		t.Fatalf("program status --json: %v\n%s", err, res.stdout)
	}
	return rows
}

// stateOf returns the state of one template id.
func stateOf(t *testing.T, rows []statusRow, id string) string {
	t.Helper()
	for _, r := range rows {
		if r.ID == id {
			return r.State
		}
	}
	t.Fatalf("no template %q in status rows", id)
	return ""
}

// writeFileIn creates a workspace-relative file, making parents as needed.
func writeFileIn(t *testing.T, ws, rel, body string) {
	t.Helper()
	dest := filepath.Join(ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- the full lifecycle ----------------------------------------------------

// TestE2E_ProgramLifecycle walks the whole documented flow against the real
// binary: init → list/show/status/next → draft an artifact → re-status → trace →
// scaffold a second pack → init update. Each step asserts on real stdout and real
// files in a temp workspace.
func TestE2E_ProgramLifecycle(t *testing.T) {
	ws := newProject(t, "--git")

	t.Run("init provisions the default pack and records it", func(t *testing.T) {
		manifestPath := filepath.Join(ws, "programs", defaultPack, "program.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("init project did not provision %s: %v", defaultPack, err)
		}
		m := readManifest(t, ws)
		if got := strings.Join(m.Options.WithPrograms, ","); got != defaultPack {
			t.Errorf("manifest with_programs = %q, want %q (the RESOLVED pack list)", got, defaultPack)
		}
		if m.Options.NoPrograms {
			t.Error("manifest recorded no_programs on a default init")
		}
		// The pack's files are tracked in the provenance baseline, which is what
		// makes them re-syncable by `init update` like any other artifact.
		var tracked int
		for rel := range m.Files {
			if strings.HasPrefix(rel, "programs/"+defaultPack+"/") {
				tracked++
			}
		}
		if tracked != defaultPackFiles {
			t.Errorf("manifest tracks %d pack files, want %d", tracked, defaultPackFiles)
		}
	})

	t.Run("list surfaces every shipped pack", func(t *testing.T) {
		res := mustRunADB(t, ws, "program", "list", "--json")
		var rows []programListRow
		if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
			t.Fatalf("list --json: %v\n%s", err, res.stdout)
		}
		got := map[string]bool{}
		for _, r := range rows {
			if r.Error != "" {
				t.Errorf("pack %s is unreadable through the binary: %s", r.ID, r.Error)
			}
			got[r.ID] = true
		}
		for _, want := range []string{
			"product-discovery", "technical-design", "delivery-readiness",
			"operations", "change-adoption",
		} {
			if !got[want] {
				t.Errorf("list omits shipped pack %q", want)
			}
		}
	})

	t.Run("show prints the manifest structure", func(t *testing.T) {
		res := mustRunADB(t, ws, "program", "show", defaultPack)
		for _, want := range []string{"Technical Design", "Lineage:", "requirements", "output:", "review:"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("show output missing %q:\n%s", want, res.stdout)
			}
		}
	})

	t.Run("a fresh workspace has nothing generated", func(t *testing.T) {
		rows := programStatus(t, ws, defaultPack)
		if len(rows) != 8 {
			t.Fatalf("got %d templates, want 8", len(rows))
		}
		for _, r := range rows {
			if r.State == "generated" {
				t.Errorf("%s is generated in a fresh workspace (output %s)", r.ID, r.OutputPath)
			}
			// Every resolved output must sit inside THIS workspace. If ADB_HOME
			// leaked, these would point at the developer's real workspace.
			if !strings.HasPrefix(r.OutputPath, ws+string(os.PathSeparator)) {
				t.Errorf("%s output_path %q escapes the temp workspace %q — ADB_HOME leaked",
					r.ID, r.OutputPath, ws)
			}
		}
		if got := stateOf(t, rows, "business-requirements"); got != "ready" {
			t.Errorf("business-requirements = %q, want ready", got)
		}
		if got := stateOf(t, rows, "prd"); got != "blocked" {
			t.Errorf("prd = %q, want blocked", got)
		}
		if got := stateOf(t, rows, "design-doc-mini"); got != "pending" {
			t.Errorf("design-doc-mini (manual) = %q, want pending", got)
		}
	})

	t.Run("next prints a from: path that exists in this workspace", func(t *testing.T) {
		res := mustRunADB(t, ws, "program", "next", defaultPack)
		var froms []string
		for _, line := range strings.Split(res.stdout, "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "from:") {
				froms = append(froms, strings.TrimSpace(strings.TrimPrefix(trimmed, "from:")))
			}
		}
		if len(froms) == 0 {
			t.Fatalf("next printed no from: path:\n%s", res.stdout)
		}
		// This is the shipped-defect regression, checked at the level a user hits
		// it: copy the printed path, resolve it from the workspace root, open it.
		for _, from := range froms {
			resolved := filepath.Join(ws, filepath.FromSlash(from))
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("next printed from: %q which does not exist at %s: %v", from, resolved, err)
			}
		}
	})

	t.Run("drafting the upstream artifact unblocks its dependent", func(t *testing.T) {
		writeFileIn(t, ws, brdOutput, "---\nsources: []\n---\n# Business Requirements\n")

		rows := programStatus(t, ws, defaultPack)
		if got := stateOf(t, rows, "business-requirements"); got != "generated" {
			t.Errorf("business-requirements = %q, want generated", got)
		}
		if got := stateOf(t, rows, "prd"); got != "ready" {
			t.Errorf("prd = %q, want ready once its requirement exists", got)
		}

		// The human view must surface the outstanding review obligation.
		res := mustRunADB(t, ws, "program", "status", defaultPack)
		if !strings.Contains(res.stdout, "still need human review") {
			t.Errorf("status does not surface the human-review obligation:\n%s", res.stdout)
		}
	})

	t.Run("trace reads the sources frontmatter", func(t *testing.T) {
		writeFileIn(t, ws, "project-doc/vision.md", "# Vision\n")
		writeFileIn(t, ws, "docs/technical-design/requirements/prd.md",
			"---\nsources:\n"+
				"  - "+brdOutput+"\n"+
				"  - path: project-doc/vision.md\n    note: framing\n"+
				"  - project-doc/gone.md\n"+
				"---\n# PRD\n")

		res := mustRunADB(t, ws, "program", "trace", defaultPack, "prd", "--json")
		var tr struct {
			TemplateID string `json:"template_id"`
			Artifact   string `json:"artifact"`
			Sources    []struct {
				Path       string `json:"path"`
				Note       string `json:"note"`
				TemplateID string `json:"template_id"`
				Exists     bool   `json:"exists"`
			} `json:"sources"`
			MissingRequires []string `json:"missing_requires"`
		}
		if err := json.Unmarshal([]byte(res.stdout), &tr); err != nil {
			t.Fatalf("trace --json: %v\n%s", err, res.stdout)
		}
		if len(tr.Sources) != 3 {
			t.Fatalf("trace found %d sources, want 3", len(tr.Sources))
		}
		if tr.Sources[0].TemplateID != "business-requirements" {
			t.Errorf("a cited program artifact should resolve to its template id, got %q", tr.Sources[0].TemplateID)
		}
		if !tr.Sources[0].Exists || !tr.Sources[1].Exists {
			t.Error("cited files that exist should be marked present")
		}
		if tr.Sources[2].Exists {
			t.Error("a cited-but-absent file must not be marked present")
		}
		if tr.Sources[1].Note != "framing" {
			t.Errorf("source note = %q, want framing", tr.Sources[1].Note)
		}
		if len(tr.MissingRequires) != 0 {
			t.Errorf("missing_requires = %v, want none (the requirement is cited)", tr.MissingRequires)
		}
	})

	t.Run("trace errors before an artifact is generated", func(t *testing.T) {
		res := runADB(t, ws, "program", "trace", defaultPack, "threat-model")
		if res.err == nil {
			t.Fatal("trace should fail for an artifact that has not been generated")
		}
		if !strings.Contains(res.combined(), "not been generated") {
			t.Errorf("error should say the artifact is not generated yet:\n%s", res.combined())
		}
	})

	t.Run("scaffolding a second pack is idempotent then clobber-safe", func(t *testing.T) {
		dest := filepath.Join(ws, "programs", "operations")

		plan := mustRunADB(t, ws, "program", "scaffold", "operations", "--dry-run")
		if !strings.Contains(plan.stdout, "Would scaffold") {
			t.Errorf("--dry-run should say it would scaffold:\n%s", plan.stdout)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatalf("--dry-run wrote to disk (err=%v)", err)
		}

		mustRunADB(t, ws, "program", "scaffold", "operations")
		manifest := filepath.Join(dest, "program.yaml")
		if _, err := os.Stat(manifest); err != nil {
			t.Fatalf("scaffold did not install the manifest: %v", err)
		}

		// A second run must report unchanged and write nothing new.
		again := mustRunADB(t, ws, "program", "scaffold", "operations")
		if strings.Contains(again.stdout, "installed") {
			t.Errorf("re-scaffold should be unchanged, not installed:\n%s", again.stdout)
		}

		// A local edit survives; --force overwrites it.
		if err := os.WriteFile(manifest, []byte("EDITED\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustRunADB(t, ws, "program", "scaffold", "operations")
		if b, _ := os.ReadFile(manifest); string(b) != "EDITED\n" {
			t.Error("re-scaffold clobbered a local edit without --force")
		}
		mustRunADB(t, ws, "program", "scaffold", "operations", "--force")
		if b, _ := os.ReadFile(manifest); string(b) == "EDITED\n" {
			t.Error("--force did not overwrite the edited file")
		}

		// The newly scaffolded pack is fully usable through the gate.
		rows := programStatus(t, ws, "operations")
		if len(rows) != 5 {
			t.Errorf("operations has %d templates, want 5", len(rows))
		}
		for _, r := range rows {
			// operations is entirely manual/event triggered, so nothing is ready.
			if r.State == "ready" {
				t.Errorf("operations template %s is ready, want pending (trigger %s)", r.ID, r.Trigger)
			}
		}
	})

	t.Run("unknown ids fail with an actionable message", func(t *testing.T) {
		res := runADB(t, ws, "program", "status", "no-such-program")
		if res.err == nil {
			t.Fatal("an unknown program should exit non-zero")
		}
		if !strings.Contains(res.combined(), "adb program list") {
			t.Errorf("the error should point at `adb program list`:\n%s", res.combined())
		}
	})
}

// --- init update × the four manifest states --------------------------------

// TestE2E_InitUpdateManifestStates drives `adb init update` at the binary level
// across every state the manifest's program answers can be in. The states are
// unit-covered in internal/core; what this adds is the real command, the real
// files, and the post-apply idempotency that only shows up on a second run.
func TestE2E_InitUpdateManifestStates(t *testing.T) {
	t.Run("with_programs re-renders exactly the recorded packs", func(t *testing.T) {
		ws := newProject(t, "--with-program", defaultPack, "--with-program", "operations")

		m := readManifest(t, ws)
		got := append([]string{}, m.Options.WithPrograms...)
		sort.Strings(got)
		if strings.Join(got, ",") != "operations,technical-design" {
			t.Fatalf("manifest with_programs = %v, want both packs", m.Options.WithPrograms)
		}
		for _, pack := range []string{defaultPack, "operations"} {
			if _, err := os.Stat(filepath.Join(ws, "programs", pack, "program.yaml")); err != nil {
				t.Errorf("pack %s was not provisioned: %v", pack, err)
			}
		}

		// Right after init, an update has nothing to do for either pack.
		res := mustRunADB(t, ws, "init", "update", ".")
		groups := updateGroups(res.stdout)
		for _, pack := range []string{defaultPack, "operations"} {
			if extra := packFilesIn(groups["added"], pack); len(extra) > 0 {
				t.Errorf("update wants to add %d files for the already-provisioned pack %s: %v",
					len(extra), pack, extra)
			}
		}
		// Both packs' files are known to the update, as unchanged.
		for _, pack := range []string{defaultPack, "operations"} {
			if unchanged := packFilesIn(groups["unchanged"], pack); len(unchanged) == 0 {
				t.Errorf("update does not track pack %s at all (groups: %v)", pack, groups)
			}
		}
	})

	t.Run("no_programs adds nothing", func(t *testing.T) {
		ws := newProject(t, "--no-programs")

		m := readManifest(t, ws)
		if !m.Options.NoPrograms {
			t.Fatal("manifest should record no_programs: true")
		}
		if len(m.Options.WithPrograms) != 0 {
			t.Errorf("with_programs = %v, want empty under an opt-out", m.Options.WithPrograms)
		}
		if _, err := os.Stat(filepath.Join(ws, "programs")); !os.IsNotExist(err) {
			t.Errorf("--no-programs still created a programs/ directory (err=%v)", err)
		}

		// The opt-out is honoured on update: no pack file is ever offered.
		res := mustRunADB(t, ws, "init", "update", ".")
		for group, files := range updateGroups(res.stdout) {
			for _, f := range files {
				if strings.HasPrefix(f, "programs/") {
					t.Errorf("update offered pack file %q in group %q despite no_programs", f, group)
				}
			}
		}

		// And --apply writes none, leaving the opt-out intact for the next update.
		mustRunADB(t, ws, "init", "update", ".", "--apply")
		if _, err := os.Stat(filepath.Join(ws, "programs")); !os.IsNotExist(err) {
			t.Errorf("--apply provisioned a pack despite no_programs (err=%v)", err)
		}
		if !readManifest(t, ws).Options.NoPrograms {
			t.Error("--apply dropped the no_programs opt-out from the manifest")
		}
	})

	t.Run("a pre-programs workspace is retro-provisioned, then idempotent", func(t *testing.T) {
		ws := newProject(t)
		makeLegacy(t, ws)

		// Dry run: the pack's files show as `added`, so they are visible BEFORE
		// anything is written and --apply is the explicit consent.
		plan := mustRunADB(t, ws, "init", "update", ".")
		added := packFilesIn(updateGroups(plan.stdout)["added"], defaultPack)
		if len(added) != defaultPackFiles {
			t.Fatalf("dry-run planned %d pack files as added, want %d:\n%s",
				len(added), defaultPackFiles, plan.stdout)
		}
		if !strings.Contains(plan.stdout, "--apply") {
			t.Errorf("dry-run should tell the user to re-run with --apply:\n%s", plan.stdout)
		}
		if _, err := os.Stat(filepath.Join(ws, "programs", defaultPack, "program.yaml")); !os.IsNotExist(err) {
			t.Fatalf("the dry run wrote the pack (err=%v)", err)
		}

		// Apply: files land, and the RESOLVED answers are persisted.
		mustRunADB(t, ws, "init", "update", ".", "--apply")
		if _, err := os.Stat(filepath.Join(ws, "programs", defaultPack, "program.yaml")); err != nil {
			t.Fatalf("--apply did not retro-provision the pack: %v", err)
		}
		m := readManifest(t, ws)
		if strings.Join(m.Options.WithPrograms, ",") != defaultPack {
			t.Errorf("after retro-provisioning, with_programs = %v, want [%s] — "+
				"without this write-back the manifest keeps claiming it has no packs",
				m.Options.WithPrograms, defaultPack)
		}

		// The second update must be a clean no-op. This is the property the
		// answers write-back exists for: without it the same 9 files would be
		// offered as `added` forever.
		second := mustRunADB(t, ws, "init", "update", ".")
		if again := packFilesIn(updateGroups(second.stdout)["added"], defaultPack); len(again) > 0 {
			t.Errorf("a second update re-offers %d pack files: %v", len(again), again)
		}
		if !strings.Contains(second.stdout, "Already up to date") {
			t.Errorf("a settled workspace should report it is up to date:\n%s", second.stdout)
		}

		// The retro-provisioned pack works: the gate resolves and next's path exists.
		rows := programStatus(t, ws, defaultPack)
		if got := stateOf(t, rows, "business-requirements"); got != "ready" {
			t.Errorf("retro-provisioned pack: business-requirements = %q, want ready", got)
		}
	})

	t.Run("applying twice is a no-op", func(t *testing.T) {
		ws := newProject(t)
		mustRunADB(t, ws, "init", "update", ".", "--apply")
		before := readManifest(t, ws)
		second := mustRunADB(t, ws, "init", "update", ".", "--apply")
		after := readManifest(t, ws)

		if len(updateGroups(second.stdout)["added"]) > 0 {
			t.Errorf("a second --apply added files:\n%s", second.stdout)
		}
		if before.TemplateVersion != after.TemplateVersion {
			t.Errorf("template version drifted across identical applies: %s → %s",
				before.TemplateVersion, after.TemplateVersion)
		}
		if len(before.Files) != len(after.Files) {
			t.Errorf("baseline file count changed across identical applies: %d → %d",
				len(before.Files), len(after.Files))
		}
		if strings.Join(before.Options.WithPrograms, ",") != strings.Join(after.Options.WithPrograms, ",") {
			t.Errorf("recorded packs drifted: %v → %v", before.Options.WithPrograms, after.Options.WithPrograms)
		}

	})
}

// makeLegacy rewrites a project to look like one scaffolded BEFORE document
// programs existed: no packs on disk, no pack files in the provenance baseline,
// and — the load-bearing part — neither `with_programs` nor `no_programs`
// recorded. That absence is what the retro-provisioning path keys on.
func makeLegacy(t *testing.T, ws string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(ws, "programs")); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, ws)
	for rel := range m.Files {
		if strings.HasPrefix(rel, "programs/") {
			delete(m.Files, rel)
		}
	}
	m.Options.WithPrograms = nil
	m.Options.NoPrograms = false
	writeManifest(t, ws, m)

	// Guard the fixture itself: if a future manifest change made the absence
	// unrepresentable, this test would silently stop testing retro-provisioning.
	reread := readManifest(t, ws)
	if len(reread.Options.WithPrograms) != 0 || reread.Options.NoPrograms {
		t.Fatalf("legacy fixture did not persist as a pre-programs manifest: %+v", reread.Options)
	}
}

// --- external packs and config ---------------------------------------------

// TestE2E_ExternalPackViaConfig proves the `programs_search_paths` custom setting
// resolves through the real config tiers in a separate process — the seam that
// lets a private, non-redistributable pack live entirely outside this repo.
func TestE2E_ExternalPackViaConfig(t *testing.T) {
	ws := newProject(t)
	packs := filepath.Join(t.TempDir(), "private-packs")
	writeExternalPack(t, filepath.Join(packs, "house-style"))

	appendTaskrcSetting(t, ws, "programs_search_paths", packs)

	// The setting resolves, and names the tier it came from.
	cfg := mustRunADB(t, ws, "config", "get", "programs_search_paths", "--source")
	if !strings.Contains(cfg.stdout, packs) {
		t.Fatalf("config get did not resolve the search path:\n%s", cfg.stdout)
	}

	res := mustRunADB(t, ws, "program", "list", "--json")
	var rows []programListRow
	if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
		t.Fatalf("list --json: %v\n%s", err, res.stdout)
	}
	var found bool
	for _, r := range rows {
		if r.ID != "house-style" {
			continue
		}
		found = true
		if r.Error != "" {
			t.Errorf("external pack is unreadable: %s", r.Error)
		}
		if r.Source != packs {
			t.Errorf("external pack source = %q, want the search-path dir %q", r.Source, packs)
		}
	}
	if !found {
		t.Fatalf("an external pack on a configured search path did not list:\n%s", res.stdout)
	}

	// It is fully usable: status classifies it and the gate keys on the workspace.
	rows2 := programStatus(t, ws, "house-style")
	if len(rows2) != 1 || rows2[0].State != "ready" {
		t.Fatalf("external pack status = %+v, want one ready template", rows2)
	}
	writeFileIn(t, ws, rows2[0].Output, "# Charter\n")
	if got := stateOf(t, programStatus(t, ws, "house-style"), "charter"); got != "generated" {
		t.Errorf("external pack template = %q, want generated after drafting", got)
	}
}

// TestE2E_ExternalPackFromPathResolves pins the `from:` contract for an EXTERNAL
// pack, at the level a user hits it.
//
// `adb program next` used to anchor `from:` at programs/<id>/ unconditionally —
// the PROVISIONED location. That is right once a pack is provisioned (init does it
// for the default pack, `program scaffold` for any other), but an external pack
// discovered through `programs_search_paths` and never scaffolded has nothing
// there, while its template demonstrably sits at
// <search-path>/<id>/<template-path>. The path printed for it therefore resolved
// nowhere — the same class of bug 9134a4f fixed for the pack-relative case, in a
// case that fix did not cover.
//
// Both halves are asserted here because they are one contract: an unprovisioned
// external pack prints the absolute search-path location, and once scaffolded the
// same command switches to the workspace-relative form.
func TestE2E_ExternalPackFromPathResolves(t *testing.T) {
	ws := newProject(t)
	packs := filepath.Join(t.TempDir(), "private-packs")
	packRoot := filepath.Join(packs, "house-style")
	writeExternalPack(t, packRoot)
	appendTaskrcSetting(t, ws, "programs_search_paths", packs)

	// The template really is on disk — at the search path, not under programs/.
	realTemplate := filepath.Join(packRoot, "templates", "draft", "charter.md")
	if _, err := os.Stat(realTemplate); err != nil {
		t.Fatalf("fixture is wrong, the external template should exist: %v", err)
	}

	from := nextFromPath(t, ws, "house-style")
	if !filepath.IsAbs(from) {
		t.Fatalf("from: %q should be the ABSOLUTE search-path location for an "+
			"unprovisioned external pack; there is no workspace-relative form for it", from)
	}
	if _, err := os.Stat(from); err != nil {
		t.Errorf("from: %q does not exist: %v", from, err)
	}
	if from != realTemplate {
		t.Errorf("from: %q, want the template's real location %q", from, realTemplate)
	}

	// Once the pack is provisioned into the workspace, `from:` switches to the
	// workspace-relative form, which resolves from the root `write:`/`reads:` are
	// anchored to.
	mustRunADB(t, ws, "program", "scaffold", "house-style")
	after := nextFromPath(t, ws, "house-style")
	if filepath.IsAbs(after) {
		t.Fatalf("after scaffolding, from: %q should be workspace-relative", after)
	}
	if want := "programs/house-style/templates/draft/charter.md"; after != want {
		t.Errorf("after scaffolding, from: %q, want %q", after, want)
	}
	if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(after))); err != nil {
		t.Errorf("from: %q does not resolve from the workspace root: %v", after, err)
	}
}

// nextFromPath runs `adb program next <pack>` and returns the single `from:` value
// it printed, failing if there is not exactly one.
func nextFromPath(t *testing.T, ws, pack string) string {
	t.Helper()
	res := mustRunADB(t, ws, "program", "next", pack)
	var froms []string
	for _, line := range strings.Split(res.stdout, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "from:") {
			froms = append(froms, strings.TrimSpace(strings.TrimPrefix(trimmed, "from:")))
		}
	}
	if len(froms) != 1 {
		t.Fatalf("want exactly one from: path for %s, got %d:\n%s", pack, len(froms), res.stdout)
	}
	return froms[0]
}

// TestE2E_DottedConfigKeyIsTolerated pins the config-tolerance contract at the
// level an operator hits it: a mis-shaped `custom_settings:` block must never cost
// you the command you were running.
//
// This used to be TestE2E_DottedConfigKeyFailsLegibly, which characterized the
// opposite behaviour. `custom_settings` is a flat map[string]string and Viper treats
// "." as a key-NESTING delimiter, so `programs.search_paths:` reached mapstructure
// as custom_settings[programs] = map[string]any{…} and the tier failed to unmarshal.
// Config load happens at app init, so that ONE key killed every adb command in the
// workspace — not just `adb program`. internal/core/config.go's
// normalizeCustomSettings now flattens each tier back to dotted keys before it is
// decoded, which makes all of this non-fatal.
//
// Four shapes, one contract, asserted against the real binary because the failure it
// replaced only ever appeared in a separate process resolving its own config tiers:
//
//   - the canonical underscore spelling (the one to write) resolves;
//   - a dotted key resolves IDENTICALLY, rather than being fatal;
//   - a genuinely nested mapping resolves too — it flattens to the same dotted key;
//   - a value that cannot be a string (a YAML list) is SKIPPED with a warning, and
//     the warning goes to stderr so `--json` on stdout still parses.
//
// Every case also asserts a sibling setting in the same block still resolves: one
// unusable entry must not cost you the others, the other tiers, or the command.
func TestE2E_DottedConfigKeyIsTolerated(t *testing.T) {
	// The pack fixture writeExternalPack installs, and a phase-triggered template it
	// declares — the pack showing up is what proves the setting actually RESOLVED,
	// as opposed to config load merely surviving.
	const (
		extPack     = "house-style"
		extTemplate = "charter"
	)

	// NOTE for anyone adding a row: keep COMMAS out of the subtest name. t.TempDir()
	// derives its directory from the test name, and `programs_search_paths` is a
	// comma-or-PATH-separated LIST — so a comma in the name splits the fixture's own
	// search path in half and the pack silently stops resolving.
	tests := []struct {
		name string
		// settingYAML is spliced verbatim into custom_settings:, with one %s for the
		// search-path directory. Raw YAML because two of these shapes are not
		// scalars, so appendTaskrcSetting cannot express them.
		settingYAML string
		// wantPack is whether the external pack must be discovered. False means the
		// setting was legitimately skipped — the command must still work.
		wantPack bool
		// wantWarning is a substring the stderr warning must carry. Empty means
		// stderr must stay quiet. It names the offending KEY rather than quoting the
		// reason: which key to fix is the contract this test owns, while the exact
		// wording of "why" belongs to internal/core/config.go and may be reworded
		// without breaking any consumer.
		wantWarning string
	}{
		{
			name:        "canonical underscore spelling",
			settingYAML: "  programs_search_paths: \"%s\"\n",
			wantPack:    true,
		},
		{
			name: "dotted key is flattened rather than fatal",
			// The exact spelling that used to take the workspace down.
			settingYAML: "  programs.search_paths: \"%s\"\n",
			wantPack:    true,
		},
		{
			name: "nested mapping flattens to the same dotted key",
			// Viper's own nesting, written out longhand. flattenCustomSettingsInto
			// rejoins it with "." — the round trip is lossless.
			settingYAML: "  programs:\n    search_paths: \"%s\"\n",
			wantPack:    true,
		},
		{
			name: "a list value is skipped with a warning",
			// customSettingValue has no single-value spelling for a sequence, so this
			// entry is dropped. Two items, because the plausible operator error is
			// "surely this takes a list".
			settingYAML: "  programs_search_paths:\n    - \"%s\"\n    - /tmp/adb-e2e-second-path\n",
			wantPack:    false,
			wantWarning: `skipping custom setting "programs_search_paths"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := newProject(t)
			packs := filepath.Join(t.TempDir(), "private-packs")
			writeExternalPack(t, filepath.Join(packs, extPack))
			appendTaskrcCustomSettings(t, ws,
				fmt.Sprintf(tt.settingYAML, packs)+"  sibling_setting: \"kept\"\n")

			// 1. The command SUCCEEDS. This is the whole point: the old behaviour was
			//    a non-zero exit from config load, before any program logic ran.
			res := runADB(t, ws, "program", "list", "--json")
			if res.err != nil {
				t.Fatalf("`adb program list` must not fail on this custom_settings shape "+
					"(exit %d): %v\nstdout:\n%s\nstderr:\n%s",
					res.exitCode, res.err, res.stdout, res.stderr)
			}

			// 2. stdout is clean, parseable JSON. A scripted consumer must be able to
			//    read it even when the config carried a diagnostic.
			var rows []programListRow
			if err := json.Unmarshal([]byte(res.stdout), &rows); err != nil {
				t.Fatalf("stdout is not parseable JSON, so a warning leaked into it: %v\n%s",
					err, res.stdout)
			}

			// 3. The diagnostic, if any, is on stderr — and never a panic.
			if tt.wantWarning == "" {
				if strings.Contains(res.stderr, "Warning:") {
					t.Errorf("a well-formed setting warned anyway:\n%s", res.stderr)
				}
			} else {
				if !strings.Contains(res.stderr, tt.wantWarning) {
					t.Errorf("stderr does not name the skipped setting %q:\n%s",
						tt.wantWarning, res.stderr)
				}
				// The warning has to name the file to edit, or it is not actionable.
				if !strings.Contains(res.stderr, ".taskrc") {
					t.Errorf("the warning does not name the config file to fix:\n%s", res.stderr)
				}
				// …and say what was wrong with the value, without pinning the phrasing.
				if !strings.Contains(res.stderr, "list") {
					t.Errorf("the warning does not say the value was a list:\n%s", res.stderr)
				}
			}
			if out := res.combined(); strings.Contains(out, "goroutine ") || strings.Contains(out, "panic:") {
				t.Errorf("a mis-shaped config key panicked instead of degrading:\n%s", out)
			}

			// 4. The shipped packs are always there — config tolerance must not come
			//    at the cost of discovery working at all.
			byID := map[string]programListRow{}
			for _, r := range rows {
				if r.Error != "" {
					t.Errorf("pack %s is unreadable: %s", r.ID, r.Error)
				}
				byID[r.ID] = r
			}
			if _, ok := byID[defaultPack]; !ok {
				t.Errorf("list omits the shipped pack %q:\n%s", defaultPack, res.stdout)
			}

			// 5. The setting RESOLVED (or was legitimately skipped).
			ext, found := byID[extPack]
			switch {
			case tt.wantPack && !found:
				t.Fatalf("the search path did not resolve — %q is absent from list:\n%s",
					extPack, res.stdout)
			case tt.wantPack && ext.Source != packs:
				t.Errorf("%s source = %q, want the configured search-path dir %q",
					extPack, ext.Source, packs)
			case !tt.wantPack && found:
				t.Errorf("%q listed from %q, but the setting carrying that path was skipped",
					extPack, ext.Source)
			}

			// 6. A resolved pack is fully USABLE, not merely listed: the gate has to
			//    classify it against this workspace.
			if tt.wantPack {
				if got := stateOf(t, programStatus(t, ws, extPack), extTemplate); got != "ready" {
					t.Errorf("external pack template %s = %q, want ready", extTemplate, got)
				}
			}

			// 7. The rest of the block survived. Under the old behaviour a single bad
			//    key took every setting in the file with it.
			sib := mustRunADB(t, ws, "config", "get", "sibling_setting", "--source")
			if !strings.Contains(sib.stdout, "kept") {
				t.Errorf("a sibling custom setting did not survive this shape:\n%s", sib.stdout)
			}
		})
	}
}

// writeExternalPack writes a minimal valid pack (manifest + one template) at dir.
func writeExternalPack(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "templates", "draft"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `program:
  id: house-style
  name: House Style
  lineage: ["an internal house standard"]
phases:
  - {id: draft, name: Draft}
templates:
  - id: charter
    name: Charter
    phase: draft
    path: templates/draft/charter.md
    output: docs/house-style/draft/charter.md
    human_review:
      required: true
      note: "A human owns the charter."
      risk: "An unowned charter is not a charter."
`
	if err := os.WriteFile(filepath.Join(dir, "program.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "draft", "charter.md"),
		[]byte("---\nsources: []\n---\n# Charter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// appendTaskrcSetting adds one custom_settings entry to a project's .taskrc (the
// repo tier), creating the block if the scaffolded file has none.
func appendTaskrcSetting(t *testing.T, ws, key, value string) {
	t.Helper()
	// A quoted scalar keeps a Windows path or a comma-bearing value intact.
	appendTaskrcCustomSettings(t, ws, "  "+key+": \""+value+"\"\n")
}

// appendTaskrcCustomSettings splices raw, already-indented YAML lines into a
// project's .taskrc custom_settings: block, creating the block if the scaffolded
// file has none.
//
// It exists because appendTaskrcSetting can only express a QUOTED SCALAR, and the
// config-tolerance cases above deliberately write shapes that are not scalars at all
// — a nested mapping and a sequence. Passing the YAML through verbatim is the only
// way to author those from a test; there is no key/value pair to hand over.
func appendTaskrcCustomSettings(t *testing.T, ws, lines string) {
	t.Helper()
	path := filepath.Join(ws, ".taskrc")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .taskrc: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "custom_settings:") {
		text += "\ncustom_settings:\n"
	} else if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text = strings.Replace(text, "custom_settings:\n", "custom_settings:\n"+lines, 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write .taskrc: %v", err)
	}
}
