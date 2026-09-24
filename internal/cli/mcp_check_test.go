package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// These tests cover `adb mcp check`: the per-server verdict, the rendering, the
// --json contract, --exit-code, and the command resolver.
//
// They are hermetic by construction, and that is the whole point of the exercise.
// The implementation this replaced ran exec.LookPath("npx") and an HTTP GET to
// localhost:3000 per server, so its test could only assert the no-config branch —
// anything else made the result a function of the developer's installed toolchain
// and of whatever happened to be listening locally. Three seams remove that:
//
//	mcpCheckDiscover  — swapped for a fixture, so no real ~/.claude.json is read
//	                    (the developer's has five servers; if a test here ever
//	                    saw them, the isolation is broken)
//	mcpCheckResolver  — swapped for a fake, so no PATH lookup depends on what is
//	                    installed
//	mcpCheckNewClient — swapped for a fake, so the http half of the command
//	                    (including the --no-cache wiring) is reachable with no
//	                    listener and no network
//
// All three are swapped exactly like taskRunWithRufloCommander in
// launch_events_test.go. No test here reaches the network.
//
// No t.Parallel anywhere in this file: `App` is a package-level singleton,
// captureStdout reassigns os.Stdout, and two tests use t.Chdir/t.Setenv — so these
// tests must stay serial (same barrier as hook_options_test.go).

// fakeResolver answers command resolution from a fixture instead of the machine's
// PATH.
type fakeResolver struct {
	// resolved maps a command to the path it resolves to.
	resolved map[string]string
	// errs maps a command to the failure message the resolver returns.
	errs map[string]string
	// calls records what was asked for, in order — the old implementation ignored
	// the server's command entirely, so "was the RIGHT command resolved?" is worth
	// asserting.
	calls []string
	// baseDirs records the base directory each call was given, so the cwd-
	// independence contract (a relative command resolves against its config file's
	// directory) is observable without touching the filesystem.
	baseDirs []string
}

func (f *fakeResolver) Resolve(command, baseDir string) (string, error) {
	f.calls = append(f.calls, command)
	f.baseDirs = append(f.baseDirs, baseDir)
	if msg, ok := f.errs[command]; ok {
		return "", errors.New(msg)
	}
	if path, ok := f.resolved[command]; ok {
		return path, nil
	}
	return "", fmt.Errorf("command not found: %s", command)
}

var _ commandResolver = (*fakeResolver)(nil)
var _ commandResolver = execCommandResolver{}

// fakeMCPClient answers health probes from a fixture. MCPClient is an interface, so
// the http verdicts are reachable with no listener and no network.
//
// It is mutex-guarded because probes now run CONCURRENTLY: without the lock the
// bookkeeping below is a data race that -race would (correctly) fail on, and the
// test fixture would be reporting a problem it created itself.
type fakeMCPClient struct {
	mu      sync.Mutex
	health  map[string]integration.MCPHealthCheck
	errs    map[string]error
	probes  []string
	cleared int
	ttl     time.Duration
	// events is the ORDERED call log, so a decorator that clears the cache at the
	// wrong moment (after the probe rather than before it) is distinguishable from
	// one that gets it right.
	events []string
}

func (f *fakeMCPClient) CheckHealth(serverURL string) (integration.MCPHealthCheck, error) {
	f.mu.Lock()
	f.probes = append(f.probes, serverURL)
	f.events = append(f.events, "probe:"+serverURL)
	health, hasHealth := f.health[serverURL]
	err, hasErr := f.errs[serverURL]
	f.mu.Unlock()

	if hasErr {
		return integration.MCPHealthCheck{ServerURL: serverURL, Error: err}, err
	}
	if hasHealth {
		return health, health.Error
	}
	return integration.MCPHealthCheck{ServerURL: serverURL}, fmt.Errorf("no fixture for %s", serverURL)
}

func (f *fakeMCPClient) ClearCache() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
	f.events = append(f.events, "clear")
}

func (f *fakeMCPClient) SetTTL(ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ttl = ttl
}

// snapshot returns a copy of the mutable bookkeeping, so an assertion never reads
// it while a worker is still writing.
func (f *fakeMCPClient) snapshot() (probes []string, events []string, cleared int, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.probes...), append([]string(nil), f.events...), f.cleared, f.ttl
}

var _ integration.MCPClient = (*fakeMCPClient)(nil)

// withMCPCheckSeams points discovery at a fixture and command resolution at a fake,
// restoring both on cleanup. It also returns a pointer to the workspace dir the
// command passed to discovery, so a test can assert the command hands over
// App.BasePath (which is what makes the project .mcp.json and the
// projects[<workspace>] block resolve to the right project).
func withMCPCheckSeams(t *testing.T, servers []integration.MCPServer, sources []integration.MCPConfigSource, res commandResolver) *string {
	t.Helper()
	origDiscover, origResolver := mcpCheckDiscover, mcpCheckResolver
	var gotWorkspace string
	mcpCheckDiscover = func(workspaceDir string) ([]integration.MCPServer, []integration.MCPConfigSource, error) {
		gotWorkspace = workspaceDir
		return servers, sources, nil
	}
	mcpCheckResolver = res
	t.Cleanup(func() {
		mcpCheckDiscover = origDiscover
		mcpCheckResolver = origResolver
	})
	return &gotWorkspace
}

// withMCPCheckClient swaps the http-prober constructor for one that always hands
// back the given fake, and returns a pointer to the TTLs it was asked to build
// with. An empty slice therefore proves the stronger property: nothing constructed
// a client at all, so nothing could have touched the network.
func withMCPCheckClient(t *testing.T, client integration.MCPClient) *[]time.Duration {
	t.Helper()
	orig := mcpCheckNewClient
	var built []time.Duration
	mcpCheckNewClient = func(ttl time.Duration) integration.MCPClient {
		built = append(built, ttl)
		return client
	}
	t.Cleanup(func() { mcpCheckNewClient = orig })
	return &built
}

// stdioServer / httpServer / brokenServer build fixture entries.
func stdioServer(name, command string) integration.MCPServer {
	return integration.MCPServer{
		Name: name, Source: "/fixture/.mcp.json", Scope: "global",
		Transport: integration.MCPTransportStdio, Command: command,
	}
}

// httpServer builds a credential-free http entry, so URL and URLDisplay agree.
// Discovery always fills URLDisplay; a fixture that left it empty would be
// exercising the defensive fallback in mcpDisplayURL rather than the real path.
func httpServer(name, url string) integration.MCPServer {
	return integration.MCPServer{
		Name: name, Source: "/fixture/.mcp.json", Scope: "global",
		Transport: integration.MCPTransportHTTP, URL: url,
		URLDisplay: integration.RedactURL(url),
	}
}

func brokenServer(name string) integration.MCPServer {
	return integration.MCPServer{
		Name: name, Source: "/fixture/.mcp.json", Scope: "global",
		Transport: integration.MCPTransportStdio,
	}
}

