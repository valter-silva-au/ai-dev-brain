package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"pgregory.net/rapid"
)

// =============================================================================
// Generators
// =============================================================================

// genNonEmptyAlphaString generates a non-empty string of lowercase letters.
func genNonEmptyAlphaString(t *rapid.T, label string) string {
	return rapid.StringMatching(`[a-z]{1,20}`).Draw(t, label)
}

// genValidPriority generates a valid Priority value.
func genValidPriority(t *rapid.T, label string) models.Priority {
	priorities := []models.Priority{models.P0, models.P1, models.P2, models.P3}
	return priorities[rapid.IntRange(0, len(priorities)-1).Draw(t, label)]
}

// genGlobalConfigValues generates random valid global config field values.
type globalConfigValues struct {
	DefaultAI        string
	TaskIDPrefix     string
	TaskIDCounter    int
	DefaultPriority  models.Priority
	DefaultOwner     string
	ScreenshotHotkey string
	OfflineMode      bool
}

func genGlobalConfigValues(t *rapid.T) globalConfigValues {
	return globalConfigValues{
		DefaultAI:        rapid.SampledFrom([]string{"kiro", "claude", "gemini", "copilot"}).Draw(t, "ai"),
		TaskIDPrefix:     rapid.StringMatching(`[A-Z]{2,6}`).Draw(t, "prefix"),
		TaskIDCounter:    rapid.IntRange(0, 99999).Draw(t, "counter"),
		DefaultPriority:  genValidPriority(t, "priority"),
		DefaultOwner:     "@" + genNonEmptyAlphaString(t, "owner"),
		ScreenshotHotkey: rapid.SampledFrom([]string{"ctrl+shift+s", "ctrl+alt+p", "cmd+shift+4"}).Draw(t, "hotkey"),
		OfflineMode:      rapid.Bool().Draw(t, "offline"),
	}
}

// genRepoConfigValues generates random valid repo config field values.
type repoConfigValues struct {
	BuildCommand     string
	TestCommand      string
	DefaultReviewers []string
	Conventions      []string
}

func genRepoConfigValues(t *rapid.T) repoConfigValues {
	numReviewers := rapid.IntRange(0, 3).Draw(t, "numReviewers")
	reviewers := make([]string, numReviewers)
	for i := range reviewers {
		reviewers[i] = "@" + genNonEmptyAlphaString(t, fmt.Sprintf("reviewer_%d", i))
	}

	numConventions := rapid.IntRange(0, 3).Draw(t, "numConventions")
	conventions := make([]string, numConventions)
	for i := range conventions {
		conventions[i] = genNonEmptyAlphaString(t, fmt.Sprintf("convention_%d", i))
	}

	return repoConfigValues{
		BuildCommand:     rapid.SampledFrom([]string{"make build", "go build ./...", "cargo build", "npm run build"}).Draw(t, "buildCmd"),
		TestCommand:      rapid.SampledFrom([]string{"make test", "go test ./...", "cargo test", "npm test"}).Draw(t, "testCmd"),
		DefaultReviewers: reviewers,
		Conventions:      conventions,
	}
}

// =============================================================================
// YAML writers
// =============================================================================

// mustWriteTaskconfigYAML writes a .taskconfig.yaml file with the given values.
// It calls t.Fatal on error.
func mustWriteTaskconfigYAML(t *testing.T, dir string, v globalConfigValues) {
	t.Helper()
	content := fmt.Sprintf(`version: "1.0"
defaults:
  ai: %s
  priority: %s
  owner: "%s"
task_id:
  prefix: "%s"
  counter: %d
screenshot:
  hotkey: "%s"
offline_mode: %v
`, v.DefaultAI, string(v.DefaultPriority), v.DefaultOwner,
		v.TaskIDPrefix, v.TaskIDCounter,
		v.ScreenshotHotkey, v.OfflineMode)

	path := filepath.Join(dir, ".taskconfig.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .taskconfig.yaml: %v", err)
	}
}

// mustWriteTaskrcYAML writes a .taskrc.yaml file with the given values.
// It calls t.Fatal on error.
func mustWriteTaskrcYAML(t *testing.T, dir string, v repoConfigValues) {
	t.Helper()
	content := fmt.Sprintf("build_command: \"%s\"\ntest_command: \"%s\"\n",
		v.BuildCommand, v.TestCommand)

	if len(v.DefaultReviewers) > 0 {
		content += "default_reviewers:\n"
		for _, r := range v.DefaultReviewers {
			content += fmt.Sprintf("  - \"%s\"\n", r)
		}
	}

	if len(v.Conventions) > 0 {
		content += "conventions:\n"
		for _, c := range v.Conventions {
			content += fmt.Sprintf("  - \"%s\"\n", c)
		}
	}

	path := filepath.Join(dir, ".taskrc.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write .taskrc.yaml: %v", err)
	}
}

