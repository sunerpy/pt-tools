// Package epgroup 按 TMDB 的剧集组换算季与集。种子标题与文件名有时按另一种排法写集号（动画的绝对集号、另一种分季、DVD 的分季），
// 追剧选了这部剧的一个剧集组时，订阅对种子与整理入库都先把写的（第几组，组里第几集）换成 TMDB 默认排法的（季，集），
// 再照默认的季集处理。两边用这里同一套规则，换算的结果一致。
//
// 规则：
//   - 组按剧集组里的顺序数，从 1 起；只有特别篇（都是第 0 季的集）的组不算。组里的集也按顺序数，从 1 起。
//   - 只写了集、没写季的（例如绝对集号「- 25」）按第 1 组算；文件名里没写季时按种子标题写的那一组（见 CoordOf）。
//   - 写的组或集剧集组里没有时不换算：多半是按 TMDB 默认的季集写的，调用方照默认的季集处理并写明（Convert 的 ok 为假）。
package epgroup

import (
	"fmt"
	"slices"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// Ref 是 TMDB 默认排法里的一集。
type Ref struct {
	Season  int
	Episode int
}

// Coord 是按剧集组写的季与集：SeasonEnd 大于 Season 时是几组的合集（不看集），Episode 为 0 时是一整组。
type Coord struct {
	Season, SeasonEnd   int
	Episode, EpisodeEnd int
}

// String 是给人看的写法：「第 2 组第 13 集」「第 2 组第 13–14 集」「第 2 组」「第 1–3 组」。
func (c Coord) String() string {
	switch {
	case c.SeasonEnd > c.Season:
		return fmt.Sprintf("第 %d–%d 组", c.Season, c.SeasonEnd)
	case c.Episode <= 0:
		return fmt.Sprintf("第 %d 组", c.Season)
	case c.EpisodeEnd > c.Episode:
		return fmt.Sprintf("第 %d 组第 %d–%d 集", c.Season, c.Episode, c.EpisodeEnd)
	}
	return fmt.Sprintf("第 %d 组第 %d 集", c.Season, c.Episode)
}

// CoordOf 是 md 里写的季集。只写了集的（季是 0）按 season 那一组算：文件名里没写季时传种子标题写的组（见 TitleSeason），
// 种子标题本身传 0（按第 1 组）。季与集都没写时第二个返回值为假（没得换算）。
func CoordOf(md meta.Meta, season int) (Coord, bool) {
	c := Coord{Season: md.Season, SeasonEnd: md.SeasonEnd, Episode: md.Episode, EpisodeEnd: md.EpisodeEnd}
	if c.Season <= 0 {
		if c.Episode <= 0 {
			return Coord{}, false
		}
		c.Season = max(season, 1)
	}
	if c.EpisodeEnd < c.Episode {
		c.EpisodeEnd = c.Episode
	}
	return c, true
}

// TitleSeason 是种子标题写的组（给文件名里没写季的文件用）：写了季时是这一季，只写了集时是 1，都没写时是 0。
func TitleSeason(md meta.Meta) int {
	switch {
	case md.Season > 0:
		return md.Season
	case md.Episode > 0:
		return 1
	}
	return 0
}

// Map 是一个剧集组的换算表。
type Map struct {
	ID   string
	Name string
	Type int
	// parts[i] 是第 i+1 组里按顺序的集（只有特别篇的组不在里面）
	parts [][]Ref
}

// New 按剧集组的详情建换算表（tmdb.Client.EpisodeGroup 已把组与组里的集按顺序排好）。g 为 nil 时返回 nil。
func New(g *tmdb.EpisodeGroup) *Map {
	if g == nil {
		return nil
	}
	m := &Map{ID: g.ID, Name: g.Name, Type: g.Type}
	for _, part := range g.Groups {
		refs := make([]Ref, 0, len(part.Episodes))
		specials := true
		for _, e := range part.Episodes {
			if e.Season < 0 || e.Episode <= 0 {
				continue
			}
			refs = append(refs, Ref{Season: e.Season, Episode: e.Episode})
			if e.Season != 0 {
				specials = false
			}
		}
		if specials && len(refs) > 0 {
			continue
		}
		m.parts = append(m.parts, refs)
	}
	return m
}

// Parts 是有几组（只有特别篇的组不算）。
func (m *Map) Parts() int {
	if m == nil {
		return 0
	}
	return len(m.parts)
}

// Label 是写进说明的名字：剧集组「名字」。
func (m *Map) Label() string {
	if m == nil || m.Name == "" {
		return "剧集组"
	}
	return "剧集组「" + m.Name + "」"
}

func (m *Map) part(n int) []Ref {
	if n < 1 || n > len(m.parts) {
		return nil
	}
	return m.parts[n-1]
}

// Convert 把按剧集组写的季集换成 TMDB 默认排法的季集，按剧集组里的顺序、去掉重复。写的组或集剧集组里都有时 ok 为真；
// 有一个没有（组号、集号超出，或那一组是空的）时 ok 为假：这个写法不是按这个剧集组排的，调用方照 TMDB 默认的季集处理。
func (m *Map) Convert(c Coord) (refs []Ref, ok bool) {
	if m == nil || c.Season <= 0 {
		return nil, false
	}
	seen := map[Ref]bool{}
	add := func(r Ref) {
		if !seen[r] {
			seen[r] = true
			refs = append(refs, r)
		}
	}
	if c.SeasonEnd > c.Season || c.Episode <= 0 {
		for n := c.Season; n <= max(c.SeasonEnd, c.Season); n++ {
			part := m.part(n)
			if len(part) == 0 {
				return nil, false
			}
			for _, r := range part {
				add(r)
			}
		}
		return refs, true
	}
	part := m.part(c.Season)
	end := max(c.EpisodeEnd, c.Episode)
	if end > len(part) {
		return nil, false
	}
	for p := c.Episode; p <= end; p++ {
		add(part[p-1])
	}
	return refs, true
}

// Season 是 refs 里第 season 季的集号（升序）。
func Season(refs []Ref, season int) []int {
	var out []int
	for _, r := range refs {
		if r.Season == season {
			out = append(out, r.Episode)
		}
	}
	slices.Sort(out)
	return out
}

// Contiguous 报告升序的集号是不是连着的。
func Contiguous(eps []int) bool {
	for i := 1; i < len(eps); i++ {
		if eps[i] != eps[i-1]+1 {
			return false
		}
	}
	return len(eps) > 0
}
