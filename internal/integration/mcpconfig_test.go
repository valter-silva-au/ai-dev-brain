package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// These tests cover MCP config discovery — the half of `adb mcp check` that decides
// WHICH servers exist and WHERE each one is configured.
//
// Two properties are load-bearing and are pinned deliberately rather than
// incidentally:
//
//   - PRECEDENCE. A name configured in several files must be reported once, from
//     the most specific file. Get this wrong and `adb mcp check` blames the wrong
//     config for a broken entry, which is the whole reason the command records a
//     Source at all.
//   - ENV KEYS ONLY, NEVER VALUES. An MCP `env` block routinely holds tokens and
//     MCPServer is marshalled to JSON, so a value leaking into the struct is a
//     secret-disclosure bug, not a cosmetic one. See
//     TestDiscoverMCPServers_EnvCarriesKeysNeverValues.
//   - NOR IN A URL OR IN ARGS. `env` was not the only place a credential hides: a
//     url carries basic-auth userinfo and `?api_key=`, and stdio args carry
//     `--api-key`/`--header`. Both are redacted at this boundary (RedactURL /
//     RedactArgs), and MCPServer.URL — the raw value the probe needs — is
//     `json:"-"` so it cannot be marshalled. See
//     TestDiscoverMCPServers_URLAndArgsNeverCarrySecrets, TestRedactURL,
//     TestRedactArgs.
//   - ONE BAD ENTRY NEVER COSTS A FILE ITS GOOD ONES. ~/.claude.json is keyed by
//     every directory the user has ever opened, so schema drift under an
//     unrelated project must not fail the whole file's parse. See
//     TestDiscoverMCPServers_BrokenProjectEntry.
//
// Every path is injected through MCPDiscoveryOptions and every fixture lives in a
// t.TempDir(), so no test here can read the developer's real ~/.claude.json. (If
// one ever did, it would see their five real servers — that is the tell that
// isolation broke.)

// The four candidate config paths, in the PRECEDENCE order candidateSources emits
// them. Indexes into the []MCPConfigSource that DiscoverMCPServers returns.
const (
	srcProject    = iota // <workspace>/.mcp.json
	srcClaudeCode        // ~/.claude.json
	srcDesktopXDG        // ~/.config/Claude/claude_desktop_config.json
	srcDesktopMac        // ~/Library/Application Support/Claude/claude_desktop_config.json
	numSources
)

func projectPath(ws string) string   { return filepath.Join(ws, ".mcp.json") }
func claudeCodePath(h string) string { return filepath.Join(h, ".claude.json") }
func desktopXDGPath(h string) string {
	return filepath.Join(h, ".config", "Claude", "claude_desktop_config.json")
}
func desktopMacPath(h string) string {
	return filepath.Join(h, "Library", "Application Support", "Claude", "claude_desktop_config.json")
}

// writeConfig plants a config file, creating its parent directories.
func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mustJSON marshals a fixture map. Config bodies are built rather than written as
// string literals because a Windows workspace path is a JSON-escaping hazard when
// it is used as a projects[<key>] key.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(data)
}

// serversConfig is the ordinary `{"mcpServers": {...}}` shape both Claude Code and
// Claude Desktop use.
func serversConfig(t *testing.T, servers map[string]any) string {
	t.Helper()
	return mustJSON(t, map[string]any{"mcpServers": servers})
}

// claudeCodeConfig is ~/.claude.json: a root mcpServers block plus per-project
// blocks keyed by absolute project path.
func claudeCodeConfig(t *testing.T, root map[string]any, projectKey string, scoped map[string]any) string {
	t.Helper()
	cfg := map[string]any{"mcpServers": root}
	if projectKey != "" {
		cfg["projects"] = map[string]any{projectKey: map[string]any{"mcpServers": scoped}}
	}
	return mustJSON(t, cfg)
}

// names extracts server names in the order discovery returned them.
func names(servers []MCPServer) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		out = append(out, s.Name)
	}
	return out
}

// find returns the named server, or fails.
func find(t *testing.T, servers []MCPServer, name string) MCPServer {
	t.Helper()
	for _, s := range servers {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("server %q not discovered; got %v", name, names(servers))
	return MCPServer{}
}

// TestDiscoverMCPServers_EachSource covers every candidate config file on its own
// and then all four together. Each source must be REPORTED whether or not it
// exists — "I looked here and found nothing" is the diagnostic a user needs when
// their servers fail to show up — so every case asserts the full source report,
// not just the servers.
func TestDiscoverMCPServers_EachSource(t *testing.T) {
	t.Parallel()

	stdio := map[string]any{"command": "tool"}

	tests := []struct {
		name  string
		plant func(t *testing.T, home, ws string)
		// wantNames is the discovered server set, sorted by name.
		wantNames []string
		// wantPresent / wantCounts are indexed by the src* constants.
		wantPresent [numSources]bool
		wantCounts  [numSources]int
	}{
		{
			name: "project .mcp.json only",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{"proj": stdio}))
			},
			wantNames:   []string{"proj"},
			wantPresent: [numSources]bool{srcProject: true},
			wantCounts:  [numSources]int{srcProject: 1},
		},
		{
			name: "claude code ~/.claude.json only",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{"code": stdio}))
			},
			wantNames:   []string{"code"},
			wantPresent: [numSources]bool{srcClaudeCode: true},
			wantCounts:  [numSources]int{srcClaudeCode: 1},
		},
		{
			name: "claude desktop xdg only",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, desktopXDGPath(home), serversConfig(t, map[string]any{"deskx": stdio}))
			},
			wantNames:   []string{"deskx"},
			wantPresent: [numSources]bool{srcDesktopXDG: true},
			wantCounts:  [numSources]int{srcDesktopXDG: 1},
		},
		{
			name: "claude desktop macos library only",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, desktopMacPath(home), serversConfig(t, map[string]any{"deskm": stdio}))
			},
			wantNames:   []string{"deskm"},
			wantPresent: [numSources]bool{srcDesktopMac: true},
			wantCounts:  [numSources]int{srcDesktopMac: 1},
		},
		{
			name: "all four at once",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{"proj": stdio}))
				writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{"code": stdio}))
				writeConfig(t, desktopXDGPath(home), serversConfig(t, map[string]any{"deskx": stdio}))
				writeConfig(t, desktopMacPath(home), serversConfig(t, map[string]any{"deskm": stdio}))
			},
			// Sorted by name, not by source: code, deskm, deskx, proj.
			wantNames:   []string{"code", "deskm", "deskx", "proj"},
			wantPresent: [numSources]bool{true, true, true, true},
			wantCounts:  [numSources]int{1, 1, 1, 1},
		},
		{
			name:        "nothing configured anywhere",
			plant:       func(t *testing.T, home, ws string) {},
			wantNames:   nil,
			wantPresent: [numSources]bool{},
			wantCounts:  [numSources]int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, ws := t.TempDir(), t.TempDir()
			tc.plant(t, home, ws)

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: ws, ProjectKey: ws,
			})

			if got := names(servers); !reflect.DeepEqual(got, tc.wantNames) && !(len(got) == 0 && len(tc.wantNames) == 0) {
				t.Errorf("servers = %v, want %v", got, tc.wantNames)
			}

			// Every source is reported, always, in precedence order.
			if len(sources) != numSources {
				t.Fatalf("got %d sources, want %d (every candidate file must be reported, "+
					"including absent ones): %+v", len(sources), numSources, sources)
			}
			wantPaths := [numSources]string{
				projectPath(ws), claudeCodePath(home), desktopXDGPath(home), desktopMacPath(home),
			}
			for i, src := range sources {
				if src.Path != wantPaths[i] {
					t.Errorf("sources[%d].Path = %q, want %q (precedence order moved)", i, src.Path, wantPaths[i])
				}
				if src.Present != tc.wantPresent[i] {
					t.Errorf("sources[%d] (%s).Present = %v, want %v", i, src.Label, src.Present, tc.wantPresent[i])
				}
				if src.Servers != tc.wantCounts[i] {
					t.Errorf("sources[%d] (%s).Servers = %d, want %d", i, src.Label, src.Servers, tc.wantCounts[i])
				}
				if src.Err != "" {
					t.Errorf("sources[%d] (%s).Err = %q, want none", i, src.Label, src.Err)
				}
				if src.Label == "" {
					t.Errorf("sources[%d] has no Label; the human output prints it", i)
				}
			}
		})
	}
}

