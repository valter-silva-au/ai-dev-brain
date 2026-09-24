package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/valter-silva-au/ai-dev-brain/internal/integration"
)

// This file is `adb mcp check`. It replaces an implementation that could not tell
// the truth about anything:
//
//   - it read ONLY claude_desktop_config.json, so a Claude Code user (whose
//     servers live in ~/.claude.json) was told "no MCP servers configured";
//   - its per-server check ignored the server name entirely and returned one
//     global verdict repeated N times;
//   - that verdict asked whether `npx` was on PATH, whatever the server's actual
//     command was, and then GET localhost:3000/health — meaningless for a stdio
//     server, and a false "healthy" for every server whenever anything unrelated
//     answered on that port;
//   - `--no-cache` controlled nothing, because the code path had no cache.
//
// What it reports now is deliberately narrower and true: for a stdio server, that
// its configured command RESOLVES to an executable (the thing that actually breaks
// — a renamed binary, a tool not installed, a stale absolute path). That is
// launchability, not liveness, and the output says so rather than printing a
// "healthy" it cannot know. For an http server it reports only that the url
// ANSWERED, because without an MCP handshake nothing stronger is knowable.
//
// Two properties are load-bearing enough to state up front, because both were
// defects here once and neither is visible from reading a happy-path run:
//
//   - EVERY url that reaches output — human or JSON — is MCPServer.URLDisplay, the
//     redacted spelling. MCPServer.URL keeps the raw url solely so the probe can
//     dial it, and is `json:"-"` so it cannot reach a report by accident. An MCP
//     url routinely carries userinfo or an `api_key=` query parameter, and this
//     report is both printed to a terminal and marshalled for scripts. The same
//     rule covers a redirect's DESTINATION: MCPHealthCheck.Location arrives already
//     redacted (integration.RedactLocation) and has no raw counterpart, so this
//     file cannot un-redact what it never received.
//   - A verdict must not depend on adb's cwd. A relative `command` is resolved
//     against the directory of the CONFIG FILE that declared it (MCPServer.Source),
//     which is what a client launching that server would do — not against whatever
//     directory `adb` happened to be started from.

// mcpCheckState is one server's verdict.
type mcpCheckState string

const (
	// mcpStateLaunchable means a stdio server's command resolved to an executable.
	mcpStateLaunchable mcpCheckState = "launchable"
	// mcpStateUnresolved means it did not — the actionable failure.
	mcpStateUnresolved mcpCheckState = "unresolved"
	// mcpStateReachable means the url ANSWERED — nothing more. It is annotated with
	// the status code precisely so it cannot be read as "the server is healthy":
	// a Streamable-HTTP MCP endpoint correctly answers a bare GET (no
	// `Accept: text/event-stream`) with 405/406/400, and a 3xx to an SSO login page
	// answers too while never reaching an MCP server at all. Without a handshake,
	// "it answered" is the strongest honest claim available.
	mcpStateReachable mcpCheckState = "reachable"
	// mcpStateUnreachable means NO http response came back: connection refused,
	// DNS failure, TLS failure, timeout. A transport error, not a status code.
	mcpStateUnreachable mcpCheckState = "unreachable"
	// mcpStateUnknown means nothing could be checked — the entry declared neither a
	// usable command nor a url, or no prober was available. A malformed entry is
	// surfaced, not hidden.
	mcpStateUnknown mcpCheckState = "unknown"
)

// ok reports whether a state counts as a pass for --exit-code. This is the SINGLE
// definition of "usable": the flag counts `!ok()` rather than re-deriving the same
// rule from the summary's fields, so a state added here cannot silently be counted
// the other way there.
func (s mcpCheckState) ok() bool {
	return s == mcpStateLaunchable || s == mcpStateReachable
}

// maxMCPProbeConcurrency bounds how many http probes are in flight at once.
//
// Serial probing made the command unusable against remote servers: three
// unroutable urls took the sum of their timeouts (measured: 30s) with no output
// until the very end. Probing is pure IO wait, so the cap exists only to avoid
// opening an unreasonable number of sockets at once, not to protect adb.
const maxMCPProbeConcurrency = 8

// mcpProbeBudget bounds the WHOLE probe phase, so no config can make the command
// hang for minutes however many remote servers it lists. It is deliberately much
// larger than the client's per-request timeout — it is a backstop for a
// pathological config, not the normal timeout — and it is a var so a test can
// shrink it instead of sleeping.
var mcpProbeBudget = 30 * time.Second

