package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file discovers the MCP servers a machine actually has configured.
//
// It exists because `adb mcp check` used to look ONLY at Claude Desktop's
// claude_desktop_config.json. That is the wrong file for a Claude Code user:
// their servers live in ~/.claude.json (and, per project, in a .mcp.json), so the
// command reported "no MCP servers configured" on a machine running five of them.
//
// Four sources are read, and each server remembers which one it came from — an
// answer like "which of my configs is this stale entry in?" is most of the value
// of the command.
//
// Everything discovered here is PRINTED to a terminal and MARSHALLED by
// `adb mcp check --json`, output which routinely gets pasted into an issue or a
// chat. So this layer, not the renderer, is where credentials are dropped: env
// values never reach the struct (MCPServer.EnvKeys), and the two fields that can
// carry a secret inline — a url's userinfo/query and a stdio entry's args — are
// redacted on the way in (RedactURL / RedactArgs). Filtering at the edges would
// mean every future consumer had to remember to do it.

// MCPTransport is how a client talks to an MCP server.
type MCPTransport string

const (
	// MCPTransportStdio is a server launched as a subprocess and spoken to over
	// stdin/stdout. This is the overwhelmingly common case and the only one
	// claude_desktop_config.json ever described.
	MCPTransportStdio MCPTransport = "stdio"
	// MCPTransportHTTP is a server reached over HTTP/SSE at a URL.
	MCPTransportHTTP MCPTransport = "http"
)

// MCPServer is one configured server, normalised across config formats.
type MCPServer struct {
	// Name is the key the config filed it under.
	Name string `json:"name"`
	// Source is the config file it came from, so a bad entry can be found again.
	Source string `json:"source"`
	// Scope is "global" for a user-level entry, or the project/workspace path an
	// entry is specific to (a projects[<path>] block, or a <workspace>/.mcp.json).
	Scope string `json:"scope,omitempty"`
	// Transport is stdio or http, inferred when the config does not say.
	Transport MCPTransport `json:"transport"`
	// Command launches a stdio server.
	Command string `json:"command,omitempty"`
	// Args are the stdio server's arguments, stored ALREADY REDACTED — see
	// RedactArgs. An MCP entry commonly authenticates with `--api-key sk-live-…`
	// or `--header "Authorization: Bearer …"`, and this field is emitted by
	// `--json`. Nothing in adb ever launches a configured server (`adb mcp check`
	// resolves the command, it never execs it), so the raw args have no consumer
	// and keeping them would only add a second place to leak a token.
	Args []string `json:"args,omitempty"`
	// URL is the RAW url of an http/sse server, kept verbatim because the health
	// probe has to dial exactly what was configured. It is `json:"-"` on purpose:
	// a url is the field most likely to carry a credential — basic-auth userinfo
	// (`https://svc:S3CR3T@host/mcp`) or a query parameter (`?api_key=sk-live-…`)
	// — so it must never be marshalled. Print URLDisplay instead.
	//
	// This mattered concretely: net/http's own url.Error already redacts the
	// password to `svc:***@`, and the report used to prefix the raw url onto the
	// same line, un-redacting what the stdlib was careful about.
	URL string `json:"-"`
	// URLDisplay is the redacted url, and is what any output — human or JSON —
	// should show. Empty when there is no url. See RedactURL for the rules.
	URLDisplay string `json:"url,omitempty"`
	// EnvKeys lists the names of env vars the entry sets — NAMES ONLY, never
	// values. An MCP env block routinely holds tokens, and this struct is printed
	// and marshalled to JSON, so the values must not be carried here at all.
	EnvKeys []string `json:"env_keys,omitempty"`
}

// mcpServerEntry is the on-disk shape of one server across the config formats.
// Claude Desktop and Claude Code agree on command/args/env; newer entries add an
// explicit type, and remote entries carry a url.
type mcpServerEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	URL     string            `json:"url"`
	Env     map[string]string `json:"env"`
}