// TestDiscoverMCPServers_Precedence pins the rule the whole Source field exists
// for: a name in several configs is reported ONCE, from the most specific file,
// and the surviving entry's Source/Scope name that file.
//
// Each case plants the SAME server name in two places with a distinguishable
// command, so "which one won" is observable rather than inferred.
func TestDiscoverMCPServers_Precedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		plant func(t *testing.T, home, ws string)
		// wantSource/wantScope are computed from the fixture dirs.
		wantSource func(home, ws string) string
		wantScope  func(home, ws string) string
		// wantCommand identifies which colliding entry survived.
		wantCommand string
		// wantCounts attributes the single surviving entry to one source.
		wantCounts [numSources]int
	}{
		{
			name: "project .mcp.json beats claude code",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-project"},
				}))
				writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-claude-code"},
				}))
			},
			wantSource: func(home, ws string) string { return projectPath(ws) },
			// The WORKSPACE path, not "global": <workspace>/.mcp.json is the most
			// project-specific of the four files, and labelling its entries "global"
			// contradicted MCPServer.Scope's own contract ("global, or the project
			// path"). It also made the field useless for the one question it answers
			// — is this entry mine, or the machine's?
			wantScope:   func(home, ws string) string { return ws },
			wantCommand: "from-project",
			wantCounts:  [numSources]int{srcProject: 1},
		},
		{
			name: "claude code beats claude desktop",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-claude-code"},
				}))
				writeConfig(t, desktopXDGPath(home), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-desktop"},
				}))
			},
			wantSource:  func(home, ws string) string { return claudeCodePath(home) },
			wantScope:   func(home, ws string) string { return "global" },
			wantCommand: "from-claude-code",
			wantCounts:  [numSources]int{srcClaudeCode: 1},
		},
		{
			name: "the earlier desktop path beats the later one",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, desktopXDGPath(home), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-xdg"},
				}))
				writeConfig(t, desktopMacPath(home), serversConfig(t, map[string]any{
					"dup": map[string]any{"command": "from-library"},
				}))
			},
			wantSource:  func(home, ws string) string { return desktopXDGPath(home) },
			wantScope:   func(home, ws string) string { return "global" },
			wantCommand: "from-xdg",
			wantCounts:  [numSources]int{srcDesktopXDG: 1},
		},
		{
			// Precedence WITHIN one file. ~/.claude.json carries both a root
			// mcpServers block and per-project blocks, and the per-project one is
			// more specific — so a name in both resolves to the project entry, and
			// its Scope reports the project path rather than "global".
			//
			// This is the collision that is easiest to get backwards, because add()
			// SKIPS an already-claimed name: "more specific" means "added FIRST", so
			// the project block has to go in before the root block. Applying it
			// second reads like an override and in fact loses every collision.
			name: "claude code projects block beats its own root block",
			plant: func(t *testing.T, home, ws string) {
				writeConfig(t, claudeCodePath(home), claudeCodeConfig(t,
					map[string]any{"dup": map[string]any{"command": "from-root-block"}},
					ws,
					map[string]any{"dup": map[string]any{"command": "from-project-block"}},
				))
			},
			wantSource:  func(home, ws string) string { return claudeCodePath(home) },
			wantScope:   func(home, ws string) string { return ws },
			wantCommand: "from-project-block",
			wantCounts:  [numSources]int{srcClaudeCode: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, ws := t.TempDir(), t.TempDir()
			tc.plant(t, home, ws)

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: ws, ProjectKey: ws,
			})

			if len(servers) != 1 {
				t.Fatalf("a duplicated name must be reported ONCE; got %d: %v", len(servers), names(servers))
			}
			got := servers[0]
			if got.Command != tc.wantCommand {
				t.Errorf("surviving Command = %q, want %q (the wrong config won the collision)",
					got.Command, tc.wantCommand)
			}
			if want := tc.wantSource(home, ws); got.Source != want {
				t.Errorf("Source = %q, want %q", got.Source, want)
			}
			if want := tc.wantScope(home, ws); got.Scope != want {
				t.Errorf("Scope = %q, want %q", got.Scope, want)
			}
			for i, src := range sources {
				if src.Servers != tc.wantCounts[i] {
					t.Errorf("sources[%d] (%s).Servers = %d, want %d — the losing source must "+
						"not be credited with the entry", i, src.Label, src.Servers, tc.wantCounts[i])
				}
			}
		})
	}
}

// TestDiscoverMCPServers_ProjectScoped covers the projects[<key>] blocks of
// ~/.claude.json, which is where Claude Code keeps per-project servers. The key is
// an absolute project path, so the wrong key — or none — must contribute nothing
// rather than leaking another project's servers into this report.
func TestDiscoverMCPServers_ProjectScoped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// projectKey is what the caller passes; "" means "skip project blocks".
		projectKey func(ws string) string
		wantNames  []string
		// wantScope is the Scope of the "scoped" entry, when it is discovered.
		wantScope func(ws string) string
	}{
		{
			name:       "matching project key contributes the scoped entry",
			projectKey: func(ws string) string { return ws },
			wantNames:  []string{"root", "scoped"},
			wantScope:  func(ws string) string { return ws },
		},
		{
			name:       "non-matching project key contributes nothing",
			projectKey: func(ws string) string { return filepath.Join(ws, "some", "other", "project") },
			wantNames:  []string{"root"},
		},
		{
			name:       "empty project key skips project blocks entirely",
			projectKey: func(ws string) string { return "" },
			wantNames:  []string{"root"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, ws := t.TempDir(), t.TempDir()
			writeConfig(t, claudeCodePath(home), claudeCodeConfig(t,
				map[string]any{"root": map[string]any{"command": "root-tool"}},
				ws,
				map[string]any{"scoped": map[string]any{"command": "scoped-tool"}},
			))

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: ws, ProjectKey: tc.projectKey(ws),
			})

			if got := names(servers); !reflect.DeepEqual(got, tc.wantNames) {
				t.Fatalf("servers = %v, want %v", got, tc.wantNames)
			}
			if want := len(tc.wantNames); sources[srcClaudeCode].Servers != want {
				t.Errorf("claude code source credited %d servers, want %d",
					sources[srcClaudeCode].Servers, want)
			}
			if tc.wantScope != nil {
				scoped := find(t, servers, "scoped")
				if want := tc.wantScope(ws); scoped.Scope != want {
					t.Errorf("scoped entry Scope = %q, want the project path %q", scoped.Scope, want)
				}
			}
			// A root-level entry is always "global", whichever key was passed.
			if root := find(t, servers, "root"); root.Scope != "global" {
				t.Errorf("root entry Scope = %q, want %q", root.Scope, "global")
			}
		})
	}
}

