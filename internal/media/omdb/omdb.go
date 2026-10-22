// Package omdb 从 OMDb（omdbapi.com）按 IMDb 编号取评分与分级：IMDb 评分与人数、烂番茄、Metacritic。
//
// 要用户自己的 API Key（免费的每天 1000 次）。Key 在地址里，错误里不带地址。结果缓存一周。
package omdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

// 默认的接口地址与缓存时间。
const (
	DefaultBaseURL = "https://www.omdbapi.com/"
	titleTTL       = 7 * 24 * time.Hour
	maxBody        = 1 << 20
)

var (
	ErrNoKey        = errors.New("没有填写 OMDb API Key")
	ErrUnauthorized = errors.New("OMDb API Key 无效")
	ErrRateLimited  = errors.New("OMDb 今天的请求次数用完了")
	// ErrUnavailable 表示连不上 OMDb，或它回了别的错误
	ErrUnavailable = errors.New("OMDb 暂时不能访问")
)

// Cache 是响应缓存（和 TMDB 共用 media_cache 表）。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Options 是客户端的选项。
type Options struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
	Cache   Cache
}

// Client 是 OMDb 客户端。
type Client struct {
	key   string
	base  string
	http  *http.Client
	cache Cache
}

// New 建一个客户端；没有 API Key 时返回 ErrNoKey。
func New(o Options) (*Client, error) {
	key := strings.TrimSpace(o.APIKey)
	if key == "" {
		return nil, ErrNoKey
	}
	base := strings.TrimSpace(o.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	// Key 在地址里：跳转到别的地址（换了域名、端口或协议）时不跟，免得 Key 随跳转与 Referer 带过去
	guarded := *hc
	guarded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		first := via[0].URL
		if len(via) >= 5 || !strings.EqualFold(req.URL.Scheme, first.Scheme) || !strings.EqualFold(req.URL.Host, first.Host) {
			return errors.New("OMDb 跳转到了别的地址，没有跟过去")
		}
		return nil
	}
	return &Client{key: key, base: base, http: &guarded, cache: o.Cache}, nil
}

// Title 是一部电影或剧集的评分与分级。没有的项为零值。
type Title struct {
	IMDbID string `json:"imdb_id"`
	Title  string `json:"title"`
	// Rated 是分级（例如 PG-13、TV-MA），N/A、Not Rated、Unrated 记成空
	Rated      string  `json:"rated,omitempty"`
	IMDbRating float64 `json:"imdb_rating,omitempty"`
	IMDbVotes  int     `json:"imdb_votes,omitempty"`
	// RottenTomatoes 是烂番茄的新鲜度（百分比）
	RottenTomatoes int `json:"rotten_tomatoes,omitempty"`
	// Metacritic 是 Metacritic 的评分（百分制）
	Metacritic int `json:"metacritic,omitempty"`
}

var imdbRe = regexp.MustCompile(`^tt\d{5,12}$`)

// ByIMDb 按 IMDb 编号取。OMDb 上没有时返回 nil、nil。
func (c *Client) ByIMDb(ctx context.Context, imdbID string) (*Title, error) {
	imdbID = strings.TrimSpace(imdbID)
	if !imdbRe.MatchString(imdbID) {
		return nil, fmt.Errorf("IMDb 编号格式不对: %q", imdbID)
	}
	key := "omdb:v1:" + imdbID
	var raw rawTitle
	if c.cache != nil && !cachectl.Bypassed(ctx) {
		if b, ok := c.cache.Get(key); ok && json.Unmarshal(b, &raw) == nil {
			return raw.title(), nil
		}
	}
	body, err := c.get(ctx, url.Values{"i": {imdbID}, "r": {"json"}, "plot": {"short"}})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("OMDb 返回的不是预期的内容: %w", err)
	}
	if raw.Response != "True" {
		msg := strings.ToLower(raw.Error)
		switch {
		case strings.Contains(msg, "api key"):
			return nil, ErrUnauthorized
		case strings.Contains(msg, "limit"):
			return nil, ErrRateLimited
		case strings.Contains(msg, "not found") || strings.Contains(msg, "incorrect imdb"):
			// 没有这部：也缓存（Response 为 False 的那份），免得每次都问
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnavailable, raw.Error)
		}
	}
	if c.cache != nil {
		c.cache.Set(key, body, titleTTL)
	}
	return raw.title(), nil
}

type rawTitle struct {
	Response   string `json:"Response"`
	Error      string `json:"Error"`
	Title      string `json:"Title"`
	IMDbID     string `json:"imdbID"`
	Rated      string `json:"Rated"`
	IMDbRating string `json:"imdbRating"`
	IMDbVotes  string `json:"imdbVotes"`
	Metascore  string `json:"Metascore"`
	Ratings    []struct {
		Source string `json:"Source"`
		Value  string `json:"Value"`
	} `json:"Ratings"`
}

// title 是解析好的结果；Response 不是 True（没有这部）时返回 nil。
func (r rawTitle) title() *Title {
	if r.Response != "True" {
		return nil
	}
	t := &Title{IMDbID: r.IMDbID, Title: r.Title}
	switch rated := strings.TrimSpace(r.Rated); strings.ToLower(rated) {
	case "", "n/a", "not rated", "unrated":
	default:
		t.Rated = rated
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(r.IMDbRating), 64); err == nil && v > 0 && v <= 10 {
		t.IMDbRating = v
	}
	if v, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(r.IMDbVotes), ",", "")); err == nil && v > 0 {
		t.IMDbVotes = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(r.Metascore)); err == nil && v > 0 && v <= 100 {
		t.Metacritic = v
	}
	for _, x := range r.Ratings {
		if x.Source == "Rotten Tomatoes" {
			if v, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(x.Value), "%")); err == nil && v >= 0 && v <= 100 {
				t.RottenTomatoes = v
			}
		}
	}
	return t
}

// get 发请求，返回响应。401 是 Key 不对；错误里不带地址（Key 在地址里）。
func (c *Client) get(ctx context.Context, q url.Values) ([]byte, error) {
	q.Set("apikey", c.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, errors.New("OMDb 接口地址格式不对")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("读取 OMDb 的响应失败: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		// OMDb 的 Key 不对与次数用完都回 401，正文里写明是哪种
		var raw rawTitle
		if json.Unmarshal(body, &raw) == nil && strings.Contains(strings.ToLower(raw.Error), "limit") {
			return nil, ErrRateLimited
		}
		return nil, ErrUnauthorized
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if len(body) > maxBody {
		return nil, errors.New("OMDb 的响应太大")
	}
	return body, nil
}

// Validate 用一部固定的电影（tt0137523，搏击俱乐部）检查 Key 能不能用。
func (c *Client) Validate(ctx context.Context) error {
	body, err := c.get(ctx, url.Values{"i": {"tt0137523"}, "r": {"json"}})
	if err != nil {
		return err
	}
	var raw rawTitle
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("OMDb 返回的不是预期的内容: %w", err)
	}
	if raw.Response != "True" {
		if strings.Contains(strings.ToLower(raw.Error), "api key") {
			return ErrUnauthorized
		}
		return fmt.Errorf("%w: %s", ErrUnavailable, raw.Error)
	}
	return nil
}
