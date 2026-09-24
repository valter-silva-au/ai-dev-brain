package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/core"
	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"github.com/valter-silva-au/ai-dev-brain/templates"
)

// This file is the CLI surface for DOCUMENT PROGRAMS (TASK-00023): phase-ordered
// packs of artifact templates declared by a `program.yaml` manifest. Every handler
// here is thin — resolve the pack, call into internal/core, print. All of the
// engine (loading, validation, the presence-based dependency gate, provenance
// tracing) lives in internal/core/program.go, mirroring how `adb compliance` and
// `adb gtm` delegate to the shared pack scaffolder.

// NewProgramCmd creates the `adb program` command group.
func NewProgramCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "program",
		Short: "Phase-ordered document programs (artifact template packs)",
		Long: `Work with document programs: phase-ordered packs of artifact templates.

A program is DATA — a program.yaml manifest plus its templates — so adding one is a
pack you drop in, never a code change. Each template declares the artifact it
produces and the upstream templates it requires; a requirement is satisfied when the
upstream artifact FILE EXISTS in the workspace (the same presence-based evidence rule
the founder-playbook stage gates use).

External packs are discovered through the ` + "`programs_search_paths`" + ` custom
config setting (comma-separated directories, resolved via the usual
Repo > Org > Global precedence). An embedded pack always wins an id collision.`,
	}
	cmd.AddCommand(
		newProgramListCmd(),
		newProgramShowCmd(),
		newProgramScaffoldCmd(),
		newProgramStatusCmd(),
		newProgramNextCmd(),
		newProgramTraceCmd(),
	)
	return cmd
}

// --- pack resolution -------------------------------------------------------

// programSource locates one program pack: an fs.FS plus the packs ROOT within it,
// so callers can hand (fsys, root, id) straight to core.LoadProgram or
// core.ScaffoldProgram without caring whether the pack is embedded or external.
type programSource struct {
	fsys fs.FS
	root string
	id   string
	// external names the on-disk search-path directory the pack came from ("" for
	// an embedded pack), purely so output can say where a definition lives.
	external string
}

func (s programSource) load() (*models.Program, error) {
	return core.LoadProgram(s.fsys, path.Join(s.root, s.id))
}

// resolveProgramSource finds a program by id: the embedded packs first — the
// shipped pack is the reference definition and wins an id collision, matching
// core.ListPrograms — then each configured search path in order.
func resolveProgramSource(id string) (programSource, error) {
	if strings.TrimSpace(id) == "" {
		return programSource{}, fmt.Errorf("program id is required")
	}
	candidates := []programSource{{fsys: templates.FS, root: core.ProgramsRoot, id: id}}
	for _, dir := range programSearchPaths() {
		candidates = append(candidates, programSource{fsys: os.DirFS(dir), root: ".", id: id, external: dir})
	}
	for _, c := range candidates {
		_, err := c.load()
		switch {
		case err == nil:
			return c, nil
		case errors.Is(err, fs.ErrNotExist):
			continue // not in this location — try the next
		default:
			// The pack IS here but its manifest is malformed. Report that, rather than
			// falling through and claiming the program does not exist.
			return programSource{}, err
		}
	}
	return programSource{}, fmt.Errorf("unknown program %q (run `adb program list`)", id)
}

// programSearchPathsKeys are the custom-setting SPELLINGS consulted for external
// pack directories, most-preferred first. They are spellings of one setting, not
// two settings — see programSearchPathsSetting for how they combine with the
// config tiers.
//
// `programs_search_paths` is the CANONICAL spelling. The dotted
// `programs.search_paths` exists because Viper — which parses every tier — treats
// "." as a key-NESTING delimiter, so that spelling used to decode as a nested map
// and fail to unmarshal into the flat map[string]string that CustomSettings is.
// That is no longer true: `normalizeCustomSettings` (internal/core/config.go) now
// flattens every tier's `custom_settings:` block back to dotted keys before it is
// decoded, so a dotted (or genuinely nested) spelling loads fine and reaches the
// merged config intact — which is exactly what makes this second entry a LIVE
// fallback rather than the dead code it was.
//
// Flattening normalizes the shape, not the name: the two spellings remain DISTINCT
// keys in the merged config, so both have to be looked up.
var programSearchPathsKeys = []string{"programs_search_paths", "programs.search_paths"}

