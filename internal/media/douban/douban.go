// Package douban 从豆瓣取电影与剧集（一季）的简介和评分，刮削时补充 TMDB 的数据。
//
// 豆瓣没有开放的接口，这里用的是它网页自己用的两个：搜索 movie.douban.com/j/subject_suggest（只认名字，不认 IMDb 编号），
// 详情 m.douban.com/rexxar/api/v2/{movie|tv}/{id}（要带 Referer）。剧集在豆瓣上一季是一个条目。接口随时可能变或限流：
// 取不到时返回错误，由调用方改用 TMDB 的数据。两次请求之间至少隔 Interval，结果缓存一段时间。
package douban

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

// 默认的接口地址、请求间隔与缓存时间。
const (
	DefaultSearchURL = "https://movie.douban.com/j/subject_suggest"
	DefaultDetailURL = "https://m.douban.com/rexxar/api/v2"
	DefaultInterval  = 2 * time.Second

	searchTTL = 24 * time.Hour
	detailTTL = 7 * 24 * time.Hour
	maxBody   = 1 << 20
	userAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"
)

// 条目的种类，和 tmdb.KindMovie、tmdb.KindTV 一样。
const (
	KindMovie = "movie"
	KindTV    = "tv"
)

// Cache 是响应缓存（和 TMDB 共用 media_cache 表，见 tmdb.DBCache）。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Config 是豆瓣客户端的配置；零值的项用默认值。
type Config struct {
	SearchURL string
	DetailURL string
	HTTP      *http.Client
	Cache     Cache
	// Interval 是两次请求之间至少隔多久
	Interval time.Duration
}

// Client 是豆瓣客户端，可以并发使用（请求按 Interval 排队）。
type Client struct {
	cfg  Config
	mu   sync.Mutex
	next time.Time
}

// New 建一个豆瓣客户端。
func New(cfg Config) *Client {
	if cfg.SearchURL == "" {
		cfg.SearchURL = DefaultSearchURL
	}
	if cfg.DetailURL == "" {
		cfg.DetailURL = DefaultDetailURL
	}
	cfg.DetailURL = strings.TrimRight(cfg.DetailURL, "/")
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !isDouban(req.URL.Hostname()) {
				return errors.New("豆瓣的跳转不对")
			}
			return nil
		}}
	}
	return &Client{cfg: cfg}
}

func isDouban(host string) bool {
	return host == "douban.com" || strings.HasSuffix(host, ".douban.com")
}

// Query 是要找的条目：TMDB 的中文名、原名与年份；剧集再加第几季（一季是一个条目，年份用这一季的）。
type Query struct {
	Kind          string
	Title         string
	OriginalTitle string
	Year          int
	// Season 是剧集的第几季，0 当第 1 季
	Season int
}

// Subject 是豆瓣的一个条目（剧集是一季）。
type Subject struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          int    `json:"year"`
	Intro         string `json:"intro"`
	// Rating 是 10 分制的评分，没人评分时为 0
	Rating float64 `json:"rating"`
	Votes  int     `json:"votes"`
}

type suggestion struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	SubTitle string `json:"sub_title"`
	Year     string `json:"year"`
	Type     string `json:"type"`
	// Episode 是集数：剧集才有（可能是 "unknow"），电影是空串
	Episode string `json:"episode"`
}

type detail struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          string `json:"year"`
	Intro         string `json:"intro"`
	Rating        *struct {
		Value float64 `json:"value"`
		Count int     `json:"count"`
	} `json:"rating"`
}

var idRe = regexp.MustCompile(`^\d{1,12}$`)

// Find 按名字在豆瓣上找同一部（剧集是同一季）：先用中文名搜，没找到再用原名。名字（或豆瓣给的原名）、年份（差一年以内）、
// 种类与季都对得上才算。没找到时返回 nil、nil；豆瓣不能访问或返回的内容不对时返回错误。
func (c *Client) Find(ctx context.Context, q Query) (*Subject, error) {
	tried := map[string]bool{}
	for _, name := range []string{q.Title, q.OriginalTitle} {
		name = strings.TrimSpace(name)
		if name == "" || tried[name] {
			continue
		}
		tried[name] = true
		cands, err := c.suggest(ctx, name)
		if err != nil {
			return nil, err
		}
		if id := match(cands, q); id != "" {
			return c.Subject(ctx, q.Kind, id)
		}
	}
	return nil, nil
}

// Subject 取一个条目的详情（kind 是 KindMovie 或 KindTV）。
func (c *Client) Subject(ctx context.Context, kind, id string) (*Subject, error) {
	if !idRe.MatchString(id) {
		return nil, fmt.Errorf("豆瓣编号不对: %q", id)
	}
	if kind != KindTV {
		kind = KindMovie
	}
	var d detail
	if err := c.getJSON(ctx, "douban:v1:subject:"+kind+":"+id, detailTTL, c.cfg.DetailURL+"/"+kind+"/"+id,
		"https://m.douban.com/movie/subject/"+id+"/", &d); err != nil {
		return nil, err
	}
	if d.ID == "" {
		return nil, errors.New("豆瓣返回的条目是空的")
	}
	s := &Subject{ID: d.ID, Title: d.Title, OriginalTitle: d.OriginalTitle, Intro: strings.TrimSpace(d.Intro)}
	s.Year, _ = strconv.Atoi(strings.TrimSpace(d.Year))
	if d.Rating != nil {
		s.Rating, s.Votes = d.Rating.Value, d.Rating.Count
	}
	return s, nil
}

