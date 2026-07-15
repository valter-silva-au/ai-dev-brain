package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// ProjectInitializer handles full workspace scaffolding for new projects
type ProjectInitializer interface {
	// InitializeProject creates a complete workspace with all scaffolding
	InitializeProject(path string, options InitOptions) error
}

// InitOptions configures project initialization
type InitOptions struct {
	Name         string // Project name
	AIProvider   string // AI provider (claude, gpt)
	TaskIDPrefix string // Task ID prefix (TASK, FEAT, etc)
	BuildCommand string // Build command written to .taskrc (language-agnostic; empty = unset)
	TestCommand  string // Test command written to .taskrc (language-agnostic; empty = unset)
	GitInit      bool   // Initialize git repository
	WithBMAD     bool   // Include BMAD artifacts templates
}

// Scaffold source roots inside the embedded templates filesystem. Every regular
// file under these directories is rendered through text/template with the
// resolved InitOptions and written to the mirrored path in the target project —
// so adding, editing, or removing a scaffolded artifact is a matter of dropping
// a file into templates/claude/projectinit/, with no Go change required.
const (
	scaffoldBaseDir  = "projectinit/base" // always written, at the project root
	scaffoldGitDir   = "projectinit/git"  // written only when GitInit, at the project root
	scaffoldBMADDir  = "projectinit/bmad" // written only when WithBMAD
	scaffoldBMADDest = "docs/bmad"        // …into this subdirectory

	// scaffoldVersionFile holds the template-set version inside the embedded FS.
	// It is metadata (not under base/git/bmad) so it is never scaffolded into a
	// project — only recorded in the manifest.
	scaffoldVersionFile = "projectinit/VERSION"
	// defaultTemplateVersion is used when the template FS carries no VERSION file
	// (e.g. a synthetic FS in a test); an update from it is a well-defined no-op.
	defaultTemplateVersion = "0"
	// templateManifestRelPath is where the provenance manifest lives in a project.
	templateManifestRelPath = ".adb/template-manifest.yaml"
)

// UpdateStatus classifies one file in a template-update plan.
type UpdateStatus string

const (
	// UpdateAdded: the template has a file the project lacks.
	UpdateAdded UpdateStatus = "added"
	// UpdateModified: the template changed the file since scaffold AND the project
	// copy is untouched, so it can be safely updated.
	UpdateModified UpdateStatus = "updated"
	// UpdateConflict: the template changed the file AND the project copy was
	// edited by the user since scaffold — updating would clobber local changes.
	UpdateConflict UpdateStatus = "conflict"
	// UpdateUnchanged: the template did not change the file since scaffold.
	UpdateUnchanged UpdateStatus = "unchanged"
)

// UpdatePlan is the diff between a scaffolded project and the current template
// set — the copier/cruft "update" plan. Entries are grouped by status; each
// carries the workspace-relative path.
type UpdatePlan struct {
	FromVersion string
	ToVersion   string
	Added       []string
	Updated     []string
	Conflicts   []string
	Unchanged   []string
}

// HasChanges reports whether applying the plan would write anything (added or
// safely-updated files). Conflicts alone do not count — they are skipped unless
// forced.
func (p *UpdatePlan) HasChanges() bool {
	return len(p.Added) > 0 || len(p.Updated) > 0
}

// FileProjectInitializer implements ProjectInitializer by rendering the embedded
// scaffold templates. It reads every artifact from templatesFS rather than
// hardcoding content inline, so the produced documents are drop-in/pluggable.
type FileProjectInitializer struct {
	templatesFS fs.FS // Embedded filesystem for templates
}

// NewFileProjectInitializer creates a new project initializer. templatesFS is
// typically claude.FS (an embed.FS), but any fs.FS carrying the projectinit tree
// works — which is what makes the scaffolding testable with a synthetic FS.
func NewFileProjectInitializer(templatesFS fs.FS) *FileProjectInitializer {
	return &FileProjectInitializer{
		templatesFS: templatesFS,
	}
}

