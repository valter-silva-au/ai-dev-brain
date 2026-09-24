package repository

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/organization"
)

func TestValidateComponentRejectsUnsafeUnixAndWindowsShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		validate func(string) error
		wantErr  bool
	}{
		{name: "component empty", value: "", validate: ValidateComponent, wantErr: true},
		{name: "component current directory", value: ".", validate: ValidateComponent, wantErr: true},
		{name: "component parent directory", value: "..", validate: ValidateComponent, wantErr: true},
		{name: "component slash", value: "owner/repo", validate: ValidateComponent, wantErr: true},
		{name: "component backslash", value: `owner\repo`, validate: ValidateComponent, wantErr: true},
		{name: "component drive relative", value: `C:repo`, validate: ValidateComponent, wantErr: true},
		{name: "component trailing dot", value: "trailing.", validate: ValidateComponent, wantErr: true},
		{name: "component trailing space", value: "trailing ", validate: ValidateComponent, wantErr: true},
		{name: "component reserved CON", value: "CON", validate: ValidateComponent, wantErr: true},
		{name: "component reserved NUL", value: "nul", validate: ValidateComponent, wantErr: true},
		{name: "component reserved COM1", value: "COM1", validate: ValidateComponent, wantErr: true},
		{name: "component control character", value: "line\nbreak", validate: ValidateComponent, wantErr: true},
		{name: "component host shape", value: "github.com", validate: ValidateComponent},
		{name: "component hyphenated", value: "valter-silva-au", validate: ValidateComponent},
		{name: "component underscore", value: "ai_dev-brain", validate: ValidateComponent},
		{name: "component mixed case", value: "Repo.Name", validate: ValidateComponent},
		{name: "host valid", value: "github.com", validate: ValidateHost},
		{name: "host mixed case", value: "GitHub.com", validate: ValidateHost, wantErr: true},
		{name: "host empty label", value: "github..com", validate: ValidateHost, wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.validate(test.value)
			if test.wantErr && err == nil {
				t.Fatalf("validation of %q succeeded, want rejection", test.value)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("validation of %q failed: %v", test.value, err)
			}
		})
	}
}

func TestLayoutUsesOrganizationRepositoryRoleAndCanonicalPaths(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "AWS")
	organizationLayout, err := organization.NewLayout(root, "amazon")
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	layout, err := NewLayout(
		organizationLayout,
		"source",
		"github.com",
		"valter-silva-au",
		"ai-dev-brain",
	)
	if err != nil {
		t.Fatalf("new repository layout: %v", err)
	}
	wantRoot := filepath.Join(
		organizationLayout.Root(),
		"source",
		"github.com",
		"valter-silva-au",
		"ai-dev-brain",
	)
	if layout.Root() != wantRoot {
		t.Fatalf("root = %q, want %q", layout.Root(), wantRoot)
	}
	for _, path := range []string{
		layout.ControlDir(),
		layout.ManifestPath(),
		layout.ConfigPath(),
		layout.AgentsPath(),
		layout.CloneDir(),
		layout.TicketsDir(),
		layout.WorktreesDir(),
		layout.KnowledgeDir(),
	} {
		if !pathInside(wantRoot, path) {
			t.Fatalf("repository path escaped root: %q", path)
		}
	}
	if layout.HostConfigPath() != filepath.Join(
		organizationLayout.Root(),
		"source",
		"github.com",
		".aidb",
		"config.yaml",
	) {
		t.Fatalf("host config path = %q", layout.HostConfigPath())
	}
	if layout.OwnerConfigPath() != filepath.Join(
		organizationLayout.Root(),
		"source",
		"github.com",
		"valter-silva-au",
		".aidb",
		"config.yaml",
	) {
		t.Fatalf("owner config path = %q", layout.OwnerConfigPath())
	}
}