// commandResolver resolves a stdio server's command to an executable path.
//
// It is an interface so a test can answer without depending on what happens to be
// installed on the machine running it — the previous implementation's
// exec.LookPath("npx") made the suite's result a function of the developer's
// toolchain. Defined here, where it is consumed (house style).
type commandResolver interface {
	// Resolve returns the absolute path a command would launch from, or an error
	// explaining why it would not launch.
	//
	// baseDir is the directory a RELATIVE command is resolved against — the
	// directory of the config file the entry came from. It is a parameter rather
	// than ambient state because os.Stat/filepath.Abs resolve against the process
	// cwd, which made the same config report `launchable` from the workspace root
	// and `no such file` from a subdirectory of it. An empty baseDir falls back to
	// the cwd, which is only reached by an entry with no known source file.
	Resolve(command, baseDir string) (string, error)
}

// execCommandResolver is the real resolver: a PATH lookup for a bare name, and for
// a path, a stat for a clear diagnosis followed by exec.LookPath for the
// executability decision itself.
type execCommandResolver struct{}

func (execCommandResolver) Resolve(command, baseDir string) (string, error) {
	// A command containing a separator is a path, not a PATH lookup.
	if strings.ContainsRune(command, os.PathSeparator) {
		return resolveCommandPath(command, baseDir)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("command not found: %s", quoteCommand(command))
	}
	return path, nil
}

// resolveCommandPath handles the path form of a command.
//
// The stat dance before exec.LookPath is not redundant: LookPath's own error for a
// path is `exec: "<p>": permission denied`, which does not distinguish "the file
// is not there", "that is a directory", and "you may not execute it" — and those
// are three different things to go and fix. So the clearly-diagnosable cases are
// named here, and the one judgement that is easy to get WRONG by hand is delegated:
// whether the current user can actually execute the file.
//
// That delegation is the point. This branch used to test `info.Mode().Perm()&0o111
// == 0`, which asks whether ANY of the three execute bits is set — so a `chmod 001`
// file the current user cannot possibly run reported `✓ launchable`, and so did a
// root-owned 0o700 tool and a group-restricted 0o750 one. Those are the realistic
// hits, not exotica. exec.LookPath performs an access(2)-style check with the
// EFFECTIVE uid/gid, which is the same question the OS will answer when a client
// actually tries to spawn the server — and it is what the bare-name branch above
// has always used, so the path branch was strictly weaker than the branch it was
// written to improve on.
func resolveCommandPath(command, baseDir string) (string, error) {
	target := command
	if !filepath.IsAbs(target) {
		switch {
		case baseDir != "":
			target = filepath.Join(baseDir, target)
		default:
			// No owning config file: fall back to the cwd, the historical behaviour.
			if abs, err := filepath.Abs(target); err == nil {
				target = abs
			}
		}
	}
	desc := describeCommand(command, target)

	// Lstat, not Stat: Stat FOLLOWS a symlink, so a dangling link reported
	// "no such file: <p>" for a path that plainly exists — sending the reader to
	// look for a missing file rather than at the link that needs repointing.
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such file: %s", desc)
		}
		// Anything else (an unreadable parent directory, most often) surfaces as
		// itself rather than being flattened into "no such file".
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		followed, serr := os.Stat(target)
		if serr != nil {
			if link, rerr := os.Readlink(target); rerr == nil {
				return "", fmt.Errorf("broken symlink: %s → %s", desc, link)
			}
			return "", serr
		}
		// Re-stat through the link so the is-a-directory test below sees the target.
		info = followed
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file: %s", desc)
	}
	resolved, err := exec.LookPath(target)
	if err != nil {
		return "", fmt.Errorf("not executable: %s", desc)
	}
	return resolved, nil
}

// quoteCommand renders a command for a message. It QUOTES rather than
// interpolating bare, because trailing or embedded whitespace is otherwise erased
// twice over — once by the reader's eye, once by printMCPCheck's TrimRight — and
// `no such file: /bin/ls` for a configured `"/bin/ls "` reads as an outright lie
// about a file that is right there.
func quoteCommand(command string) string {
	return fmt.Sprintf("%q", command)
}

// describeCommand names the command as configured, and where it resolved to when
// the two differ. A relative command's verdict is otherwise undiagnosable: the
// reader cannot tell which directory it was resolved against.
func describeCommand(command, target string) string {
	if target == command {
		return quoteCommand(command)
	}
	return fmt.Sprintf("%s (resolved to %s)", quoteCommand(command), target)
}

// mcpCommandBaseDir is the directory a relative command from this entry resolves
// against: the directory of the config file that declared it. A client launching
// the server resolves relative to the project, not to adb's cwd.
func mcpCommandBaseDir(s integration.MCPServer) string {
	if s.Source == "" {
		return ""
	}
	return filepath.Dir(s.Source)
}