// InitializeProject creates a complete workspace from templates
func (pi *FileProjectInitializer) InitializeProject(target string, options InitOptions) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Apply defaults once, so both the .taskrc and any other scaffolded file see
	// the same resolved values.
	options = pi.applyDefaults(absPath, options)

	// Create directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Initialize git if requested
	if options.GitInit {
		if err := pi.initGit(absPath, options); err != nil {
			return fmt.Errorf("failed to initialize git: %w", err)
		}
	}

	// Create directory structure
	if err := pi.createDirectories(absPath); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Render every applicable artifact once (base + git-if + bmad-if), keyed by
	// workspace-relative path, then write them. Rendering to a map first lets the
	// same render feed the provenance manifest (below) and a later `update` diff.
	artifacts, err := pi.renderArtifacts(options)
	if err != nil {
		return err
	}
	if err := writeArtifacts(absPath, artifacts); err != nil {
		return err
	}

	// Record the provenance manifest so the project can be re-synced to newer
	// template versions later (`adb init update`).
	manifest := models.TemplateManifest{
		TemplateVersion: pi.TemplateVersion(),
		Options:         answersFromOptions(options),
		Files:           hashArtifacts(artifacts),
	}
	if err := writeManifest(absPath, manifest); err != nil {
		return fmt.Errorf("failed to write template manifest: %w", err)
	}

	return nil
}

// renderArtifacts renders every applicable scaffold file for options into a map
// keyed by workspace-relative path (forward slashes). It is the single place the
// scaffold set is enumerated, shared by InitializeProject (write + manifest) and
// the update diff (compare). Git files are included only when GitInit; BMAD only
// when WithBMAD (mapped under docs/bmad).
func (pi *FileProjectInitializer) renderArtifacts(options InitOptions) (map[string][]byte, error) {
	out := map[string][]byte{}
	collect := func(srcDir, destPrefix string) error {
		return fs.WalkDir(pi.templatesFS, srcDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				// A missing optional tree (e.g. no git/ in a synthetic FS) is not
				// fatal — just nothing to collect.
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel := strings.TrimPrefix(p, srcDir+"/")
			if rel == p {
				return nil
			}
			rendered, rerr := pi.renderFile(p, options)
			if rerr != nil {
				return fmt.Errorf("rendering %s: %w", p, rerr)
			}
			out[path.Join(destPrefix, rel)] = rendered
			return nil
		})
	}
	if err := collect(scaffoldBaseDir, ""); err != nil {
		return nil, fmt.Errorf("failed to render base files: %w", err)
	}
	if options.GitInit {
		if err := collect(scaffoldGitDir, ""); err != nil {
			return nil, fmt.Errorf("failed to render git files: %w", err)
		}
	}
	if options.WithBMAD {
		if err := collect(scaffoldBMADDir, scaffoldBMADDest); err != nil {
			return nil, fmt.Errorf("failed to render BMAD artifacts: %w", err)
		}
	}
	return out, nil
}

// templateVersion reads the template-set version from the embedded FS, falling
// back to defaultTemplateVersion when absent (a synthetic FS in tests, or an
// older template tree). The value is trimmed of surrounding whitespace.
func (pi *FileProjectInitializer) TemplateVersion() string {
	data, err := fs.ReadFile(pi.templatesFS, scaffoldVersionFile)
	if err != nil {
		return defaultTemplateVersion
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return defaultTemplateVersion
	}
	return v
}

// PlanUpdate computes the diff between a scaffolded project at projectDir and the
// current template set, without writing anything. It reads the recorded manifest
// (answers + per-file baseline hashes), re-renders the templates at the current
// version, and classifies each file added/updated/conflict/unchanged via a
// three-way comparison (baseline hash vs on-disk file vs newly rendered content).
func (pi *FileProjectInitializer) PlanUpdate(projectDir string) (*UpdatePlan, error) {
	manifest, err := readManifest(projectDir)
	if err != nil {
		return nil, err
	}

	options := optionsFromAnswers(manifest.Options)
	rendered, err := pi.renderArtifacts(options)
	if err != nil {
		return nil, err
	}

	plan := &UpdatePlan{
		FromVersion: manifest.TemplateVersion,
		ToVersion:   pi.TemplateVersion(),
	}

	for rel, content := range rendered {
		newHash := hashBytes(content)
		oldHash, hadBaseline := manifest.Files[rel]

		diskContent, readErr := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(rel)))
		diskMissing := os.IsNotExist(readErr)
		if readErr != nil && !diskMissing {
			return nil, fmt.Errorf("reading %s: %w", rel, readErr)
		}

		switch {
		case diskMissing || !hadBaseline:
			// The project lacks the file, or it is new to the template set.
			plan.Added = append(plan.Added, rel)
		case newHash == oldHash:
			// Template did not change this file since the recorded baseline.
			plan.Unchanged = append(plan.Unchanged, rel)
		case hashBytes(diskContent) == oldHash:
			// Template changed it and the user did not touch it → safe update.
			plan.Updated = append(plan.Updated, rel)
		default:
			// Template changed it AND the user edited it → conflict.
			plan.Conflicts = append(plan.Conflicts, rel)
		}
	}

	sort.Strings(plan.Added)
	sort.Strings(plan.Updated)
	sort.Strings(plan.Conflicts)
	sort.Strings(plan.Unchanged)
	return plan, nil
}

