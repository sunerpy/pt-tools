// Package iyuu 是 IYUU 辅种接口的客户端。
//
// 接口按 MIT 许可的 iyuuplus-dev 客户端（composer/reseed-client）核对：请求头 token，响应信封 {code, msg, data}，
// code 为 0 成功、429 限流（data 里有 Retry-After），其余为错误。只参考接口行为，不移植代码。
package iyuu

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL 是 IYUU 的接口地址（token 在请求头里，只用 https）。
	DefaultBaseURL = "https://2025.iyuu.cn"
	// Version 是查询时报给 IYUU 的客户端版本，与现行 iyuuplus-dev 一致。
	Version = "8.3.24"
	// MaxBatch 是一次查询最多的 info hash 数。
	MaxBatch = 200
	// defaultRetryAfter 是限流时没给 Retry-After 的等待时间。
	defaultRetryAfter = 30 * time.Second
	maxBodyBytes      = 16 << 20
)

// ErrNoToken 表示没有配置 IYUU token。
var ErrNoToken = errors.New("没有设置 IYUU token")

// APIError 是 IYUU 返回的非 0 code（限流除外）。
type APIError struct {
	Code int
	Msg  string
}

func (e *APIError) Error() string { return fmt.Sprintf("IYUU 返回错误 %d: %s", e.Code, e.Msg) }

// RateLimitError 是 IYUU 返回的 429：RetryAfter 之后再试。
type RateLimitError struct {
	Msg        string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("IYUU 限流（%s），%d 秒后再试", e.Msg, int(e.RetryAfter.Seconds()))
}

// Site 是 IYUU 支持的一个站点。
type Site struct {
	SID          int    `json:"id"`
	Site         string `json:"site"`
	Nickname     string `json:"nickname"`
	BaseURL      string `json:"base_url"`
	DownloadPage string `json:"download_page"`
	IsHTTPS      int    `json:"is_https"`
}

// Candidate 是一个可以辅种的种子：哪个站点（sid）、站内种子 ID、info hash。
type Candidate struct {
	SID       int    `json:"sid"`
	TorrentID string `json:"torrent_id"`
	InfoHash  string `json:"info_hash"`
}

// UnmarshalJSON 兼容 torrent_id 是数字或字符串两种写法。
func (c *Candidate) UnmarshalJSON(b []byte) error {
	var raw struct {
		SID       json.Number `json:"sid"`
		TorrentID json.Number `json:"torrent_id"`
		InfoHash  string      `json:"info_hash"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		// torrent_id 可能是带引号的字符串
		var alt struct {
			SID       json.Number `json:"sid"`
			TorrentID string      `json:"torrent_id"`
			InfoHash  string      `json:"info_hash"`
		}
		if err2 := json.Unmarshal(b, &alt); err2 != nil {
			return err
		}
		raw.SID, raw.TorrentID, raw.InfoHash = alt.SID, json.Number(alt.TorrentID), alt.InfoHash
	}
	sid, err := strconv.Atoi(raw.SID.String())
	if err != nil {
		return fmt.Errorf("sid 不是数字: %q", raw.SID)
	}
	c.SID, c.TorrentID, c.InfoHash = sid, raw.TorrentID.String(), strings.ToLower(raw.InfoHash)
	return nil
}

// Client 调用 IYUU 接口。
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Now     func() time.Time
}

// New 返回一个用默认地址的客户端。
func New(token string) *Client {
	return &Client{BaseURL: DefaultBaseURL, Token: token, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, form url.Values) (json.RawMessage, error) {
	if strings.TrimSpace(c.Token) == "" {
		return nil, ErrNoToken
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	var body io.Reader
	target := base + path
	if method == http.MethodGet && len(form) > 0 {
		target += "?" + form.Encode()
	} else if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	httpc := c.HTTP
	if httpc == nil {
		httpc = http.DefaultClient
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 IYUU 失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("读取 IYUU 响应失败: %w", err)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("IYUU 服务器繁忙（HTTP %d）", resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("IYUU 响应不是 JSON（HTTP %d）", resp.StatusCode)
	}
	switch env.Code {
	case 0:
		return env.Data, nil
	case http.StatusTooManyRequests:
		return nil, &RateLimitError{Msg: env.Msg, RetryAfter: retryAfter(env.Data)}
	}
	return nil, &APIError{Code: env.Code, Msg: env.Msg}
}

func retryAfter(data json.RawMessage) time.Duration {
	var m map[string]json.Number
	if json.Unmarshal(data, &m) == nil {
		if n, err := strconv.Atoi(m["Retry-After"].String()); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultRetryAfter
}

// Sites 返回 IYUU 支持的站点。
func (c *Client) Sites(ctx context.Context) ([]Site, error) {
	data, err := c.do(ctx, http.MethodGet, "/reseed/sites/index", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Sites []Site `json:"sites"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("解析 IYUU 站点列表失败: %w", err)
	}
	return out.Sites, nil
}

// ReportExisting 报告用户开启辅种的站点（sid 列表），返回查询时要带的 sid_sha1（有效 7 天）。
func (c *Client) ReportExisting(ctx context.Context, sids []int) (string, error) {
	form := url.Values{}
	for _, sid := range sids {
		form.Add("sid_list[]", strconv.Itoa(sid))
	}
	data, err := c.do(ctx, http.MethodPost, "/reseed/sites/reportExisting", form)
	if err != nil {
		return "", err
	}
	var out struct {
		SidSha1 string `json:"sid_sha1"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.SidSha1 == "" {
		return "", errors.New("IYUU 的响应缺少 sid_sha1")
	}
	return out.SidSha1, nil
}

// HashParam 是查询用的 hash 参数：排序后的 info hash 数组的 JSON（与 PHP json_encode 一致，无空格）及其 SHA-1。
func HashParam(hashes []string) (string, string) {
	sorted := make([]string, 0, len(hashes))
	for _, h := range hashes {
		sorted = append(sorted, strings.ToLower(strings.TrimSpace(h)))
	}
	sort.Strings(sorted)
	b, _ := json.Marshal(sorted)
	sum := sha1.Sum(b)
	return string(b), hex.EncodeToString(sum[:])
}

// Query 查询一批（最多 MaxBatch 个）info hash 的辅种结果：源 info hash → 可辅种的种子。
func (c *Client) Query(ctx context.Context, hashes []string, sidSha1 string) (map[string][]Candidate, error) {
	if len(hashes) == 0 {
		return map[string][]Candidate{}, nil
	}
	if len(hashes) > MaxBatch {
		return nil, fmt.Errorf("一次最多查询 %d 个 info hash", MaxBatch)
	}
	hash, sum := HashParam(hashes)
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	form := url.Values{
		"hash":      {hash},
		"sha1":      {sum},
		"sid_sha1":  {sidSha1},
		"timestamp": {strconv.FormatInt(now().Unix(), 10)},
		"version":   {Version},
	}
	data, err := c.do(ctx, http.MethodPost, "/reseed/index/index", form)
	if err != nil {
		return nil, err
	}
	out := map[string][]Candidate{}
	// 没有结果时 PHP 的空数组编码成 []
	if trimmed := strings.TrimSpace(string(data)); trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return out, nil
	}
	var raw map[string]struct {
		Torrent []Candidate `json:"torrent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析 IYUU 辅种结果失败: %w", err)
	}
	for h, v := range raw {
		out[strings.ToLower(h)] = v.Torrent
	}
	return out, nil
}
