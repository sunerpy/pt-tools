package subscribe

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 分辨率、来源、编码的可选值（internal/media/meta 解析出的写法），列表里从好到差。档案里没有选时按这个顺序比较
// 分辨率与来源，编码不比。
var (
	Resolutions = []string{"4320p", "2160p", "1440p", "1080p", "720p", "576p", "480p"}
	Sources     = []string{"UHD BluRay", "BluRay", "WEB-DL", "WEBRip", "HDTV", "DVD"}
	Codecs      = []string{"H.265", "AV1", "H.264", "VC-1", "MPEG-2", "XviD"}
)

// 打分的权重：分辨率 > 来源 > 编码 > 加分（Remux、HDR、中字、偏好的制作组）。每一级高一档都胜过下面各级加起来的最大值：
// 加分合计最多 115 < 编码一档 200；编码最多 6 档 1200 + 115 < 来源一档 2000；来源最多 6 档 12000 + 1315 < 分辨率一档 20000。
// 免费与做种数不计分，只在同分时排先后。
const (
	weightResolution = 20000
	weightSource     = 2000
	weightCodec      = 200
	bonusRemux       = 50
	bonusChineseSubs = 30
	bonusHDR         = 20
	bonusGroup       = 15
)

const gib = 1 << 30

// Profile 是解开 JSON 以后的质量档案。
type Profile struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Resolutions []string `json:"resolutions"`
	Sources     []string `json:"sources"`
	Codecs      []string `json:"codecs"`
	Remux       string   `json:"remux"`
	HDR         string   `json:"hdr"`
	ChineseSubs string   `json:"chinese_subs"`
	Free        string   `json:"free"`
	Groups      []string `json:"groups"`
	MinSizeGB   float64  `json:"min_size_gb"`
	MaxSizeGB   float64  `json:"max_size_gb"`
	MinSeeders  int      `json:"min_seeders"`
	ExcludeHR   bool     `json:"exclude_hr"`
}

