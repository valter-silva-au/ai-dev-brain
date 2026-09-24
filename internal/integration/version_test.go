package integration

import (
	"testing"
)

func TestParseSemanticVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    SemanticVersion
		expectError bool
	}{
		{
			name:  "basic version",
			input: "1.2.3",
			expected: SemanticVersion{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
			expectError: false,
		},
		{
			name:  "version with v prefix",
			input: "v2.0.1",
			expected: SemanticVersion{
				Major: 2,
				Minor: 0,
				Patch: 1,
			},
			expectError: false,
		},
		{
			name:  "version with prerelease",
			input: "1.0.0-alpha",
			expected: SemanticVersion{
				Major:      1,
				Minor:      0,
				Patch:      0,
				Prerelease: "alpha",
			},
			expectError: false,
		},
		{
			name:  "version with metadata",
			input: "1.0.0+build.123",
			expected: SemanticVersion{
				Major:    1,
				Minor:    0,
				Patch:    0,
				Metadata: "build.123",
			},
			expectError: false,
		},
		{
			name:  "version with prerelease and metadata",
			input: "2.1.0-beta.1+exp.sha.5114f85",
			expected: SemanticVersion{
				Major:      2,
				Minor:      1,
				Patch:      0,
				Prerelease: "beta.1",
				Metadata:   "exp.sha.5114f85",
			},
			expectError: false,
		},
		{
			name:        "invalid version - missing patch",
			input:       "1.2",
			expectError: true,
		},
		{
			name:        "invalid version - non-numeric",
			input:       "a.b.c",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSemanticVersion(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Major != tt.expected.Major {
				t.Errorf("Major: expected %d, got %d", tt.expected.Major, result.Major)
			}
			if result.Minor != tt.expected.Minor {
				t.Errorf("Minor: expected %d, got %d", tt.expected.Minor, result.Minor)
			}
			if result.Patch != tt.expected.Patch {
				t.Errorf("Patch: expected %d, got %d", tt.expected.Patch, result.Patch)
			}
			if result.Prerelease != tt.expected.Prerelease {
				t.Errorf("Prerelease: expected %s, got %s", tt.expected.Prerelease, result.Prerelease)
			}
			if result.Metadata != tt.expected.Metadata {
				t.Errorf("Metadata: expected %s, got %s", tt.expected.Metadata, result.Metadata)
			}
		})
	}
}

func TestSemanticVersionString(t *testing.T) {
	tests := []struct {
		name     string
		version  SemanticVersion
		expected string
	}{
		{
			name:     "basic version",
			version:  SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			expected: "1.2.3",
		},
		{
			name:     "version with prerelease",
			version:  SemanticVersion{Major: 1, Minor: 0, Patch: 0, Prerelease: "alpha"},
			expected: "1.0.0-alpha",
		},
		{
			name:     "version with metadata",
			version:  SemanticVersion{Major: 2, Minor: 1, Patch: 0, Metadata: "build.123"},
			expected: "2.1.0+build.123",
		},
		{
			name:     "version with prerelease and metadata",
			version:  SemanticVersion{Major: 3, Minor: 0, Patch: 0, Prerelease: "rc.1", Metadata: "20230101"},
			expected: "3.0.0-rc.1+20230101",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.version.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSemanticVersionCompare(t *testing.T) {
	tests := []struct {
		name     string
		v1       SemanticVersion
		v2       SemanticVersion
		expected int
	}{
		{
			name:     "equal versions",
			v1:       SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			v2:       SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			expected: 0,
		},
		{
			name:     "v1 major > v2",
			v1:       SemanticVersion{Major: 2, Minor: 0, Patch: 0},
			v2:       SemanticVersion{Major: 1, Minor: 9, Patch: 9},
			expected: 1,
		},
		{
			name:     "v1 major < v2",
			v1:       SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			v2:       SemanticVersion{Major: 2, Minor: 0, Patch: 0},
			expected: -1,
		},
		{
			name:     "v1 minor > v2",
			v1:       SemanticVersion{Major: 1, Minor: 5, Patch: 0},
			v2:       SemanticVersion{Major: 1, Minor: 2, Patch: 9},
			expected: 1,
		},
		{
			name:     "v1 patch > v2",
			v1:       SemanticVersion{Major: 1, Minor: 2, Patch: 5},
			v2:       SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			expected: 1,
		},
		{
			name:     "release > prerelease",
			v1:       SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			v2:       SemanticVersion{Major: 1, Minor: 0, Patch: 0, Prerelease: "alpha"},
			expected: 1,
		},
		{
			name:     "prerelease < release",
			v1:       SemanticVersion{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta"},
			v2:       SemanticVersion{Major: 1, Minor: 0, Patch: 0},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.v1.Compare(tt.v2)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestFeatureGates(t *testing.T) {
	fg := NewFeatureGates()

	// Test initial state
	if fg.IsEnabled("feature1") {
		t.Error("feature1 should not be enabled initially")
	}

	// Test Enable
	fg.Enable("feature1")
	if !fg.IsEnabled("feature1") {
		t.Error("feature1 should be enabled after Enable()")
	}

	// Test Disable
	fg.Disable("feature1")
	if fg.IsEnabled("feature1") {
		t.Error("feature1 should be disabled after Disable()")
	}

	// Test SetAll
	gates := map[string]bool{
		"feature2": true,
		"feature3": false,
		"feature4": true,
	}
	fg.SetAll(gates)

	if !fg.IsEnabled("feature2") {
		t.Error("feature2 should be enabled")
	}
	if fg.IsEnabled("feature3") {
		t.Error("feature3 should be disabled")
	}
	if !fg.IsEnabled("feature4") {
		t.Error("feature4 should be enabled")
	}

	// Test GetAll
	allGates := fg.GetAll()
	if len(allGates) != 4 { // feature1 (disabled), feature2, feature3, feature4
		t.Errorf("expected 4 gates, got %d", len(allGates))
	}
}
