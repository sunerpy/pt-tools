package tmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// Episode 是剧集一季里的一集。
type Episode struct {
	ID        int    `json:"id,omitempty"`
	Number    int    `json:"episode_number"`
	Name      string `json:"name,omitempty"`
	Overview  string `json:"overview,omitempty"`
	AirDate   string `json:"air_date,omitempty"`
	StillPath string `json:"still_path,omitempty"`
	Runtime   int    `json:"runtime,omitempty"`
}

// Season 是剧集的一季（含各集）。
type Season struct {
	Number     int       `json:"season_number"`
	Name       string    `json:"name,omitempty"`
	Overview   string    `json:"overview,omitempty"`
	AirDate    string    `json:"air_date,omitempty"`
	PosterPath string    `json:"poster_path,omitempty"`
	Episodes   []Episode `json:"episodes,omitempty"`
}

// Episode 返回第 n 集；没有时返回 nil。
func (s *Season) Episode(n int) *Episode {
	if s == nil {
		return nil
	}
	for i := range s.Episodes {
		if s.Episodes[i].Number == n {
			return &s.Episodes[i]
		}
	}
	return nil
}

// Season 取剧集一季的详情：各集的标题、简介、播出日期与截图。
func (c *Client) Season(ctx context.Context, tvID, number int) (*Season, error) {
	if tvID <= 0 || number < 0 {
		return nil, ErrNotFound
	}
	var out Season
	err := c.cached(ctx, fmt.Sprintf("season:%d:%d:%s", tvID, number, c.language), DetailsTTL, &out, func() (any, error) {
		var raw Season
		if err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", tvID, number), url.Values{"language": {c.language}}, &raw); err != nil {
			return nil, err
		}
		return raw, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// 图片的尺寸（TMDB 图片地址里的一段）。
const (
	SizePoster   = "w780"
	SizeBackdrop = "w1280"
	SizeStill    = "w300"
)

// maxImage 是一张图片最多读多少字节。
const maxImage = 10 << 20

var (
	imagePathRe = regexp.MustCompile(`^/[A-Za-z0-9_\-]+\.(?:jpg|jpeg|png|webp)$`)
	imageSizeRe = regexp.MustCompile(`^(?:w\d{2,4}|h\d{2,4}|original)$`)
	// ErrNotImage 表示图片地址回的不是图片。
	ErrNotImage = errors.New("TMDB 图片地址返回的不是图片")
)

// Image 下载一张 TMDB 图片（path 是条目里的 poster_path 一类，size 如 w780）。
// 只接受 image/* 响应，最多 10 MiB；图片走图片服务器，不占接口的限速额度。
func (c *Client) Image(ctx context.Context, path, size string) ([]byte, error) {
	if !imagePathRe.MatchString(path) || !imageSizeRe.MatchString(size) {
		return nil, fmt.Errorf("TMDB 图片地址格式不对: %q", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.imageBase+size+path, nil)
	if err != nil {
		return nil, errors.New("TMDB 图片地址格式不对")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, fmt.Errorf("%w: 图片 HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); !strings.HasPrefix(mt, "image/") {
		return nil, ErrNotImage
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil {
		return nil, fmt.Errorf("读取 TMDB 图片失败: %w", err)
	}
	if len(body) > maxImage {
		return nil, errors.New("TMDB 图片超过 10 MiB")
	}
	return body, nil
}

// Titles 是条目的英文名与别名（罗马音、各地译名）：种子名常用这些名字，中文搜索结果里只有中文名与原名。
// 用 en-US 取详情并带上 alternative_titles，一次请求；去掉重复与空的。
func (c *Client) Titles(ctx context.Context, kind string, id int) ([]string, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	var out []string
	err := c.cached(ctx, fmt.Sprintf("titles:%s:%d", kind, id), DetailsTTL, &out, func() (any, error) {
		var raw struct {
			Title             string `json:"title"`
			Name              string `json:"name"`
			AlternativeTitles struct {
				Titles []struct {
					Title string `json:"title"`
				} `json:"titles"`
				Results []struct {
					Title string `json:"title"`
				} `json:"results"`
			} `json:"alternative_titles"`
		}
		q := url.Values{"language": {"en-US"}, "append_to_response": {"alternative_titles"}}
		if err := c.get(ctx, fmt.Sprintf("/%s/%d", kind, id), q, &raw); err != nil {
			return nil, err
		}
		titles := []string{}
		add := func(t string) {
			if t = strings.TrimSpace(t); t != "" && !slices.Contains(titles, t) {
				titles = append(titles, t)
			}
		}
		add(raw.Title)
		add(raw.Name)
		for _, t := range raw.AlternativeTitles.Titles {
			add(t.Title)
		}
		for _, t := range raw.AlternativeTitles.Results {
			add(t.Title)
		}
		return titles, nil
	})
	return out, err
}
