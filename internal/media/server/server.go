// Package server 是媒体服务器的客户端：Emby、Jellyfin 与 Plex。整理入库后通知媒体服务器扫描新目录
// （按路径或刷新整个媒体库），也能测试连通性、按 TMDB 或 IMDb 编号查一个条目是不是已经入库，
// 分页列出服务器里的电影、剧与集（索引用），取用户与没看完的（继续观看，Emby 与 Jellyfin），
// 拼条目在服务器网页里的打开链接，解析三家发来的 Webhook。
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

var (
	// ErrUnauthorized 表示 API Key 或 Token 不对，或者没有管理员权限。
	ErrUnauthorized = errors.New("媒体服务器拒绝了请求：API Key 或 Token 不对，或者没有管理员权限")
	// ErrUnavailable 表示连不上媒体服务器，或它回了其它错误。
	ErrUnavailable = errors.New("连不上媒体服务器")
	// ErrRedirect 表示媒体服务器回了跳转：不跟着跳（会把 Token 发到别的地址）。
	ErrRedirect = errors.New("媒体服务器回了跳转（3xx）：地址要填完整的最终地址，例如 https:// 与端口")
	// ErrNoLibrary 表示 Plex 里没有包含这个路径的媒体库。
	ErrNoLibrary = errors.New("Plex 里没有包含这个路径的媒体库：检查媒体服务器的路径映射")
	// ErrBadConfig 表示配置不对（错误信息写明哪一项）。
	ErrBadConfig = errors.New("媒体服务器配置无效")
	// ErrUnsupported 表示这种媒体服务器没有这项功能（Plex 没有继续观看的用户接口）。
	ErrUnsupported = errors.New("这种媒体服务器不支持")
)

// Info 是测试连通性时读到的服务器信息。ID 是服务器自己的编号（Emby、Jellyfin 的 Id，Plex 的 machineIdentifier）。
type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Libraries int    `json:"libraries,omitempty"`
	ID        string `json:"-"`
}

// Query 是「是不是已经入库」的查询：按名字找，再比对 TMDB 或 IMDb 编号。
type Query struct {
	Kind   string // movie 或 tv
	TMDBID int
	IMDbID string
	Title  string
}

// Client 是一个媒体服务器。
type Client interface {
	// Test 用 API Key 或 Token 访问一次，返回服务器名称与版本。
	Test(ctx context.Context) (Info, error)
	// RefreshPaths 通知媒体服务器这些目录（媒体服务器里看到的路径）有新内容。
	RefreshPaths(ctx context.Context, paths []string) error
	// RefreshAll 刷新整个媒体库。
	RefreshAll(ctx context.Context) error
	// Exists 报告条目是不是已经入库。
	Exists(ctx context.Context, q Query) (bool, error)
	// ListItems 列出服务器里的电影、剧与集，一次一页：cursor 为空时从头开始，返回的 Next 为空时列完了。
	ListItems(ctx context.Context, cursor string) (ItemPage, error)
	// Users 列出服务器的用户（选继续观看用哪个用户）；Plex 回 ErrUnsupported。
	Users(ctx context.Context) ([]User, error)
	// Resume 是一个用户没看完的电影与集，最近看的在前，最多 limit 项；Plex 回 ErrUnsupported。
	Resume(ctx context.Context, userID string, limit int) ([]ResumeItem, error)
}

// Item 是服务器里的一项：Kind 是 models.MediaServerItemMovie、Series 或 Episode。编号是这一项自己的
// （集的 TMDB 编号是这一集的，索引里换成所在剧的）；Season、Episode、EpisodeEnd 只有集才有；AddedAt 是入库时间，不知道时是零值。
type Item struct {
	ID         string
	Kind       string
	SeriesID   string
	TMDBID     int
	IMDbID     string
	TVDBID     string
	Season     int
	Episode    int
	EpisodeEnd int
	Title      string
	Year       int
	Path       string
	AddedAt    time.Time
}

// ItemPage 是列条目的一页。Next 为空表示列完了；Changed 为真表示列的过程中服务器里的条目数变了（有新加或删除，
// 分页可能错位），这一轮列到的可能不全。
type ItemPage struct {
	Items   []Item
	Next    string
	Changed bool
}

// itemPageSize 是列条目时一页多少项。
const itemPageSize = 500

// User 是服务器的一个用户。
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResumeItem 是一个没看完的：Kind 是 movie 或 episode；集带剧的条目编号、剧名、季与集。TMDBID、IMDbID 是这一项自己的
// （集的是这一集的）；Percent 是看到百分之几，RemainingSeconds 是还剩几秒（不知道片长时是 0）。
type ResumeItem struct {
	ID               string
	Kind             string
	SeriesID         string
	SeriesName       string
	Title            string
	Season           int
	Episode          int
	Year             int
	TMDBID           int
	IMDbID           string
	Percent          float64
	RemainingSeconds int64
}

// ItemURL 是一项在媒体服务器网页里的打开链接。base 是浏览器能打开的服务器地址，uid 是服务器编号（Plex 没有编号时拼不出）。
func ItemURL(kind, base, uid, itemID string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || itemID == "" {
		return ""
	}
	var u string
	switch kind {
	case models.MediaServerEmby:
		u = base + "/web/index.html#!/item?id=" + url.QueryEscape(itemID)
	case models.MediaServerJellyfin:
		u = base + "/web/#/details?id=" + url.QueryEscape(itemID)
	case models.MediaServerPlex:
		if uid == "" {
			return ""
		}
		return base + "/web/index.html#!/server/" + url.PathEscape(uid) + "/details?key=" + url.QueryEscape("/library/metadata/"+itemID)
	default:
		return ""
	}
	if uid != "" {
		u += "&serverId=" + url.QueryEscape(uid)
	}
	return u
}

