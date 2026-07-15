package core

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// This file enumerates and scaffolds the Idea/MVP VALIDATION template pack
// embedded under templates/claude/validation/: founder-playbook worksheets
// (problem-hypothesis, interview-framework, evidence-ledger, scope,
// measurement-framework, sean-ellis-survey, false-positive-registry), each paired
// with a `<name>.adversarial.md` companion prompt the devils-advocate agent uses
// to pressure-test the filled-in worksheet. Enumeration is data-driven (whatever
// exists under validation/), so adding a worksheet needs no Go change — the same
// principle as the pluggable projectinit scaffolding and the harness manifest.

const (
	validationRoot    = "validation"
	adversarialSuffix = ".adversarial.md"
)

// ValidationTemplate is one validation worksheet plus its companion adversarial
// prompt.
type ValidationTemplate struct {
	Name        string // slug, e.g. "problem-hypothesis"
	FileName    string // worksheet file name, e.g. "problem-hypothesis.md"
	Content     []byte // the worksheet
	Adversarial []byte // the companion adversarial prompt (nil if none authored)
}

// ValidationTemplates enumerates the validation pack in fsys: every
// validation/<name>.md that is NOT a *.adversarial.md companion is a template,
// paired with validation/<name>.adversarial.md when present. Ordered by name; a
// missing validation/ root yields no templates rather than an error.
func ValidationTemplates(fsys fs.FS) ([]ValidationTemplate, error) {
	entries, err := fs.ReadDir(fsys, validationRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading validation templates: %w", err)
	}

	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			present[e.Name()] = true
		}
	}

	var out []ValidationTemplate
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasSuffix(name, adversarialSuffix) {
			continue
		}
		content, err := fs.ReadFile(fsys, path.Join(validationRoot, name))
		if err != nil {
			return nil, fmt.Errorf("reading validation template %s: %w", name, err)
		}
		vt := ValidationTemplate{
			Name:     strings.TrimSuffix(name, ".md"),
			FileName: name,
			Content:  content,
		}
		if adv := vt.Name + adversarialSuffix; present[adv] {
			data, err := fs.ReadFile(fsys, path.Join(validationRoot, adv))
			if err != nil {
				return nil, fmt.Errorf("reading adversarial prompt %s: %w", adv, err)
			}
			vt.Adversarial = data
		}
		out = append(out, vt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ValidationScaffoldEntry is the outcome for one scaffolded worksheet.
type ValidationScaffoldEntry struct {
	Name   string
	Dest   string
	Action HarnessInstallAction // installed / unchanged / skipped (shared write semantics)
}

// ValidationScaffoldResult summarises a ScaffoldValidationTemplates call.
type ValidationScaffoldResult struct {
	DestDir string
	DryRun  bool
	Entries []ValidationScaffoldEntry
}

// Count returns how many entries had the given action.
func (r ValidationScaffoldResult) Count(action HarnessInstallAction) int {
	n := 0
	for _, e := range r.Entries {
		if e.Action == action {
			n++
		}
	}
	return n
}

// ScaffoldValidationTemplates writes the validation WORKSHEETS (not the
// adversarial prompts — those feed the agent, not the evidence dir) into destDir,
// with the same idempotent, clobber-safe semantics as InstallHarness: a matching
// file is "unchanged", a differing file is "skipped" (a user's edits are never
// clobbered) unless opts.Force overwrites it, and DryRun plans without writing.
func ScaffoldValidationTemplates(fsys fs.FS, destDir string, opts HarnessInstallOptions) (ValidationScaffoldResult, error) {
	if destDir == "" {
		return ValidationScaffoldResult{}, fmt.Errorf("destination directory not resolved")
	}
	templates, err := ValidationTemplates(fsys)
	if err != nil {
		return ValidationScaffoldResult{}, err
	}
	res := ValidationScaffoldResult{DestDir: destDir, DryRun: opts.DryRun}
	for _, t := range templates {
		dest := filepath.Join(destDir, t.FileName)
		action, err := installHarnessFile(dest, t.Content, opts)
		if err != nil {
			return ValidationScaffoldResult{}, err
		}
		res.Entries = append(res.Entries, ValidationScaffoldEntry{Name: t.Name, Dest: dest, Action: action})
	}
	return res, nil
}