// TestDiscoverMCPServers_BrokenSourceDoesNotHideGoodOnes covers the failure modes
// of reading a single config: absent, malformed, valid-JSON-but-not-an-object,
// empty, null, and unreadable.
//
// The invariant that matters is best-effort-per-file: a broken config records its
// error on ITS source and the other files still contribute. One unparseable file
// must never hide the servers that are fine, and must never be silently treated as
// empty either.
func TestDiscoverMCPServers_BrokenSourceDoesNotHideGoodOnes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// body is planted at <ws>/.mcp.json; nil means "do not create the file".
		body *string
		// chmod000 makes the planted file unreadable instead.
		chmod000    bool
		wantPresent bool
		// wantErrSubstr is a fragment the recorded error must contain; "" means the
		// source must record NO error.
		wantErrSubstr string
	}{
		{
			name:        "absent file is present=false with no error",
			body:        nil,
			wantPresent: false,
		},
		{
			name:          "malformed json records a parse error",
			body:          strptr(`{"mcpServers": {`),
			wantPresent:   true,
			wantErrSubstr: "parse:",
		},
		{
			name:          "valid json that is not an object records an error",
			body:          strptr(`["not", "an", "object"]`),
			wantPresent:   true,
			wantErrSubstr: "parse:",
		},
		{
			name:          "valid json scalar records an error",
			body:          strptr(`42`),
			wantPresent:   true,
			wantErrSubstr: "parse:",
		},
		{
			name:        "empty mcpServers object is not an error",
			body:        strptr(`{"mcpServers": {}}`),
			wantPresent: true,
		},
		{
			name:        "null mcpServers is not an error",
			body:        strptr(`{"mcpServers": null}`),
			wantPresent: true,
		},
		{
			name:        "an object with no mcpServers key at all is not an error",
			body:        strptr(`{"somethingElse": true}`),
			wantPresent: true,
		},
		{
			// Present comes from a Stat, not from the read succeeding, so an
			// unreadable file reports `present: true` ALONGSIDE its error. It used
			// to report present:false with "permission denied" — a row that
			// contradicts itself, and that made a chmod'd config look identical to
			// an absent one on the field a reader checks first. The fix a user needs
			// here ("chmod the file") is nothing like the fix for absent ("configure
			// a server"), so the two must not look the same.
			name:          "unreadable file is present WITH a permission error",
			body:          strptr(`{"mcpServers": {"hidden": {"command": "x"}}}`),
			chmod000:      true,
			wantPresent:   true,
			wantErrSubstr: "permission denied",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.chmod000 && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("chmod 000 does not deny access on windows or to root")
			}
			home, ws := t.TempDir(), t.TempDir()

			if tc.body != nil {
				writeConfig(t, projectPath(ws), *tc.body)
				if tc.chmod000 {
					if err := os.Chmod(projectPath(ws), 0o000); err != nil {
						t.Fatalf("chmod 000: %v", err)
					}
					// Restore so the temp dir can be cleaned up.
					t.Cleanup(func() { _ = os.Chmod(projectPath(ws), 0o644) })
				}
			}
			// A healthy config in a LATER source: it must still contribute.
			writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{
				"healthy": map[string]any{"command": "healthy-tool"},
			}))

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: ws, ProjectKey: ws,
			})

			if got := names(servers); !reflect.DeepEqual(got, []string{"healthy"}) {
				t.Errorf("servers = %v, want [healthy] — a broken config must not hide a good one", got)
			}
			proj := sources[srcProject]
			if proj.Present != tc.wantPresent {
				t.Errorf("project source Present = %v, want %v", proj.Present, tc.wantPresent)
			}
			switch {
			case tc.wantErrSubstr == "":
				if proj.Err != "" {
					t.Errorf("project source Err = %q, want none", proj.Err)
				}
			case !strings.Contains(proj.Err, tc.wantErrSubstr):
				t.Errorf("project source Err = %q, want it to contain %q", proj.Err, tc.wantErrSubstr)
			}
			if proj.Servers != 0 {
				t.Errorf("project source credited %d servers, want 0", proj.Servers)
			}
			if sources[srcClaudeCode].Servers != 1 {
				t.Errorf("claude code source credited %d servers, want 1", sources[srcClaudeCode].Servers)
			}
		})
	}
}

func strptr(s string) *string { return &s }

// TestDiscoverMCPServers_TransportInference walks the full inference matrix. Older
// config entries never declared a `type`, so the transport has to be inferred, and
// the interesting rows are the ones where the two signals disagree: an explicit
// `type: stdio` alongside a url is still stdio (the declaration wins), and an
// unrecognised type with a url is http (the url is the only usable signal).
//
// HomeDir is deliberately left empty: only the project file is read, so these
// cases cannot see a home directory at all.
func TestDiscoverMCPServers_TransportInference(t *testing.T) {
	t.Parallel()

	const url = "http://127.0.0.1:65535/mcp"

	tests := []struct {
		name  string
		entry map[string]any
		want  MCPTransport
	}{
		{"explicit type stdio", map[string]any{"type": "stdio", "command": "tool"}, MCPTransportStdio},
		{"explicit type stdio wins over a url", map[string]any{"type": "stdio", "command": "tool", "url": url}, MCPTransportStdio},
		{"type sse", map[string]any{"type": "sse", "url": url}, MCPTransportHTTP},
		{"type http", map[string]any{"type": "http", "url": url}, MCPTransportHTTP},
		{"type streamable-http", map[string]any{"type": "streamable-http", "url": url}, MCPTransportHTTP},
		{"type http with no url", map[string]any{"type": "http"}, MCPTransportHTTP},
		{"url with no type", map[string]any{"url": url}, MCPTransportHTTP},
		{"command with no type", map[string]any{"command": "tool"}, MCPTransportStdio},
		{"unrecognised type with a command", map[string]any{"type": "quantum", "command": "tool"}, MCPTransportStdio},
		{"unrecognised type with a url", map[string]any{"type": "quantum", "url": url}, MCPTransportHTTP},
		{"neither command nor url nor type", map[string]any{}, MCPTransportStdio},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ws := t.TempDir()
			writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{"s": tc.entry}))

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{WorkspaceDir: ws})
			if len(sources) != 1 {
				t.Fatalf("an empty HomeDir must yield only the project source; got %d", len(sources))
			}
			if len(servers) != 1 {
				t.Fatalf("got %d servers, want 1", len(servers))
			}
			if servers[0].Transport != tc.want {
				t.Errorf("Transport = %q, want %q", servers[0].Transport, tc.want)
			}
		})
	}
}

// TestDiscoverMCPServers_CarriesCommandArgsAndURL asserts the plain passthrough
// fields survive normalisation — `adb mcp check` resolves Command and probes URL,
// so a dropped field silently becomes an "unknown" verdict.
func TestDiscoverMCPServers_CarriesCommandArgsAndURL(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
		"stdio": map[string]any{"command": "npx", "args": []string{"-y", "some-server"}},
		"remote": map[string]any{
			"type": "sse", "url": "https://example.invalid/sse",
		},
	}))

	servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{WorkspaceDir: ws})

	stdio := find(t, servers, "stdio")
	if stdio.Command != "npx" {
		t.Errorf("Command = %q, want npx", stdio.Command)
	}
	if !reflect.DeepEqual(stdio.Args, []string{"-y", "some-server"}) {
		t.Errorf("Args = %v, want [-y some-server]", stdio.Args)
	}
	if stdio.URL != "" {
		t.Errorf("URL = %q, want empty for a stdio entry", stdio.URL)
	}
	remote := find(t, servers, "remote")
	if remote.URL != "https://example.invalid/sse" {
		t.Errorf("URL = %q, want the configured url", remote.URL)
	}
	if remote.Command != "" {
		t.Errorf("Command = %q, want empty", remote.Command)
	}
}

