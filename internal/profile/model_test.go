package profile

import (
	"strings"
	"testing"
)

func TestDecodeProfileIsStrictAndValidatesArtifactPolicy(t *testing.T) {
	t.Parallel()

	document := `
schema_version: aidb.profile/v1
id: ticket/engineering
version: v2
inherits:
  - id: ticket/default
    version: v1
work_types: [feat, fix]
artifacts:
  - role: ticket.context
    path: context.md
    kind: file
    authority: authored
    required: true
    append_only: false
    search: semantic
    retention: durable
    git: tracked
    template:
      id: ticket/context
      version: v2
`
	got, err := DecodeProfile(strings.NewReader(document))
	if err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if got.SchemaVersion != SchemaVersion ||
		got.ID != "ticket/engineering" ||
		got.Version != "v2" ||
		len(got.Inherits) != 1 ||
		len(got.Artifacts) != 1 {
		t.Fatalf("decoded profile = %#v", got)
	}

	invalid := []string{
		document + "unknown: true\n",
		strings.Replace(document, "path: context.md", "path: ../context.md", 1),
		strings.Replace(document, "path: context.md", `path: C:\context.md`, 1),
		strings.Replace(document, "path: context.md", "path: .aidb/rendered.yaml", 1),
		strings.Replace(document, "kind: file", "kind: directory", 1),
		strings.Replace(document, "search: semantic", "search: everywhere", 1),
		strings.Replace(document, "retention: durable", "retention: forever", 1),
		strings.Replace(document, "git: tracked", "git: maybe", 1),
		document + "---\nschema_version: aidb.profile/v1\n",
	}
	for _, input := range invalid {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeProfile(strings.NewReader(input)); err == nil {
				t.Fatalf("invalid profile was accepted:\n%s", input)
			}
		})
	}
}

func TestProfileRejectsDuplicateAndIncompatibleArtifacts(t *testing.T) {
	t.Parallel()

	base := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/example",
		Version:       "v1",
		WorkTypes:     []string{"feat"},
		Artifacts: []Artifact{
			{
				Role:       "ticket.context",
				Path:       "context.md",
				Kind:       ArtifactFile,
				Authority:  AuthorityAuthored,
				Required:   true,
				Search:     SearchSemantic,
				Retention:  RetentionDurable,
				Git:        GitTracked,
				Template:   &TemplateReference{ID: "ticket/context", Version: "v1"},
				AppendOnly: false,
			},
		},
	}
	if err := ValidateProfile(base); err != nil {
		t.Fatalf("valid profile: %v", err)
	}

	tests := map[string]func(*Profile){
		"duplicate role": func(profile *Profile) {
			profile.Artifacts = append(profile.Artifacts, profile.Artifacts[0])
		},
		"duplicate path": func(profile *Profile) {
			artifact := profile.Artifacts[0]
			artifact.Role = "ticket.other"
			profile.Artifacts = append(profile.Artifacts, artifact)
		},
		"case folded path collision": func(profile *Profile) {
			artifact := profile.Artifacts[0]
			artifact.Role = "ticket.other"
			artifact.Path = "Context.md"
			profile.Artifacts = append(profile.Artifacts, artifact)
		},
		"file parent collision": func(profile *Profile) {
			artifact := profile.Artifacts[0]
			profile.Artifacts[0].Path = "context"
			artifact.Role = "ticket.other"
			artifact.Path = "context/detail.md"
			profile.Artifacts = append(profile.Artifacts, artifact)
		},
		"unicode normalization collision": func(profile *Profile) {
			artifact := profile.Artifacts[0]
			artifact.Role = "ticket.other"
			artifact.Path = "cafe\u0301.md"
			profile.Artifacts = append(profile.Artifacts, artifact)
		},
		"append only generated": func(profile *Profile) {
			profile.Artifacts[0].Authority = AuthorityGenerated
			profile.Artifacts[0].AppendOnly = true
		},
		"directory template": func(profile *Profile) {
			profile.Artifacts[0].Kind = ArtifactDirectory
		},
		"generated without template": func(profile *Profile) {
			profile.Artifacts[0].Authority = AuthorityGenerated
			profile.Artifacts[0].Template = nil
		},
		"structural searchable": func(profile *Profile) {
			profile.Artifacts[0].Authority = AuthorityStructural
			profile.Artifacts[0].Template = nil
			profile.Artifacts[0].Search = SearchLexical
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneProfile(base)
			mutate(&candidate)
			if err := ValidateProfile(candidate); err == nil {
				t.Fatalf("%s profile was accepted: %#v", name, candidate)
			}
		})
	}
}

