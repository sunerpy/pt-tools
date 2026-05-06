package service

import (
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ParsedName 从文件名解析的结构化信息。
type ParsedName struct {
	Title     string // 清洗后的标题
	Year      int    // 1900-现在+5，0=未识别
	Season    int    // 0=未识别 (非剧集)
	Episode   int    // 0=未识别
	Quality   string // "1080p" / "2160p" / "4K" / "720p" 等
	Source    string // "BluRay" / "WEB-DL" / "WEBRip" / "HDTV" / "DVDRip"
	Codec     string // "x265" / "x264" / "HEVC" / "AVC" / "AV1"
	Group     string // 发布组（破折号后缀）
	IsShow    bool   // 是否为剧集（Season/Episode 之一非零）
	Extension string // ".mkv" 不含点
}

type tokenPattern struct {
	value string
	re    *regexp.Regexp
}

var (
	reSxxExx         = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,2})e(\d{1,3})(?:[^a-z0-9]|$)`)
	reNxN            = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{1,3})(?:[^a-z0-9]|$)`)
	reSeasonEpisode  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])season\s*(\d{1,2})\s*episode\s*(\d{1,3})(?:[^a-z0-9]|$)`)
	reEpisodeOnly    = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])e(?:p)?(\d{1,3})(?:[^a-z0-9]|$)`)
	reAnimeEpisode   = regexp.MustCompile(`(?i)\s-\s(\d{1,3})(?:\s|$|\[|\()`)
	reYear           = regexp.MustCompile(`(?i)(?:^|[^0-9])((?:19|20)\d{2})(?:[^0-9]|$)`)
	reGroup          = regexp.MustCompile(`(?i)^(?:\[[a-z0-9][a-z0-9.\-]*\]|[a-z0-9][a-z0-9.\-]{1,31})$`)
	reLeadingBracket = regexp.MustCompile(`(?i)^\[[^\]]+\]\s*`)
	reEmptyBrackets  = regexp.MustCompile(`(?i)[\[\(\{]\s*[\]\)\}]`)
	reDashSep        = regexp.MustCompile(`\s+-\s*|\s*-\s+`)
	reTrimPunct      = regexp.MustCompile(`(?i)^[\s\-._:]+|[\s\-._:]+$`)
	reSep            = regexp.MustCompile(`(?i)[._]+`)
	reJunk           = regexp.MustCompile(`(?i)[\[\]\(\){}]+`)
	reMultiSpace     = regexp.MustCompile(`\s+`)

	qualityPatterns = []tokenPattern{
		{value: "2160p", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])2160p(?:[^a-z0-9]|$)`)},
		{value: "4K", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])4k(?:[^a-z0-9]|$)`)},
		{value: "1080p", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])1080p(?:[^a-z0-9]|$)`)},
		{value: "720p", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])720p(?:[^a-z0-9]|$)`)},
		{value: "480p", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])480p(?:[^a-z0-9]|$)`)},
		{value: "UHD", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])uhd(?:[^a-z0-9]|$)`)},
		{value: "HDR10", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdr10(?:[^a-z0-9]|$)`)},
		{value: "HDR", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdr(?:[^a-z0-9]|$)`)},
		{value: "DV", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dv(?:[^a-z0-9]|$)`)},
		{value: "Dolby Vision", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dolby[ ._-]*vision(?:[^a-z0-9]|$)`)},
	}
	sourcePatterns = []tokenPattern{
		{value: "BluRay", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])blu[ ._-]*ray(?:[^a-z0-9]|$)`)},
		{value: "BDRip", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])bdrip(?:[^a-z0-9]|$)`)},
		{value: "BRRip", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])brrip(?:[^a-z0-9]|$)`)},
		{value: "WEB-DL", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])web[ ._-]*dl(?:[^a-z0-9]|$)`)},
		{value: "WEBRip", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])webrip(?:[^a-z0-9]|$)`)},
		{value: "WEB", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])web(?:[^a-z0-9]|$)`)},
		{value: "HDTV", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdtv(?:[^a-z0-9]|$)`)},
		{value: "DVDRip", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dvd[ ._-]*rip(?:[^a-z0-9]|$)`)},
		{value: "DVD", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dvd(?:[^a-z0-9]|$)`)},
		{value: "HDRip", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdrip(?:[^a-z0-9]|$)`)},
		{value: "Remux", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])remux(?:[^a-z0-9]|$)`)},
	}
	codecPatterns = []tokenPattern{
		{value: "x265", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])x265(?:[^a-z0-9]|$)`)},
		{value: "x264", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])x264(?:[^a-z0-9]|$)`)},
		{value: "HEVC", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hevc(?:[^a-z0-9]|$)`)},
		{value: "AVC", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])avc(?:[^a-z0-9]|$)`)},
		{value: "H.265", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])h[ ._-]?265(?:[^a-z0-9]|$)`)},
		{value: "H.264", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])h[ ._-]?264(?:[^a-z0-9]|$)`)},
		{value: "AV1", re: regexp.MustCompile(`(?i)(?:^|[^a-z0-9])av1(?:[^a-z0-9]|$)`)},
	}
	noisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])2160p(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])4k(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])1080p(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])720p(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])480p(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])uhd(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdr10(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdr(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dv(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dolby[ ._-]*vision(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])blu[ ._-]*ray(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])bdrip(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])brrip(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])web[ ._-]*dl(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])webrip(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])web(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdtv(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dvd[ ._-]*rip(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dvd(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hdrip(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])remux(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])x265(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])x264(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hevc(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])avc(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])h[ ._-]?265(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])h[ ._-]?264(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])av1(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])atmos(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])truehd(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dts(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dts[ ._-]?hd(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])ddp?(?:[ ._-]?\d(?:\.\d)?)?(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dd\+(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])aac(?:[ ._-]?\d(?:\.\d)?)?(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])ac3(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])eac3(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])flac(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])opus(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:2\.0|5\.1|7\.1)(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])repack(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])proper(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])extended(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])directors?[ ._-]*cut(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])dir[ ._-]*cut(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])uncut(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])imax(?:[^a-z0-9]|$)`),
		regexp.MustCompile(`(?i)(?:^|[^a-z0-9])hmax(?:[^a-z0-9]|$)`),
	}
	metadataHyphenPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^web-dl$`),
		regexp.MustCompile(`(?i)^blu-ray$`),
		regexp.MustCompile(`(?i)^dvd-rip$`),
	}
)

// ParseFilename 解析视频文件名。
func ParseFilename(name string) (*ParsedName, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("empty filename")
	}

	base := filepath.Base(name)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	p := &ParsedName{
		Extension: strings.TrimPrefix(strings.ToLower(ext), "."),
	}

	stem = strings.TrimSpace(stem)
	stem = extractGroup(stem, p)
	stem = parseSeasonEpisode(stem, p)
	stem = parseYear(stem, p)

	p.Quality = extractFirst(stem, qualityPatterns)
	p.Source = extractFirst(stem, sourcePatterns)
	p.Codec = extractFirst(stem, codecPatterns)

	stem = stripNoise(stem)
	stem = cleanTitleStem(stem)
	p.Title = stem

	return p, nil
}

func extractGroup(stem string, p *ParsedName) string {
	idx := strings.LastIndex(stem, "-")
	if idx <= 0 {
		return stem
	}
	if hyphenBelongsToToken(stem, idx) {
		return stem
	}
	candidate := strings.TrimSpace(stem[idx+1:])
	if candidate == "" || strings.Contains(candidate, " ") || !reGroup.MatchString(candidate) {
		return stem
	}
	if looksLikeMetadata(candidate) || !looksLikeReleaseGroup(candidate) {
		return stem
	}
	p.Group = strings.Trim(candidate, "[]")
	return strings.TrimSpace(stem[:idx])
}

func parseSeasonEpisode(stem string, p *ParsedName) string {
	patterns := []*regexp.Regexp{reSxxExx, reNxN, reSeasonEpisode}
	for _, re := range patterns {
		idx := re.FindStringSubmatchIndex(stem)
		if len(idx) < 6 {
			continue
		}
		season, _ := strconv.Atoi(stem[idx[2]:idx[3]])
		episode, _ := strconv.Atoi(stem[idx[4]:idx[5]])
		p.Season = season
		p.Episode = episode
		p.IsShow = season > 0 || episode > 0
		return strings.TrimSpace(stem[:idx[0]] + " " + stem[idx[1]:])
	}

	if idx := reEpisodeOnly.FindStringSubmatchIndex(stem); len(idx) >= 4 {
		episode, _ := strconv.Atoi(stem[idx[2]:idx[3]])
		p.Episode = episode
		p.IsShow = episode > 0
		return strings.TrimSpace(stem[:idx[0]] + " " + stem[idx[1]:])
	}

	if (strings.Contains(stem, "[") || strings.Contains(stem, "]")) && reAnimeEpisode.MatchString(stem) {
		idx := reAnimeEpisode.FindStringSubmatchIndex(stem)
		if len(idx) >= 4 {
			episode, _ := strconv.Atoi(stem[idx[2]:idx[3]])
			p.Episode = episode
			p.IsShow = episode > 0
			return strings.TrimSpace(stem[:idx[0]] + " " + stem[idx[1]:])
		}
	}

	return stem
}

func parseYear(stem string, p *ParsedName) string {
	indices := reYear.FindAllStringSubmatchIndex(stem, -1)
	maxYear := time.Now().Year() + 5
	for _, idx := range indices {
		if len(idx) < 4 {
			continue
		}
		year, err := strconv.Atoi(stem[idx[2]:idx[3]])
		if err != nil || year < 1900 || year > maxYear {
			continue
		}
		p.Year = year
		return strings.TrimSpace(stem[:idx[0]] + " " + stem[idx[1]:])
	}
	return stem
}

func extractFirst(s string, patterns []tokenPattern) string {
	for _, pattern := range patterns {
		if pattern.re.MatchString(s) {
			return pattern.value
		}
	}
	return ""
}

func stripNoise(s string) string {
	for _, re := range noisePatterns {
		s = re.ReplaceAllString(s, " ")
	}
	return s
}

func cleanTitleStem(stem string) string {
	stem = reLeadingBracket.ReplaceAllString(stem, "")
	stem = reSep.ReplaceAllString(stem, " ")
	stem = reJunk.ReplaceAllString(stem, " ")
	stem = reEmptyBrackets.ReplaceAllString(stem, " ")
	stem = reDashSep.ReplaceAllString(stem, " ")
	stem = reTrimPunct.ReplaceAllString(stem, "")
	stem = reMultiSpace.ReplaceAllString(stem, " ")
	return strings.TrimSpace(stem)
}

func hyphenBelongsToToken(stem string, idx int) bool {
	start := idx - 1
	for start >= 0 {
		r := stem[start]
		if r == ' ' || r == '.' || r == '_' || r == '[' || r == '(' {
			break
		}
		start--
	}
	end := idx + 1
	for end < len(stem) {
		r := stem[end]
		if r == ' ' || r == '.' || r == '_' || r == ']' || r == ')' {
			break
		}
		end++
	}
	if start+1 >= end {
		return false
	}
	token := stem[start+1 : end]
	for _, pattern := range metadataHyphenPatterns {
		if pattern.MatchString(token) {
			return true
		}
	}
	return false
}

func looksLikeMetadata(s string) bool {
	for _, pattern := range qualityPatterns {
		if pattern.re.MatchString(s) {
			return true
		}
	}
	for _, pattern := range sourcePatterns {
		if pattern.re.MatchString(s) {
			return true
		}
	}
	for _, pattern := range codecPatterns {
		if pattern.re.MatchString(s) {
			return true
		}
	}
	return false
}

func looksLikeReleaseGroup(candidate string) bool {
	candidate = strings.Trim(candidate, "[]")
	if candidate == "" {
		return false
	}

	var upperCount int
	for _, r := range candidate {
		if unicode.IsUpper(r) {
			upperCount++
		}
	}

	return upperCount >= 2 || strings.ContainsAny(candidate, ".0123456789")
}
