package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewMCPClient(t *testing.T) {
	// Test with positive TTL
	client := NewMCPClient(30 * time.Second)
	if client == nil {
		t.Error("expected non-nil client")
	}

	// Test with zero TTL (should use default)
	client = NewMCPClient(0)
	if client == nil {
		t.Error("expected non-nil client with default TTL")
	}

	// Test with negative TTL (should use default)
	client = NewMCPClient(-1 * time.Second)
	if client == nil {
		t.Error("expected non-nil client with default TTL")
	}
}

func TestCheckHealth(t *testing.T) {
	// Test empty URL
	client := NewMCPClient(30 * time.Second)
	_, err := client.CheckHealth("")
	if err == nil {
		t.Error("expected error for empty URL")
	}

	// Test successful health check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	result, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !result.Healthy {
		t.Error("expected healthy result")
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status code 200, got %d", result.StatusCode)
	}
	if result.ServerURL != server.URL {
		t.Errorf("expected server URL %s, got %s", server.URL, result.ServerURL)
	}

	// Test unhealthy status code
	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	result, err = client.CheckHealth(unhealthyServer.URL)
	if err == nil {
		t.Error("expected error for unhealthy status")
	}
	if result.Healthy {
		t.Error("expected unhealthy result")
	}
	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status code 500, got %d", result.StatusCode)
	}

	// Test 3xx redirect (should be considered healthy)
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer redirectServer.Close()

	result, err = client.CheckHealth(redirectServer.URL)
	if err != nil {
		t.Errorf("unexpected error for redirect: %v", err)
	}
	if !result.Healthy {
		t.Error("expected healthy result for 3xx status")
	}
}

func TestCheckHealthCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewMCPClient(1 * time.Second)

	// First call - should hit the server
	result1, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call - should use cache
	result2, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call (cached), got %d", callCount)
	}

	// Results should be the same
	if result1.ServerURL != result2.ServerURL {
		t.Error("cached result differs from original")
	}

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Third call - cache expired, should hit server again
	_, err = client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache expiry, got %d", callCount)
	}
}

func TestClearCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewMCPClient(10 * time.Second)

	// First call
	_, err := client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Clear cache
	client.ClearCache()

	// Second call - should hit server again after cache clear
	_, err = client.CheckHealth(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache clear, got %d", callCount)
	}
}

func TestSetTTL(t *testing.T) {
	client := NewMCPClient(1 * time.Second)

	// Set new TTL
	client.SetTTL(5 * time.Second)

	// Test that invalid TTL is ignored
	client.SetTTL(0)
	client.SetTTL(-1 * time.Second)

	// If we get here without panic, the test passes
}

func TestCheckHealthInvalidURL(t *testing.T) {
	client := NewMCPClient(30 * time.Second)

	// Test invalid URL
	result, err := client.CheckHealth("http://invalid-host-that-does-not-exist.local:9999")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
	if result.Healthy {
		t.Error("expected unhealthy result for invalid URL")
	}
	if result.Error == nil {
		t.Error("expected error to be stored in result")
	}
}

// The tests below pin the probe's DATA contract, after a review found the verdict
// wrong in both directions:
//
//   - a healthy Streamable-HTTP MCP endpoint answers a bare GET with 405 (correct
//     per spec — the request carries no `Accept: text/event-stream`) and was
//     reported unreachable;
//   - a url that 302s to an SSO login page returning 200 was reported reachable,
//     because a default http.Client FOLLOWS redirects. That was the surviving
//     half of the old "anything that answers is healthy" bug.
//
// The fix here is deliberately about the data, not the policy: redirects are no
// longer followed, and StatusCode is populated for every response so a caller can
// distinguish "answered, with this status" from "nothing answered". Healthy stays
// 2xx||3xx for compatibility — TestCheckHealth above asserts a 3xx is healthy —
// and is documented as the coarse legacy signal it is.

