package llm

import (
	"regexp"
	"strings"
	"time"
)

var (
	imdbIDRegex     = regexp.MustCompile(`^tt\d{7,9}$`)
	urlRegex        = regexp.MustCompile(`https?://\S+|www\.\S+`)
	releaseTagRegex = regexp.MustCompile(`(?i)\b(1080p|2160p|4k|720p|480p|hdr|dv|dolby[ .-]?vision|bluray|web[ .-]?dl|webrip|bdrip|hdtv|dvdrip|x265|x264|h\.265|h\.264|hevc|avc|atmos|dts(?:-hd)?|aac|ac3|flac|truehd|remux)\b`)
	spaceRegex      = regexp.MustCompile(`\s+`)
	trailingRegex   = regexp.MustCompile(`[-\s._]+$`)
)

// sanitize 应用反幻觉规则到 NFOResult。
func sanitize(r *NFOResult) {
	if r == nil {
		return
	}

	currentYear := time.Now().Year()
	if r.Year != 0 && (r.Year < 1900 || r.Year > currentYear+5) {
		r.Year = 0
	}
	if r.TMDBID != 0 && (r.TMDBID < 1 || r.TMDBID > 10_000_000) {
		r.TMDBID = 0
	}
	if r.IMDBID != "" && !imdbIDRegex.MatchString(r.IMDBID) {
		r.IMDBID = ""
	}

	r.Title = cleanFreeText(r.Title)
	r.OriginalTitle = cleanFreeText(r.OriginalTitle)
	r.Plot = cleanFreeText(r.Plot)
	r.Language = strings.TrimSpace(r.Language)

	switch strings.ToLower(strings.TrimSpace(r.Type)) {
	case "movie", "tv":
		r.Type = strings.ToLower(strings.TrimSpace(r.Type))
	default:
		r.Type = "unknown"
	}

	if r.Season < 0 || r.Season > 100 {
		r.Season = 0
	}
	if r.Episode < 0 || r.Episode > 10000 {
		r.Episode = 0
	}
	if r.Runtime < 0 || r.Runtime > 600 {
		r.Runtime = 0
	}

	r.Cast = cleanSlice(r.Cast, 20)
	r.Directors = cleanSlice(r.Directors, 10)
	r.Genres = cleanSlice(r.Genres, 10)
}

func stripReleaseTags(s string) string {
	s = releaseTagRegex.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "[]", "")
	s = strings.ReplaceAll(s, "()", "")
	s = spaceRegex.ReplaceAllString(s, " ")
	s = trailingRegex.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func cleanFreeText(s string) string {
	s = strings.TrimSpace(urlRegex.ReplaceAllString(s, ""))
	return stripReleaseTags(s)
}

func cleanSlice(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := cleanFreeText(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, value)
		if max > 0 && len(cleaned) >= max {
			break
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}
