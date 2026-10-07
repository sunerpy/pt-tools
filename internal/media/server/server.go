// Package server 是媒体服务器的客户端：Emby、Jellyfin 与 Plex。整理入库后通知媒体服务器扫描新目录
// （按路径或刷新整个媒体库），也能测试连通性、按 TMDB 或 IMDb 编号查一个条目是不是已经入库。
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
)

// Info 是测试连通性时读到的服务器信息。
type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Libraries int    `json:"libraries,omitempty"`
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