// mcpServerStatus is one row of the report.
type mcpServerStatus struct {
	integration.MCPServer
	State mcpCheckState `json:"state"`
	// Resolved is the executable path a stdio command resolved to.
	Resolved string `json:"resolved,omitempty"`
	// Detail explains a non-ok state, or annotates an ok one.
	Detail string `json:"detail,omitempty"`
}

// mcpCheckReport is the whole `--json` shape.
type mcpCheckReport struct {
	Sources []integration.MCPConfigSource `json:"sources"`
	Servers []mcpServerStatus             `json:"servers"`
	Summary mcpCheckSummary               `json:"summary"`
}

type mcpCheckSummary struct {
	Total       int `json:"total"`
	Launchable  int `json:"launchable"`
	Unresolved  int `json:"unresolved"`
	Reachable   int `json:"reachable"`
	Unreachable int `json:"unreachable"`
	Unknown     int `json:"unknown"`
}

// checkMCPServers classifies every discovered server. It is pure with respect to
// its injected dependencies, so the whole matrix is testable without a real home
// directory, a real PATH, or a network.
//
// httpClient may be nil, in which case an http server is reported unknown rather
// than probed — a caller that does not want network access does not get any.
//
// It runs in two passes: everything decidable locally is classified in input
// order, and the http entries are probed CONCURRENTLY afterwards. Results are
// written back by index, so the returned slice is always in the caller's order
// however the probes happened to finish (the report is sorted by name, and a
// report whose row order shifted run-to-run would be undiffable).
func checkMCPServers(servers []integration.MCPServer, res commandResolver, httpClient integration.MCPClient) []mcpServerStatus {
	out := make([]mcpServerStatus, len(servers))
	// probeURLs maps a row index to the RAW url to dial. Raw, because a redacted
	// url does not resolve — this is the one place MCPServer.URL is legitimately
	// read, and it never reaches output.
	probeURLs := map[int]string{}

	for i, s := range servers {
		st := mcpServerStatus{MCPServer: s}
		switch {
		case s.Transport == integration.MCPTransportHTTP && s.URL != "":
			if httpClient == nil {
				st.State = mcpStateUnknown
				st.Detail = "not probed"
				break
			}
			probeURLs[i] = s.URL
		case strings.TrimSpace(s.Command) != "":
			resolved, err := res.Resolve(s.Command, mcpCommandBaseDir(s))
			if err != nil {
				st.State = mcpStateUnresolved
				st.Detail = err.Error()
				break
			}
			st.State = mcpStateLaunchable
			st.Resolved = resolved
		default:
			// Neither a url nor a command with anything in it. A whitespace-only
			// command lands here too, alongside an empty one: they are the same
			// defect, so they must not classify differently (they once did —
			// `unresolved` for "   ", `unknown` for "" — which made the same
			// mistake look like two).
			st.State = mcpStateUnknown
			st.Detail = mcpNothingToCheckDetail(s)
		}
		out[i] = st
	}

	if len(probeURLs) > 0 {
		probes := probeMCPServers(httpClient, probeURLs)
		for i := range probeURLs {
			probe, answered := probes[i]
			out[i] = classifyMCPProbe(out[i], probe, answered)
		}
	}
	return out
}

// mcpNothingToCheckDetail explains an unknown entry without misdescribing it. A
// blank command is not the same complaint as no command at all, even though both
// leave nothing to check.
func mcpNothingToCheckDetail(s integration.MCPServer) string {
	if s.Command != "" {
		return fmt.Sprintf("entry declares a blank command (%s)", quoteCommand(s.Command))
	}
	return "entry declares neither a command nor a url"
}

// mcpProbe is one probe outcome, tagged with the rows it belongs to so results can
// be reassembled in the report's order rather than completion order.
type mcpProbe struct {
	health integration.MCPHealthCheck
	err    error
	// rows are every report row this one probe answers — more than one when
	// several entries name the same url and the probe was coalesced.
	rows []int
	url  string
}

