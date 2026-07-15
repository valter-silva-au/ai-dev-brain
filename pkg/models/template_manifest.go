package models

// TemplateManifest records the provenance of a scaffolded workspace so it can be
// re-synced to newer template versions (the copier/cruft "update" model, #128).
// It is written to .adb/template-manifest.yaml at `adb init project` time and
// rewritten by `adb init update`. A single manifest — rather than a per-artifact
// stamp comment — is the source of truth: many scaffolded artifacts are YAML/JSON
// where an inline stamp would be awkward or invalid, and a manifest enables a
// clean three-way diff (recorded hash vs on-disk file vs newly rendered content).
type TemplateManifest struct {
	// TemplateVersion is the version of the template set the workspace was last
	// scaffolded/synced from (read from projectinit/VERSION in the template FS).
	TemplateVersion string `yaml:"template_version" json:"template_version"`

	// Options are the answers used to render the templates, so an update can
	// re-render deterministically without re-prompting (copier's .copier-answers).
	Options TemplateAnswers `yaml:"options" json:"options"`

	// Files maps each scaffolded artifact's workspace-relative path (forward
	// slashes) to the sha256 hex of its rendered content at scaffold/last-sync
	// time. The diff compares this baseline against the on-disk file and the newly
	// rendered content to classify each file as unchanged/updated/conflicted.
	Files map[string]string `yaml:"files" json:"files"`
}

// TemplateAnswers are the render inputs recorded in the manifest. It mirrors the
// scaffolding options (core.InitOptions) as a persisted, storage-agnostic type.
type TemplateAnswers struct {
	Name         string `yaml:"name,omitempty" json:"name,omitempty"`
	AIProvider   string `yaml:"ai_provider,omitempty" json:"ai_provider,omitempty"`
	TaskIDPrefix string `yaml:"task_id_prefix,omitempty" json:"task_id_prefix,omitempty"`
	BuildCommand string `yaml:"build_command,omitempty" json:"build_command,omitempty"`
	TestCommand  string `yaml:"test_command,omitempty" json:"test_command,omitempty"`
	GitInit      bool   `yaml:"git_init,omitempty" json:"git_init,omitempty"`
	WithBMAD     bool   `yaml:"with_bmad,omitempty" json:"with_bmad,omitempty"`
}