// TestCheckMCPServers_StateMatrix walks every verdict `adb mcp check` can reach and
// asserts BOTH renderings of it: the human line and the --json row. A verdict that
// classifies correctly but prints the wrong glyph or drops the actionable detail is
// still a broken command — the failure this replaced printed a plausible verdict
// for every server and told the user nothing.
func TestCheckMCPServers_StateMatrix(t *testing.T) {
	const (
		okURL          = "http://192.0.2.10:8080/mcp"  // TEST-NET-1, never routed
		serverErrURL   = "http://192.0.2.11:8080/mcp"  // and never probed for real:
		unreachableURL = "http://192.0.2.12:59999/mcp" // the client below is a fake
		silentURL      = "http://192.0.2.13:8080/mcp"  //
		methodURL      = "http://192.0.2.14:8080/mcp"  //
		redirectURL    = "https://192.0.2.15:8443/mcp" //
		redirectSSOURL = "https://192.0.2.16:8443/mcp" //
		// ssoDestination is what the redirector's `Location` header named. It arrives
		// on MCPHealthCheck.Location ALREADY REDACTED (see integration.RedactLocation),
		// so a fixture spells it the way the probe would hand it over.
		ssoDestination = "https://idp.example.invalid/authorize?client_id=adb&state=***"
	)

	res := &fakeResolver{
		resolved: map[string]string{
			"npx":                     "/fake/bin/npx",
			"/fake/opt/absolute-tool": "/fake/opt/absolute-tool",
		},
		errs: map[string]string{
			"definitely-not-installed": "command not found: definitely-not-installed",
		},
	}
	client := &fakeMCPClient{
		health: map[string]integration.MCPHealthCheck{
			okURL: {ServerURL: okURL, Healthy: true, StatusCode: 200},
			// A REAL Streamable-HTTP MCP endpoint answers a bare GET with 405: the
			// probe sends no `Accept: text/event-stream`, so refusing it is correct
			// behaviour by a server that is working perfectly.
			methodURL: {ServerURL: methodURL, Healthy: false, StatusCode: 405,
				Error: errors.New("unhealthy status code: 405")},
			serverErrURL: {ServerURL: serverErrURL, Healthy: false, StatusCode: 503,
				Error: errors.New("unhealthy status code: 503")},
			// A url that 302s to an SSO login page. The client no longer follows the
			// redirect, so the 302 itself is what comes back — here WITHOUT a Location,
			// which is malformed but emitted in the wild, and is the row that pins the
			// destination-less wording.
			redirectURL: {ServerURL: redirectURL, Healthy: false, StatusCode: 302},
			// The same redirect, with the destination its `Location` header named.
			redirectSSOURL: {ServerURL: redirectSSOURL, Healthy: false, StatusCode: 302,
				Location: ssoDestination},
			// Neither healthy, nor a status code, nor an error.
			silentURL: {ServerURL: silentURL, Healthy: false},
		},
		errs: map[string]error{
			unreachableURL: fmt.Errorf("failed to connect to MCP server: Get %q: dial tcp 192.0.2.12:59999: connect: connection refused", unreachableURL),
		},
	}

	// The two note halves, spelled once. The note is transport-aware: an http-only
	// report used to claim it had verified a command resolved, which nothing had.
	const (
		stdioNote = "note: verifies each command resolves to an executable, not that the server responds"
		httpNote  = "note: verifies each url answered, not that an MCP server is behind it"
	)

	tests := []struct {
		name   string
		server integration.MCPServer
		// client is the injected health prober; nil exercises the no-network path.
		client integration.MCPClient
		// wantState / wantDetail / wantResolved are the classification.
		wantState    mcpCheckState
		wantDetail   string
		wantResolved string
		// wantLine are substrings the rendered human line must contain.
		wantLine []string
		// wantNote is the limits note the report must end with; "" means the report
		// must carry NO note, because nothing was verified.
		wantNote string
	}{
		{
			name:         "launchable via a PATH lookup",
			server:       stdioServer("path-tool", "npx"),
			client:       client,
			wantState:    mcpStateLaunchable,
			wantResolved: "/fake/bin/npx",
			// The `name → path` form is what tells a PATH lookup apart from an
			// already-absolute command.
			wantLine: []string{"✓", "path-tool", "stdio", "npx → /fake/bin/npx"},
			wantNote: stdioNote,
		},
		{
			name:         "launchable via an absolute command",
			server:       stdioServer("abs-tool", "/fake/opt/absolute-tool"),
			client:       client,
			wantState:    mcpStateLaunchable,
			wantResolved: "/fake/opt/absolute-tool",
			// Command == Resolved, so the arrow form would just repeat itself.
			wantLine: []string{"✓", "abs-tool", "stdio", "/fake/opt/absolute-tool"},
			wantNote: stdioNote,
		},
		{
			name:       "unresolved when the command is not on PATH",
			server:     stdioServer("gone", "definitely-not-installed"),
			client:     client,
			wantState:  mcpStateUnresolved,
			wantDetail: "command not found: definitely-not-installed",
			wantLine:   []string{"✗", "gone", "stdio", "command not found: definitely-not-installed"},
			wantNote:   stdioNote,
		},
		{
			name:       "reachable when the url answers 200, annotated with the status",
			server:     httpServer("remote-up", okURL),
			client:     client,
			wantState:  mcpStateReachable,
			wantDetail: okURL + ": status 200",
			wantLine:   []string{"✓", "remote-up", "http", okURL, "status 200"},
			wantNote:   httpNote,
		},
		{
			// The regression this policy exists for: a working Streamable-HTTP
			// endpoint answering 405 to a bare GET was reported ✗ unreachable, which
			// sent people hunting a server that was fine.
			name:       "a 405 is a CORRECT answer from a real endpoint, so it is reachable",
			server:     httpServer("remote-405", methodURL),
			client:     client,
			wantState:  mcpStateReachable,
			wantDetail: methodURL + ": status 405",
			wantLine:   []string{"✓", "remote-405", "http", "status 405"},
			wantNote:   httpNote,
		},
		{
			// Reachable, because something answered — the status is what says it is
			// not serving. Without a handshake there is no stronger honest claim.
			name:       "a 503 answered, so it is reachable with the status shown",
			server:     httpServer("remote-503", serverErrURL),
			client:     client,
			wantState:  mcpStateReachable,
			wantDetail: serverErrURL + ": status 503",
			wantLine:   []string{"✓", "remote-503", "http", "status 503"},
			wantNote:   httpNote,
		},
		{
			// The other half of the old bug: this used to follow the redirect and
			// report ✓ reachable off an SSO login page's 200. Reachable is still the
			// verdict — something answered — but the report must SAY it redirected,
			// because a login redirect means the MCP server was never reached.
			// A 3xx that carried NO Location header. Malformed, but real — so the row
			// keeps the generic explanation rather than printing an empty arrow, and
			// this is the case that must not regress when a destination is available.
			name:      "a 3xx with no destination is reachable but must say it redirected",
			server:    httpServer("remote-sso", redirectURL),
			client:    client,
			wantState: mcpStateReachable,
			wantDetail: redirectURL + ": status 302 (redirect — the response came from " +
				"the redirector, typically an SSO login page, not from an MCP server)",
			wantLine: []string{"✓", "remote-sso", "http", "status 302", "redirect"},
			wantNote: httpNote,
		},
		{
			// The gap this closes: the probe held the `Location` header and the report
			// could not say where the redirect went, which is the single most useful
			// fact about it — "it sent you to your identity provider" and "it sent you
			// to a path you mistyped" are different problems, and `status 302` alone
			// separates neither. The explanation survives alongside the destination,
			// because knowing WHERE it went does not by itself tell a reader that the
			// answer did not come from their MCP server.
			name:      "a 3xx names its destination when the response gave one",
			server:    httpServer("remote-sso-dest", redirectSSOURL),
			client:    client,
			wantState: mcpStateReachable,
			wantDetail: redirectSSOURL + ": status 302 (redirect → " + ssoDestination +
				" — the response came from the redirector, not from an MCP server)",
			wantLine: []string{"✓", "remote-sso-dest", "http", "status 302",
				"redirect → " + ssoDestination, "not from an MCP server"},
			wantNote: httpNote,
		},
		{
			// Unhealthy with no status code and no error at all: there is nothing to
			// add, so the detail is the bare url rather than an invented reason.
			name:       "no response and nothing to add reports just the url",
			server:     httpServer("remote-silent", silentURL),
			client:     client,
			wantState:  mcpStateUnreachable,
			wantDetail: silentURL,
			wantLine:   []string{"✗", "remote-silent", "http", silentURL},
			wantNote:   httpNote,
		},
		{
			name:      "unreachable on a transport error, concisely",
			server:    httpServer("remote-down", unreachableURL),
			client:    client,
			wantState: mcpStateUnreachable,
			// The wrapper layers are trimmed; see TestMCPCheck_ConciseProbeError.
			wantDetail: unreachableURL + ": connection refused",
			wantLine:   []string{"✗", "remote-down", "http", "connection refused"},
			wantNote:   httpNote,
		},
		{
			name:       "unknown when the entry declares neither a command nor a url",
			server:     brokenServer("malformed"),
			client:     client,
			wantState:  mcpStateUnknown,
			wantDetail: "entry declares neither a command nor a url",
			wantLine:   []string{"?", "malformed", "entry declares neither a command nor a url"},
			// Nothing was verified, so there is nothing to qualify.
			wantNote: "",
		},
		{
			// A nil client is how the command guarantees "no http servers ⇒ no
			// network calls". An http server then reports unknown/not probed rather
			// than a verdict it did not earn — and earns no note either.
			name:       "unknown when there is no http client to probe with",
			server:     httpServer("remote-unprobed", okURL),
			client:     nil,
			wantState:  mcpStateUnknown,
			wantDetail: "not probed",
			wantLine:   []string{"?", "remote-unprobed", "http", "not probed"},
			wantNote:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			statuses := checkMCPServers([]integration.MCPServer{tc.server}, res, tc.client)
			if len(statuses) != 1 {
				t.Fatalf("got %d statuses, want 1", len(statuses))
			}
			got := statuses[0]
			if got.State != tc.wantState {
				t.Errorf("State = %q, want %q (detail: %q)", got.State, tc.wantState, got.Detail)
			}
			if got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
			if got.Resolved != tc.wantResolved {
				t.Errorf("Resolved = %q, want %q", got.Resolved, tc.wantResolved)
			}
			if got.Name != tc.server.Name {
				t.Errorf("Name = %q, want %q — the verdict must belong to the server it "+
					"was computed for", got.Name, tc.server.Name)
			}

			report := mcpCheckReport{Servers: statuses, Summary: summarize(statuses)}

			// The human rendering.
			out := captureStdout(t, func() { printMCPCheck(report) })
			for _, want := range tc.wantLine {
				if !strings.Contains(out, want) {
					t.Errorf("human output missing %q:\n%s", want, out)
				}
			}
			if tc.wantNote == "" {
				if strings.Contains(out, "note:") {
					t.Errorf("nothing was verified, so the report must carry no limits "+
						"note:\n%s", out)
				}
			} else if !strings.Contains(out, tc.wantNote) {
				t.Errorf("human output missing the limits note %q — a ✓ must not be "+
					"allowed to imply more than was checked:\n%s", tc.wantNote, out)
			}

			// The --json row for the same verdict.
			row := marshalFirstServerRow(t, report)
			if row["state"] != string(tc.wantState) {
				t.Errorf("json state = %v, want %q", row["state"], tc.wantState)
			}
			if tc.wantDetail == "" {
				if _, present := row["detail"]; present {
					t.Errorf("json row carries an empty detail (omitempty): %v", row)
				}
			} else if row["detail"] != tc.wantDetail {
				t.Errorf("json detail = %v, want %q", row["detail"], tc.wantDetail)
			}
			if tc.wantResolved == "" {
				if _, present := row["resolved"]; present {
					t.Errorf("json row carries an empty resolved (omitempty): %v", row)
				}
			} else if row["resolved"] != tc.wantResolved {
				t.Errorf("json resolved = %v, want %q", row["resolved"], tc.wantResolved)
			}
			// The embedded MCPServer fields must survive into the row — a consumer
			// reads the verdict and the entry together.
			if row["name"] != tc.server.Name {
				t.Errorf("json name = %v, want %q", row["name"], tc.server.Name)
			}
			if row["transport"] != string(tc.server.Transport) {
				t.Errorf("json transport = %v, want %q", row["transport"], tc.server.Transport)
			}
			// `url` in JSON is URLDisplay, never the raw URL (which is json:"-").
			if tc.server.URLDisplay != "" && row["url"] != tc.server.URLDisplay {
				t.Errorf("json url = %v, want the DISPLAY url %q", row["url"], tc.server.URLDisplay)
			}
		})
	}
}

// marshalFirstServerRow round-trips a report through encoding/json and returns its
// first server row as a map, so assertions read the shape a consumer receives
// rather than the Go struct.
func marshalFirstServerRow(t *testing.T, r mcpCheckReport) map[string]any {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	var decoded struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	if len(decoded.Servers) != 1 {
		t.Fatalf("got %d server rows, want 1: %s", len(decoded.Servers), data)
	}
	return decoded.Servers[0]
}

// TestMCPCheck_CredentialURLIsNeverPrinted is the redaction boundary, asserted on
// the two surfaces that leak: the terminal and --json.
//
// An MCP url routinely carries userinfo and/or an `api_key=` query parameter. Three
// separate mistakes used to put those on screen, and each is pinned here:
//
//  1. the detail line was built from the RAW url (`s.URL`), which restored the
//     password onto the very line net/http had masked it out of;
//  2. conciseProbeError rebuilt the `Get "<url>": ` needle from the raw url, so it
//     never matched the redacted spelling net/http had actually emitted — leaving
//     the whole wrapper, and its unmasked QUERY STRING, in the message;
//  3. --json marshalled MCPServer.URL.
//
// The pre-existing coverage missed all three precisely because its fixture urls
// were credential-free.
func TestMCPCheck_CredentialURLIsNeverPrinted(t *testing.T) {
	const (
		password = "S3CR3T"
		apiKey   = "sk-live-DEADBEEF"
		rawURL   = "https://svc:" + password + "@mcp.example.invalid/mcp?api_key=" + apiKey
	)
	display := integration.RedactURL(rawURL)
	if strings.Contains(display, password) || strings.Contains(display, apiKey) {
		t.Fatalf("integration.RedactURL left a secret in %q — the rest of this test "+
			"assumes it does not", display)
	}

	// net/http's own error shape: url.Error renders `Get "<url>": <cause>`, and its
	// stripPassword masks the PASSWORD only — the query string survives verbatim,
	// which is why the wrapper has to be stripped rather than merely tolerated.
	httpErr := fmt.Errorf("failed to connect to MCP server: Get %q: dial tcp: lookup mcp.example.invalid: no such host",
		"https://svc:***@mcp.example.invalid/mcp?api_key="+apiKey)

	server := integration.MCPServer{
		Name: "secret-remote", Source: "/fixture/.mcp.json", Scope: "global",
		Transport: integration.MCPTransportHTTP, URL: rawURL, URLDisplay: display,
	}

	tests := []struct {
		name   string
		client integration.MCPClient
	}{
		{
			name: "on a transport error",
			client: &fakeMCPClient{
				errs: map[string]error{rawURL: httpErr},
			},
		},
		{
			name: "on an answered probe",
			client: &fakeMCPClient{
				health: map[string]integration.MCPHealthCheck{
					rawURL: {ServerURL: rawURL, Healthy: false, StatusCode: 401,
						Error: errors.New("unhealthy status code: 401")},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			statuses := checkMCPServers([]integration.MCPServer{server}, &fakeResolver{}, tc.client)
			report := mcpCheckReport{Servers: statuses, Summary: summarize(statuses)}

			human := captureStdout(t, func() { printMCPCheck(report) })
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatalf("marshal report: %v", err)
			}

			for surface, text := range map[string]string{"human output": human, "--json": string(data)} {
				for _, secret := range []string{password, apiKey, rawURL} {
					if strings.Contains(text, secret) {
						t.Errorf("%s leaked %q:\n%s", surface, secret, text)
					}
				}
				if !strings.Contains(text, display) {
					t.Errorf("%s does not carry the redacted url %q at all — the row is "+
						"unusable without it:\n%s", surface, display, text)
				}
			}

			// The url must appear ONCE on the line, not twice: the caller prints it
			// and the wrapper repeated it. (The redacted form appears once in the
			// detail; the human line is that detail.)
			if n := strings.Count(statuses[0].Detail, display); n != 1 {
				t.Errorf("detail names the url %d times, want once: %q", n, statuses[0].Detail)
			}
		})
	}
}

