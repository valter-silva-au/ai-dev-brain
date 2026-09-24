package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// This is the HTTP half of `adb mcp check`: a cached GET probe of an http/sse MCP
// server's url.
//
// It answers one narrow question — "did anything answer at this url, and with
// what?" — and the caller decides what that means. Two things it deliberately
// does NOT do, both learned from wrong verdicts in both directions:
//
//   - It does not follow redirects. A default http.Client does, so a url that
//     302s to an SSO login page returning 200 was reported as a reachable MCP
//     server. That is the surviving half of the old "anything that answers on
//     localhost:3000 is healthy" bug. With redirects off, a 3xx is reported as
//     itself and a caller can say "reachable, but it redirected" — and, through
//     Location, say WHERE to, which is what tells "your identity provider answered"
//     apart from "you mistyped the path". Not following the redirect is precisely
//     what makes that destination trustworthy: it is read off the redirector's own
//     response headers, and is never itself dialed.
//   - It does not treat a non-2xx status as "no server". A healthy
//     Streamable-HTTP MCP endpoint answers a bare GET with 405, correctly, since
//     the request carries no `Accept: text/event-stream`. So StatusCode — not
//     Healthy — is the field that tells you whether something answered.

const (
	// DefaultMCPProbeTimeout bounds one probe. It is short on purpose: probes are
	// serial, so three unroutable servers at the old 10s each meant a 30s wait for
	// a command a user runs to get a quick answer. A real MCP endpoint on a LAN or
	// over the internet answers a GET well inside 5s; anything slower is
	// indistinguishable from down for this diagnostic's purposes. Use
	// NewMCPClientWithTimeout to widen it.
	DefaultMCPProbeTimeout = 5 * time.Second
	// defaultMCPCacheTTL is used when a caller passes a non-positive TTL.
	defaultMCPCacheTTL = 30 * time.Second
)

// MCPHealthCheck represents a cached health check result
type MCPHealthCheck struct {
	ServerURL string
	// Healthy is a coarse legacy signal: 2xx or 3xx. Prefer StatusCode — a 405
	// from a correct Streamable-HTTP endpoint is not unhealthy, and a 302 to a
	// login page is not healthy. Kept as-is for compatibility with existing
	// callers and tests.
	Healthy bool
	// StatusCode is the status of the response, and is populated for EVERY
	// response — 2xx, 3xx (redirects are not followed), 4xx and 5xx alike. It is
	// therefore the reliable discriminator: a non-zero StatusCode means an HTTP
	// server answered, and a zero StatusCode with a non-nil Error means the
	// transport never got a reply.
	StatusCode int
	// Location is where a redirect pointed: the response's `Location` header,
	// populated ONLY for a 3xx. Every other status class leaves it empty, because
	// there is no destination to report — and because a stray `Location` on a 200
	// is not a redirect and must not read as one.
	//
	// It is the single most useful fact about a redirect. "It redirected to your
	// identity provider" and "it redirected to a path you mistyped" are different
	// problems with different fixes, and the status code alone separates neither;
	// the probe holds the answer in the response headers, so not carrying it here
	// was simply losing it.
	//
	// It is REDACTED on the way in, by RedactLocation. That is not defensive
	// tidiness: the destination of an authentication redirect routinely carries a
	// `redirect_uri=`, `state=` or `code=` parameter, and this value is printed to
	// a terminal and marshalled by `adb mcp check --json`. This package's rule is
	// that credentials are dropped at the boundary they enter, never filtered at
	// the renderer (see the header comment in mcpconfig.go), so that no consumer
	// can leak a value it was never handed — and there is deliberately no raw
	// spelling of this field for a consumer to reach for instead.
	//
	// A RELATIVE destination (`Location: /login`, entirely legal — RFC 9110
	// §10.2.2) is reported VERBATIM rather than resolved against the request url.
	// Three reasons, in order of weight:
	//
	//   - It is what the origin actually said. Resolving would print a url the
	//     server never sent, which is the one thing a diagnostic field must not do.
	//   - It reads correctly where it is consumed. `adb mcp check` prints the
	//     destination on the same line as the url that was probed, so `/login`
	//     composes for the reader without inventing anything; resolving would
	//     instead repeat the host twice on one line.
	//   - Resolution is a failure mode for no gain. It means re-parsing the request
	//     url, which can fail, and a wrong base would silently produce a confidently
	//     wrong absolute url. The interesting case — a redirect off-host to an SSO
	//     login page — is absolute by necessity and passes through unchanged.
	Location  string
	CheckedAt time.Time
	Error     error
}

