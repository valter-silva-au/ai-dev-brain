package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/valter-silva-au/ai-dev-brain/templates/claude"
	"gopkg.in/yaml.v3"
)

func TestFileProjectInitializer_InitializeProject(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "projectinit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectPath := filepath.Join(tmpDir, "test-project")

	// Create initializer
	pi := NewFileProjectInitializer(claude.FS)

	// Test basic initialization
	options := InitOptions{
		Name:         "test-project",
		AIProvider:   "claude",
		TaskIDPrefix: "TEST",
		GitInit:      false,
		WithBMAD:     false,
	}

	err = pi.InitializeProject(projectPath, options)
	if err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}

	// Verify directory structure
	expectedDirs := []string{
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

	for _, dir := range expectedDirs {
		dirPath := filepath.Join(projectPath, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Expected directory %s not found", dir)
		}
	}

	// Verify config files
	expectedFiles := []string{
		"backlog.yaml",
		".taskrc",
		".claude/project_context.md",
		".claude/rules/task-isolation.md",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(projectPath, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s not found", file)
		}
	}

	// Verify .taskrc content
	taskrcPath := filepath.Join(projectPath, ".taskrc")
	content, err := os.ReadFile(taskrcPath)
	if err != nil {
		t.Fatalf("Failed to read .taskrc: %v", err)
	}

	taskrcContent := string(content)
	if taskrcContent == "" {
		t.Error(".taskrc should not be empty")
	}
}

// TestYAMLScalar_EscapesMetacharacters is the unit guard for #156: user values
// with YAML metacharacters must round-trip, not corrupt the document.
func TestYAMLScalar_EscapesMetacharacters(t *testing.T) {
	cases := []string{
		`Acme "Rocket" Labs`, // embedded double-quotes (the headline break)
		`scripts\build.bat`,  // backslash (mangled to 0x08 by naive "%s")
		`name: with: colons`, // YAML key-value metacharacter
		`plain`,              // ordinary value
		``,                   // empty
		`# not a comment`,    // leading hash
		"tab\tand\nnewline",  // control characters
	}
	for _, want := range cases {
		// Render a one-key doc using the encoded scalar, parse it back.
		doc := "name: " + YAMLScalar(want) + "\n"
		var parsed struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Errorf("YAMLScalar(%q) produced invalid YAML %q: %v", want, doc, err)
			continue
		}
		if parsed.Name != want {
			t.Errorf("round-trip mismatch: YAMLScalar(%q) -> parsed %q", want, parsed.Name)
		}
	}
}

// TestFileProjectInitializer_HostileNameProducesValidTaskrc proves the scaffolded
// .taskrc parses (and round-trips) even when --name/--build-command carry YAML
// metacharacters — the whole point of #156 (before the fix this .taskrc was
// invalid YAML and every later command failed at app init).
func TestFileProjectInitializer_HostileNameProducesValidTaskrc(t *testing.T) {
	tmpDir := t.TempDir()
	projectPath := filepath.Join(tmpDir, "hostile")
	pi := NewFileProjectInitializer(claude.FS)

	opts := InitOptions{
		Name:         `Acme "Rocket" Labs`,
		AIProvider:   "claude",
		TaskIDPrefix: "TASK",
		BuildCommand: `scripts\build.bat`,
		TestCommand:  "",
	}
	if err := pi.InitializeProject(projectPath, opts); err != nil {
		t.Fatalf("InitializeProject: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projectPath, ".taskrc"))
	if err != nil {
		t.Fatalf("read .taskrc: %v", err)
	}

	var parsed struct {
		Name  string `yaml:"name"`
		Build struct {
			Command string `yaml:"command"`
		} `yaml:"build"`
	}
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("scaffolded .taskrc is invalid YAML (#156 regression): %v\n---\n%s", err, content)
	}
	if parsed.Name != opts.Name {
		t.Errorf("name round-trip: got %q, want %q", parsed.Name, opts.Name)
	}
	if parsed.Build.Command != opts.BuildCommand {
		t.Errorf("build.command round-trip: got %q, want %q (backslash must survive)", parsed.Build.Command, opts.BuildCommand)
	}
}

func TestFileProjectInitializer_WithGitInit(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "projectinit-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectPath := filepath.Join(tmpDir, "git-project")

	// Create initializer
	pi := NewFileProjectInitializer(claude.FS)

	// Test with git init
	options := InitOptions{
		Name:         "git-project",
		AIProvider:   "claude",
		TaskIDPrefix: "GIT",
		GitInit:      true,
		WithBMAD:     false,
	}

	err = pi.InitializeProject(projectPath, options)
	if err != nil {
		t.Fatalf("InitializeProject with git failed: %v", err)
	}

	// Verify .git directory exists
	gitDir := filepath.Join(projectPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error("Expected .git directory not found")
	}

	// Verify .gitignore exists and ignores the whole .adb/ state dir (#186)
	// via a single entry, with the stale per-file .adb_* lines removed.
	gitignorePath := filepath.Join(projectPath, ".gitignore")
	giBytes, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("Expected .gitignore not found: %v", err)
	}
	gi := string(giBytes)
	if !strings.Contains(gi, ".adb/") {
		t.Errorf(".gitignore should ignore the whole .adb/ state dir:\n%s", gi)
	}
	for _, stale := range []string{".task_counter", ".events.jsonl", ".adb_terminal_state.json"} {
		if strings.Contains(gi, stale) {
			t.Errorf(".gitignore still lists stale per-file entry %q (collapsed into .adb/ in #186):\n%s", stale, gi)
		}
	}
}