// TestMCPCheck_RedirectDestinationIsRedacted is the redaction boundary for the
// SECOND url a redirect row can print: where it pointed.
//
// A redirect off an MCP url is usually the start or the callback of an OAuth
// exchange, and those urls carry credentials — an authorization `code` is a
// single-use bearer credential and `state` is the CSRF binding. This report is
// printed to a terminal and marshalled by `--json`, so neither may appear raw in
// either surface.
//
// The mechanism under test is where the redaction HAPPENS: the destination is
// redacted by the probe (integration.RedactLocation) and MCPHealthCheck carries no
// raw spelling of it at all, so this file has nothing to un-redact and no renderer
// has to remember to. The fixture therefore holds exactly what the real client would
// hand over — hence RedactLocation here rather than a hand-written string, the same
// way httpServer builds URLDisplay with RedactURL.
func TestMCPCheck_RedirectDestinationIsRedacted(t *testing.T) {
	const (
		code     = "SECRETAUTHCODE"
		state    = "XYZCSRFSTATE"
		password = "S3CR3T"
		rawDest  = "https://svc:" + password + "@idp.example.invalid/authorize?" +
			"code=" + code + "&state=" + state + "&client_id=adb"
		probedURL = "https://192.0.2.20:8443/mcp"
	)
	dest := integration.RedactLocation(rawDest)
	for _, secret := range []string{code, state, password} {
		if strings.Contains(dest, secret) {
			t.Fatalf("integration.RedactLocation left %q in %q — the rest of this test "+
				"assumes it does not", secret, dest)
		}
	}

	client := &fakeMCPClient{
		health: map[string]integration.MCPHealthCheck{
			probedURL: {ServerURL: probedURL, Healthy: false, StatusCode: 302, Location: dest},
		},
	}

	statuses := checkMCPServers(
		[]integration.MCPServer{httpServer("sso-redirect", probedURL)},
		&fakeResolver{}, client)
	report := mcpCheckReport{Servers: statuses, Summary: summarize(statuses)}

	human := captureStdout(t, func() { printMCPCheck(report) })
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	// Absence is asserted against the RAW marshalled bytes, so nothing can hide in a
	// field this test forgot to decode.
	for surface, text := range map[string]string{"human output": human, "--json": string(data)} {
		for _, secret := range []string{code, state, password, rawDest} {
			if strings.Contains(text, secret) {
				t.Errorf("%s leaked %q from the redirect destination:\n%s", surface, secret, text)
			}
		}
	}

	// Presence is asserted against the DECODED row rather than those same bytes,
	// because encoding/json HTML-escapes an ampersand into a unicode escape: a
	// destination with two query parameters is never byte-present in the marshalled
	// form, and a substring check there would fail for a reason that has nothing to
	// do with redaction.
	decodedDetail, _ := marshalFirstServerRow(t, report)["detail"].(string)
	for surface, text := range map[string]string{"human output": human, "--json detail": decodedDetail} {
		// The destination must actually be there: a row that redacts the answer into
		// nothing has closed the leak by deleting the diagnosis.
		if !strings.Contains(text, dest) {
			t.Errorf("%s does not name the redacted destination %q — the destination is the "+
				"whole point of the annotation:\n%s", surface, dest, text)
		}
	}
}

// TestMCPCheck_DisplayURLNeverFallsBackToRaw covers the defensive half of the
// redaction boundary. Discovery always fills URLDisplay, so the fallback should
// never fire — but if a future discovery path or a hand-built entry ever forgets
// it, the failure is a credential on screen and it is silent. So the fallback
// redacts rather than echoing MCPServer.URL.
func TestMCPCheck_DisplayURLNeverFallsBackToRaw(t *testing.T) {
	tests := []struct {
		name   string
		server integration.MCPServer
		want   string
	}{
		{
			name:   "URLDisplay wins when discovery set it",
			server: integration.MCPServer{URL: "https://svc:S3CR3T@host/mcp", URLDisplay: "https://svc:***@host/mcp"},
			want:   "https://svc:***@host/mcp",
		},
		{
			name:   "a missing URLDisplay is redacted here rather than echoed raw",
			server: integration.MCPServer{URL: "https://svc:S3CR3T@host/mcp"},
			want:   integration.RedactURL("https://svc:S3CR3T@host/mcp"),
		},
		{
			name:   "no url at all is the empty string, not a stray marker",
			server: integration.MCPServer{},
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpDisplayURL(tc.server)
			if got != tc.want {
				t.Errorf("mcpDisplayURL = %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "S3CR3T") {
				t.Errorf("mcpDisplayURL leaked the password: %q", got)
			}
		})
	}
}

// TestCheckMCPServers_ProbeConcurrencyIsCapped asserts the fan-out is BOUNDED.
// Unbounded goroutines would open one socket per configured server at once, which
// is a poor thing to do to a laptop (and to whatever is on the other end) for a
// diagnostic — the cap exists so "probe concurrently" does not become "probe all
// at once".
//
// The assertion is deterministic rather than timed: once maxMCPProbeConcurrency
// probes are parked, no worker is free, so a further probe CANNOT start until one
// is released. If the cap were missing, the extra probes would arrive immediately.
func TestCheckMCPServers_ProbeConcurrencyIsCapped(t *testing.T) {
	n := maxMCPProbeConcurrency + 4
	client := &barrierClient{arrived: make(chan string, n), release: make(chan struct{})}

	servers := make([]integration.MCPServer, 0, n)
	for i := 0; i < n; i++ {
		servers = append(servers, httpServer(fmt.Sprintf("srv-%02d", i),
			fmt.Sprintf("http://192.0.2.%d:8080/mcp", 100+i)))
	}

	done := make(chan []mcpServerStatus, 1)
	go func() { done <- checkMCPServers(servers, &fakeResolver{}, client) }()

	for i := 0; i < maxMCPProbeConcurrency; i++ {
		select {
		case <-client.arrived:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d probes started; want the cap's worth (%d) in flight",
				i, maxMCPProbeConcurrency)
		}
	}
	select {
	case url := <-client.arrived:
		t.Errorf("probe %d started while all %d workers were parked (%s) — the "+
			"concurrency cap is not applied", maxMCPProbeConcurrency+1, maxMCPProbeConcurrency, url)
	case <-time.After(100 * time.Millisecond):
	}
	close(client.release)

	var statuses []mcpServerStatus
	select {
	case statuses = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("checkMCPServers did not return after the probes were released")
	}
	if len(statuses) != n {
		t.Fatalf("got %d rows, want %d", len(statuses), n)
	}
	for _, st := range statuses {
		if st.State != mcpStateReachable {
			t.Errorf("%s: State = %q (%s), want every capped probe to still complete",
				st.Name, st.State, st.Detail)
		}
	}
}

// TestCheckMCPServers_ResolvesTheServersOwnCommand pins the specific bug the
// rewrite fixed: the old per-server check ignored the server name and command
// entirely and returned ONE global verdict repeated N times (it asked whether
// `npx` was on PATH, whatever the entry actually said). So a mixed set must produce
// per-entry verdicts, and the resolver must be asked about each entry's own
// command — with the base directory of the config file that declared it.
func TestCheckMCPServers_ResolvesTheServersOwnCommand(t *testing.T) {
	res := &fakeResolver{
		resolved: map[string]string{"good-tool": "/fake/bin/good-tool"},
		errs:     map[string]string{"bad-tool": "command not found: bad-tool"},
	}
	servers := []integration.MCPServer{
		stdioServer("alpha", "good-tool"),
		stdioServer("beta", "bad-tool"),
		brokenServer("gamma"),
	}

	statuses := checkMCPServers(servers, res, nil)

	want := []mcpCheckState{mcpStateLaunchable, mcpStateUnresolved, mcpStateUnknown}
	got := make([]mcpCheckState, 0, len(statuses))
	for _, st := range statuses {
		got = append(got, st.State)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("states = %v, want %v — one global verdict repeated N times is the "+
			"failure mode this replaced", got, want)
	}
	// gamma has no command, so it must never reach the resolver.
	if !reflect.DeepEqual(res.calls, []string{"good-tool", "bad-tool"}) {
		t.Errorf("resolver calls = %v, want [good-tool bad-tool]", res.calls)
	}
	// Each call carries the directory of the config file the entry came from.
	if !reflect.DeepEqual(res.baseDirs, []string{"/fixture", "/fixture"}) {
		t.Errorf("resolver base dirs = %v, want the config file's directory twice — a "+
			"relative command must not resolve against adb's cwd", res.baseDirs)
	}
	if s := summarize(statuses); s != (mcpCheckSummary{Total: 3, Launchable: 1, Unresolved: 1, Unknown: 1}) {
		t.Errorf("summary = %+v, want 1 launchable / 1 unresolved / 1 unknown of 3", s)
	}
}

// TestCheckMCPServers_BlankCommandClassifiesLikeNoCommand pins the consistency
// fix. `{"command": ""}` and `{"command": "   "}` are the same defect — an entry
// that names nothing to launch — and they used to classify DIFFERENTLY (`unknown`
// vs `unresolved`), which made one mistake look like two. Both are `unknown` now,
// neither reaches the resolver, and the detail still says which shape it was.
func TestCheckMCPServers_BlankCommandClassifiesLikeNoCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantDetail string
	}{
		{
			name:       "no command at all",
			command:    "",
			wantDetail: "entry declares neither a command nor a url",
		},
		{
			name:       "a whitespace-only command",
			command:    "   \t ",
			wantDetail: `entry declares a blank command ("   \t ")`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &fakeResolver{}
			statuses := checkMCPServers([]integration.MCPServer{
				stdioServer("blank", tc.command),
			}, res, nil)

			if statuses[0].State != mcpStateUnknown {
				t.Errorf("State = %q, want %q — a blank command is not a resolution "+
					"failure, there is nothing to resolve", statuses[0].State, mcpStateUnknown)
			}
			if statuses[0].Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", statuses[0].Detail, tc.wantDetail)
			}
			if len(res.calls) != 0 {
				t.Errorf("resolver was called with %v; a blank command must never reach "+
					"it (the `no command configured` branch it used to answer with is "+
					"gone)", res.calls)
			}
		})
	}
}

