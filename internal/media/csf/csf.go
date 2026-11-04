// Package csf 是 ChineseSubFinder（自动下载中文字幕的工具）的外部接口客户端：整理出新视频以后交给它下载字幕。
//
// 接口出自 ChineseSubFinder 的源码（github.com/ChineseSubFinder/ChineseSubFinder，MIT）：
// /api/v1 下的接口认请求头 Authorization: Bearer <API Key>（设置里开了「API Key」才有）；
// POST /api/v1/add-job 加一个下载字幕的任务（video_type 电影 0、剧集 1，physical_video_file_full_path 是 ChineseSubFinder
// 看到的视频路径，task_priority_level 0 最高、10 最低），回 {"job_id","message"}：message 是 ok 或 job is already in queue
// 算加上了，physical video file not found 是那边没有这个文件；剧集要读这一集旁边的 NFO 与剧的目录，读不到时回 500。
package csf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrBadConfig 是地址或 API Key 不对（没填、不是 http/https 的地址）。
	ErrBadConfig = errors.New("ChineseSubFinder 的设置不对")
	// ErrUnauthorized 是 API Key 不对（ChineseSubFinder 回 401）。
	ErrUnauthorized = errors.New("ChineseSubFinder 的 API Key 不对")
	// ErrFileNotFound 是 ChineseSubFinder 那边没有这个视频文件（多半是库目录填得不对）。
	ErrFileNotFound = errors.New("ChineseSubFinder 那边找不到这个视频文件")
	// ErrUnavailable 是连不上，或者回的不是 ChineseSubFinder 的应答（地址填错了）。
	ErrUnavailable = errors.New("连不上 ChineseSubFinder")
)

// 视频的种类（add-job 的 video_type）。
const (
	VideoMovie  = 0
	VideoSeries = 1
)

// Priority 是 pt-tools 交的任务的优先级（0 最高、10 最低；ChineseSubFinder 自己扫到的是 5）。
const Priority = 3

// maxBody 是应答最多读多少字节（ChineseSubFinder 回的是很短的 JSON）。
const maxBody = 1 << 20

// Client 是一台 ChineseSubFinder。
type Client struct {
	base string // 以 / 结尾
	key  string
	hc   *http.Client
}

// NewHTTPClient 是访问 ChineseSubFinder 用的 HTTP 客户端：多半在局域网里，不走代理、不跟跳转，15 秒超时（同媒体服务器）。
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       15 * time.Second,
		Transport:     &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 60 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// NormalizeURL 检查并规整地址：http 或 https、有主机、不带用户名密码与查询参数；规整成以 / 结尾。
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if raw == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%w：地址要写成 http://主机:端口/ 这样", ErrBadConfig)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", fmt.Errorf("%w：地址里不要带用户名、密码或问号后面的参数", ErrBadConfig)
	}
	p := strings.TrimRight(u.EscapedPath(), "/")
	return u.Scheme + "://" + u.Host + p + "/", nil
}

// New 按地址与 API Key 建客户端；hc 为空时用 NewHTTPClient。
func New(rawURL, key string, hc *http.Client) (*Client, error) {
	base, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("%w：没有填 API Key", ErrBadConfig)
	}
	if hc == nil {
		hc = NewHTTPClient()
	}
	return &Client{base: base, key: key, hc: hc}, nil
}

// Job 是交给 ChineseSubFinder 的一个视频。
type Job struct {
	VideoType int
	// Path 是 ChineseSubFinder 看到的视频路径（见 PathFor）
	Path     string
	IsBluray bool
}

type reply struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