// =============================================================================
// Property 9: Configuration Precedence Merging
// =============================================================================

// Feature: ai-dev-brain, Property 9: Configuration Precedence Merging
// *For any* configuration key that exists in multiple sources, the merged
// configuration SHALL use the value from the highest-precedence source:
// repository .taskrc > global .taskconfig > defaults.
//
// **Validates: Requirements 15.3, 15.4**
func TestProperty9_ConfigurationPrecedenceMerging(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random global and repo config values.
		globalVals := genGlobalConfigValues(rt)
		repoVals := genRepoConfigValues(rt)

		// Set up directories.
		globalDir := t.TempDir()
		repoDir := t.TempDir()

		// Write config files.
		mustWriteTaskconfigYAML(t, globalDir, globalVals)
		mustWriteTaskrcYAML(t, repoDir, repoVals)

		// Load merged config.
		cm := NewConfigurationManager(globalDir)
		merged, err := cm.GetMergedConfig(repoDir)
		if err != nil {
			rt.Fatalf("GetMergedConfig failed: %v", err)
		}

		// --- Verify global config values are loaded from .taskconfig ---
		if merged.DefaultAI != globalVals.DefaultAI {
			rt.Errorf("DefaultAI: got %q, want %q", merged.DefaultAI, globalVals.DefaultAI)
		}
		if merged.TaskIDPrefix != globalVals.TaskIDPrefix {
			rt.Errorf("TaskIDPrefix: got %q, want %q", merged.TaskIDPrefix, globalVals.TaskIDPrefix)
		}
		if merged.TaskIDCounter != globalVals.TaskIDCounter {
			rt.Errorf("TaskIDCounter: got %d, want %d", merged.TaskIDCounter, globalVals.TaskIDCounter)
		}
		if merged.DefaultPriority != globalVals.DefaultPriority {
			rt.Errorf("DefaultPriority: got %q, want %q", merged.DefaultPriority, globalVals.DefaultPriority)
		}
		if merged.DefaultOwner != globalVals.DefaultOwner {
			rt.Errorf("DefaultOwner: got %q, want %q", merged.DefaultOwner, globalVals.DefaultOwner)
		}
		if merged.ScreenshotHotkey != globalVals.ScreenshotHotkey {
			rt.Errorf("ScreenshotHotkey: got %q, want %q", merged.ScreenshotHotkey, globalVals.ScreenshotHotkey)
		}
		if merged.OfflineMode != globalVals.OfflineMode {
			rt.Errorf("OfflineMode: got %v, want %v", merged.OfflineMode, globalVals.OfflineMode)
		}

		// --- Verify repo config takes precedence (overlays onto merged) ---
		if merged.Repo == nil {
			rt.Fatal("expected non-nil Repo in merged config")
		}
		if merged.Repo.BuildCommand != repoVals.BuildCommand {
			rt.Errorf("Repo.BuildCommand: got %q, want %q", merged.Repo.BuildCommand, repoVals.BuildCommand)
		}
		if merged.Repo.TestCommand != repoVals.TestCommand {
			rt.Errorf("Repo.TestCommand: got %q, want %q", merged.Repo.TestCommand, repoVals.TestCommand)
		}
		if len(merged.Repo.DefaultReviewers) != len(repoVals.DefaultReviewers) {
			rt.Errorf("Repo.DefaultReviewers length: got %d, want %d",
				len(merged.Repo.DefaultReviewers), len(repoVals.DefaultReviewers))
		}
		for i, r := range repoVals.DefaultReviewers {
			if i < len(merged.Repo.DefaultReviewers) && merged.Repo.DefaultReviewers[i] != r {
				rt.Errorf("Repo.DefaultReviewers[%d]: got %q, want %q",
					i, merged.Repo.DefaultReviewers[i], r)
			}
		}
		if len(merged.Repo.Conventions) != len(repoVals.Conventions) {
			rt.Errorf("Repo.Conventions length: got %d, want %d",
				len(merged.Repo.Conventions), len(repoVals.Conventions))
		}

		// --- Verify defaults are used when no .taskconfig exists ---
		emptyGlobalDir := t.TempDir()
		cmDefaults := NewConfigurationManager(emptyGlobalDir)
		mergedDefaults, err := cmDefaults.GetMergedConfig(repoDir)
		if err != nil {
			rt.Fatalf("GetMergedConfig with defaults failed: %v", err)
		}

		// Default values should be applied.
		if mergedDefaults.DefaultAI != "kiro" {
			rt.Errorf("Default DefaultAI: got %q, want %q", mergedDefaults.DefaultAI, "kiro")
		}
		if mergedDefaults.TaskIDPrefix != "TASK" {
			rt.Errorf("Default TaskIDPrefix: got %q, want %q", mergedDefaults.TaskIDPrefix, "TASK")
		}
		if mergedDefaults.DefaultPriority != models.P2 {
			rt.Errorf("Default DefaultPriority: got %q, want %q", mergedDefaults.DefaultPriority, models.P2)
		}

		// Repo config should still take precedence over defaults.
		if mergedDefaults.Repo == nil {
			rt.Fatal("expected non-nil Repo in merged defaults config")
		}
		if mergedDefaults.Repo.BuildCommand != repoVals.BuildCommand {
			rt.Errorf("Default merge Repo.BuildCommand: got %q, want %q",
				mergedDefaults.Repo.BuildCommand, repoVals.BuildCommand)
		}
	})
}