// planMCPProbes turns the rows needing a probe into the probes to actually run.
//
// By default, rows sharing a url become ONE probe whose result is fanned out to
// each of them. That coalescing used to fall out of MCPClient's in-memory TTL
// cache, and it cannot any more: with probes running concurrently, two entries
// naming the same url both dial before either has a result to cache, so the
// "second one is free" property became a race the cache always lost. Making it a
// planning decision restores it deterministically — and, more to the point, keeps
// --no-cache meaningful, since a flag whose only observable effect had quietly
// evaporated is precisely the state this command was rewritten out of.
//
// Whether to coalesce is read off the CLIENT — uncachedMCPClient is precisely what
// --no-cache installs — so the flag keeps exactly ONE definition. Threading a
// `coalesce bool` down from newMCPCheckCmd alongside the decorator would state the
// same decision twice and let the two drift, which is the shape of defect this file
// has already been bitten by (see mcpCheckState.ok).
//
// Order is by first row index so a fixture's probe log is deterministic.
func planMCPProbes(client integration.MCPClient, urls map[int]string) []mcpProbe {
	idxs := make([]int, 0, len(urls))
	for idx := range urls {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)

	if _, independent := client.(uncachedMCPClient); independent {
		probes := make([]mcpProbe, 0, len(idxs))
		for _, idx := range idxs {
			probes = append(probes, mcpProbe{rows: []int{idx}, url: urls[idx]})
		}
		return probes
	}

	probes := make([]mcpProbe, 0, len(idxs))
	byURL := map[string]int{} // url → its slot in probes
	for _, idx := range idxs {
		url := urls[idx]
		if slot, seen := byURL[url]; seen {
			probes[slot].rows = append(probes[slot].rows, idx)
			continue
		}
		byURL[url] = len(probes)
		probes = append(probes, mcpProbe{rows: []int{idx}, url: url})
	}
	return probes
}

// probeMCPServers probes the given rows concurrently and returns what came back,
// keyed by row index. A row missing from the result is one that did not answer
// within mcpProbeBudget.
func probeMCPServers(client integration.MCPClient, urls map[int]string) map[int]mcpProbe {
	planned := planMCPProbes(client, urls)

	jobs := make(chan mcpProbe, len(planned))
	for _, p := range planned {
		jobs <- p
	}
	close(jobs)

	// Buffered to len(planned) so a worker's send can NEVER block. That matters
	// because the collector below may walk away at the budget: with an unbuffered
	// channel every outstanding worker would park on its send forever, leaking a
	// goroutine (and its socket) for the life of the process. Buffered, an
	// abandoned worker completes its send and exits, and — because only the
	// collector touches `out` — nothing races with the caller.
	results := make(chan mcpProbe, len(planned))
	workers := len(planned)
	if workers > maxMCPProbeConcurrency {
		workers = maxMCPProbeConcurrency
	}
	for w := 0; w < workers; w++ {
		go func() {
			for p := range jobs {
				p.health, p.err = client.CheckHealth(p.url)
				results <- p
			}
		}()
	}

	out := make(map[int]mcpProbe, len(urls))
	budget := time.After(mcpProbeBudget)
	for range planned {
		select {
		case r := <-results:
			for _, idx := range r.rows {
				out[idx] = r
			}
		case <-budget:
			return out
		}
	}
	return out
}

// classifyMCPProbe turns one probe outcome into a verdict.
//
// The policy, which the previous version got wrong in BOTH directions:
//
//   - ANY http response ⇒ reachable, annotated with its status. A healthy
//     Streamable-HTTP MCP endpoint answers a bare GET with 405 (correctly — the GET
//     carried no `Accept: text/event-stream`), and calling that `✗ unreachable` sent
//     people looking for a server that was working.
//   - A 3xx ⇒ reachable, SAID to be a redirect, and — when the response named one —
//     told WHERE it went (mcpRedirectNote). A url that 302s to an SSO login page
//     previously reported `✓ reachable` off the login page's 200 — the surviving
//     half of the old "anything that answers is healthy" bug. The response came from
//     the redirector, not from an MCP server, and the reader has to see that; the
//     destination is what turns "something redirected me" into a diagnosis.
//   - No response at all (connection refused, DNS, TLS, timeout) ⇒ unreachable.
//     This is the only case where nothing answered, and so the only honest ✗.
func classifyMCPProbe(st mcpServerStatus, probe mcpProbe, answered bool) mcpServerStatus {
	display := mcpDisplayURL(st.MCPServer)
	switch {
	case !answered:
		// The probe phase's overall budget expired before this row came back.
		st.State = mcpStateUnreachable
		st.Detail = fmt.Sprintf("%s: no response within %s", display, mcpProbeBudget)
	case probe.health.StatusCode != 0:
		st.State = mcpStateReachable
		st.Detail = fmt.Sprintf("%s: status %d", display, probe.health.StatusCode)
		if probe.health.StatusCode >= 300 && probe.health.StatusCode < 400 {
			st.Detail += mcpRedirectNote(probe.health.Location)
		}
	case probe.err != nil:
		st.State = mcpStateUnreachable
		st.Detail = fmt.Sprintf("%s: %s", display,
			conciseProbeError(st.URL, display, probe.err))
	default:
		// No status code, no error: the client promises one or the other, so this
		// is a client bug rather than anything about the server. Report the url and
		// claim nothing.
		st.State = mcpStateUnreachable
		st.Detail = display
	}
	return st
}