// TestCheckHealthDoesNotFollowRedirects is the load-bearing one: a 3xx must be
// reported as ITSELF, not as whatever it points at. Following the redirect is how
// a login page came to be reported as a reachable MCP server.
func TestCheckHealthDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	client := NewMCPClient(30 * time.Second)
	result, _ := client.CheckHealth(redirector.URL)

	if result.StatusCode != http.StatusFound {
		t.Errorf("StatusCode = %d, want %d — the redirect must be reported as itself, not "+
			"as the status of wherever it leads (that is how an SSO login page reads as a "+
			"reachable MCP server)", result.StatusCode, http.StatusFound)
	}
	if targetHits != 0 {
		t.Errorf("the redirect target was requested %d time(s), want 0", targetHits)
	}
	// The destination is reported from the redirector's OWN response headers. That
	// pairing is the whole property: the caller learns where the url points WITHOUT
	// the probe going there, so nothing about the target — its status, its latency,
	// its existence — can be mistaken for a fact about the configured server.
	if result.Location != target.URL {
		t.Errorf("Location = %q, want %q — the 3xx names where it went, read off the "+
			"redirector's headers rather than by following it", result.Location, target.URL)
	}
}

// TestCheckHealthRedirectLocation is the destination contract, dimension by
// dimension: which status classes record one, what a missing header does, how a
// relative destination is reported, and that a credential-bearing destination is
// redacted BEFORE it is stored.
//
// The last of those is the one worth stating plainly. A redirect to an
// authentication endpoint routinely carries `?code=…&state=…` — an authorization
// code is a single-use bearer credential — and this value is printed to a terminal
// and marshalled by `adb mcp check --json`. So it is redacted at the boundary it
// enters (RedactLocation), exactly like MCPServer.URLDisplay, rather than at each
// renderer that might remember to.
func TestCheckHealthRedirectLocation(t *testing.T) {
	t.Parallel()

	const (
		absolute  = "https://idp.example.invalid/login?redirect_uri=https%3A%2F%2Fmcp.example.invalid%2Fmcp"
		oauthRaw  = "https://idp.example.invalid/authorize?code=SECRETCODE&state=XYZSTATE&client_id=adb"
		oauthWant = "https://idp.example.invalid/authorize?code=***&state=***&client_id=adb"
	)

	// Field order is name / location / wantLocation / wantAbsent / status /
	// sendLocation — pointer-bearing fields first, which is what govet's
	// fieldalignment wants, and readable anyway because every case name already
	// states its status.
	tests := []struct {
		name     string
		location string
		// wantLocation is the value the probe must STORE, i.e. already redacted.
		wantLocation string
		// wantAbsent are substrings that must NOT survive into the stored value.
		wantAbsent []string
		status     int
		// sendLocation distinguishes "no Location header at all" from "a Location
		// header whose value happens to be empty" — only the former is what a
		// malformed 3xx looks like in the wild.
		sendLocation bool
	}{
		// Every redirect status, because the range — not one favourite code — is what
		// the implementation keys off. 307/308 matter in their own right: they are the
		// method-preserving pair a modern gateway emits.
		{"301 moved permanently", absolute, absolute, nil, http.StatusMovedPermanently, true},
		{"302 found", absolute, absolute, nil, http.StatusFound, true},
		{"303 see other", absolute, absolute, nil, http.StatusSeeOther, true},
		{"307 temporary redirect", absolute, absolute, nil, http.StatusTemporaryRedirect, true},
		{"308 permanent redirect", absolute, absolute, nil, http.StatusPermanentRedirect, true},

		// A Location outside 3xx is not a redirect and must not read as one. 405 is
		// the one that matters most: it is what a CORRECT Streamable-HTTP endpoint
		// answers a bare GET with, so a stray header there must not make a working
		// server look like it bounced the reader somewhere.
		{"a 200 records no destination even when the header is present",
			absolute, "", nil, http.StatusOK, true},
		{"a 405 records no destination even when the header is present",
			absolute, "", nil, http.StatusMethodNotAllowed, true},
		{"a 500 records no destination even when the header is present",
			absolute, "", nil, http.StatusInternalServerError, true},

		// Malformed but real: the caller must render the destination-less form rather
		// than an empty arrow, so the field has to be reliably empty.
		{"a 3xx with no Location header at all is empty",
			"", "", nil, http.StatusFound, false},
		{"a 3xx with an empty Location header is empty",
			"", "", nil, http.StatusFound, true},

		// The documented relative-destination decision: reported VERBATIM, not
		// resolved against the request url. `adb mcp check` prints it beside the url
		// it probed, so `/login` composes for the reader — and resolving would print
		// a url the origin never sent.
		{"a relative destination is reported verbatim, not resolved",
			"/login", "/login", nil, http.StatusFound, true},
		{"a relative destination keeps its query, redacted",
			"/callback?code=SECRETCODE", "/callback?code=***",
			[]string{"SECRETCODE"}, http.StatusFound, true},

		// The redaction boundary.
		{"an oauth destination is redacted before it is stored",
			oauthRaw, oauthWant,
			[]string{"SECRETCODE", "XYZSTATE"}, http.StatusFound, true},
		{"a destination's userinfo password is redacted too",
			"https://svc:S3CR3T@idp.example.invalid/login",
			"https://svc:***@idp.example.invalid/login",
			[]string{"S3CR3T"}, http.StatusFound, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.sendLocation {
					w.Header().Set("Location", tc.location)
				}
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			client := NewMCPClient(30 * time.Second)
			result, _ := client.CheckHealth(server.URL)

			if result.Location != tc.wantLocation {
				t.Errorf("Location = %q, want %q", result.Location, tc.wantLocation)
			}
			for _, secret := range tc.wantAbsent {
				if strings.Contains(result.Location, secret) {
					t.Errorf("Location %q kept the secret %q — a destination is redacted on "+
						"the way IN, because every consumer of this field prints it",
						result.Location, secret)
				}
			}
			// Location must not have moved the existing verdict. StatusCode stays the
			// discriminator callers are told to read, and Healthy stays 2xx||3xx — a
			// 3xx is still "healthy" here however alarming its destination is.
			if result.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", result.StatusCode, tc.status)
			}
			wantHealthy := tc.status >= 200 && tc.status < 400
			if result.Healthy != wantHealthy {
				t.Errorf("Healthy = %v, want %v", result.Healthy, wantHealthy)
			}
		})
	}
}

