package version

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		major      uint64
		minor      uint64
		patch      uint64
		prerelease string
		metadata   string
		invalid    bool
	}{
		{
			name:  "simple version",
			input: "1.2.3",
			major: 1,
			minor: 2,
			patch: 3,
		},
		{
			name:  "version with v prefix",
			input: "v1.2.3",
			major: 1,
			minor: 2,
			patch: 3,
		},
		{
			name:       "version with prerelease",
			input:      "v1.2.3-beta.1",
			major:      1,
			minor:      2,
			patch:      3,
			prerelease: "beta.1",
		},
		{
			name:     "version with build metadata",
			input:    "v1.2.3+build.123",
			major:    1,
			minor:    2,
			patch:    3,
			metadata: "build.123",
		},
		{
			name:       "version with prerelease and build",
			input:      "v1.2.3-alpha.1+build.456",
			major:      1,
			minor:      2,
			patch:      3,
			prerelease: "alpha.1",
			metadata:   "build.456",
		},
		{
			name:    "invalid version - text only",
			input:   "unknown",
			invalid: true,
		},
		{
			name:    "invalid version - partial",
			input:   "1.2",
			invalid: true,
		},
		{
			name:    "invalid version - empty",
			input:   "",
			invalid: true,
		},
		{
			// 严格模式按 semver §9 拒绝数字标识符的前导零
			name:    "invalid version - leading zero in prerelease",
			input:   "v1.0.0-rc.09",
			invalid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseVersion(tt.input)
			if tt.invalid {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			assert.Equal(t, tt.major, result.Major())
			assert.Equal(t, tt.minor, result.Minor())
			assert.Equal(t, tt.patch, result.Patch())
			assert.Equal(t, tt.prerelease, result.Prerelease())
			assert.Equal(t, tt.metadata, result.Metadata())
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
			// semver §11.4：数字标识符按数值比较，不能按字典序（"9" > "3"）
			name:     "prerelease numeric: rc.9 < rc.35",
			a:        "1.0.0-rc.9",
			b:        "1.0.0-rc.35",
			expected: -1,
		},
		{
			name:     "prerelease numeric: rc.35 > rc.9",
			a:        "1.0.0-rc.35",
			b:        "1.0.0-rc.9",
			expected: 1,
		},
		{
			name:     "prerelease numeric multi-digit: rc.10 > rc.9",
			a:        "1.0.0-rc.10",
			b:        "1.0.0-rc.9",
			expected: 1,
		},
		{
			name:     "prerelease numeric multi-digit: rc.2 < rc.10",
			a:        "1.0.0-rc.2",
			b:        "1.0.0-rc.10",
			expected: -1,
		},
		{
			name:     "prerelease beta numeric: beta.2 < beta.10",
			a:        "1.0.0-beta.2",
			b:        "1.0.0-beta.10",
			expected: -1,
		},
		{
			// semver §11.4：数字标识符优先级低于字母数字标识符
			name:     "prerelease numeric lower than alphanumeric",
			a:        "1.0.0-1",
			b:        "1.0.0-alpha",
			expected: -1,
		},
		{
			// semver §11.4：前面都相同时，字段数多者更大
			name:     "prerelease more fields wins: rc.1.1 > rc.1",
			a:        "1.0.0-rc.1.1",
			b:        "1.0.0-rc.1",
			expected: 1,
		},
		{
			// 预览版升正式版：rc 号再大也低于同号的正式版（tag 带 v 前缀）
			name:     "rc.35 < release: v1.0.0-rc.35 < v1.0.0",
			a:        "v1.0.0-rc.35",
			b:        "v1.0.0",
			expected: -1,
		},
		{
			// 先比通道名，后面的数字再大也越不过去（"beta" < "rc"）
			name:     "channel before number: beta.10 < rc.1",
			a:        "1.0.0-beta.10",
			b:        "1.0.0-rc.1",
			expected: -1,
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

	newReleases := checker.filterNewReleases(releases, false)

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

	newReleases := checker.filterNewReleases(releases, false)
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

	newReleases := checker.filterNewReleases(releases, false)

	require.Len(t, newReleases, 3)
	assert.Equal(t, "v3.0.0", newReleases[0].Version)
	assert.Equal(t, "v2.0.0", newReleases[1].Version)
	assert.Equal(t, "v1.1.0", newReleases[2].Version)
}

func TestFilterNewReleases_IncludePrerelease(t *testing.T) {
	checker := NewChecker()
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{TagName: "v2.0.0", Name: "Release", HTMLURL: "u"},
		{TagName: "v2.1.0-beta.1", Name: "Beta", HTMLURL: "u", Prerelease: true},
		{TagName: "v2.2.0-rc.2", Name: "RC", HTMLURL: "u", Prerelease: true},
		{TagName: "draft", Name: "Draft", HTMLURL: "u", Draft: true},
	}

	includePre := checker.filterNewReleases(releases, true)
	require.Len(t, includePre, 3)
	assert.Equal(t, "v2.2.0-rc.2", includePre[0].Version)
	assert.True(t, includePre[0].Prerelease)
	assert.Equal(t, "rc", includePre[0].PrereleaseLabel)
	assert.Equal(t, "v2.1.0-beta.1", includePre[1].Version)
	assert.True(t, includePre[1].Prerelease)
	assert.Equal(t, "beta", includePre[1].PrereleaseLabel)
	assert.Equal(t, "v2.0.0", includePre[2].Version)
	assert.False(t, includePre[2].Prerelease)
	assert.Empty(t, includePre[2].PrereleaseLabel)

	excludePre := checker.filterNewReleases(releases, false)
	require.Len(t, excludePre, 1)
	assert.Equal(t, "v2.0.0", excludePre[0].Version)
}

// 回归：当前版本 v1.0.0-rc.35 时，rc.9 / rc.10 这类「字典序更大但版本更老」的预发版
// 不能被当成新版本；rc.36 / rc.100 才是，且顺序必须按版本号而不是字典序。
func TestFilterNewReleases_PrereleaseNumericOrder(t *testing.T) {
	checker := NewChecker()
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0-rc.35"

	releases := []GitHubRelease{
		{TagName: "v1.0.0-rc.9", Name: "old", HTMLURL: "u", Prerelease: true},
		{TagName: "v1.0.0-rc.10", Name: "old", HTMLURL: "u", Prerelease: true},
		{TagName: "v1.0.0-rc.35", Name: "current", HTMLURL: "u", Prerelease: true},
		{TagName: "v1.0.0-rc.36", Name: "new", HTMLURL: "u", Prerelease: true},
		{TagName: "v1.0.0-rc.100", Name: "newest", HTMLURL: "u", Prerelease: true},
	}

	got := checker.filterNewReleases(releases, true)

	versions := make([]string, 0, len(got))
	for _, r := range got {
		versions = append(versions, r.Version)
	}
	assert.Equal(t, []string{"v1.0.0-rc.100", "v1.0.0-rc.36"}, versions,
		"只有 rc.36 与 rc.100 比 rc.35 新；rc.9/rc.10 虽字典序更大但版本更老")
}

func TestFilterNewReleases_TagSuffixDetection(t *testing.T) {
	checker := NewChecker()
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{TagName: "v2.0.0-beta.1", Name: "Beta (flag missing)", HTMLURL: "u", Prerelease: false},
	}

	got := checker.filterNewReleases(releases, true)
	require.Len(t, got, 1)
	assert.True(t, got[0].Prerelease, "tag -beta.1 suffix 应识别为预发版，即使 GitHub prerelease 勾选被漏掉")
	assert.Equal(t, "beta", got[0].PrereleaseLabel)
}

