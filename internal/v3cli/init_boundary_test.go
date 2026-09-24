package v3cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/internal/capability"
)

// TestInitBoundaryIsTheExplicitSpellingForTheAidbBoundary pins Q2's resolution.
//
// `adb init <path>` created the `.aidb` boundary while `adb init workspace
// <path>` created the task workspace — two different things, one of them
// addressed by a bare positional on the parent. That asymmetry is what made
// people reach for the wrong one: nothing in `adb init --help` said the bare
// form was even a distinct concept, let alone which concept.
//
// Both are now named subcommands.
func TestInitBoundaryIsTheExplicitSpellingForTheAidbBoundary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := initializeResult(root, capability.OutcomePlanned)
	service := &fakeFoundation{initializeResult: result}

	output := execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "init", "boundary", root, "--dry-run", "--format", "json")

	assertSingleJSONValue(t, output, result)
	if len(service.initializeRequests) != 1 {
		t.Fatalf("initialize requests = %#v", service.initializeRequests)
	}
	request := service.initializeRequests[0]
	if request.Root != root {
		t.Fatalf("root = %q, want %q", request.Root, root)
	}
	if request.Name != "AWS" {
		t.Fatalf("name = %q, want the path basename AWS", request.Name)
	}
	if request.Apply {
		t.Fatal("dry-run must not apply")
	}
}

// TestInitBoundaryCarriesTheFullFlagSetAndApplies proves the new spelling is the
// same capability, not a thinner wrapper over it.
func TestInitBoundaryCarriesTheFullFlagSetAndApplies(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	service := &fakeFoundation{
		initializeResult: initializeResult(root, capability.OutcomeApplied),
	}

	execute(t, NewRoot(RootOptions{
		Foundation: service,
	}), "init", "boundary", root, "--name", "Amazon", "--apply", "--format", "json")

	request := service.initializeRequests[0]
	if request.Name != "Amazon" || !request.Apply {
		t.Fatalf("initialize request = %#v", request)
	}
}

// TestInitBoundaryRejectsApplyWithDryRun keeps the mutually-exclusive guard that
// the bare form had; a rename must not lose a safety check.
func TestInitBoundaryRejectsApplyWithDryRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	command := NewRoot(RootOptions{Foundation: &fakeFoundation{}})
	command.SetArgs([]string{"init", "boundary", root, "--apply", "--dry-run"})
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)

	if err := command.ExecuteContext(context.Background()); err == nil {
		t.Fatal("--apply with --dry-run was accepted")
	}
}

// TestInitBareFormStillWorksAndWarnsOnStderr is the back-compat half. Scripts
// and muscle memory use `adb init <path>`; it must keep working, warn about the
// move, and put that warning on stderr so `--format json` stays parseable.
func TestInitBareFormStillWorksAndWarnsOnStderr(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	result := initializeResult(root, capability.OutcomePlanned)
	service := &fakeFoundation{initializeResult: result}

	command := NewRoot(RootOptions{Foundation: service})
	command.SetArgs([]string{"init", root, "--dry-run", "--format", "json"})
	var stdout, stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("bare init form broke: %v\n%s", err, stderr.String())
	}

	// Same capability, same envelope.
	assertSingleJSONValue(t, stdout.String(), result)
	if len(service.initializeRequests) != 1 ||
		service.initializeRequests[0].Root != root {
		t.Fatalf("initialize requests = %#v", service.initializeRequests)
	}
	// The warning must not be in the JSON stream.
	if strings.Contains(stdout.String(), "deprecated") {
		t.Fatalf("deprecation warning polluted stdout JSON:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "init boundary") {
		t.Fatalf("bare form did not point at `init boundary`:\n%s", stderr.String())
	}
}

// TestInitWithNoArgsShowsHelpRatherThanInitializing guards the sharp edge the
// bare positional leaves behind: now that `init` is a namespace, running it bare
// must explain itself, not silently initialize something.
func TestInitWithNoArgsShowsHelpRatherThanInitializing(t *testing.T) {
	service := &fakeFoundation{}
	command := NewRoot(RootOptions{Foundation: service})
	command.SetArgs([]string{"init"})
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)

	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("bare `adb init` errored: %v\n%s", err, out.String())
	}
	if len(service.initializeRequests) != 0 {
		t.Fatalf("bare `adb init` initialized something: %#v", service.initializeRequests)
	}
	for _, want := range []string{"boundary", "workspace"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("bare `adb init` help omits %q:\n%s", want, out.String())
		}
	}
}

// TestInitChildrenVisibility folds `boundary` into the visibility rule the rest
// of the `init` children follow.
func TestInitChildrenVisibility(t *testing.T) {
	root := NewRoot(RootOptions{
		Foundation: &fakeFoundation{},
		LoadLegacyApp: func() error {
			return nil
		},
	})

	for name, wantVisible := range map[string]bool{
		"boundary":  true,
		"workspace": true,
		"project":   true,
		"claude":    false,
		"update":    false,
	} {
		t.Run(name, func(t *testing.T) {
			child, _, err := root.Find([]string{"init", name})
			if err != nil {
				t.Fatalf("find init %s: %v", name, err)
			}
			if child == nil || child.Name() != name {
				t.Fatalf("init %s is unavailable", name)
			}
			if child.Hidden == wantVisible {
				t.Fatalf(
					"init %s hidden = %t, want hidden = %t",
					name,
					child.Hidden,
					!wantVisible,
				)
			}
		})
	}
}

// TestInitBoundaryDoesNotLoadTheLegacyApp keeps `boundary` on the v3 side of the
// annotation seam — it is a foundation capability and must not drag the legacy
// App in, while its sibling `workspace` still must.
func TestInitBoundaryDoesNotLoadTheLegacyApp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AWS")
	loadCount := 0
	execute(t, NewRoot(RootOptions{
		Foundation: &fakeFoundation{
			initializeResult: initializeResult(root, capability.OutcomePlanned),
		},
		LoadLegacyApp: func() error {
			loadCount++
			return nil
		},
	}), "init", "boundary", root, "--dry-run", "--format", "json")

	if loadCount != 0 {
		t.Fatalf("init boundary loaded the legacy app %d time(s)", loadCount)
	}
}