// TestRedactLocation covers the rule itself, including the shapes a live server is
// unlikely to hand over on a good day but which decide whether the pass is safe: a
// fragment, an escaped parameter name, a valueless flag, and a url net/url will not
// parse (which RedactURL degrades on rather than echoing raw).
//
// It also pins the byte-identical guarantee. A destination with nothing to hide must
// come back unchanged, because the value is a diagnostic: a reader comparing it
// against what they configured cannot do that against a re-encoded spelling.
func TestRedactLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty stays empty — a 3xx with no Location header",
			in:   "", want: "",
		},
		{
			name: "no query is byte-identical",
			in:   "https://idp.example.invalid/login", want: "https://idp.example.invalid/login",
		},
		{
			name: "a relative path is byte-identical",
			in:   "/login", want: "/login",
		},
		{
			name: "an innocent query is byte-identical, not re-encoded",
			in:   "https://idp.example.invalid/login?next=%2Fmcp&x=1",
			want: "https://idp.example.invalid/login?next=%2Fmcp&x=1",
		},
		{
			name: "an oauth code and state are redacted; the rest is untouched",
			in:   "https://idp.example.invalid/cb?code=SECRET&state=XYZ&client_id=adb",
			want: "https://idp.example.invalid/cb?code=***&state=***&client_id=adb",
		},
		{
			name: "a nonce is redacted",
			in:   "https://idp.example.invalid/cb?nonce=N0NC3",
			want: "https://idp.example.invalid/cb?nonce=***",
		},
		{
			// Matching on the DECODED name, so an escaped spelling cannot slip a
			// credential past by writing it differently.
			name: "an escaped parameter name is still matched",
			in:   "https://idp.example.invalid/cb?co%64e=SECRET",
			want: "https://idp.example.invalid/cb?co%64e=***",
		},
		{
			name: "matching is case-insensitive",
			in:   "https://idp.example.invalid/cb?CODE=SECRET",
			want: "https://idp.example.invalid/cb?CODE=***",
		},
		{
			// A valueless flag has nothing to hide, and blanking it would invent a
			// value the server never sent.
			name: "a valueless parameter is left alone",
			in:   "https://idp.example.invalid/cb?code&x=1",
			want: "https://idp.example.invalid/cb?code&x=1",
		},
		{
			name: "a fragment survives, and the query before it is still redacted",
			in:   "https://idp.example.invalid/cb?code=SECRET#done",
			want: "https://idp.example.invalid/cb?code=***#done",
		},
		{
			// RedactURL's own responsibilities, asserted here too so composing the two
			// cannot silently drop one: userinfo, and its secret-name query hints.
			name: "a userinfo password is redacted by the RedactURL half",
			in:   "https://svc:S3CR3T@idp.example.invalid/login",
			want: "https://svc:***@idp.example.invalid/login",
		},
		{
			// The other half of the same rule, asserted here because a redirect
			// destination is exactly where a bare-token userinfo shows up: an IdP
			// url pasted into a config with a PAT as the username and no password.
			// The token below embeds S3CR3T so the per-case secret sweep below
			// covers it without a second entry in that list.
			name: "a bare-username token is redacted by the RedactURL half",
			in:   "https://ghp_S3CR3T@idp.example.invalid/login",
			want: "https://***@idp.example.invalid/login",
		},
		{
			name: "an api_key is redacted by the RedactURL half",
			in:   "https://idp.example.invalid/login?api_key=sk-live-1&code=SECRET",
			want: "https://idp.example.invalid/login?api_key=***&code=***",
		},
		{
			// RedactURL cannot locate the parts of a url net/url refuses, so it drops
			// the whole query rather than filtering it. The result has no `k=v` pairs
			// left for this pass to act on, and must not be corrupted by it either.
			name: "an unparseable url degrades conservatively",
			in:   "http://%zz/login?code=SECRET",
			want: "http://%zz/login?***",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RedactLocation(tc.in)
			if got != tc.want {
				t.Errorf("RedactLocation(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for _, secret := range []string{"SECRET", "XYZ", "N0NC3", "S3CR3T", "sk-live-1"} {
				if strings.Contains(got, secret) {
					t.Errorf("RedactLocation(%q) = %q kept the secret %q", tc.in, got, secret)
				}
			}
		})
	}
}