func (c *Client) suggest(ctx context.Context, name string) ([]suggestion, error) {
	sum := sha256.Sum256([]byte(name))
	var out []suggestion
	err := c.getJSON(ctx, "douban:v1:suggest:"+hex.EncodeToString(sum[:16]), searchTTL,
		c.cfg.SearchURL+"?q="+url.QueryEscape(name), "https://movie.douban.com/", &out)
	return out, err
}

// getJSON 先看缓存，没有时请求 u 并把响应按 JSON 解析进 out；解析得了才写缓存。
func (c *Client) getJSON(ctx context.Context, key string, ttl time.Duration, u, referer string, out any) error {
	if c.cfg.Cache != nil && !cachectl.Bypassed(ctx) {
		if b, ok := c.cfg.Cache.Get(key); ok && json.Unmarshal(b, out) == nil {
			return nil
		}
	}
	body, err := c.get(ctx, u, referer)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("豆瓣返回的不是预期的内容（可能被限流了）: %w", err)
	}
	if c.cfg.Cache != nil {
		c.cfg.Cache.Set(key, body, ttl)
	}
	return nil
}

func (c *Client) get(ctx context.Context, u, referer string) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "application/json")
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上豆瓣: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == 418:
		return nil, fmt.Errorf("豆瓣拒绝了请求（HTTP %d），可能是请求太多，过一阵再试", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("豆瓣返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("读取豆瓣的响应失败: %w", err)
	}
	if len(body) > maxBody {
		return nil, errors.New("豆瓣的响应太大")
	}
	return body, nil
}

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

var (
	seasonRe    = regexp.MustCompile(`\s*第\s*([0-9零一二两三四五六七八九十百]+)\s*季\s*$`)
	subSeasonRe = regexp.MustCompile(`(?i)\s*season\s*\d+\s*$`)
)

// Matches 报告这个条目就是要找的那部（例如站点给的豆瓣编号取到的条目）：名字（标题去掉季，或原名去掉 Season N）
// 和 TMDB 的中文名或原名之一相同，年份差一年以内（有一边不知道时只看名字）。
func (s *Subject) Matches(q Query) bool {
	title := s.Title
	if m := seasonRe.FindStringIndex(title); m != nil {
		title = title[:m[0]]
	}
	if !sameName(q, title, subSeasonRe.ReplaceAllString(s.OriginalTitle, "")) {
		return false
	}
	return q.Year == 0 || s.Year == 0 || (s.Year >= q.Year-1 && s.Year <= q.Year+1)
}

// match 在搜索结果里挑出对得上的条目编号：种类（剧集有集数）、季（标题里的「第 N 季」，没写是第 1 季）、
// 名字（豆瓣的标题去掉季，或豆瓣给的原名，和 TMDB 的中文名或原名之一相同，不看大小写、空格与标点）都对得上的里面，
// 年份相同的优先；没有年份相同的时，差一年以内的只有一个才用它。没给年份时，对得上的只有一个才用它。
// 拿不准（同名的不止一个）或没有时返回空串，调用方用 TMDB 的。
func match(cands []suggestion, q Query) string {
	want := max(q.Season, 1)
	var exact, near, any []string
	for _, c := range cands {
		if c.Type != "movie" {
			// 搜索结果里电影与剧集都是 movie，别的（书、音乐等）不要
			continue
		}
		if tv := c.Episode != ""; tv != (q.Kind == KindTV) {
			continue
		}
		title, season := c.Title, 1
		if m := seasonRe.FindStringSubmatchIndex(title); m != nil {
			season = cnNumber(title[m[2]:m[3]])
			title = title[:m[0]]
		}
		if q.Kind == KindTV && season != want {
			continue
		}
		if !sameName(q, title, subSeasonRe.ReplaceAllString(c.SubTitle, "")) {
			continue
		}
		any = append(any, c.ID)
		y, _ := strconv.Atoi(strings.TrimSpace(c.Year))
		switch {
		case q.Year == 0 || y == 0:
		case y == q.Year:
			exact = append(exact, c.ID)
		case y >= q.Year-1 && y <= q.Year+1:
			near = append(near, c.ID)
		}
	}
	switch {
	case q.Year == 0:
		if len(any) == 1 {
			return any[0]
		}
	case len(exact) > 0:
		return exact[0]
	case len(near) == 1:
		return near[0]
	}
	return ""
}

func sameName(q Query, names ...string) bool {
	for _, n := range names {
		n = norm(n)
		if n != "" && (n == norm(q.Title) || n == norm(q.OriginalTitle)) {
			return true
		}
	}
	return false
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

// cnNumber 把「三」「十二」「二十一」或阿拉伯数字换成数；认不出时为 0。
func cnNumber(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	digits := map[rune]int{'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	n, cur := 0, 0
	for _, r := range s {
		switch r {
		case '十', '百':
			unit := 10
			if r == '百' {
				unit = 100
			}
			if cur == 0 {
				cur = 1
			}
			n += cur * unit
			cur = 0
		default:
			d, ok := digits[r]
			if !ok {
				return 0
			}
			cur = d
		}
	}
	return n + cur
}