// mcpConfigFile is the subset of a config file this cares about. Both
// ~/.claude.json and claude_desktop_config.json put servers under mcpServers;
// ~/.claude.json additionally scopes some per project.
type mcpConfigFile struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
	// Projects is held as RAW json and decoded one key at a time (see
	// projectMap/projectEntryServers) rather than as a typed map.
	//
	// ~/.claude.json is a large Claude-Code-owned file keyed by EVERY directory
	// the user has ever opened. As a typed map, ONE malformed entry anywhere in it
	// — say an `"mcpServers": []` under a project the user last touched a year ago
	// — failed the entire json.Unmarshal and dropped the file's perfectly valid
	// ROOT servers, reporting "No MCP servers configured." That is schema drift in
	// an unrelated project breaking discovery for this one. The file-level promise
	// ("one broken file should not hide the servers that are fine") has to hold
	// WITHIN a file too.
	Projects json.RawMessage `json:"projects"`
}

// projectMap decodes the projects object one level deep, leaving each entry raw.
// A `projects` value that is not an object at all is an error rather than a
// panic, and the caller keeps the file's root servers regardless.
func (c mcpConfigFile) projectMap() (map[string]json.RawMessage, error) {
	if len(c.Projects) == 0 {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(c.Projects, &m); err != nil {
		return nil, fmt.Errorf("not an object: %w", err)
	}
	return m, nil
}

// projectEntryServers decodes one projects[<key>] block's mcpServers.
func projectEntryServers(raw json.RawMessage) (map[string]mcpServerEntry, error) {
	var block struct {
		MCPServers map[string]mcpServerEntry `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, err
	}
	return block.MCPServers, nil
}

// MCPDiscoveryOptions locates the config files to read. Every path is explicit so
// a caller — notably a test — can point discovery somewhere other than a real
// home directory. DefaultMCPDiscoveryOptions fills in the real locations.
type MCPDiscoveryOptions struct {
	// HomeDir is the root the user-level config paths hang off.
	HomeDir string
	// WorkspaceDir is the project root searched for a .mcp.json.
	WorkspaceDir string
	// ProjectKey selects which projects[<key>] block of ~/.claude.json applies
	// (Claude Code keys these by absolute project path). Empty skips them. Both
	// the literal path and its symlink-resolved form are tried — see
	// candidateProjectKeys.
	ProjectKey string
}

// DefaultMCPDiscoveryOptions returns the real locations for this machine.
func DefaultMCPDiscoveryOptions(workspaceDir string) (MCPDiscoveryOptions, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return MCPDiscoveryOptions{}, fmt.Errorf("resolve home directory: %w", err)
	}
	return MCPDiscoveryOptions{
		HomeDir:      home,
		WorkspaceDir: workspaceDir,
		ProjectKey:   workspaceDir,
	}, nil
}

// MCPConfigSource is one config file discovery looked at.
type MCPConfigSource struct {
	// Label names the client whose config this is, for output.
	Label string `json:"label"`
	Path  string `json:"path"`
	// Present is whether the file EXISTS, taken from a Stat before the read is
	// attempted — not from a successful read. Deriving it from the read made a
	// chmod-000 config indistinguishable from an absent one on this field, so the
	// row said `present: false, error: "permission denied"`, which contradicts
	// itself and hides the one case whose fix is "chmod the file" rather than
	// "configure the server". An ABSENT source is reported rather than skipped:
	// "I looked here and found nothing" is the diagnostic a user needs when they
	// expected their servers to show up.
	Present bool `json:"present"`
	// Servers is how many entries it contributed after precedence.
	Servers int `json:"servers"`
	// Err is a parse/read failure. A broken config is surfaced, never silently
	// treated as empty. Several problems in one file are joined with "; " — a
	// malformed project block does not erase the note about the file itself.
	Err string `json:"error,omitempty"`
}

// noteErr records another problem with this source without discarding one that is
// already recorded.
func (s *MCPConfigSource) noteErr(msg string) {
	if s.Err == "" {
		s.Err = msg
		return
	}
	s.Err += "; " + msg
}

// mcpSourceSpec is a candidate config file plus how to interpret it.
type mcpSourceSpec struct {
	label string
	path  string
	// rootScope is the Scope given to entries in this file's ROOT mcpServers
	// block. It is "global" for the user-level configs; for
	// <workspace>/.mcp.json — the most project-specific file of the four — it is
	// the workspace path, because labelling those entries "global" contradicted
	// MCPServer.Scope's own contract ("global, or the project path").
	rootScope string
	// projectScoped reads the projects[<ProjectKey>] block as well as the root.
	projectScoped bool
}

// globalScope labels an entry that applies to the whole user, not one project.
const globalScope = "global"

// utf8BOM is the byte-order mark encoding/json rejects as an "invalid character".
// Spelled as an escape on purpose — as a literal it is an invisible edit hazard.
var utf8BOM = []byte("\uFEFF")

// candidateSources lists the config files to read, in PRECEDENCE order: the most
// specific config wins a name collision, mirroring how adb's own config tiers
// resolve (Repo > Org > Global).
func candidateSources(opts MCPDiscoveryOptions) []mcpSourceSpec {
	var out []mcpSourceSpec
	if opts.WorkspaceDir != "" {
		// Project-scoped, and the file `adb harness build` emits. Its entries are
		// scoped to the workspace, not global — this IS the project config.
		out = append(out, mcpSourceSpec{
			label:     "project .mcp.json",
			path:      filepath.Join(opts.WorkspaceDir, ".mcp.json"),
			rootScope: opts.WorkspaceDir,
		})
	}
	if opts.HomeDir != "" {
		out = append(out,
			// Claude Code: user-level, plus per-project blocks.
			mcpSourceSpec{
				label:         "Claude Code",
				path:          filepath.Join(opts.HomeDir, ".claude.json"),
				rootScope:     globalScope,
				projectScoped: true,
			},
			// Claude Desktop, both platform locations.
			mcpSourceSpec{
				label:     "Claude Desktop",
				path:      filepath.Join(opts.HomeDir, ".config", "Claude", "claude_desktop_config.json"),
				rootScope: globalScope,
			},
			mcpSourceSpec{
				label: "Claude Desktop",
				path: filepath.Join(opts.HomeDir, "Library", "Application Support",
					"Claude", "claude_desktop_config.json"),
				rootScope: globalScope,
			},
		)
	}
	return out
}

// candidateProjectKeys returns the projects[<key>] keys to try, most literal
// first.
//
// Claude Code keys its projects blocks by the RESOLVED absolute path, which on
// macOS routinely differs from the path adb is holding: /tmp is a symlink to
// /private/tmp, and a workspace reached through a symlinked ~/Code resolves
// somewhere else entirely. Using the key verbatim meant such a workspace matched
// nothing and its whole project block vanished — with no diagnostic, because
// "this key is not in the file" and "this project has no servers" are the same
// observation.
//
// Both forms are tried, so a config keyed either way is found. The literal key
// goes FIRST, so it wins a name collision and Scope reports the spelling the
// config actually used. EvalSymlinks failing (the path does not exist yet, or a
// permission error) simply means there is no second form to try.
func candidateProjectKeys(key string) []string {
	if key == "" {
		return nil
	}
	keys := []string{key}
	if resolved, err := filepath.EvalSymlinks(key); err == nil && resolved != key {
		keys = append(keys, resolved)
	}
	return keys
}

// DiscoverMCPServers reads every candidate config and returns the servers found,
// sorted by name, along with a report of every source it looked at.
//
// A name configured in more than one place is reported ONCE, from the most
// specific source (see candidateSources). Reading is best-effort per file: a
// malformed config records its error on the source and does not stop the others,
// because one broken file should not hide the servers that are fine. That
// best-effort rule applies WITHIN a file as well — a malformed projects[…] entry
// is noted without costing the file its root servers (see mcpConfigFile.Projects).
func DiscoverMCPServers(opts MCPDiscoveryOptions) ([]MCPServer, []MCPConfigSource) {
	var sources []MCPConfigSource
	byName := map[string]MCPServer{}
	projectKeys := candidateProjectKeys(opts.ProjectKey)

	for _, spec := range candidateSources(opts) {
		src := MCPConfigSource{Label: spec.label, Path: spec.path}

		// Presence is a property of the filesystem, not of the read succeeding.
		// See MCPConfigSource.Present.
		if _, err := os.Stat(spec.path); err == nil {
			src.Present = true
		}

		data, err := os.ReadFile(spec.path)
		if err != nil {
			if !os.IsNotExist(err) {
				src.Err = err.Error()
			}
			sources = append(sources, src)
			continue
		}

		var parsed mcpConfigFile
		// Strip a leading UTF-8 BOM. A .mcp.json is plausibly hand-written or
		// emitted by a Windows tool, and encoding/json rejects the BOM outright
		// (`parse: invalid character 'ï»¿'`), which reads like a corrupt config
		// rather than an encoding detail. core.parseArtifactFrontmatter already
		// tolerates one (L600 §8), so this is consistency, not novelty.
		if err := json.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &parsed); err != nil {
			src.Err = fmt.Sprintf("parse: %v", err)
			sources = append(sources, src)
			continue
		}

		// add claims names for one block. Because it SKIPS a name another block has
		// already taken, "more specific" means "added FIRST" — so the per-project
		// block goes in before the root block, not after. Getting this backwards is
		// an easy mistake: applying the project block second reads like it should
		// override, when in fact it loses every collision.
		add := func(entries map[string]mcpServerEntry, scope string) {
			names := make([]string, 0, len(entries))
			for name := range entries {
				names = append(names, name)
			}
			sort.Strings(names) // deterministic, so collisions resolve the same way twice
			for _, name := range names {
				if _, taken := byName[name]; taken {
					continue // a more specific source already claimed this name
				}
				byName[name] = normalizeEntry(name, spec.path, scope, entries[name])
				src.Servers++
			}
		}

		// The projects object is only looked at when a key could match it, so a
		// caller that passed no ProjectKey is never told about a shape it did not
		// consult.
		if spec.projectScoped && len(projectKeys) > 0 {
			projects, err := parsed.projectMap()
			if err != nil {
				src.noteErr(fmt.Sprintf("parse: projects: %v", err))
			}
			for _, key := range projectKeys {
				raw, ok := projects[key]
				if !ok {
					continue
				}
				entries, err := projectEntryServers(raw)
				if err != nil {
					// The MATCHING key is malformed: say so, and still fall through
					// to the root block below rather than losing the whole file.
					src.noteErr(fmt.Sprintf("parse: projects[%s]: %v", key, err))
					continue
				}
				add(entries, key)
			}
		}
		add(parsed.MCPServers, spec.rootScope)

		sources = append(sources, src)
	}

	out := make([]MCPServer, 0, len(byName))
	for _, s := range byName {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, sources
}

// normalizeEntry converts an on-disk entry into an MCPServer, inferring the
// transport when the config does not state one (older entries never did) and
// redacting the two fields that can carry an inline credential.
func normalizeEntry(name, source, scope string, e mcpServerEntry) MCPServer {
	s := MCPServer{
		Name:    name,
		Source:  source,
		Scope:   scope,
		Command: e.Command,
		// Redacted at the boundary — see MCPServer.Args / MCPServer.URLDisplay.
		Args:       RedactArgs(e.Args),
		URL:        e.URL,
		URLDisplay: RedactURL(e.URL),
	}
	// Only env KEYS are carried — see MCPServer.EnvKeys.
	for k := range e.Env {
		s.EnvKeys = append(s.EnvKeys, k)
	}
	sort.Strings(s.EnvKeys)

	switch {
	case e.Type == string(MCPTransportStdio):
		s.Transport = MCPTransportStdio
	case e.Type == "sse" || e.Type == "http" || e.Type == "streamable-http":
		s.Transport = MCPTransportHTTP
	case e.URL != "":
		// No declared type but a URL: it is reached over HTTP.
		s.Transport = MCPTransportHTTP
	default:
		// A command with no declared type is the historical stdio shape.
		s.Transport = MCPTransportStdio
	}
	return s
}

// ── Redaction ────────────────────────────────────────────────────────────────
//
// One place, two exported functions, so the rule is testable on its own and every
// caller gets the same answer. The bias throughout is REDACT WHEN IN DOUBT: a
// false redaction costs one unreadable value in a diagnostic, a missed one writes
// a live credential to a terminal and into `--json` output that gets pasted
// around.

// redactedValue is what replaces a secret. It matches what net/http's own
// url.Error prints (`svc:***@host`), so a url redacted here reads the same as one
// redacted by the stdlib in a failed probe's error.
const redactedValue = "***"

// secretNameHints are substrings that make a query-parameter or flag NAME look
// like it names a credential. Matching is case-insensitive and by SUBSTRING, so
// "x-api-key", "X-API-KEY" and "apikey" are all caught by "key", and
// "access_token" by "token".
//
// Substring matching is what makes the short list sufficient: "sig" also catches
// "signature", "auth" also catches "authorization". Three hints beyond the
// reviewed list are included deliberately — "credential", "cookie", "session" —
// because each names something that is a bearer secret in practice.
var secretNameHints = []string{
	"token", "key", "secret", "password", "passwd", "pwd",
	"auth", "sig", "credential", "cookie", "session",
}

// secretLongFlags are flags whose NAME carries no hint but whose VALUE is a
// credential, so no substring rule can catch them.
//
// `--header` is the important one: `--header "Authorization: Bearer <token>"` is
// how a remote MCP server is usually authenticated, and nothing in the word
// "header" suggests a secret.
var secretLongFlags = map[string]bool{
	"header":  true,
	"headers": true,
	"bearer":  true,
}

// secretShortFlags are matched CASE-SENSITIVELY, because the case is the whole
// signal for a short flag: curl's -H is a header and -u is user:password, while a
// lowercase -h is almost always help. -k was named explicitly in review as a
// common --api-key short form.
var secretShortFlags = map[string]bool{
	"k": true,
	"u": true,
	"H": true,
}

// isSecretName reports whether a parameter or flag name suggests a credential.
func isSecretName(name string) bool {
	lower := strings.ToLower(name)
	for _, hint := range secretNameHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// isSecretFlagName reports whether a flag name (already stripped of its leading
// dashes) introduces a credential value. Kept separate from isSecretFlag so the
// `--flag=value` form is judged by exactly the same rule as `--flag value` —
// checking only the substring hints there would have missed `--header=…`, whose
// whole point is that its name suggests nothing.
func isSecretFlagName(name string) bool {
	if secretShortFlags[name] {
		return true
	}
	if secretLongFlags[strings.ToLower(name)] {
		return true
	}
	return isSecretName(name)
}

// isSecretFlag reports whether the argument is a flag whose FOLLOWING argument is
// a credential.
func isSecretFlag(arg string) bool {
	if !strings.HasPrefix(arg, "-") {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if name == "" {
		return false // a bare "-" or "--" separator
	}
	return isSecretFlagName(name)
}

// RedactURL returns a url safe to print, with the credential part of any
// userinfo and any secret-looking query-parameter VALUE replaced by "***".
//
// It is exported because a url is the single field most likely to carry a
// credential — basic-auth userinfo (`https://svc:S3CR3T@host/mcp`), a bare token
// as the username (`https://ghp_…@host/mcp`), or a query parameter
// (`?api_key=sk-live-…`) — and the rule needs to live in exactly one testable
// place. MCPServer.URLDisplay is populated with it; MCPServer.URL keeps the raw
// value because the probe has to dial what was configured.
//
// Which part of a userinfo is the credential depends on its shape — see
// redactUserinfo, which owns that rule.
//
// Two guarantees:
//
//   - A url with no secrets comes back BYTE-IDENTICAL. That is why the result is
//     built by textual surgery on the input rather than by reassembling a
//     url.URL: url.URL.String() re-encodes as it goes (it would even
//     percent-escape the "***" it had just written into the userinfo).
//   - A url net/url refuses to parse degrades CONSERVATIVELY rather than being
//     echoed raw — see redactURLBlind. A malformed url is exactly the case where
//     printing the original "so the user can see what is wrong" prints the
//     credential.
func RedactURL(raw string) string {
	if raw == "" {
		return raw
	}
	// The parse decides only whether the string is well-formed enough to locate
	// its parts; it is never used to rebuild the output.
	if _, err := url.Parse(raw); err != nil {
		return redactURLBlind(raw)
	}
	head, frag := raw, ""
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		head, frag = raw[:i], raw[i:] // frag keeps its '#'
	}
	if i := strings.IndexByte(head, '?'); i >= 0 {
		return redactUserinfo(head[:i]) + "?" + redactQueryValues(head[i+1:]) + frag
	}
	return redactUserinfo(head) + frag
}

// redactURLBlind is the fallback for a url net/url will not parse. It cannot
// trust its own idea of where the parts are, so it is blunt: the whole query (or
// fragment) is dropped rather than filtered, and the userinfo credential is
// redacted textually. Keeping the scheme/host/path means the row is still
// diagnosable.
func redactURLBlind(raw string) string {
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		return redactUserinfo(raw[:i]) + raw[i:i+1] + redactedValue
	}
	return redactUserinfo(raw)
}

// redactQueryValues replaces the value of every secret-looking parameter. It
// works on the RAW query string so a parameter it leaves alone is not re-encoded.
func redactQueryValues(query string) string {
	if query == "" {
		return query
	}
	parts := strings.Split(query, "&")
	for i, part := range parts {
		name, value, found := strings.Cut(part, "=")
		if !found || value == "" {
			continue // a valueless flag has nothing to hide
		}
		// Match on the decoded name when it decodes, so "x%2Dapi%2Dkey" is caught
		// as well as "x-api-key".
		probe := name
		if decoded, err := url.QueryUnescape(name); err == nil {
			probe = decoded
		}
		if isSecretName(probe) {
			parts[i] = name + "=" + redactedValue
		}
	}
	return strings.Join(parts, "&")
}

// redactUserinfo removes credential material from a url's userinfo component.
// The input must already have had any query and fragment removed.
//
// The rule follows the SHAPE of the userinfo, because the two shapes put the
// credential in different slots:
//
//	https://svc:S3CR3T@host/mcp  → https://svc:***@host/mcp  (password only)
//	https://ghp_TOKEN@host/mcp   → https://***@host/mcp       (the whole userinfo)
//	https://@host/mcp            → unchanged, byte-identical
//
// Where net/http has a rule — a password exists — match it: that is the shape
// net/http's url.Error prints, and the username there is a login identity, so
// keeping it makes a redacted url recognisable and says which service account is
// configured. Where net/http's shape does not apply — no password — the username
// is the only slot a credential can be in, so it goes. A personal access token
// embedded as a bare username (`https://ghp_…@host`) is the dominant real-world
// spelling of a credential in a url, and this boundary's whole contract is that
// credentials are dropped on the way in. This mirrors the reasoning recorded on
// cloudsync.StripOriginCredentials, which drops the whole userinfo over http(s)
// for exactly that reason.
//
// An EMPTY userinfo is left byte-identical: there is no credential in
// `https://@host` to remove, and writing "***@" would invent one. Inventing a
// secret that was never configured misleads a reader the same way echoing a real
// one does — the same reasoning that leaves a valueless query parameter alone.
func redactUserinfo(s string) string {
	start := -1
	switch {
	case strings.HasPrefix(s, "//"):
		start = 2 // a scheme-relative url
	default:
		if i := strings.Index(s, "://"); i >= 0 {
			start = i + 3
		}
	}
	if start < 0 {
		return s // no authority component, so no userinfo
	}
	end := len(s)
	if i := strings.IndexByte(s[start:], '/'); i >= 0 {
		end = start + i
	}
	authority := s[start:end]
	// The LAST '@' separates userinfo from host: an unencoded '@' inside a
	// password is malformed but should still leave the host intact.
	at := strings.LastIndexByte(authority, '@')
	if at < 0 {
		return s
	}
	userinfo := authority[:at]
	colon := strings.IndexByte(userinfo, ':')
	if colon < 0 {
		if userinfo == "" {
			return s // no credential to remove, and "***@" would invent one
		}
		// No password slot, so the username IS the credential.
		return s[:start] + redactedValue + authority[at:] + s[end:]
	}
	return s[:start] + userinfo[:colon+1] + redactedValue + authority[at:] + s[end:]
}

// RedactArgs returns a copy of a stdio entry's arguments with credential values
// replaced by "***". A nil slice stays nil, so `json:"args,omitempty"` behaves
// exactly as before for an entry that declares no args.
//
// Four shapes are handled:
//
//   - `--api-key sk-live-…` — a flag whose NAME suggests a secret redacts the
//     argument that FOLLOWS it;
//   - `--header "Authorization: Bearer …"`, `-k`, `-u`, `-H` — flags whose name
//     suggests nothing but whose value is a credential (secretLongFlags /
//     secretShortFlags);
//   - `--token=…` and `API_KEY=…` — the joined form, judged on the part before
//     the '=' with any leading dashes stripped;
//   - any argument that contains "://" is passed through RedactURL, because a
//     url on the command line carries userinfo and query parameters exactly like
//     a configured one does.
func RedactArgs(args []string) []string {
	if args == nil {
		return nil
	}
	out := make([]string, len(args))
	redactNext := false
	for i, arg := range args {
		switch {
		case redactNext:
			out[i] = redactedValue
			redactNext = false
		case strings.Contains(arg, "://"):
			// Checked BEFORE the `name=value` case: a url's own query contains an
			// '=' too, so splitting on the first one would treat the whole url as a
			// flag name — and then redact only its last parameter while echoing the
			// userinfo credential verbatim.
			out[i] = redactArgURL(arg)
		case strings.Contains(arg, "="):
			name, value, _ := strings.Cut(arg, "=")
			switch {
			case value == "":
				out[i] = arg // nothing to hide
			case isSecretFlagName(strings.TrimLeft(name, "-")):
				out[i] = name + "=" + redactedValue
			default:
				out[i] = arg
			}
		case isSecretFlag(arg):
			// The flag itself is not a secret and stays readable; its value goes.
			out[i] = arg
			redactNext = true
		default:
			// redactArgURL is total — it returns an argument that carries no url
			// unchanged — so an ordinary argument passes through here untouched.
			out[i] = redactArgURL(arg)
		}
	}
	return out
}

// redactArgURL redacts an argument that contains a url, keeping any `--flag=`
// prefix readable. The prefix is split off on the text BEFORE the scheme, so a
// url's own `?api_key=` cannot be mistaken for the flag-name/value boundary.
func redactArgURL(arg string) string {
	scheme := strings.Index(arg, "://")
	if scheme < 0 {
		return arg
	}
	if eq := strings.IndexByte(arg[:scheme], '='); eq >= 0 {
		name, value := arg[:eq], arg[eq+1:]
		if isSecretFlagName(strings.TrimLeft(name, "-")) {
			// e.g. `--auth-url=https://…` — the whole value is the credential.
			return name + "=" + redactedValue
		}
		return name + "=" + RedactURL(value)
	}
	return RedactURL(arg)
}