// TestCheckHealthCachedResultCarriesLocation pins that the destination survives the
// TTL cache. It is part of the RESULT, not a side-channel computed at render time,
// so a cached hit has to carry it — otherwise `adb mcp check` would report a
// destination for a url probed once and nothing for the same url served from cache,
// which is the shape of bug that makes a diagnostic untrustworthy.
func TestCheckHealthCachedResultCarriesLocation(t *testing.T) {
	t.Parallel()

	const dest = "https://idp.example.invalid/login?code=SECRETCODE"
	const wantDest = "https://idp.example.invalid/login?code=***"

	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.Header().Set("Location", dest)
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	client := NewMCPClient(time.Minute)

	first, _ := client.CheckHealth(server.URL)
	if first.Location != wantDest {
		t.Fatalf("first probe Location = %q, want %q", first.Location, wantDest)
	}

	second, _ := client.CheckHealth(server.URL)
	mu.Lock()
	got := hits
	mu.Unlock()
	if got != 1 {
		t.Fatalf("server was hit %d time(s), want 1 — the second call must be the CACHED "+
			"path, or this test is not asserting anything about the cache", got)
	}
	if second.Location != wantDest {
		t.Errorf("cached Location = %q, want %q — the destination is part of the cached "+
			"result, not something recomputed per call", second.Location, wantDest)
	}
	if second.StatusCode != first.StatusCode || second.Healthy != first.Healthy {
		t.Errorf("cached result diverged from the original: %+v vs %+v", second, first)
	}
}

// TestCheckHealthStatusCodeForEveryResponse pins that StatusCode is populated for
// every status class, which is what lets a caller reinterpret the result: any HTTP
// response at all means something answered (StatusCode != 0), while a transport
// failure leaves it 0. 405 is the row that matters most — a correct
// Streamable-HTTP endpoint answers a bare GET that way.
func TestCheckHealthStatusCodeForEveryResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		wantHealthy bool
		// wantErr is whether CheckHealth returns a non-nil error for this status.
		wantErr bool
	}{
		{"200 ok", http.StatusOK, true, false},
		{"204 no content", http.StatusNoContent, true, false},
		{"301 moved", http.StatusMovedPermanently, true, false},
		{"302 found", http.StatusFound, true, false},
		{"401 unauthorized", http.StatusUnauthorized, false, true},
		{"403 forbidden", http.StatusForbidden, false, true},
		// A HEALTHY Streamable-HTTP MCP server answers a bare GET like this.
		{"405 method not allowed", http.StatusMethodNotAllowed, false, true},
		{"500 server error", http.StatusInternalServerError, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			client := NewMCPClient(30 * time.Second)
			result, err := client.CheckHealth(server.URL)

			if result.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d — every response must record its status, "+
					"or a caller cannot tell 'answered' from 'never answered'",
					result.StatusCode, tc.status)
			}
			if result.Healthy != tc.wantHealthy {
				t.Errorf("Healthy = %v, want %v", result.Healthy, tc.wantHealthy)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, want an error: %v", err, tc.wantErr)
			}
			if result.ServerURL != server.URL {
				t.Errorf("ServerURL = %q, want %q", result.ServerURL, server.URL)
			}
		})
	}
}

