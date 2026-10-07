package meta

import (
	"regexp"
	"strconv"
	"strings"
)

// tag 匹配一个独立的技术词：前后不能紧挨字母或数字。
func tag(p string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:` + p + `)(?:[^A-Za-z0-9]|$)`)
}

// audioTag 匹配音轨词：后面可以紧跟声道数（DDP5.1、AAC2.0）。
func audioTag(p string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(?:` + p + `)(?:[^A-Za-z]|$)`)
}

type label struct {
	re   *regexp.Regexp
	name string
}

var (
	seasonEpRe    = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])S(\d{1,3})[ .]?E(\d{1,4})(?:[ .]?-[ .]?(?:S\d{1,3})?E?(\d{1,4}))?(?:[^A-Za-z0-9]|$)`)
	seasonRangeRe = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])S(\d{1,3})[ .]?-[ .]?S(\d{1,3})(?:[^A-Za-z0-9]|$)`)
	seasonOnlyRe  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])S(\d{1,3})(?:[^A-Za-z0-9]|$)`)
	seasonWordRe  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])Season[ .]?(\d{1,3})(?:[^A-Za-z0-9]|$)`)
	episodeRe     = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])EP?(\d{1,4})(?:[ .]?-[ .]?EP?(\d{1,4}))?(?:[^A-Za-z0-9]|$)`)
	animeBrRe     = regexp.MustCompile(`\[(\d{1,4})(?:v(\d))?\]`)
	completeRe    = tag(`Complete`)

	cnSeasonRe   = regexp.MustCompile(`第\s*([\d一二三四五六七八九十]{1,4})\s*季`)
	cnEpRangeRe  = regexp.MustCompile(`第\s*(\d{1,4})\s*[-~～至]\s*(\d{1,4})\s*集`)
	cnEpisodeRe  = regexp.MustCompile(`第\s*(\d{1,4})\s*集`)
	cnTotalRe    = regexp.MustCompile(`全\s*(\d{1,4})\s*集`)
	cnTotalAltRe = regexp.MustCompile(`(\d{1,4})\s*集全`)
)

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// episodes 是从一段文字里读出的季与集。
type episodes struct {
	season, seasonEnd, episode, episodeEnd, total int
	complete                                      bool
	versionText                                   string
}

func latinEpisodes(s string) episodes {
	var e episodes
	if mm := seasonEpRe.FindStringSubmatch(s); mm != nil {
		e.season, e.episode = atoi(mm[1]), atoi(mm[2])
		if end := atoi(mm[3]); end > e.episode {
			e.episodeEnd = end
		}
	} else if mm := seasonRangeRe.FindStringSubmatch(s); mm != nil {
		e.season = atoi(mm[1])
		if end := atoi(mm[2]); end > e.season {
			e.seasonEnd = end
		}
		e.complete = true
	} else if mm := seasonOnlyRe.FindStringSubmatch(s); mm != nil {
		e.season = atoi(mm[1])
	} else if mm := seasonWordRe.FindStringSubmatch(s); mm != nil {
		e.season = atoi(mm[1])
	}
	if e.episode == 0 {
		if mm := episodeRe.FindStringSubmatch(s); mm != nil {
			e.episode = atoi(mm[1])
			if end := atoi(mm[2]); end > e.episode {
				e.episodeEnd = end
			}
		}
	}
	if e.episode == 0 {
		if mm := animeDashRe.FindStringSubmatch(s); mm != nil {
			e.episode = atoi(mm[1])
			if mm[2] != "" {
				e.versionText = "v" + mm[2]
			}
		}
	}
	if e.episode == 0 {
		for _, mm := range animeBrRe.FindAllStringSubmatch(s, -1) {
			if n := atoi(mm[1]); n > 0 && !isYear(mm[1]) {
				e.episode = n
				if mm[2] != "" {
					e.versionText = "v" + mm[2]
				}
				break
			}
		}
	}
	if completeRe.MatchString(s) {
		e.complete = true
	}
	return e
}

func chineseEpisodes(s string) episodes {
	var e episodes
	if mm := cnSeasonRe.FindStringSubmatch(s); mm != nil {
		e.season = chineseNumber(mm[1])
	}
	if mm := cnEpRangeRe.FindStringSubmatch(s); mm != nil {
		e.episode = atoi(mm[1])
		if end := atoi(mm[2]); end > e.episode {
			e.episodeEnd = end
		}
	} else if mm := cnEpisodeRe.FindStringSubmatch(s); mm != nil {
		e.episode = atoi(mm[1])
	}
	if mm := cnTotalRe.FindStringSubmatch(s); mm != nil {
		e.total = atoi(mm[1])
	} else if mm := cnTotalAltRe.FindStringSubmatch(s); mm != nil {
		e.total = atoi(mm[1])
	}
	if e.total > 0 {
		e.complete = true
	}
	return e
}

// parseEpisodes 合并标题与副标题里的季与集：标题里的拉丁写法优先，其次标题里的中文写法，最后副标题。
func parseEpisodes(m *Meta, title, subtitle string) {
	sources := []episodes{latinEpisodes(title), chineseEpisodes(title), chineseEpisodes(subtitle), latinEpisodes(subtitle)}
	complete := false
	for _, e := range sources {
		if m.Season == 0 && e.season > 0 {
			m.Season, m.SeasonEnd = e.season, e.seasonEnd
		}
		if m.Episode == 0 && e.episode > 0 {
			m.Episode, m.EpisodeEnd = e.episode, e.episodeEnd
		}
		if m.TotalEpisodes == 0 && e.total > 0 {
			m.TotalEpisodes = e.total
		}
		if m.Version == "" && e.versionText != "" {
			m.Version = e.versionText
		}
		complete = complete || e.complete
	}
	m.Complete = complete || (m.Season > 0 && m.Episode == 0)
	if m.Episode > 0 && m.TotalEpisodes == 0 && m.SeasonEnd == 0 {
		m.Complete = false
	}
}

var (
	resolutionRe = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9])(\d{3,4})[pi](?:[^A-Za-z0-9]|$)`)
	res4K        = tag(`4K|UHD|2160`)
	res8K        = tag(`8K|4320`)
	uhdRe        = tag(`UHD`)
	blurayRe     = tag(`Blu-?ray|BDRip|BDMV|BD25|BD50`)
	webripRe     = tag(`WEB-?Rip`)
	webdlRe      = tag(`WEB-?DL|WEB`)
	hdtvRe       = tag(`HDTV|HDTVRip|UHDTV`)
	dvdRe        = tag(`DVD|DVDRip|DVD5|DVD9`)
	remuxRe      = tag(`REMUX`)
	codecs       = []label{
		{tag(`x265|H\.?265|HEVC`), "H.265"},
		{tag(`x264|H\.?264|AVC`), "H.264"},
		{tag(`AV1`), "AV1"},
		{tag(`VC-?1`), "VC-1"},
		{tag(`MPEG-?2`), "MPEG-2"},
		{tag(`XviD|DivX`), "XviD"},
	}
	bitDepthRe = regexp.MustCompile(`(?i)(?:^|[^0-9])(8|10|12)\s?-?bits?(?:[^A-Za-z]|$)`)
	hi10Re     = tag(`Hi10P`)
	fpsRe      = regexp.MustCompile(`(?i)(?:^|[^0-9])(\d{2,3})\s?fps(?:[^A-Za-z]|$)`)
	hdrVivid   = tag(`HDR[ .]?Vivid`)
	hdr10Plus  = tag(`HDR10\+|HDR10Plus|HDR10P`)
	hdrLabels  = []label{
		{tag(`DV|DoVi|DOVI|Dolby[ .]?Vision`), "DV"},
		{tag(`HDR10`), "HDR10"},
		{tag(`HDR`), "HDR"},
		{tag(`HLG`), "HLG"},
	}
	audioLabels = []label{
		{audioTag(`TrueHD`), "TrueHD"},
		{audioTag(`Atmos`), "Atmos"},
		{audioTag(`DTS-?HD[ .\-]?MA`), "DTS-HD MA"},
		{audioTag(`DTS-?HD[ .\-]?HRA`), "DTS-HD HRA"},
		{audioTag(`DTS[:\-]?X`), "DTS:X"},
		{audioTag(`DDP|DD\+|EAC3|E-AC-3`), "DDP"},
		{audioTag(`DD|AC3|AC-3`), "DD"},
		{audioTag(`AAC`), "AAC"},
		{audioTag(`FLAC`), "FLAC"},
		{audioTag(`LPCM|PCM`), "LPCM"},
		{audioTag(`Opus`), "Opus"},
		{audioTag(`AV3A`), "AV3A"},
		{audioTag(`MP3`), "MP3"},
	}
	dtsRe      = audioTag(`DTS`)
	channelsRe = regexp.MustCompile(`(?:^|[^\d])([1-9]\.[0-2])(?:[^\d]|$)`)
	platforms  = []label{
		{tag(`NF|Netflix`), "Netflix"},
		{tag(`AMZN|Amazon`), "Amazon"},
		{tag(`DSNP|DSNY|Disney\+|DisneyPlus`), "Disney+"},
		{tag(`HMAX`), "HBO Max"},
		{tag(`MAX`), "Max"},
		{tag(`ATVP|AppleTV\+?|ATV\+`), "Apple TV+"},
		{tag(`HULU`), "Hulu"},
		{tag(`PCOK`), "Peacock"},
		{tag(`PMTP`), "Paramount+"},
		{tag(`CR|Crunchyroll`), "Crunchyroll"},
		{tag(`Baha|Bahamut`), "Baha"},
		{tag(`CBC`), "CBC"},
		{tag(`iQIYI|IQ`), "iQIYI"},
		{tag(`YOUKU|YK`), "Youku"},
		{tag(`MGTV`), "MGTV"},
		{tag(`BiliBili|BILI|B-Global`), "Bilibili"},
		{tag(`KKTV`), "KKTV"},
		{tag(`Friday|FriDay`), "friDay"},
		{tag(`LINETV`), "LINE TV"},
		{tag(`Viu`), "Viu"},
		{tag(`WeTV|TX`), "Tencent Video"},
	}
	editions = []label{
		{tag(`Extended(?:[ .](?:Cut|Edition))?`), "Extended"},
		{tag(`Director'?s[ .]Cut`), "Director's Cut"},
		{tag(`IMAX`), "IMAX"},
		{tag(`Remastered|REMASTER`), "Remastered"},
		{tag(`Hybrid`), "Hybrid"},
		{tag(`Criterion`), "Criterion"},
		{tag(`Theatrical(?:[ .]Cut)?`), "Theatrical"},
		{tag(`Uncut`), "Uncut"},
		{tag(`Unrated`), "Unrated"},
		{tag(`Special[ .]Edition`), "Special Edition"},
	}
	versionRe = tag(`REPACK\d?|PROPER|RERIP`)
	threeDRe  = tag(`3D|Half-?SBS|Half-?OU|H-?SBS|H-?OU`)
)