func decodeList(raw string) []string {
	var out []string
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// orEmpty 把 nil 换成空切片：接口按数组返回，页面不用判 null。只用在返回给接口的视图上，
// 内部的 nil 另有含义（例如订阅的站点为 nil 表示不限）。
func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func encodeList(v []string) string {
	if len(v) == 0 {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// profileOf 解开库里的质量档案。
func profileOf(r models.MediaQualityProfile) Profile {
	return Profile{
		ID: r.ID, Name: r.Name, Resolutions: orEmpty(decodeList(r.Resolutions)), Sources: orEmpty(decodeList(r.Sources)), Codecs: orEmpty(decodeList(r.Codecs)),
		Remux: r.Remux, HDR: r.HDR, ChineseSubs: r.ChineseSubs, Free: r.Free, Groups: orEmpty(decodeList(r.Groups)),
		MinSizeGB: r.MinSizeGB, MaxSizeGB: r.MaxSizeGB, MinSeeders: r.MinSeeders, ExcludeHR: r.ExcludeHR,
	}
}

// Candidate 是一个候选种子：站点上的种子、从标题解析出的信息，以及从哪里来的（rss 或 search）。
type Candidate struct {
	Site string
	Item v2.TorrentItem
	Meta meta.Meta
	From string
	// SiteHR 是站点整站都有 H&R 要求
	SiteHR bool
}

// 候选种子的来源。
const (
	fromRSS    = "rss"
	fromSearch = "search"
)

// Verdict 是一个候选种子按质量档案的结果：Reject 不为空时不要（写明原因），否则 Score 是分数。
type Verdict struct {
	Score  int
	Reject string
}

func rank(list []string, v string) int {
	i := slices.IndexFunc(list, func(s string) bool { return strings.EqualFold(s, v) })
	if i < 0 {
		return 0
	}
	return len(list) - i
}

func hasFold(list []string, v string) bool {
	return v != "" && slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(s, v) })
}

// Score 按档案给候选种子打分。episodes 是种子里有几集（剧集按每集算体积；电影与认不出集数的传 1）。
// RSS 里刚发布的种子做种数还没有意义，最少做种数只对搜索到的种子生效。
func (p Profile) Score(c Candidate, episodes int) Verdict {
	m := c.Meta
	if len(p.Resolutions) > 0 && !hasFold(p.Resolutions, m.Resolution) {
		return Verdict{Reject: fmt.Sprintf("分辨率%s不在档案里", orUnknown(m.Resolution))}
	}
	if len(p.Sources) > 0 && !hasFold(p.Sources, m.Source) {
		return Verdict{Reject: fmt.Sprintf("来源%s不在档案里", orUnknown(m.Source))}
	}
	if len(p.Codecs) > 0 && !hasFold(p.Codecs, m.VideoCodec) {
		return Verdict{Reject: fmt.Sprintf("编码%s不在档案里", orUnknown(m.VideoCodec))}
	}
	if reason := wanted("Remux", p.Remux, m.Remux); reason != "" {
		return Verdict{Reject: reason}
	}
	if reason := wanted("HDR", p.HDR, len(m.HDR) > 0); reason != "" {
		return Verdict{Reject: reason}
	}
	if reason := wanted("中字", p.ChineseSubs, m.ChineseSubs); reason != "" {
		return Verdict{Reject: reason}
	}
	if p.Free == models.MediaPrefRequire && !c.Item.IsFree() {
		return Verdict{Reject: "不是免费"}
	}
	if p.ExcludeHR && (c.Item.HasHR || c.SiteHR) {
		return Verdict{Reject: "有 H&R 要求"}
	}
	if c.From == fromSearch && p.MinSeeders > 0 && c.Item.Seeders < p.MinSeeders {
		return Verdict{Reject: fmt.Sprintf("做种 %d 个，少于 %d 个", c.Item.Seeders, p.MinSeeders)}
	}
	if c.Item.SizeBytes > 0 && (p.MinSizeGB > 0 || p.MaxSizeGB > 0) {
		per := float64(c.Item.SizeBytes) / gib / float64(max(episodes, 1))
		if p.MinSizeGB > 0 && per < p.MinSizeGB {
			return Verdict{Reject: fmt.Sprintf("体积 %.1f GB，小于 %.1f GB", per, p.MinSizeGB)}
		}
		if p.MaxSizeGB > 0 && per > p.MaxSizeGB {
			return Verdict{Reject: fmt.Sprintf("体积 %.1f GB，大于 %.1f GB", per, p.MaxSizeGB)}
		}
	}

	return Verdict{Score: p.points(m)}
}

// points 是不看硬条件的分数：候选过了硬条件以后用它排先后，已有的版本（库里的、下载过的）按当前档案重算时也用它。
func (p Profile) points(m meta.Meta) int {
	resolutions, sources := p.Resolutions, p.Sources
	if len(resolutions) == 0 {
		resolutions = Resolutions
	}
	if len(sources) == 0 {
		sources = Sources
	}
	score := rank(resolutions, m.Resolution)*weightResolution + rank(sources, m.Source)*weightSource
	if len(p.Codecs) > 0 {
		score += rank(p.Codecs, m.VideoCodec) * weightCodec
	}
	if p.Remux == models.MediaPrefPrefer && m.Remux {
		score += bonusRemux
	}
	if p.HDR == models.MediaPrefPrefer && len(m.HDR) > 0 {
		score += bonusHDR
	}
	if p.ChineseSubs == models.MediaPrefPrefer && m.ChineseSubs {
		score += bonusChineseSubs
	}
	if hasFold(p.Groups, m.Group) {
		score += bonusGroup
	}
	return score
}

// wanted 检查 require（必须有）与 avoid（不要）。
func wanted(name, pref string, has bool) string {
	switch {
	case pref == models.MediaPrefRequire && !has:
		return "没有" + name
	case pref == models.MediaPrefAvoid && has:
		return "有" + name
	}
	return ""
}

func orUnknown(v string) string {
	if v == "" {
		return "（认不出来）"
	}
	return " " + v + " "
}

// TargetReached 报告资源是不是已经达到档案里排第一的分辨率与来源（档案要优先 Remux 时还要是 Remux）：
// 洗版到这里就停。档案里没有选分辨率或来源时，那一项不限。
func (p Profile) TargetReached(m meta.Meta) bool {
	if len(p.Resolutions) > 0 && !strings.EqualFold(m.Resolution, p.Resolutions[0]) {
		return false
	}
	if len(p.Sources) > 0 && !strings.EqualFold(m.Source, p.Sources[0]) {
		return false
	}
	return p.Remux != models.MediaPrefPrefer || m.Remux
}

// better 报告候选 a 是不是比 b 好：分数高的先；同分时档案要免费的免费先，再看做种数，最后体积小的先。
func (p Profile) better(a, b Candidate, va, vb Verdict) bool {
	if va.Score != vb.Score {
		return va.Score > vb.Score
	}
	if p.Free == models.MediaPrefPrefer && a.Item.IsFree() != b.Item.IsFree() {
		return a.Item.IsFree()
	}
	if a.Item.Seeders != b.Item.Seeders {
		return a.Item.Seeders > b.Item.Seeders
	}
	return a.Item.SizeBytes < b.Item.SizeBytes
}