// TestCheckHealthTransportFailureLeavesStatusCodeZero is the other side of the
// discriminator above: nothing answered, so there is no status to report.
func TestCheckHealthTransportFailureLeavesStatusCodeZero(t *testing.T) {
	t.Parallel()

	client := NewMCPClient(30 * time.Second)
	result, err := client.CheckHealth("http://invalid-host-that-does-not-exist.invalid:9999")

	if err == nil {
		t.Fatal("want an error for an unroutable host")
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 — a transport failure has no status", result.StatusCode)
	}
	if result.Healthy {
		t.Error("Healthy = true for a transport failure")
	}
}

// TestNewMCPClientWithTimeoutBoundsASlowProbe covers the timeout seam. The old
// 10s default was measured at 30s for three unroutable servers probed serially,
// which is a long time to wait for a diagnostic; the default is now 5s and a
// caller that needs a different bound passes one.
func TestNewMCPClientWithTimeoutBoundsASlowProbe(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	// Deferred LIFO, and the ORDER is load-bearing: httptest.Server.Close blocks
	// until every in-flight handler returns, so the handler has to be released
	// FIRST or the test deadlocks (observed: "httptest.Server blocked in Close
	// after 5 seconds"). The client's own timeout abandons the request; it does
	// not stop the handler.
	defer slow.Close()
	defer close(release)

	client := NewMCPClientWithTimeout(30*time.Second, 100*time.Millisecond)
	start := time.Now()
	result, err := client.CheckHealth(slow.URL)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("want a timeout error from a server that never responds")
	}
	if result.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 for a timeout", result.StatusCode)
	}
	if elapsed > 5*time.Second {
		t.Errorf("probe took %s, want it bounded by the configured 100ms timeout", elapsed)
	}
}

// TestNewMCPClientWithTimeoutDefaults asserts the documented defaults, including
// the lowered probe timeout — a number a caller reasons about when it decides how
// long `adb mcp check` may take.
func TestNewMCPClientWithTimeoutDefaults(t *testing.T) {
	t.Parallel()

	if DefaultMCPProbeTimeout != 5*time.Second {
		t.Errorf("DefaultMCPProbeTimeout = %s, want 5s (a diagnostic must not hang; three "+
			"unroutable servers at the old 10s each measured 30s)", DefaultMCPProbeTimeout)
	}

	tests := []struct {
		name        string
		ttl         time.Duration
		timeout     time.Duration
		wantTTL     time.Duration
		wantTimeout time.Duration
	}{
		{"explicit values are kept", 2 * time.Second, time.Second, 2 * time.Second, time.Second},
		{"a zero ttl falls back", 0, time.Second, 30 * time.Second, time.Second},
		{"a negative ttl falls back", -time.Second, time.Second, 30 * time.Second, time.Second},
		{"a zero timeout falls back", time.Second, 0, time.Second, DefaultMCPProbeTimeout},
		{"a negative timeout falls back", time.Second, -time.Second, time.Second, DefaultMCPProbeTimeout},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, ok := NewMCPClientWithTimeout(tc.ttl, tc.timeout).(*DefaultMCPClient)
			if !ok {
				t.Fatal("NewMCPClientWithTimeout did not return a *DefaultMCPClient")
			}
			if client.ttl != tc.wantTTL {
				t.Errorf("ttl = %s, want %s", client.ttl, tc.wantTTL)
			}
			if client.httpClient.Timeout != tc.wantTimeout {
				t.Errorf("timeout = %s, want %s", client.httpClient.Timeout, tc.wantTimeout)
			}
			if client.httpClient.CheckRedirect == nil {
				t.Error("CheckRedirect is nil — the client would follow redirects again")
			}
		})
	}
}
