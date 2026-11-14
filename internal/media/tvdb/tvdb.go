// Package tvdb 从 TheTVDB（API v4）取剧集每一集的标题、简介、播出日期与截图，刮削时补 TMDB 没有的。
//
// 先用 API Key 登录（POST /login；user-supported 的 Key 还要带订阅 PIN，没有 PIN 时请求里不带 pin），拿到的令牌
// 有效一个月，存在内存里（同一个服务建的客户端共用），之后的请求带 Authorization: Bearer。令牌过期或失效（401）时
// 重新登录一次。每一集按播出顺序（official，即 Aired Order）与语言取，按页翻，每页缓存一天。
// 错误里不带 Key、PIN 与令牌。图片只从 TheTVDB 自己的图片服务器下载，不带令牌，最多 10 MiB。
package tvdb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

// 默认的接口地址、图片地址与几个上限。
const (
	DefaultBaseURL = "https://api4.thetvdb.com/v4"
	imageBase      = "https://artworks.thetvdb.com"
	episodesTTL    = 24 * time.Hour
	// tokenTTL 是令牌在内存里用多久：TheTVDB 给的是一个月，提前几天换
	tokenTTL = 25 * 24 * time.Hour
	maxBody  = 8 << 20
	maxImage = 10 << 20
	// MaxPages 是一部剧最多取几页（第 0 到第 19 页）
	MaxPages = 20
	// validateSeries 是测试 Key 时取的剧集（Breaking Bad）
	validateSeries = 81189
	// DefaultRatePerSecond 是默认限速（每秒请求数，读缓存不算）
	DefaultRatePerSecond = 4
)

var (
	ErrNoKey = errors.New("没有填写 TheTVDB API Key")
	// ErrUnauthorized 表示登录不了：Key 不对，或者是要订阅 PIN 的 Key 没填 PIN、PIN 不对
	ErrUnauthorized = errors.New("TheTVDB 登录不了：API Key 或订阅 PIN 不对")
	ErrRateLimited  = errors.New("TheTVDB 的请求太频繁，稍后再试")
	// ErrUnavailable 表示连不上 TheTVDB，或它回了别的错误
	ErrUnavailable = errors.New("TheTVDB 暂时不能访问")
	// ErrNotImage 表示图片地址回的不是图片
	ErrNotImage = errors.New("TheTVDB 图片地址返回的不是图片")
	// ErrTooMany 表示一部剧的集太多，MaxPages 页还没取完
	ErrTooMany = fmt.Errorf("TheTVDB 上这部的集太多（超过 %d 页）", MaxPages)
)

// Cache 是响应缓存（和 TMDB 共用 media_cache 表）。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Tokens 是登录令牌的内存表：同一个服务建的客户端共用，不用每次刮削都登录。按接口地址、Key 与 PIN 分开存。
type Tokens struct {
	mu sync.Mutex
	m  map[string]token
}

type token struct {
	value   string
	expires time.Time
}

// NewTokens 建一个空的令牌表。
func NewTokens() *Tokens { return &Tokens{m: map[string]token{}} }

func (t *Tokens) get(k string, now time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if v, ok := t.m[k]; ok && now.Before(v.expires) {
		return v.value
	}
	return ""
}

func (t *Tokens) set(k, v string, expires time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[k] = token{value: v, expires: expires}
}

// drop 删掉失效的令牌：只删还是 v 的那个（别的请求已经换了新的就不删）。
func (t *Tokens) drop(k, v string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, ok := t.m[k]; ok && cur.value == v {
		delete(t.m, k)
	}
}

// NewLimiter 建一个限速器（perSec 为 0 时用 DefaultRatePerSecond）。
func NewLimiter(perSec float64) *rate.Limiter {
	if perSec <= 0 {
		perSec = DefaultRatePerSecond
	}
	return rate.NewLimiter(rate.Limit(perSec), max(int(perSec), 1))
}

// Options 是客户端的选项。
type Options struct {
	APIKey string
	// PIN 是订阅 PIN（user-supported 的 Key 才要），可以为空
	PIN     string
	BaseURL string
	// ImageHosts 是允许下载图片的主机；为空时只认 https 的 artworks.thetvdb.com 与 *.thetvdb.com
	ImageHosts []string
	HTTP       *http.Client
	Cache      Cache
	// Tokens 为空时这个客户端自己建一个
	Tokens *Tokens
	// Limiter 为空时这个客户端自己建一个（DefaultRatePerSecond）
	Limiter *rate.Limiter
	// Now 为空时用 time.Now
	Now func() time.Time
}

