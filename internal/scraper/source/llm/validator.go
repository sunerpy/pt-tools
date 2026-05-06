package llm

import (
	"context"
	"regexp"
	"strings"
	"time"
)

var imdbIDValidatorRegex = regexp.MustCompile(`^tt\d{7,9}$`)

// ValidateAgainstTMDB 使用 TMDB 对 LLM 产出的 tmdb_id 做宽松校验。
func ValidateAgainstTMDB(ctx context.Context, nfo *NFOResult, tmdbValidator TMDBValidator, mediaType string) (*NFOResult, string) {
	if nfo == nil {
		return nil, ""
	}
	if tmdbValidator == nil {
		return nfo, "TMDB 不可用，跳过 ID 验证"
	}
	if nfo.TMDBID <= 0 {
		return nfo, ""
	}

	var (
		info *ValidationInfo
		err  error
	)

	switch mediaType {
	case "movie":
		info, err = tmdbValidator.ValidateMovie(ctx, nfo.TMDBID)
	case "tv":
		info, err = tmdbValidator.ValidateTvShow(ctx, nfo.TMDBID)
	default:
		return nfo, ""
	}

	if err != nil || info == nil {
		nfo.TMDBID = 0
		return nfo, "TMDB 验证失败，清空 ID"
	}

	lowerLLM := strings.ToLower(strings.TrimSpace(nfo.Title))
	lowerTMDB := strings.ToLower(strings.TrimSpace(info.Title))
	if lowerLLM != "" && lowerTMDB != "" && !strings.Contains(lowerLLM, lowerTMDB) && !strings.Contains(lowerTMDB, lowerLLM) {
		nfo.TMDBID = 0
		return nfo, "TMDB 标题与 LLM 不匹配，清空 ID"
	}
	if info.Year > 0 && nfo.Year > 0 && abs(info.Year-nfo.Year) > 1 {
		nfo.TMDBID = 0
		return nfo, "TMDB 年份与 LLM 相差 >1 年，清空 ID"
	}

	return nfo, ""
}

// ValidateFieldFormat 对 sanitize 后结果做边界检查并返回 warning 列表。
func ValidateFieldFormat(nfo *NFOResult) []string {
	var warnings []string
	if nfo == nil {
		return warnings
	}
	if nfo.Year != 0 && nfo.Year < 1900 {
		warnings = append(warnings, "year < 1900")
	}
	if nfo.Year > time.Now().Year()+5 {
		warnings = append(warnings, "year too far in future")
	}
	if nfo.TMDBID > 0 && nfo.TMDBID > 10_000_000 {
		warnings = append(warnings, "tmdb_id out of range")
	}
	if nfo.IMDBID != "" && !imdbIDValidatorRegex.MatchString(nfo.IMDBID) {
		warnings = append(warnings, "imdb_id invalid format")
	}
	return warnings
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
