package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *SemVer
	}{
		{
			name:  "simple version",
			input: "1.2.3",
			expected: &SemVer{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
		},
		{
			name:  "version with v prefix",
			input: "v1.2.3",
			expected: &SemVer{
				Major: 1,
				Minor: 2,
				Patch: 3,
			},
		},
		{
			name:  "version with prerelease",
			input: "v1.2.3-beta.1",
			expected: &SemVer{
				Major:      1,
				Minor:      2,
				Patch:      3,
				Prerelease: "beta.1",
			},
		},
		{
			name:  "version with build metadata",
			input: "v1.2.3+build.123",
			expected: &SemVer{
				Major: 1,
				Minor: 2,
				Patch: 3,
				Build: "build.123",
			},
		},
		{
			name:  "version with prerelease and build",
			input: "v1.2.3-alpha.1+build.456",
			expected: &SemVer{
				Major:      1,
				Minor:      2,
				Patch:      3,
				Prerelease: "alpha.1",
				Build:      "build.456",
			},
		},
		{
			name:     "invalid version - text only",
			input:    "unknown",
			expected: nil,
		},
		{
			name:     "invalid version - partial",
			input:    "1.2",
			expected: nil,
		},
		{
			name:     "invalid version - empty",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseVersion(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Major, result.Major)
				assert.Equal(t, tt.expected.Minor, result.Minor)
				assert.Equal(t, tt.expected.Patch, result.Patch)
				assert.Equal(t, tt.expected.Prerelease, result.Prerelease)
				assert.Equal(t, tt.expected.Build, result.Build)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{
			name:     "a > b - major",
			a:        "2.0.0",
			b:        "1.0.0",
			expected: 1,
		},
		{
			name:     "a < b - major",
			a:        "1.0.0",
			b:        "2.0.0",
			expected: -1,
		},
		{
			name:     "a > b - minor",
			a:        "1.2.0",
			b:        "1.1.0",
			expected: 1,
		},
		{
			name:     "a < b - minor",
			a:        "1.1.0",
			b:        "1.2.0",
			expected: -1,
		},
		{
			name:     "a > b - patch",
			a:        "1.0.2",
			b:        "1.0.1",
			expected: 1,
		},
		{
			name:     "a < b - patch",
			a:        "1.0.1",
			b:        "1.0.2",
			expected: -1,
		},
		{
			name:     "equal versions",
			a:        "1.2.3",
			b:        "1.2.3",
			expected: 0,
		},
		{
			name:     "release > prerelease",
			a:        "1.0.0",
			b:        "1.0.0-beta",
			expected: 1,
		},
		{
			name:     "prerelease < release",
			a:        "1.0.0-beta",
			b:        "1.0.0",
			expected: -1,
		},
		{
			name:     "prerelease comparison",
			a:        "1.0.0-beta",
			b:        "1.0.0-alpha",
			expected: 1,
		},
		{
			name:     "with v prefix",
			a:        "v2.0.0",
			b:        "v1.0.0",
			expected: 1,
		},
		{
			name:     "nil versions",
			a:        "invalid",
			b:        "1.0.0",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			av := ParseVersion(tt.a)
			bv := ParseVersion(tt.b)
			result := CompareVersions(av, bv)

			if tt.expected > 0 {
				assert.Greater(t, result, 0, "expected a > b")
			} else if tt.expected < 0 {
				assert.Less(t, result, 0, "expected a < b")
			} else {
				assert.Equal(t, 0, result, "expected a == b")
			}
		})
	}
}

func TestGetVersionInfo(t *testing.T) {
	info := GetVersionInfo()
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.BuildTime)
	assert.NotEmpty(t, info.CommitID)
}

func TestNewChecker(t *testing.T) {
	checker := NewChecker()
	require.NotNil(t, checker)
}

func TestGetChecker(t *testing.T) {
	checker1 := GetChecker()
	checker2 := GetChecker()
	assert.Same(t, checker1, checker2, "GetChecker should return singleton")
}

func TestChecker_ShouldCheck(t *testing.T) {
	checker := NewChecker()
	assert.True(t, checker.ShouldCheck(), "should check when no previous check")
}

func TestChecker_GetCachedResult(t *testing.T) {
	checker := NewChecker()
	result := checker.GetCachedResult()
	assert.Nil(t, result, "should return nil when no cached result")
}

func TestFilterNewReleases(t *testing.T) {
	checker := NewChecker()

	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{TagName: "v2.0.0", Name: "Version 2.0.0", HTMLURL: "https://github.com/test/v2.0.0"},
		{TagName: "v1.5.0", Name: "Version 1.5.0", HTMLURL: "https://github.com/test/v1.5.0"},
		{TagName: "v1.0.0", Name: "Version 1.0.0", HTMLURL: "https://github.com/test/v1.0.0"},
		{TagName: "v0.9.0", Name: "Version 0.9.0", HTMLURL: "https://github.com/test/v0.9.0"},
		{TagName: "v2.1.0-beta", Name: "Version 2.1.0 Beta", HTMLURL: "https://github.com/test/v2.1.0-beta", Prerelease: true},
		{TagName: "draft-release", Name: "Draft", HTMLURL: "https://github.com/test/draft", Draft: true},
	}

	newReleases := checker.filterNewReleases(releases)

	assert.Len(t, newReleases, 2, "should filter to only newer non-draft non-prerelease versions")
	assert.Equal(t, "v2.0.0", newReleases[0].Version)
	assert.Equal(t, "v1.5.0", newReleases[1].Version)
}

func TestFilterNewReleases_InvalidCurrentVersion(t *testing.T) {
	checker := NewChecker()

	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "unknown"

	releases := []GitHubRelease{
		{TagName: "v1.0.0", Name: "Version 1.0.0"},
	}

	newReleases := checker.filterNewReleases(releases)
	assert.Nil(t, newReleases, "should return nil when current version is invalid")
}

func TestFilterNewReleases_SortedByVersion(t *testing.T) {
	checker := NewChecker()

	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{TagName: "v1.1.0", Name: "Minor update"},
		{TagName: "v3.0.0", Name: "Major update"},
		{TagName: "v2.0.0", Name: "Another major"},
	}

	newReleases := checker.filterNewReleases(releases)

	require.Len(t, newReleases, 3)
	assert.Equal(t, "v3.0.0", newReleases[0].Version)
	assert.Equal(t, "v2.0.0", newReleases[1].Version)
	assert.Equal(t, "v1.1.0", newReleases[2].Version)
}