// Client 是 TheTVDB 客户端。
type Client struct {
	key, pin   string
	base       string
	imageHosts []string
	http       *http.Client
	cache      Cache
	tokens     *Tokens
	tokenKey   string
	limiter    *rate.Limiter
	now        func() time.Time
}

// New 建一个客户端；没有 API Key 时返回 ErrNoKey。
func New(o Options) (*Client, error) {
	key := strings.TrimSpace(o.APIKey)
	if key == "" {
		return nil, ErrNoKey
	}
	base := strings.TrimRight(strings.TrimSpace(o.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	c := &Client{
		key: key, pin: strings.TrimSpace(o.PIN), base: base, imageHosts: o.ImageHosts, http: o.HTTP, cache: o.Cache,
		tokens: o.Tokens, limiter: o.Limiter, now: o.Now,
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 20 * time.Second}
	}
	if c.tokens == nil {
		c.tokens = NewTokens()
	}
	if c.limiter == nil {
		c.limiter = NewLimiter(0)
	}
	if c.now == nil {
		c.now = time.Now
	}
	sum := sha256.Sum256([]byte(base + "\x00" + key + "\x00" + c.pin))
	c.tokenKey = hex.EncodeToString(sum[:])
	return c, nil
}

// Episode 是一集。Name 与 Overview 是请求的语言的（没有这种语言时接口可能给原语言的，见 NameLangs）。
type Episode struct {
	ID     int
	Season int
	Number int
	Name   string
	// Overview 是简介
	Overview string
	// Aired 是播出日期（yyyy-mm-dd，没有时为空）
	Aired string
	// Image 是截图的完整地址（见 ImageURL；没有时为空）
	Image string
	// NameLangs、OverviewLangs 是 TheTVDB 上这一集有标题、简介的语言（三个字母的代码，例如 zho、eng）
	NameLangs     []string
	OverviewLangs []string
}

// SeriesEpisodes 是一部剧按播出顺序的每一集。没有这部时 Episodes 为空。
type SeriesEpisodes struct {
	SeriesID int
	// OriginalLanguage 是剧集的原语言（三个字母的代码）
	OriginalLanguage string
	Episodes         []Episode
	// Untranslated 表示带语言的列表回了 404、这是不带语言的列表：Name 与 Overview 是原语言的（截图照样能用）
	Untranslated bool
}

type rawEpisode struct {
	ID                   int      `json:"id"`
	SeasonNumber         int      `json:"seasonNumber"`
	Number               int      `json:"number"`
	Name                 string   `json:"name"`
	Overview             string   `json:"overview"`
	Aired                string   `json:"aired"`
	Image                string   `json:"image"`
	NameTranslations     []string `json:"nameTranslations"`
	OverviewTranslations []string `json:"overviewTranslations"`
}

type episodesPage struct {
	Data struct {
		Series *struct {
			ID               int          `json:"id"`
			OriginalLanguage string       `json:"originalLanguage"`
			Episodes         []rawEpisode `json:"episodes"`
		} `json:"series"`
		// Episodes 是不带语言的接口的写法（data.episodes），带语言的在 data.series.episodes
		Episodes []rawEpisode `json:"episodes"`
	} `json:"data"`
	Links json.RawMessage `json:"links"`
	// NotFound 是 pt-tools 自己记进缓存的：这一页回了 404
	NotFound bool `json:"pt_tools_not_found"`
}

// hasNext 报告响应顶层的 links 说还有下一页；第二个值报告响应里有没有 links。
func (p *episodesPage) hasNext() (more, present bool) {
	if len(p.Links) == 0 || string(p.Links) == "null" {
		return false, false
	}
	var l struct {
		Next json.RawMessage `json:"next"`
	}
	if json.Unmarshal(p.Links, &l) != nil {
		return false, true
	}
	var next string
	if json.Unmarshal(l.Next, &next) != nil {
		return false, true
	}
	return strings.TrimSpace(next) != "", true
}

var langRe = regexp.MustCompile(`^[a-z]{3,4}$`)

// notFoundBody 是缓存里记下「这一页 404」的写法。
var notFoundBody = []byte(`{"pt_tools_not_found":true}`)

// Episodes 取一部剧按播出顺序（official）的每一集，lang 是三个字母的语言代码（例如 zho、eng）。从第 0 页翻起：
// 响应顶层有 links 时按 links.next 判断还有没有下一页（不跟它给的地址）；没有 links 时这一页有没见过的集就接着翻。
// 后面的页 404 就停。带语言的列表第 0 页就 404 时（TheTVDB 有剧集明明在、这个接口却回 404 的问题，thetvdb/v4-api#324），
// 改取不带语言的列表（Untranslated 为真）；它也 404 才是没有这部（返回空的结果）。超过 MaxPages 页时返回 ErrTooMany。
func (c *Client) Episodes(ctx context.Context, seriesID int, lang string) (*SeriesEpisodes, error) {
	if seriesID <= 0 {
		return nil, fmt.Errorf("TVDB 编号不对: %d", seriesID)
	}
	if !langRe.MatchString(lang) {
		return nil, fmt.Errorf("TheTVDB 语言代码不对: %q", lang)
	}
	out, err := c.episodes(ctx, seriesID, lang)
	if err != nil || out != nil {
		return out, err
	}
	if out, err = c.episodes(ctx, seriesID, ""); err != nil || out == nil {
		return &SeriesEpisodes{SeriesID: seriesID}, err
	}
	out.Untranslated = true
	return out, nil
}

// episodes 按页取一种列表（lang 为空时是不带语言的）；第 0 页 404 时返回 nil、nil。
func (c *Client) episodes(ctx context.Context, seriesID int, lang string) (*SeriesEpisodes, error) {
	out := &SeriesEpisodes{SeriesID: seriesID}
	seen := map[int]bool{}
	for page := 0; page < MaxPages; page++ {
		p, err := c.episodesPage(ctx, seriesID, lang, page)
		if err != nil {
			return nil, err
		}
		if p.NotFound {
			if page == 0 {
				return nil, nil
			}
			return out, nil
		}
		eps := p.Data.Episodes
		if s := p.Data.Series; s != nil {
			eps = append(eps, s.Episodes...)
			if out.OriginalLanguage == "" {
				out.OriginalLanguage = strings.ToLower(strings.TrimSpace(s.OriginalLanguage))
			}
		}
		fresh := false
		for _, e := range eps {
			key := e.ID
			if key == 0 {
				key = -(e.SeasonNumber*100000 + e.Number)
			}
			if seen[key] {
				continue
			}
			seen[key], fresh = true, true
			out.Episodes = append(out.Episodes, Episode{
				ID: e.ID, Season: e.SeasonNumber, Number: e.Number, Name: strings.TrimSpace(e.Name), Overview: strings.TrimSpace(e.Overview),
				Aired: strings.TrimSpace(e.Aired), Image: c.ImageURL(e.Image), NameLangs: lower(e.NameTranslations), OverviewLangs: lower(e.OverviewTranslations),
			})
		}
		more, present := p.hasNext()
		if !present {
			more = len(eps) > 0 && fresh
		}
		if !more {
			return out, nil
		}
	}
	return nil, ErrTooMany
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lower(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// episodesPage 取一页（先看缓存）：404 记成 NotFound。lang 为空时是不带语言的列表。
func (c *Client) episodesPage(ctx context.Context, seriesID int, lang string, page int) (*episodesPage, error) {
	key := fmt.Sprintf("tvdb:v1:episodes:%d:official:%s:%d", seriesID, firstNonEmpty(lang, "base"), page)
	path := fmt.Sprintf("%s/series/%d/episodes/official/%s?page=%d", c.base, seriesID, lang, page)
	if lang == "" {
		path = fmt.Sprintf("%s/series/%d/episodes/official?page=%d", c.base, seriesID, page)
	}
	var p episodesPage
	if c.cache != nil && !cachectl.Bypassed(ctx) {
		if b, ok := c.cache.Get(key); ok && json.Unmarshal(b, &p) == nil {
			return &p, nil
		}
	}
	body, status, err := c.authGet(ctx, path)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		body = notFoundBody
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("解析 TheTVDB 的响应失败: %w", err)
	}
	if c.cache != nil {
		c.cache.Set(key, body, episodesTTL)
	}
	return &p, nil
}

// token 是能用的令牌：内存里有就用（fresh 为真时不用内存里的），否则登录一次。
func (c *Client) token(ctx context.Context, fresh bool) (string, error) {
	if !fresh {
		if t := c.tokens.get(c.tokenKey, c.now()); t != "" {
			return t, nil
		}
	}
	return c.login(ctx)
}

// login 登录：请求体是 apikey，有 PIN 时再加 pin（没有 PIN 时不带这个字段）。
func (c *Client) login(ctx context.Context) (string, error) {
	in := map[string]string{"apikey": c.key}
	if c.pin != "" {
		in["pin"] = c.pin
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/login", bytes.NewReader(payload))
	if err != nil {
		return "", errors.New("TheTVDB 接口地址格式不对")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	body, status, err := c.send(ctx, req)
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ErrUnauthorized
	default:
		return "", statusErr(status)
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil || strings.TrimSpace(out.Data.Token) == "" {
		return "", fmt.Errorf("%w: 登录的响应里没有令牌", ErrUnavailable)
	}
	t := strings.TrimSpace(out.Data.Token)
	c.tokens.set(c.tokenKey, t, c.now().Add(tokenTTL))
	return t, nil
}

// authGet 带令牌发 GET：200 与 404 返回响应；401 时重新登录再试一次，还是 401 就是 ErrUnauthorized。
func (c *Client) authGet(ctx context.Context, u string) ([]byte, int, error) {
	t, err := c.token(ctx, false)
	if err != nil {
		return nil, 0, err
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, 0, errors.New("TheTVDB 接口地址格式不对")
		}
		req.Header.Set("Authorization", "Bearer "+t)
		req.Header.Set("Accept", "application/json")
		body, status, err := c.send(ctx, req)
		if err != nil {
			return nil, 0, err
		}
		switch status {
		case http.StatusOK, http.StatusNotFound:
			return body, status, nil
		case http.StatusUnauthorized:
			c.tokens.drop(c.tokenKey, t)
			if attempt > 0 {
				return nil, 0, ErrUnauthorized
			}
			if t, err = c.token(ctx, true); err != nil {
				return nil, 0, err
			}
		default:
			return nil, 0, statusErr(status)
		}
	}
}

func statusErr(status int) error {
	if status == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	return fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
}

// send 限速后发请求，只跟同源的跳转（请求头或请求体里有 Key 与令牌）。错误里不带地址。
func (c *Client) send(ctx context.Context, req *http.Request) ([]byte, int, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, 0, err
	}
	origin := req.URL
	resp, err := c.guarded(func(u *url.URL) bool { return sameOrigin(u, origin) }).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, 0, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, 0, fmt.Errorf("读取 TheTVDB 的响应失败: %w", err)
	}
	if len(body) > maxBody {
		return nil, 0, errors.New("TheTVDB 的响应太大")
	}
	return body, resp.StatusCode, nil
}

// sameOrigin 报告两个地址的协议与主机（含端口）是不是一样。
func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// guarded 是只跟 allow 认可的跳转（最多 5 次）的客户端（和 c.http 共用连接）。
func (c *Client) guarded(allow func(*url.URL) bool) *http.Client {
	hc := *c.http
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allow(req.URL) {
			return errors.New("TheTVDB 跳转到了别的地址，没有跟过去")
		}
		return nil
	}
	return &hc
}

