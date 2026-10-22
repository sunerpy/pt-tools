// Package fanart 从 Fanart.tv 取电影与剧集的图片：高清 Logo、clearart、横幅、横版海报、碟片、海报、背景与季海报。
//
// 接口是 webservice.fanart.tv/v3.2：电影按 TMDB 或 IMDb 编号（/movies/{id}），剧集按 TVDB 编号（/tv/{id}）。
// 要带 API Key（请求头 api-key），个人 Key（client-key）可选，新上传的图片更快能取到。错误里不带 Key。
// 图片只从 Fanart.tv 自己的图片服务器下载，最多 10 MiB。响应缓存一段时间。
package fanart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

// 默认的接口地址与缓存时间。
const (
	DefaultBaseURL = "https://webservice.fanart.tv/v3.2"
	imagesTTL      = 3 * 24 * time.Hour
	maxBody        = 4 << 20
	maxImage       = 10 << 20
)

var (
	ErrNoKey        = errors.New("没有填写 Fanart.tv API Key")
	ErrUnauthorized = errors.New("Fanart.tv API Key 无效")
	// ErrUnavailable 表示连不上 Fanart.tv，或它回了别的错误
	ErrUnavailable = errors.New("Fanart.tv 暂时不能访问")
	// ErrNotImage 表示图片地址回的不是图片
	ErrNotImage = errors.New("Fanart.tv 图片地址返回的不是图片")
)

// Cache 是响应缓存（和 TMDB 共用 media_cache 表）。
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, ttl time.Duration)
}

// Options 是客户端的选项。
type Options struct {
	APIKey string
	// ClientKey 是个人 Key（可选）
	ClientKey string
	BaseURL   string
	// ImageHosts 是允许下载图片的域名；为空时只认 assets.fanart.tv 与 *.fanart.tv
	ImageHosts []string
	HTTP       *http.Client
	Cache      Cache
}

// Client 是 Fanart.tv 客户端。
type Client struct {
	key, clientKey string
	base           string
	imageHosts     []string
	http           *http.Client
	cache          Cache
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
	hc := o.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{key: key, clientKey: strings.TrimSpace(o.ClientKey), base: base, imageHosts: o.ImageHosts, http: hc, cache: o.Cache}, nil
}

