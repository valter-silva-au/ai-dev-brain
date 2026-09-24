package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file is the engine for DOCUMENT PROGRAMS: phase-ordered packs of artifact
// templates declared by a `program.yaml` manifest at the root of a template pack
// (see pkg/models/program.go for the schema). A program is DATA — adding one is a
// pack you drop under programs/, never a Go change — exactly like the compliance
// (#133) and GTM (#135) packs, whose scaffolder this reuses (packs.go).
//
// The dependency gate is PRESENCE-BASED: a template counts as generated when its
// declared `output` file exists in the workspace. That deliberately matches how
// the stage gates treat file evidence (stagegate.go: artifactSatisfied) instead
// of introducing a second completion state machine that could disagree with it.

const (
	// ProgramsRoot is the embedded-FS directory holding the program packs. It is
	// exported so callers (the CLI, project init) pass the same root rather than
	// each hard-coding the string.
	ProgramsRoot = "programs"
	// programManifestName is the manifest file at the root of every program pack.
	programManifestName = "program.yaml"
)

// LoadProgram parses and validates the program manifest at root within fsys
// (root being a pack root such as "programs/technical-design"). Trigger and
// requires-mode defaults are filled in so callers never branch on "".
func LoadProgram(fsys fs.FS, root string) (*models.Program, error) {
	if root == "" {
		return nil, fmt.Errorf("program root not resolved")
	}
	data, err := fs.ReadFile(fsys, path.Join(root, programManifestName))
	if err != nil {
		return nil, fmt.Errorf("read program manifest in %s: %w", root, err)
	}
	var p models.Program
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse program manifest in %s: %w", root, err)
	}
	p.Root = root
	for i := range p.Templates {
		if p.Templates[i].Trigger == "" {
			p.Templates[i].Trigger = models.TriggerPhase
		}
		if p.Templates[i].RequiresMode == "" {
			p.Templates[i].RequiresMode = models.RequiresAll
		}
	}
	if err := ValidateProgram(&p); err != nil {
		return nil, fmt.Errorf("invalid program manifest in %s: %w", root, err)
	}
	return &p, nil
}