// TestDiscoverMCPServers_EnvCarriesKeysNeverValues pins a SECURITY property, not a
// convenience: an MCP `env` block routinely holds API tokens, and MCPServer is
// both printed and marshalled to JSON by `adb mcp check --json`. So the values
// must not be carried on the struct AT ALL — not in an unexported field, not
// behind an omitempty tag — because anything present can be serialised later.
//
// Two independent assertions, because either alone is escapable: the struct's own
// text representation (%#v reaches every field, exported or not, so an
// "internal-only" copy fails this) and the marshalled JSON a consumer receives.
func TestDiscoverMCPServers_EnvCarriesKeysNeverValues(t *testing.T) {
	t.Parallel()

	// Distinctive, so a substring match cannot be a false positive.
	const (
		tokenValue  = "sk-live-DO-NOT-LEAK-8f3a91"
		secondValue = "hunter2-also-secret"
	)

	ws := t.TempDir()
	writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
		"secretive": map[string]any{
			"command": "tool",
			// Deliberately NOT in sorted order on disk.
			"env": map[string]any{
				"ZZ_LAST_KEY":     secondValue,
				"AA_API_TOKEN":    tokenValue,
				"MIDDLE_SETTING":  "3",
				"ANOTHER_SETTING": "true",
			},
		},
	}))

	servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{WorkspaceDir: ws})
	got := find(t, servers, "secretive")

	wantKeys := []string{"AA_API_TOKEN", "ANOTHER_SETTING", "MIDDLE_SETTING", "ZZ_LAST_KEY"}
	if !reflect.DeepEqual(got.EnvKeys, wantKeys) {
		t.Errorf("EnvKeys = %v, want %v (names only, sorted)", got.EnvKeys, wantKeys)
	}
	if !sort.StringsAreSorted(got.EnvKeys) {
		t.Errorf("EnvKeys = %v is not sorted; the output order must be deterministic", got.EnvKeys)
	}

	leaks := []string{tokenValue, secondValue}

	// 1. The struct itself carries no value, in any field.
	dump := fmt.Sprintf("%#v", got)
	for _, leak := range leaks {
		if strings.Contains(dump, leak) {
			t.Errorf("MCPServer carries the env VALUE %q (%%#v = %s) — env values must never "+
				"be held on this struct; it is printed and marshalled", leak, dump)
		}
	}

	// 2. Neither does the JSON a consumer receives.
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal server: %v", err)
	}
	for _, leak := range leaks {
		if strings.Contains(string(data), leak) {
			t.Errorf("json.Marshal(MCPServer) leaked the env VALUE %q:\n%s", leak, data)
		}
	}
	// The KEYS must survive — the point is a usable report, not an empty one.
	if !strings.Contains(string(data), "AA_API_TOKEN") {
		t.Errorf("json.Marshal(MCPServer) dropped the env KEYS as well as the values:\n%s", data)
	}
}

// TestDiscoverMCPServers_DeterministicOrdering asserts servers come back sorted by
// name and that two runs over identical input are identical. Discovery walks Go
// maps internally, whose iteration order is randomised per run, so without the
// sorts the human output would reshuffle between invocations and a `--json` diff
// would be noise.
func TestDiscoverMCPServers_DeterministicOrdering(t *testing.T) {
	t.Parallel()
	home, ws := t.TempDir(), t.TempDir()
	writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
		"zeta":  map[string]any{"command": "z"},
		"alpha": map[string]any{"command": "a"},
		"mid":   map[string]any{"command": "m"},
	}))
	writeConfig(t, claudeCodePath(home), serversConfig(t, map[string]any{
		"beta":  map[string]any{"command": "b"},
		"omega": map[string]any{"command": "o"},
	}))

	opts := MCPDiscoveryOptions{HomeDir: home, WorkspaceDir: ws, ProjectKey: ws}
	first, firstSources := DiscoverMCPServers(opts)

	want := []string{"alpha", "beta", "mid", "omega", "zeta"}
	if got := names(first); !reflect.DeepEqual(got, want) {
		t.Errorf("servers = %v, want %v (sorted by name across sources)", got, want)
	}

	// Several runs, because map iteration order varies per range, not per process.
	for i := 0; i < 5; i++ {
		again, againSources := DiscoverMCPServers(opts)
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differed:\n first = %+v\n again = %+v", i, first, again)
		}
		if !reflect.DeepEqual(firstSources, againSources) {
			t.Fatalf("run %d source report differed:\n first = %+v\n again = %+v", i, firstSources, againSources)
		}
	}
}

// TestDiscoverMCPServers_NoPathsConfigured asserts the zero options are inert: no
// HomeDir and no WorkspaceDir means no candidate files, not a panic and not a
// fallback to the real home directory.
func TestDiscoverMCPServers_NoPathsConfigured(t *testing.T) {
	t.Parallel()
	servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{})
	if len(servers) != 0 {
		t.Errorf("got %d servers from empty options, want 0: %v", len(servers), names(servers))
	}
	if len(sources) != 0 {
		t.Errorf("got %d sources from empty options, want 0: %+v", len(sources), sources)
	}
}

// TestDefaultMCPDiscoveryOptions asserts the real-machine defaults: the home
// directory is resolved once and the workspace doubles as the ProjectKey, because
// Claude Code keys its projects blocks by absolute project path.
//
// $HOME is redirected at a temp dir so the assertion does not depend on the
// developer's actual home — and so this test cannot be t.Parallel.
func TestDefaultMCPDiscoveryOptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)        // os.UserHomeDir on unix
	t.Setenv("USERPROFILE", home) // os.UserHomeDir on windows

	ws := filepath.Join(home, "workspace")
	opts, err := DefaultMCPDiscoveryOptions(ws)
	if err != nil {
		t.Fatalf("DefaultMCPDiscoveryOptions: %v", err)
	}
	if opts.HomeDir != home {
		t.Errorf("HomeDir = %q, want %q", opts.HomeDir, home)
	}
	if opts.WorkspaceDir != ws {
		t.Errorf("WorkspaceDir = %q, want %q", opts.WorkspaceDir, ws)
	}
	if opts.ProjectKey != ws {
		t.Errorf("ProjectKey = %q, want the workspace dir %q (Claude Code keys projects "+
			"blocks by absolute project path)", opts.ProjectKey, ws)
	}
}

// TestDefaultMCPDiscoveryOptions_NoHome asserts an unresolvable home is a wrapped
// error rather than options carrying an empty HomeDir — which would silently look
// like "this machine has no user-level MCP config" and report zero servers.
func TestDefaultMCPDiscoveryOptions_NoHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir has different fallbacks on windows")
	}
	t.Setenv("HOME", "")

	opts, err := DefaultMCPDiscoveryOptions("/tmp/does-not-matter")
	if err == nil {
		t.Fatalf("want an error with no $HOME, got options %+v", opts)
	}
	if !strings.Contains(err.Error(), "resolve home directory") {
		t.Errorf("error = %q, want the wrapping context %q", err, "resolve home directory")
	}
	if opts != (MCPDiscoveryOptions{}) {
		t.Errorf("options = %+v, want the zero value alongside an error", opts)
	}
}

