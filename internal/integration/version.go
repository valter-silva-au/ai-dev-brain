package integration

import (
	"fmt"
	"regexp"
	"strconv"
)

// SemanticVersion represents a parsed semantic version
type SemanticVersion struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Metadata   string
}

// String returns the string representation of the version
func (v SemanticVersion) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Metadata != "" {
		s += "+" + v.Metadata
	}
	return s
}

// Compare compares two semantic versions
// Returns: -1 if v < other, 0 if v == other, 1 if v > other
func (v SemanticVersion) Compare(other SemanticVersion) int {
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
	// Prerelease comparison: version with prerelease < version without
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != other.Prerelease {
		if v.Prerelease < other.Prerelease {
			return -1
		}
		return 1
	}
	return 0
}

// ParseSemanticVersion parses a semantic version string using regex
// Supports formats: X.Y.Z, X.Y.Z-prerelease, X.Y.Z+metadata, X.Y.Z-prerelease+metadata
func ParseSemanticVersion(version string) (SemanticVersion, error) {
	// Regex pattern for semantic versioning
	// Matches: major.minor.patch[-prerelease][+metadata]
	pattern := `^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z\-\.]+))?(?:\+([0-9A-Za-z\-\.]+))?$`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(version)
	if matches == nil {
		return SemanticVersion{}, fmt.Errorf("invalid semantic version format: %s", version)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return SemanticVersion{}, fmt.Errorf("invalid major version: %w", err)
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return SemanticVersion{}, fmt.Errorf("invalid minor version: %w", err)
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return SemanticVersion{}, fmt.Errorf("invalid patch version: %w", err)
	}

	return SemanticVersion{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Metadata:   matches[5],
	}, nil
}

// FeatureGates manages feature flags based on versions or configurations
type FeatureGates struct {
	gates map[string]bool
}

// NewFeatureGates creates a new feature gates manager
func NewFeatureGates() *FeatureGates {
	return &FeatureGates{
		gates: make(map[string]bool),
	}
}

// Enable enables a feature gate
func (fg *FeatureGates) Enable(feature string) {
	fg.gates[feature] = true
}

// Disable disables a feature gate
func (fg *FeatureGates) Disable(feature string) {
	fg.gates[feature] = false
}

// IsEnabled checks if a feature gate is enabled
func (fg *FeatureGates) IsEnabled(feature string) bool {
	enabled, exists := fg.gates[feature]
	return exists && enabled
}

// SetAll sets multiple feature gates at once
func (fg *FeatureGates) SetAll(gates map[string]bool) {
	for feature, enabled := range gates {
		fg.gates[feature] = enabled
	}
}

// GetAll returns all feature gates
func (fg *FeatureGates) GetAll() map[string]bool {
	result := make(map[string]bool, len(fg.gates))
	for k, v := range fg.gates {
		result[k] = v
	}
	return result
}