// TestCheckMCPServers_ProbesOnlyHTTPServers asserts the network boundary at the
// level where it is injectable: a stdio server is never probed, an http server is
// probed exactly once at its own RAW url (a redacted url does not dial), and
// checkMCPServers never clears the cache itself (that is uncachedMCPClient's job,
// installed by --no-cache one layer up in newMCPCheckCmd — see
// TestMCPCheck_UncachedClient).
//
// The old implementation issued a GET to localhost:3000/health for EVERY server,
// stdio included — so anything answering on that port made every server "healthy".
func TestCheckMCPServers_ProbesOnlyHTTPServers(t *testing.T) {
	const (
		rawURL     = "https://svc:S3CR3T@192.0.2.20:8080/mcp"
		displayURL = "https://svc:***@192.0.2.20:8080/mcp"
	)
	client := &fakeMCPClient{
		health: map[string]integration.MCPHealthCheck{
			rawURL: {ServerURL: rawURL, Healthy: true, StatusCode: 204},
		},
	}
	res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}

	servers := []integration.MCPServer{
		stdioServer("local", "good-tool"),
		{
			Name: "remote", Source: "/fixture/.mcp.json", Scope: "global",
			Transport: integration.MCPTransportHTTP, URL: rawURL, URLDisplay: displayURL,
		},
		brokenServer("neither"),
	}
	checkMCPServers(servers, res, client)

	probes, _, cleared, ttl := client.snapshot()
	if !reflect.DeepEqual(probes, []string{rawURL}) {
		t.Errorf("probes = %v, want exactly [%s] — only an http server with a url may be "+
			"probed, and it must be dialled with the RAW url", probes, rawURL)
	}
	if cleared != 0 {
		t.Errorf("ClearCache called %d times; checkMCPServers must not clear the cache — "+
			"--no-cache owns that decision", cleared)
	}
	if ttl != 0 {
		t.Errorf("SetTTL called with %v; the TTL is the client's own configuration", ttl)
	}
}

// TestCheckMCPServers_CoalescesEntriesSharingAURL covers the one form of reuse a
// single run can have, and why the decision moved out of the client's TTL cache.
//
// The cache used to provide it for free: the second entry naming a url found the
// first's result. Once probes ran concurrently that became a race the cache always
// lost — both entries dial before either has anything to cache — so the default's
// coalescing silently stopped happening and --no-cache silently stopped meaning
// anything. Both halves are asserted here: one probe by default (every row still
// getting its verdict), one probe PER ENTRY under the --no-cache marker.
func TestCheckMCPServers_CoalescesEntriesSharingAURL(t *testing.T) {
	const (
		sharedURL = "http://192.0.2.90:8080/mcp"
		otherURL  = "http://192.0.2.91:8080/mcp"
	)
	fixture := map[string]integration.MCPHealthCheck{
		sharedURL: {ServerURL: sharedURL, Healthy: false, StatusCode: 405,
			Error: errors.New("unhealthy status code: 405")},
		otherURL: {ServerURL: otherURL, Healthy: true, StatusCode: 200},
	}
	servers := []integration.MCPServer{
		httpServer("dup-a", sharedURL),
		httpServer("other", otherURL),
		httpServer("dup-b", sharedURL),
	}

	tests := []struct {
		name string
		// wrap decides whether the client carries the --no-cache marker.
		wrap       func(integration.MCPClient) integration.MCPClient
		wantProbes []string
		wantClears int
	}{
		{
			name:       "by default the shared url is probed once",
			wrap:       func(c integration.MCPClient) integration.MCPClient { return c },
			wantProbes: []string{sharedURL, otherURL},
			wantClears: 0,
		},
		{
			name: "--no-cache probes each entry",
			wrap: func(c integration.MCPClient) integration.MCPClient { return uncachedMCPClient{c} },
			// Planned in row order, and with three probes and eight workers all
			// three are in flight, so the completion order is not asserted — only
			// the multiset. The shared url appearing TWICE is the point.
			wantProbes: []string{sharedURL, otherURL, sharedURL},
			wantClears: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := &fakeMCPClient{health: fixture}
			statuses := checkMCPServers(servers, &fakeResolver{}, tc.wrap(inner))

			// Every row gets its own verdict either way — coalescing is an
			// optimisation, never a missing row.
			wantDetails := []string{
				sharedURL + ": status 405",
				otherURL + ": status 200",
				sharedURL + ": status 405",
			}
			for i, st := range statuses {
				if st.State != mcpStateReachable || st.Detail != wantDetails[i] {
					t.Errorf("%s: State/Detail = %q/%q, want %q/%q",
						st.Name, st.State, st.Detail, mcpStateReachable, wantDetails[i])
				}
			}

			probes, _, cleared, _ := inner.snapshot()
			sortedGot := append([]string(nil), probes...)
			sortedWant := append([]string(nil), tc.wantProbes...)
			sort.Strings(sortedGot)
			sort.Strings(sortedWant)
			if !reflect.DeepEqual(sortedGot, sortedWant) {
				t.Errorf("probes = %v, want %v", probes, tc.wantProbes)
			}
			if cleared != tc.wantClears {
				t.Errorf("ClearCache called %d times, want %d", cleared, tc.wantClears)
			}
		})
	}
}

// barrierClient is a fake prober that parks every probe until it is released, so a
// test can prove probes overlap in time instead of merely counting them.
type barrierClient struct {
	arrived chan string
	release chan struct{}
}

func (b *barrierClient) CheckHealth(serverURL string) (integration.MCPHealthCheck, error) {
	b.arrived <- serverURL
	<-b.release
	return integration.MCPHealthCheck{ServerURL: serverURL, Healthy: true, StatusCode: 200}, nil
}
func (b *barrierClient) ClearCache()            {}
func (b *barrierClient) SetTTL(_ time.Duration) {}
func (b *barrierClient) probed() int            { return len(b.arrived) }

var _ integration.MCPClient = (*barrierClient)(nil)