// TestRedactURL pins the url redaction rules. A url is the field most likely to
// carry a credential — basic-auth userinfo and a `?api_key=` query parameter are
// both ordinary ways to authenticate a remote MCP server — and it is printed on
// every row of `adb mcp check` and emitted by `--json`.
//
// The two properties worth stating separately from the individual cases:
//
//   - a url with NO secrets must come back byte-identical, or the command starts
//     lying about what is configured;
//   - a url net/url will not parse must degrade conservatively, because a
//     malformed url is exactly when the instinct to "echo it so the user can see"
//     prints the token.
//
// Userinfo is redacted by SHAPE, and the two shapes are pinned separately below
// because they are two different rules rather than one rule with an exception:
//
//   - WITH a password, the username survives (`svc:***@`) — that is the shape
//     net/http itself prints, and the username there is a login identity, so
//     keeping it makes the row recognisable and says which service account is
//     configured;
//   - with NO password, the whole userinfo goes (`***@`) — net/http's shape does
//     not apply, and the username is then the ONLY slot a credential can be in.
//     A bare-username personal access token (`https://ghp_…@host/mcp`) is the
//     dominant real-world spelling of an embedded credential.
func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty stays empty", "", ""},
		{
			"a url with no secrets is byte-identical",
			"https://example.invalid/mcp?mode=sse&v=1",
			"https://example.invalid/mcp?mode=sse&v=1",
		},
		{
			"userinfo password goes, username stays (as net/http does)",
			"https://svc:S3CR3T@example.invalid/mcp",
			"https://svc:***@example.invalid/mcp",
		},
		{
			// This case used to assert the opposite — that a bare username was
			// "nothing secret to remove" and came back byte-identical. That was
			// wrong in the one direction that matters at a redaction boundary: a
			// personal access token embedded as the username with no password
			// (`https://ghp_…@host`) is the DOMINANT real-world shape of a
			// credential in a url, and it was being printed verbatim on every
			// `adb mcp check` row and in `--json`'s `url`.
			//
			// The rule now follows the shape. Where net/http has one (a password
			// exists) we match it and keep the username. Where net/http's shape
			// does not apply, the username is the only slot a credential can be
			// in, so the whole userinfo goes — the same reasoning cloudsync's
			// StripOriginCredentials already records for http(s) origins.
			"a username with no password is a bare token: the whole userinfo goes",
			"https://ghp_DEADBEEFDEADBEEF@example.invalid/mcp",
			"https://***@example.invalid/mcp",
		},
		{
			"a bare-username token is redacted in the scheme-relative form too",
			"//ghp_DEADBEEFDEADBEEF@example.invalid/mcp",
			"//***@example.invalid/mcp",
		},
		{
			// Both halves at once: the userinfo rule and the query rule are
			// independent passes, and neither may swallow the other.
			"a bare-username token and a secret query parameter both go",
			"https://ghp_DEADBEEFDEADBEEF@example.invalid/mcp?api_key=sk-live-DEADBEEF&mode=sse",
			"https://***@example.invalid/mcp?api_key=***&mode=sse",
		},
		{
			// An EMPTY userinfo is left byte-identical, deliberately. There is no
			// credential in `https://@host` to remove, so writing `***@` would
			// invent one — inventing a secret that was never configured is the
			// same dishonesty as echoing one that was, and it is the reasoning
			// this table already applies to a valueless query parameter. The
			// byte-identical guarantee is for urls with nothing to hide, and this
			// is one.
			"an empty userinfo is left alone — there is no credential to remove",
			"https://@example.invalid/mcp",
			"https://@example.invalid/mcp",
		},
		{
			// The bare-username rule must not leak into the password shape: this
			// is the regression pin for the `svc:***@` half of the rule.
			"a password-bearing userinfo keeps its username, scheme-relative too",
			"//svc:S3CR3T@example.invalid/mcp?api_key=sk-live-DEADBEEF",
			"//svc:***@example.invalid/mcp?api_key=***",
		},
		{
			// An empty PASSWORD still redacts, unchanged from before: the colon
			// says a password slot exists, so the shape is net/http's.
			"an empty password still redacts, and the username still stays",
			"https://svc:@example.invalid/mcp",
			"https://svc:***@example.invalid/mcp",
		},
		{
			// A userinfo that is ONLY a colon has an empty username in the
			// password shape — the colon still decides, so the username slot is
			// preserved as written (empty) and the password slot is redacted.
			"a userinfo of just a colon keeps the password shape",
			"https://:S3CR3T@example.invalid/mcp",
			"https://:***@example.invalid/mcp",
		},
		{
			"api_key value goes",
			"https://example.invalid/mcp?api_key=sk-live-DEADBEEF",
			"https://example.invalid/mcp?api_key=***",
		},
		{
			// The substring rule is what catches this: the parameter is named
			// x-api-key, not api_key.
			"x-api-key is caught by substring matching",
			"https://example.invalid/mcp?x-api-key=abc123&mode=sse",
			"https://example.invalid/mcp?x-api-key=***&mode=sse",
		},
		{
			"matching is case-insensitive",
			"https://example.invalid/mcp?ACCESS_TOKEN=abc123",
			"https://example.invalid/mcp?ACCESS_TOKEN=***",
		},
		{
			"only the secret-looking parameters are touched",
			"https://example.invalid/mcp?user=bob&password=hunter2&page=2&signature=zz",
			"https://example.invalid/mcp?user=bob&password=***&page=2&signature=***",
		},
		{
			"userinfo and query together",
			"https://svc:S3CR3T@example.invalid/mcp?api_key=sk-live-DEADBEEF",
			"https://svc:***@example.invalid/mcp?api_key=***",
		},
		{
			"a fragment survives redaction of the query",
			"https://example.invalid/mcp?token=abc#section",
			"https://example.invalid/mcp?token=***#section",
		},
		{
			// A valueless parameter has nothing to hide, and an empty value cannot
			// be a credential. Both are left as written so the row stays honest.
			"a valueless parameter is untouched",
			"https://example.invalid/mcp?token",
			"https://example.invalid/mcp?token",
		},
		{
			"an empty value is untouched",
			"https://example.invalid/mcp?token=",
			"https://example.invalid/mcp?token=",
		},
		{
			"a trailing bare ? is preserved",
			"https://example.invalid/mcp?",
			"https://example.invalid/mcp?",
		},
		{
			"a percent-encoded parameter name is decoded before matching",
			"https://example.invalid/mcp?x%2Dapi%2Dkey=abc",
			"https://example.invalid/mcp?x%2Dapi%2Dkey=***",
		},
		{
			"a scheme-relative url still has its userinfo redacted",
			"//svc:S3CR3T@example.invalid/mcp",
			"//svc:***@example.invalid/mcp",
		},
		{
			"a url with no authority still has its query filtered",
			"example.invalid/mcp?token=abc",
			"example.invalid/mcp?token=***",
		},
		{
			// An invalid port makes net/url refuse the whole string. The fallback
			// cannot trust its own parse, so it drops the entire query rather than
			// filtering it, and still redacts the userinfo textually.
			"an unparseable url loses its whole query",
			"https://svc:S3CR3T@example.invalid:notaport/mcp?api_key=sk-live-DEADBEEF",
			"https://svc:***@example.invalid:notaport/mcp?***",
		},
		{
			"an unparseable url with a fragment loses the fragment",
			"https://example.invalid:notaport/mcp#api_key=sk-live-DEADBEEF",
			"https://example.invalid:notaport/mcp#***",
		},
		{
			// Nothing to drop, but the userinfo is still redacted textually.
			"an unparseable url with no query still loses its password",
			"https://svc:S3CR3T@example.invalid:notaport/mcp",
			"https://svc:***@example.invalid:notaport/mcp",
		},
		{
			// The blind fallback shares redactUserinfo, so the bare-username rule
			// has to hold on the path where net/url gave up too — that is the
			// case where echoing the input would be worst.
			"an unparseable url loses a bare-username token as well",
			"https://ghp_DEADBEEFDEADBEEF@example.invalid:notaport/mcp",
			"https://***@example.invalid:notaport/mcp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RedactURL(tc.raw); got != tc.want {
				t.Errorf("RedactURL(%q)\n got: %q\nwant: %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestRedactURL_NeverEchoesASecret is the property behind the table: whatever
// shape the url has, the secret must not survive. It is separate because the
// table asserts an exact string (which pins formatting) while this asserts the
// only thing that is actually load-bearing.
func TestRedactURL_NeverEchoesASecret(t *testing.T) {
	t.Parallel()

	const secret = "sk-live-DO-NOT-LEAK-8f3a91"
	raws := []string{
		"https://svc:" + secret + "@example.invalid/mcp",
		// A bare-username token: no password slot, so the credential IS the
		// username. This is the shape a PAT-in-a-url actually takes.
		"https://" + secret + "@example.invalid/mcp",
		"//" + secret + "@example.invalid/mcp",
		"https://" + secret + "@example.invalid/mcp?api_key=" + secret,
		"https://" + secret + "@example.invalid:nope/mcp",
		"https://example.invalid/mcp?api_key=" + secret,
		"https://example.invalid/mcp?apikey=" + secret,
		"https://example.invalid/mcp?TOKEN=" + secret,
		"https://example.invalid/mcp?x-api-key=" + secret,
		"https://example.invalid/mcp?access_token=" + secret,
		"https://example.invalid/mcp?sig=" + secret,
		"https://example.invalid/mcp?signature=" + secret,
		"https://example.invalid/mcp?secret=" + secret,
		"https://example.invalid/mcp?password=" + secret,
		"https://example.invalid/mcp?passwd=" + secret,
		"https://example.invalid/mcp?pwd=" + secret,
		"https://example.invalid/mcp?auth=" + secret,
		"https://svc:" + secret + "@example.invalid/mcp?api_key=" + secret,
		// Unparseable (invalid port), so the blunt fallback has to hold too.
		"https://svc:" + secret + "@example.invalid:nope/mcp?api_key=" + secret,
	}

	for _, raw := range raws {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if got := RedactURL(raw); strings.Contains(got, secret) {
				t.Errorf("RedactURL(%q) = %q — the credential survived", raw, got)
			}
		})
	}
}