func TestRepositoryManifestRoundTripAndStrictValidation(t *testing.T) {
	t.Parallel()

	layout := testRepositoryLayout(t)
	createdAt := time.Date(2026, time.September, 10, 10, 0, 0, 0, time.UTC)
	manifest := NewManifest(
		"repository-1",
		"organization-1",
		layout,
		Remote{
			Name:     "origin",
			Type:     RemoteTypeCanonical,
			FetchURL: "https://github.com/valter-silva-au/ai-dev-brain.git",
		},
		createdAt,
		Provenance{
			OperationID: "operation-1",
			ActorType:   "human",
			Tool:        "adb-cli",
		},
	)
	manifest.AdditionalRemotes = []Remote{{
		Name:     "upstream",
		Type:     RemoteTypeUpstream,
		FetchURL: "ssh://git@github.com/aws/ai-dev-brain.git",
	}}
	manifest.ExternalClones = []ExternalClone{{
		Path:    filepath.Join(t.TempDir(), "external"),
		Purpose: "read-only mirror",
	}}
	manifest.Subprojects = []Subproject{{
		Name: "cli",
		Path: "cmd/adb",
	}}
	manifest.Aliases = []string{"github.com/old-owner/ai-dev-brain"}
	if err := manifest.Validate(layout); err != nil {
		t.Fatalf("validate repository manifest: %v", err)
	}

	var encoded bytes.Buffer
	if err := EncodeManifest(&encoded, manifest); err != nil {
		t.Fatalf("encode repository manifest: %v", err)
	}
	decoded, err := DecodeManifest(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode repository manifest: %v", err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip\n got: %#v\nwant: %#v", decoded, manifest)
	}

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "unknown field",
			content: strings.Replace(
				encoded.String(),
				"kind: Repository\n",
				"kind: Repository\nunknown: true\n",
				1,
			),
		},
		{
			name:    "multiple documents",
			content: encoded.String() + "---\n{}\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeManifest(
				strings.NewReader(test.content),
			); err == nil {
				t.Fatal("invalid repository manifest was accepted")
			}
		})
	}
}

func TestNormalizeRemoteURLSupportsHTTPSAndSSHWithoutCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want RemoteURL
	}{
		{
			name: "https",
			raw:  "HTTPS://GitHub.com/valter-silva-au/ai-dev-brain.git/",
			want: RemoteURL{
				Scheme:     "https",
				Host:       "github.com",
				Owner:      "valter-silva-au",
				Repository: "ai-dev-brain",
				Normalized: "https://github.com/valter-silva-au/ai-dev-brain.git",
			},
		},
		{
			name: "scp ssh",
			raw:  "git@GitHub.com:valter-silva-au/ai-dev-brain.git",
			want: RemoteURL{
				Scheme:     "ssh",
				User:       "git",
				Host:       "github.com",
				Owner:      "valter-silva-au",
				Repository: "ai-dev-brain",
				Normalized: "ssh://git@github.com/valter-silva-au/ai-dev-brain.git",
			},
		},
		{
			name: "ssh url",
			raw:  "ssh://git@github.com/valter-silva-au/ai-dev-brain",
			want: RemoteURL{
				Scheme:     "ssh",
				User:       "git",
				Host:       "github.com",
				Owner:      "valter-silva-au",
				Repository: "ai-dev-brain",
				Normalized: "ssh://git@github.com/valter-silva-au/ai-dev-brain.git",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeRemoteURL(test.raw)
			if err != nil {
				t.Fatalf("normalize %q: %v", test.raw, err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalized URL\n got: %#v\nwant: %#v", got, test.want)
			}
		})
	}
	invalid := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "file URL", raw: "file:///tmp/repo"},
		{
			name: "HTTPS credentials",
			raw:  "https://user:secret@github.com/owner/repo.git",
		},
		{
			name: "extra path segment",
			raw:  "https://github.com/owner/repo/extra",
		},
		{name: "Windows path", raw: `C:\repo`},
	}
	for _, test := range invalid {
		test := test
		t.Run("reject "+test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeRemoteURL(test.raw); err == nil {
				t.Fatalf("NormalizeRemoteURL(%q) = nil error", test.raw)
			}
		})
	}
}