// MCPClient checks MCP server health with caching
type MCPClient interface {
	// CheckHealth performs an HTTP GET health check on an MCP server
	// Returns cached result if within TTL, otherwise performs new check
	CheckHealth(serverURL string) (MCPHealthCheck, error)

	// ClearCache clears all cached health check results
	ClearCache()

	// SetTTL sets the cache TTL duration
	SetTTL(ttl time.Duration)
}

// DefaultMCPClient implements MCPClient with TTL caching
type DefaultMCPClient struct {
	httpClient *http.Client
	cache      map[string]MCPHealthCheck
	cacheMux   sync.RWMutex
	ttl        time.Duration
}

// NewMCPClient creates a new MCP client with the specified TTL and the default
// per-request timeout (DefaultMCPProbeTimeout).
func NewMCPClient(ttl time.Duration) MCPClient {
	return NewMCPClientWithTimeout(ttl, DefaultMCPProbeTimeout)
}

// NewMCPClientWithTimeout creates a client with an explicit per-request timeout,
// for a caller that knowingly wants to wait longer (or, in a test, far less) than
// the diagnostic default. A non-positive value of either duration falls back to
// its default.
func NewMCPClientWithTimeout(ttl, timeout time.Duration) MCPClient {
	if ttl <= 0 {
		ttl = defaultMCPCacheTTL
	}
	if timeout <= 0 {
		timeout = DefaultMCPProbeTimeout
	}

	return &DefaultMCPClient{
		httpClient: &http.Client{
			Timeout: timeout,
			// Report a redirect as itself instead of following it. See the file
			// comment: following it turned "this url sends you to an SSO login
			// page" into "this is a reachable MCP server".
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cache: make(map[string]MCPHealthCheck),
		ttl:   ttl,
	}
}

// CheckHealth performs an HTTP GET health check on an MCP server
func (c *DefaultMCPClient) CheckHealth(serverURL string) (MCPHealthCheck, error) {
	if serverURL == "" {
		return MCPHealthCheck{}, fmt.Errorf("serverURL cannot be empty")
	}

	// Check cache first
	c.cacheMux.RLock()
	cached, exists := c.cache[serverURL]
	c.cacheMux.RUnlock()

	// Return cached result if within TTL
	if exists && time.Since(cached.CheckedAt) < c.ttl {
		return cached, nil
	}

	// Perform new health check
	result := MCPHealthCheck{
		ServerURL: serverURL,
		CheckedAt: time.Now(),
	}

	resp, err := c.httpClient.Get(serverURL)
	if err != nil {
		// A transport failure: nothing answered, so StatusCode stays 0.
		result.Healthy = false
		result.Error = fmt.Errorf("failed to connect to MCP server: %w", err)
	} else {
		defer resp.Body.Close()
		// Recorded before any verdict, and for every status class, so a caller can
		// distinguish "answered 405" from "never answered".
		result.StatusCode = resp.StatusCode
		// Only a redirect has a destination, so only a redirect records one. Note
		// this does NOT touch Healthy: a 3xx stays healthy (2xx||3xx) and StatusCode
		// stays the field callers are told to read — Location adds a fact about the
		// response, it does not restate the verdict.
		if isRedirectStatus(resp.StatusCode) {
			// Redacted here, at the boundary, and never stored raw. Header.Get
			// returns "" for a 3xx that carried no Location — malformed, but a real
			// server can emit one — and RedactLocation passes that through, so the
			// field is simply empty and the caller renders the destination-less form.
			result.Location = RedactLocation(resp.Header.Get("Location"))
		}
		// Consider 2xx and 3xx status codes as healthy
		result.Healthy = resp.StatusCode >= 200 && resp.StatusCode < 400
		if !result.Healthy {
			result.Error = fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
		}
	}

	// Update cache
	c.cacheMux.Lock()
	c.cache[serverURL] = result
	c.cacheMux.Unlock()

	return result, result.Error
}

// isRedirectStatus reports whether a status carries a redirect's semantics, and so
// whether a `Location` on it is a destination rather than an incidental header.
// Named rather than inlined because the same 300..399 range decides two things —
// which responses record a Location here, and which ones the CLI annotates as a
// redirect — and a range spelled twice is a range that eventually disagrees.
func isRedirectStatus(status int) bool {
	return status >= 300 && status < 400
}

// redirectSecretParams are query-parameter names whose VALUES are credentials when
// they appear in a redirect destination. Matched by EXACT (case-insensitive) name,
// unlike RedactURL's substring hints.
//
// These are the OAuth/OIDC exchange parameters. An authorization `code` is a
// single-use bearer credential — enough, with a public client, to mint a token —
// and `state`/`nonce` are the CSRF/replay bindings for that exchange. None of them
// contains any of RedactURL's hints ("token", "key", "secret", "auth", …), so
// RedactURL alone passes `?code=SECRET&state=XYZ` through byte-identically, which
// is precisely the shape a login redirect's Location has.
var redirectSecretParams = map[string]bool{
	"code":  true,
	"state": true,
	"nonce": true,
}

// RedactLocation returns a redirect destination safe to print: RedactURL, plus the
// exact-name pass over redirectSecretParams.
//
// Why the extra rule lives HERE rather than in RedactURL's secretNameHints: those
// hints are SUBSTRING matches applied to every url adb prints, so adding "code" or
// "state" there would redact a `?country_code=` or `?state=NSW` in a url that has
// no credential in it at all — degrading the general redactor to buy a rule that is
// only unambiguous in this one position. A `code=` on the destination of a 3xx from
// an authentication endpoint is an authorization code; a `code=` anywhere else is
// anybody's guess. If a second caller ever needs the same rule, that is the moment
// to move it next to RedactURL — not to copy it.
//
// It is exported for the same reason RedactURL is: the value it produces is what
// reaches a terminal and `--json`, so the rule needs one testable home, and a test
// building a MCPHealthCheck fixture must be able to spell a redacted destination
// exactly as the probe would.
func RedactLocation(location string) string {
	return redactRedirectParams(RedactURL(location))
}

// redactRedirectParams replaces the value of every redirectSecretParams parameter.
// Like redactQueryValues it works on the RAW query string, so a parameter it leaves
// alone comes back byte-identical rather than re-encoded — the same guarantee
// RedactURL makes, and the reason a secret-free destination is unchanged by this
// whole path.
func redactRedirectParams(location string) string {
	head, frag := location, ""
	if i := strings.IndexByte(location, '#'); i >= 0 {
		head, frag = location[:i], location[i:] // frag keeps its '#'
	}
	i := strings.IndexByte(head, '?')
	if i < 0 {
		return location // no query, nothing this pass can act on
	}
	path, query := head[:i], head[i+1:]

	parts := strings.Split(query, "&")
	for j, part := range parts {
		name, value, found := strings.Cut(part, "=")
		if !found || value == "" {
			continue // a valueless flag has nothing to hide
		}
		// Match on the decoded name when it decodes, so an escaped spelling is
		// caught too (the same courtesy redactQueryValues extends).
		probe := name
		if decoded, err := url.QueryUnescape(name); err == nil {
			probe = decoded
		}
		if redirectSecretParams[strings.ToLower(probe)] {
			parts[j] = name + "=" + redactedValue
		}
	}
	return path + "?" + strings.Join(parts, "&") + frag
}

// ClearCache clears all cached health check results
func (c *DefaultMCPClient) ClearCache() {
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	c.cache = make(map[string]MCPHealthCheck)
}

// SetTTL sets the cache TTL duration
func (c *DefaultMCPClient) SetTTL(ttl time.Duration) {
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()
	if ttl > 0 {
		c.ttl = ttl
	}
}
