package integration

import (
	"errors"
	"strings"
	"testing"
)

func TestParseVersionString_ValidFormats(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ClaudeCodeVersion
	}{
		{
			name:     "with v prefix",
			input:    "v2.1.50",
			expected: ClaudeCodeVersion{Major: 2, Minor: 1, Patch: 50},
		},
		{
			name:     "without v prefix",
			input:    "2.1.50",
			expected: ClaudeCodeVersion{Major: 2, Minor: 1, Patch: 50},
		},
		{
			name:     "with whitespace",
			input:    "  v2.1.32  \n",
			expected: ClaudeCodeVersion{Major: 2, Minor: 1, Patch: 32},
		},
		{
			name:     "major version 1",
			input:    "1.0.0",
			expected: ClaudeCodeVersion{Major: 1, Minor: 0, Patch: 0},
		},
		{
			name:     "large version numbers",
			input:    "99.123.456",
			expected: ClaudeCodeVersion{Major: 99, Minor: 123, Patch: 456},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := parseVersionString(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if version.Major != tt.expected.Major || version.Minor != tt.expected.Minor || version.Patch != tt.expected.Patch {
				t.Errorf("expected %+v, got %+v", tt.expected, version)
			}
		})
	}
}

func TestParseVersionString_InvalidFormats(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "no dots", input: "2150"},
		{name: "only major.minor", input: "2.1"},
		{name: "too many parts", input: "2.1.50.1"},
		{name: "non-numeric", input: "two.one.fifty"},
		{name: "mixed format", input: "v2.1.beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVersionString(tt.input)
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}
			if !strings.Contains(err.Error(), "invalid version string") {
				t.Errorf("expected error message to contain 'invalid version string', got: %v", err)
			}
		})
	}
}