// TestRedactArgs pins the args redaction rules. A stdio entry's args are emitted
// by `--json`, and `--api-key sk-live-…` / `--header "Authorization: Bearer …"`
// are the ordinary ways an MCP wrapper is authenticated.
//
// Nothing in adb launches a configured server, so args are stored ALREADY
// redacted — there is no consumer that needs the raw value, and a second copy of
// a token is only a second place to leak it.
func TestRedactArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"nil stays nil so json omitempty is unchanged", nil, nil},
		{"empty stays empty", []string{}, []string{}},
		{
			"ordinary args are untouched",
			[]string{"-y", "some-server", "--port", "8080"},
			[]string{"-y", "some-server", "--port", "8080"},
		},
		{
			"a secret-named flag redacts the value that follows",
			[]string{"--api-key", "sk-live-DEADBEEF"},
			[]string{"--api-key", "***"},
		},
		{
			"the joined form is redacted too",
			[]string{"--api-key=sk-live-DEADBEEF"},
			[]string{"--api-key=***"},
		},
		{
			// The one no substring rule can catch: nothing in "header" suggests a
			// secret, and its value is the bearer token.
			"--header carries a credential despite its innocent name",
			[]string{"--header", "Authorization: Bearer sk-live-DEADBEEF"},
			[]string{"--header", "***"},
		},
		{
			"--header in the joined form",
			[]string{"--header=Authorization: Bearer sk-live-DEADBEEF"},
			[]string{"--header=***"},
		},
		{
			"-k is redacted (named explicitly in review)",
			[]string{"-k", "sk-live-DEADBEEF"},
			[]string{"-k", "***"},
		},
		{
			"-u carries user:password, so its value goes",
			[]string{"-u", "svc:S3CR3T"},
			[]string{"-u", "***"},
		},
		{
			// Case is the whole signal for a short flag: curl's -H is a header,
			// while a lowercase -h is almost always help.
			"-H is a header but -h is not",
			[]string{"-H", "Authorization: Bearer x", "-h", "localhost"},
			[]string{"-H", "***", "-h", "localhost"},
		},
		{
			"a value that looks like another flag is still redacted",
			[]string{"--token", "--not-really-a-flag"},
			[]string{"--token", "***"},
		},
		{
			"a secret flag with nothing after it is harmless",
			[]string{"serve", "--token"},
			[]string{"serve", "--token"},
		},
		{
			"an empty joined value is left alone",
			[]string{"--token="},
			[]string{"--token="},
		},
		{
			"an env-style assignment is judged on its name",
			[]string{"API_KEY=sk-live-DEADBEEF", "MODE=sse"},
			[]string{"API_KEY=***", "MODE=sse"},
		},
		{
			// A url on the command line hides a credential exactly the way a
			// configured one does, so it goes through RedactURL.
			"a url argument is url-redacted",
			[]string{"--url", "https://svc:S3CR3T@h.invalid/mcp?api_key=sk-live-DEADBEEF"},
			[]string{"--url", "https://svc:***@h.invalid/mcp?api_key=***"},
		},
		{
			"a url in the joined form is url-redacted",
			[]string{"--url=https://h.invalid/mcp?token=sk-live-DEADBEEF"},
			[]string{"--url=https://h.invalid/mcp?token=***"},
		},
		{
			// RedactArgs routes urls through RedactURL, so it inherits the
			// bare-username rule: an arg is another surface `--json` emits, and a
			// PAT-as-username is as plausible on a command line as in a config.
			"a url argument with a bare-username token loses the whole userinfo",
			[]string{"--url", "https://ghp_DEADBEEFDEADBEEF@h.invalid/mcp"},
			[]string{"--url", "https://***@h.invalid/mcp"},
		},
		{
			// The flag name says the whole value is a credential, so the url is
			// dropped entirely rather than filtered — an auth endpoint can carry
			// the secret in its path, which no query filter would catch.
			"a secret-named flag whose value is a url loses the whole url",
			[]string{"--auth-url=https://h.invalid/sso/sk-live-DEADBEEF"},
			[]string{"--auth-url=***"},
		},
		{
			"a bare -- separator is not treated as a flag",
			[]string{"--", "positional"},
			[]string{"--", "positional"},
		},
		{
			"redaction does not cascade past the one value",
			[]string{"--token", "sk-live-DEADBEEF", "--verbose", "keep-me"},
			[]string{"--token", "***", "--verbose", "keep-me"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RedactArgs(tc.args)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RedactArgs(%q) = %q, want %q", tc.args, got, tc.want)
			}
			// nil-ness is part of the contract, not an implementation detail:
			// MCPServer.Args is `json:",omitempty"`.
			if (tc.args == nil) != (got == nil) {
				t.Errorf("RedactArgs turned nil-ness %v into %v", tc.args == nil, got == nil)
			}
			// The input must not be mutated in place — discovery hands it a slice
			// decoded straight out of the config.
			if tc.args != nil {
				RedactArgs(tc.args)
			}
		})
	}
}

// TestRedactArgs_DoesNotMutateInput pins that the returned slice is a copy. A
// caller that also held the original would otherwise find it silently redacted.
func TestRedactArgs_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	in := []string{"--api-key", "sk-live-DEADBEEF"}
	_ = RedactArgs(in)
	if in[1] != "sk-live-DEADBEEF" {
		t.Errorf("input was mutated: %q", in)
	}
}