// Image 是一张图片。Lang 是图上文字的语言，空串是没有文字（接口里的 "00"）；Season 是季海报的季号（"all" 是所有季，记成 -1）。
type Image struct {
	URL    string `json:"url"`
	Lang   string `json:"lang,omitempty"`
	Likes  int    `json:"likes,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Season int    `json:"season,omitempty"`
}

// Images 是一部电影或剧集的图片，按种类分组，每组按点赞数从高到低排好。
// 种类：poster、fanart、logo、clearart、banner、landscape、discart、season_poster（只有剧集有）。
type Images map[string][]Image

// flexInt 认数字与数字字符串（接口有时把数字写成字符串）。
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" || s == "all" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

type rawImage struct {
	URL    string  `json:"url"`
	Lang   string  `json:"lang"`
	Likes  flexInt `json:"likes"`
	Width  flexInt `json:"width"`
	Height flexInt `json:"height"`
	Season string  `json:"season"`
}

// 接口里的分组 → 种类，排在前面的组优先（高清的 Logo 在普通的前面）。
var (
	movieGroups = []struct{ field, kind string }{
		{"movieposter", "poster"},
		{"moviebackground", "fanart"},
		{"hdmovielogo", "logo"},
		{"movielogo", "logo"},
		{"hdmovieclearart", "clearart"},
		{"movieart", "clearart"},
		{"moviebanner", "banner"},
		{"moviethumb", "landscape"},
		{"moviedisc", "discart"},
	}
	tvGroups = []struct{ field, kind string }{
		{"tvposter", "poster"},
		{"showbackground", "fanart"},
		{"hdtvlogo", "logo"},
		{"clearlogo", "logo"},
		{"hdclearart", "clearart"},
		{"clearart", "clearart"},
		{"tvbanner", "banner"},
		{"tvthumb", "landscape"},
		{"seasonposter", "season_poster"},
	}
)

// Movie 取电影的图片：id 是 TMDB 编号或 IMDb 编号（tt 开头）。没有时返回空的 Images。
func (c *Client) Movie(ctx context.Context, id string) (Images, error) {
	id = strings.TrimSpace(id)
	if id == "" || strings.ContainsAny(id, "/?#") {
		return nil, fmt.Errorf("编号不对: %q", id)
	}
	return c.images(ctx, "movies", id, movieGroups)
}

// TV 取剧集的图片（按 TVDB 编号）。没有时返回空的 Images。
func (c *Client) TV(ctx context.Context, tvdbID int) (Images, error) {
	if tvdbID <= 0 {
		return nil, fmt.Errorf("TVDB 编号不对: %d", tvdbID)
	}
	return c.images(ctx, "tv", strconv.Itoa(tvdbID), tvGroups)
}

func (c *Client) images(ctx context.Context, kind, id string, groups []struct{ field, kind string }) (Images, error) {
	key := "fanart:v1:" + kind + ":" + id
	var raw map[string]json.RawMessage
	if c.cache != nil && !cachectl.Bypassed(ctx) {
		if b, ok := c.cache.Get(key); ok && json.Unmarshal(b, &raw) == nil {
			return parseImages(raw, groups), nil
		}
	}
	body, status, err := c.get(ctx, c.base+"/"+kind+"/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// 没有这部的图片：也缓存，免得每次都问
		body = []byte(`{}`)
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("解析 Fanart.tv 的响应失败: %w", err)
	}
	if c.cache != nil {
		c.cache.Set(key, body, imagesTTL)
	}
	return parseImages(raw, groups), nil
}

func parseImages(raw map[string]json.RawMessage, groups []struct{ field, kind string }) Images {
	out := Images{}
	for _, g := range groups {
		var list []rawImage
		if b, ok := raw[g.field]; !ok || json.Unmarshal(b, &list) != nil {
			continue
		}
		var imgs []Image
		for _, r := range list {
			if !strings.HasPrefix(r.URL, "https://") && !strings.HasPrefix(r.URL, "http://") {
				continue
			}
			lang := strings.ToLower(strings.TrimSpace(r.Lang))
			if lang == "00" {
				lang = ""
			}
			img := Image{URL: r.URL, Lang: lang, Likes: int(r.Likes), Width: int(r.Width), Height: int(r.Height)}
			if g.kind == "season_poster" {
				switch s := strings.TrimSpace(r.Season); s {
				case "all", "":
					img.Season = -1
				default:
					n, err := strconv.Atoi(s)
					if err != nil {
						continue
					}
					img.Season = n
				}
			}
			imgs = append(imgs, img)
		}
		// 点赞多的在前；同一种里高清的组在前面，所以只在组内排
		sortByLikes(imgs)
		out[g.kind] = append(out[g.kind], imgs...)
	}
	return out
}

func sortByLikes(imgs []Image) {
	slices.SortStableFunc(imgs, func(a, b Image) int { return b.Likes - a.Likes })
}

// get 发请求：200 与 404 返回响应；401、403 是 Key 不对，其它是 ErrUnavailable。错误里不带 Key 与地址。
func (c *Client) get(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, errors.New("Fanart.tv 接口地址格式不对")
	}
	req.Header.Set("api-key", c.key)
	if c.clientKey != "" {
		req.Header.Set("client-key", c.clientKey)
	}
	req.Header.Set("Accept", "application/json")
	// 跳转到别的地址（换了域名、端口，或从 https 换成 http）时不跟：请求头里有 API Key
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
		return nil, 0, fmt.Errorf("读取 Fanart.tv 的响应失败: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNotFound:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, 0, ErrUnauthorized
	default:
		return nil, 0, fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if len(body) > maxBody {
		return nil, 0, errors.New("Fanart.tv 的响应太大")
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
			return errors.New("Fanart.tv 跳转到了别的地址，没有跟过去")
		}
		return nil
	}
	return &hc
}

// allowedImage 报告图片地址是不是 Fanart.tv 自己的图片服务器（或配置里允许的）。
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
	return u.Scheme == "https" && (host == "fanart.tv" || strings.HasSuffix(host, ".fanart.tv"))
}

// Download 下载一张图片：只认 Fanart.tv 自己的图片服务器（接口给的 http:// 地址换成 https://），只接受 image/* 响应，最多 10 MiB。
func (c *Client) Download(ctx context.Context, raw string) ([]byte, error) {
	if len(c.imageHosts) == 0 && strings.HasPrefix(raw, "http://") {
		raw = "https://" + strings.TrimPrefix(raw, "http://")
	}
	if !c.allowedImage(raw) {
		return nil, fmt.Errorf("不是 Fanart.tv 的图片地址: %q", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("Fanart.tv 图片地址格式不对")
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
		return nil, fmt.Errorf("读取 Fanart.tv 图片失败: %w", err)
	}
	if len(body) > maxImage {
		return nil, errors.New("Fanart.tv 图片超过 10 MiB")
	}
	return body, nil
}

// Validate 用一部固定的电影（TMDB 550，搏击俱乐部）检查 Key 能不能用：Key 不对时返回 ErrUnauthorized。
func (c *Client) Validate(ctx context.Context) error {
	_, status, err := c.get(ctx, c.base+"/movies/550")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, status)
	}
	return nil
}
