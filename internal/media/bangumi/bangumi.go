// Package bangumi 从 Bangumi（bgm.tv）取动画的中文简介、评分与每一集的中文标题，刮削时补充 TMDB 的数据。
//
// 用的是 Bangumi 的公开接口 api.bgm.tv/v0（不用登录、不用 Key，但要带写明来历的 User-Agent）：搜索
// POST /v0/search/subjects、条目 GET /v0/subjects/{id}、章节 GET /v0/episodes。Bangumi 上一季（一部分）是一个条目，
// 按名字与首播日期找同一季（见 Client.Find）。两次请求之间至少隔 Interval，结果缓存一段时间。
package bangumi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

// 默认的接口地址、请求间隔与缓存时间。
const (
	DefaultBaseURL   = "https://api.bgm.tv"
	DefaultInterval  = time.Second
	DefaultUserAgent = "sunerpy/pt-tools (https://github.com/sunerpy/pt-tools)"

	searchTTL  = 24 * time.Hour
	subjectTTL = 7 * 24 * time.Hour
	// 章节会随着播出更新，缓存短一些
	episodesTTL = 24 * time.Hour
	maxBody     = 2 << 20
	// maxEpisodePages 是章节最多翻几页（每页 100 集）
	maxEpisodePages = 10
)

// SubjectAnime 是 Bangumi 的「动画」条目类型。
const SubjectAnime = 2

// Cache 是响应缓存（和 TMDB 共用 media_cache 表，见 tmdb.DBCache）。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Config 是 Bangumi 客户端的配置；零值的项用默认值。
type Config struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
	Cache     Cache
	// Interval 是两次请求之间至少隔多久
	Interval time.Duration
}

// Client 是 Bangumi 客户端，可以并发使用（请求按 Interval 排队）。
type Client struct {
	cfg  Config
	mu   sync.Mutex
	next time.Time
}

// New 建一个 Bangumi 客户端。
func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{cfg: cfg}
}

// Subject 是 Bangumi 的一个条目（动画一季或一部剧场版）。
type Subject struct {
	ID       int    `json:"id"`
	Type     int    `json:"type"`
	Name     string `json:"name"`
	NameCN   string `json:"name_cn"`
	Summary  string `json:"summary"`
	Date     string `json:"date"`
	Platform string `json:"platform"`
	// Score 是 10 分制的评分，没人评分时为 0
	Score float64 `json:"score"`
	Votes int     `json:"votes"`
	Eps   int     `json:"eps"`
}

// Episode 是条目的一集（正片）。Ep 是这个条目里的第几集。
type Episode struct {
	ID      int    `json:"id"`
	Ep      int    `json:"ep"`
	Name    string `json:"name"`
	NameCN  string `json:"name_cn"`
	AirDate string `json:"airdate"`
	Desc    string `json:"desc"`
}

type rawSubject struct {
	ID       int    `json:"id"`
	Type     int    `json:"type"`
	Name     string `json:"name"`
	NameCN   string `json:"name_cn"`
	Summary  string `json:"summary"`
	Date     string `json:"date"`
	Platform string `json:"platform"`
	Eps      int    `json:"eps"`
	Rating   *struct {
		Score float64 `json:"score"`
		Total int     `json:"total"`
	} `json:"rating"`
}

func (r rawSubject) subject() Subject {
	s := Subject{
		ID: r.ID, Type: r.Type, Name: strings.TrimSpace(r.Name), NameCN: strings.TrimSpace(r.NameCN),
		Summary: strings.TrimSpace(r.Summary), Date: strings.TrimSpace(r.Date), Platform: r.Platform, Eps: r.Eps,
	}
	if r.Rating != nil {
		s.Score, s.Votes = r.Rating.Score, r.Rating.Total
	}
	return s
}

