package configuration

import (
	"strings"
	"testing"
)

func TestDecodeDocumentIsStrictButAllowsNamespacedPlugins(t *testing.T) {
	t.Parallel()

	document, err := DecodeDocument(strings.NewReader(`
schema_version: aidb.config/v1
settings:
  profile: engineering@v1
  policy:
    freshness:
      enabled: true
plugins:
  acme.example:
    arbitrary:
      nested: accepted
    token_limit: 4096
secret_references:
  github:
    provider: environment
    key: GITHUB_TOKEN
`))
	if err != nil {
		t.Fatalf("decode valid configuration: %v", err)
	}
	if document.Settings.Profile == nil ||
		*document.Settings.Profile != "engineering@v1" {
		t.Fatalf("profile = %#v", document.Settings.Profile)
	}
	if _, ok := document.Plugins["acme.example"]; !ok {
		t.Fatalf("plugins = %#v", document.Plugins)
	}
	if document.SecretReferences["github"].Key != "GITHUB_TOKEN" {
		t.Fatalf("secret references = %#v", document.SecretReferences)
	}
	defaults := DefaultDocument()
	if defaults.Settings.Profile == nil ||
		*defaults.Settings.Profile != "default@v1" {
		t.Fatalf("default profile selector = %#v", defaults.Settings.Profile)
	}

	for _, test := range []struct {
		name       string
		content    string
		secretText string
	}{
		{
			name: "unsupported schema version",
			content: `
schema_version: aidb.config/v2
`,
		},
		{
			name: "unversioned profile",
			content: `
schema_version: aidb.config/v1
settings:
  profile: engineering
`,
		},
		{
			name: "empty profile",
			content: `
schema_version: aidb.config/v1
settings:
  profile: ""
`,
		},
		{
			name: "unsupported conformance policy",
			content: `
schema_version: aidb.config/v1
settings:
  policy:
    conformance: repair-everything
`,
		},
		{
			name: "negative freshness ttl",
			content: `
schema_version: aidb.config/v1
settings:
  policy:
    freshness:
      ttl: -1h
`,
		},
		{
			name: "duplicate tag",
			content: `
schema_version: aidb.config/v1
settings:
  tags: [shared, shared]
`,
		},
		{
			name: "duplicate search path",
			content: `
schema_version: aidb.config/v1
settings:
  search_paths: [docs, docs]
`,
		},
		{
			name: "duplicate template id",
			content: `
schema_version: aidb.config/v1
settings:
  templates:
    - id: design
      version: v1
      source: workspace
    - id: design
      version: v2
      source: workspace
`,
		},
		{
			name: "unknown core field",
			content: `
schema_version: aidb.config/v1
settings:
  mystery: true
`,
		},
		{
			name: "unscoped plugin",
			content: `
schema_version: aidb.config/v1
plugins:
  unscoped:
    enabled: true
`,
		},
		{
			name: "literal secret value",
			content: `
schema_version: aidb.config/v1
secret_references:
  github:
    provider: environment
    key: GITHUB_TOKEN
    value: top-secret-value
`,
			secretText: "top-secret-value",
		},
		{
			name: "literal plugin secret",
			content: `
schema_version: aidb.config/v1
plugins:
  acme.example:
    api_token: top-secret-value
`,
			secretText: "top-secret-value",
		},
		{
			name: "multiple yaml documents",
			content: `
schema_version: aidb.config/v1
---
schema_version: aidb.config/v1
			`,
		},
		{
			name: "duplicate keyed environment file",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    allow: [GITHUB_TOKEN]
    gitignored: true
  - path: .env
    allow: [OTHER_TOKEN]
    gitignored: true
`,
		},
		{
			name: "empty lock reason",
			content: `
schema_version: aidb.config/v1
locks:
  settings.profile: ""
`,
		},
		{
			name: "unsupported lock path",
			content: `
schema_version: aidb.config/v1
locks:
  settings.unknown: policy
`,
		},
		{
			name: "overlapping lock paths",
			content: `
schema_version: aidb.config/v1
locks:
  settings.policy: parent policy
  settings.policy.conformance: child policy
`,
		},
		{
			name: "duplicate reset path",
			content: `
schema_version: aidb.config/v1
reset:
  - settings.profile
  - settings.profile
`,
		},
		{
			name: "invalid environment secret key",
			content: `
schema_version: aidb.config/v1
secret_references:
  github:
    provider: environment
    key: github-token
`,
		},
		{
			name: "empty environment allowlist",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    gitignored: true
`,
		},
		{
			name: "duplicate environment allowlist entry",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    allow: [TOKEN, TOKEN]
    gitignored: true
`,
		},
		{
			name: "invalid environment allowlist entry",
			content: `
schema_version: aidb.config/v1
environment_files:
  - path: .env
    allow: [not-a-variable]
    gitignored: true
`,
		},
		{
			name: "empty machine binding",
			content: `
schema_version: aidb.config/v1
machine_bindings:
  editor: {}
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeDocument(strings.NewReader(test.content))
			if err == nil {
				t.Fatal("invalid configuration was accepted")
			}
			if test.secretText != "" &&
				strings.Contains(err.Error(), test.secretText) {
				t.Fatalf("decode error leaked secret value: %v", err)
			}
		})
	}
}
