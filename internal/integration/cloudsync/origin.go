package cloudsync

import "strings"

// schemesWithLoginUsername lists the schemes whose userinfo USERNAME is a login
// identity rather than a credential, and therefore has to survive.
//
// `ssh://git@github.com/org/repo.git` does not clone as `ssh://github.com/...`
// — ssh would offer the local account name instead of `git` and be refused. So
// for these schemes only a PASSWORD is removed. That is not a weaker rule: ssh
// authenticates with a key or an agent, never with a URL password, so a
// password here is unambiguously credential material that ssh will not use
// anyway.
//
// Every other scheme (http, https, git, an unrecognised one) loses the WHOLE
// userinfo, because over http(s) the username is credential material too: the
// usual way a token is embedded is as the username, with no password at all
// (`https://ghp_…@github.com/o/r.git`). An https clone needs no username — the
// credential helper supplies both parts at clone time.
var schemesWithLoginUsername = map[string]bool{
	"ssh":     true,
	"git+ssh": true,
}

// StripOriginCredentials removes credential material from a git remote URL
// while keeping the URL RE-CLONEABLE, which is what makes it the right tool
// here and blanket redaction the wrong one.
//
// `git remote get-url origin` returns whatever the clone was configured with,
// so a clone set up as `https://user:ghp_…@github.com/o/r.git` hands back the
// token verbatim — and that string used to land in repos-manifest.tsv, an
// object Push uploads to S3. Replacing the credential with "***" (as
// integration.RedactURL does, correctly, for a url that only has to be
// printed) would defeat the manifest's stated purpose: RepoEntry.Origin exists
// so a reader can reconstruct which repos were archived, which means it has to
// remain something you can hand to `git clone`.
//
// Removing the userinfo satisfies both. What is left identifies the same
// repository, and an embedded credential was never portable to another machine
// in the first place — so the stripped URL is strictly MORE useful to the
// reader of an archive, not less.
//
// Guarantees:
//
//   - A URL carrying no userinfo comes back BYTE-IDENTICAL. That is why this is
//     textual surgery and not a net/url round-trip: url.URL.String() re-encodes
//     as it goes, and url.Parse accepts an scp-style remote as an opaque path
//     whose String() form can differ from the input.
//   - It is IDEMPOTENT, so applying it at both the capture and the render
//     boundary (see manifest.go) costs nothing.
//   - Only the authority is ever touched. A '@' or ':' in the path, or a query
//     string, is left alone.
//
// Shapes, with what happens to each:
//
//	https://user:tok@host/p    → https://host/p           (whole userinfo dropped)
//	https://tok@host/p         → https://host/p           (a bare token username)
//	http://user:tok@host/p     → http://host/p
//	ssh://user:tok@host/p      → ssh://user@host/p         (password only)
//	ssh://user@host/p          → unchanged, byte-identical (a login identity)
//	git@github.com:org/r.git   → unchanged, byte-identical (scp-style: not a
//	                             URL at all, and that `git@` is a username)
//	https://github.com/o/r.git → unchanged, byte-identical
//	/srv/mirrors/repo.git      → unchanged, byte-identical (a local path)
//
// Deliberately NOT handled: a scheme-relative `//user:tok@host/p`. git has no
// such remote form — a remote is either schemed or scp-style/a path — and
// treating a leading `//` as an authority would risk mangling a POSIX path
// that legitimately contains '@' and ':'.
func StripOriginCredentials(raw string) string {
	// No "://" means no authority component, so no userinfo slot: an scp-style
	// remote (git@host:path), a filesystem path, or a transport-helper spelling
	// that has neither. Nothing to strip, and nothing to risk mangling.
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 0 {
		return raw
	}
	scheme := strings.ToLower(raw[:schemeEnd])

	// The authority runs from after "://" to the first '/', '?' or '#'.
	authStart := schemeEnd + len("://")
	authEnd := len(raw)
	if i := strings.IndexAny(raw[authStart:], "/?#"); i >= 0 {
		authEnd = authStart + i
	}
	authority := raw[authStart:authEnd]

	// The LAST '@' separates userinfo from host, so an unencoded '@' inside a
	// password still leaves the host intact.
	at := strings.LastIndexByte(authority, '@')
	if at < 0 {
		return raw // already credential-free
	}
	userinfo, hostPort := authority[:at], authority[at+1:]

	keep := ""
	if schemesWithLoginUsername[scheme] {
		// Keep the username, drop any password.
		if colon := strings.IndexByte(userinfo, ':'); colon >= 0 {
			keep = userinfo[:colon]
		} else {
			keep = userinfo
		}
	}
	if keep == "" {
		// Drop the '@' along with the userinfo — an empty username before it
		// would be a URL nothing can clone.
		return raw[:authStart] + hostPort + raw[authEnd:]
	}
	return raw[:authStart] + keep + "@" + hostPort + raw[authEnd:]
}