// ListPrograms returns the sorted, deduplicated ids of every available program:
// the embedded packs under root in fsys, plus any subdirectory of a searchPaths
// entry that carries a program.yaml. An EMBEDDED program wins an id collision —
// the shipped pack is the reference definition, so an external directory cannot
// shadow it by reusing its id. A missing embedded root or a missing search path
// is not an error (a workspace need not have either).
func ListPrograms(fsys fs.FS, root string, searchPaths []string) ([]string, error) {
	seen := map[string]bool{} // id -> embedded?

	dirs, err := listPackDirs(fsys, root)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, d := range dirs {
		if _, err := fs.Stat(fsys, path.Join(root, d, programManifestName)); err != nil {
			continue // a directory without a manifest is not a program
		}
		seen[d] = true
	}

	for _, dir := range searchPaths {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read program search path %s: %w", dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), programManifestName)); err != nil {
				continue
			}
			if seen[e.Name()] {
				continue // embedded (or an earlier search path) already owns this id
			}
			seen[e.Name()] = false
		}
	}

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// ValidateProgram enforces the manifest's invariants. It rejects, in order:
// a missing identity; an EMPTY lineage (every program must cite the public
// sources it derives from); malformed phases; and, per template, a missing
// id/name/path/output, a duplicate id or output, an unknown phase, an invalid
// trigger or requires_mode, a MISSING human_review block, a `requires` entry
// naming an unknown template id, and any dependency CYCLE (reported as a path).
func ValidateProgram(p *models.Program) error {
	if p == nil {
		return fmt.Errorf("program is nil")
	}
	if strings.TrimSpace(p.Program.ID) == "" {
		return fmt.Errorf("program id is required")
	}
	if strings.TrimSpace(p.Program.Name) == "" {
		return fmt.Errorf("program %q: name is required", p.Program.ID)
	}
	if len(p.Program.Lineage) == 0 {
		return fmt.Errorf("program %q: lineage must list at least one public source", p.Program.ID)
	}
	for i, src := range p.Program.Lineage {
		if strings.TrimSpace(src) == "" {
			return fmt.Errorf("program %q: lineage entry %d is empty", p.Program.ID, i)
		}
	}

	if len(p.Phases) == 0 {
		return fmt.Errorf("program %q: at least one phase is required", p.Program.ID)
	}
	phases := make(map[string]bool, len(p.Phases))
	for _, ph := range p.Phases {
		if strings.TrimSpace(ph.ID) == "" {
			return fmt.Errorf("program %q: every phase needs an id", p.Program.ID)
		}
		if phases[ph.ID] {
			return fmt.Errorf("program %q: duplicate phase id %q", p.Program.ID, ph.ID)
		}
		phases[ph.ID] = true
	}

	if len(p.Templates) == 0 {
		return fmt.Errorf("program %q: at least one template is required", p.Program.ID)
	}
	ids := make(map[string]bool, len(p.Templates))
	outputs := make(map[string]string, len(p.Templates))
	for _, t := range p.Templates {
		if strings.TrimSpace(t.ID) == "" {
			return fmt.Errorf("program %q: every template needs an id", p.Program.ID)
		}
		if ids[t.ID] {
			return fmt.Errorf("program %q: duplicate template id %q", p.Program.ID, t.ID)
		}
		ids[t.ID] = true

		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("program %q: template %q: name is required", p.Program.ID, t.ID)
		}
		if strings.TrimSpace(t.Path) == "" {
			return fmt.Errorf("program %q: template %q: path is required", p.Program.ID, t.ID)
		}
		if strings.TrimSpace(t.Output) == "" {
			return fmt.Errorf("program %q: template %q: output is required", p.Program.ID, t.ID)
		}
		// Two templates writing the same output would make the presence-based gate
		// ambiguous (one artifact would satisfy both), so it is rejected.
		if other, dup := outputs[t.Output]; dup {
			return fmt.Errorf("program %q: template %q: duplicate output %q (also produced by %q)",
				p.Program.ID, t.ID, t.Output, other)
		}
		outputs[t.Output] = t.ID

		if !phases[t.Phase] {
			return fmt.Errorf("program %q: template %q: unknown phase %q", p.Program.ID, t.ID, t.Phase)
		}
		switch t.EffectiveTrigger() {
		case models.TriggerPhase, models.TriggerManual, models.TriggerEvent:
		default:
			return fmt.Errorf("program %q: template %q: invalid trigger %q (want phase|manual|event)",
				p.Program.ID, t.ID, t.Trigger)
		}
		switch t.EffectiveRequiresMode() {
		case models.RequiresAll, models.RequiresAny:
		default:
			return fmt.Errorf("program %q: template %q: invalid requires_mode %q (want all|any)",
				p.Program.ID, t.ID, t.RequiresMode)
		}
		// human_review is mandatory: these artifacts are generated, so each one has
		// to say what a human must check and what it costs to get wrong.
		if t.HumanReview == nil {
			return fmt.Errorf("program %q: template %q: human_review is required", p.Program.ID, t.ID)
		}
	}

	// requires must name template IDS, never file paths — resolving ids is what
	// makes a typo an error instead of a template that silently never fires.
	for _, t := range p.Templates {
		for _, req := range t.Requires {
			if !ids[req] {
				return fmt.Errorf("program %q: template %q requires unknown template id %q",
					p.Program.ID, t.ID, req)
			}
		}
	}
	if cycle := findProgramCycle(p); len(cycle) > 0 {
		return fmt.Errorf("program %q: dependency cycle: %s", p.Program.ID, strings.Join(cycle, " -> "))
	}
	return nil
}