// mcpRedirectNote annotates a 3xx: where it went, and why that matters.
//
// The destination is the single most useful fact here — "it sent you to your
// identity provider" and "it sent you to a path you mistyped" are different
// problems with different fixes, and `status 302` alone separates neither. So it is
// named whenever the response carried a Location, and the explanation stays
// alongside it, because knowing the destination does not by itself tell a reader
// that the answer did not come from their MCP server.
//
// When no Location came back the note degrades to the original generic form rather
// than printing an empty arrow. A 3xx without a Location is malformed, but a real
// server can emit one, and this command's job in that case is to stay legible.
// (That is also why the destination-less wording keeps "typically an SSO login
// page": with no destination, the likely case is the only guidance available. Once
// the destination IS known, guessing at it would be noise sitting next to the fact.)
//
// The argument is integration.MCPHealthCheck.Location, which is redacted at the
// probe boundary — a redirect to an authentication endpoint routinely carries a
// `code=`/`state=`/`redirect_uri=` parameter. There is deliberately no raw spelling
// of it anywhere in reach, which is the same property that makes MCPServer.URL
// `json:"-"`: this file cannot leak what it was never handed.
func mcpRedirectNote(location string) string {
	if location == "" {
		return " (redirect — the response came from the redirector, " +
			"typically an SSO login page, not from an MCP server)"
	}
	return fmt.Sprintf(" (redirect → %s — the response came from the redirector, "+
		"not from an MCP server)", location)
}

// mcpDisplayURL is the ONLY url spelling allowed out of this file.
//
// Discovery fills URLDisplay with the redacted form; the fallback is belt and
// braces for an entry that somehow arrives without one — redact here rather than
// print a raw url, because the failure mode is a leaked credential in a terminal
// and in JSON, and it is silent.
func mcpDisplayURL(s integration.MCPServer) string {
	if s.URLDisplay != "" {
		return s.URLDisplay
	}
	if s.URL == "" {
		return ""
	}
	return integration.RedactURL(s.URL)
}

// uncachedMCPClient is what --no-cache installs, and it does two things: its
// PRESENCE tells planMCPProbes to schedule one probe per entry rather than one per
// distinct url, and it clears the TTL cache before every probe.
//
// Both halves are needed, and neither is the obvious implementation:
//
//   - Clearing once at startup would be a no-op. MCPClient's cache is in-memory and
//     the client is built per invocation, so there is nothing cached yet — the flag
//     would look wired while doing exactly what the old no-op version did.
//   - Clearing before every probe is still not sufficient on its own, because the
//     planner may have coalesced two entries into one probe before the client is
//     ever called. Hence planMCPProbes reading the type.
//   - And the planning alone is not sufficient either: with more entries than
//     workers, a repeated url can come round again after the first probe has
//     already populated the cache, and only the clear stops that second entry
//     silently reusing the first's answer.
type uncachedMCPClient struct{ integration.MCPClient }

func (u uncachedMCPClient) CheckHealth(url string) (integration.MCPHealthCheck, error) {
	u.ClearCache()
	return u.MCPClient.CheckHealth(url)
}

// probeGetWrapper matches net/http's own error wrapper, `Get "<url>": <reason>`
// (url.Error renders as `<Op> <quoted url>: <err>`).
//
// It is matched by PATTERN, not by rebuilding the exact string from a url we hold,
// and that is the whole point: net/http quotes the url through stripPassword, so
// the url INSIDE the error (`https://svc:***@host/…`) is not the url we passed
// (`https://svc:S3CR3T@host/…`). An exact-match strip therefore never fired on
// precisely the entries that most need it, and the url ended up printed twice.
var probeGetWrapper = regexp.MustCompile(`^[A-Za-z]+ "[^"]*": `)

// conciseProbeError trims the layers a failed probe accumulates. The client wraps
// its cause as `failed to connect to MCP server: Get "<url>": <real reason>`, and
// the caller already prints the (redacted) url — so quoting the whole chain repeats
// the url three times and buries the one part that matters ("connection refused").
//
// It is also a redaction boundary, not merely a tidying pass. net/http's wrapper
// masks a url's PASSWORD but leaves its query string intact, so an unstripped
// `Get "https://svc:***@host/mcp?api_key=sk-live-…"` publishes the token. rawURL
// and displayURL are both taken so that anything surviving the trims can have the
// raw url substituted for its redacted spelling as a last line of defence.
func conciseProbeError(rawURL, displayURL string, err error) string {
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "failed to connect to MCP server: ")
	msg = probeGetWrapper.ReplaceAllString(msg, "")
	// The transport layer then prefixes `dial tcp <addr>: `, which restates the
	// host the url already gave.
	if _, rest, found := strings.Cut(msg, ": connect: "); found {
		msg = rest
	}
	if rawURL != "" && displayURL != "" && rawURL != displayURL {
		msg = strings.ReplaceAll(msg, rawURL, displayURL)
	}
	return msg
}