// TestCheckMCPServers_ProbesConcurrentlyInStableOrder covers the performance defect
// and the property that makes fixing it safe.
//
// Measured before: three unroutable remote servers took 30.04s — the SUM of their
// timeouts — with no output until the very end, because probing was a plain loop.
// Probing is pure IO wait, so it fans out.
//
// The concurrency assertion is a barrier, not a stopwatch: every probe must arrive
// before ANY is allowed to finish, which a serial loop can never satisfy (it would
// deadlock on the first release) and which no amount of machine-load noise can make
// flaky. The second assertion is the safety half: results are written back by row
// index, so the report's order is the caller's order however the probes finished —
// a report whose rows shuffled run-to-run would be undiffable.
func TestCheckMCPServers_ProbesConcurrentlyInStableOrder(t *testing.T) {
	const n = 4 // below maxMCPProbeConcurrency, so all four may be in flight
	client := &barrierClient{arrived: make(chan string, n), release: make(chan struct{})}

	servers := make([]integration.MCPServer, 0, n)
	wantNames := make([]string, 0, n)
	for i := 0; i < n; i++ {
		// Names deliberately DESCEND while indices ascend, so an ordering that came
		// from sorting rather than from the caller's slice would show up.
		name := fmt.Sprintf("srv-%d", n-i)
		servers = append(servers, httpServer(name, fmt.Sprintf("http://192.0.2.%d:8080/mcp", 40+i)))
		wantNames = append(wantNames, name)
	}

	done := make(chan []mcpServerStatus, 1)
	go func() { done <- checkMCPServers(servers, &fakeResolver{}, client) }()

	for i := 0; i < n; i++ {
		select {
		case <-client.arrived:
		case <-time.After(10 * time.Second):
			t.Fatalf("only %d of %d probes had started before any was allowed to finish; "+
				"probing is serial", i, n)
		}
	}
	close(client.release)

	var statuses []mcpServerStatus
	select {
	case statuses = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("checkMCPServers did not return after the probes were released")
	}

	gotNames := make([]string, 0, len(statuses))
	for _, st := range statuses {
		gotNames = append(gotNames, st.Name)
		if st.State != mcpStateReachable {
			t.Errorf("%s: State = %q, want %q", st.Name, st.State, mcpStateReachable)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("row order = %v, want the input order %v — completion order must not "+
			"reorder the report", gotNames, wantNames)
	}
}

// TestCheckMCPServers_ProbeBudgetBoundsTheRun covers the backstop: however many
// remote servers a config lists, the probe phase cannot run forever. The budget is
// a package var precisely so this can be asserted in milliseconds rather than by
// sleeping, and the row that did not come back is reported as unreachable naming
// the budget it exceeded — not silently dropped, and not left as a zero verdict.
func TestCheckMCPServers_ProbeBudgetBoundsTheRun(t *testing.T) {
	origBudget := mcpProbeBudget
	mcpProbeBudget = 20 * time.Millisecond
	t.Cleanup(func() { mcpProbeBudget = origBudget })

	// A client that never answers. Released on cleanup so the worker goroutine
	// finishes rather than outliving the test.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	client := &barrierClient{arrived: make(chan string, 1), release: release}

	statuses := checkMCPServers([]integration.MCPServer{
		httpServer("hangs", "http://192.0.2.50:8080/mcp"),
	}, &fakeResolver{}, client)

	if statuses[0].State != mcpStateUnreachable {
		t.Errorf("State = %q, want %q for a probe that never answered",
			statuses[0].State, mcpStateUnreachable)
	}
	if !strings.Contains(statuses[0].Detail, "no response within 20ms") {
		t.Errorf("Detail = %q, want it to name the budget it exceeded", statuses[0].Detail)
	}
	if client.probed() == 0 {
		t.Error("the probe never started, so the budget was not what ended the wait")
	}
}

// TestMCPCheck_UncachedClient is what --no-cache actually installs. The interesting
// property is the ORDER: clearing has to happen before each probe, not once at
// startup, because MCPClient's cache is in-memory and the client is built per
// invocation — a single ClearCache() at startup clears an empty cache and changes
// nothing, which is indistinguishable from the old no-op flag.
//
// So the assertions are: the cache is cleared before EVERY probe, two entries
// sharing a url become two independent probes, and both the result and the error
// pass through untouched (the decorator must not change any verdict).
func TestMCPCheck_UncachedClient(t *testing.T) {
	const (
		healthyURL = "http://192.0.2.30:8080/mcp" // TEST-NET-1; the inner client is a fake
		brokenURL  = "http://192.0.2.31:8080/mcp"
	)
	probeErr := errors.New("failed to connect to MCP server: boom")

	newInner := func() *fakeMCPClient {
		return &fakeMCPClient{
			health: map[string]integration.MCPHealthCheck{
				healthyURL: {ServerURL: healthyURL, Healthy: true, StatusCode: 200},
			},
			errs: map[string]error{brokenURL: probeErr},
		}
	}

	tests := []struct {
		name string
		urls []string
		// wantEvents is the exact ordered call log the inner client must see.
		wantEvents []string
	}{
		{
			name:       "one probe clears first",
			urls:       []string{healthyURL},
			wantEvents: []string{"clear", "probe:" + healthyURL},
		},
		{
			// The one case the cache could ever affect within a single run.
			name: "the same url twice is two independent probes",
			urls: []string{healthyURL, healthyURL},
			wantEvents: []string{
				"clear", "probe:" + healthyURL,
				"clear", "probe:" + healthyURL,
			},
		},
		{
			name: "a failing probe is cleared and reported like any other",
			urls: []string{brokenURL, healthyURL},
			wantEvents: []string{
				"clear", "probe:" + brokenURL,
				"clear", "probe:" + healthyURL,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := newInner()
			client := uncachedMCPClient{inner}

			for _, url := range tc.urls {
				health, err := client.CheckHealth(url)
				// Pass-through, both ways: the decorator adds cache-clearing and
				// nothing else.
				if url == brokenURL {
					if !errors.Is(err, probeErr) {
						t.Errorf("CheckHealth(%s) error = %v, want the inner error %v", url, err, probeErr)
					}
					continue
				}
				if err != nil {
					t.Errorf("CheckHealth(%s): %v", url, err)
				}
				if !health.Healthy || health.StatusCode != 200 || health.ServerURL != url {
					t.Errorf("CheckHealth(%s) = %+v, want the inner result unchanged", url, health)
				}
			}

			_, events, cleared, _ := inner.snapshot()
			if !reflect.DeepEqual(events, tc.wantEvents) {
				t.Errorf("inner call log = %v, want %v — the cache must be cleared BEFORE "+
					"each probe", events, tc.wantEvents)
			}
			if cleared != len(tc.urls) {
				t.Errorf("ClearCache called %d times for %d probes; want one clear per probe",
					cleared, len(tc.urls))
			}
		})
	}
}

// TestMCPCheckCmd_HTTPPathIsWiredAndNoCacheIsObservable closes the gap the old
// suite documented but could not fix: newMCPCheckCmd built its prober with a direct
// integration.NewMCPClient call, so nothing could assert that --no-cache installs
// the decorator, and no command-level fixture could carry an http server without a
// real network call. The mcpCheckNewClient seam makes both observable.
func TestMCPCheckCmd_HTTPPathIsWiredAndNoCacheIsObservable(t *testing.T) {
	const (
		sharedURL = "http://192.0.2.60:8080/mcp"
		otherURL  = "http://192.0.2.61:8080/mcp"
	)
	newFake := func() *fakeMCPClient {
		return &fakeMCPClient{
			health: map[string]integration.MCPHealthCheck{
				sharedURL: {ServerURL: sharedURL, Healthy: false, StatusCode: 405,
					Error: errors.New("unhealthy status code: 405")},
				otherURL: {ServerURL: otherURL, Healthy: true, StatusCode: 200},
			},
		}
	}
	// Two entries share a url — the only reuse a single run can have — plus one
	// distinct url, so the coalescing --no-cache suppresses is present to suppress.
	httpServers := []integration.MCPServer{
		httpServer("dup-a", sharedURL),
		httpServer("dup-b", sharedURL),
		httpServer("other", otherURL),
	}

	tests := []struct {
		name    string
		servers []integration.MCPServer
		args    []string
		// wantBuilt is whether a prober was constructed at all.
		wantBuilt bool
		// wantClears is how many ClearCache calls the run must make.
		wantClears int
		// wantProbes is how many HTTP probes the run must issue for three entries
		// over two distinct urls — the whole observable difference --no-cache makes.
		wantProbes int
		// wantOut are substrings the human report must carry.
		wantOut []string
	}{
		{
			name:       "http servers are probed and annotated with their status",
			servers:    httpServers,
			args:       []string{"mcp", "check"},
			wantBuilt:  true,
			wantClears: 0,
			// The two entries sharing a url coalesce into ONE probe, and every row
			// still gets its verdict.
			wantProbes: 2,
			wantOut: []string{
				"status 405", "status 200", "3 reachable",
				"note: verifies each url answered",
			},
		},
		{
			name:      "--no-cache installs the decorator: one probe and one clear per entry",
			servers:   httpServers,
			args:      []string{"mcp", "check", "--no-cache"},
			wantBuilt: true,
			// Three entries ⇒ three probes ⇒ three clears. Without the decorator
			// this is 2 probes and 0 clears.
			wantClears: 3,
			wantProbes: 3,
			wantOut:    []string{"3 reachable"},
		},
		{
			name:      "a stdio-only config builds no prober at all",
			servers:   []integration.MCPServer{stdioServer("local", "good-tool")},
			args:      []string{"mcp", "check"},
			wantBuilt: false,
			wantOut:   []string{"1 launchable", "note: verifies each command resolves"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := withAppAt(t, t.TempDir())
			res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}
			withMCPCheckSeams(t, tc.servers, fixtureSources(app.BasePath), res)
			fake := newFake()
			built := withMCPCheckClient(t, fake)

			out := captureStdout(t, func() {
				if err := runADB(t, tc.args...); err != nil {
					t.Fatalf("%v: %v", tc.args, err)
				}
			})

			if got := len(*built) > 0; got != tc.wantBuilt {
				t.Errorf("prober constructed = %v, want %v (ttls: %v)", got, tc.wantBuilt, *built)
			}
			probes, _, cleared, _ := fake.snapshot()
			if cleared != tc.wantClears {
				t.Errorf("ClearCache called %d times, want %d", cleared, tc.wantClears)
			}
			if tc.wantBuilt && len(probes) != tc.wantProbes {
				t.Errorf("issued %d probe(s) (%v), want %d — this is the whole "+
					"observable difference --no-cache makes", len(probes), probes, tc.wantProbes)
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// TestMCPCheckCmd_LimitsNoteIsTransportAware covers the footer defect end to end.
// The note exists to stop a ✓ over-claiming, so the note itself has to be true: an
// http-only report used to end "verifies each command resolves to an executable,
// not that the server responds", where nothing had resolved a command and the
// server was the one thing that HAD responded.
func TestMCPCheckCmd_LimitsNoteIsTransportAware(t *testing.T) {
	const url = "http://192.0.2.70:8080/mcp"
	const (
		stdioHalf = "each command resolves to an executable, not that the server responds"
		httpHalf  = "each url answered, not that an MCP server is behind it"
	)

	tests := []struct {
		name       string
		servers    []integration.MCPServer
		wantNote   string
		wantAbsent []string
	}{
		{
			name:       "stdio only",
			servers:    []integration.MCPServer{stdioServer("local", "good-tool")},
			wantNote:   "note: verifies " + stdioHalf,
			wantAbsent: []string{httpHalf},
		},
		{
			name:       "http only",
			servers:    []integration.MCPServer{httpServer("remote", url)},
			wantNote:   "note: verifies " + httpHalf,
			wantAbsent: []string{stdioHalf},
		},
		{
			name: "both transports get both halves",
			servers: []integration.MCPServer{
				stdioServer("local", "good-tool"),
				httpServer("remote", url),
			},
			wantNote: "note: verifies " + stdioHalf + "; " + httpHalf,
		},
		{
			name:       "nothing verifiable gets no note",
			servers:    []integration.MCPServer{brokenServer("malformed")},
			wantAbsent: []string{"note:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := withAppAt(t, t.TempDir())
			res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}
			withMCPCheckSeams(t, tc.servers, fixtureSources(app.BasePath), res)
			withMCPCheckClient(t, &fakeMCPClient{
				health: map[string]integration.MCPHealthCheck{
					url: {ServerURL: url, Healthy: true, StatusCode: 200},
				},
			})

			out := captureStdout(t, func() {
				if err := runADB(t, "mcp", "check"); err != nil {
					t.Fatalf("mcp check: %v", err)
				}
			})

			if tc.wantNote != "" && !strings.Contains(out, tc.wantNote) {
				t.Errorf("output missing the note %q:\n%s", tc.wantNote, out)
			}
			for _, unwanted := range tc.wantAbsent {
				if strings.Contains(out, unwanted) {
					t.Errorf("output claims %q, which nothing in this report verified:\n%s",
						unwanted, out)
				}
			}
		})
	}
}

// TestMCPCheckState_OkAgreesWithExitCodeAccounting keeps the two definitions of
// "usable" in step. --exit-code now counts !state.ok() directly rather than
// re-adding the summary's unresolved/unreachable/unknown fields, so this asserts
// the remaining pair — ok() and summarize() — cannot drift: a state neither
// recognises still lands in `unknown` AND fails ok(), so a future state added to
// one and not the other cannot become a silent pass.
func TestMCPCheckState_OkAgreesWithExitCodeAccounting(t *testing.T) {
	tests := []struct {
		state  mcpCheckState
		wantOk bool
	}{
		{mcpStateLaunchable, true},
		{mcpStateReachable, true},
		{mcpStateUnresolved, false},
		{mcpStateUnreachable, false},
		{mcpStateUnknown, false},
		// A state neither function knows about: it must fail closed on both sides.
		{mcpCheckState("some-future-state"), false},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := tc.state.ok(); got != tc.wantOk {
				t.Errorf("%q.ok() = %v, want %v", tc.state, got, tc.wantOk)
			}
			// The same state, run through summarize + the --exit-code arithmetic.
			s := summarize([]mcpServerStatus{{State: tc.state}})
			bad := s.Unresolved + s.Unreachable + s.Unknown
			if wantBad := 0; tc.wantOk && bad != wantBad {
				t.Errorf("%q counts as bad in the --exit-code arithmetic but ok() says it "+
					"passes", tc.state)
			}
			if !tc.wantOk && bad != 1 {
				t.Errorf("%q counts as ok in the --exit-code arithmetic but ok() says it "+
					"fails", tc.state)
			}
		})
	}
}

// TestMCPCheckCmd_JSONContract pins the --json shape a scripted consumer reads: ONE
// top-level object with sources / servers / summary, snake_case keys at every
// depth, and a summary whose counts equal the rows it summarises.
func TestMCPCheckCmd_JSONContract(t *testing.T) {
	app := withAppAt(t, t.TempDir())
	res := &fakeResolver{
		resolved: map[string]string{"good-tool": "/fake/bin/good-tool"},
		errs:     map[string]string{"bad-tool": "command not found: bad-tool"},
	}
	servers := []integration.MCPServer{
		{
			Name: "alpha", Source: filepath.Join(app.BasePath, ".mcp.json"), Scope: "global",
			Transport: integration.MCPTransportStdio, Command: "good-tool",
			Args: []string{"-y", "pkg"}, EnvKeys: []string{"ADB_HOME", "TOKEN"},
		},
		stdioServer("beta", "bad-tool"),
		brokenServer("gamma"),
	}
	sources := fixtureSources(app.BasePath)
	gotWorkspace := withMCPCheckSeams(t, servers, sources, res)

	out := captureStdout(t, func() {
		if err := runADB(t, "mcp", "check", "--json"); err != nil {
			t.Fatalf("mcp check --json: %v", err)
		}
	})

	if *gotWorkspace != app.BasePath {
		t.Errorf("discovery was handed workspace %q, want App.BasePath %q — the project "+
			".mcp.json and the projects[<workspace>] block resolve off it",
			*gotWorkspace, app.BasePath)
	}

	// ONE top-level object, not an array and not JSONL.
	var top map[string]any
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("--json is not one decodable JSON object: %v\n%s", err, out)
	}
	wantKeys := []string{"servers", "sources", "summary"}
	gotKeys := sortedKeys(top)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("top-level keys = %v, want %v", gotKeys, wantKeys)
	}

	// snake_case RECURSIVELY — the servers rows embed integration.MCPServer, so a
	// missing json tag there would surface PascalCase inside an otherwise
	// snake_case document.
	keys := jsonKeys(top)
	if len(keys) == 0 {
		t.Fatal("no object keys at all, so the casing rule was not exercised")
	}
	for _, k := range keys {
		if !snakeCaseKey.MatchString(k) {
			t.Errorf("key %q is not snake_case; one command must not need two naming "+
				"conventions", k)
		}
	}

	// The summary must count the rows it ships with, not something else.
	rows, ok := top["servers"].([]any)
	if !ok {
		t.Fatalf("servers is not an array: %T", top["servers"])
	}
	counted := map[string]int{}
	for _, r := range rows {
		row, isObject := r.(map[string]any)
		if !isObject {
			t.Fatalf("server row is not an object: %T", r)
		}
		state, _ := row["state"].(string)
		counted[state]++
	}
	summary, ok := top["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary is not an object: %T", top["summary"])
	}
	if got := summary["total"]; got != float64(len(rows)) {
		t.Errorf("summary.total = %v, want %d", got, len(rows))
	}
	for _, state := range []string{"launchable", "unresolved", "reachable", "unreachable", "unknown"} {
		if got, want := summary[state], float64(counted[state]); got != want {
			t.Errorf("summary.%s = %v, want %v (the rows say otherwise)", state, got, want)
		}
	}

	// sources round-trips whole, including the absent ones a consumer needs.
	srcRows, ok := top["sources"].([]any)
	if !ok || len(srcRows) != len(sources) {
		t.Fatalf("sources = %v, want %d rows", top["sources"], len(sources))
	}
}

