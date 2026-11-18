package subscribe

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

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
	// SeenAt 是取得这条数据的时间（RSS 是取详情的时间，搜索是发起搜索的时间）
	SeenAt time.Time
	// Chosen 是用户在一键追剧里自己选的种子：推送前不让 AI 复核
	Chosen bool
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
	if why := p.rejects(c, episodes, true); len(why) > 0 {
		return Verdict{Reject: why[0]}
	}
	return Verdict{Score: p.points(c.Meta)}
}

// rejects 是候选种子不满足档案硬条件的原因，按分辨率、来源、编码、Remux、HDR、中字、免费、H&R、做种、体积的顺序；
// first 为真时找到第一条就停（推送时只要一条），按条目搜索要全部列出来。
func (p Profile) rejects(c Candidate, episodes int, first bool) []string {
	var out []string
	add := func(reason string) bool {
		if reason == "" {
			return false
		}
		out = append(out, reason)
		return first
	}
	m := c.Meta
	if len(p.Resolutions) > 0 && !hasFold(p.Resolutions, m.Resolution) && add(fmt.Sprintf("分辨率%s不在档案里", orUnknown(m.Resolution))) {
		return out
	}
	if len(p.Sources) > 0 && !hasFold(p.Sources, m.Source) && add(fmt.Sprintf("来源%s不在档案里", orUnknown(m.Source))) {
		return out
	}
	if len(p.Codecs) > 0 && !hasFold(p.Codecs, m.VideoCodec) && add(fmt.Sprintf("编码%s不在档案里", orUnknown(m.VideoCodec))) {
		return out
	}
	if add(wanted("Remux", p.Remux, m.Remux)) || add(wanted("HDR", p.HDR, len(m.HDR) > 0)) || add(wanted("中字", p.ChineseSubs, m.ChineseSubs)) {
		return out
	}
	if p.Free == models.MediaPrefRequire && !c.Item.IsFree() && add("不是免费") {
		return out
	}
	if p.ExcludeHR && (c.Item.HasHR || c.SiteHR) && add("有 H&R 要求") {
		return out
	}
	if c.From == fromSearch && p.MinSeeders > 0 && c.Item.Seeders < p.MinSeeders &&
		add(fmt.Sprintf("做种 %d 个，少于 %d 个", c.Item.Seeders, p.MinSeeders)) {
		return out
	}
	if c.Item.SizeBytes > 0 && (p.MinSizeGB > 0 || p.MaxSizeGB > 0) {
		per := float64(c.Item.SizeBytes) / gib / float64(max(episodes, 1))
		if p.MinSizeGB > 0 && per < p.MinSizeGB {
			add(fmt.Sprintf("体积 %.1f GB，小于 %.1f GB", per, p.MinSizeGB))
		}
		if p.MaxSizeGB > 0 && per > p.MaxSizeGB {
			add(fmt.Sprintf("体积 %.1f GB，大于 %.1f GB", per, p.MaxSizeGB))
		}
	}
	return out
}

// percent 把分数换成给人看的 0–100：除以这个档案能拿到的最高分（排第一的分辨率、来源与编码，加上全部加分）。
// 分数本身按档位加权（几万到十几万），排序仍用它。
func (p Profile) percent(score int) int {
	resolutions, sources := p.Resolutions, p.Sources
	if len(resolutions) == 0 {
		resolutions = Resolutions
	}
	if len(sources) == 0 {
		sources = Sources
	}
	top := len(resolutions)*weightResolution + len(sources)*weightSource + len(p.Codecs)*weightCodec
	for _, b := range []struct {
		on    bool
		bonus int
	}{
		{p.Remux == models.MediaPrefPrefer, bonusRemux},
		{p.HDR == models.MediaPrefPrefer, bonusHDR},
		{p.ChineseSubs == models.MediaPrefPrefer, bonusChineseSubs},
		{len(p.Groups) > 0, bonusGroup},
	} {
		if b.on {
			top += b.bonus
		}
	}
	return min(max(score*100/top, 0), 100)
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