// summarize counts states for the footer and for --exit-code.
func summarize(statuses []mcpServerStatus) mcpCheckSummary {
	s := mcpCheckSummary{Total: len(statuses)}
	for _, st := range statuses {
		switch st.State {
		case mcpStateLaunchable:
			s.Launchable++
		case mcpStateUnresolved:
			s.Unresolved++
		case mcpStateReachable:
			s.Reachable++
		case mcpStateUnreachable:
			s.Unreachable++
		case mcpStateUnknown:
			s.Unknown++
		default:
			// A state added without a counter lands in Unknown rather than
			// vanishing from a summary whose Total already counted it.
			s.Unknown++
		}
	}
	return s
}

// mcpCheckDiscover is the discovery seam. Tests replace it to point at fixture
// config files instead of a real home directory, the same way
// taskRunWithRufloCommander is swapped elsewhere in this package.
var mcpCheckDiscover = func(workspaceDir string) ([]integration.MCPServer, []integration.MCPConfigSource, error) {
	opts, err := integration.DefaultMCPDiscoveryOptions(workspaceDir)
	if err != nil {
		return nil, nil, err
	}
	servers, sources := integration.DiscoverMCPServers(opts)
	return servers, sources, nil
}

// mcpCheckResolver is the command-resolution seam, swapped by tests so the result
// does not depend on the machine's installed tooling.
var mcpCheckResolver commandResolver = execCommandResolver{}

// mcpCheckNewClient is the http-prober seam. Without it the command's only prober
// was a real integration.NewMCPClient, which meant no test could observe the
// --no-cache wiring and no command-level fixture could carry an http server
// without making a real network call — so the entire http half of the command was
// unreachable from a test.
var mcpCheckNewClient = integration.NewMCPClient