// fixtureSources is the four-source report discovery would return: two present
// (one contributing, one empty), one absent, one broken.
func fixtureSources(base string) []integration.MCPConfigSource {
	return []integration.MCPConfigSource{
		{Label: "project .mcp.json", Path: filepath.Join(base, ".mcp.json"), Present: true, Servers: 3},
		{Label: "Claude Code", Path: filepath.Join(base, "home", ".claude.json"), Present: true},
		{Label: "Claude Desktop", Path: filepath.Join(base, "home", ".config", "Claude", "claude_desktop_config.json")},
		{Label: "Claude Desktop", Path: filepath.Join(base, "home", "Library", "Application Support", "Claude", "claude_desktop_config.json"), Present: true, Err: "parse: unexpected end of JSON input"},
	}
}

// TestMCPCheckCmd_ExitCode covers the CI gate. Without the flag the command always
// exits zero (it is a report); with it, an unusable server is a non-zero exit whose
// message names the counts — matching `adb audit security --exit-code`.
func TestMCPCheckCmd_ExitCode(t *testing.T) {
	res := &fakeResolver{
		resolved: map[string]string{"good-tool": "/fake/bin/good-tool"},
		errs:     map[string]string{"bad-tool": "command not found: bad-tool"},
	}

	allGood := []integration.MCPServer{stdioServer("a", "good-tool"), stdioServer("b", "good-tool")}
	someBad := []integration.MCPServer{
		stdioServer("a", "good-tool"),
		stdioServer("b", "bad-tool"),
		brokenServer("c"),
	}

	tests := []struct {
		name    string
		servers []integration.MCPServer
		args    []string
		wantErr bool
		// wantErrSubstrs must all appear in the error message.
		wantErrSubstrs []string
	}{
		{
			name:    "no flag, all ok",
			servers: allGood,
			args:    []string{"mcp", "check"},
		},
		{
			name:    "no flag, some bad — still zero",
			servers: someBad,
			args:    []string{"mcp", "check"},
		},
		{
			name:    "flag, all ok",
			servers: allGood,
			args:    []string{"mcp", "check", "--exit-code"},
		},
		{
			name:           "flag, some bad — non-zero naming the counts",
			servers:        someBad,
			args:           []string{"mcp", "check", "--exit-code"},
			wantErr:        true,
			wantErrSubstrs: []string{"2 of 3", "mcp server(s) unusable"},
		},
		{
			name:    "flag, nothing configured — nothing to fail on",
			servers: nil,
			args:    []string{"mcp", "check", "--exit-code"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := withAppAt(t, t.TempDir())
			withMCPCheckSeams(t, tc.servers, fixtureSources(app.BasePath), res)

			var execErr error
			_ = captureStdout(t, func() { execErr = runADB(t, tc.args...) })

			if tc.wantErr && execErr == nil {
				t.Fatalf("%v: want a non-zero exit, got nil", tc.args)
			}
			if !tc.wantErr && execErr != nil {
				t.Fatalf("%v: want a clean exit, got %v", tc.args, execErr)
			}
			for _, want := range tc.wantErrSubstrs {
				if !strings.Contains(execErr.Error(), want) {
					t.Errorf("error %q missing %q", execErr, want)
				}
			}
		})
	}
}

// TestMCPCheckCmd_ExitCodeCountsAnAnsweredHTTPServerAsUsable is the http half of
// the gate, and it is a behaviour CHANGE worth pinning: a 405 from a real
// Streamable-HTTP endpoint used to be counted `unreachable` and fail the gate. A CI
// job would have gone red for a server that was working.
func TestMCPCheckCmd_ExitCodeCountsAnAnsweredHTTPServerAsUsable(t *testing.T) {
	const (
		answeredURL = "http://192.0.2.80:8080/mcp"
		deadURL     = "http://192.0.2.81:8080/mcp"
	)
	app := withAppAt(t, t.TempDir())
	withMCPCheckSeams(t, []integration.MCPServer{
		httpServer("answers-405", answeredURL),
	}, fixtureSources(app.BasePath), &fakeResolver{})
	withMCPCheckClient(t, &fakeMCPClient{
		health: map[string]integration.MCPHealthCheck{
			answeredURL: {ServerURL: answeredURL, Healthy: false, StatusCode: 405,
				Error: errors.New("unhealthy status code: 405")},
		},
		errs: map[string]error{
			deadURL: errors.New("failed to connect to MCP server: boom"),
		},
	})

	var execErr error
	_ = captureStdout(t, func() { execErr = runADB(t, "mcp", "check", "--exit-code") })
	if execErr != nil {
		t.Errorf("--exit-code failed on a server that answered 405: %v", execErr)
	}
}

// TestMCPCheckCmd_NoServersListsWhereItLooked asserts the diagnostic that made the
// old command useless when it got it wrong: with nothing found, the report must say
// so AND list every file consulted, including the absent ones. A user whose five
// servers do not show up needs to see which files were read before they can tell
// whether the answer is wrong.
func TestMCPCheckCmd_NoServersListsWhereItLooked(t *testing.T) {
	app := withAppAt(t, t.TempDir())
	sources := fixtureSources(app.BasePath)
	withMCPCheckSeams(t, nil, sources, &fakeResolver{})

	out := captureStdout(t, func() {
		if err := runADB(t, "mcp", "check"); err != nil {
			t.Fatalf("mcp check: %v", err)
		}
	})

	if !strings.Contains(out, "No MCP servers configured.") {
		t.Errorf("expected the no-servers notice:\n%s", out)
	}
	if !strings.Contains(out, "Looked in:") {
		t.Errorf("expected the Looked in: list:\n%s", out)
	}
	for _, src := range sources {
		if !strings.Contains(out, filepath.Base(filepath.Dir(src.Path))) &&
			!strings.Contains(out, filepath.Base(src.Path)) {
			t.Errorf("Looked in: omits %s (%s):\n%s", src.Path, src.Label, out)
		}
	}
	// The three source conditions must each be distinguishable.
	for _, want := range []string{
		"not found",                           // the absent one
		"no servers",                          // present but empty
		"parse: unexpected end of JSON input", // broken, error surfaced
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Looked in: does not distinguish the %q case:\n%s", want, out)
		}
	}
}

// TestMCPCheckCmd_NoCacheDoesNotChangeStdioVerdict asserts --no-cache parses and is
// inert for a stdio-only workspace: the cache it clears is the http probe cache, so
// a set of stdio servers must produce byte-identical output either way.
//
// What --no-cache DOES is covered from two directions:
// TestMCPCheck_UncachedClient exercises the decorator itself, and
// TestMCPCheckCmd_HTTPPathIsWiredAndNoCacheIsObservable asserts newMCPCheckCmd
// installs it (via the mcpCheckNewClient seam, which is why an http fixture at the
// command level no longer implies a real network call).
func TestMCPCheckCmd_NoCacheDoesNotChangeStdioVerdict(t *testing.T) {
	res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}
	servers := []integration.MCPServer{stdioServer("a", "good-tool"), stdioServer("b", "good-tool")}

	// ONE workspace for both runs: the rendered report quotes config paths, so two
	// temp workspaces would differ for a reason that has nothing to do with the flag.
	app := withAppAt(t, t.TempDir())
	withMCPCheckSeams(t, servers, fixtureSources(app.BasePath), res)

	run := func(t *testing.T, args ...string) string {
		t.Helper()
		var out string
		var execErr error
		out = captureStdout(t, func() { execErr = runADB(t, args...) })
		if execErr != nil {
			t.Fatalf("%v: %v", args, execErr)
		}
		return out
	}

	plain := run(t, "mcp", "check")
	noCache := run(t, "mcp", "check", "--no-cache")

	if plain != noCache {
		t.Errorf("--no-cache changed a stdio-only verdict; it must only affect http probes\n"+
			"without:\n%s\nwith:\n%s", plain, noCache)
	}
	if !strings.Contains(plain, "2 launchable") {
		t.Errorf("expected both stdio servers launchable:\n%s", plain)
	}
}

// TestMCPCheckCmd_DiscoveryErrorIsWrapped asserts a discovery failure surfaces as a
// wrapped error rather than an empty "no MCP servers configured" report — the
// latter is exactly the lie the old implementation told.
func TestMCPCheckCmd_DiscoveryErrorIsWrapped(t *testing.T) {
	withAppAt(t, t.TempDir())

	orig := mcpCheckDiscover
	wantErr := errors.New("resolve home directory: no home")
	mcpCheckDiscover = func(string) ([]integration.MCPServer, []integration.MCPConfigSource, error) {
		return nil, nil, wantErr
	}
	t.Cleanup(func() { mcpCheckDiscover = orig })

	var execErr error
	out := captureStdout(t, func() { execErr = runADB(t, "mcp", "check") })
	if execErr == nil {
		t.Fatalf("want an error, got nil (output: %s)", out)
	}
	if !errors.Is(execErr, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", execErr, wantErr)
	}
	if !strings.Contains(execErr.Error(), "failed to discover mcp servers") {
		t.Errorf("error = %q, want the discovery context", execErr)
	}
	if strings.Contains(out, "No MCP servers configured") {
		t.Errorf("a discovery failure must not be rendered as an empty report:\n%s", out)
	}
}