func TestFileProjectInitializer_WithBMAD(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "projectinit-bmad-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectPath := filepath.Join(tmpDir, "bmad-project")

	// Create initializer
	pi := NewFileProjectInitializer(claude.FS)

	// Test with BMAD artifacts
	options := InitOptions{
		Name:         "bmad-project",
		AIProvider:   "claude",
		TaskIDPrefix: "BMAD",
		GitInit:      false,
		WithBMAD:     true,
	}

	err = pi.InitializeProject(projectPath, options)
	if err != nil {
		t.Fatalf("InitializeProject with BMAD failed: %v", err)
	}

	// Verify BMAD artifacts
	expectedBMADFiles := []string{
		"docs/bmad/PRD.md",
		"docs/bmad/product-brief.md",
		"docs/bmad/tech-spec.md",
		"docs/bmad/architecture-doc.md",
		"docs/bmad/quality-gates.md",
	}

	for _, file := range expectedBMADFiles {
		filePath := filepath.Join(projectPath, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected BMAD file %s not found", file)
		}
	}

	// Verify PRD content
	prdPath := filepath.Join(projectPath, "docs/bmad/PRD.md")
	content, err := os.ReadFile(prdPath)
	if err != nil {
		t.Fatalf("Failed to read PRD.md: %v", err)
	}

	prdContent := string(content)
	if prdContent == "" {
		t.Error("PRD.md should not be empty")
	}
}

func TestFileProjectInitializer_DefaultValues(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "projectinit-defaults-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectPath := filepath.Join(tmpDir, "defaults-project")

	// Create initializer
	pi := NewFileProjectInitializer(claude.FS)

	// Test with empty options (should use defaults)
	options := InitOptions{}

	err = pi.InitializeProject(projectPath, options)
	if err != nil {
		t.Fatalf("InitializeProject with defaults failed: %v", err)
	}

	// Verify .taskrc has default values
	taskrcPath := filepath.Join(projectPath, ".taskrc")
	content, err := os.ReadFile(taskrcPath)
	if err != nil {
		t.Fatalf("Failed to read .taskrc: %v", err)
	}

	taskrcContent := string(content)

	// Check for default values in content
	if taskrcContent == "" {
		t.Error(".taskrc should not be empty")
	}
}

// TestFileProjectInitializer_ScaffoldFromEmbeddedFS asserts the scaffolded files
// come from the embedded filesystem (rendered), not inline literals — e.g. the
// project name flows into .claude/project_context.md is generic and .taskrc is
// templated with the resolved options.
func TestFileProjectInitializer_ScaffoldFromEmbeddedFS(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "embedded-project")

	pi := NewFileProjectInitializer(claude.FS)
	if err := pi.InitializeProject(projectPath, InitOptions{Name: "acme", TaskIDPrefix: "ACME"}); err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}

	// Assert on the PARSED values, not the literal quoting: since #156 the
	// scaffold YAML-encodes each value, and yaml.v3 legitimately leaves a simple
	// value unquoted (name: acme). What matters is that it round-trips.
	taskrc := readFileString(t, filepath.Join(projectPath, ".taskrc"))
	var parsed struct {
		Name         string `yaml:"name"`
		TaskIDPrefix string `yaml:"task_id_prefix"`
	}
	if err := yaml.Unmarshal([]byte(taskrc), &parsed); err != nil {
		t.Fatalf(".taskrc is invalid YAML: %v\n%s", err, taskrc)
	}
	if parsed.Name != "acme" {
		t.Errorf(".taskrc should carry the templated name; got name=%q\n%s", parsed.Name, taskrc)
	}
	if parsed.TaskIDPrefix != "ACME" {
		t.Errorf(".taskrc should carry the templated prefix; got prefix=%q\n%s", parsed.TaskIDPrefix, taskrc)
	}
}

