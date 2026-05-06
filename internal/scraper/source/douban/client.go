package douban

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

var defaultUserAgents = []string{
	"api-client/1 com.douban.frodo/7.22.0.beta9(231) Android/23 product/Mate 40 vendor/HUAWEI model/Mate 40 brand/HUAWEI rom/android network/wifi platform/AndroidPad",
	"api-client/1 com.douban.frodo/7.18.0(230) Android/22 product/MI 9 vendor/Xiaomi model/MI 9 brand/Android rom/miui6 network/wifi platform/mobile nd/1",
	"api-client/1 com.douban.frodo/7.1.0(205) Android/29 product/perseus vendor/Xiaomi model/Mi MIX 3 rom/miui6 network/wifi platform/mobile nd/1",
	"api-client/1 com.douban.frodo/7.3.0(207) Android/22 product/MI 9 vendor/Xiaomi model/MI 9 brand/Android rom/miui6 network/wifi platform/mobile nd/1",
}

const (
	defaultHTTPTimeout = 15 * time.Second
	defaultRateLimit   = 2 * time.Second
	maxAttempts        = 2
)

type Client struct {
	baseURL    string
	htmlURL    string
	httpClient *http.Client
	userAgents []string
	rateLimit  time.Duration

	now      func() time.Time
	randIntn func(int) int
	sleeper  func(time.Duration)
}

type Config struct {
	BaseURL    string
	HTMLURL    string
	HTTPClient *http.Client
	RateLimit  time.Duration
}

func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	htmlURL := strings.TrimRight(cfg.HTMLURL, "/")
	if htmlURL == "" {
		htmlURL = defaultHTMLURL
	}
	rateLimit := cfg.RateLimit
	if rateLimit <= 0 {
		rateLimit = defaultRateLimit
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &Client{
		baseURL:    baseURL,
		htmlURL:    htmlURL,
		httpClient: httpClient,
		userAgents: append([]string(nil), defaultUserAgents...),
		rateLimit:  rateLimit,
		now:        time.Now,
		randIntn:   rng.Intn,
		sleeper:    time.Sleep,
	}
}

func (c *Client) GetMovie(ctx context.Context, id string) (*subjectDetailResponse, error) {
	var resp subjectDetailResponse
	if err := c.getJSON(ctx, http.MethodGet, "/movie/"+id, url.Values{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetTV(ctx context.Context, id string) (*subjectDetailResponse, error) {
	var resp subjectDetailResponse
	if err := c.getJSON(ctx, http.MethodGet, "/tv/"+id, url.Values{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Search(ctx context.Context, query string, count int) (*searchResponse, error) {
	if count <= 0 {
		count = 20
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(count))
	var resp searchResponse
	if err := c.getJSON(ctx, http.MethodGet, "/search/weixin", params, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetMovieCelebrities(ctx context.Context, id string) (*celebritiesResponse, error) {
	var resp celebritiesResponse
	if err := c.getJSON(ctx, http.MethodGet, "/movie/"+id+"/celebrities", url.Values{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetMoviePhotos(ctx context.Context, id string) (*photosResponse, error) {
	var resp photosResponse
	if err := c.getJSON(ctx, http.MethodGet, "/movie/"+id+"/photos", url.Values{}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetHTMLDetail(ctx context.Context, id string) (*htmlDetail, error) {
	if err := c.beforeRequest(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.htmlURL+"/subject/"+id+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("build douban html request: %w", err)
	}
	req.Header.Set("User-Agent", c.randomUserAgent())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, wrapClientError("request douban html", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("douban html subject %s: %w", id, core.ErrNotFound)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("douban html subject %s: unexpected status %d", id, resp.StatusCode)
	}

	return parseHTMLDetail(id, resp.Body)
}

func (c *Client) getJSON(ctx context.Context, method, endpoint string, params url.Values, dest any) error {
	if params == nil {
		params = url.Values{}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.beforeRequest(); err != nil {
			return err
		}

		reqURL, err := c.signedURL(method, endpoint, params)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			return fmt.Errorf("build douban request: %w", err)
		}
		req.Header.Set("User-Agent", c.randomUserAgent())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return wrapClientError("request douban frodo", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxAttempts {
			resp.Body.Close()
			continue
		}

		err = decodeResponse(resp, dest)
		resp.Body.Close()
		if err != nil {
			return err
		}
		return nil
	}

	return core.ErrRateLimited
}

func (c *Client) signedURL(method, endpoint string, params url.Values) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	base.Path = path.Join(base.Path, endpoint)
	if !strings.HasPrefix(base.Path, "/") {
		base.Path = "/" + base.Path
	}

	query := cloneValues(params)
	now := c.now()
	tsUnix := now.Unix()
	ts := strconv.FormatInt(tsUnix, 10)
	query.Set("apikey", apiKey)
	query.Set("_ts", ts)
	query.Set("_sig", signFrodo(method, base.Path, tsUnix))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func (c *Client) beforeRequest() error {
	if c.rateLimit <= 0 || c.sleeper == nil {
		return nil
	}
	delay := c.jitterDelay()
	if delay > 0 {
		c.sleeper(delay)
	}
	return nil
}

func (c *Client) jitterDelay() time.Duration {
	if c.rateLimit <= 0 {
		return 0
	}
	window := time.Second
	min := c.rateLimit - window
	max := c.rateLimit + window
	if min < 0 {
		min = 0
	}
	span := max - min
	if span <= 0 || c.randIntn == nil {
		return c.rateLimit
	}
	return min + time.Duration(c.randIntn(int(span)+1))
}

func (c *Client) randomUserAgent() string {
	if len(c.userAgents) == 0 {
		return "Mozilla/5.0"
	}
	if len(c.userAgents) == 1 || c.randIntn == nil {
		return c.userAgents[0]
	}
	return c.userAgents[c.randIntn(len(c.userAgents))]
}

func decodeResponse(resp *http.Response, dest any) error {
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode douban json: %w: %v", core.ErrParseFailed, err)
		}
		return nil
	case http.StatusNotFound:
		return core.ErrNotFound
	case http.StatusForbidden:
		return core.ErrUnauthorized
	case http.StatusTooManyRequests:
		return core.ErrRateLimited
	default:
		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("douban frodo unexpected status %d", resp.StatusCode)
		}
		return nil
	}
}

func wrapClientError(msg string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s: %w", msg, core.ErrTimeout)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%s: %w", msg, core.ErrTimeout)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func cloneValues(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for key, values := range src {
		copied := append([]string(nil), values...)
		dst[key] = copied
	}
	return dst
}
