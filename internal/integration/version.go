package integration

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ClaudeCodeVersion represents a semantic version of Claude Code.
type ClaudeCodeVersion struct {
	Major int
	Minor int
	Patch int
}

// String returns the version in semver format (e.g., "2.1.50").
func (v ClaudeCodeVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other.
func (v ClaudeCodeVersion) Compare(other ClaudeCodeVersion) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// ClaudeCodeVersionChecker defines operations for detecting and validating
// Claude Code versions.
type ClaudeCodeVersionChecker interface {
	// DetectVersion runs `claude --version` and parses the output.
	DetectVersion() (*ClaudeCodeVersion, error)
	// CheckMinimumVersion returns an error if the detected version is less than the required minimum.
	CheckMinimumVersion(min ClaudeCodeVersion) error
	// SupportsFeature returns true if the detected version supports the named feature.
	SupportsFeature(feature string) (bool, error)
}

// claudeCodeVersionChecker implements ClaudeCodeVersionChecker.
type claudeCodeVersionChecker struct {
	// versionParser is injected for testability. If nil, uses parseVersionString.
	versionParser func(string) (*ClaudeCodeVersion, error)
	// commandRunner is injected for testability. If nil, uses exec.Command.
	commandRunner func() (string, error)
	// cachedVersion stores the detected version to avoid repeated exec calls.
	cachedVersion *ClaudeCodeVersion
}

// NewClaudeCodeVersionChecker creates a new ClaudeCodeVersionChecker.
func NewClaudeCodeVersionChecker() ClaudeCodeVersionChecker {
	return &claudeCodeVersionChecker{
		versionParser: parseVersionString,
		commandRunner: runClaudeVersion,
	}
}

// NewClaudeCodeVersionCheckerWithParser creates a version checker with a custom
// version parser and command runner for testing.
func NewClaudeCodeVersionCheckerWithParser(
	parser func(string) (*ClaudeCodeVersion, error),
	runner func() (string, error),
) ClaudeCodeVersionChecker {
	return &claudeCodeVersionChecker{
		versionParser: parser,
		commandRunner: runner,
	}
}

// runClaudeVersion executes `claude --version` and returns the output.
func runClaudeVersion() (string, error) {
	cmd := exec.Command("claude", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running claude --version: %w", err)
	}
	return string(output), nil
}

// parseVersionString parses a semantic version string like "2.1.50" or "v2.1.50"
// and returns a ClaudeCodeVersion.
func parseVersionString(s string) (*ClaudeCodeVersion, error) {
	s = strings.TrimSpace(s)

	// Match semver pattern: optional 'v' prefix, then major.minor.patch.
	// After the patch number, only allow end of string or non-dot non-digit characters.
	// This rejects "2.1.50.1" but allows "2.1.50-beta" or "2.1.50 ".
	re := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:[^\d.].*)?$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid version string %q: expected format v?MAJOR.MINOR.PATCH", s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &ClaudeCodeVersion{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// DetectVersion runs `claude --version` and parses the output.
func (c *claudeCodeVersionChecker) DetectVersion() (*ClaudeCodeVersion, error) {
	if c.cachedVersion != nil {
		return c.cachedVersion, nil
	}

	output, err := c.commandRunner()
	if err != nil {
		return nil, fmt.Errorf("detecting Claude Code version: %w", err)
	}

	version, err := c.versionParser(output)
	if err != nil {
		return nil, fmt.Errorf("parsing version output %q: %w", output, err)
	}

	c.cachedVersion = version
	return version, nil
}

// CheckMinimumVersion returns an error if the detected version is less than
// the required minimum.
func (c *claudeCodeVersionChecker) CheckMinimumVersion(min ClaudeCodeVersion) error {
	detected, err := c.DetectVersion()
	if err != nil {
		return err
	}

	if detected.Compare(min) < 0 {
		return fmt.Errorf("Claude Code version %s is less than required minimum %s", detected, min)
	}

	return nil
}

// SupportsFeature returns true if the detected version meets the minimum version
// required for the named feature.
func (c *claudeCodeVersionChecker) SupportsFeature(feature string) (bool, error) {
	detected, err := c.DetectVersion()
	if err != nil {
		return false, err
	}

	minVersion, found := featureGates[feature]
	if !found {
		return false, fmt.Errorf("unknown feature %q", feature)
	}

	return detected.Compare(minVersion) >= 0, nil
}

// featureGates maps feature names to the minimum Claude Code version that supports them.
var featureGates = map[string]ClaudeCodeVersion{
	"agent_teams":        {Major: 2, Minor: 1, Patch: 32},
	"worktree_hooks":     {Major: 2, Minor: 1, Patch: 50},
	"worktree_isolation": {Major: 2, Minor: 1, Patch: 32},
	"1m_context":         {Major: 2, Minor: 1, Patch: 32},
	"config_change_hook": {Major: 2, Minor: 1, Patch: 49},
}