// =============================================================================
// Property 10: Configuration Validation
// =============================================================================

// Feature: ai-dev-brain, Property 10: Configuration Validation
// *For any* invalid configuration file (malformed YAML, missing required fields,
// invalid field values), the Configuration_Manager SHALL return a validation
// error with a clear message identifying the problem.
//
// **Validates: Requirements 15.3, 15.4**
func TestProperty10_ConfigurationValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		cm := NewConfigurationManager(t.TempDir())

		// Choose which type of invalid config to generate.
		invalidType := rapid.IntRange(0, 4).Draw(rt, "invalidType")

		switch invalidType {
		case 0:
			// Empty DefaultAI (required field).
			cfg := &models.GlobalConfig{
				DefaultAI:       "",
				TaskIDPrefix:    rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "prefix"),
				TaskIDCounter:   rapid.IntRange(0, 1000).Draw(rt, "counter"),
				DefaultPriority: genValidPriority(rt, "priority"),
			}
			err := cm.ValidateConfig(cfg)
			if err == nil {
				rt.Fatal("expected validation error for empty DefaultAI, got nil")
			}

		case 1:
			// Empty TaskIDPrefix (required field).
			cfg := &models.GlobalConfig{
				DefaultAI:       rapid.SampledFrom([]string{"kiro", "claude"}).Draw(rt, "ai"),
				TaskIDPrefix:    "",
				TaskIDCounter:   rapid.IntRange(0, 1000).Draw(rt, "counter"),
				DefaultPriority: genValidPriority(rt, "priority"),
			}
			err := cm.ValidateConfig(cfg)
			if err == nil {
				rt.Fatal("expected validation error for empty TaskIDPrefix, got nil")
			}

		case 2:
			// Invalid priority value.
			invalidPriorities := []models.Priority{"P4", "P5", "P9", "HIGH", "LOW", "CRITICAL"}
			invalidPriority := invalidPriorities[rapid.IntRange(0, len(invalidPriorities)-1).Draw(rt, "invalidPriorityIdx")]
			cfg := &models.GlobalConfig{
				DefaultAI:       rapid.SampledFrom([]string{"kiro", "claude"}).Draw(rt, "ai"),
				TaskIDPrefix:    rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "prefix"),
				TaskIDCounter:   rapid.IntRange(0, 1000).Draw(rt, "counter"),
				DefaultPriority: invalidPriority,
			}
			err := cm.ValidateConfig(cfg)
			if err == nil {
				rt.Fatalf("expected validation error for invalid priority %q, got nil", invalidPriority)
			}

		case 3:
			// Negative counter.
			negCounter := -rapid.IntRange(1, 10000).Draw(rt, "negCounter")
			cfg := &models.GlobalConfig{
				DefaultAI:       rapid.SampledFrom([]string{"kiro", "claude"}).Draw(rt, "ai"),
				TaskIDPrefix:    rapid.StringMatching(`[A-Z]{2,6}`).Draw(rt, "prefix"),
				TaskIDCounter:   negCounter,
				DefaultPriority: genValidPriority(rt, "priority"),
			}
			err := cm.ValidateConfig(cfg)
			if err == nil {
				rt.Fatalf("expected validation error for negative counter %d, got nil", negCounter)
			}

		case 4:
			// Invalid template task type in RepoConfig.
			invalidTypes := []models.TaskType{"invalid", "hotfix", "chore", "release", "deploy"}
			invalidTaskType := invalidTypes[rapid.IntRange(0, len(invalidTypes)-1).Draw(rt, "invalidTypeIdx")]
			rc := &models.RepoConfig{
				BuildCommand: "make build",
				Templates: map[models.TaskType]string{
					invalidTaskType: "template.md",
				},
			}
			err := cm.ValidateConfig(rc)
			if err == nil {
				rt.Fatalf("expected validation error for invalid template type %q, got nil", invalidTaskType)
			}
		}
	})
}