// parseTech 读技术信息。平台、版本说明、3D 只在名字之后的部分里找（Mad.Max 里的 Max 不是平台）。
func parseTech(m *Meta, title, tail string) {
	if mm := resolutionRe.FindStringSubmatch(title); mm != nil {
		switch n := atoi(mm[1]); n {
		case 480, 576, 720, 1080, 1440, 2160, 4320:
			m.Resolution = strconv.Itoa(n) + "p"
		}
	}
	if m.Resolution == "" {
		switch {
		case res8K.MatchString(title):
			m.Resolution = "4320p"
		case res4K.MatchString(title):
			m.Resolution = "2160p"
		}
	}
	switch {
	case uhdRe.MatchString(title) && blurayRe.MatchString(title):
		m.Source = "UHD BluRay"
	case blurayRe.MatchString(title):
		m.Source = "BluRay"
	case webripRe.MatchString(title):
		m.Source = "WEBRip"
	case webdlRe.MatchString(title):
		m.Source = "WEB-DL"
	case hdtvRe.MatchString(title):
		m.Source = "HDTV"
	case dvdRe.MatchString(title):
		m.Source = "DVD"
	}
	m.Remux = remuxRe.MatchString(title)
	for _, c := range codecs {
		if c.re.MatchString(title) {
			m.VideoCodec = c.name
			break
		}
	}
	if mm := bitDepthRe.FindStringSubmatch(title); mm != nil {
		m.BitDepth = atoi(mm[1])
	} else if hi10Re.MatchString(title) {
		m.BitDepth = 10
	}
	if mm := fpsRe.FindStringSubmatch(title); mm != nil {
		m.FPS = atoi(mm[1])
	}
	// HDR Vivid 与 HDR10+ 先认出来再拿掉，免得同一处再算成 HDR、HDR10
	rest := title
	var hdr []string
	for _, l := range hdrLabels[:1] {
		if l.re.MatchString(rest) {
			hdr = append(hdr, l.name)
		}
	}
	if hdr10Plus.MatchString(rest) {
		hdr = append(hdr, "HDR10+")
		rest = hdr10Plus.ReplaceAllString(rest, " ")
	}
	vivid := hdrVivid.MatchString(rest)
	rest = hdrVivid.ReplaceAllString(rest, " ")
	for _, l := range hdrLabels[1:] {
		if l.re.MatchString(rest) {
			hdr = append(hdr, l.name)
		}
	}
	if vivid {
		hdr = append(hdr, "HDR Vivid")
	}
	m.HDR = hdr

	var audio []string
	hasDTS := false
	for _, l := range audioLabels {
		if l.re.MatchString(title) {
			audio = append(audio, l.name)
			if strings.HasPrefix(l.name, "DTS") {
				hasDTS = true
			}
		}
	}
	if !hasDTS && dtsRe.MatchString(title) {
		audio = append(audio, "DTS")
	}
	m.Audio = audio
	if mm := channelsRe.FindStringSubmatch(title); mm != nil {
		m.Channels = mm[1]
	}

	for _, p := range platforms {
		if p.re.MatchString(tail) {
			m.Platform = p.name
			break
		}
	}
	var ed []string
	for _, e := range editions {
		if e.re.MatchString(tail) {
			ed = append(ed, e.name)
		}
	}
	m.Edition = ed
	if m.Version == "" {
		if mm := versionRe.FindString(tail); mm != "" {
			m.Version = strings.ToUpper(strings.Trim(mm, " .-_[]()"))
		}
	}
	m.ThreeD = threeDRe.MatchString(tail)
}