// TestMCPCheckCmd_GroupsRowsByConfigFile asserts rows are grouped under the config
// that owns them. "Which of my configs is this stale entry in?" is most of the
// value of the command, and the old one could not answer it at all.
func TestMCPCheckCmd_GroupsRowsByConfigFile(t *testing.T) {
	app := withAppAt(t, t.TempDir())
	projectCfg := filepath.Join(app.BasePath, ".mcp.json")
	codeCfg := filepath.Join(app.BasePath, "home", ".claude.json")

	res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}
	servers := []integration.MCPServer{
		{Name: "from-code", Source: codeCfg, Transport: integration.MCPTransportStdio, Command: "good-tool"},
		{Name: "from-project", Source: projectCfg, Transport: integration.MCPTransportStdio, Command: "good-tool"},
	}
	withMCPCheckSeams(t, servers, fixtureSources(app.BasePath), res)

	out := captureStdout(t, func() {
		if err := runADB(t, "mcp", "check"); err != nil {
			t.Fatalf("mcp check: %v", err)
		}
	})

	// One header per owning config, each carrying its row count.
	for _, want := range []string{".mcp.json (1)", ".claude.json (1)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing the group header %q:\n%s", want, out)
		}
	}
	// Each server must appear AFTER its own config's header.
	for _, tc := range []struct{ header, server string }{
		{".mcp.json (1)", "from-project"},
		{".claude.json (1)", "from-code"},
	} {
		hi := strings.Index(out, tc.header)
		si := strings.Index(out, tc.server)
		if hi < 0 || si < 0 || si < hi {
			t.Errorf("%q is not listed under %q:\n%s", tc.server, tc.header, out)
		}
	}
}

// TestMCPCheckCmd_BrokenConfigStaysVisibleWhenServersWereFound pins the asymmetry
// between the two source renderings, because getting it wrong is silent by
// construction:
//
//   - nothing found  → the FULL "Looked in:" list ("where did you look?" is the
//     whole question)
//   - servers found  → only the sources that FAILED to parse, as a warning
//
// Without the second, the one run where a config broke looks like a completely
// normal run and the servers it should have contributed are just missing.
func TestMCPCheckCmd_BrokenConfigStaysVisibleWhenServersWereFound(t *testing.T) {
	res := &fakeResolver{resolved: map[string]string{"good-tool": "/fake/bin/good-tool"}}
	servers := []integration.MCPServer{stdioServer("a", "good-tool")}

	tests := []struct {
		name string
		// sources is the report discovery returns.
		sources func(base string) []integration.MCPConfigSource
		// wantSubstrs / wantAbsent are asserted against the human output.
		wantSubstrs []string
		wantAbsent  []string
	}{
		{
			name:    "a broken config is warned about",
			sources: fixtureSources,
			wantSubstrs: []string{
				"1 config file(s) could not be fully read",
				// "may be missing", not "are missing": a failure confined to one
				// projects[<key>] block still lets the file's root servers through.
				"servers in them may be missing above",
				"parse: unexpected end of JSON input",
			},
			// The full list stays reserved for the nothing-found case.
			wantAbsent: []string{"Looked in:", "not found"},
		},
		{
			name: "no broken config, no warning at all",
			sources: func(base string) []integration.MCPConfigSource {
				return []integration.MCPConfigSource{
					{Label: "project .mcp.json", Path: filepath.Join(base, ".mcp.json"), Present: true, Servers: 1},
					{Label: "Claude Code", Path: filepath.Join(base, "home", ".claude.json")},
				}
			},
			wantSubstrs: []string{"1 launchable"},
			wantAbsent:  []string{"could not be fully read", "Looked in:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := withAppAt(t, t.TempDir())
			withMCPCheckSeams(t, servers, tc.sources(app.BasePath), res)

			out := captureStdout(t, func() {
				if err := runADB(t, "mcp", "check"); err != nil {
					t.Fatalf("mcp check: %v", err)
				}
			})
			for _, want := range tc.wantSubstrs {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			for _, unwanted := range tc.wantAbsent {
				if strings.Contains(out, unwanted) {
					t.Errorf("output should not contain %q:\n%s", unwanted, out)
				}
			}
		})
	}
}

// TestPrintMCPCheck_NoServersAndNoSources covers the degenerate report: no servers
// AND no sources at all (which is what discovery returns when neither a home nor a
// workspace directory resolved). The notice still prints; the empty "Looked in:"
// header does not, because a header over an empty list says nothing.
func TestPrintMCPCheck_NoServersAndNoSources(t *testing.T) {
	out := captureStdout(t, func() { printMCPCheck(mcpCheckReport{}) })
	if !strings.Contains(out, "No MCP servers configured.") {
		t.Errorf("expected the no-servers notice:\n%s", out)
	}
	if strings.Contains(out, "Looked in:") {
		t.Errorf("an empty source list must not get a header:\n%s", out)
	}
}

// TestMCPCheck_DisplayPath asserts the output does not splash an absolute home directory
// across every line, and — more importantly — that it never mangles a path OUTSIDE
// the home directory into a misleading ~/ form. The printed path is what a user
// opens to fix a broken entry, so it has to stay a real path.
//
// $HOME is redirected, so this test cannot be t.Parallel (it already could not — see
// the file header).
func TestMCPCheck_DisplayPath(t *testing.T) {
	tests := []struct {
		name string
		// home is what $HOME is set to; "" unsets it, "root" pins it to the
		// filesystem root.
		home string
		path func(home string) string
		want func(home string) string
		unix bool // POSIX-only ($HOME semantics)
	}{
		{
			name: "a path under home is shortened",
			home: "set",
			path: func(home string) string { return filepath.Join(home, ".claude.json") },
			want: func(home string) string { return filepath.Join("~", ".claude.json") },
		},
		{
			name: "a nested path under home keeps its tail",
			home: "set",
			path: func(home string) string {
				return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
			},
			want: func(home string) string {
				return filepath.Join("~", ".config", "Claude", "claude_desktop_config.json")
			},
		},
		{
			name: "a path outside home is left absolute",
			home: "set",
			path: func(home string) string { return filepath.Join(string(filepath.Separator), "etc", "mcp.json") },
			want: func(home string) string { return filepath.Join(string(filepath.Separator), "etc", "mcp.json") },
		},
		{
			name: "an unresolvable home leaves the path alone",
			home: "",
			path: func(home string) string { return filepath.Join(string(filepath.Separator), "srv", ".mcp.json") },
			want: func(home string) string { return filepath.Join(string(filepath.Separator), "srv", ".mcp.json") },
			unix: true,
		},
		{
			// A source path that is not absolute cannot be made home-relative at
			// all (filepath.Rel refuses to mix the two), so it is left exactly as
			// it came — printing a mangled path is worse than printing a long one.
			name: "a relative path is left alone",
			home: "set",
			path: func(home string) string { return filepath.Join("some", "where", ".mcp.json") },
			want: func(home string) string { return filepath.Join("some", "where", ".mcp.json") },
		},
		{
			// HOME=/ makes EVERY absolute path "under home", and the rewrite then
			// produces a LONGER string that is not even a real path
			// (~/var/folders/…). Only-ever-shorten is the guard.
			name: "a home of / never lengthens a path",
			home: "root",
			path: func(home string) string {
				return filepath.Join(string(filepath.Separator), "var", "folders", "x", "mcp.json")
			},
			want: func(home string) string {
				return filepath.Join(string(filepath.Separator), "var", "folders", "x", "mcp.json")
			},
			unix: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unix && runtime.GOOS == "windows" {
				t.Skip("$HOME-unset semantics are POSIX-only")
			}
			home := ""
			switch tc.home {
			case "set":
				home = t.TempDir()
			case "root":
				home = string(filepath.Separator)
			}
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)

			if got, want := displayPath(tc.path(home)), tc.want(home); got != want {
				t.Errorf("displayPath = %q, want %q", got, want)
			}
		})
	}
}

// TestMCPCheckCmd_RequiresApp asserts the house guard: every handler in this package
// refuses to run without the injected App rather than panicking on a nil pointer.
func TestMCPCheckCmd_RequiresApp(t *testing.T) {
	orig := App
	App = nil
	t.Cleanup(func() { App = orig })

	err := runADB(t, "mcp", "check")
	if err == nil {
		t.Fatal("want an error with no App, got nil")
	}
	if !strings.Contains(err.Error(), "app not initialized") {
		t.Errorf("error = %q, want %q", err, "app not initialized")
	}
}

// TestMCPCheck_ConciseProbeError covers the trimming of a failed http probe. The
// client wraps its cause twice and the caller already prints the url, so the raw
// chain repeats it three times and buries the one part a user acts on. Anything
// that is NOT that shape must pass through untouched — over-trimming would throw
// away the only diagnostic.
//
// The wrapper is now matched by PATTERN rather than by rebuilding the exact
// `Get "<url>": ` needle from the url we hold, and the last two cases are why:
// net/http quotes the url through stripPassword, so for any credential-bearing
// entry the url INSIDE the error is not the url we passed, the exact match never
// fired, and the whole wrapper — including its unmasked query string — survived
// into the report.
func TestMCPCheck_ConciseProbeError(t *testing.T) {
	const url = "http://127.0.0.1:59999/mcp"

	tests := []struct {
		name string
		// raw/display are the two spellings of the entry's url; display is what may
		// appear in output.
		raw     string
		display string
		err     error
		want    string
	}{
		{
			name: "the full wrapper chain collapses to the cause",
			raw:  url, display: url,
			err:  fmt.Errorf("failed to connect to MCP server: Get %q: dial tcp 127.0.0.1:59999: connect: connection refused", url),
			want: "connection refused",
		},
		{
			name: "a connection timeout keeps its wording",
			raw:  url, display: url,
			err:  fmt.Errorf("failed to connect to MCP server: Get %q: dial tcp 127.0.0.1:59999: connect: operation timed out", url),
			want: "operation timed out",
		},
		{
			name: "a non-dial cause keeps the http wrapper's own words",
			raw:  url, display: url,
			err:  fmt.Errorf("failed to connect to MCP server: Get %q: context deadline exceeded", url),
			want: "context deadline exceeded",
		},
		{
			name: "the client's own status error passes through",
			raw:  url, display: url,
			err:  errors.New("unhealthy status code: 503"),
			want: "unhealthy status code: 503",
		},
		{
			name: "an unrelated error passes through unchanged",
			raw:  url, display: url,
			err:  errors.New("something else entirely went wrong"),
			want: "something else entirely went wrong",
		},
		{
			// The case the exact-match version got wrong. net/http masked the
			// password, so its embedded url differs from ours — and the api_key in
			// the query is NOT masked, so failing to strip publishes a live token.
			name:    "a wrapper whose url differs from ours is still stripped",
			raw:     "https://svc:S3CR3T@host.invalid/mcp?api_key=sk-live-DEADBEEF",
			display: "https://svc:***@host.invalid/mcp?api_key=***",
			err: fmt.Errorf("failed to connect to MCP server: Get %q: dial tcp: lookup host.invalid: no such host",
				"https://svc:***@host.invalid/mcp?api_key=sk-live-DEADBEEF"),
			want: "dial tcp: lookup host.invalid: no such host",
		},
		{
			// Belt and braces: a raw url surviving in some OTHER position is
			// rewritten to its display spelling rather than printed.
			name:    "a raw url left anywhere else is replaced by the display form",
			raw:     "https://svc:S3CR3T@host.invalid/mcp",
			display: "https://svc:***@host.invalid/mcp",
			err:     errors.New("proxy rejected https://svc:S3CR3T@host.invalid/mcp"),
			want:    "proxy rejected https://svc:***@host.invalid/mcp",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := conciseProbeError(tc.raw, tc.display, tc.err)
			if got != tc.want {
				t.Errorf("conciseProbeError = %q, want %q", got, tc.want)
			}
			if tc.raw != tc.display && strings.Contains(got, tc.raw) {
				t.Errorf("conciseProbeError leaked the raw url: %q", got)
			}
		})
	}
}

