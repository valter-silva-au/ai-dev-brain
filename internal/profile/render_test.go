package profile

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRenderIsDeterministicAndRecordsProvenance(t *testing.T) {
	t.Parallel()

	profile := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/example",
		Version:       "v3",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.notes", "notes.md", AuthorityAuthored),
			testDirectory("ticket.artifacts", "artifacts", true),
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	profile.Artifacts[0].Template = &TemplateReference{
		ID: "ticket/notes", Version: "v5",
	}
	profile.Artifacts[2].Template = &TemplateReference{
		ID: "ticket/context", Version: "v4",
	}
	templates := []Template{
		{
			ID:          "ticket/context",
			Version:     "v4",
			SourceScope: "workspace",
			Content:     []byte("# Context\n\nTicket: {{.TicketID}}\n"),
		},
		{
			ID:          "ticket/notes",
			Version:     "v5",
			SourceScope: "builtin",
			Content:     []byte("# Notes\n"),
		},
	}
	first, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, templates),
		Data: map[string]string{
			"TicketID":     "TASK-00039",
			"DisplayTitle": "First title",
			"PathSlug":     "first-path",
			"BranchSlug":   "first-branch",
		},
	})
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, templates),
		Data: map[string]string{
			"TicketID":     "TASK-00039",
			"DisplayTitle": "A renamed ticket",
			"PathSlug":     "renamed-path",
			"BranchSlug":   "renamed-branch",
		},
	})
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("unrelated display/path/branch values changed render:\nfirst=%#v\nsecond=%#v", first, second)
	}

	if len(first.Files) != 2 ||
		first.Files[0].Role != "ticket.context" ||
		first.Files[1].Role != "ticket.notes" {
		t.Fatalf("rendered files are not deterministic: %#v", first.Files)
	}
	if got := string(first.Files[0].Content); got != "# Context\n\nTicket: TASK-00039\n" {
		t.Fatalf("context content = %q", got)
	}
	if first.Manifest.SchemaVersion != RenderManifestSchemaVersion ||
		first.Manifest.Profile.ID != profile.ID ||
		first.Manifest.Profile.Version != profile.Version ||
		len(first.Manifest.Artifacts) != 2 {
		t.Fatalf("render manifest = %#v", first.Manifest)
	}
	context := first.Manifest.Artifacts[0]
	if context.Template.ID != "ticket/context" ||
		context.Template.Version != "v4" ||
		context.Template.SourceScope != "workspace" ||
		context.GeneratedHash != HashContent(first.Files[0].Content) ||
		context.ObservedHash != context.GeneratedHash {
		t.Fatalf("context manifest record = %#v", context)
	}
}

func TestRenderRejectsMissingAmbiguousAndInvalidTemplates(t *testing.T) {
	t.Parallel()

	profile := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/example",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	valid := Template{
		ID:          "ticket/context",
		Version:     "v1",
		SourceScope: "builtin",
		Content:     []byte("{{.Required}}\n"),
	}
	if _, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, nil),
	}); err == nil {
		t.Fatal("missing template was accepted")
	}
	if _, err := NewTemplateCatalog([]Template{valid, valid}); err == nil {
		t.Fatal("duplicate template identity was accepted")
	}
	if _, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, []Template{valid}),
		Data:     map[string]string{},
	}); err == nil {
		t.Fatal("missing render value was accepted")
	}

	invalidSyntax := valid
	invalidSyntax.Content = []byte("{{")
	if _, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, []Template{invalidSyntax}),
	}); err == nil {
		t.Fatal("invalid template syntax was accepted")
	}

	invalidScope := valid
	invalidScope.SourceScope = "/arbitrary/ancestor"
	if _, err := NewTemplateCatalog([]Template{invalidScope}); err == nil {
		t.Fatal("arbitrary template source scope was accepted")
	}

	unresolved := cloneProfile(profile)
	unresolved.Inherits = []ProfileReference{{
		ID: "ticket/base", Version: "v1",
	}}
	if _, err := Render(RenderRequest{
		Profile:  unresolved,
		Resolver: mustTemplateCatalog(t, []Template{valid}),
		Data:     map[string]string{"Required": "present"},
	}); err == nil {
		t.Fatal("unresolved inherited profile was rendered")
	}
}

func TestRenderNormalizesTemplateAndInputNewlines(t *testing.T) {
	t.Parallel()

	profile := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/example",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	result, err := Render(RenderRequest{
		Profile: profile,
		Resolver: mustTemplateCatalog(t, []Template{{
			ID:          "ticket/context",
			Version:     "v1",
			SourceScope: "builtin",
			Content:     []byte("heading\r\n{{.Body}}\r\n"),
		}}),
		Data: map[string]string{"Body": "line one\r\nline two"},
	})
	if err != nil {
		t.Fatalf("render newline fixture: %v", err)
	}
	if got, want := string(result.Files[0].Content), "heading\nline one\nline two\n"; got != want {
		t.Fatalf("normalized content = %q, want %q", got, want)
	}
}

func TestBuiltinRenderReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	profile, err := BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}
	templates := BuiltinTemplates()
	rendered, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, templates),
		Data: map[string]string{
			"TicketID":  "TASK-00039",
			"Title":     "Example",
			"Status":    "backlog",
			"CreatedAt": "2026-09-10T12:00:00+08:00",
			"UpdatedAt": "2026-09-10T12:00:00+08:00",
		},
	})
	if err != nil {
		t.Fatalf("render built-in profile: %v", err)
	}
	if len(rendered.Files) != 2 {
		t.Fatalf("rendered built-in files = %#v", rendered.Files)
	}

	original := append([]byte(nil), rendered.Files[0].Content...)
	templates[0].Content[0] ^= 0xff
	rendered.Files[0].Content[0] ^= 0xff
	again := BuiltinTemplates()
	if bytes.Equal(templates[0].Content, again[0].Content) {
		t.Fatal("built-in template content aliases caller mutation")
	}
	fresh, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, again),
		Data: map[string]string{
			"TicketID":  "TASK-00039",
			"Title":     "Example",
			"Status":    "backlog",
			"CreatedAt": "2026-09-10T12:00:00+08:00",
			"UpdatedAt": "2026-09-10T12:00:00+08:00",
		},
	})
	if err != nil {
		t.Fatalf("render fresh built-ins: %v", err)
	}
	if !bytes.Equal(fresh.Files[0].Content, original) {
		t.Fatal("render result or built-in assets retained caller mutation")
	}
}

func TestRenderEnforcesResourceBoundsAndSafeTemplateActions(t *testing.T) {
	t.Parallel()

	profile := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/example",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
		},
	}
	oversized := Template{
		ID:          "ticket/context",
		Version:     "v1",
		SourceScope: "builtin",
		Content:     bytes.Repeat([]byte("x"), maxTemplateBytes+1),
	}
	if _, err := NewTemplateCatalog([]Template{oversized}); err == nil {
		t.Fatal("oversized template was accepted")
	}

	functionTemplate := oversized
	functionTemplate.Content = []byte(`{{printf "%1000000000s" "x"}}`)
	if _, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, []Template{functionTemplate}),
	}); err == nil {
		t.Fatal("template function capable of unbounded allocation was accepted")
	}

	repeated := oversized
	repeated.Content = []byte(
		"{{.Value}}{{.Value}}{{.Value}}{{.Value}}{{.Value}}",
	)
	if _, err := Render(RenderRequest{
		Profile:  profile,
		Resolver: mustTemplateCatalog(t, []Template{repeated}),
		Data: map[string]string{
			"Value": strings.Repeat(
				"x",
				maxRenderDataBytes-len("Value"),
			),
		},
	}); err == nil {
		t.Fatal("oversized rendered artifact was accepted")
	}

	manyArtifacts := cloneProfile(profile)
	manyArtifacts.Artifacts = make(
		[]Artifact,
		0,
		maxRenderedArtifacts+1,
	)
	for index := 0; index <= maxRenderedArtifacts; index++ {
		manyArtifacts.Artifacts = append(
			manyArtifacts.Artifacts,
			testArtifact(
				fmt.Sprintf("ticket.generated.%03d", index),
				fmt.Sprintf("generated-%03d.md", index),
				AuthorityGenerated,
			),
		)
		manyArtifacts.Artifacts[index].Template = &TemplateReference{
			ID: "ticket/context", Version: "v1",
		}
	}
	if _, err := Render(RenderRequest{
		Profile:  manyArtifacts,
		Resolver: mustTemplateCatalog(t, []Template{repeated}),
		Data:     map[string]string{"Value": "small"},
	}); err == nil {
		t.Fatal("profile with excessive rendered artifacts was accepted")
	}

	aggregateProfile := cloneProfile(profile)
	aggregateProfile.Artifacts = nil
	for index := 0; index < 5; index++ {
		aggregateProfile.Artifacts = append(
			aggregateProfile.Artifacts,
			testArtifact(
				fmt.Sprintf("ticket.aggregate.%d", index),
				fmt.Sprintf("aggregate-%d.md", index),
				AuthorityGenerated,
			),
		)
		aggregateProfile.Artifacts[index].Template = &TemplateReference{
			ID: "ticket/aggregate", Version: "v1",
		}
	}
	aggregateTemplate := Template{
		ID:          "ticket/aggregate",
		Version:     "v1",
		SourceScope: "builtin",
		Content:     []byte("{{.Value}}{{.Value}}{{.Value}}{{.Value}}"),
	}
	if _, err := Render(RenderRequest{
		Profile:  aggregateProfile,
		Resolver: mustTemplateCatalog(t, []Template{aggregateTemplate}),
		Data: map[string]string{
			"Value": strings.Repeat(
				"x",
				maxRenderDataBytes-len("Value"),
			),
		},
	}); err == nil {
		t.Fatal("aggregate rendered content limit was not enforced")
	}
}

func mustTemplateCatalog(
	t *testing.T,
	templates []Template,
) *TemplateCatalog {
	t.Helper()
	catalog, err := NewTemplateCatalog(templates)
	if err != nil {
		t.Fatalf("create template catalog: %v", err)
	}
	return catalog
}