// TestDiscoverMCPServers_URLAndArgsNeverCarrySecrets is the URL/Args sibling of
// TestDiscoverMCPServers_EnvCarriesKeysNeverValues, and pins the same kind of
// SECURITY property rather than a convenience.
//
// The design is asymmetric on purpose, and the assertions spell out which half is
// which:
//
//   - Args are stored REDACTED, because nothing consumes the raw value.
//   - URL keeps the RAW value, because the health probe must dial exactly what
//     was configured — and is therefore `json:"-"`, with the redacted form in
//     URLDisplay for anything that renders.
//
// So the struct dump is checked with URL deliberately cleared: that catches a
// secret hiding in ANY other field (a future copy into a detail string, an
// unexported cache) while still allowing the one field that is contractually
// allowed to hold it.
func TestDiscoverMCPServers_URLAndArgsNeverCarrySecrets(t *testing.T) {
	t.Parallel()

	// Distinctive, so a substring match cannot be a false positive.
	const (
		urlPassword = "URLPW-DO-NOT-LEAK-8f3a91"
		urlAPIKey   = "sk-live-URLKEY-DO-NOT-LEAK"
		argAPIKey   = "sk-live-ARGKEY-DO-NOT-LEAK"
		argBearer   = "sk-live-BEARER-DO-NOT-LEAK"
	)
	rawURL := "https://svc:" + urlPassword + "@remote.invalid/mcp?api_key=" + urlAPIKey

	ws := t.TempDir()
	writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
		"remote": map[string]any{"type": "sse", "url": rawURL},
		"wrapper": map[string]any{
			"command": "some-mcp-wrapper",
			"args": []string{
				"--api-key", argAPIKey,
				"--header", "Authorization: Bearer " + argBearer,
			},
		},
	}))

	servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{WorkspaceDir: ws})
	leaks := []string{urlPassword, urlAPIKey, argAPIKey, argBearer}

	remote := find(t, servers, "remote")
	// The probe's contract: the raw url survives on URL and nowhere else.
	if remote.URL != rawURL {
		t.Errorf("URL = %q, want the RAW url %q — the probe has to dial what was configured",
			remote.URL, rawURL)
	}
	wantDisplay := "https://svc:***@remote.invalid/mcp?api_key=***"
	if remote.URLDisplay != wantDisplay {
		t.Errorf("URLDisplay = %q, want %q", remote.URLDisplay, wantDisplay)
	}

	wrapper := find(t, servers, "wrapper")
	wantArgs := []string{"--api-key", "***", "--header", "***"}
	if !reflect.DeepEqual(wrapper.Args, wantArgs) {
		t.Errorf("Args = %q, want %q (stored already redacted)", wrapper.Args, wantArgs)
	}

	for _, got := range []MCPServer{remote, wrapper} {
		// 1. No field other than URL carries a secret. %#v reaches unexported
		//    fields too, so an "internal-only" copy fails this.
		sanitized := got
		sanitized.URL = "" // the one field contractually allowed to hold it
		dump := fmt.Sprintf("%#v", sanitized)
		for _, leak := range leaks {
			if strings.Contains(dump, leak) {
				t.Errorf("%s: MCPServer carries the secret %q outside URL (%%#v = %s)",
					got.Name, leak, dump)
			}
		}

		// 2. Nor does the JSON a consumer receives — URL is json:"-", Args and
		//    URLDisplay are redacted.
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal server: %v", err)
		}
		for _, leak := range leaks {
			if strings.Contains(string(data), leak) {
				t.Errorf("%s: json.Marshal(MCPServer) leaked %q:\n%s", got.Name, leak, data)
			}
		}
		// The report must still be USABLE — a redacted url, not a missing one.
		if got.URLDisplay != "" && !strings.Contains(string(data), `"url":`) {
			t.Errorf("%s: json dropped the url row entirely instead of redacting it:\n%s",
				got.Name, data)
		}
	}
}

// TestDiscoverMCPServers_BrokenProjectEntry covers the failure that made this
// whole file's "one broken config does not hide the good ones" promise false
// WITHIN a file.
//
// ~/.claude.json is a large Claude-Code-owned file keyed by EVERY directory the
// user has ever opened. Decoded as a typed map, a single malformed entry anywhere
// in it failed the entire json.Unmarshal, so schema drift under some project
// touched a year ago produced "No MCP servers configured." on a machine whose
// root block was perfectly fine.
func TestDiscoverMCPServers_BrokenProjectEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// body is built from the fixture dirs, so a Windows path is escaped by
		// encoding/json rather than by hand.
		body func(t *testing.T, ws string) string
		// wantNames is the discovered set. The root servers must survive in every
		// case — that is the whole point.
		wantNames []string
		// wantErrSubstr is "" when the source must record no error at all.
		wantErrSubstr string
	}{
		{
			// The reproduction from review, verbatim in shape: an unrelated
			// project's mcpServers is an array instead of an object.
			name: "an UNRELATED bad project entry is ignored entirely",
			body: func(t *testing.T, ws string) string {
				return mustJSON(t, map[string]any{
					"mcpServers": map[string]any{
						"a": map[string]any{"command": "/bin/ls"},
						"b": map[string]any{"command": "/bin/cat"},
					},
					"projects": map[string]any{
						"/some/other/project": map[string]any{"mcpServers": []any{}},
					},
				})
			},
			wantNames: []string{"a", "b"},
			// Nothing is wrong with THIS project, so nothing is reported. A
			// diagnostic about a directory the user is not in would be noise.
			wantErrSubstr: "",
		},
		{
			name: "a bad MATCHING project entry is reported but keeps the root servers",
			body: func(t *testing.T, ws string) string {
				return mustJSON(t, map[string]any{
					"mcpServers": map[string]any{"root": map[string]any{"command": "root-tool"}},
					"projects": map[string]any{
						ws: map[string]any{"mcpServers": []any{}},
					},
				})
			},
			wantNames:     []string{"root"},
			wantErrSubstr: "projects[",
		},
		{
			name: "a projects value that is not an object is reported, root survives",
			body: func(t *testing.T, ws string) string {
				return mustJSON(t, map[string]any{
					"mcpServers": map[string]any{"root": map[string]any{"command": "root-tool"}},
					"projects":   []any{"not", "an", "object"},
				})
			},
			wantNames:     []string{"root"},
			wantErrSubstr: "projects",
		},
		{
			name: "a null projects block is not an error",
			body: func(t *testing.T, ws string) string {
				return mustJSON(t, map[string]any{
					"mcpServers": map[string]any{"root": map[string]any{"command": "root-tool"}},
					"projects":   nil,
				})
			},
			wantNames:     []string{"root"},
			wantErrSubstr: "",
		},
		{
			name: "a good matching entry alongside a bad unrelated one still contributes",
			body: func(t *testing.T, ws string) string {
				return mustJSON(t, map[string]any{
					"mcpServers": map[string]any{"root": map[string]any{"command": "root-tool"}},
					"projects": map[string]any{
						"/some/other/project": map[string]any{"mcpServers": 42},
						ws: map[string]any{"mcpServers": map[string]any{
							"scoped": map[string]any{"command": "scoped-tool"},
						}},
					},
				})
			},
			wantNames:     []string{"root", "scoped"},
			wantErrSubstr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home, ws := t.TempDir(), t.TempDir()
			writeConfig(t, claudeCodePath(home), tc.body(t, ws))

			servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: ws, ProjectKey: ws,
			})

			if got := names(servers); !reflect.DeepEqual(got, tc.wantNames) {
				t.Errorf("servers = %v, want %v — a bad projects entry must not cost the "+
					"file its root servers", got, tc.wantNames)
			}
			src := sources[srcClaudeCode]
			if !src.Present {
				t.Errorf("source Present = false, want true")
			}
			if src.Servers != len(tc.wantNames) {
				t.Errorf("source credited %d servers, want %d", src.Servers, len(tc.wantNames))
			}
			switch {
			case tc.wantErrSubstr == "":
				if src.Err != "" {
					t.Errorf("source Err = %q, want none", src.Err)
				}
			case !strings.Contains(src.Err, tc.wantErrSubstr):
				t.Errorf("source Err = %q, want it to contain %q — a malformed block the "+
					"caller ASKED for must be surfaced", src.Err, tc.wantErrSubstr)
			}
		})
	}
}