var (
	subsRe      = regexp.MustCompile(`(?i)中字|中英|简繁|繁简|简中|繁中|中文字幕|简体|繁体|双语字幕|简英|繁英|\[GB\]|\[BIG5\]|(?:^|[^A-Za-z0-9])(?:CHS|CHT)(?:[^A-Za-z0-9]|$)`)
	mandarinRe  = regexp.MustCompile(`国语|国配|普通话|国[日英粤韩泰]双语`)
	cantoneseRe = regexp.MustCompile(`粤语|粤配|国粤双语|粤[国英]双语`)
)

// parseFlags 读中字、国语、粤语标记（标题与副标题都看）。
func parseFlags(m *Meta, s string) {
	m.ChineseSubs = subsRe.MatchString(s)
	m.Mandarin = mandarinRe.MatchString(s)
	m.Cantonese = cantoneseRe.MatchString(s)
}

var (
	bracketRe = regexp.MustCompile(`\[[^\]]*\]|【[^】]*】`)
	// 带短横线的技术词先合成一个词，免得把 Blu-ray 的 ray、WEB-DL 的 DL 当成制作组
	compoundRes = []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?i)WEB-(DL|Rip)`), "WEB$1"},
		{regexp.MustCompile(`(?i)Blu-ray`), "Bluray"},
		{regexp.MustCompile(`(?i)DTS-HD([ .\-]?(?:MA|HRA)|[\s.]|$)`), "DTSHD$1"},
		{regexp.MustCompile(`(?i)DTS-X`), "DTSX"},
		{regexp.MustCompile(`(?i)E-AC-3`), "EAC3"},
		{regexp.MustCompile(`(?i)VC-1`), "VC1"},
		{regexp.MustCompile(`(?i)MPEG-2`), "MPEG2"},
		{regexp.MustCompile(`(?i)Half-(SBS|OU)`), "Half$1"},
		{regexp.MustCompile(`(?i)H-(SBS|OU)`), "H$1"},
	}
	groupRe = regexp.MustCompile(`^[A-Za-z0-9@&][A-Za-z0-9@&_]{0,31}$`)
)

// parseGroup 取制作组：标题末尾「-组名」（user@组名 取 @ 之后）；没有时用开头方括号里的字幕组。
func parseGroup(body, lead string) string {
	s := bracketRe.ReplaceAllString(body, " ")
	for _, c := range compoundRes {
		s = c.re.ReplaceAllString(s, c.repl)
	}
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "-"); i >= 0 {
		g := strings.TrimSpace(s[i+1:])
		if groupRe.MatchString(g) && !isStop(g) && !isAllDigits(g) {
			if at := strings.LastIndex(g, "@"); at >= 0 {
				g = g[at+1:]
			}
			if g != "" {
				return g
			}
		}
	}
	return lead
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