// findProgramCycle returns the first dependency cycle as a path (e.g.
// ["a","b","a"]), or nil when the requires graph is acyclic. Plain colour-marking
// DFS over the id graph; the returned path names the offenders so the manifest
// author can see the loop rather than a bare "there is a cycle".
func findProgramCycle(p *models.Program) []string {
	const (
		white = 0 // unvisited
		grey  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make(map[string]int, len(p.Templates))
	var stack []string

	var visit func(id string) []string
	visit = func(id string) []string {
		color[id] = grey
		stack = append(stack, id)
		t, _ := p.Template(id)
		for _, req := range t.Requires {
			switch color[req] {
			case grey:
				// Cut the stack at req to report just the loop, closed on itself.
				for i, s := range stack {
					if s == req {
						return append(append([]string{}, stack[i:]...), req)
					}
				}
				return []string{req, req}
			case white:
				if cycle := visit(req); cycle != nil {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return nil
	}

	for _, t := range p.Templates {
		if color[t.ID] == white {
			if cycle := visit(t.ID); cycle != nil {
				return cycle
			}
		}
	}
	return nil
}

// ScaffoldProgram writes a program pack (root/<program>/, manifest and all) into
// destDir, preserving its subdirectory layout via the recursive pack walker. The
// install is idempotent and clobber-safe: content that already matches is
// "unchanged", content that differs is "skipped" unless opts.Force, and
// opts.DryRun plans without writing. The manifest is loaded and VALIDATED first,
// so a malformed pack never gets half-scaffolded into a workspace.
func ScaffoldProgram(fsys fs.FS, root, program, destDir string, opts HarnessInstallOptions) ([]PackScaffoldEntry, error) {
	if destDir == "" {
		return nil, fmt.Errorf("destination directory not resolved")
	}
	if _, err := LoadProgram(fsys, path.Join(root, program)); err != nil {
		return nil, fmt.Errorf("program %q: %w", program, err)
	}
	return scaffoldPackTree(fsys, root, program, destDir, opts)
}

// TemplateState is one template's classification within a workspace.
type TemplateState struct {
	Template models.ProgramTemplate `yaml:"template" json:"template"`
	Phase    models.ProgramPhase    `yaml:"phase" json:"phase"`
	Status   models.TemplateStatus  `yaml:"status" json:"status"`
	// OutputPath is the resolved on-disk path of the artifact.
	OutputPath string `yaml:"output_path" json:"output_path"`
	// BlockedBy names each unsatisfied requirement as "<template-id> (<output>)" —
	// the id AND its output path, so a CLI can tell the user both which template
	// to run and which file it is waiting for.
	BlockedBy []string `yaml:"blocked_by,omitempty" json:"blocked_by,omitempty"`
	// Detail is a short human explanation of the status.
	Detail string `yaml:"detail,omitempty" json:"detail,omitempty"`
}

// ProgramStatus classifies every template in p against workspaceRoot, in PHASE
// order and then manifest order within a phase. The gate is presence-based:
//
//	generated — the template's output file exists
//	ready     — not generated, trigger=phase, requires satisfied per requires_mode
//	blocked   — not generated, trigger=phase, requires NOT satisfied
//	pending   — trigger is manual or event (waits on a human or an event)
func ProgramStatus(p *models.Program, workspaceRoot string) ([]TemplateState, error) {
	if p == nil {
		return nil, fmt.Errorf("program is nil")
	}
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root not resolved")
	}

	// One presence pass first: every requirement check reads this map, so an
	// artifact is stat'ed once and the classification below is pure.
	generated := make(map[string]bool, len(p.Templates))
	for _, t := range p.Templates {
		generated[t.ID] = artifactExists(workspaceRoot, t.Output)
	}

	ordered := orderedTemplates(p)
	states := make([]TemplateState, 0, len(ordered))
	for _, t := range ordered {
		st := TemplateState{
			Template:   t,
			OutputPath: artifactPath(workspaceRoot, t.Output),
		}
		if idx := p.PhaseIndex(t.Phase); idx >= 0 {
			st.Phase = p.Phases[idx]
		}
		switch {
		case generated[t.ID]:
			st.Status = models.TemplateGenerated
			st.Detail = fmt.Sprintf("%s exists", t.Output)
		case t.EffectiveTrigger() != models.TriggerPhase:
			st.Status = models.TemplatePending
			st.Detail = fmt.Sprintf("waits on a %s trigger", t.EffectiveTrigger())
		default:
			missing := missingRequirements(p, t, generated)
			if requirementsSatisfied(t, len(t.Requires), len(missing)) {
				st.Status = models.TemplateReady
				st.Detail = "requirements satisfied — draft it next"
			} else {
				st.Status = models.TemplateBlocked
				st.BlockedBy = missing
				st.Detail = fmt.Sprintf("waiting on %d of %d requirement(s) (%s)",
					len(missing), len(t.Requires), t.EffectiveRequiresMode())
			}
		}
		states = append(states, st)
	}
	return states, nil
}

// NextTemplates returns the READY subset of ProgramStatus, in the same order —
// the artifacts a human or agent can draft right now.
func NextTemplates(p *models.Program, workspaceRoot string) ([]TemplateState, error) {
	states, err := ProgramStatus(p, workspaceRoot)
	if err != nil {
		return nil, err
	}
	var out []TemplateState
	for _, st := range states {
		if st.Status == models.TemplateReady {
			out = append(out, st)
		}
	}
	return out, nil
}

// orderedTemplates returns p's templates in phase order, preserving manifest
// order within a phase (a stable sort on the phase index).
func orderedTemplates(p *models.Program) []models.ProgramTemplate {
	out := append([]models.ProgramTemplate{}, p.Templates...)
	sort.SliceStable(out, func(i, j int) bool {
		return p.PhaseIndex(out[i].Phase) < p.PhaseIndex(out[j].Phase)
	})
	return out
}

// missingRequirements lists t's unsatisfied requirements as
// "<template-id> (<output>)". Every missing requirement is reported even under
// requires_mode: any, so the caller can offer the user a choice of which upstream
// artifact to produce.
func missingRequirements(p *models.Program, t models.ProgramTemplate, generated map[string]bool) []string {
	var missing []string
	for _, req := range t.Requires {
		if generated[req] {
			continue
		}
		label := req
		if rt, ok := p.Template(req); ok {
			label = fmt.Sprintf("%s (%s)", req, rt.Output)
		}
		missing = append(missing, label)
	}
	return missing
}

// requirementsSatisfied applies the quantifier: `all` needs zero missing, `any`
// needs at least one satisfied. An empty requires list is always satisfied.
func requirementsSatisfied(t models.ProgramTemplate, total, missing int) bool {
	if total == 0 {
		return true
	}
	if t.EffectiveRequiresMode() == models.RequiresAny {
		return missing < total
	}
	return missing == 0
}

// artifactPath resolves a manifest output (forward-slash, workspace-relative) to
// an on-disk path.
func artifactPath(workspaceRoot, output string) string {
	return filepath.Join(workspaceRoot, filepath.FromSlash(output))
}

// artifactExists reports whether an artifact is present as a regular file. This
// is the whole gate: presence, mirroring the stage gates' file evidence.
func artifactExists(workspaceRoot, output string) bool {
	info, err := os.Stat(artifactPath(workspaceRoot, output))
	return err == nil && !info.IsDir()
}

// TraceSource is one input recorded in a generated artifact's `sources:`
// frontmatter, resolved against the program and the workspace.
type TraceSource struct {
	// Path is the workspace-relative path (or free-form reference) as recorded.
	Path string `yaml:"path" json:"path"`
	// Note is the optional annotation from the mapping form of an entry.
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
	// TemplateID is the program template that produces this path, when the source
	// is another program artifact ("" for external context like project-doc/**).
	TemplateID string `yaml:"template_id,omitempty" json:"template_id,omitempty"`
	// Exists reports whether the referenced file is present in the workspace.
	Exists bool `yaml:"exists" json:"exists"`
}

// Trace is the provenance report for one generated artifact.
type Trace struct {
	TemplateID string `yaml:"template_id" json:"template_id"`
	// Artifact is the on-disk path of the generated artifact.
	Artifact string        `yaml:"artifact" json:"artifact"`
	Sources  []TraceSource `yaml:"sources,omitempty" json:"sources,omitempty"`
	// MissingRequires names declared `requires` template ids whose output is NOT
	// cited in the artifact's sources — the artifact may have been drafted without
	// the input the program says informs it.
	MissingRequires []string `yaml:"missing_requires,omitempty" json:"missing_requires,omitempty"`
}

// artifactFrontmatter is the YAML frontmatter block a generated artifact carries.
// Only `sources:` is read here; unknown keys are ignored so the frontmatter stays
// free for other tooling.
type artifactFrontmatter struct {
	Sources []frontmatterSource `yaml:"sources"`
}

// frontmatterSource accepts either form of a sources entry: a bare scalar path
// ("- project-doc/vision.md") or a mapping ("- {path: …, note: …}").
type frontmatterSource struct {
	Path string `yaml:"path"`
	Note string `yaml:"note,omitempty"`
}

// UnmarshalYAML lets a scalar node decode into Path so authors can write the
// short form without a schema error.
func (s *frontmatterSource) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&s.Path)
	}
	type raw frontmatterSource // avoid recursing into this method
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*s = frontmatterSource(r)
	return nil
}

