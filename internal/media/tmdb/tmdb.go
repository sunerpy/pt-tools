// Package tmdb 是 TMDB v3 接口的客户端：搜索电影与剧集、按 IMDb 编号查找、取详情。
// API Key 由用户自己填写（v3 的 API Key 或 v4 的读取令牌都可以）；默认中文，中文简介为空时取英文的。
// 走设置里的代理，没有时用环境变量里的代理；响应缓存在 SQLite 里；自己限速。错误里不带请求地址（API Key 在地址里）。
package tmdb

import (
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

	"golang.org/x/time/rate"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// DefaultBaseURL 是 TMDB v3 接口地址；ImageBaseURL 是图片地址。
const (
	DefaultBaseURL = "https://api.themoviedb.org/3"
	ImageBaseURL   = "https://image.tmdb.org/t/p/"
)

// Kind 是条目类型。
const (
	KindMovie = "movie"
	KindTV    = "tv"
)

// 缓存时长：搜索结果一天，查找与详情一周。
const (
	SearchTTL  = 24 * time.Hour
	DetailsTTL = 7 * 24 * time.Hour
)

var (
	ErrNoKey        = errors.New("没有填写 TMDB API Key")
	ErrUnauthorized = errors.New("TMDB API Key 无效")
	ErrNotFound     = errors.New("TMDB 上没有这个条目")
	ErrRateLimited  = errors.New("TMDB 请求太频繁，稍后再试")
)

// Result 是一个电影或剧集条目（搜索、查找与详情共用）。
type Result struct {
	ID               int     `json:"id"`
	MediaType        string  `json:"media_type"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	Year             int     `json:"year,omitempty"`
	Date             string  `json:"date,omitempty"`
	Overview         string  `json:"overview,omitempty"`
	PosterPath       string  `json:"poster_path,omitempty"`
	Popularity       float64 `json:"popularity,omitempty"`
	VoteAverage      float64 `json:"vote_average,omitempty"`
	OriginalLanguage string  `json:"original_language,omitempty"`
	IMDbID           string  `json:"imdb_id,omitempty"`
	Seasons          int     `json:"seasons,omitempty"`
}

// PosterURL 是海报图片地址（size 为空时用 w342）。
func PosterURL(path, size string) string {
	if path == "" {
		return ""
	}
	if size == "" {
		size = "w342"
	}
	return ImageBaseURL + size + "/" + strings.TrimPrefix(path, "/")
}

// Options 是客户端的选项。
type Options struct {
	APIKey   string
	BaseURL  string // 为空时用 DefaultBaseURL
	Language string // 为空时用 zh-CN
	ProxyURL string // 为空时用环境变量里的代理
	Cache    Cache  // 为空时不缓存
	// RatePerSecond 是每秒最多几个请求（为 0 时 8 个）；读缓存不算。
	RatePerSecond float64
}

// Client 是 TMDB 客户端。
type Client struct {
	base     string
	key      string
	bearer   bool
	language string
	http     *http.Client
	cache    Cache
	limiter  *rate.Limiter
}

// New 建一个客户端。没有 API Key 时返回 ErrNoKey。
func New(o Options) (*Client, error) {
	key := strings.TrimSpace(o.APIKey)
	if key == "" {
		return nil, ErrNoKey
	}
	var proxy func(*http.Request) (*url.URL, error)
	if p := strings.TrimSpace(o.ProxyURL); p != "" {
		u, err := ParseProxyURL(p)
		if err != nil {
			return nil, err
		}
		proxy = http.ProxyURL(u)
	} else {
		proxy = func(r *http.Request) (*url.URL, error) {
			if s := httpclient.ResolveProxyFromEnvironment(r.URL.String()); s != "" {
				return url.Parse(s)
			}
			return nil, nil
		}
	}
	base := strings.TrimRight(strings.TrimSpace(o.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	lang := strings.TrimSpace(o.Language)
	if lang == "" {
		lang = "zh-CN"
	}
	perSec := o.RatePerSecond
	if perSec <= 0 {
		perSec = 8
	}
	burst := max(int(perSec), 1)
	return &Client{
		base:     base,
		key:      key,
		bearer:   strings.HasPrefix(key, "eyJ"),
		language: lang,
		http: &http.Client{
			Timeout:   20 * time.Second,
			Transport: &http.Transport{Proxy: proxy, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second},
		},
		cache:   o.Cache,
		limiter: rate.NewLimiter(rate.Limit(perSec), burst),
	}, nil
}

// ParseProxyURL 校验代理地址：http、https、socks5 或 socks5h，不能带路径以外的多余部分。
func ParseProxyURL(s string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" {
		return nil, errors.New("代理地址格式不对")
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return u, nil
	}
	return nil, errors.New("代理地址要以 http://、https:// 或 socks5:// 开头")
}

// Validate 用 /configuration 检查 API Key 是否可用。
func (c *Client) Validate(ctx context.Context) error {
	var out map[string]any
	return c.get(ctx, "/configuration", url.Values{}, &out)
}

type rawItem struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	Name             string  `json:"name"`
	OriginalTitle    string  `json:"original_title"`
	OriginalName     string  `json:"original_name"`
	ReleaseDate      string  `json:"release_date"`
	FirstAirDate     string  `json:"first_air_date"`
	Overview         string  `json:"overview"`
	PosterPath       string  `json:"poster_path"`
	Popularity       float64 `json:"popularity"`
	VoteAverage      float64 `json:"vote_average"`
	OriginalLanguage string  `json:"original_language"`
	IMDbID           string  `json:"imdb_id"`
	NumberOfSeasons  int     `json:"number_of_seasons"`
	ExternalIDs      struct {
		IMDbID string `json:"imdb_id"`
	} `json:"external_ids"`
}

func (r rawItem) result(kind string) Result {
	out := Result{
		ID: r.ID, MediaType: kind, Overview: r.Overview, PosterPath: r.PosterPath, Popularity: r.Popularity,
		VoteAverage: r.VoteAverage, OriginalLanguage: r.OriginalLanguage, Seasons: r.NumberOfSeasons,
	}
	if kind == KindTV {
		out.Title, out.OriginalTitle, out.Date = r.Name, r.OriginalName, r.FirstAirDate
	} else {
		out.Title, out.OriginalTitle, out.Date = r.Title, r.OriginalTitle, r.ReleaseDate
	}
	if len(out.Date) >= 4 {
		out.Year, _ = strconv.Atoi(out.Date[:4])
	}
	out.IMDbID = v2.NormalizeIMDbID(r.IMDbID)
	if out.IMDbID == "" {
		out.IMDbID = v2.NormalizeIMDbID(r.ExternalIDs.IMDbID)
	}
	return out
}

func checkKind(kind string) error {
	if kind != KindMovie && kind != KindTV {
		return fmt.Errorf("条目类型无效: %q", kind)
	}
	return nil
}

// Search 按名字搜索电影或剧集。电影带上年份过滤；剧集不带（首播年份过滤会漏掉后面几季）。
func (c *Client) Search(ctx context.Context, kind, query string, year int) ([]Result, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	q := url.Values{"query": {strings.TrimSpace(query)}, "include_adult": {"false"}, "page": {"1"}, "language": {c.language}}
	if kind == KindMovie && year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	var out []Result
	err := c.cached(ctx, "search:"+kind+":"+q.Encode(), SearchTTL, &out, func() (any, error) {
		var raw struct {
			Results []rawItem `json:"results"`
		}
		if err := c.get(ctx, "/search/"+kind, q, &raw); err != nil {
			return nil, err
		}
		res := make([]Result, 0, len(raw.Results))
		for _, r := range raw.Results {
			res = append(res, r.result(kind))
		}
		return res, nil
	})
	return out, err
}

// Find 按 IMDb 编号查找（链接也可以），电影在前、剧集在后。
func (c *Client) Find(ctx context.Context, imdb string) ([]Result, error) {
	id := v2.NormalizeIMDbID(imdb)
	if id == "" {
		return nil, fmt.Errorf("IMDb 编号格式不对: %q", imdb)
	}
	q := url.Values{"external_source": {"imdb_id"}, "language": {c.language}}
	var out []Result
	err := c.cached(ctx, "find:"+id+":"+c.language, DetailsTTL, &out, func() (any, error) {
		var raw struct {
			MovieResults []rawItem `json:"movie_results"`
			TVResults    []rawItem `json:"tv_results"`
		}
		if err := c.get(ctx, "/find/"+id, q, &raw); err != nil {
			return nil, err
		}
		res := make([]Result, 0, len(raw.MovieResults)+len(raw.TVResults))
		for _, r := range raw.MovieResults {
			res = append(res, r.result(KindMovie))
		}
		for _, r := range raw.TVResults {
			res = append(res, r.result(KindTV))
		}
		return res, nil
	})
	return out, err
}

// Details 取一个条目的详情（带 IMDb 编号）。中文简介为空时取英文的。
func (c *Client) Details(ctx context.Context, kind string, id int) (*Result, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	var out Result
	err := c.cached(ctx, fmt.Sprintf("details:%s:%d:%s", kind, id, c.language), DetailsTTL, &out, func() (any, error) {
		r, err := c.details(ctx, kind, id, c.language)
		if err != nil {
			return nil, err
		}
		if (r.Overview == "" || r.Title == "") && !strings.HasPrefix(c.language, "en") {
			if en, err := c.details(ctx, kind, id, "en-US"); err == nil {
				if r.Overview == "" {
					r.Overview = en.Overview
				}
				if r.Title == "" {
					r.Title = en.Title
				}
			}
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) details(ctx context.Context, kind string, id int, lang string) (Result, error) {
	var raw rawItem
	q := url.Values{"append_to_response": {"external_ids"}, "language": {lang}}
	if err := c.get(ctx, fmt.Sprintf("/%s/%d", kind, id), q, &raw); err != nil {
		return Result{}, err
	}
	return raw.result(kind), nil
}

// cached 读缓存；没有时调用 fetch，成功的结果写进缓存。
func (c *Client) cached(ctx context.Context, key string, ttl time.Duration, out any, fetch func() (any, error)) error {
	key = "tmdb:v1:" + key
	if c.cache != nil {
		if b, ok := c.cache.Get(key); ok && json.Unmarshal(b, out) == nil {
			return nil
		}
	}
	v, err := fetch()
	if err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if c.cache != nil {
		c.cache.Set(key, b, ttl)
	}
	return json.Unmarshal(b, out)
}

// maxBody 是 TMDB 响应最多读多少字节。
const maxBody = 4 << 20

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if !c.bearer {
		q.Set("api_key", c.key)
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("等待 TMDB 限速: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		// 解析错误会带上整个地址（含 API Key），不往外传
		return errors.New("TMDB 接口地址格式不对")
	}
	req.Header.Set("Accept", "application/json")
	if c.bearer {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("请求 TMDB 失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("读取 TMDB 响应失败: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("TMDB 返回 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 TMDB 响应失败: %w", err)
	}
	return nil
}
