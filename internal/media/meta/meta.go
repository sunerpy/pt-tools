// Package meta 解析 PT 种子的标题与副标题：中英文名、年份、类型、季与集（含范围、「全 N 集」「第 1-10 集」）、
// 分辨率、来源、编码、HDR、音轨、制作组、版本，以及中字、国语、粤语等标记。只看文字，不联网，不会 panic。
package meta

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// MediaType 是媒体类型。
type MediaType string

const (
	TypeUnknown MediaType = ""
	TypeMovie   MediaType = "movie"
	TypeTV      MediaType = "tv"
)

// Meta 是解析结果。没解析出来的字段是零值。
type Meta struct {
	NameCN        string    `json:"name_cn,omitempty"`
	NameEN        string    `json:"name_en,omitempty"`
	Year          int       `json:"year,omitempty"`
	Type          MediaType `json:"type,omitempty"`
	Season        int       `json:"season,omitempty"`
	SeasonEnd     int       `json:"season_end,omitempty"`
	Episode       int       `json:"episode,omitempty"`
	EpisodeEnd    int       `json:"episode_end,omitempty"`
	TotalEpisodes int       `json:"total_episodes,omitempty"`
	// Complete 表示整季（或多季）的合集：只写了季、写了 Complete，或写了「全 N 集」。
	Complete    bool     `json:"complete,omitempty"`
	Resolution  string   `json:"resolution,omitempty"`
	Source      string   `json:"source,omitempty"`
	Remux       bool     `json:"remux,omitempty"`
	Platform    string   `json:"platform,omitempty"`
	VideoCodec  string   `json:"video_codec,omitempty"`
	BitDepth    int      `json:"bit_depth,omitempty"`
	FPS         int      `json:"fps,omitempty"`
	HDR         []string `json:"hdr,omitempty"`
	Audio       []string `json:"audio,omitempty"`
	Channels    string   `json:"channels,omitempty"`
	Group       string   `json:"group,omitempty"`
	Edition     []string `json:"edition,omitempty"`
	Version     string   `json:"version,omitempty"`
	ThreeD      bool     `json:"three_d,omitempty"`
	ChineseSubs bool     `json:"chinese_subs,omitempty"`
	Mandarin    bool     `json:"mandarin,omitempty"`
	Cantonese   bool     `json:"cantonese,omitempty"`
}

// String 是给人看的简短描述：名字加年份，剧集加季与集。
func (m Meta) String() string {
	name := m.NameEN
	if name == "" {
		name = m.NameCN
	}
	switch {
	case m.Type == TypeTV && m.Season > 0 && m.Episode > 0 && m.EpisodeEnd > m.Episode:
		return fmt.Sprintf("%s S%02dE%02d-E%02d", name, m.Season, m.Episode, m.EpisodeEnd)
	case m.Type == TypeTV && m.Season > 0 && m.Episode > 0:
		return fmt.Sprintf("%s S%02dE%02d", name, m.Season, m.Episode)
	case m.Type == TypeTV && m.Season > 0:
		return fmt.Sprintf("%s S%02d", name, m.Season)
	case m.Type == TypeTV && m.Episode > 0:
		return fmt.Sprintf("%s E%02d", name, m.Episode)
	case m.Year > 0:
		return fmt.Sprintf("%s %d", name, m.Year)
	}
	return name
}

func maxYear() int { return time.Now().Year() + 1 }

// Parse 解析标题与副标题。中文名先看标题，标题里没有时看副标题；季与集先看标题，标题里没有时看副标题。
func Parse(title, subtitle string) Meta {
	var m Meta
	// 站点偶尔给出坏掉的字节：先去掉，结果里的字符串都是合法的 UTF-8
	title = strings.TrimSpace(strings.ToValidUTF8(title, ""))
	subtitle = strings.TrimSpace(strings.ToValidUTF8(subtitle, ""))
	if title == "" && subtitle == "" {
		return m
	}
	body := extRe.ReplaceAllString(title, "")
	// 开头的 [..]：含中文是中文名（[流浪地球].The.Wandering.Earth），否则是字幕组（[ANi] 葬送的芙莉莲 - 01）
	lead := ""
	if mm := leadBracketRe.FindStringSubmatch(body); mm != nil {
		content := strings.TrimSpace(mm[1])
		if hasHan(content) {
			m.NameCN = cnName(content)
		} else {
			lead = content
		}
		body = strings.TrimLeft(body[len(mm[0]):], " ._-")
	}

	nameArea, rest := cutNameArea(body)
	tokens := tokenize(nameArea)
	stop := len(tokens)
	for i, tk := range tokens {
		if !hasHan(tk) && isStop(tk) {
			stop = i
			break
		}
	}
	nameEnd := stop
	if j := lastYearIndex(tokens[:stop]); j > 0 {
		m.Year, _ = strconv.Atoi(tokens[j])
		nameEnd = j
	}
	m.NameEN = joinLatin(tokens[:nameEnd])
	if m.NameCN == "" {
		m.NameCN = firstCN(tokens)
	}
	tail := strings.Join(tokens[stop:], " ") + " " + rest
	if m.Year == 0 {
		m.Year = firstYear(tokenize(tail))
	}

	parseEpisodes(&m, body, subtitle)
	parseTech(&m, title, tail)
	parseFlags(&m, title+" "+subtitle)
	m.Group = parseGroup(body, lead)

	if m.NameCN == "" {
		m.NameCN = subtitleCN(subtitle)
	}
	if m.Year == 0 {
		m.Year = firstYear(tokenize(subtitle))
	}
	switch {
	case m.Season > 0 || m.Episode > 0 || m.TotalEpisodes > 0:
		m.Type = TypeTV
	case m.Year > 0:
		m.Type = TypeMovie
	}
	return m
}

