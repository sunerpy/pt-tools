package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/requests"
)

const (
	GitHubRepoOwner    = "sunerpy"
	GitHubRepoName     = "pt-tools"
	GitHubReleasesURL  = "https://api.github.com/repos/" + GitHubRepoOwner + "/" + GitHubRepoName + "/releases"
	GitHubChangelogURL = "https://github.com/" + GitHubRepoOwner + "/" + GitHubRepoName + "/blob/main/CHANGELOG.md"
	MaxDisplayReleases = 3
	CheckInterval      = 24 * time.Hour
	RequestTimeout     = 15 * time.Second
)

type GitHubAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt time.Time     `json:"published_at"`
	Prerelease  bool          `json:"prerelease"`
	Draft       bool          `json:"draft"`
	Assets      []GitHubAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Size        int64  `json:"size"`
}

type ReleaseInfo struct {
	Version     string         `json:"version"`
	Name        string         `json:"name"`
	Changelog   string         `json:"changelog"`
	URL         string         `json:"url"`
	PublishedAt int64          `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets,omitempty"`
}

type VersionCheckResult struct {
	CurrentVersion  string        `json:"current_version"`
	HasUpdate       bool          `json:"has_update"`
	NewReleases     []ReleaseInfo `json:"new_releases,omitempty"`
	ChangelogURL    string        `json:"changelog_url,omitempty"`
	HasMoreReleases bool          `json:"has_more_releases,omitempty"`
	CheckedAt       int64         `json:"checked_at"`
	Error           string        `json:"error,omitempty"`
}

type VersionInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	CommitID  string `json:"commit_id"`
}

type CheckOptions struct {
	Force    bool
	ProxyURL string
}

type Checker struct {
	mu         sync.RWMutex
	lastCheck  time.Time
	lastResult *VersionCheckResult
	lastProxy  string
}

var (
	defaultChecker *Checker
	checkerOnce    sync.Once
	semverRegex    = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
)

func GetChecker() *Checker {
	checkerOnce.Do(func() {
		defaultChecker = NewChecker()
	})
	return defaultChecker
}

func NewChecker() *Checker {
	return &Checker{}
}

func GetVersionInfo() VersionInfo {
	return VersionInfo{
		Version:   Version,
		BuildTime: BuildTime,
		CommitID:  CommitID,
	}
}

func (c *Checker) CheckForUpdates(ctx context.Context, opts CheckOptions) (*VersionCheckResult, error) {
	c.mu.RLock()
	proxyChanged := opts.ProxyURL != c.lastProxy
	if !opts.Force && !proxyChanged && c.lastResult != nil && time.Since(c.lastCheck) < CheckInterval {
		result := c.lastResult
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	proxyChanged = opts.ProxyURL != c.lastProxy
	if !opts.Force && !proxyChanged && c.lastResult != nil && time.Since(c.lastCheck) < CheckInterval {
		return c.lastResult, nil
	}

	result := &VersionCheckResult{
		CurrentVersion: Version,
		CheckedAt:      time.Now().Unix(),
		ChangelogURL:   GitHubChangelogURL,
	}

	releases, err := c.fetchGitHubReleases(ctx, opts.ProxyURL)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	newReleases := c.filterNewReleases(releases)
	if len(newReleases) > 0 {
		result.HasUpdate = true
		if len(newReleases) > MaxDisplayReleases {
			result.NewReleases = newReleases[:MaxDisplayReleases]
			result.HasMoreReleases = true
		} else {
			result.NewReleases = newReleases
		}
	}

	c.lastResult = result
	c.lastCheck = time.Now()
	c.lastProxy = opts.ProxyURL
	return result, nil
}

func (c *Checker) GetCachedResult() *VersionCheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastResult
}

func (c *Checker) ShouldCheck() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastResult == nil || time.Since(c.lastCheck) >= CheckInterval
}

func (c *Checker) fetchGitHubReleases(ctx context.Context, proxyURL string) ([]GitHubRelease, error) {
	req, err := requests.NewGet(GitHubReleasesURL).
		WithContext(ctx).
		WithHeader("Accept", "application/vnd.github.v3+json").
		WithHeader("User-Agent", "pt-tools/"+Version).
		Build()
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	var resp *requests.Response
	if proxyURL != "" {
		session := requests.NewSession().
			WithProxy(proxyURL).
			WithTimeout(RequestTimeout)
		defer session.Close()
		resp, err = session.Do(req)
	} else {
		session := requests.NewSession().WithTimeout(RequestTimeout)
		defer session.Close()
		resp, err = session.Do(req)
	}
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 返回错误: %d", resp.StatusCode)
	}

	var releases []GitHubRelease
	if err := json.Unmarshal(resp.Bytes(), &releases); err != nil {
		return nil, fmt.Errorf("解析 GitHub 响应失败: %w", err)
	}

	return releases, nil
}

func (c *Checker) filterNewReleases(releases []GitHubRelease) []ReleaseInfo {
	currentParsed := ParseVersion(Version)
	if currentParsed == nil {
		return nil
	}

	var newReleases []ReleaseInfo
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}

		releaseParsed := ParseVersion(r.TagName)
		if releaseParsed == nil {
			continue
		}

		if CompareVersions(releaseParsed, currentParsed) > 0 {
			assets := make([]ReleaseAsset, 0, len(r.Assets))
			for _, a := range r.Assets {
				assets = append(assets, ReleaseAsset{
					Name:        a.Name,
					DownloadURL: a.DownloadURL,
					Size:        a.Size,
				})
			}
			newReleases = append(newReleases, ReleaseInfo{
				Version:     r.TagName,
				Name:        r.Name,
				Changelog:   r.Body,
				URL:         r.HTMLURL,
				PublishedAt: r.PublishedAt.Unix(),
				Assets:      assets,
			})
		}
	}

	sort.Slice(newReleases, func(i, j int) bool {
		vi := ParseVersion(newReleases[i].Version)
		vj := ParseVersion(newReleases[j].Version)
		return CompareVersions(vi, vj) > 0
	})

	return newReleases
}

type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

func ParseVersion(version string) *SemVer {
	matches := semverRegex.FindStringSubmatch(version)
	if matches == nil {
		return nil
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
	}
}

func CompareVersions(a, b *SemVer) int {
	if a == nil || b == nil {
		return 0
	}

	if a.Major != b.Major {
		return a.Major - b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor - b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch - b.Patch
	}

	if a.Prerelease == "" && b.Prerelease != "" {
		return 1
	}
	if a.Prerelease != "" && b.Prerelease == "" {
		return -1
	}
	return strings.Compare(a.Prerelease, b.Prerelease)
}