// TestDiscoverMCPServers_SymlinkedProjectKey covers the silent failure a
// symlinked workspace used to cause.
//
// Claude Code keys projects blocks by the RESOLVED absolute path. On macOS the
// path adb holds routinely differs — /tmp is a symlink to /private/tmp, and a
// workspace under a symlinked ~/Code resolves elsewhere entirely — so using the
// key verbatim matched nothing and the whole project block vanished. With no
// diagnostic, because "this key is absent" and "this project has no servers" are
// the same observation.
func TestDiscoverMCPServers_SymlinkedProjectKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevation on windows")
	}
	t.Parallel()

	tests := []struct {
		name string
		// configKey picks which spelling the fixture config is keyed by.
		configKey func(link, resolved string) string
		// wantScope is the spelling the surviving entry should report.
		wantScope func(link, resolved string) string
	}{
		{
			// The defect: the config is keyed by the resolved path, the caller
			// holds the symlink.
			name:      "a config keyed by the RESOLVED path is found through a symlink",
			configKey: func(link, resolved string) string { return resolved },
			wantScope: func(link, resolved string) string { return resolved },
		},
		{
			// The literal key still works, and its spelling is what is reported.
			name:      "a config keyed by the LITERAL path still matches",
			configKey: func(link, resolved string) string { return link },
			wantScope: func(link, resolved string) string { return link },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()

			// A real workspace directory, plus a symlink pointing at it: the
			// symlink is the path the caller passes.
			real := filepath.Join(t.TempDir(), "real-workspace")
			if err := os.MkdirAll(real, 0o755); err != nil {
				t.Fatalf("mkdir workspace: %v", err)
			}
			link := filepath.Join(t.TempDir(), "linked-workspace")
			if err := os.Symlink(real, link); err != nil {
				t.Skipf("cannot create a symlink here: %v", err)
			}
			resolved, err := filepath.EvalSymlinks(link)
			if err != nil {
				t.Fatalf("EvalSymlinks: %v", err)
			}
			if resolved == link {
				t.Fatalf("the fixture symlink resolved to itself (%q); the test proves nothing", link)
			}

			writeConfig(t, claudeCodePath(home), claudeCodeConfig(t,
				map[string]any{"root": map[string]any{"command": "root-tool"}},
				tc.configKey(link, resolved),
				map[string]any{"scoped": map[string]any{"command": "scoped-tool"}},
			))

			servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{
				HomeDir: home, WorkspaceDir: link, ProjectKey: link,
			})

			if got := names(servers); !reflect.DeepEqual(got, []string{"root", "scoped"}) {
				t.Fatalf("servers = %v, want [root scoped] — the project block was lost", got)
			}
			scoped := find(t, servers, "scoped")
			if want := tc.wantScope(link, resolved); scoped.Scope != want {
				t.Errorf("Scope = %q, want %q", scoped.Scope, want)
			}
		})
	}
}

// TestDiscoverMCPServers_LiteralProjectKeyWinsOverResolved pins which spelling
// wins when a config carries BOTH, so the outcome is deterministic rather than a
// function of map iteration order.
func TestDiscoverMCPServers_LiteralProjectKeyWinsOverResolved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevation on windows")
	}
	t.Parallel()

	home := t.TempDir()
	real := filepath.Join(t.TempDir(), "real-workspace")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	link := filepath.Join(t.TempDir(), "linked-workspace")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	writeConfig(t, claudeCodePath(home), mustJSON(t, map[string]any{
		"projects": map[string]any{
			link: map[string]any{"mcpServers": map[string]any{
				"dup": map[string]any{"command": "from-literal-key"},
			}},
			resolved: map[string]any{"mcpServers": map[string]any{
				"dup": map[string]any{"command": "from-resolved-key"},
			}},
		},
	}))

	servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{
		HomeDir: home, WorkspaceDir: link, ProjectKey: link,
	})

	if len(servers) != 1 {
		t.Fatalf("a name in both key spellings must be reported ONCE; got %v", names(servers))
	}
	if servers[0].Command != "from-literal-key" {
		t.Errorf("Command = %q, want from-literal-key (the literal key is tried first, so "+
			"Scope reports the spelling the config used)", servers[0].Command)
	}
	if servers[0].Scope != link {
		t.Errorf("Scope = %q, want %q", servers[0].Scope, link)
	}
}

// TestDiscoverMCPServers_ToleratesUTF8BOM covers a BOM-prefixed config, which
// encoding/json rejects outright ("invalid character") — a message that reads
// like a corrupt config rather than an encoding detail. A .mcp.json is plausibly
// hand-written or emitted by a Windows tool, and this repo already tolerates a
// BOM in core.parseArtifactFrontmatter (L600 §8), so the behaviour is consistent
// rather than novel.
func TestDiscoverMCPServers_ToleratesUTF8BOM(t *testing.T) {
	t.Parallel()

	// Built from bytes rather than written as a literal: a BOM in source is
	// invisible and the next editor to touch this line would lose it.
	bom := string([]byte{0xEF, 0xBB, 0xBF})

	ws := t.TempDir()
	writeConfig(t, projectPath(ws), bom+serversConfig(t, map[string]any{
		"bommed": map[string]any{"command": "tool"},
	}))

	servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{WorkspaceDir: ws})

	if got := names(servers); !reflect.DeepEqual(got, []string{"bommed"}) {
		t.Errorf("servers = %v, want [bommed]", got)
	}
	if sources[srcProject].Err != "" {
		t.Errorf("source Err = %q, want none — a leading BOM is stripped, not a parse failure",
			sources[srcProject].Err)
	}
	if sources[srcProject].Servers != 1 {
		t.Errorf("source credited %d servers, want 1", sources[srcProject].Servers)
	}
}

// TestDiscoverMCPServers_ScopeNamesTheRightThing asserts the Scope contract
// across all three shapes at once: the project file is scoped to the workspace,
// a projects[<key>] entry to that key, and a user-level file to "global". The
// field's doc says "global, or the project path", and <workspace>/.mcp.json — the
// most project-specific file of the four — used to say "global".
func TestDiscoverMCPServers_ScopeNamesTheRightThing(t *testing.T) {
	t.Parallel()

	home, ws := t.TempDir(), t.TempDir()
	writeConfig(t, projectPath(ws), serversConfig(t, map[string]any{
		"from-project-file": map[string]any{"command": "p"},
	}))
	writeConfig(t, claudeCodePath(home), claudeCodeConfig(t,
		map[string]any{"from-user-root": map[string]any{"command": "u"}},
		ws,
		map[string]any{"from-project-block": map[string]any{"command": "s"}},
	))
	writeConfig(t, desktopXDGPath(home), serversConfig(t, map[string]any{
		"from-desktop": map[string]any{"command": "d"},
	}))

	servers, _ := DiscoverMCPServers(MCPDiscoveryOptions{
		HomeDir: home, WorkspaceDir: ws, ProjectKey: ws,
	})

	wantScopes := map[string]string{
		"from-project-file":  ws, // the project's own .mcp.json
		"from-project-block": ws, // ~/.claude.json projects[<ws>]
		"from-user-root":     "global",
		"from-desktop":       "global",
	}
	for name, want := range wantScopes {
		if got := find(t, servers, name).Scope; got != want {
			t.Errorf("%s: Scope = %q, want %q", name, got, want)
		}
	}
}

// TestDiscoverMCPServers_SeveralProblemsInOneFileAreAllReported pins that a
// second problem in the same file does not overwrite the first. Two malformed
// project blocks can match one workspace (the literal path and its
// symlink-resolved form), and a report that mentioned only one of them would send
// the reader to fix half the file.
func TestDiscoverMCPServers_SeveralProblemsInOneFileAreAllReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevation on windows")
	}
	t.Parallel()

	home := t.TempDir()
	real := filepath.Join(t.TempDir(), "real-workspace")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	link := filepath.Join(t.TempDir(), "linked-workspace")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Both spellings present, both malformed, root block fine.
	writeConfig(t, claudeCodePath(home), mustJSON(t, map[string]any{
		"mcpServers": map[string]any{"root": map[string]any{"command": "root-tool"}},
		"projects": map[string]any{
			link:     map[string]any{"mcpServers": []any{}},
			resolved: map[string]any{"mcpServers": 42},
		},
	}))

	servers, sources := DiscoverMCPServers(MCPDiscoveryOptions{
		HomeDir: home, WorkspaceDir: link, ProjectKey: link,
	})

	if got := names(servers); !reflect.DeepEqual(got, []string{"root"}) {
		t.Errorf("servers = %v, want [root] — two broken project blocks must still leave "+
			"the root block intact", got)
	}
	gotErr := sources[srcClaudeCode].Err
	for _, key := range []string{link, resolved} {
		if !strings.Contains(gotErr, key) {
			t.Errorf("Err = %q, want it to mention the malformed key %q — a second problem "+
				"must not overwrite the first", gotErr, key)
		}
	}
}