// Search 搜动画条目：keyword 是名字，from、to 是首播日期的范围（yyyy-mm-dd，含 from、不含 to；为空不限）。
func (c *Client) Search(ctx context.Context, keyword, from, to string) ([]Subject, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, nil
	}
	filter := map[string]any{"type": []int{SubjectAnime}, "nsfw": false}
	var dates []string
	if from != "" {
		dates = append(dates, ">="+from)
	}
	if to != "" {
		dates = append(dates, "<"+to)
	}
	if len(dates) > 0 {
		filter["air_date"] = dates
	}
	body, _ := json.Marshal(map[string]any{"keyword": keyword, "sort": "match", "filter": filter})
	sum := sha256.Sum256(body)
	var raw struct {
		Data []rawSubject `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, "bangumi:v1:search:"+hex.EncodeToString(sum[:16]), searchTTL, "/v0/search/subjects?limit=10", body, &raw); err != nil {
		return nil, err
	}
	out := make([]Subject, 0, len(raw.Data))
	for _, r := range raw.Data {
		if r.ID > 0 {
			out = append(out, r.subject())
		}
	}
	return out, nil
}

// Get 取一个条目。
func (c *Client) Get(ctx context.Context, id int) (*Subject, error) {
	if id <= 0 {
		return nil, fmt.Errorf("Bangumi 编号不对: %d", id)
	}
	var raw rawSubject
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("bangumi:v1:subject:%d", id), subjectTTL, fmt.Sprintf("/v0/subjects/%d", id), nil, &raw); err != nil {
		return nil, err
	}
	if raw.ID == 0 {
		return nil, errors.New("Bangumi 返回的条目是空的")
	}
	s := raw.subject()
	return &s, nil
}

// Episodes 取一个条目的正片（最多 maxEpisodePages 页）。
func (c *Client) Episodes(ctx context.Context, id int) ([]Episode, error) {
	if id <= 0 {
		return nil, fmt.Errorf("Bangumi 编号不对: %d", id)
	}
	var out []Episode
	for page := range maxEpisodePages {
		var raw struct {
			Data []struct {
				ID      int     `json:"id"`
				Ep      float64 `json:"ep"`
				Sort    float64 `json:"sort"`
				Name    string  `json:"name"`
				NameCN  string  `json:"name_cn"`
				AirDate string  `json:"airdate"`
				Desc    string  `json:"desc"`
			} `json:"data"`
			Total int `json:"total"`
		}
		path := fmt.Sprintf("/v0/episodes?subject_id=%d&type=0&limit=100&offset=%d", id, page*100)
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("bangumi:v1:episodes:%d:%d", id, page), episodesTTL, path, nil, &raw); err != nil {
			return nil, err
		}
		for _, e := range raw.Data {
			n := e.Ep
			if n <= 0 {
				n = e.Sort
			}
			// 13.5 这样的总集篇不对应 TMDB 的集
			if n <= 0 || n != math.Trunc(n) {
				continue
			}
			out = append(out, Episode{
				ID: e.ID, Ep: int(n), Name: strings.TrimSpace(e.Name), NameCN: strings.TrimSpace(e.NameCN),
				AirDate: strings.TrimSpace(e.AirDate), Desc: strings.TrimSpace(e.Desc),
			})
		}
		if len(raw.Data) < 100 || (page+1)*100 >= raw.Total {
			break
		}
	}
	return out, nil
}

// Query 是要找的条目：TMDB 的中文名、原名（日本动画一般是日文）与首播日期（剧集是这一季的）。
type Query struct {
	Title         string
	OriginalTitle string
	Date          string
}

// dateWindow 是按首播日期找时前后放宽几天（各地首播时间不同、Bangumi 记的是日本的）。
const dateWindow = 10 * 24 * time.Hour

// Find 按名字与首播日期找同一部（剧集是同一季）：先用原名搜，没找到再用中文名。名字（条目的原名或中文名以 TMDB 的原名或
// 中文名开头，第二季多半是「XX 第2期」）与首播日期（前后 10 天以内）都对得上的只有一个时用它；不止一个时取名字完全相同的，
// 还拿不准就不用。没给首播日期时只认名字完全相同、只有一个的。没找到时返回 nil、nil。
func (c *Client) Find(ctx context.Context, q Query) (*Subject, error) {
	var from, to string
	day, dateErr := time.Parse(time.DateOnly, strings.TrimSpace(q.Date))
	if dateErr == nil {
		from, to = day.Add(-dateWindow).Format(time.DateOnly), day.Add(dateWindow+24*time.Hour).Format(time.DateOnly)
	}
	tried := map[string]bool{}
	for _, name := range []string{q.OriginalTitle, q.Title} {
		name = strings.TrimSpace(name)
		if name == "" || tried[name] {
			continue
		}
		tried[name] = true
		cands, err := c.Search(ctx, name, from, to)
		if err != nil {
			return nil, err
		}
		if s := match(cands, q, dateErr == nil); s != nil {
			return c.Get(ctx, s.ID)
		}
	}
	return nil, nil
}

// match 在搜索结果里挑出对得上的条目（规则见 Find）。
func match(cands []Subject, q Query, dated bool) *Subject {
	names := []string{norm(q.OriginalTitle), norm(q.Title)}
	var prefix, exact []*Subject
	for i := range cands {
		s := &cands[i]
		if s.Type != SubjectAnime {
			continue
		}
		sn, scn := norm(s.Name), norm(s.NameCN)
		isExact, isPrefix := false, false
		for _, n := range names {
			if n == "" {
				continue
			}
			if sn == n || scn == n {
				isExact = true
			}
			if (sn != "" && strings.HasPrefix(sn, n)) || (scn != "" && strings.HasPrefix(scn, n)) {
				isPrefix = true
			}
		}
		if isExact {
			exact = append(exact, s)
		}
		if isPrefix || isExact {
			prefix = append(prefix, s)
		}
	}
	switch {
	case !dated:
		if len(exact) == 1 {
			return exact[0]
		}
	case len(prefix) == 1:
		return prefix[0]
	case len(exact) == 1:
		return exact[0]
	}
	return nil
}

// norm 去掉空格、标点与符号，转成小写。
func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// do 先看缓存，没有时发请求并把响应按 JSON 解析进 out；解析得了才写缓存。
func (c *Client) do(ctx context.Context, method, key string, ttl time.Duration, path string, body []byte, out any) error {
	if c.cfg.Cache != nil && !cachectl.Bypassed(ctx) {
		if b, ok := c.cfg.Cache.Get(key); ok && json.Unmarshal(b, out) == nil {
			return nil
		}
	}
	if err := c.wait(ctx); err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, rd)
	if err != nil {
		return errors.New("Bangumi 接口地址格式不对")
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("连不上 Bangumi: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return fmt.Errorf("读取 Bangumi 的响应失败: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("Bangumi 拒绝了请求（HTTP %d），可能是请求太多，过一阵再试", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("Bangumi 返回 HTTP %s", strconv.Itoa(resp.StatusCode))
	case len(data) > maxBody:
		return errors.New("Bangumi 的响应太大")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Bangumi 返回的不是预期的内容: %w", err)
	}
	if c.cfg.Cache != nil {
		c.cfg.Cache.Set(key, data, ttl)
	}
	return nil
}

// errNotFound 是条目不存在（被删了或编号不对）。
var errNotFound = errors.New("Bangumi 上没有这个条目")

// IsNotFound 报告错误是不是条目不存在。
func IsNotFound(err error) bool { return errors.Is(err, errNotFound) }

// wait 等到可以发下一个请求：每个请求占一个时间位，并发的请求依次往后排。
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	at := c.next
	if at.Before(now) {
		at = now
	}
	c.next = at.Add(c.cfg.Interval)
	c.mu.Unlock()
	d := time.Until(at)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