var (
	extRe         = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|ts|m2ts|torrent|rmvb|wmv|iso)$`)
	leadBracketRe = regexp.MustCompile(`^[\[【]([^\]】]{1,80})[\]】]`)
	// 动漫的「 - 01 」：前后有空格的短横线加集数
	animeDashRe = regexp.MustCompile(`\s-\s(\d{1,4})(?:v(\d))?(?:\s|\[|\(|$)`)
	yearRe      = regexp.MustCompile(`^(19|20)\d{2}$`)
	splitRe     = regexp.MustCompile(`[\s._()（）,，]+`)
)

// cutNameArea 把名字所在的部分与其后的技术信息分开：名字在第一个方括号或动漫集数之前。
func cutNameArea(body string) (string, string) {
	cut := len(body)
	if i := strings.IndexAny(body, "[【"); i >= 0 && i < cut {
		cut = i
	}
	if loc := animeDashRe.FindStringIndex(body); loc != nil && loc[0] < cut {
		cut = loc[0]
	}
	return body[:cut], body[cut:]
}

func tokenize(s string) []string {
	var out []string
	for _, tk := range splitRe.Split(s, -1) {
		tk = strings.Trim(tk, "-:：/|")
		if tk != "" {
			out = append(out, tk)
		}
	}
	return out
}

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func isYear(tk string) bool {
	if !yearRe.MatchString(tk) {
		return false
	}
	y, _ := strconv.Atoi(tk)
	return y >= 1900 && y <= maxYear()
}

// lastYearIndex 是名字区里最后一个像年份的词的位置：1917.2019、2001.A.Space.Odyssey.1968 这类名字里带数字的，
// 年份是最后一个；返回 0 表示名字只有这一个数字（如 2012），当名字不当年份。
func lastYearIndex(tokens []string) int {
	for j := len(tokens) - 1; j >= 0; j-- {
		if !hasHan(tokens[j]) && isYear(tokens[j]) {
			if latinBefore(tokens[:j]) {
				return j
			}
			return -1
		}
	}
	return -1
}

func latinBefore(tokens []string) bool {
	for _, tk := range tokens {
		if !hasHan(tk) {
			return true
		}
	}
	return false
}

func firstYear(tokens []string) int {
	for _, tk := range tokens {
		if isYear(tk) {
			y, _ := strconv.Atoi(tk)
			return y
		}
	}
	return 0
}

func joinLatin(tokens []string) string {
	var parts []string
	for _, tk := range tokens {
		if !hasHan(tk) {
			parts = append(parts, tk)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

var stopRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\d{3,4}[pi]$`),
	regexp.MustCompile(`(?i)^(4K|8K|UHD|HDR|HDR10|HDR10\+|HDR10Plus|DV|DoVi|HLG|SDR)$`),
	regexp.MustCompile(`(?i)^(Blu-?ray|BluRay|BDRip|BDMV|WEB-?DL|WEBDL|WEB-?Rip|WEB|HDTV|HDTVRip|DVD|DVDRip|HDDVD|REMUX|UHDTV)$`),
	regexp.MustCompile(`(?i)^(x264|x265|H264|H265|HEVC|AVC|AV1|VC-?1|MPEG-?2|XviD|DivX)$`),
	regexp.MustCompile(`(?i)^S\d{1,3}(E\d{1,4})?(-E?\d{1,4})?$`),
	regexp.MustCompile(`(?i)^S\d{1,3}-S\d{1,3}$`),
	regexp.MustCompile(`(?i)^EP?\d{1,4}(-EP?\d{1,4})?$`),
	regexp.MustCompile(`(?i)^(Season|Complete|Extended|Remastered|IMAX|Hybrid|Unrated|Uncut|Criterion|Theatrical|REPACK\d?|PROPER|RERIP|3D|Half-SBS|Half-OU|SBS)$`),
	regexp.MustCompile(`(?i)^\d{2,3}fps$`),
	regexp.MustCompile(`(?i)^(DTS|DTS-HD|TrueHD|Atmos|DDP|DDP\d\.?\d?|DD\d\.?\d?|AAC|AAC\d\.?\d?|FLAC|LPCM|AC3|EAC3|Opus)$`),
}

// isStop 判断一个词是不是名字之后的技术信息（年份另算）。带短横线的词（x264-SPARKS）看每一段。
func isStop(tk string) bool {
	for _, re := range stopRes {
		if re.MatchString(tk) {
			return true
		}
	}
	if strings.Contains(tk, "-") {
		for _, part := range strings.Split(tk, "-") {
			if part == "" {
				continue
			}
			for _, re := range stopRes[:4] {
				if re.MatchString(part) {
					return true
				}
			}
		}
	}
	return false
}