// ImageURL 把接口给的图片地址规整成完整的地址：/banners/… 这样的路径补上 https://artworks.thetvdb.com；
// 没有配置图片主机时 http:// 换成 https://。空串还是空串。
func (c *Client) ImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return ""
	case strings.HasPrefix(raw, "/"):
		return imageBase + raw
	case len(c.imageHosts) == 0 && strings.HasPrefix(raw, "http://"):
		return "https://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
}

// allowedImage 报告图片地址是不是 TheTVDB 自己的图片服务器（或配置里允许的主机）。
func (c *Client) allowedImage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if len(c.imageHosts) > 0 {
		for _, h := range c.imageHosts {
			if strings.EqualFold(h, u.Host) || strings.EqualFold(h, host) {
				return true
			}
		}
		return false
	}
	return u.Scheme == "https" && (host == "thetvdb.com" || strings.HasSuffix(host, ".thetvdb.com"))
}

// Download 下载一张图片（地址先按 ImageURL 规整）：只认 TheTVDB 自己的图片服务器，不带令牌，只接受 image/* 响应，最多 10 MiB。
func (c *Client) Download(ctx context.Context, raw string) ([]byte, error) {
	raw = c.ImageURL(raw)
	if !c.allowedImage(raw) {
		return nil, fmt.Errorf("不是 TheTVDB 的图片地址: %q", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("TheTVDB 图片地址格式不对")
	}
	resp, err := c.guarded(func(u *url.URL) bool { return c.allowedImage(u.String()) }).Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: 图片 HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); !strings.HasPrefix(mt, "image/") {
		return nil, ErrNotImage
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return nil, fmt.Errorf("读取 TheTVDB 图片失败: %w", err)
	}
	if len(body) > maxImage {
		return nil, errors.New("TheTVDB 图片超过 10 MiB")
	}
	return body, nil
}

// Validate 检查 Key 与 PIN 能不能用：不用内存里的令牌，重新登录一次，再取一部固定的剧（不读缓存）。
func (c *Client) Validate(ctx context.Context) error {
	if _, err := c.token(ctx, true); err != nil {
		return err
	}
	_, status, err := c.authGet(ctx, c.base+"/series/"+strconv.Itoa(validateSeries))
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
	}
	return nil
}