func TestResolveInheritanceUsesExactVersionedParents(t *testing.T) {
	t.Parallel()

	base := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/base",
		Version:       "v1",
		WorkTypes:     []string{"feat", "fix"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "context.md", AuthorityAuthored),
			testArtifact("ticket.notes", "notes.md", AuthorityAuthored),
		},
	}
	base.Artifacts[1].AppendOnly = true
	child := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/child",
		Version:       "v2",
		Inherits: []ProfileReference{{
			ID:      base.ID,
			Version: base.Version,
		}},
		WorkTypes: []string{"spike", "feat"},
		Artifacts: []Artifact{
			testArtifact("ticket.context", "brief.md", AuthorityAuthored),
			testDirectory("ticket.artifacts", "artifacts", true),
		},
	}
	resolved, err := ResolveInheritance(
		[]Profile{child, base},
		ProfileReference{ID: child.ID, Version: child.Version},
	)
	if err != nil {
		t.Fatalf("resolve inheritance: %v", err)
	}
	if len(resolved.Inherits) != 0 {
		t.Fatalf("resolved inheritance retained parents: %#v", resolved.Inherits)
	}
	if got, want := strings.Join(resolved.WorkTypes, ","), "feat,fix,spike"; got != want {
		t.Fatalf("work types = %q, want %q", got, want)
	}
	if got := artifactByRole(t, resolved, "ticket.context").Path; got != "brief.md" {
		t.Fatalf("overridden context path = %q", got)
	}
	if got := artifactByRole(t, resolved, "ticket.notes").Path; got != "notes.md" {
		t.Fatalf("inherited notes path = %q", got)
	}

	missing := cloneProfile(child)
	missing.Inherits[0].Version = "v9"
	if _, err := ResolveInheritance(
		[]Profile{missing, base},
		ProfileReference{ID: missing.ID, Version: missing.Version},
	); err == nil {
		t.Fatal("missing exact parent version was accepted")
	}

	cycleBase := cloneProfile(base)
	cycleBase.Inherits = []ProfileReference{{ID: child.ID, Version: child.Version}}
	if _, err := ResolveInheritance(
		[]Profile{child, cycleBase},
		ProfileReference{ID: child.ID, Version: child.Version},
	); err == nil {
		t.Fatal("inheritance cycle was accepted")
	}

	pureChild := Profile{
		SchemaVersion: SchemaVersion,
		ID:            "ticket/pure-child",
		Version:       "v1",
		Inherits: []ProfileReference{{
			ID: base.ID, Version: base.Version,
		}},
	}
	pure, err := ResolveInheritance(
		[]Profile{base, pureChild},
		profileReferenceOf(pureChild),
	)
	if err != nil {
		t.Fatalf("resolve pure inheritance: %v", err)
	}
	if len(pure.Artifacts) != len(base.Artifacts) ||
		len(pure.WorkTypes) != len(base.WorkTypes) {
		t.Fatalf("pure inherited profile = %#v", pure)
	}

	multipleParents := cloneProfile(pureChild)
	multipleParents.Inherits = append(
		multipleParents.Inherits,
		ProfileReference{ID: "ticket/other", Version: "v1"},
	)
	if err := ValidateProfile(multipleParents); err == nil {
		t.Fatal("ambiguous multiple profile inheritance was accepted")
	}
}

