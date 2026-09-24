package cloudsync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Config bundles the knobs the orchestrator reads. The concrete backing
// stores/scanners are injected here so the whole package is unit-testable
// offline (no AWS account, no network, no credentials).
type Config struct {
	BasePath string      // workspace root (all rel paths resolved against this)
	Bucket   string      // S3 bucket name (used by callers building an S3Store)
	Region   string      // AWS region — default ap-southeast-2 in the CLI
	DryRun   bool        // print the plan, don't upload / don't scan
	Store    ObjectStore // required unless DryRun; the interface seam
	Leak     LeakRunner  // required unless DryRun; injectable gitleaks runner
}

// StatusReport is the output of Status.
type StatusReport struct {
	RemoteObjects  int
	LocalUploadSet int
}

// Push stages the allowlisted upload set, runs gitleaks over it (fail-CLOSED),
// then uploads each file plus a fresh repos-manifest.tsv.
//
// SECURITY properties:
//   - allowlist.ShouldUpload gates every path (deny-first)
//   - the walker never descends into denied dirs (walk.WalkUploadSet)
//   - gitleaks is scoped to the STAGING copy (exactly what will be uploaded)
//   - any scanner error / finding aborts BEFORE any Put
//   - DryRun short-circuits everything (never contacts scanner or store)
func Push(ctx context.Context, cfg Config) error {
	if cfg.BasePath == "" {
		return errors.New("cloudsync.Push: BasePath is required")
	}

	// 1. Walk the workspace and collect the allowlisted set.
	uploadSet, err := WalkUploadSet(cfg.BasePath)
	if err != nil {
		return fmt.Errorf("walk upload set: %w", err)
	}

	// 2. Stage the set into a temp dir so gitleaks scans exactly what
	//    will ship (not the whole workspace).
	staging, err := os.MkdirTemp("", "adb-cloudsync-*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(staging)

	for _, rel := range uploadSet {
		if err := stageFile(cfg.BasePath, staging, rel); err != nil {
			return fmt.Errorf("stage %q: %w", rel, err)
		}
	}

	// 3. Add repos-manifest.tsv to the staging set.
	entries, err := GenerateManifest(cfg.BasePath)
	if err != nil {
		return fmt.Errorf("generate manifest: %w", err)
	}
	manifestBody := FormatManifest(entries)
	manifestKey := "repos-manifest.tsv"
	if err := os.WriteFile(filepath.Join(staging, manifestKey), []byte(manifestBody), 0o644); err != nil {
		return fmt.Errorf("stage manifest: %w", err)
	}

	// 4. Dry-run: report the plan and stop before scanner/upload.
	if cfg.DryRun {
		return nil
	}

	// 5. Fail-CLOSED gitleaks scan over the staging copy.
	if cfg.Leak == nil {
		return errors.New("cloudsync.Push: Leak runner is required (fail-closed)")
	}
	clean, report, err := ScanForSecretsWith(cfg.Leak, staging)
	if err != nil {
		return fmt.Errorf("secret scan failed (fail-closed): %w", err)
	}
	if !clean {
		return fmt.Errorf("secret scan found leaks; upload aborted:\n%s", report)
	}

	// 6. Upload each staged file + the manifest.
	if cfg.Store == nil {
		return errors.New("cloudsync.Push: Store is required")
	}
	toUpload := append([]string{}, uploadSet...)
	toUpload = append(toUpload, manifestKey)
	for _, rel := range toUpload {
		if err := putStagedFile(ctx, cfg.Store, staging, rel); err != nil {
			return fmt.Errorf("upload %q: %w", rel, err)
		}
	}
	return nil
}

// stageFile copies basePath/rel into staging/rel, preserving the
// workspace-relative path so gitleaks sees the same tree S3 will hold.
// mkdir semantics + safety: relative dirs only, no traversal.
//
// SECURITY: os.Lstat FIRST — refuse anything that isn't a regular file.
// This is defence in-depth for the walker's symlink skip: even if a
// future refactor lets a symlink through, we never dereference it and
// copy the (potentially out-of-tree) target into the upload set.
func stageFile(basePath, staging, rel string) error {
	src := filepath.Join(basePath, rel)
	dst := filepath.Join(staging, rel)

	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	// !info.Mode().IsRegular() covers symlinks, sockets, devices, named
	// pipes — everything that isn't a plain file.
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refuse to stage %q: not a regular file (mode=%s)", rel, info.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// putStagedFile opens the staged copy at staging/rel and Puts it under
// key=rel (using / separators, per S3 convention).
func putStagedFile(ctx context.Context, store ObjectStore, staging, rel string) error {
	f, err := os.Open(filepath.Join(staging, rel))
	if err != nil {
		return err
	}
	defer f.Close()
	key := filepath.ToSlash(rel)
	return store.Put(ctx, key, f)
}

// Pull downloads every object from Store into destDir, preserving the
// key hierarchy. Refuses to write anywhere outside destDir (defence
// in-depth against a compromised bucket serving ../-escaped keys).
func Pull(ctx context.Context, cfg Config, destDir string) error {
	if cfg.Store == nil {
		return errors.New("cloudsync.Pull: Store is required")
	}
	if destDir == "" {
		return errors.New("cloudsync.Pull: destDir is required")
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	keys, err := cfg.Store.List(ctx, "")
	if err != nil {
		return fmt.Errorf("list bucket: %w", err)
	}
	for _, key := range keys {
		if err := pullOne(ctx, cfg.Store, absDest, key); err != nil {
			return err
		}
	}
	return nil
}

// pullOne materialises one object under destDir, hardened against a
// key like "../../etc/passwd" or "raw/../../evil". The final absolute
// destination MUST have absDest as a prefix.
func pullOne(ctx context.Context, store ObjectStore, absDest, key string) error {
	// Reject an obviously bad key up front.
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("refuse absolute S3 key %q", key)
	}
	target := filepath.Join(absDest, filepath.FromSlash(key))
	cleaned, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return err
	}
	// The cleaned target must live inside absDest.
	if !hasPathPrefix(cleaned, absDest) {
		return fmt.Errorf("refuse key %q: escapes destDir (%q -> %q)", key, absDest, cleaned)
	}
	if err := os.MkdirAll(filepath.Dir(cleaned), 0o755); err != nil {
		return err
	}
	rc, err := store.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(cleaned)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// hasPathPrefix reports whether path lives inside prefix (with a
// trailing / on prefix so /foo doesn't accidentally match /foobar).
func hasPathPrefix(path, prefix string) bool {
	if prefix == "" {
		return false
	}
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if path+string(filepath.Separator) == prefix {
		return true
	}
	return strings.HasPrefix(path+string(filepath.Separator), prefix)
}

// Status returns a small drift summary: how many objects exist remotely,
// how many local files would upload right now.
func Status(ctx context.Context, cfg Config) (StatusReport, error) {
	var rep StatusReport
	if cfg.Store != nil {
		keys, err := cfg.Store.List(ctx, "")
		if err != nil {
			return rep, err
		}
		rep.RemoteObjects = len(keys)
	}
	if cfg.BasePath != "" {
		set, err := WalkUploadSet(cfg.BasePath)
		if err != nil {
			return rep, err
		}
		rep.LocalUploadSet = len(set)
	}
	return rep, nil
}

// Destroy clears every object in the bucket. Requires an explicit
// confirm=true. This does NOT tear down the bucket — that's `cdk destroy`.
// Emptying the (versioned) bucket is a prerequisite step for a clean
// stack teardown.
func Destroy(ctx context.Context, cfg Config, confirm bool) error {
	if !confirm {
		return errors.New("cloudsync.Destroy: refuse without confirm=true")
	}
	if cfg.Store == nil {
		return errors.New("cloudsync.Destroy: Store is required")
	}
	keys, err := cfg.Store.List(ctx, "")
	if err != nil {
		return fmt.Errorf("list bucket: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	return cfg.Store.Delete(ctx, keys)
}
