// Package cloudsync selects, scans, and ships the allowlisted KB content to
// an S3 archive bucket. The allowlist here is the security boundary: it is
// fail-CLOSED (deny-first, then a strict include-root allowlist) so that a new
// top-level dir is never uploaded by accident. Mirrors the hard denials in the
// workspace .gitignore (backlog.yaml, the whole .adb/ state dir, sessions/,
// work/, repos/, .env, .omnictx/) plus the non-.gitignore-carried
// tickets/**/communications/ which is Slack/PR correspondence and MUST NOT ride
// to the cloud. Legacy root-level state names (.task_counter, .events.jsonl,
// .adb_memory.sqlite*, …) are also denied so an un-migrated workspace stays
// safe (state moved under .adb/ in #186).
//
// The denylist is deliberately NOT parsed from .gitignore at runtime — a
// parse bug there could silently WIDEN the allowlist. This is an explicit,
// code-reviewed subset that must be hardened via /security-review before any
// live deploy.
package cloudsync

import (
	"path/filepath"
	"strings"
)

// includeRoots are the only top-level trees eligible for upload. A path must
// live under one of these AND survive the denylist.
var includeRoots = map[string]bool{
	"raw":     true,
	"scripts": true,
	"skills":  true,
	"wiki":    true,
	"tickets": true,
}

// includeRootFiles are individual root-level config files that ship. Nothing
// else at the workspace root is eligible.
var includeRootFiles = map[string]bool{
	"CLAUDE.md":               true,
	"Taskfile.yaml":           true,
	"WIKI.md":                 true,
	".markdownlint-cli2.yaml": true,
	".gitleaks.toml":          true,
	".pre-commit-config.yaml": true,
	".gitignore":              true,
}

// ShouldUpload reports whether the workspace-relative path rel is eligible
// for cloud upload. Deny-first, allow-second. rel uses OS separators; empty
// / "." / absolute / traversal ("../…") paths return false. This is the
// binding security predicate — every uploader MUST route through it.
func ShouldUpload(rel string) bool {
	if rel == "" {
		return false
	}
	// Reject absolute paths outright — the caller must supply a
	// workspace-relative path. This is a defence-in-depth check.
	if filepath.IsAbs(rel) {
		return false
	}
	// Normalise separators and collapse ./ segments so the segment-wise
	// denylist has stable input.
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == "" {
		return false
	}
	// After Clean, any surviving leading "../" (or a bare "..") means the
	// path escapes the workspace root — reject.
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	segs := strings.Split(clean, "/")
	if isDenied(segs) {
		return false
	}
	// Root-level config file allowlist.
	if len(segs) == 1 {
		return includeRootFiles[segs[0]]
	}
	// Must sit under an include root.
	return includeRoots[segs[0]]
}

// isDenied enforces the hard denylist against every path segment. Any hit
// anywhere in the path is a hard NO — this defends against a nested match
// (e.g. tickets/x/communications/slack.md, or wiki/x/.omnictx/y).
func isDenied(segs []string) bool {
	for _, s := range segs {
		if deniedSegment(s) {
			return true
		}
	}
	return false
}

// deniedSegment returns true when a single path component names something
// that MUST NOT ship. Kept as a switch (not a map) to make the audit trail
// obvious in the diff — every entry is a deliberate, reviewed denial.
func deniedSegment(s string) bool {
	switch s {
	// adb machinery + workspace-private state.
	//
	// As of #186 all adb-owned per-workspace state lives under .adb/, so the
	// ".adb" segment below denies the whole relocated set (memory db, event
	// logs, counters, scheduler state, …) by a single blanket rule — a hit on
	// any path component is a hard NO, so ".adb/events.jsonl" etc. are denied.
	// The legacy root-level names are RETAINED (not redundant) so an un-migrated
	// workspace — one whose one-shot migration hasn't run or couldn't move a
	// file — still has those files denied at the root. New names never need a
	// per-file entry: dropping them under .adb/ is enough.
	case "backlog.yaml",
		".task_counter",
		".session_counter",
		".adb",
		".adb_memory.sqlite",
		".adb_memory.sqlite-shm",
		".adb_memory.sqlite-wal",
		".adb_mcp_cache.json",
		".adb_session_changes",
		".taskrc",
		".taskconfig",
		".adb-workspace-README.md",
		".events.jsonl",
		".governance.jsonl":
		return true

	// worktrees + read-only mirror + omnictx corpus
	case "work", "repos", ".omnictx":
		return true

	// per-ticket private correspondence + session transcripts
	case "communications", "sessions":
		return true
	}
	// .env and any .env.<variant> (deny even nested)
	if s == ".env" || strings.HasPrefix(s, ".env.") {
		return true
	}
	return false
}