// TestFileProjectInitializer_TaskrcHasNoHardcodedGoCommands guards acceptance
// criterion 3: scaffolded repo config must not hardcode Go-only build/test
// commands. By default the build/test commands are unset, and a provided command
// is written verbatim regardless of language.
func TestFileProjectInitializer_TaskrcHasNoHardcodedGoCommands(t *testing.T) {
	// Default: no build/test command hardcoded.
	defaultPath := filepath.Join(t.TempDir(), "default-project")
	pi := NewFileProjectInitializer(claude.FS)
	if err := pi.InitializeProject(defaultPath, InitOptions{Name: "p"}); err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}
	taskrc := readFileString(t, filepath.Join(defaultPath, ".taskrc"))
	for _, banned := range []string{"go build ./...", "go test ./..."} {
		if strings.Contains(taskrc, banned) {
			t.Errorf(".taskrc must not hardcode Go command %q; got:\n%s", banned, taskrc)
		}
	}

	// Provided commands (a non-Go stack) are written through unchanged.
	nodePath := filepath.Join(t.TempDir(), "node-project")
	if err := pi.InitializeProject(nodePath, InitOptions{
		Name:         "p",
		BuildCommand: "npm run build",
		TestCommand:  "npm test",
	}); err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}
	nodeTaskrc := readFileString(t, filepath.Join(nodePath, ".taskrc"))
	var parsed struct {
		Build struct {
			Command     string `yaml:"command"`
			TestCommand string `yaml:"test_command"`
		} `yaml:"build"`
	}
	if err := yaml.Unmarshal([]byte(nodeTaskrc), &parsed); err != nil {
		t.Fatalf(".taskrc is invalid YAML: %v\n%s", err, nodeTaskrc)
	}
	if parsed.Build.Command != "npm run build" || parsed.Build.TestCommand != "npm test" {
		t.Errorf(".taskrc should carry the provided non-Go commands; got command=%q test_command=%q\n%s",
			parsed.Build.Command, parsed.Build.TestCommand, nodeTaskrc)
	}
}

// TestFileProjectInitializer_DropInTemplateRenders is the behavioral proof of the
// data-driven acceptance criterion: a template file that exists only in the
// backing filesystem (never referenced by Go code) is rendered and written with
// no code change. It uses a synthetic fs.FS so the assertion needs no repo file.
func TestFileProjectInitializer_DropInTemplateRenders(t *testing.T) {
	fsys := fstest.MapFS{
		"projectinit/base/backlog.yaml":         {Data: []byte("tasks: []\n")},
		"projectinit/base/DROPPED.md":           {Data: []byte("# {{.Name}} was dropped in\n")},
		"projectinit/base/nested/also-here.txt": {Data: []byte("prefix={{.TaskIDPrefix}}\n")},
	}

	projectPath := filepath.Join(t.TempDir(), "dropin-project")
	pi := NewFileProjectInitializer(fsys)
	if err := pi.InitializeProject(projectPath, InitOptions{Name: "acme", TaskIDPrefix: "ZZ"}); err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}

	dropped := readFileString(t, filepath.Join(projectPath, "DROPPED.md"))
	if dropped != "# acme was dropped in\n" {
		t.Errorf("dropped-in template not rendered; got %q", dropped)
	}
	nested := readFileString(t, filepath.Join(projectPath, "nested", "also-here.txt"))
	if nested != "prefix=ZZ\n" {
		t.Errorf("nested dropped-in template not rendered; got %q", nested)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func TestFileProjectInitializer_ExistingDirectory(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "projectinit-existing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use existing temp directory as project path
	projectPath := tmpDir

	// Create initializer
	pi := NewFileProjectInitializer(claude.FS)

	// Test with existing directory
	options := InitOptions{
		Name:         "existing-project",
		AIProvider:   "claude",
		TaskIDPrefix: "EXIST",
		GitInit:      false,
		WithBMAD:     false,
	}

	err = pi.InitializeProject(projectPath, options)
	if err != nil {
		t.Fatalf("InitializeProject with existing dir failed: %v", err)
	}

	// Verify files were created
	backlogPath := filepath.Join(projectPath, "backlog.yaml")
	if _, err := os.Stat(backlogPath); os.IsNotExist(err) {
		t.Error("Expected backlog.yaml not found in existing directory")
	}
}