// TestMCPCheck_RelativeCommandIsCwdIndependent pins the defect that made the same
// config report two different verdicts:
//
//	.mcp.json: {"rel": {"command": "./bin/tool"}}   # tool exists at <ws>/bin/tool
//	run from <ws>      →  ✓ launchable
//	run from <ws>/sub  →  ✗ no such file: ./bin/tool
//
// Same config, same ADB_HOME, opposite answers, because os.Stat/filepath.Abs
// resolve against the PROCESS cwd. A client launching the server resolves against
// the project — i.e. against the directory of the config file that declared it —
// so that is what the resolver is given.
func TestMCPCheck_RelativeCommandIsCwdIndependent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only semantics (the executable bit / extensionless binaries)")
	}
	ws := t.TempDir()
	binDir := filepath.Join(ws, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tool := writeExecutable(t, binDir, "tool")
	sub := filepath.Join(ws, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	server := integration.MCPServer{
		Name: "rel", Source: filepath.Join(ws, ".mcp.json"), Scope: "global",
		Transport: integration.MCPTransportStdio, Command: "./bin/tool",
	}

	for _, cwd := range []struct{ name, dir string }{
		{"from the workspace root", ws},
		{"from a subdirectory of it", sub},
		{"from somewhere else entirely", t.TempDir()},
	} {
		t.Run(cwd.name, func(t *testing.T) {
			t.Chdir(cwd.dir)
			statuses := checkMCPServers([]integration.MCPServer{server}, execCommandResolver{}, nil)
			if statuses[0].State != mcpStateLaunchable {
				t.Fatalf("State = %q (%s), want %q — the verdict must not depend on adb's cwd",
					statuses[0].State, statuses[0].Detail, mcpStateLaunchable)
			}
			if statuses[0].Resolved != tool {
				t.Errorf("Resolved = %q, want %q", statuses[0].Resolved, tool)
			}
		})
	}

	t.Run("with no owning config file it falls back to the cwd", func(t *testing.T) {
		// The documented fallback: an entry with no Source has no directory to
		// anchor to, so the cwd is all there is.
		t.Chdir(ws)
		noSource := server
		noSource.Source = ""
		statuses := checkMCPServers([]integration.MCPServer{noSource}, execCommandResolver{}, nil)
		if statuses[0].State != mcpStateLaunchable {
			t.Errorf("State = %q (%s), want %q", statuses[0].State, statuses[0].Detail, mcpStateLaunchable)
		}
	})
}

// TestMCPCheck_ExecCommandResolver covers the real resolver against fixtures built in a
// t.TempDir() with a pinned PATH. It deliberately does NOT ask what is installed on
// this machine — the previous implementation's exec.LookPath("npx") made the
// suite's result a function of the developer's toolchain, which is the sin this
// file exists to avoid.
func TestMCPCheck_ExecCommandResolver(t *testing.T) {
	tests := []struct {
		name string
		// setup builds the fixture and returns the command to resolve plus the path
		// it must resolve to ("" when an error is expected).
		setup func(t *testing.T, dir string) (command, wantResolved string)
		// wantErrSubstr is a fragment the error must contain; "" means success.
		wantErrSubstr string
		// skipOnWindows marks a case whose semantics are POSIX-only.
		skipOnWindows bool
		// skipAsRoot marks a case that relies on a permission actually denying.
		skipAsRoot bool
	}{
		{
			name: "a bare name found on PATH resolves to its PATH entry",
			setup: func(t *testing.T, dir string) (string, string) {
				bin := writeExecutable(t, dir, "adb-fixture-tool")
				t.Setenv("PATH", dir)
				return "adb-fixture-tool", bin
			},
			skipOnWindows: true,
		},
		{
			name: "a bare name not on PATH is a clear command-not-found",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Setenv("PATH", dir) // empty dir
				return "adb-fixture-definitely-absent", ""
			},
			wantErrSubstr: `command not found: "adb-fixture-definitely-absent"`,
		},
		{
			name: "a bare name on PATH without the executable bit is not found",
			setup: func(t *testing.T, dir string) (string, string) {
				writeFileMode(t, dir, "adb-fixture-notexec", 0o644)
				t.Setenv("PATH", dir)
				return "adb-fixture-notexec", ""
			},
			wantErrSubstr: `command not found: "adb-fixture-notexec"`,
			skipOnWindows: true,
		},
		{
			name: "an absolute path that exists and is executable resolves to itself",
			setup: func(t *testing.T, dir string) (string, string) {
				bin := writeExecutable(t, dir, "abs-tool")
				return bin, bin
			},
			skipOnWindows: true,
		},
		{
			name: "an absolute path that exists but is not executable is rejected",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeFileMode(t, dir, "not-exec", 0o644), ""
			},
			wantErrSubstr: "not executable:",
			skipOnWindows: true, // the executable bit has no meaning on windows
		},
		{
			// The Perm()&0o111 bug: 0o001 sets an execute bit, so the old check said
			// "✓ launchable" for a file THIS user cannot execute. The realistic hits
			// are a root-owned 0o700 tool and a group-restricted 0o750 one; this is
			// the same question, expressible without another uid.
			name: "a file only OTHERS may execute is not launchable by this user",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeFileMode(t, dir, "other-exec-only", 0o001), ""
			},
			wantErrSubstr: "not executable:",
			skipOnWindows: true,
			skipAsRoot:    true,
		},
		{
			name: "a path that does not exist says so",
			setup: func(t *testing.T, dir string) (string, string) {
				return filepath.Join(dir, "nope", "missing-binary"), ""
			},
			wantErrSubstr: "no such file:",
		},
		{
			// A trailing space is a real misconfiguration and used to be reported as
			// `no such file: /bin/ls` — a file that plainly exists — because the
			// message interpolated the command bare and printMCPCheck then trimmed
			// the trailing space off the line. Quoting makes it visible.
			name: "a trailing space stays visible in the message",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeExecutable(t, dir, "spaced-tool") + " ", ""
			},
			wantErrSubstr: `spaced-tool "`, // i.e. ...spaced-tool " — the space is inside the quotes
			skipOnWindows: true,
		},
		{
			// os.Stat FOLLOWS the link, so a dangling symlink reported "no such
			// file" for a path that is right there. os.Lstat first names the real
			// problem and where the link points.
			name: "a broken symlink says so, and where it points",
			setup: func(t *testing.T, dir string) (string, string) {
				link := filepath.Join(dir, "dangling")
				if err := os.Symlink(filepath.Join(dir, "gone-tool"), link); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return link, ""
			},
			wantErrSubstr: "broken symlink:",
			skipOnWindows: true,
		},
		{
			name: "a symlink to a real executable resolves",
			setup: func(t *testing.T, dir string) (string, string) {
				target := writeExecutable(t, dir, "real-tool")
				link := filepath.Join(dir, "linked-tool")
				if err := os.Symlink(target, link); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return link, link
			},
			skipOnWindows: true,
		},
		{
			// A symlink to a DIRECTORY must report the directory, not sail past the
			// is-a-directory check because Lstat only saw a link.
			name: "a symlink to a directory is not a launchable command",
			setup: func(t *testing.T, dir string) (string, string) {
				target := filepath.Join(dir, "a-directory")
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				link := filepath.Join(dir, "linked-dir")
				if err := os.Symlink(target, link); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return link, ""
			},
			wantErrSubstr: "not a file:",
			skipOnWindows: true,
		},
		{
			// A stat failure that is NOT "does not exist" must surface as itself
			// rather than being flattened into "no such file", which would send the
			// user looking for a missing binary that is actually right there.
			name: "an unreadable parent directory surfaces the real stat error",
			setup: func(t *testing.T, dir string) (string, string) {
				locked := filepath.Join(dir, "locked")
				if err := os.MkdirAll(locked, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				target := writeExecutable(t, locked, "tool")
				if err := os.Chmod(locked, 0o000); err != nil {
					t.Fatalf("chmod 000: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
				return target, ""
			},
			wantErrSubstr: "permission denied",
			skipOnWindows: true,
			skipAsRoot:    true,
		},
		{
			name: "a directory is not a launchable command",
			setup: func(t *testing.T, dir string) (string, string) {
				sub := filepath.Join(dir, "a-directory")
				if err := os.MkdirAll(sub, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				return sub, ""
			},
			wantErrSubstr: "not a file:",
		},
		{
			// A relative path with no base directory falls back to the cwd, and the
			// message names BOTH spellings so the reader can tell which directory
			// was searched.
			name: "a relative path names what it resolved to",
			setup: func(t *testing.T, dir string) (string, string) {
				t.Chdir(dir)
				return filepath.Join(".", "nope", "missing-binary"), ""
			},
			wantErrSubstr: "(resolved to ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipOnWindows && runtime.GOOS == "windows" {
				t.Skip("POSIX-only semantics (the executable bit / extensionless binaries)")
			}
			if tc.skipAsRoot && os.Geteuid() == 0 {
				t.Skip("file permissions do not deny root")
			}
			dir := t.TempDir()
			command, wantResolved := tc.setup(t, dir)

			// An empty base dir: these fixtures use absolute paths or the cwd
			// fallback. The config-relative behaviour is
			// TestMCPCheck_RelativeCommandIsCwdIndependent.
			got, err := execCommandResolver{}.Resolve(command, "")

			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("Resolve(%q) = %q, want an error containing %q", command, got, tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Errorf("Resolve(%q) error = %q, want it to contain %q", command, err, tc.wantErrSubstr)
				}
				if got != "" {
					t.Errorf("Resolve(%q) returned %q alongside an error; want empty", command, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q): %v", command, err)
			}
			if got != wantResolved {
				t.Errorf("Resolve(%q) = %q, want %q", command, got, wantResolved)
			}
		})
	}
}

// writeExecutable creates a runnable fixture file and returns its path. It is never
// executed — only stat'd, access-checked, or found on PATH — so the contents only
// have to be a plausible script.
func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	return writeFileMode(t, dir, name, 0o755)
}

func writeFileMode(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	// os.WriteFile applies umask to the mode, so set it explicitly.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	return path
}

// TestMCPCheckCmd_Registered is the wiring sanity check: `adb mcp check` exists
// with the three flags the docs promise.
func TestMCPCheckCmd_Registered(t *testing.T) {
	mcpCmd := findCobraSub(NewRootCmd(), "mcp")
	if mcpCmd == nil {
		t.Fatal("mcp command not registered")
	}
	checkCmd := findCobraSub(mcpCmd, "check")
	if checkCmd == nil {
		t.Fatal("mcp check not registered")
	}
	for _, flag := range []string{"json", "no-cache", "exit-code"} {
		if checkCmd.Flags().Lookup(flag) == nil {
			t.Errorf("mcp check has no --%s flag", flag)
		}
	}
	if checkCmd.Args == nil {
		t.Error("mcp check should reject positional arguments (cobra.NoArgs)")
	}
}