// TraceArtifact reports which inputs informed a generated artifact by reading the
// `sources:` list from its YAML frontmatter, resolving each entry to the program
// template that produces it (when it is a program artifact) and checking whether
// it is still present. It errors clearly when the artifact has not been generated
// yet — there is nothing to trace until the file exists.
func TraceArtifact(p *models.Program, workspaceRoot, templateID string) (*Trace, error) {
	if p == nil {
		return nil, fmt.Errorf("program is nil")
	}
	if workspaceRoot == "" {
		return nil, fmt.Errorf("workspace root not resolved")
	}
	t, ok := p.Template(templateID)
	if !ok {
		return nil, fmt.Errorf("program %q has no template %q", p.Program.ID, templateID)
	}
	dest := artifactPath(workspaceRoot, t.Output)
	data, err := os.ReadFile(dest)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("artifact for template %q has not been generated yet (expected %s)", templateID, t.Output)
		}
		return nil, fmt.Errorf("read artifact %s: %w", dest, err)
	}

	fm, err := parseArtifactFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter of %s: %w", t.Output, err)
	}

	// outputOwner maps a declared output path back to its template id so a cited
	// source can be named as the upstream template, not just a path.
	outputOwner := make(map[string]string, len(p.Templates))
	for _, other := range p.Templates {
		outputOwner[other.Output] = other.ID
	}

	tr := &Trace{TemplateID: templateID, Artifact: dest}
	cited := make(map[string]bool, len(fm.Sources))
	for _, s := range fm.Sources {
		src := TraceSource{
			Path:       s.Path,
			Note:       s.Note,
			TemplateID: outputOwner[s.Path],
			Exists:     artifactExists(workspaceRoot, s.Path),
		}
		if src.TemplateID != "" {
			cited[src.TemplateID] = true
		}
		tr.Sources = append(tr.Sources, src)
	}
	for _, req := range t.Requires {
		if !cited[req] {
			tr.MissingRequires = append(tr.MissingRequires, req)
		}
	}
	return tr, nil
}

// parseArtifactFrontmatter extracts the leading `---`-delimited YAML block. An
// artifact without frontmatter is not an error — it simply records no sources.
func parseArtifactFrontmatter(data []byte) (artifactFrontmatter, error) {
	var fm artifactFrontmatter
	// Tolerate a UTF-8 BOM and CRLF line endings — an artifact may have been
	// touched by an editor on another platform.
	text := strings.TrimPrefix(string(data), "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fm, nil
	}
	for i := 1; i < len(lines); i++ {
		switch strings.TrimSpace(lines[i]) {
		case "---", "...":
			block := strings.Join(lines[1:i], "\n")
			if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
				return artifactFrontmatter{}, err
			}
			return fm, nil
		}
	}
	// An unterminated block is malformed rather than absent — say so.
	return fm, fmt.Errorf("frontmatter block opened with --- but never closed")
}