// configTier is one layer of the merged config's free-form settings, paired with
// the tier name `adb config get --source` would report for it.
type configTier struct {
	name     string
	settings map[string]string
}

// customSettingTiers lists the merged config's tiers MOST SPECIFIC FIRST — the
// Repo > Org > Global order the whole config surface promises. The org tier is
// absent when no org is active (the historical two-tier merge).
//
// It exists because models.MergedConfig.SettingSource resolves ONE key across all
// three tiers and hands back only the winner, which is the wrong shape for a
// setting that has more than one accepted spelling (below). The per-tier maps are
// exported fields, so this needs nothing from pkg/models.
func customSettingTiers(mc *models.MergedConfig) []configTier {
	if mc == nil {
		return nil
	}
	var out []configTier
	if mc.Repo != nil {
		out = append(out, configTier{name: "repo", settings: mc.Repo.CustomSettings})
	}
	if mc.Org != nil {
		out = append(out, configTier{name: "org", settings: mc.Org.CustomSettings})
	}
	if mc.Global != nil {
		out = append(out, configTier{name: "global", settings: mc.Global.CustomSettings})
	}
	return out
}

// programSearchPathsSetting resolves the search-paths setting TIER-MAJOR: every
// accepted spelling is tried at the repo tier, then every spelling at the org tier,
// then every spelling at the global tier. Within a tier the canonical spelling
// beats the dotted one. tier names the winner, "" when no tier defines either.
//
// The nesting order is the whole point, and getting it backwards was a real defect.
// Looping key-major over models.MergedConfig.SettingSource — which itself resolves
// across all three tiers — means the FIRST SPELLING WINS AT WHATEVER TIER IT SITS,
// so a canonical key in `.taskconfig` (global) beat a dotted key in `.taskrc`
// (repo) and the documented "most-specific tier wins" promise silently inverted.
// A spelling preference must never outrank a tier preference: a repo saying
// something is more specific than a global saying it, however either spelled it.
//
// ok is true even for an EMPTY value, matching SettingSource: a tier that sets the
// key to "" is deliberately saying "no search paths", and must not fall through to
// a less-specific tier that sets some.
func programSearchPathsSetting() (value, tier string, ok bool) {
	if App == nil {
		return "", "", false
	}
	for _, t := range customSettingTiers(App.MergedConfig) {
		for _, key := range programSearchPathsKeys {
			if v, found := t.settings[key]; found {
				return v, t.name, true
			}
		}
	}
	return "", "", false
}

// programSearchPaths resolves the external-pack search paths through the layered
// config, so external packs are configured, never hard-coded. The rule lives in
// core.ProgramSearchPaths (shared with the MCP tools so both entries resolve
// identically); this wrapper supplies the App's merged config and base path.
func programSearchPaths() []string {
	if App == nil {
		return nil
	}
	return core.ProgramSearchPaths(App.MergedConfig, App.BasePath)
}

// --- list ------------------------------------------------------------------

// programListJSON is the stable `--json` row for `adb program list`.
type programListJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Phases      int    `json:"phases"`
	Templates   int    `json:"templates"`
	Source      string `json:"source"` // "embedded" or the search-path directory
	Error       string `json:"error,omitempty"`
}

func newProgramListCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available document programs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			ids, err := core.ListPrograms(templates.FS, core.ProgramsRoot, programSearchPaths())
			if err != nil {
				return fmt.Errorf("failed to list programs: %w", err)
			}

			rows := make([]programListJSON, 0, len(ids))
			for _, id := range ids {
				row := programListJSON{ID: id, Source: "embedded"}
				src, err := resolveProgramSource(id)
				if err != nil {
					// A pack that lists but will not load still gets a row: a broken
					// external pack must be visible, not silently absent.
					row.Error = err.Error()
					rows = append(rows, row)
					continue
				}
				if src.external != "" {
					row.Source = src.external
				}
				p, err := src.load()
				if err != nil {
					row.Error = err.Error()
					rows = append(rows, row)
					continue
				}
				row.Name = p.Program.Name
				row.Description = collapseSpace(p.Program.Description)
				row.Phases = len(p.Phases)
				row.Templates = len(p.Templates)
				rows = append(rows, row)
			}

			if jsonOutput {
				return printJSON(rows)
			}
			if len(rows) == 0 {
				fmt.Println("No programs available.")
				return nil
			}
			width := 0
			for _, r := range rows {
				if len(r.ID) > width {
					width = len(r.ID)
				}
			}
			for _, r := range rows {
				if r.Error != "" {
					fmt.Printf("  %-*s  (unreadable: %s)\n", width, r.ID, r.Error)
					continue
				}
				fmt.Printf("  %-*s  %-24s %2d templates · %d phases\n", width, r.ID, r.Name, r.Templates, r.Phases)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// collapseSpace folds newlines and runs of whitespace in a manifest description
// into single spaces so a folded YAML block prints on one line.
func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- show ------------------------------------------------------------------

func newProgramShowCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "show <program-id>",
		Short: "Show a program's phases, templates, and lineage",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			src, err := resolveProgramSource(args[0])
			if err != nil {
				return err
			}
			p, err := src.load()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(p)
			}

			fmt.Printf("%s — %s", p.Program.ID, p.Program.Name)
			if p.Program.Version > 0 {
				fmt.Printf(" (v%d)", p.Program.Version)
			}
			fmt.Println()
			// `show` renders the MANIFEST, so its heading is the manifest's declared
			// id — that is the thing being displayed. But pack resolution keys on the
			// DIRECTORY name, so when the two disagree the heading names an id no
			// subcommand accepts. Say which one to type. Shipped packs never hit this
			// (the provenance guard requires directory == manifest id), so their output
			// is unchanged.
			if src.id != p.Program.ID {
				fmt.Printf("(addressed as %q — the pack directory, which is what every "+
					"`adb program` subcommand resolves)\n", src.id)
			}
			if desc := collapseSpace(p.Program.Description); desc != "" {
				fmt.Printf("%s\n", desc)
			}
			if len(p.Program.Lineage) > 0 {
				fmt.Println("\nLineage:")
				for _, src := range p.Program.Lineage {
					fmt.Printf("  - %s\n", src)
				}
			}
			for _, phase := range p.Phases {
				fmt.Printf("\n%s — %s\n", phase.ID, phase.Name)
				for _, t := range p.Templates {
					if t.Phase != phase.ID {
						continue
					}
					fmt.Printf("  %s (%s)\n", t.ID, t.Name)
					fmt.Printf("      output:   %s\n", t.Output)
					requires := "—"
					if len(t.Requires) > 0 {
						requires = fmt.Sprintf("%s (%s)", strings.Join(t.Requires, ", "), t.EffectiveRequiresMode())
					}
					fmt.Printf("      trigger:  %s    requires: %s\n", t.EffectiveTrigger(), requires)
					if t.HumanReview != nil {
						fmt.Printf("      review:   %s\n", humanReviewLine(t.HumanReview))
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the manifest as JSON")
	return cmd
}

// humanReviewLine renders a template's mandatory human-review contract on one
// line. It is surfaced everywhere a template is listed because these artifacts are
// GENERATED — the note says what to check, the risk says what it costs to skip.
func humanReviewLine(hr *models.HumanReview) string {
	if hr == nil {
		return "not declared"
	}
	label := "optional"
	if hr.Required {
		label = "REQUIRED"
	}
	if hr.Note != "" {
		label += " — " + collapseSpace(hr.Note)
	}
	return label
}

// --- scaffold --------------------------------------------------------------

// programScaffoldJSON is the stable `--json` row for `adb program scaffold`. It
// exists for the same reason programTemplateJSON does — the CLI owns its own flat
// projection rather than marshalling a core type — plus one thing the core type
// cannot carry: whether the row is a PLAN or a write.
//
// Without DryRun a scripted consumer cannot tell the two apart, because a planned
// install and a real one both report action "installed"; only the human view says
// "Would scaffold".
type programScaffoldJSON struct {
	Name   string `json:"name"`
	Dest   string `json:"dest"`
	Action string `json:"action"`
	DryRun bool   `json:"dry_run"`
}

// scaffoldRows projects the core scaffold entries into the CLI's `--json` rows,
// stamping each with the dry-run marker. The slice is always non-nil so an
// empty result marshals as `[]` rather than `null`.
func scaffoldRows(entries []core.PackScaffoldEntry, dryRun bool) []programScaffoldJSON {
	out := make([]programScaffoldJSON, 0, len(entries))
	for _, e := range entries {
		out = append(out, programScaffoldJSON{
			Name:   e.Name,
			Dest:   e.Dest,
			Action: string(e.Action),
			DryRun: dryRun,
		})
	}
	return out
}

func newProgramScaffoldCmd() *cobra.Command {
	var (
		dryRun bool
		force  bool
		json   bool
	)
	cmd := &cobra.Command{
		Use:   "scaffold <program-id> [dest]",
		Short: "Scaffold a program pack (manifest + templates) into the workspace",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			src, err := resolveProgramSource(args[0])
			if err != nil {
				return err
			}
			destDir := filepath.Join(App.BasePath, "programs", src.id)
			if len(args) == 2 {
				destDir = args[1]
			}
			entries, err := core.ScaffoldProgram(src.fsys, src.root, src.id, destDir,
				core.HarnessInstallOptions{DryRun: dryRun, Force: force})
			if err != nil {
				return fmt.Errorf("failed to scaffold %s: %w", src.id, err)
			}
			if json {
				return printJSON(scaffoldRows(entries, dryRun))
			}
			verb := "Scaffolded"
			if dryRun {
				verb = "Would scaffold"
			}
			fmt.Printf("%s %s into %s:\n", verb, src.id, destDir)
			for _, e := range entries {
				fmt.Printf("  %-10s %s\n", e.Action, e.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite locally-edited files")
	cmd.Flags().BoolVar(&json, "json", false, "output the scaffold entries as JSON")
	return cmd
}

// --- status / next ---------------------------------------------------------

// programTemplateJSON is the stable `--json` row for `adb program status` and
// `adb program next`. It is a FLAT projection rather than core.TemplateState,
// which nests the whole manifest template: a scripted consumer should not have to
// track manifest-shape changes to read a status.
//
// The three `template_*` fields answer "where do I READ the template", the mirror of
// the `output`/`output_path` pair's "where do I WRITE the artifact". They exist
// because the row previously carried no template location at all: the human `from:`
// line was resolved while the machine view — which is what an agent reads, i.e. the
// primary consumer — had nothing resolvable to go on.
//
//   - template_path is the RAW pack-relative `path` exactly as the manifest declares
//     it, and exactly as `show --json` reports it under `templates[].path`. It is
//     shipped alongside the resolved form for the same reason `output` is shipped
//     alongside `output_path`: a consumer joining a status row to a manifest needs
//     the manifest's own spelling, and on its own it resolves nowhere.
//   - template_from is the RESOLVED location — the same value the human view prints
//     as `from:`, minus the not-provisioned prose (template_source carries that).
//   - template_source says which branch produced it (see templateLocationKind), so a
//     consumer can tell "read this now" from "this is where it would be".
type programTemplateJSON struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Phase          string               `json:"phase"`
	PhaseName      string               `json:"phase_name,omitempty"`
	State          string               `json:"state"`
	Output         string               `json:"output"`
	OutputPath     string               `json:"output_path"`
	TemplatePath   string               `json:"template_path"`
	TemplateFrom   string               `json:"template_from"`
	TemplateSource templateLocationKind `json:"template_source"`
	Trigger        string               `json:"trigger"`
	Requires       []string             `json:"requires,omitempty"`
	BlockedBy      []string             `json:"blocked_by,omitempty"`
	HumanReview    *models.HumanReview  `json:"human_review,omitempty"`
	Detail         string               `json:"detail,omitempty"`
}

// templateRows projects the gate's states into the `--json` rows. It takes the
// resolved programSource — not just the states — because a template's location
// depends on WHERE THE PACK CAME FROM, which core.TemplateState does not know.
func templateRows(src programSource, states []core.TemplateState) []programTemplateJSON {
	out := make([]programTemplateJSON, 0, len(states))
	for _, st := range states {
		loc := resolveTemplateLocation(src, st.Template.Path)
		out = append(out, programTemplateJSON{
			ID:             st.Template.ID,
			Name:           st.Template.Name,
			Phase:          st.Template.Phase,
			PhaseName:      st.Phase.Name,
			State:          string(st.Status),
			Output:         st.Template.Output,
			OutputPath:     st.OutputPath,
			TemplatePath:   st.Template.Path,
			TemplateFrom:   loc.Path,
			TemplateSource: loc.Kind,
			Trigger:        string(st.Template.EffectiveTrigger()),
			Requires:       st.Template.Requires,
			BlockedBy:      st.BlockedBy,
			HumanReview:    st.Template.HumanReview,
			Detail:         st.Detail,
		})
	}
	return out
}

// stateGlyph maps a template status to its one-character marker.
func stateGlyph(status models.TemplateStatus) string {
	switch status {
	case models.TemplateGenerated:
		return "✓"
	case models.TemplateReady:
		return "→"
	case models.TemplateBlocked:
		return "⨯"
	case models.TemplatePending:
		return "·"
	default:
		return "?"
	}
}

// blockedByLabel reformats a core.TemplateState BlockedBy entry
// ("<template-id> (<output>)") as "<template-id> → <output>" — the same
// information, with the arrow reading as "which file that template produces".
func blockedByLabel(entry string) string {
	if id, output, found := strings.Cut(entry, " ("); found {
		return id + " → " + strings.TrimSuffix(output, ")")
	}
	return entry
}

func newProgramStatusCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status <program-id>",
		Short: "Show each template's state in this workspace, grouped by phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			src, err := resolveProgramSource(args[0])
			if err != nil {
				return err
			}
			p, err := src.load()
			if err != nil {
				return err
			}
			states, err := core.ProgramStatus(p, App.BasePath)
			if err != nil {
				return fmt.Errorf("failed to compute program status: %w", err)
			}
			if jsonOutput {
				return printJSON(templateRows(src, states))
			}
			printProgramStatus(src.id, p, states)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// printProgramStatus renders the human status view: phase headings in manifest
// order, one glyph-prefixed line per template, and — the part that makes the view
// actionable — the SPECIFIC missing dependency for a blocked template plus a
// standing reminder that generated artifacts still owe a human review.
//
// id is the id that ADDRESSES this pack (programSource.id — the directory name),
// not p.Program.ID (the manifest's declared id). For every shipped pack they are
// identical (the provenance guard requires it), but an external pack may declare a
// manifest id that differs from its directory, and resolution keys on the directory
// — so the heading has to echo the id the reader typed, or the view identifies the
// pack by a name no other command accepts.
func printProgramStatus(id string, p *models.Program, states []core.TemplateState) {
	fmt.Printf("%s — %s\n", id, p.Program.Name)

	width := 0
	for _, st := range states {
		if len(st.Template.ID) > width {
			width = len(st.Template.ID)
		}
	}

	var generated, ready, blocked, pending, needReview int
	currentPhase := ""
	for _, st := range states {
		if st.Template.Phase != currentPhase {
			currentPhase = st.Template.Phase
			fmt.Printf("\n%s\n", currentPhase)
		}
		suffix := ""
		switch st.Status {
		case models.TemplateGenerated:
			generated++
			if st.Template.HumanReview != nil && st.Template.HumanReview.Required {
				needReview++
				suffix = "  (needs human review)"
			}
		case models.TemplateReady:
			ready++
		case models.TemplateBlocked:
			blocked++
			labels := make([]string, 0, len(st.BlockedBy))
			for _, entry := range st.BlockedBy {
				labels = append(labels, blockedByLabel(entry))
			}
			suffix = fmt.Sprintf("  (needs: %s)", strings.Join(labels, ", "))
		case models.TemplatePending:
			pending++
			suffix = fmt.Sprintf("  (%s)", st.Template.EffectiveTrigger())
		default:
			// An unrecognised status is surfaced rather than folded into pending,
			// so a status added to the model without updating this switch is
			// visible in the output instead of silently mislabelled.
			suffix = fmt.Sprintf("  (unknown status %q)", st.Status)
		}
		// TrimRight because the state column is padded: a template with no suffix
		// would otherwise end the line in trailing spaces.
		fmt.Println(strings.TrimRight(fmt.Sprintf("  %s %-*s  %-9s%s",
			stateGlyph(st.Status), width, st.Template.ID, st.Status, suffix), " "))
	}

	fmt.Printf("\n%d of %d generated · %d ready · %d blocked · %d pending\n",
		generated, len(states), ready, blocked, pending)
	if needReview > 0 {
		fmt.Printf("⚠ %d generated artifact(s) still need human review\n", needReview)
	}
}

// templateLocationKind names WHICH of programTemplateLocation's three branches
// produced a location. It is what the `--json` rows carry as `template_source`,
// because a scripted consumer needs the distinction the human view carries in
// prose: an `unprovisioned` location is where the template WILL be, not where it
// is, and reading it now would fail.
type templateLocationKind string

const (
	// templateProvisioned: the copy under programs/<id>/ is on disk.
	templateProvisioned templateLocationKind = "provisioned"
	// templateSearchPath: the pack came from a programs_search_paths directory and
	// its template is read from there.
	templateSearchPath templateLocationKind = "search_path"
	// templateUnprovisioned: an embedded pack with nothing at programs/<id>/ — the
	// template is inside the binary, so Path is a destination, not a source.
	templateUnprovisioned templateLocationKind = "unprovisioned"
)

// programTemplateLocation is where one template of one pack can be read in THIS
// workspace, plus how that answer was reached. Splitting the answer from its
// rendering is what let `--json` gain the same resolved location the human `from:`
// line has, instead of the two drifting apart: programTemplateFrom formats this for
// a human, templateRows projects it for a machine, and neither re-derives it.
type programTemplateLocation struct {
	// Path is the location itself: WORKSPACE-RELATIVE (forward-slash) for a
	// provisioned or not-yet-provisioned pack, ABSOLUTE for a search-path pack.
	Path string
	Kind templateLocationKind
}

// resolveTemplateLocation locates one template, with three cases in precedence
// order:
//
//  1. the PROVISIONED copy at <workspace>/programs/<id>/<path> exists → report it
//     WORKSPACE-RELATIVE, so it copy-pastes from the same root `write:` and
//     `reads:` are anchored to. This is the common case (`adb init project`
//     provisions the default pack; `adb program scaffold` any other) and it is
//     verified on disk rather than assumed.
//  2. the pack came from an external `programs_search_paths` directory and was
//     never scaffolded → report the ABSOLUTE <search-path>/<id>/<path>, which is
//     where the template demonstrably is. A search path need not live inside the
//     workspace, so there is no relative form to report, and an external pack never
//     has to be scaffolded to be usable.
//  3. otherwise the pack is embedded and unprovisioned: its templates exist only
//     inside the binary. Report the provisioned path it WOULD have, marked
//     `templateUnprovisioned` so every caller can say it is not there yet — an
//     unresolvable path with no explanation is the defect this replaces.
//
// Note what case 1 does and does not check: only `programs/<id>/`. `adb program
// scaffold <id> <custom-dest>` installs a pack SOMEWHERE ELSE, and nothing records
// where, so an EMBEDDED pack installed that way reaches case 3 even though it is
// provisioned. (An external one still reaches case 2 — its search path is
// configured, so it is still locatable.) The rendered wording therefore says which
// path was checked, rather than claiming the pack was never installed.
//
// Path discipline: `path` builds the forward-slash values that get printed and
// index the embedded FS; `filepath` builds the on-disk paths that get stat'd.
func resolveTemplateLocation(src programSource, templatePath string) programTemplateLocation {
	rel := path.Join(core.ProgramsRoot, src.id, templatePath)
	if App != nil {
		provisioned := filepath.Join(App.BasePath, core.ProgramsRoot, src.id, filepath.FromSlash(templatePath))
		if info, err := os.Stat(provisioned); err == nil && !info.IsDir() {
			return programTemplateLocation{Path: rel, Kind: templateProvisioned}
		}
	}
	if src.external != "" {
		return programTemplateLocation{
			Path: filepath.Join(src.external, src.id, filepath.FromSlash(templatePath)),
			Kind: templateSearchPath,
		}
	}
	return programTemplateLocation{Path: rel, Kind: templateUnprovisioned}
}

// programTemplateFrom renders the `from:` value `adb program next` prints for one
// template — the location a reader can actually open.
//
// For an unprovisioned pack the path alone would be the unresolvable path this
// whole seam exists to eliminate, so the line also states WHAT WAS CHECKED
// (`programs/<id>/`, and only that) plus the command that puts the pack there. It
// says "not provisioned at <dir>" rather than a bare "not provisioned" because a
// pack scaffolded to a custom dest IS provisioned — just not where this looks.
func programTemplateFrom(src programSource, templatePath string) string {
	loc := resolveTemplateLocation(src, templatePath)
	if loc.Kind != templateUnprovisioned {
		return loc.Path
	}
	return fmt.Sprintf("%s (not provisioned at %s/ — run `adb program scaffold %s`)",
		loc.Path, path.Join(core.ProgramsRoot, src.id), src.id)
}

func newProgramNextCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "next <program-id>",
		Short: "List the templates that can be drafted right now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			src, err := resolveProgramSource(args[0])
			if err != nil {
				return err
			}
			p, err := src.load()
			if err != nil {
				return err
			}
			states, err := core.NextTemplates(p, App.BasePath)
			if err != nil {
				return fmt.Errorf("failed to compute next templates: %w", err)
			}
			if jsonOutput {
				return printJSON(templateRows(src, states))
			}
			// src.id, never p.Program.ID: resolution keys on the DIRECTORY name, so
			// the manifest id is not guaranteed to be a thing any command accepts.
			// Printing it inside a command the reader is told to run made that hint
			// exit 1 with `unknown program` — the same "prints something that resolves
			// nowhere" defect the from: resolution above exists to eliminate.
			if len(states) == 0 {
				fmt.Printf("%s: nothing is ready — run `adb program status %s` to see what is blocked.\n",
					src.id, src.id)
				return nil
			}
			fmt.Printf("%s — ready to draft:\n", src.id)
			for _, st := range states {
				fmt.Printf("  → %s (%s)\n", st.Template.ID, st.Phase.Name)
				fmt.Printf("      write:  %s\n", st.Template.Output)
				// Template.Path is pack-relative and resolves nowhere on its own;
				// programTemplateFrom turns it into a path that actually locates the
				// template for this pack in this workspace.
				fmt.Printf("      from:   %s\n", programTemplateFrom(src, st.Template.Path))
				if len(st.Template.Reads) > 0 {
					fmt.Printf("      reads:  %s\n", strings.Join(st.Template.Reads, ", "))
				}
				fmt.Printf("      review: %s\n", humanReviewLine(st.Template.HumanReview))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}

// --- trace -----------------------------------------------------------------

func newProgramTraceCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "trace <program-id> <template-id>",
		Short: "Trace which inputs informed a generated artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}
			src, err := resolveProgramSource(args[0])
			if err != nil {
				return err
			}
			p, err := src.load()
			if err != nil {
				return err
			}
			trace, err := core.TraceArtifact(p, App.BasePath, args[1])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(trace)
			}
			fmt.Printf("%s → %s\n", trace.TemplateID, trace.Artifact)
			if len(trace.Sources) == 0 {
				fmt.Println("  (no `sources:` frontmatter — provenance was not recorded)")
			}
			for _, s := range trace.Sources {
				marker := "✓"
				if !s.Exists {
					marker = "⨯"
				}
				line := fmt.Sprintf("  %s %s", marker, s.Path)
				if s.TemplateID != "" {
					line += fmt.Sprintf("  [%s]", s.TemplateID)
				}
				if s.Note != "" {
					line += "  — " + collapseSpace(s.Note)
				}
				fmt.Println(line)
			}
			if len(trace.MissingRequires) > 0 {
				fmt.Printf("⚠ declared requires not cited as sources: %s\n",
					strings.Join(trace.MissingRequires, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	return cmd
}
