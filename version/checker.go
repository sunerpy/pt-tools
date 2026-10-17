package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/utils/httpclient"

	"github.com/Masterminds/semver/v3"
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
	// Prerelease 标记此版本是否为预发版（beta/rc/alpha）
	Prerelease bool `json:"prerelease,omitempty"`
	// PrereleaseLabel 从 tag 中解析出的预发版通道名（beta/rc/alpha），用于前端徽章展示
	PrereleaseLabel string `json:"prerelease_label,omitempty"`
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
	// IncludePrerelease 是否在结果中包含预发版（beta/rc/alpha）。
	// 默认 false，保持与旧客户端兼容；前端通过查询参数按需启用。
	IncludePrerelease bool
}

type Checker struct {
	mu                sync.RWMutex
	lastCheck         time.Time
	lastResult        *VersionCheckResult
	lastProxy         string
	lastIncludePrerel bool
}

var (
	defaultChecker *Checker
	checkerOnce    sync.Once
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
	prerelChanged := opts.IncludePrerelease != c.lastIncludePrerel
	if !opts.Force && !proxyChanged && !prerelChanged && c.lastResult != nil && time.Since(c.lastCheck) < CheckInterval {
		result := c.lastResult
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	proxyChanged = opts.ProxyURL != c.lastProxy
	prerelChanged = opts.IncludePrerelease != c.lastIncludePrerel
	if !opts.Force && !proxyChanged && !prerelChanged && c.lastResult != nil && time.Since(c.lastCheck) < CheckInterval {
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

	newReleases := c.filterNewReleases(releases, opts.IncludePrerelease)
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
	c.lastIncludePrerel = opts.IncludePrerelease
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
		session := httpclient.NewSession().
			WithProxy(proxyURL).
			WithTimeout(RequestTimeout)
		defer session.Close()
		resp, err = session.Do(req)
	} else {
		session := httpclient.NewSession().WithTimeout(RequestTimeout)
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

func (c *Checker) filterNewReleases(releases []GitHubRelease, includePrerelease bool) []ReleaseInfo {
	currentParsed := ParseVersion(Version)
	if currentParsed == nil {
		return nil
	}

	var newReleases []ReleaseInfo
	for _, r := range releases {
		if r.Draft {
			continue
		}
		if r.Prerelease && !includePrerelease {
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
			// 同时认两种信号：GitHub Release 的 prerelease 标记 + tag 中的 -beta/-rc/-alpha 后缀，
			// 任一命中即当作预发版，避免发版时漏勾 prerelease 勾选导致误判。
			label := extractPrereleaseLabel(releaseParsed.Prerelease())
			isPrerelease := r.Prerelease || label != ""
			newReleases = append(newReleases, ReleaseInfo{
				Version:         r.TagName,
				Name:            r.Name,
				Changelog:       r.Body,
				URL:             r.HTMLURL,
				PublishedAt:     r.PublishedAt.Unix(),
				Assets:          assets,
				Prerelease:      isPrerelease,
				PrereleaseLabel: label,
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

// extractPrereleaseLabel 从 semver prerelease 段（例如 "beta.1" / "rc.2" / "alpha"）
// 提取首个 dot 分段的通道名，返回小写字符串。未识别或为空时返回 ""。
// 只认 beta/rc/alpha 三类，其它标签（如 "dev" / "snapshot"）一律当作非预发版处理。
func extractPrereleaseLabel(prereleaseSegment string) string {
	if prereleaseSegment == "" {
		return ""
	}
	first := strings.ToLower(strings.SplitN(prereleaseSegment, ".", 2)[0])
	switch first {
	case "beta", "rc", "alpha":
		return first
	}
	return ""
}

// ParseVersion 解析 tag 形式的版本号（允许 v 前缀，例如 v1.0.0-rc.35）。
//
// 解析交给 Masterminds/semver 的严格模式：必须是 major.minor.patch，
// 可带 -prerelease / +build，数字标识符不允许前导零（semver §9）。
// 不符合上述规则的字符串返回 nil。
func ParseVersion(version string) *semver.Version {
	v, err := semver.StrictNewVersion(strings.TrimPrefix(version, "v"))
	if err != nil {
		return nil
	}
	return v
}

// CompareVersions 按 semver 2.0.0 的优先级规则比较两个版本：
// a > b 返回正数，a < b 返回负数，相等返回 0；任一为 nil 返回 0。
//
// 预发版规则由 Masterminds/semver 保证：预发版低于同号正式版；
// 预发版之间按标识符逐段比较，纯数字段按数值比较（"rc.9" < "rc.35"），
// 纯数字段优先级低于含字母的段，前面相同则段数多者更大。
func CompareVersions(a, b *semver.Version) int {
	if a == nil || b == nil {
		return 0
	}
	return a.Compare(b)
}
