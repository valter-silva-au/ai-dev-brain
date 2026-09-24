package cloudsync

import (
	"net/url"
	"strings"
	"testing"
)

// fakeToken is obviously-fake credential material. Deliberately unrealistic:
// this repo commits under a blocking Code Defender profile plus a gitleaks
// pre-commit hook, and a realistic-looking token in a fixture would (correctly)
// block the commit.
const fakeToken = "ghp_FAKEFAKEFAKE" //nolint:gosec // fixture, not a credential

// TestStripOriginCredentials covers both halves of the contract: a URL that
// carries a credential loses it, and a URL that carries none survives
// BYTE-IDENTICAL. The second half is the one that keeps the manifest useful, so
// the pass-through cases are asserted as strictly as the stripping ones.
func TestStripOriginCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// --- credential-bearing: the leak this closes ---
		{
			name: "https user and password",
			in:   "https://svcuser:" + fakeToken + "@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "https bare token as username (how PATs are usually embedded)",
			in:   "https://" + fakeToken + "@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "http user and password",
			in:   "http://svcuser:" + fakeToken + "@git.internal/org/repo.git",
			want: "http://git.internal/org/repo.git",
		},
		{
			name: "https with an explicit port",
			in:   "https://svcuser:" + fakeToken + "@git.internal:8443/org/repo.git",
			want: "https://git.internal:8443/org/repo.git",
		},
		{
			name: "ssh keeps the login username, drops the password",
			in:   "ssh://git:" + fakeToken + "@github.com/org/repo.git",
			want: "ssh://git@github.com/org/repo.git",
		},
		{
			name: "git+ssh keeps the login username, drops the password",
			in:   "git+ssh://git:" + fakeToken + "@github.com/org/repo.git",
			want: "git+ssh://git@github.com/org/repo.git",
		},
		{
			name: "transport-helper prefix keeps its prefix, loses the userinfo",
			in:   "git::https://svcuser:" + fakeToken + "@github.com/org/repo.git",
			want: "git::https://github.com/org/repo.git",
		},
		{
			name: "uppercase scheme is still recognised",
			in:   "HTTPS://svcuser:" + fakeToken + "@github.com/org/repo.git",
			want: "HTTPS://github.com/org/repo.git",
		},
		{
			name: "unencoded @ inside the password still leaves the host intact",
			in:   "https://svcuser:tok@word@github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "empty username before the password drops the @ too",
			in:   "ssh://:" + fakeToken + "@github.com/org/repo.git",
			want: "ssh://github.com/org/repo.git",
		},
		{
			name: "authority-only url with no path",
			in:   "https://svcuser:" + fakeToken + "@github.com",
			want: "https://github.com",
		},
		{
			name: "query string is preserved, userinfo is not",
			in:   "https://svcuser:" + fakeToken + "@git.internal/org/repo.git?depth=1",
			want: "https://git.internal/org/repo.git?depth=1",
		},

		// --- must survive byte-identical: no secret to remove ---
		{
			name: "scp-style is not a url and must not be mangled",
			in:   "git@github.com:org/repo.git",
			want: "git@github.com:org/repo.git",
		},
		{
			name: "scp-style with a tilde path",
			in:   "git@git.internal:~mirrors/repo.git",
			want: "git@git.internal:~mirrors/repo.git",
		},
		{
			name: "plain https",
			in:   "https://github.com/org/repo.git",
			want: "https://github.com/org/repo.git",
		},
		{
			name: "ssh with only a login username",
			in:   "ssh://git@github.com/org/repo.git",
			want: "ssh://git@github.com/org/repo.git",
		},
		{
			name: "git protocol, anonymous",
			in:   "git://github.com/org/repo.git",
			want: "git://github.com/org/repo.git",
		},
		{
			name: "file url",
			in:   "file:///srv/mirrors/repo.git",
			want: "file:///srv/mirrors/repo.git",
		},
		{
			name: "bare local absolute path",
			in:   "/srv/mirrors/repo.git",
			want: "/srv/mirrors/repo.git",
		},
		{
			name: "relative local path",
			in:   "../mirrors/repo.git",
			want: "../mirrors/repo.git",
		},
		{
			name: "an @ in the PATH is not userinfo",
			in:   "https://git.internal/org/repo@v2.git",
			want: "https://git.internal/org/repo@v2.git",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := StripOriginCredentials(tc.in)
			if got != tc.want {
				t.Errorf("StripOriginCredentials(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
			// The credential must be gone from every result, whatever the shape.
			if strings.Contains(got, fakeToken) {
				t.Errorf("result still contains the token: %q", got)
			}
			// Idempotent — this is what lets both manifest.go boundaries apply it.
			if again := StripOriginCredentials(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}

// TestStripOriginCredentials_RemainsRecloneable is the re-cloneability half of
// the contract, asserted structurally: for every credential-bearing shape, the
// scheme, host, port and path that `git clone` actually needs come back
// unchanged, and only the userinfo is gone. That is what distinguishes this from
// blanket redaction — integration.RedactURL would satisfy the "no token"
// assertion above by writing "***" into the host-adjacent userinfo and leave a
// URL nothing can clone.
func TestStripOriginCredentials_RemainsRecloneable(t *testing.T) {
	tests := []struct {
		name         string
		credentialed string
		wantUser     string // userinfo expected to survive ("" = none)
	}{
		{"https user and password", "https://svcuser:" + fakeToken + "@github.com/org/repo.git", ""},
		{"https bare token", "https://" + fakeToken + "@github.com/org/repo.git", ""},
		{"https with port", "https://svcuser:" + fakeToken + "@git.internal:8443/org/repo.git", ""},
		{"ssh with password", "ssh://git:" + fakeToken + "@github.com/org/repo.git", "git"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig, err := url.Parse(tc.credentialed)
			if err != nil {
				t.Fatalf("fixture is not a parseable url: %v", err)
			}
			stripped := StripOriginCredentials(tc.credentialed)
			got, err := url.Parse(stripped)
			if err != nil {
				t.Fatalf("stripped url no longer parses (%q): %v", stripped, err)
			}
			// Everything git needs to reach the repository is untouched.
			if got.Scheme != orig.Scheme {
				t.Errorf("scheme changed: %q -> %q", orig.Scheme, got.Scheme)
			}
			if got.Host != orig.Host {
				t.Errorf("host changed: %q -> %q", orig.Host, got.Host)
			}
			if got.Path != orig.Path {
				t.Errorf("path changed: %q -> %q", orig.Path, got.Path)
			}
			// Only the userinfo differs, and only as documented.
			switch {
			case tc.wantUser == "":
				if got.User != nil {
					t.Errorf("userinfo survived: %q", got.User.String())
				}
			default:
				if got.User == nil || got.User.Username() != tc.wantUser {
					t.Errorf("login username %q did not survive: %v", tc.wantUser, got.User)
				}
				if _, hasPass := got.User.Password(); hasPass {
					t.Errorf("password survived in %q", stripped)
				}
			}
		})
	}
}

// TestFormatManifest_StripsHandBuiltOrigin pins the defence-in-depth half: the
// renderer produces the uploaded bytes, so it must strip even when handed an
// entry that did not come through GenerateManifest.
func TestFormatManifest_StripsHandBuiltOrigin(t *testing.T) {
	body := FormatManifest([]RepoEntry{{
		Path:   "github.com/org/repo",
		Origin: "https://svcuser:" + fakeToken + "@github.com/org/repo.git",
		Head:   "abc123",
		Branch: "main",
	}})
	if strings.Contains(body, fakeToken) || strings.Contains(body, "@") {
		t.Errorf("FormatManifest leaked a hand-built credentialed origin:\n%s", body)
	}
	if !strings.Contains(body, "https://github.com/org/repo.git") {
		t.Errorf("FormatManifest lost the clone-able URL:\n%s", body)
	}
}