func TestClaudeCodeVersion_String(t *testing.T) {
	tests := []struct {
		version  ClaudeCodeVersion
		expected string
	}{
		{ClaudeCodeVersion{2, 1, 50}, "2.1.50"},
		{ClaudeCodeVersion{1, 0, 0}, "1.0.0"},
		{ClaudeCodeVersion{99, 123, 456}, "99.123.456"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.version.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestClaudeCodeVersion_Compare(t *testing.T) {
	tests := []struct {
		name     string
		a        ClaudeCodeVersion
		b        ClaudeCodeVersion
		expected int
	}{
		{
			name:     "equal versions",
			a:        ClaudeCodeVersion{2, 1, 50},
			b:        ClaudeCodeVersion{2, 1, 50},
			expected: 0,
		},
		{
			name:     "a less than b by major",
			a:        ClaudeCodeVersion{1, 99, 99},
			b:        ClaudeCodeVersion{2, 0, 0},
			expected: -1,
		},
		{
			name:     "a greater than b by major",
			a:        ClaudeCodeVersion{3, 0, 0},
			b:        ClaudeCodeVersion{2, 99, 99},
			expected: 1,
		},
		{
			name:     "a less than b by minor",
			a:        ClaudeCodeVersion{2, 1, 99},
			b:        ClaudeCodeVersion{2, 2, 0},
			expected: -1,
		},
		{
			name:     "a greater than b by minor",
			a:        ClaudeCodeVersion{2, 2, 0},
			b:        ClaudeCodeVersion{2, 1, 99},
			expected: 1,
		},
		{
			name:     "a less than b by patch",
			a:        ClaudeCodeVersion{2, 1, 49},
			b:        ClaudeCodeVersion{2, 1, 50},
			expected: -1,
		},
		{
			name:     "a greater than b by patch",
			a:        ClaudeCodeVersion{2, 1, 51},
			b:        ClaudeCodeVersion{2, 1, 50},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Compare(tt.b)
			if result != tt.expected {
				t.Errorf("Compare(%v, %v) = %d, expected %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestClaudeCodeVersionChecker_DetectVersion(t *testing.T) {
	tests := []struct {
		name          string
		commandOutput string
		commandError  error
		expectError   bool
		expectedVer   *ClaudeCodeVersion
	}{
		{
			name:          "valid version with v prefix",
			commandOutput: "v2.1.50",
			expectedVer:   &ClaudeCodeVersion{Major: 2, Minor: 1, Patch: 50},
		},
		{
			name:          "valid version without v prefix",
			commandOutput: "2.1.32",
			expectedVer:   &ClaudeCodeVersion{Major: 2, Minor: 1, Patch: 32},
		},
		{
			name:         "command not found",
			commandError: errors.New("exec: \"claude\": executable file not found in $PATH"),
			expectError:  true,
		},
		{
			name:          "unparseable output",
			commandOutput: "Claude Code version 2-1-50",
			expectError:   true,
		},
		{
			name:          "empty output",
			commandOutput: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock command runner.
			runner := func() (string, error) {
				if tt.commandError != nil {
					return "", tt.commandError
				}
				return tt.commandOutput, nil
			}

			checker := NewClaudeCodeVersionCheckerWithParser(parseVersionString, runner)

			version, err := checker.DetectVersion()

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if version.Major != tt.expectedVer.Major || version.Minor != tt.expectedVer.Minor || version.Patch != tt.expectedVer.Patch {
				t.Errorf("expected version %+v, got %+v", tt.expectedVer, version)
			}
		})
	}
}

func TestClaudeCodeVersionChecker_DetectVersion_Caching(t *testing.T) {
	callCount := 0
	runner := func() (string, error) {
		callCount++
		return "2.1.50", nil
	}

	checker := NewClaudeCodeVersionCheckerWithParser(parseVersionString, runner)

	// First call should execute the runner.
	_, err := checker.DetectVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected runner to be called once, called %d times", callCount)
	}

	// Second call should use cached result.
	_, err = checker.DetectVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected runner to be called once (cached), called %d times", callCount)
	}
}

func TestClaudeCodeVersionChecker_CheckMinimumVersion(t *testing.T) {
	tests := []struct {
		name        string
		detected    ClaudeCodeVersion
		minimum     ClaudeCodeVersion
		expectError bool
	}{
		{
			name:        "detected equals minimum",
			detected:    ClaudeCodeVersion{2, 1, 50},
			minimum:     ClaudeCodeVersion{2, 1, 50},
			expectError: false,
		},
		{
			name:        "detected greater than minimum",
			detected:    ClaudeCodeVersion{2, 2, 0},
			minimum:     ClaudeCodeVersion{2, 1, 50},
			expectError: false,
		},
		{
			name:        "detected less than minimum by patch",
			detected:    ClaudeCodeVersion{2, 1, 49},
			minimum:     ClaudeCodeVersion{2, 1, 50},
			expectError: true,
		},
		{
			name:        "detected less than minimum by minor",
			detected:    ClaudeCodeVersion{2, 0, 99},
			minimum:     ClaudeCodeVersion{2, 1, 0},
			expectError: true,
		},
		{
			name:        "detected less than minimum by major",
			detected:    ClaudeCodeVersion{1, 99, 99},
			minimum:     ClaudeCodeVersion{2, 0, 0},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func() (string, error) {
				return tt.detected.String(), nil
			}

			checker := NewClaudeCodeVersionCheckerWithParser(parseVersionString, runner)

			err := checker.CheckMinimumVersion(tt.minimum)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "less than required minimum") {
					t.Errorf("expected error message to contain 'less than required minimum', got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClaudeCodeVersionChecker_SupportsFeature(t *testing.T) {
	tests := []struct {
		name          string
		detected      ClaudeCodeVersion
		feature       string
		expectSupport bool
		expectError   bool
	}{
		{
			name:          "worktree_hooks supported (exact match)",
			detected:      ClaudeCodeVersion{2, 1, 50},
			feature:       "worktree_hooks",
			expectSupport: true,
		},
		{
			name:          "worktree_hooks supported (newer version)",
			detected:      ClaudeCodeVersion{2, 2, 0},
			feature:       "worktree_hooks",
			expectSupport: true,
		},
		{
			name:          "worktree_hooks not supported (older version)",
			detected:      ClaudeCodeVersion{2, 1, 49},
			feature:       "worktree_hooks",
			expectSupport: false,
		},
		{
			name:          "agent_teams supported",
			detected:      ClaudeCodeVersion{2, 1, 32},
			feature:       "agent_teams",
			expectSupport: true,
		},
		{
			name:          "agent_teams not supported",
			detected:      ClaudeCodeVersion{2, 1, 31},
			feature:       "agent_teams",
			expectSupport: false,
		},
		{
			name:          "1m_context supported",
			detected:      ClaudeCodeVersion{2, 1, 32},
			feature:       "1m_context",
			expectSupport: true,
		},
		{
			name:          "config_change_hook supported",
			detected:      ClaudeCodeVersion{2, 1, 49},
			feature:       "config_change_hook",
			expectSupport: true,
		},
		{
			name:          "config_change_hook not supported",
			detected:      ClaudeCodeVersion{2, 1, 48},
			feature:       "config_change_hook",
			expectSupport: false,
		},
		{
			name:        "unknown feature",
			detected:    ClaudeCodeVersion{2, 1, 50},
			feature:     "nonexistent_feature",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func() (string, error) {
				return tt.detected.String(), nil
			}

			checker := NewClaudeCodeVersionCheckerWithParser(parseVersionString, runner)

			supported, err := checker.SupportsFeature(tt.feature)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unknown feature") {
					t.Errorf("expected error message to contain 'unknown feature', got: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if supported != tt.expectSupport {
				t.Errorf("expected SupportsFeature(%q) = %v, got %v", tt.feature, tt.expectSupport, supported)
			}
		})
	}
}

func TestFeatureGates_AllDefined(t *testing.T) {
	// Verify all expected features are defined in the feature gates map.
	requiredFeatures := []string{
		"agent_teams",
		"worktree_hooks",
		"worktree_isolation",
		"1m_context",
		"config_change_hook",
	}

	for _, feature := range requiredFeatures {
		t.Run(feature, func(t *testing.T) {
			_, found := featureGates[feature]
			if !found {
				t.Errorf("required feature %q not found in featureGates map", feature)
			}
		})
	}
}
