package cloudsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// buildFixtureWorkspace lays down a fixture workspace tree that exercises
// include roots and the hard denials. Returns the tempdir root.
func buildFixtureWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"raw/a.md":                          "raw",
		".env":                              "SECRET",
		".omnictx/corpus.enc":               "PRIVATE",
		"tickets/x/context.md":              "ok",
		"tickets/x/communications/slack.md": "PRIVATE",
		"CLAUDE.md":                         "root config",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestStageFile_RefusesSymlink is defence in-depth: even if a future
// refactor lets a symlink through the walker, stageFile MUST refuse to
// dereference it. Belt AND suspenders — the walker guard is the primary
// defence, this is the fallback.
func TestStageFile_RefusesSymlink(t *testing.T) {
	base := t.TempDir()
	staging := t.TempDir()

	target := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(target, []byte("PRIVATE"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "raw", "evil")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := stageFile(base, staging, "raw/evil")
	if err == nil {
		t.Fatal("stageFile must refuse to stage a symlink; got nil error")
	}
	// The symlink target's contents MUST NOT have been copied.
	if data, readErr := os.ReadFile(filepath.Join(staging, "raw", "evil")); readErr == nil {
		if string(data) == "PRIVATE" {
			t.Errorf("stageFile followed the symlink and wrote the target's contents")
		}
	}
}

// TestPush_UploadsAllowlistedOnly asserts Push:
//  1. never uploads .env / .omnictx / communications
//  2. always uploads a repos-manifest.tsv key
//  3. gitleaks runs BEFORE any upload (fail-closed on finding)
func TestPush_UploadsAllowlistedOnly(t *testing.T) {
	root := buildFixtureWorkspace(t)
	store := newFakeStore()
	cleanRunner := func(args ...string) ([]byte, int, error) { return []byte("clean"), 0, nil }

	cfg := Config{
		BasePath: root,
		Bucket:   "test-bucket",
		Region:   "ap-southeast-2",
		Store:    store,
		Leak:     cleanRunner,
	}
	if err := Push(context.Background(), cfg); err != nil {
		t.Fatalf("Push: %v", err)
	}

	got, _ := store.List(context.Background(), "")
	sort.Strings(got)
	want := []string{
		"CLAUDE.md",
		"raw/a.md",
		"repos-manifest.tsv",
		"tickets/x/context.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Push uploaded set mismatch\n got=%v\nwant=%v", got, want)
	}

	// Sanity: none of the deny-listed keys exist.
	for _, k := range got {
		for _, banned := range []string{".env", ".omnictx", "communications"} {
			if strings.Contains(k, banned) {
				t.Errorf("banned segment %q in uploaded key %q", banned, k)
			}
		}
	}
}

// TestPush_AbortsOnLeakFinding: gitleaks reports leak → Push must return
// an error AND upload nothing.
func TestPush_AbortsOnLeakFinding(t *testing.T) {
	root := buildFixtureWorkspace(t)
	store := newFakeStore()
	leakRunner := func(args ...string) ([]byte, int, error) {
		return []byte("finding: AKIA..."), 1, nil
	}
	cfg := Config{
		BasePath: root, Bucket: "b", Region: "ap-southeast-2",
		Store: store, Leak: leakRunner,
	}
	if err := Push(context.Background(), cfg); err == nil {
		t.Fatal("Push must return error on leak finding, got nil")
	}
	got, _ := store.List(context.Background(), "")
	if len(got) != 0 {
		t.Errorf("Push must upload NOTHING on leak, got %v", got)
	}
}

// TestPush_AbortsOnScannerError: gitleaks not on PATH / start error →
// Push must fail-CLOSED (return err, upload nothing). Never proceed on
// scanner malfunction.
func TestPush_AbortsOnScannerError(t *testing.T) {
	root := buildFixtureWorkspace(t)
	store := newFakeStore()
	brokenRunner := func(args ...string) ([]byte, int, error) {
		return nil, -1, errors.New("gitleaks not on PATH")
	}
	cfg := Config{
		BasePath: root, Bucket: "b", Region: "ap-southeast-2",
		Store: store, Leak: brokenRunner,
	}
	if err := Push(context.Background(), cfg); err == nil {
		t.Fatal("Push must fail-CLOSED on scanner error, got nil")
	}
	got, _ := store.List(context.Background(), "")
	if len(got) != 0 {
		t.Errorf("Push must upload NOTHING on scanner error, got %v", got)
	}
}

// TestPush_DryRunSkipsUpload: DryRun=true never calls Put and never
// invokes the scanner. Used by 'adb sync cloud push --dry-run'.
func TestPush_DryRunSkipsUpload(t *testing.T) {
	root := buildFixtureWorkspace(t)
	store := newFakeStore()
	scannerCalled := false
	runner := func(args ...string) ([]byte, int, error) {
		scannerCalled = true
		return []byte("clean"), 0, nil
	}
	cfg := Config{
		BasePath: root, Bucket: "b", Region: "ap-southeast-2",
		Store: store, Leak: runner, DryRun: true,
	}
	if err := Push(context.Background(), cfg); err != nil {
		t.Fatalf("Push --dry-run: %v", err)
	}
	got, _ := store.List(context.Background(), "")
	if len(got) != 0 {
		t.Errorf("--dry-run must not upload, got %v", got)
	}
	if scannerCalled {
		t.Errorf("--dry-run should NOT call the scanner")
	}
}