func TestBuiltinTicketProfileDefinesMinimumContract(t *testing.T) {
	t.Parallel()

	builtin, err := BuiltinTicketProfile()
	if err != nil {
		t.Fatalf("load built-in profile: %v", err)
	}
	if builtin.ID != BuiltinTicketProfileID ||
		builtin.Version != BuiltinTicketProfileVersion {
		t.Fatalf("built-in identity = %s@%s", builtin.ID, builtin.Version)
	}

	context := artifactByRole(t, builtin, "ticket.context")
	if context.Path != "context.md" ||
		context.Authority != AuthorityAuthored ||
		!context.Required {
		t.Fatalf("context artifact = %#v", context)
	}
	notes := artifactByRole(t, builtin, "ticket.notes")
	if notes.Path != "notes.md" ||
		notes.Authority != AuthorityAuthored ||
		!notes.Required ||
		!notes.AppendOnly {
		t.Fatalf("notes artifact = %#v", notes)
	}
	status := artifactByRole(t, builtin, "ticket.status")
	if status.Path != "status.yaml" ||
		status.Authority != AuthorityStructural ||
		!status.Required {
		t.Fatalf("status artifact = %#v", status)
	}
	for role, path := range map[string]string{
		"ticket.artifacts": "artifacts",
		"ticket.scratch":   "scratch",
	} {
		artifact := artifactByRole(t, builtin, role)
		if artifact.Path != path ||
			artifact.Kind != ArtifactDirectory ||
			artifact.Authority != AuthorityStructural ||
			!artifact.Required ||
			artifact.Retention != RetentionDurable {
			t.Fatalf("%s artifact = %#v", role, artifact)
		}
	}
	for _, artifact := range builtin.Artifacts {
		if artifact.Path == "sessions" ||
			strings.HasPrefix(artifact.Path, "sessions/") {
			t.Fatalf("built-in profile restored removed sessions role: %#v", artifact)
		}
	}
	for _, workType := range []string{
		"feat", "fix", "docs", "refactor", "test", "build",
		"ci", "perf", "style", "revert", "chore",
	} {
		if !containsString(builtin.WorkTypes, workType) {
			t.Fatalf("built-in profile is missing work type %q: %#v", workType, builtin.WorkTypes)
		}
	}
	if containsString(builtin.WorkTypes, "spike") {
		t.Fatalf("default profile must leave spike mapped to chore: %#v", builtin.WorkTypes)
	}

	templates := BuiltinTemplates()
	if len(templates) != 2 {
		t.Fatalf("built-in templates = %#v", templates)
	}
	for _, template := range templates {
		if template.ID == "" ||
			template.Version == "" ||
			template.SourceScope != "builtin" ||
			len(template.Content) == 0 {
			t.Fatalf("invalid built-in template = %#v", template)
		}
	}
}

func TestProfileReferencesRequireExactVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    ProfileReference
		wantErr bool
	}{
		{
			name:  "simple exact reference",
			input: "default@v1",
			want:  ProfileReference{ID: "default", Version: "v1"},
		},
		{
			name:  "namespaced exact reference",
			input: "ticket/engineering@2026.09-beta",
			want: ProfileReference{
				ID:      "ticket/engineering",
				Version: "2026.09-beta",
			},
		},
		{name: "missing version separator", input: "default", wantErr: true},
		{name: "empty version", input: "default@", wantErr: true},
		{name: "empty id", input: "@v1", wantErr: true},
		{
			name:    "multiple version separators",
			input:   "default@latest@v1",
			wantErr: true,
		},
		{
			name:    "leading whitespace",
			input:   " default@v1",
			wantErr: true,
		},
		{
			name:    "invalid identity traversal",
			input:   "ticket/../default@v1",
			wantErr: true,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reference, err := ParseProfileReference(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf(
						"non-exact profile selector %q was accepted",
						test.input,
					)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse exact profile reference: %v", err)
			}
			if reference != test.want ||
				reference.String() != test.input {
				t.Fatalf(
					"profile reference = %#v, want %#v",
					reference,
					test.want,
				)
			}
		})
	}
}

func testArtifact(role, artifactPath string, authority Authority) Artifact {
	return Artifact{
		Role:      role,
		Path:      artifactPath,
		Kind:      ArtifactFile,
		Authority: authority,
		Required:  true,
		Search:    SearchSemantic,
		Retention: RetentionDurable,
		Git:       GitTracked,
		Template: &TemplateReference{
			ID:      strings.ReplaceAll(role, ".", "/"),
			Version: "v1",
		},
	}
}

func testDirectory(role, artifactPath string, required bool) Artifact {
	return Artifact{
		Role:      role,
		Path:      artifactPath,
		Kind:      ArtifactDirectory,
		Authority: AuthorityStructural,
		Required:  required,
		Search:    SearchNone,
		Retention: RetentionDurable,
		Git:       GitTracked,
	}
}

func artifactByRole(t *testing.T, profile Profile, role string) Artifact {
	t.Helper()
	for _, artifact := range profile.Artifacts {
		if artifact.Role == role {
			return artifact
		}
	}
	t.Fatalf("artifact role %q not found in %#v", role, profile.Artifacts)
	return Artifact{}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