// AddJob 让 ChineseSubFinder 给这个视频下载字幕，返回任务编号；已经在队列里也算成功。
func (c *Client) AddJob(ctx context.Context, j Job) (string, error) {
	if strings.TrimSpace(j.Path) == "" {
		return "", fmt.Errorf("%w：没有视频路径", ErrBadConfig)
	}
	body, _ := json.Marshal(map[string]any{
		"video_type": j.VideoType, "physical_video_file_full_path": j.Path, "task_priority_level": Priority,
		"media_server_inside_video_id": "", "is_bluray": j.IsBluray,
	})
	r, err := c.do(ctx, http.MethodPost, "api/v1/add-job", body)
	if err != nil {
		return "", err
	}
	switch r.Message {
	case "ok", "job is already in queue":
		return r.JobID, nil
	case "physical video file not found":
		return "", fmt.Errorf("%w：%s", ErrFileNotFound, j.Path)
	}
	return "", fmt.Errorf("ChineseSubFinder 没有加上任务：%s", short(r.Message))
}

// Test 访问一次 GET api/v1/job-status（不带任务编号，不建任务）：地址与 API Key 都对时 ChineseSubFinder 回 200。
func (c *Client) Test(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "api/v1/job-status", nil)
	return err
}

// do 发一个请求，读回 ChineseSubFinder 的 JSON 应答。401 是 API Key 不对；别的不是 200 的写上状态码与 message。
// 错误里不带 API Key（它在请求头里）。
func (c *Client) do(ctx context.Context, method, rel string, body []byte) (reply, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+rel, rd)
	if err != nil {
		return reply{}, fmt.Errorf("%w：%v", ErrBadConfig, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return reply{}, fmt.Errorf("%w（%s）：%v", ErrUnavailable, c.base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	var r reply
	jsonErr := json.Unmarshal(raw, &r)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return reply{}, ErrUnauthorized
	case resp.StatusCode == http.StatusOK && jsonErr == nil:
		return r, nil
	case resp.StatusCode == http.StatusOK || jsonErr != nil:
		// 回的不是 ChineseSubFinder 的 JSON：多半是地址填成了别的网页（或者前面有反向代理出错了）
		return reply{}, fmt.Errorf("%w（%s）：回的不是 ChineseSubFinder 的应答（HTTP %d）", ErrUnavailable, c.base, resp.StatusCode)
	}
	return reply{}, fmt.Errorf("ChineseSubFinder 出错了（HTTP %d）：%s", resp.StatusCode, short(r.Message))
}

// short 把 ChineseSubFinder 回的话截到 200 个字。
func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "没有说明"
	}
	if utf8.RuneCountInString(s) > 200 {
		return string([]rune(s)[:200]) + "…"
	}
	return s
}

// PathFor 是库里一个文件在 ChineseSubFinder 那边的路径：target 在库目录 lib 下的相对路径接到 prefix（ChineseSubFinder 里这个库的目录）
// 后面；prefix 为空时就是 target。prefix 像 Windows 路径（有盘符或 \）时用 \ 连接。target 不在库目录里时 ok 为假。
func PathFor(lib, prefix, target string) (string, bool) {
	rel, ok := relPath(lib, target)
	if !ok {
		return "", false
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return target, true
	}
	if windowsPath(prefix) {
		return strings.TrimRight(prefix, `\/`) + `\` + strings.ReplaceAll(rel, "/", `\`), true
	}
	return strings.TrimRight(prefix, "/") + "/" + rel, true
}

// relPath 是 target 在 dir 下的相对路径（用 / 分隔）；两边的分隔符 \ 与 / 都认。不在 dir 下时 ok 为假。
func relPath(dir, target string) (string, bool) {
	d := path.Clean(strings.ReplaceAll(dir, `\`, "/"))
	t := path.Clean(strings.ReplaceAll(target, `\`, "/"))
	if d == "." || d == "/" || !strings.HasPrefix(t, d+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(t, d+"/")
	if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// windowsPath 报告路径是不是 Windows 的写法（C:\…、C:/…、\\主机\…）。
func windowsPath(p string) bool {
	return strings.HasPrefix(p, `\\`) || strings.Contains(p, `\`) || (len(p) >= 2 && p[1] == ':')
}