func TestExtractPrereleaseLabel(t *testing.T) {
	cases := []struct {
		in  string
		out string
	}{
		{"", ""},
		{"beta", "beta"},
		{"beta.1", "beta"},
		{"BETA.2", "beta"},
		{"rc.1", "rc"},
		{"alpha", "alpha"},
		{"dev.1", ""},
		{"snapshot", ""},
		{"foo.bar", ""},
	}
	for _, c := range cases {
		assert.Equal(t, c.out, extractPrereleaseLabel(c.in), "input=%q", c.in)
	}
}

// TestFilterNewReleases_WithAssets covers the asset-copy loop inside
// filterNewReleases (previously-uncovered lines building ReleaseAsset slices).
func TestFilterNewReleases_WithAssets(t *testing.T) {
	c := NewChecker()
	oldVersion := Version
	defer func() { Version = oldVersion }()
	Version = "v1.0.0"

	releases := []GitHubRelease{
		{
			TagName: "v2.0.0", Name: "Release 2", HTMLURL: "u",
			Assets: []GitHubAsset{
				{Name: "pt-tools-linux-amd64.tar.gz", DownloadURL: "https://x/a", Size: 111},
				{Name: "pt-tools-windows-amd64.exe.zip", DownloadURL: "https://x/b", Size: 222},
			},
		},
	}
	got := c.filterNewReleases(releases, false)
	require.Len(t, got, 1)
	require.Len(t, got[0].Assets, 2)
	assert.Equal(t, "pt-tools-linux-amd64.tar.gz", got[0].Assets[0].Name)
	assert.Equal(t, int64(111), got[0].Assets[0].Size)
	assert.Equal(t, "https://x/b", got[0].Assets[1].DownloadURL)
}

