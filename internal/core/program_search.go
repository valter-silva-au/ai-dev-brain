package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
)

// This file resolves the EXTERNAL PROGRAM PACK search paths through the
// layered config, so external packs are configured, never hard-coded. It is
// shared by the CLI (`adb program list/scaffold`) and the read-only MCP tools
// (TASK-00031 phase 5) so the two entry points cannot drift.
//
// Precedence is most-specific tier first — Repo > Org > Global — and, within
// one tier, the canonical `programs_search_paths` before the dotted
// `programs.search_paths`. A spelling preference must never outrank a tier
// preference: a repo saying something is more specific than a global saying
// it, however either spelled it (see the CLI commentary for the defect this
// rule prevents).

// programSearchPathsKeys are the custom-setting SPELLINGS consulted for
// external pack directories, most-preferred first. `programs_search_paths` is
// canonical; the dotted spelling is a live fallback because config flattening
// (normalizeCustomSettings) no longer mangles dotted keys.
var programSearchPathsKeys = []string{"programs_search_paths", "programs.search_paths"}

// ProgramSearchPaths resolves the external-pack search paths through the
// merged config: most-specific tier first — Repo > Org > Global — and, within
// one tier, the canonical spelling before the dotted one. The value is a
// comma- or path-list-separated set of directories; a relative entry resolves
// against basePath so a checked-in `.taskrc` stays portable.
func ProgramSearchPaths(mc *models.MergedConfig, basePath string) []string {
	if mc == nil {
		return nil
	}
	// Tier-major: for each tier in specificity order, try the canonical spelling
	// then the dotted one. An EMPTY value at a tier still wins (the tier is
	// deliberately saying "no search paths"), matching SettingSource. A nil tier
	// contributes nothing (the org tier is absent when no org is active).
	raw := ""
tiers:
	for _, tier := range []map[string]string{
		customSettingsOf(mc.Repo, mc.Repo != nil),
		customSettingsOf(mc.Org, mc.Org != nil),
		customSettingsOf(mc.Global, mc.Global != nil),
	} {
		for _, key := range programSearchPathsKeys {
			if v, found := tier[key]; found {
				raw = v
				break tiers
			}
		}
	}
	if raw == "" {
		return nil
	}
	var out []string
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == os.PathListSeparator
	}) {
		dir := strings.TrimSpace(field)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(basePath, dir)
		}
		out = append(out, dir)
	}
	return out
}

// customSettingsOf returns a tier's custom settings map, tolerating a nil tier.
// The two-arg shape (typed nil is not a nil interface) keeps the nil check at
// the call site where the pointer type is known.
func customSettingsOf(t any, present bool) map[string]string {
	if !present {
		return nil
	}
	switch tier := t.(type) {
	case *models.RepoConfig:
		return tier.CustomSettings
	case *models.OrgConfig:
		return tier.CustomSettings
	case *models.GlobalConfig:
		return tier.CustomSettings
	}
	return nil
}
