package imdb

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// titleDetail — 从 IMDb /title/{ttID}/ 页面解析出的结构化元数据。
// 字段来源（优先级）：JSON-LD > og:meta > 页面 DOM。
type titleDetail struct {
	ID            string
	Type          string
	Title         string
	OriginalTitle string
	Year          int
	Plot          string
	Rating        float64
	RatingCount   int
	Genres        []string
	Directors     []string
	Actors        []string
	Runtime       int
	PosterURL     string
	IMDBID        string
}

// parseTitlePage 解析 IMDb 详情页 HTML。提取策略：
//  1. 优先从 <script type="application/ld+json"> 抽取（IMDb 官方结构化数据，稳定）
//  2. 回退到 og:* meta 标签（标题、海报）
//  3. DOM 选择器兜底（title/release date）
//
// 与 TMM 的 `ImdbParser.java` 相同策略，但只保留核心字段以降低对 DOM 结构变化的敏感度。
func parseTitlePage(ttID string, body []byte) (*titleDetail, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("imdb parse html: %w: %v", core.ErrParseFailed, err)
	}

	// 检测 AWS WAF challenge 页（完全空内容 + <title>仅引导脚本）。
	if isWAFChallenge(doc) {
		return nil, fmt.Errorf("imdb returned AWS WAF challenge (current IP may be blocked): %w", core.ErrProviderDown)
	}

	detail := &titleDetail{ID: ttID, IMDBID: ttID}

	// 1. JSON-LD
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if applied := applyJSONLD(detail, s.Text()); applied {
			return false // 找到第一个可用的即止
		}
		return true
	})

	// 2. OG meta 补齐
	if detail.Title == "" {
		detail.Title = cleanText(doc.Find(`meta[property="og:title"]`).AttrOr("content", ""))
	}
	if detail.PosterURL == "" {
		detail.PosterURL = cleanText(doc.Find(`meta[property="og:image"]`).AttrOr("content", ""))
	}
	if detail.Plot == "" {
		detail.Plot = cleanText(doc.Find(`meta[name="description"]`).AttrOr("content", ""))
	}

	// 3. DOM fallback — <title> 里常含 "Movie Title (2010) - IMDb"
	if detail.Title == "" {
		domTitle := cleanText(doc.Find("title").First().Text())
		if idx := strings.LastIndex(domTitle, " - IMDb"); idx > 0 {
			domTitle = domTitle[:idx]
		}
		detail.Title = domTitle
	}

	if detail.Title == "" {
		return nil, fmt.Errorf("imdb html title missing for %s: %w", ttID, core.ErrParseFailed)
	}
	return detail, nil
}

// isWAFChallenge 检测 AWS WAF challenge 页面特征。
// 这类页面 body 里只有 `<div id="challenge-container">` 和 JS 跳转脚本，无业务内容。
func isWAFChallenge(doc *goquery.Document) bool {
	return doc.Find(`div#challenge-container`).Length() > 0 ||
		strings.Contains(doc.Find("title").First().Text(), "Just a moment")
}

// applyJSONLD 解析 IMDb 的 schema.org JSON-LD 块并填充 detail。
// 返回 true 表示该块包含了有用的字段（用于 EachWithBreak 终止后续遍历）。
func applyJSONLD(detail *titleDetail, raw string) bool {
	var ld struct {
		Type            string          `json:"@type"`
		URL             string          `json:"url"`
		Name            string          `json:"name"`
		AlternateName   string          `json:"alternateName"`
		Image           string          `json:"image"`
		Description     string          `json:"description"`
		Genre           json.RawMessage `json:"genre"`
		DatePublished   string          `json:"datePublished"`
		Duration        string          `json:"duration"`
		AggregateRating struct {
			RatingValue json.Number `json:"ratingValue"`
			RatingCount json.Number `json:"ratingCount"`
		} `json:"aggregateRating"`
		Director json.RawMessage `json:"director"`
		Actor    json.RawMessage `json:"actor"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &ld); err != nil {
		return false
	}
	if ld.Type == "" && ld.Name == "" {
		return false
	}

	detail.Type = ld.Type
	if detail.Title == "" {
		detail.Title = cleanText(ld.Name)
	}
	if detail.OriginalTitle == "" {
		detail.OriginalTitle = cleanText(firstNonEmpty(ld.AlternateName, ld.Name))
	}
	if detail.Plot == "" {
		detail.Plot = cleanText(ld.Description)
	}
	if detail.PosterURL == "" {
		detail.PosterURL = cleanText(ld.Image)
	}
	if detail.Year == 0 && ld.DatePublished != "" {
		if y := extractYear(ld.DatePublished); y > 0 {
			detail.Year = y
		}
	}
	if detail.Runtime == 0 && ld.Duration != "" {
		detail.Runtime = parseISODurationMinutes(ld.Duration)
	}
	if rating, err := ld.AggregateRating.RatingValue.Float64(); err == nil && rating > 0 {
		detail.Rating = rating
	}
	if count, err := ld.AggregateRating.RatingCount.Int64(); err == nil {
		detail.RatingCount = int(count)
	}
	if len(ld.Genre) > 0 {
		detail.Genres = decodeStringOrArray(ld.Genre)
	}
	if len(ld.Director) > 0 {
		detail.Directors = decodeNameList(ld.Director)
	}
	if len(ld.Actor) > 0 {
		detail.Actors = decodeNameList(ld.Actor)
	}
	return true
}

// decodeStringOrArray 解析 JSON-LD 中可能是 string 或 []string 的字段。
func decodeStringOrArray(raw json.RawMessage) []string {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return splitAndTrim(asString)
	}
	var asArray []string
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray
	}
	return nil
}

// decodeNameList 解析 JSON-LD 中 Person 对象数组（或单对象），提取 name 字段。
func decodeNameList(raw json.RawMessage) []string {
	var asArray []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &asArray); err == nil {
		out := make([]string, 0, len(asArray))
		for _, p := range asArray {
			if name := strings.TrimSpace(p.Name); name != "" {
				out = append(out, name)
			}
		}
		return out
	}
	var asObject struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil && asObject.Name != "" {
		return []string{asObject.Name}
	}
	return nil
}

var yearOnlyPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

func extractYear(s string) int {
	match := yearOnlyPattern.FindString(s)
	if match == "" {
		return 0
	}
	y, _ := strconv.Atoi(match)
	return y
}

// parseISODurationMinutes 把 ISO 8601 duration（IMDb 格式形如 "PT2H28M"）转为分钟。
// 只支持 H/M；丢弃秒（对于影视场景无意义）。
var isoDurationPattern = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?`)

func parseISODurationMinutes(iso string) int {
	m := isoDurationPattern.FindStringSubmatch(strings.TrimSpace(iso))
	if m == nil {
		return 0
	}
	hours, _ := strconv.Atoi(m[1])
	mins, _ := strconv.Atoi(m[2])
	return hours*60 + mins
}

func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := regexp.MustCompile(`[,;/]`).Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func cleanText(s string) string { return strings.TrimSpace(s) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func randomUserAgent() string {
	return defaultUserAgents[rand.Intn(len(defaultUserAgents))]
}