// TestCheckForUpdates_ForceError drives CheckForUpdates with Force=true through
// the fetch path; the unreachable proxy guarantees an error is recorded,
// exercising the error-return branch after the cache short-circuit checks.
func TestCheckForUpdates_ForceError(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v0.0.1"}
	c.lastCheck = time.Now()

	res, err := c.CheckForUpdates(context.Background(), CheckOptions{
		Force:    true,
		ProxyURL: "http://127.0.0.1:1",
	})
	require.Error(t, err)
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Error)
	assert.Equal(t, Version, res.CurrentVersion)
}

// TestCheckForUpdates_PrereleaseChangeBypassesCache exercises the
// prerelChanged branch of the cache guard.
func TestCheckForUpdates_PrereleaseChangeBypassesCache(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v0.0.1"}
	c.lastCheck = time.Now()
	c.lastProxy = "http://127.0.0.1:1"
	c.lastIncludePrerel = false

	res, _ := c.CheckForUpdates(context.Background(), CheckOptions{
		ProxyURL:          "http://127.0.0.1:1",
		IncludePrerelease: true,
	})
	require.NotNil(t, res)
	assert.NotEmpty(t, res.Error, "prerelease flag change must bypass cache and attempt fetch")
}

func TestCheckForUpdatesReturnsCachedResult(t *testing.T) {
	c := NewChecker()
	cached := &VersionCheckResult{CurrentVersion: "v1.2.3", HasUpdate: false}
	c.lastResult = cached
	c.lastCheck = time.Now()
	c.lastProxy = ""
	c.lastIncludePrerel = false

	got, err := c.CheckForUpdates(context.Background(), CheckOptions{})
	require.NoError(t, err)
	assert.Same(t, cached, got, "fresh cache with matching opts must short-circuit without network")
}

func TestCheckForUpdatesProxyChangeBypassesCache(t *testing.T) {
	c := NewChecker()
	c.lastResult = &VersionCheckResult{CurrentVersion: "v1.0.0"}
	c.lastCheck = time.Now()
	c.lastProxy = ""

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, _ := c.CheckForUpdates(ctx, CheckOptions{ProxyURL: "http://127.0.0.1:1"})
	require.NotNil(t, res)
	assert.Equal(t, Version, res.CurrentVersion)
	assert.NotEmpty(t, res.Error, "unreachable proxy must record an error, proving cache bypass")
}
