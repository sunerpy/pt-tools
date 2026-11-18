package tmdb

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
)

// Image 是条目的一张图片。Lang 是图上文字的语言（ISO 639-1），空串是没有文字。
type Image struct {
	Path  string  `json:"path"`
	Lang  string  `json:"lang,omitempty"`
	Vote  float64 `json:"vote,omitempty"`
	Votes int     `json:"votes,omitempty"`
}

// Images 是条目的海报、背景与 Logo，每类都按评分从高到低排好。
// 只取中文、英文、日文与没有文字的，每种语言最多 maxImagesPerLang 张；SVG 一类不能直接用的格式不要。
type Images struct {
	Posters   []Image `json:"posters,omitempty"`
	Backdrops []Image `json:"backdrops,omitempty"`
	Logos     []Image `json:"logos,omitempty"`
}

const (
	// imageLanguages 是取图片列表时带上的语言（null 是没有文字的）
	imageLanguages   = "zh,en,ja,null"
	maxImagesPerLang = 10
)

type rawImage struct {
	FilePath    string  `json:"file_path"`
	Lang        *string `json:"iso_639_1"`
	VoteAverage float64 `json:"vote_average"`
	VoteCount   int     `json:"vote_count"`
}

// Images 取条目的图片列表（刮削按语言挑海报、找 Logo 时用），缓存一周。
func (c *Client) Images(ctx context.Context, kind string, id int) (*Images, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	var out Images
	err := c.cached(ctx, fmt.Sprintf("images:%s:%d", kind, id), DetailsTTL, &out, func() (any, error) {
		var raw struct {
			Posters   []rawImage `json:"posters"`
			Backdrops []rawImage `json:"backdrops"`
			Logos     []rawImage `json:"logos"`
		}
		q := url.Values{"include_image_language": {imageLanguages}}
		if err := c.get(ctx, fmt.Sprintf("/%s/%d/images", kind, id), q, &raw); err != nil {
			return nil, err
		}
		return Images{Posters: trimImages(raw.Posters), Backdrops: trimImages(raw.Backdrops), Logos: trimImages(raw.Logos)}, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// trimImages 按评分（其次评分人数）排序，每种语言留 maxImagesPerLang 张，去掉路径格式不对的。
func trimImages(in []rawImage) []Image {
	sorted := slices.Clone(in)
	slices.SortStableFunc(sorted, func(a, b rawImage) int {
		if c := cmp.Compare(b.VoteAverage, a.VoteAverage); c != 0 {
			return c
		}
		return cmp.Compare(b.VoteCount, a.VoteCount)
	})
	count := map[string]int{}
	var out []Image
	for _, r := range sorted {
		if !imagePathRe.MatchString(r.FilePath) {
			continue
		}
		lang := ""
		if r.Lang != nil {
			lang = *r.Lang
		}
		if count[lang] >= maxImagesPerLang {
			continue
		}
		count[lang]++
		out = append(out, Image{Path: r.FilePath, Lang: lang, Vote: r.VoteAverage, Votes: r.VoteCount})
	}
	return out
}

// Language 是客户端取详情用的语言（例如 zh-CN）。
func (c *Client) Language() string { return c.language }