// ApplyUpdate computes the plan and writes the added + safely-updated files. When
// force is true, conflicted files are overwritten too; otherwise they are left
// untouched. The manifest is rewritten to the current version, with baseline
// hashes advanced for every file that was written (and left as-is for skipped
// conflicts, so they stay flagged next run). It returns the plan that was
// applied.
func (pi *FileProjectInitializer) ApplyUpdate(projectDir string, force bool) (*UpdatePlan, error) {
	manifest, err := readManifest(projectDir)
	if err != nil {
		return nil, err
	}
	plan, err := pi.PlanUpdate(projectDir)
	if err != nil {
		return nil, err
	}
	options := optionsFromAnswers(manifest.Options)
	rendered, err := pi.renderArtifacts(options)
	if err != nil {
		return nil, err
	}

	// New baseline starts from the recorded one so files not in the plan (or
	// skipped conflicts) keep their prior hash.
	newBaseline := map[string]string{}
	for k, v := range manifest.Files {
		newBaseline[k] = v
	}

	write := func(rel string) error {
		content := rendered[rel]
		outPath := filepath.Join(projectDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		newBaseline[rel] = hashBytes(content)
		return nil
	}

	for _, rel := range append(append([]string{}, plan.Added...), plan.Updated...) {
		if err := write(rel); err != nil {
			return nil, err
		}
	}
	// Unchanged files still refresh their baseline to the current render (a no-op
	// hash, but keeps the manifest internally consistent at the new version).
	for _, rel := range plan.Unchanged {
		newBaseline[rel] = hashBytes(rendered[rel])
	}
	if force {
		for _, rel := range plan.Conflicts {
			if err := write(rel); err != nil {
				return nil, err
			}
		}
	}

	manifest.TemplateVersion = pi.TemplateVersion()
	manifest.Files = newBaseline
	if err := writeManifest(projectDir, manifest); err != nil {
		return nil, fmt.Errorf("failed to rewrite template manifest: %w", err)
	}
	return plan, nil
}

// applyDefaults fills in sensible defaults for unset options.
func (pi *FileProjectInitializer) applyDefaults(absPath string, options InitOptions) InitOptions {
	if options.Name == "" {
		options.Name = filepath.Base(absPath)
	}
	if options.AIProvider == "" {
		options.AIProvider = "claude"
	}
	if options.TaskIDPrefix == "" {
		options.TaskIDPrefix = "TASK"
	}
	// BuildCommand / TestCommand intentionally default to empty: adb does not
	// assume a Go (or any) toolchain. The scaffolded .taskrc documents how to set
	// them per project.
	return options
}

// initGit initializes a git repository. The git scaffold files (e.g. .gitignore)
// are rendered and written by InitializeProject via renderArtifacts (gated on
// GitInit) so they are recorded in the provenance manifest like every other
// artifact; initGit only runs the `git init` command.
func (pi *FileProjectInitializer) initGit(absPath string, options InitOptions) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = absPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}
	return nil
}

