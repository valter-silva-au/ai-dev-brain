package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bug these tests pin (TASK-00039 follow-up 9): GenerateRepoContext skipped
// the worktree tree with
//
//	strings.HasPrefix(relPath, "work/")
//
// against a relPath that came from filepath.Rel — which returns OS separators.
// On Windows that string is `work\github.com\...`, the prefix never matched, and
// the skip silently did nothing: `adb context repos` would walk and document
// every ticket worktree, bloating .adb/repo-context.md with thousands of paths.
//
// A test that can only build "work/x" on the host it runs on cannot see that, so
// the separator is a PARAMETER of isUnderWorkTree rather than filepath.Separator
// read inside it. That is what lets the Windows rows below fail on darwin for the
// real reason instead of deferring the assertion to a platform nobody here runs.

func TestIsUnderWorkTree(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		relPath string
		sep     rune
		want    bool
	}{
		// The Unix separator — the only case the old code got right.
		{"unix: a worktree", "work/github.com/acme/thing/TASK-1", '/', true},
		{"unix: a file directly under work", "work/notes.md", '/', true},
		{"unix: a direct child of work", "work/github.com", '/', true},

		// The Windows separator. These are the rows that fail against the old
		// prefix test, on darwin, which is the point of the parameter.
		{"windows: a worktree", `work\github.com\acme\thing\TASK-1`, '\\', true},
		{"windows: a file directly under work", `work\notes.md`, '\\', true},
		{"windows: a direct child of work", `work\github.com`, '\\', true},

		// work itself is NOT under the work tree, on either platform. The walk
		// relies on that: it emits the `- work/` row and prunes at each direct
		// child instead, and that darwin-visible output must not change.
		{"unix: work itself", "work", '/', false},
		{"windows: work itself", "work", '\\', false},

		// A sibling whose name merely starts with "work".
		{"unix: workspace sibling", "workspace/x", '/', false},
		{"windows: workspace sibling", `workspace\x`, '\\', false},
		{"unix: work-ish file", "workflow.md", '/', false},

		// Only the root-level work tree is skipped; a nested "work" is not it.
		{"unix: nested work", "tickets/work/x", '/', false},
		{"windows: nested work", `tickets\work\x`, '\\', false},

		// The negative control for the Unix side, and the reason the separator is
		// a parameter rather than "treat both slashes as separators": on Unix a
		// backslash is a legal character in a file name, so `work\x` is ONE
		// component at the workspace root and must not be skipped.
		{`unix: a file literally named work\x`, `work\x`, '/', false},

		// Degenerate inputs filepath.Rel can hand back.
		{"unix: the root itself", ".", '/', false},
		{"windows: the root itself", ".", '\\', false},
		{"unix: empty", "", '/', false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isUnderWorkTree(tc.relPath, tc.sep); got != tc.want {
				t.Errorf(
					"isUnderWorkTree(%q, %q) = %v, want %v",
					tc.relPath, string(tc.sep), got, tc.want,
				)
			}
		})
	}
}

// TestIsUnderWorkTree_MatchesTheHostSeparator ties the helper back to the real
// input it is fed: whatever filepath.Rel produces on THIS platform must be
// classified correctly when the call site passes filepath.Separator. On darwin
// this duplicates the unix rows above; on Windows it is the row that matters.
func TestIsUnderWorkTree_MatchesTheHostSeparator(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	worktree := filepath.Join(root, "work", "github.com", "acme", "thing")

	rel, err := filepath.Rel(root, worktree)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if !isUnderWorkTree(rel, filepath.Separator) {
		t.Errorf(
			"isUnderWorkTree(%q, %q) = false for a path filepath.Rel produced "+
				"under work/ — the worktree skip is dead on this platform",
			rel, string(filepath.Separator),
		)
	}
}