func TestGitInventoryParsesRepositoryWorktreeRemotesAndDivergence(
	t *testing.T,
) {
	t.Parallel()

	repositoryRoot := filepath.Join(t.TempDir(), "repo")
	gitDir := filepath.Join(repositoryRoot, ".git", "worktrees", "task")
	commonDir := filepath.Join(repositoryRoot, ".git")
	runner := &fakeRunner{
		responses: map[string]RunResult{
			commandKey(repositoryRoot, "rev-parse", "--is-inside-work-tree"): {
				Stdout: "true\n",
			},
			commandKey(repositoryRoot, "rev-parse", "--show-toplevel"): {
				Stdout: repositoryRoot + "\n",
			},
			commandKey(repositoryRoot, "rev-parse", "--git-dir"): {
				Stdout: gitDir + "\n",
			},
			commandKey(repositoryRoot, "rev-parse", "--git-common-dir"): {
				Stdout: commonDir + "\n",
			},
			commandKey(repositoryRoot, "remote"): {
				Stdout: "origin\nupstream\n",
			},
			commandKey(repositoryRoot, "remote", "get-url", "origin"): {
				Stdout: "git@github.com:valter-silva-au/ai-dev-brain.git\n",
			},
			commandKey(repositoryRoot, "remote", "get-url", "--push", "origin"): {
				Stdout: "git@github.com:valter-silva-au/ai-dev-brain.git\n",
			},
			commandKey(repositoryRoot, "remote", "get-url", "upstream"): {
				Stdout: "https://github.com/aws/ai-dev-brain.git\n",
			},
			commandKey(repositoryRoot, "remote", "get-url", "--push", "upstream"): {
				Stdout: "https://github.com/aws/ai-dev-brain.git\n",
			},
			commandKey(
				repositoryRoot,
				"symbolic-ref",
				"--quiet",
				"--short",
				"refs/remotes/origin/HEAD",
			): {
				Stdout: "origin/main\n",
			},
			commandKey(
				repositoryRoot,
				"status",
				"--porcelain=v2",
				"--branch",
				"--untracked-files=normal",
			): {
				Stdout: "# branch.oid abc\n" +
					"# branch.head feat/ADB-39-v3\n" +
					"# branch.upstream origin/feat/ADB-39-v3\n" +
					"# branch.ab +2 -3\n" +
					"1 .M N... 100644 100644 100644 abc abc README.md\n" +
					"? scratch.txt\n",
			},
		},
	}
	client := NewClient(runner)
	inventory, err := client.Inventory(
		context.Background(),
		repositoryRoot,
		"origin",
	)
	if err != nil {
		t.Fatalf("inventory repository: %v", err)
	}
	if !inventory.IsRepository ||
		!inventory.IsWorktree ||
		!inventory.Dirty ||
		!inventory.Diverged ||
		inventory.Ahead != 2 ||
		inventory.Behind != 3 ||
		inventory.Branch != "feat/ADB-39-v3" ||
		inventory.Upstream != "origin/feat/ADB-39-v3" ||
		inventory.DefaultBranch != "main" {
		t.Fatalf("inventory = %#v", inventory)
	}
	if len(inventory.Remotes) != 2 ||
		inventory.Remotes[0].Name != "origin" ||
		inventory.Remotes[1].Name != "upstream" {
		t.Fatalf("remotes = %#v", inventory.Remotes)
	}
}

func TestGitClientUsesArgvOperationsAndPromptDisabledEnvironment(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	client := NewClient(runner)
	if err := client.Clone(
		context.Background(),
		"/workspace",
		"https://github.com/owner/repo.git",
		"/workspace/repo",
	); err != nil {
		t.Fatalf("clone repository: %v", err)
	}
	if err := client.Init(context.Background(), "/workspace/repo"); err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	if err := client.Fetch(
		context.Background(),
		"/workspace/repo",
		"origin",
	); err != nil {
		t.Fatalf("fetch repository: %v", err)
	}
	want := []Invocation{
		{
			Dir:  "/workspace",
			Args: []string{"clone", "--", "https://github.com/owner/repo.git", "/workspace/repo"},
		},
		{
			Dir:  "/workspace",
			Args: []string{"init", "--", "/workspace/repo"},
		},
		{
			Dir:  "/workspace/repo",
			Args: []string{"fetch", "--prune", "--no-tags", "origin"},
		},
	}
	if !reflect.DeepEqual(runner.invocations, want) {
		t.Fatalf("git invocations\n got: %#v\nwant: %#v", runner.invocations, want)
	}

	environment := promptDisabledEnvironment([]string{"PATH=/bin"})
	joined := strings.Join(environment, "\n")
	for _, expected := range []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=ssh -oBatchMode=yes",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("prompt-disabled environment missing %q: %v", expected, environment)
		}
	}
}

type fakeRunner struct {
	responses   map[string]RunResult
	errors      map[string]error
	invocations []Invocation
}

func (runner *fakeRunner) Run(
	_ context.Context,
	invocation Invocation,
) (RunResult, error) {
	runner.invocations = append(runner.invocations, invocation)
	key := commandKey(invocation.Dir, invocation.Args...)
	if err := runner.errors[key]; err != nil {
		return RunResult{}, err
	}
	return runner.responses[key], nil
}

func commandKey(dir string, args ...string) string {
	return dir + "\x00" + strings.Join(args, "\x00")
}

func testRepositoryLayout(t *testing.T) Layout {
	t.Helper()

	organizationLayout, err := organization.NewLayout(
		filepath.Join(t.TempDir(), "AWS"),
		"amazon",
	)
	if err != nil {
		t.Fatalf("new organization layout: %v", err)
	}
	layout, err := NewLayout(
		organizationLayout,
		"repos",
		"github.com",
		"valter-silva-au",
		"ai-dev-brain",
	)
	if err != nil {
		t.Fatalf("new repository layout: %v", err)
	}
	return layout
}

func pathInside(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil &&
		relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

var _ Runner = (*fakeRunner)(nil)
