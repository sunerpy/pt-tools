package subscribe

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

// 最近一次搜索找到的版本：锁住的版本等不到时，用户在「修改」里看哪些版本能补上缺的集，选一个改锁
// （2026-10-24 用户：「清理了如何重新设置条件」）。只记过了质量档案、能补上缺的集的候选；RSS 推来的单条不覆盖搜索的结果。

// SeenVersion 是一个版本（制作组 + 分辨率）在这次搜索里能补上的缺集与站点。
type SeenVersion struct {
	Group      string   `json:"group"`
	Resolution string   `json:"resolution"`
	Episodes   []int    `json:"episodes"`
	Sites      []string `json:"sites"`
}

// SeenVersions 是最近一次搜索找到的版本，能补的集多的在前。
type SeenVersions struct {
	At       string        `json:"at"`
	Versions []SeenVersion `json:"versions"`
}

// maxSeenVersions 是最多记几个版本。
const maxSeenVersions = 20

// seenVersionsOf 把候选按版本归在一起：每个版本能补上哪几个缺的集、在哪些站点上。认不出组和分辨率的候选不记。
func seenVersionsOf(cands []scored, missing map[int]bool) []SeenVersion {
	type acc struct {
		v     SeenVersion
		eps   map[int]bool
		sites map[string]bool
	}
	byKey := map[string]*acc{}
	for _, o := range cands {
		g, r, ok := versionOf(o.c.Meta)
		if !ok {
			continue
		}
		var covered []int
		for n := range missing {
			if o.sp.covers(n) {
				covered = append(covered, n)
			}
		}
		if len(covered) == 0 {
			continue
		}
		key := strings.ToLower(g) + "\x00" + strings.ToLower(r)
		a := byKey[key]
		if a == nil {
			a = &acc{v: SeenVersion{Group: g, Resolution: r}, eps: map[int]bool{}, sites: map[string]bool{}}
			byKey[key] = a
		}
		for _, n := range covered {
			a.eps[n] = true
		}
		if o.c.Site != "" {
			a.sites[o.c.Site] = true
		}
	}
	out := make([]SeenVersion, 0, len(byKey))
	for _, a := range byKey {
		v := a.v
		for n := range a.eps {
			v.Episodes = append(v.Episodes, n)
		}
		for site := range a.sites {
			v.Sites = append(v.Sites, site)
		}
		slices.Sort(v.Episodes)
		slices.Sort(v.Sites)
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b SeenVersion) int {
		if d := len(b.Episodes) - len(a.Episodes); d != 0 {
			return d
		}
		if c := strings.Compare(strings.ToLower(a.Group), strings.ToLower(b.Group)); c != 0 {
			return c
		}
		return strings.Compare(a.Resolution, b.Resolution)
	})
	if len(out) > maxSeenVersions {
		out = out[:maxSeenVersions]
	}
	return out
}

// recordSeenVersions 记下这次搜索找到的版本（覆盖上一次的）；候选都是 RSS 推来的时不记。
func (s *Service) recordSeenVersions(ctx context.Context, sub *models.MediaSubscription, cands []scored, missing map[int]bool) {
	if !slices.ContainsFunc(cands, func(o scored) bool { return o.c.From == fromSearch }) {
		return
	}
	raw, err := json.Marshal(SeenVersions{At: s.cfg.Now().UTC().Format(time.RFC3339), Versions: seenVersionsOf(cands, missing)})
	if err != nil {
		return
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		UpdateColumn("seen_versions", string(raw)).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下找到的版本失败: %v", err)
		return
	}
	sub.SeenVersions = string(raw)
}

// seenVersionsView 读出订阅里记下的版本；没有或读不出来时为 nil。
func seenVersionsView(row *models.MediaSubscription) *SeenVersions {
	if row.SeenVersions == "" {
		return nil
	}
	var v SeenVersions
	if json.Unmarshal([]byte(row.SeenVersions), &v) != nil {
		return nil
	}
	if v.Versions == nil {
		v.Versions = []SeenVersion{}
	}
	return &v
}