// providerIDs 从 Emby、Jellyfin 的 ProviderIds 里取 TMDB、IMDb、TVDB 编号（键不分大小写）。
func providerIDs(m map[string]string) (tmdb int, imdb, tvdb string) {
	for k, v := range m {
		v = strings.TrimSpace(v)
		switch strings.ToLower(k) {
		case "tmdb":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				tmdb = n
			}
		case "imdb":
			imdb = strings.ToLower(v)
		case "tvdb":
			tvdb = v
		}
	}
	return tmdb, imdb, tvdb
}

// pageCursor 是分页的位置：从第几项开始，第一页时服务器报的总数（-1 是还没有第一页）。
type pageCursor struct {
	start, total int
}

func parseCursor(c string) (pageCursor, error) {
	if c == "" {
		return pageCursor{total: -1}, nil
	}
	a, b, ok := strings.Cut(c, ":")
	start, err1 := strconv.Atoi(a)
	total, err2 := strconv.Atoi(b)
	if !ok || err1 != nil || err2 != nil || start < 0 || total < 0 {
		return pageCursor{}, fmt.Errorf("%w: 分页位置 %q 不对", ErrBadConfig, c)
	}
	return pageCursor{start: start, total: total}, nil
}

// next 是读到一页以后的下一页位置与「总数变了」：got 是这一页的项数，total 是这一页报的总数。
// 这一页是空的而还没到总数时也当作列完了（服务器里少了项），记成变了。
func (pc pageCursor) next(got, total int) (string, bool) {
	changed := pc.total >= 0 && total != pc.total
	first := pc.total
	if first < 0 {
		first = total
	}
	at := pc.start + got
	if got == 0 || at >= total {
		return "", changed || (got == 0 && pc.start < total)
	}
	return strconv.Itoa(at) + ":" + strconv.Itoa(first), changed
}

// Config 是建客户端用的配置。
type Config struct {
	Kind  string
	URL   string
	Token string
	// HTTPClient 为空时用默认的（15 秒超时、不跟跳转）
	HTTPClient *http.Client
}

// New 按种类建客户端。
func New(cfg Config) (Client, error) {
	base, err := CheckURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("%w: 没有填写 API Key 或 Token", ErrBadConfig)
	}
	if strings.ContainsAny(token, "\" \t\r\n") || len(token) > 512 {
		return nil, fmt.Errorf("%w: API Key 或 Token 里不能有空格与引号", ErrBadConfig)
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = NewHTTPClient()
	}
	h := &httpc{base: base, client: hc}
	switch cfg.Kind {
	case models.MediaServerEmby:
		h.auth = func(r *http.Request) { r.Header.Set("X-Emby-Token", token) }
		return &emby{h: h}, nil
	case models.MediaServerJellyfin:
		// Jellyfin 新版本不再认 X-Emby-Token，统一用 Authorization 头
		h.auth = func(r *http.Request) {
			r.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s", Client="pt-tools", Device="pt-tools", DeviceId="pt-tools", Version="1"`, token))
		}
		return &emby{h: h}, nil
	case models.MediaServerPlex:
		h.auth = func(r *http.Request) {
			r.Header.Set("X-Plex-Token", token)
			r.Header.Set("X-Plex-Client-Identifier", "pt-tools")
			r.Header.Set("X-Plex-Product", "pt-tools")
		}
		return &plex{h: h}, nil
	}
	return nil, fmt.Errorf("%w: 种类 %q（可选 emby、jellyfin、plex）", ErrBadConfig, cfg.Kind)
}

// CheckURL 检查媒体服务器地址：http 或 https，有主机，不带查询参数与片段；返回去掉末尾 / 的地址。
func CheckURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%w: 地址要以 http:// 或 https:// 开头，例如 http://192.168.1.10:8096", ErrBadConfig)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%w: 地址里不要带用户名、查询参数或 #", ErrBadConfig)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// NewHTTPClient 是访问媒体服务器用的 http.Client：15 秒超时，不跟跳转。媒体服务器一般在局域网里，不走代理。
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 60 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// maxBody 是媒体服务器响应最多读多少字节。
const maxBody = 8 << 20

type httpc struct {
	base   string
	client *http.Client
	auth   func(*http.Request)
}

// do 发一个请求；body 不为空时按 JSON 发；out 不为空时把响应按 JSON 解析进去。
func (h *httpc) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := h.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("%w: 地址格式不对", ErrBadConfig)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	h.auth(req)
	resp, err := h.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: 读取响应失败: %w", ErrUnavailable, err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		return ErrRedirect
	case resp.StatusCode >= 400:
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: 响应不是预期的 JSON（地址是不是填成了别的服务）", ErrUnavailable)
	}
	return nil
}

// within 报告 p 是不是在 dir 里面或就是 dir（按路径分段比较，/ 与 \ 都认，Windows 盘符不分大小写）。
func within(dir, p string) bool {
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, `\`, "/"), "/") }
	d, q := norm(dir), norm(p)
	if len(d) >= 2 && d[1] == ':' {
		d, q = strings.ToLower(d), strings.ToLower(q)
	}
	if d == "" {
		return strings.HasPrefix(q, "/")
	}
	return q == d || strings.HasPrefix(q, d+"/")
}