// newMCPCheckCmd creates the 'mcp check' command.
func newMCPCheckCmd() *cobra.Command {
	var (
		noCache    bool
		jsonOutput bool
		exitCode   bool
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check that configured MCP servers can be launched",
		Long: `Report the MCP servers this machine has configured and whether each one
could be launched.

Servers are discovered from every place a client keeps them, most specific first:

  <workspace>/.mcp.json                                  (project, and what ` + "`adb harness build`" + ` emits)
  ~/.claude.json                                         (Claude Code, incl. its per-project blocks)
  ~/.config/Claude/claude_desktop_config.json            (Claude Desktop)
  ~/Library/Application Support/Claude/…                 (Claude Desktop, macOS)

A name configured in more than one file is reported once, from the most specific
source. Every file looked at is listed, including the ones that were absent —
"I looked here and found nothing" is usually the answer you need.

For a stdio server this verifies its configured COMMAND resolves to an
executable: a PATH lookup for a bare name, and for a path, whether THIS user can
execute it. That catches the failures that actually happen — an uninstalled tool,
a renamed binary, a stale absolute path, a root-owned binary. A relative command
is resolved against the directory of the config file that declared it, not
against the directory adb was run from. It does NOT start the server or speak MCP
to it, so it reports "launchable", not "healthy".

For an http/sse server the url is probed with one GET, and any answer at all —
405, 401, 500 included — reports "reachable", because a real MCP endpoint
correctly rejects a bare GET and only a handshake could tell them apart. A
redirect is reported as such — it means the answer came from the redirector, not
from an MCP server — and names where it pointed when the response said. Only a
transport failure (refused, DNS, TLS, timeout) is "unreachable". Urls, including a
redirect's destination, are printed redacted. Probes run concurrently and are never
cached between runs, so every invocation reports what is true now.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if App == nil {
				return fmt.Errorf("app not initialized")
			}

			servers, sources, err := mcpCheckDiscover(App.BasePath)
			if err != nil {
				return fmt.Errorf("failed to discover mcp servers: %w", err)
			}

			// The HTTP client is only built when something actually needs probing,
			// so a workspace of stdio servers makes no network calls at all.
			//
			// Scope of the reuse --no-cache suppresses, stated precisely because the
			// old flag implied something that never existed: nothing is EVER reused
			// between runs (MCPClient's TTL cache is in-memory and the client is
			// constructed per invocation), so every `adb mcp check` probes fresh.
			// That is the right default for a diagnostic — you run it to find out
			// what is true now, not what was true 30 seconds ago. Within a single
			// run, reuse only arises when two entries name the SAME url, which the
			// probe planner coalesces into one probe and --no-cache splits back into
			// two independent ones.
			var httpClient integration.MCPClient
			for _, s := range servers {
				if s.Transport == integration.MCPTransportHTTP && s.URL != "" {
					httpClient = mcpCheckNewClient(30 * time.Second)
					if noCache {
						httpClient = uncachedMCPClient{httpClient}
					}
					break
				}
			}

			statuses := checkMCPServers(servers, mcpCheckResolver, httpClient)
			report := mcpCheckReport{
				Sources: sources,
				Servers: statuses,
				Summary: summarize(statuses),
			}

			if jsonOutput {
				if err := printJSON(report); err != nil {
					return err
				}
			} else {
				printMCPCheck(report)
			}

			// --exit-code makes an unusable server a non-zero exit (for CI gates),
			// matching `adb audit security` and `adb conformance check`. It counts
			// state.ok() rather than re-adding the summary's fields, so "usable" has
			// exactly one definition (see mcpCheckState.ok).
			if exitCode {
				bad := 0
				for _, st := range report.Servers {
					if !st.State.ok() {
						bad++
					}
				}
				if bad > 0 {
					return fmt.Errorf("%d of %d mcp server(s) unusable", bad, report.Summary.Total)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false,
		"probe each http/sse entry separately, even when two entries share a url "+
			"(results are never cached between runs)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the report as JSON")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit non-zero when a server is unusable (for CI/gates)")
	return cmd
}

// mcpStateGlyph maps a verdict to its one-character marker.
func mcpStateGlyph(s mcpCheckState) string {
	switch s {
	case mcpStateLaunchable, mcpStateReachable:
		return "✓"
	case mcpStateUnresolved, mcpStateUnreachable:
		return "✗"
	case mcpStateUnknown:
		return "?"
	default:
		// Nothing was checked, so claim neither ✓ nor ✗ — the same marker the
		// unknown state carries.
		return "?"
	}
}

// printMCPCheck renders the human report.
func printMCPCheck(r mcpCheckReport) {
	// Resolved ONCE per render rather than per path: displayPath used to call
	// os.UserHomeDir() for every line it printed.
	home := displayHome()

	if len(r.Servers) == 0 {
		fmt.Println("No MCP servers configured.")
		printMCPSources(r.Sources, home)
		return
	}

	// Group by source file so a user sees which config owns which server.
	bySource := map[string][]mcpServerStatus{}
	for _, st := range r.Servers {
		bySource[st.Source] = append(bySource[st.Source], st)
	}
	order := make([]string, 0, len(bySource))
	for src := range bySource {
		order = append(order, src)
	}
	sort.Strings(order)

	nameWidth := 0
	for _, st := range r.Servers {
		if len(st.Name) > nameWidth {
			nameWidth = len(st.Name)
		}
	}

	for _, src := range order {
		rows := bySource[src]
		fmt.Printf("%s (%d)\n", shortenPath(src, home), len(rows))
		for _, st := range rows {
			detail := st.Detail
			if st.State == mcpStateLaunchable {
				detail = st.Resolved
				// Name a PATH lookup as such: "npx → /opt/homebrew/bin/npx" says
				// more than either half alone.
				if st.Command != "" && st.Command != st.Resolved {
					detail = fmt.Sprintf("%s → %s", st.Command, st.Resolved)
				}
			}
			line := fmt.Sprintf("  %s %-*s  %-6s %s",
				mcpStateGlyph(st.State), nameWidth, st.Name, st.Transport, detail)
			fmt.Println(strings.TrimRight(line, " "))
		}
		fmt.Println()
	}

	var parts []string
	s := r.Summary
	if s.Launchable > 0 {
		parts = append(parts, fmt.Sprintf("%d launchable", s.Launchable))
	}
	if s.Reachable > 0 {
		parts = append(parts, fmt.Sprintf("%d reachable", s.Reachable))
	}
	if s.Unresolved > 0 {
		parts = append(parts, fmt.Sprintf("%d unresolved", s.Unresolved))
	}
	if s.Unreachable > 0 {
		parts = append(parts, fmt.Sprintf("%d unreachable", s.Unreachable))
	}
	if s.Unknown > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", s.Unknown))
	}
	fmt.Println(strings.Join(parts, " · "))
	if note := mcpLimitsNote(r.Servers); note != "" {
		fmt.Println(note)
	}

	// A config file that FAILED to parse must be visible even when other files
	// supplied servers — otherwise the one run where a config broke looks like a
	// completely normal run, and the servers it should have contributed are simply
	// missing with no explanation. The full source list is reserved for the
	// nothing-found case (below), where "where did you look?" is the whole question.
	printMCPSourceErrors(r.Sources, home)
}

// mcpLimitsNote states the limit of the check so a "✓" cannot imply more than was
// verified. It is TRANSPORT-AWARE because the note's own truth is the point: an
// http-only report used to end "verifies each command resolves to an executable,
// not that the server responds" — where no command had been resolved and the
// server was the one thing that HAD responded. A note that over-claims in the
// opposite direction is no better than the ✓ it exists to qualify.
//
// The halves key off the states actually reached, not off the configured
// transports, because a state is a record of what was verified: an http entry that
// was never probed (no client) earns no http claim.
func mcpLimitsNote(statuses []mcpServerStatus) string {
	var resolvedCommand, probedURL bool
	for _, st := range statuses {
		switch st.State {
		case mcpStateLaunchable, mcpStateUnresolved:
			resolvedCommand = true
		case mcpStateReachable, mcpStateUnreachable:
			probedURL = true
		case mcpStateUnknown:
			// Named rather than defaulted: an unknown row is one where NOTHING was
			// checked, so it must contribute no claim to the note.
		}
	}
	var halves []string
	if resolvedCommand {
		halves = append(halves, "each command resolves to an executable, not that the server responds")
	}
	if probedURL {
		halves = append(halves, "each url answered, not that an MCP server is behind it")
	}
	if len(halves) == 0 {
		// Nothing was verified at all, so there is nothing to qualify.
		return ""
	}
	return "note: verifies " + strings.Join(halves, "; ")
}

// printMCPSourceErrors reports only the sources that could not be read.
func printMCPSourceErrors(sources []integration.MCPConfigSource, home string) {
	var broken []integration.MCPConfigSource
	for _, src := range sources {
		if src.Err != "" {
			broken = append(broken, src)
		}
	}
	if len(broken) == 0 {
		return
	}
	// "may be missing" rather than "are missing": a whole-file parse failure hides
	// everything in that file, but a failure confined to one projects[<key>] block
	// still lets the file's root servers through. Claiming they are all absent would
	// be wrong in the partial case, and the report cannot tell the user which case
	// they are in without repeating the parser's own distinction here.
	fmt.Printf("\n⚠ %d config file(s) could not be fully read — servers in them may be missing above:\n", len(broken))
	for _, src := range broken {
		fmt.Printf("  ✗ %s  (%s) — %s\n", shortenPath(src.Path, home), src.Label, src.Err)
	}
}

// printMCPSources lists the files discovery consulted, so an unexpected empty
// result is diagnosable without guessing where to look.
func printMCPSources(sources []integration.MCPConfigSource, home string) {
	if len(sources) == 0 {
		return
	}
	fmt.Println("\nLooked in:")
	for _, src := range sources {
		switch {
		case src.Err != "":
			fmt.Printf("  ✗ %s  (%s) — %s\n", shortenPath(src.Path, home), src.Label, src.Err)
		case src.Present:
			fmt.Printf("  · %s  (%s) — no servers\n", shortenPath(src.Path, home), src.Label)
		default:
			fmt.Printf("  · %s  (%s) — not found\n", shortenPath(src.Path, home), src.Label)
		}
	}
}

// displayHome resolves the home directory the paths in this report hang off. It is
// os.UserHomeDir deliberately: that is the same discovery
// integration.DefaultMCPDiscoveryOptions used to BUILD those paths, so the prefix
// being stripped is the prefix that was joined on.
func displayHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// displayPath shortens a home-relative path to ~/… against the current home. It is
// the one-shot form of shortenPath, for a caller with a single path to print.
func displayPath(path string) string {
	return shortenPath(path, displayHome())
}

// shortenPath shortens a path under home to ~/…, so the output stays readable and
// does not splash an absolute home directory across every line.
//
// It only ever SHORTENS, and that guard is not theoretical: with HOME=/ every
// absolute path is "under home", and the naive rewrite turned /var/folders/… into
// a longer `~/var/folders/…` that is not even a real path. The escape test is
// separator-aware too — a bare strings.HasPrefix(rel, "..") also rejects a
// legitimate sibling like `..config/x`.
func shortenPath(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return path
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	short := filepath.Join("~", rel)
	if len(short) >= len(path) {
		return path
	}
	return short
}