// TestPull_RestoresRoundTrip: keys in the store → files under destDir.
// Guards against writes escaping destDir (path-escape).
func TestPull_RestoresRoundTrip(t *testing.T) {
	store := newFakeStore()
	// Legitimate keys.
	_ = store.Put(context.Background(), "raw/a.md", strings.NewReader("hello"))
	_ = store.Put(context.Background(), "wiki/nested/deep/x.md", strings.NewReader("deep"))
	_ = store.Put(context.Background(), "repos-manifest.tsv", strings.NewReader("path\torigin\thead\tbranch\n"))

	dest := t.TempDir()
	cfg := Config{Bucket: "b", Region: "ap-southeast-2", Store: store}
	if err := Pull(context.Background(), cfg, dest); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, rel := range []string{"raw/a.md", "wiki/nested/deep/x.md", "repos-manifest.tsv"} {
		full := filepath.Join(dest, rel)
		b, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("missing pulled file %q: %v", rel, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("pulled file %q is empty", rel)
		}
	}
}

// TestPull_RefusesPathEscape: a malicious server key with ../ MUST NOT
// escape destDir. This is defence-in-depth against a compromised bucket.
func TestPull_RefusesPathEscape(t *testing.T) {
	// Use a canary path in the *parent* of destDir — if the guard fails,
	// that canary appears; if it holds, the canary never exists.
	parent := t.TempDir()
	dest := filepath.Join(parent, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	canaryName := "should-never-exist-canary.md"

	store := newFakeStore()
	_ = store.Put(context.Background(), "../"+canaryName, strings.NewReader("malicious"))

	cfg := Config{Bucket: "b", Region: "ap-southeast-2", Store: store}
	err := Pull(context.Background(), cfg, dest)
	if err == nil {
		t.Fatal("Pull must reject a key that escapes destDir")
	}
	if _, err := os.Stat(filepath.Join(parent, canaryName)); err == nil {
		t.Errorf("path-escape wrote outside destDir at %q", filepath.Join(parent, canaryName))
	}

	// Also verify a deeper traversal is refused.
	store2 := newFakeStore()
	_ = store2.Put(context.Background(), "raw/../../"+canaryName, strings.NewReader("malicious"))
	dest2 := filepath.Join(parent, "dest2")
	if err := os.MkdirAll(dest2, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg2 := Config{Bucket: "b", Region: "ap-southeast-2", Store: store2}
	if err := Pull(context.Background(), cfg2, dest2); err == nil {
		t.Fatal("Pull must reject deep-traversal key")
	}
	if _, err := os.Stat(filepath.Join(parent, canaryName)); err == nil {
		t.Errorf("deep path-escape wrote outside destDir")
	}
}

// TestStatus_CountsRemoteAndLocal: Status returns object count remotely
// and upload-set count locally. Used by 'adb sync cloud status' to show
// drift at a glance.
func TestStatus_CountsRemoteAndLocal(t *testing.T) {
	root := buildFixtureWorkspace(t)
	store := newFakeStore()
	_ = store.Put(context.Background(), "raw/a.md", strings.NewReader("x"))
	_ = store.Put(context.Background(), "wiki/b.md", strings.NewReader("x"))

	cfg := Config{
		BasePath: root, Bucket: "b", Region: "ap-southeast-2", Store: store,
	}
	rep, err := Status(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if rep.RemoteObjects != 2 {
		t.Errorf("RemoteObjects = %d, want 2", rep.RemoteObjects)
	}
	// Local upload set: CLAUDE.md, raw/a.md, tickets/x/context.md = 3
	if rep.LocalUploadSet != 3 {
		t.Errorf("LocalUploadSet = %d, want 3", rep.LocalUploadSet)
	}
}

// TestDestroy_RequiresConfirm: no --confirm → error, no delete. Guard
// against accidental object nuke.
func TestDestroy_RequiresConfirm(t *testing.T) {
	store := newFakeStore()
	_ = store.Put(context.Background(), "raw/a.md", strings.NewReader("x"))
	cfg := Config{Bucket: "b", Region: "ap-southeast-2", Store: store}
	if err := Destroy(context.Background(), cfg, false); err == nil {
		t.Fatal("Destroy(confirm=false) must return error")
	}
	got, _ := store.List(context.Background(), "")
	if len(got) != 1 {
		t.Errorf("Destroy without confirm must not delete, got %v", got)
	}
}

// TestDestroy_WithConfirmDeletesAll: confirm=true clears every object.
// The BUCKET itself is torn down by 'cdk destroy', not this — this
// empties objects so a versioned bucket can be dropped cleanly.
func TestDestroy_WithConfirmDeletesAll(t *testing.T) {
	store := newFakeStore()
	_ = store.Put(context.Background(), "raw/a.md", strings.NewReader("x"))
	_ = store.Put(context.Background(), "wiki/b.md", strings.NewReader("x"))
	cfg := Config{Bucket: "b", Region: "ap-southeast-2", Store: store}
	if err := Destroy(context.Background(), cfg, true); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	got, _ := store.List(context.Background(), "")
	if len(got) != 0 {
		t.Errorf("Destroy(confirm=true) must clear all, got %v", got)
	}
}
