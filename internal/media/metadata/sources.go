package metadata

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Sources 是媒体库的数据来源：每一项选一个优先的来源，那个来源找不到或没有这一项时用 TMDB 的。空串是 TMDB。
//   - Plot：电影、剧集与每一季的简介（豆瓣与 Bangumi 一季是一个条目，剧集的简介用第 1 季的）；
//   - Rating：主要的评分（NFO 里写在最后，Jellyfin 与 Emby 显示它）；取到的其它来源的评分也写进 NFO；
//   - Episode：每一集的标题与简介（只有剧集有）；
//   - Artwork：海报、背景、Logo 与季海报先用哪个来源的；Fanart.tv 才有的几种图（clearart、横幅、横版海报、碟片）
//     总是用 Fanart.tv 的。
//
// Bangumi 只用于动画（TMDB 类型里有「动画」的条目），别的条目用 TMDB 的。
type Sources struct {
	Plot    string `json:"plot"`
	Rating  string `json:"rating"`
	Episode string `json:"episode"`
	Artwork string `json:"artwork"`
}

// 每一项可选的来源（空串是 TMDB）。
var (
	PlotSources    = []string{"", SourceDouban, SourceBangumi}
	RatingSources  = []string{"", SourceDouban, SourceBangumi, SourceOMDb}
	EpisodeSources = []string{"", SourceBangumi}
	ArtworkSources = []string{"", SourceFanart}
)

// LegacyDouban 是旧版本的「简介与评分」选了豆瓣（MediaLibrary.ScrapeSource 为 douban）：等于简介与评分都用豆瓣。
const LegacyDouban = "douban"

// ParseSources 读媒体库存的数据来源（JSON）。为空时按旧版本的「简介与评分」（legacy）：douban 是简介与评分都用豆瓣。
// 读不出来或有不认识的来源时当作全用 TMDB（不会因为设置坏了写错，界面上重新保存即可）。
func ParseSources(raw, legacy string) Sources {
	var s Sources
	if strings.TrimSpace(raw) == "" {
		if legacy == LegacyDouban {
			s.Plot, s.Rating = SourceDouban, SourceDouban
		}
		return s
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Sources{}
	}
	if s = s.Clean(); func() error { _, err := s.Normalize(); return err }() != nil {
		return Sources{}
	}
	return s
}

// Clean 去掉每一项两边的空白，tmdb 写成空串。
func (s Sources) Clean() Sources {
	for _, p := range []*string{&s.Plot, &s.Rating, &s.Episode, &s.Artwork} {
		if *p = strings.ToLower(strings.TrimSpace(*p)); *p == SourceTMDB {
			*p = ""
		}
	}
	return s
}

// Normalize 检查每一项都是可选的来源（先 Clean），返回存进数据库的 JSON（全是 TMDB 时为空串）。
func (s Sources) Normalize() (string, error) {
	s = s.Clean()
	check := func(what, v string, allowed []string) error {
		if !slices.Contains(allowed, v) {
			names := make([]string, 0, len(allowed))
			for _, a := range allowed {
				if a == "" {
					a = SourceTMDB
				}
				names = append(names, a)
			}
			return fmt.Errorf("%s的来源要选 %s 之一", what, strings.Join(names, "、"))
		}
		return nil
	}
	for _, c := range []struct {
		what, v string
		allowed []string
	}{
		{"简介", s.Plot, PlotSources},
		{"评分", s.Rating, RatingSources},
		{"每一集", s.Episode, EpisodeSources},
		{"图片", s.Artwork, ArtworkSources},
	} {
		if err := check(c.what, c.v, c.allowed); err != nil {
			return "", err
		}
	}
	if s == (Sources{}) {
		return "", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ForKind 去掉这种媒体库用不上的项：电影没有「每一集」。
func (s Sources) ForKind(kind string) Sources {
	if kind != KindTV {
		s.Episode = ""
	}
	return s
}

// uses 报告设置里用到了这个来源（简介、评分、每一集、图片任一项选了它）。
func (s Sources) uses(source string) bool {
	return s.Plot == source || s.Rating == source || s.Episode == source || s.Artwork == source
}

// or 是这一项的来源：空串是 TMDB。
func or(v string) string {
	if v == "" {
		return SourceTMDB
	}
	return v
}