// TestGenerateRepoContext_ExcludesTheWorktreeTree is a darwin-visible invariance
// guard, not a red test: the skip already worked here, and the fix must not
// change that. It fails if a future edit prunes too little (a worktree path
// appears) or too much (the `- work/` row disappears).
func TestGenerateRepoContext_ExcludesTheWorktreeTree(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// A repo-backed ticket's worktree, nested exactly as the correlation layout
	// puts it (L100 §5), with a file inside so the walk has something to emit.
	deep := filepath.Join(root, "work", "github.com", "acme", "thing", "TASK-1", "internal")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deep, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed worktree file: %v", err)
	}
	// Two ordinary trees that MUST survive the walk.
	if err := os.MkdirAll(filepath.Join(root, "tickets", "_local"), 0o755); err != nil {
		t.Fatalf("seed tickets: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "repos"), 0o755); err != nil {
		t.Fatalf("seed repos: %v", err)
	}
	backlog := filepath.Join(root, "backlog.yaml")
	if err := os.WriteFile(backlog, []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("seed backlog: %v", err)
	}

	cg := NewContextGenerator(backlog, filepath.Join(root, "tickets"), root, nil)
	if err := cg.GenerateRepoContext(); err != nil {
		t.Fatalf("GenerateRepoContext: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".adb", "repo-context.md"))
	if err != nil {
		t.Fatalf("read repo-context.md: %v", err)
	}
	got := string(data)

	// Nothing below work/ may be documented, under either separator spelling.
	for _, forbidden := range []string{
		"work/github.com", `work\github.com`, "TASK-1",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("repo-context.md documents %q inside the worktree tree:\n%s", forbidden, got)
		}
	}
	// …but work itself, and the ordinary trees, still appear.
	for _, want := range []string{"- `work/`", "- `tickets/`", "- `repos/`"} {
		if !strings.Contains(got, want) {
			t.Errorf("repo-context.md is missing the %q row:\n%s", want, got)
		}
	}
	// Every documented row is written with forward slashes, so the generated
	// markdown does not change shape with the host OS.
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, `\`) {
			t.Errorf("repo-context.md row uses a backslash separator: %q", line)
		}
	}
}

// seedRepoContextWorkspace builds a workspace at root, runs GenerateRepoContext
// against it, and returns the generated .adb/repo-context.md. dirs are created
// as workspace-relative directory paths (slash-separated for readability).
func seedRepoContextWorkspace(t *testing.T, root string, dirs ...string) string {
	t.Helper()

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("seed workspace root %s: %v", root, err)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatalf("seed %s: %v", d, err)
		}
	}
	backlog := filepath.Join(root, "backlog.yaml")
	if err := os.WriteFile(backlog, []byte("tasks: []\n"), 0o644); err != nil {
		t.Fatalf("seed backlog: %v", err)
	}

	cg := NewContextGenerator(backlog, filepath.Join(root, "tickets"), root, nil)
	if err := cg.GenerateRepoContext(); err != nil {
		t.Fatalf("GenerateRepoContext: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".adb", "repo-context.md"))
	if err != nil {
		t.Fatalf("read repo-context.md: %v", err)
	}
	return string(data)
}

// TestGenerateRepoContext_DotNamedWorkspaceRootIsStillWalked pins the worst of
// the three walk defects: a TOTAL, SILENT failure reported as success.
//
// The hidden-directory skip was written as
//
//	strings.HasPrefix(filepath.Base(path), ".")
//
// and on the very first callback invocation `path` IS cg.repoRoot. So for a
// workspace whose own base name starts with a dot — `~/.adb-workspace`,
// `~/.local/share/adb`, a dot-prefixed CI checkout — the ROOT matched its own
// skip rule, the callback returned filepath.SkipDir, and the walk ended before
// visiting a single child. The generated .adb/repo-context.md held nothing but
// the two header lines, and `adb context repos` printed a checkmark over it.
//
// The root is not a hidden path component WITHIN the workspace, so it must never
// prune the walk. relPath == "." is what distinguishes it, unambiguously.
func TestGenerateRepoContext_DotNamedWorkspaceRootIsStillWalked(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), ".adb-workspace")
	got := seedRepoContextWorkspace(t, root, "tickets/_local", "repos", "wiki")

	for _, want := range []string{"- `tickets/`", "- `repos/`", "- `wiki/`"} {
		if !strings.Contains(got, want) {
			t.Errorf(
				"a workspace root whose base name starts with %q yielded no %q row — "+
					"the root matched its own hidden-directory skip and pruned the whole walk:\n%s",
				".", want, got,
			)
		}
	}
}

// TestGenerateRepoContext_OmitsTheRootRow: the workspace root is not a directory
// INSIDE the workspace, so it is not a structure row. At the root relPath is
// ".", which has zero separators and so passed the depth check, and
// filepath.ToSlash(".") is "." — emitting a literal "- `./`" line that names the
// document's own container.
func TestGenerateRepoContext_OmitsTheRootRow(t *testing.T) {
	t.Parallel()

	got := seedRepoContextWorkspace(t, t.TempDir(), "tickets", "repos")

	for _, forbidden := range []string{"- `./`", "- `.`"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("repo-context.md documents the workspace root as %q:\n%s", forbidden, got)
		}
	}
	// The guard is not vacuous only if real rows are still produced.
	if !strings.Contains(got, "- `tickets/`") {
		t.Fatalf("repo-context.md lost its ordinary rows:\n%s", got)
	}
}

// TestGenerateRepoContext_PrunesNestedHiddenDirectories is the invariance half of
// the root fix: exempting the root must not weaken the skip for a hidden
// directory found INSIDE the workspace. Pruning is the whole point of returning
// filepath.SkipDir — .git alone is thousands of paths — so both the hidden
// directory's own row and its children must be absent.
func TestGenerateRepoContext_PrunesNestedHiddenDirectories(t *testing.T) {
	t.Parallel()

	got := seedRepoContextWorkspace(
		t, t.TempDir(),
		".git/objects/pack",
		".adb/cache",
		".hidden-sibling/inside",
		"tickets/_local",
	)

	for _, forbidden := range []string{
		".git", ".adb", ".hidden-sibling", "objects", "cache", "inside",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf(
				"repo-context.md documents %q — a nested hidden directory or one of "+
					"its children is no longer pruned:\n%s",
				forbidden, got,
			)
		}
	}
	if !strings.Contains(got, "- `tickets/`") {
		t.Fatalf("repo-context.md lost its ordinary rows:\n%s", got)
	}
}

// TestRepoContextWalk_BaseOfRelPathMatchesBaseOfPath pins the equivalence that
// makes the hidden test's switch from filepath.Base(path) to
// filepath.Base(relPath) behaviour-neutral: for every entry filepath.Walk
// produces under a root, EXCEPT the root itself, the two are the same string.
//
// The exception is the whole bug. At the root, Base(path) is the workspace
// directory's own name — which is why a dot-named workspace pruned its own walk —
// while Base(relPath) is ".". The implementation returns before this test on that
// case, and this test documents why nothing else moved.
func TestRepoContextWalk_BaseOfRelPathMatchesBaseOfPath(t *testing.T) {
	t.Parallel()

	// A root that itself starts with a dot, plus names chosen to be awkward:
	// hidden, spaced, and (on Unix) containing a literal backslash.
	root := filepath.Join(t.TempDir(), ".dot-named-root")
	for _, d := range []string{
		"one/two/three", ".git/objects", "work/github.com/acme/thing",
		"sp ace/child", `back\slash/child`,
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatalf("seed %s: %v", d, err)
		}
	}

	compared, rootSeen := 0, false
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			rootSeen = true
			if filepath.Base(path) == filepath.Base(rel) {
				t.Errorf(
					"at the root, Base(path)=%q and Base(relPath)=%q agree — the "+
						"divergence this fix depends on is gone",
					filepath.Base(path), filepath.Base(rel),
				)
			}
			return nil
		}
		compared++
		if filepath.Base(path) != filepath.Base(rel) {
			t.Errorf(
				"relPath %q: Base(path)=%q but Base(relPath)=%q — the hidden test is "+
					"NOT separator/spelling-neutral for non-root entries",
				rel, filepath.Base(path), filepath.Base(rel),
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if !rootSeen {
		t.Fatal("filepath.Walk never visited the root — the root case is untested")
	}
	if compared == 0 {
		t.Fatal("compared no non-root entries; this guard would pass vacuously")
	}
}

// TestGenerateRepoContext_DocumentsExactlyTwoLevels pins the depth bound where it
// is, and with it the accuracy of the comment above it.
//
// The bound is `strings.Count(relPath, sep) < 2`, which admits 0 or 1 separators.
// Before the root was exempted that was THREE distinct depths — "." (0), "a" (0)
// and "a/b" (1) — so the "first two levels" comment was false. Removing the root
// row leaves exactly the two levels the comment claims, with the bound untouched:
// changing which directories appear in every user's generated repo-context.md
// would be a behaviour change nobody asked for.
func TestGenerateRepoContext_DocumentsExactlyTwoLevels(t *testing.T) {
	t.Parallel()

	got := seedRepoContextWorkspace(t, t.TempDir(), "one/two/three/four")

	for _, want := range []string{"- `one/`", "- `one/two/`"} {
		if !strings.Contains(got, want) {
			t.Errorf("repo-context.md is missing the level-1/level-2 row %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"- `one/two/three/`", "- `one/two/three/four/`"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("repo-context.md documents %q, past the two-level bound:\n%s", forbidden, got)
		}
	}
}
