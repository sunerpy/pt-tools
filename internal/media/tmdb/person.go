package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// PersonTTL 是演员译名的缓存时间。
const PersonTTL = 30 * 24 * time.Hour

// zhRegions 是演员中文名按这个顺序取：简体（大陆、新加坡），再是繁体（台湾、香港）。
var zhRegions = []string{"CN", "SG", "TW", "HK"}

type personNames struct {
	// Names 是各地区的中文名（键是 iso_3166_1，大写）
	Names map[string]string `json:"names"`
}

// PersonChineseName 是演员在 TMDB 上的中文译名（/person/{id}/translations，缓存 30 天）：按 zh-CN、zh-SG、zh-TW、zh-HK 的顺序取，
// 都没有时回空串。TMDB 上没有这个人时也回空串。
func (c *Client) PersonChineseName(ctx context.Context, id int) (string, error) {
	if id <= 0 {
		return "", nil
	}
	var out personNames
	err := c.cached(ctx, fmt.Sprintf("person-names:%d", id), PersonTTL, &out, func() (any, error) {
		var raw struct {
			Translations []struct {
				Lang   string `json:"iso_639_1"`
				Region string `json:"iso_3166_1"`
				Data   struct {
					Name string `json:"name"`
				} `json:"data"`
			} `json:"translations"`
		}
		names := map[string]string{}
		if err := c.get(ctx, fmt.Sprintf("/person/%d/translations", id), url.Values{}, &raw); err != nil {
			if errors.Is(err, ErrNotFound) {
				return personNames{Names: names}, nil
			}
			return nil, err
		}
		for _, t := range raw.Translations {
			if n := strings.TrimSpace(t.Data.Name); t.Lang == "zh" && n != "" {
				if r := strings.ToUpper(t.Region); names[r] == "" {
					names[r] = n
				}
			}
		}
		return personNames{Names: names}, nil
	})
	if err != nil {
		return "", err
	}
	for _, r := range zhRegions {
		if n := out.Names[r]; n != "" {
			return n, nil
		}
	}
	return "", nil
}