// createDirectories creates the standard directory structure
func (pi *FileProjectInitializer) createDirectories(path string) error {
	dirs := []string{
		"tickets",
		"tickets/_archived",
		"work",
		"sessions",
		".adb",
		".claude",
		".claude/rules",
		"docs",
		"docs/bmad",
	}

	for _, dir := range dirs {
		dirPath := filepath.Join(path, dir)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	return nil
}

// writeArtifacts writes each rendered artifact (workspace-relative path → bytes)
// under destRoot, creating intermediate directories as needed. It is the write
// half of renderArtifacts, kept separate so the render output can also feed the
// manifest and the update diff without touching disk.
func writeArtifacts(destRoot string, artifacts map[string][]byte) error {
	for rel, content := range artifacts {
		outPath := filepath.Join(destRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating dir for %s: %w", outPath, err)
		}
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
	}
	return nil
}

// hashBytes returns the sha256 hex digest of b — the stable content fingerprint
// used for the manifest baseline and the three-way update diff.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// hashArtifacts maps each artifact's relative path to its content hash.
func hashArtifacts(artifacts map[string][]byte) map[string]string {
	out := make(map[string]string, len(artifacts))
	for rel, content := range artifacts {
		out[rel] = hashBytes(content)
	}
	return out
}

// answersFromOptions/optionsFromAnswers convert between the in-memory scaffold
// options and the persisted manifest answers (a storage-agnostic mirror).
func answersFromOptions(o InitOptions) models.TemplateAnswers {
	return models.TemplateAnswers{
		Name:         o.Name,
		AIProvider:   o.AIProvider,
		TaskIDPrefix: o.TaskIDPrefix,
		BuildCommand: o.BuildCommand,
		TestCommand:  o.TestCommand,
		GitInit:      o.GitInit,
		WithBMAD:     o.WithBMAD,
	}
}

func optionsFromAnswers(a models.TemplateAnswers) InitOptions {
	return InitOptions{
		Name:         a.Name,
		AIProvider:   a.AIProvider,
		TaskIDPrefix: a.TaskIDPrefix,
		BuildCommand: a.BuildCommand,
		TestCommand:  a.TestCommand,
		GitInit:      a.GitInit,
		WithBMAD:     a.WithBMAD,
	}
}

// writeManifest persists the provenance manifest to .adb/template-manifest.yaml
// under projectDir (creating .adb as needed).
func writeManifest(projectDir string, m models.TemplateManifest) error {
	outPath := filepath.Join(projectDir, filepath.FromSlash(templateManifestRelPath))
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("creating .adb dir: %w", err)
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(outPath, data, 0o644)
}

// ErrNoManifest is returned (wrapped) by readManifest when a project has no
// template manifest — it was not scaffolded by `adb init project`. Callers that
// treat "no manifest" as "nothing to do" (e.g. the drift checker) match it with
// errors.Is rather than string-sniffing.
var ErrNoManifest = errors.New("no template manifest")

// readManifest loads the provenance manifest from projectDir. A missing manifest
// is a clear, actionable error (the project predates manifest tracking or was not
// scaffolded by adb) rather than a silent empty plan.
func readManifest(projectDir string) (models.TemplateManifest, error) {
	var m models.TemplateManifest
	outPath := filepath.Join(projectDir, filepath.FromSlash(templateManifestRelPath))
	data, err := os.ReadFile(outPath)
	if err != nil {
		if os.IsNotExist(err) {
			return m, fmt.Errorf("%w at %s — was this project scaffolded by `adb init project`?", ErrNoManifest, templateManifestRelPath)
		}
		return m, fmt.Errorf("reading template manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parsing template manifest: %w", err)
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m, nil
}

// YAMLScalar encodes s as a single safe YAML flow scalar (including the quotes
// when quoting is needed), so a user-supplied value with a double-quote,
// backslash, colon, or other YAML metacharacter can never break the generated
// .taskrc. It is the fix for #156: the old scaffolds interpolated raw values
// into a `"%s"` slot, so `--name 'Acme "Rocket"'` produced invalid YAML that
// bricked every later command, and a backslash in --build-command was mangled by
// YAML double-quote escaping. yaml.Marshal of a string yields "value\n"; we trim
// the trailing newline. Marshal of a plain string is infallible, but on the
// theoretical error we fall back to a conservative double-quoted, escaped form.
func YAMLScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return strings.TrimRight(string(out), "\n")
}

// taskrcFuncMap is the template FuncMap for scaffolded config files. `yamlq`
// safely YAML-encodes a value so templates write `name: {{yamlq .Name}}` (no
// surrounding quotes in the template — the function emits them as needed).
var taskrcFuncMap = template.FuncMap{"yamlq": YAMLScalar}

// renderFile reads a single embedded file and renders it as a text/template with
// options. Files with no template actions come through byte-identical.
func (pi *FileProjectInitializer) renderFile(embedPath string, options InitOptions) ([]byte, error) {
	content, err := fs.ReadFile(pi.templatesFS, embedPath)
	if err != nil {
		return nil, fmt.Errorf("reading template: %w", err)
	}

	// Name the template after its base file name (only used in error messages).
	// yamlq is available so config templates can safely encode user values (#156).
	tmpl, err := template.New(path.Base(embedPath)).Funcs(taskrcFuncMap).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, options); err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}
	return buf.Bytes(), nil
}
