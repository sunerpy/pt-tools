package v2

import (
	"regexp"
	"strings"
)

var (
	imdbIDRe = regexp.MustCompile(`(?i)\b(tt\d{5,10})\b`)
	// 只认电影条目：movie.douban.com/subject、m.douban.com/movie/subject 与 www.douban.com/subject（不认 book、music）
	doubanIDRe   = regexp.MustCompile(`(?i)(?:^|//)(?:movie\.|m\.|www\.)?douban\.com/(?:movie/)?subject/(\d{4,12})`)
	doubanBareRe = regexp.MustCompile(`^\d{4,12}$`)
)

// NormalizeIMDbID 从 IMDb 链接或编号里取出 tt 开头的编号（如 tt0111161）；取不到时为空串。
func NormalizeIMDbID(s string) string {
	m := imdbIDRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// NormalizeDoubanID 从豆瓣电影条目链接或纯数字编号里取出数字编号；取不到时为空串。
func NormalizeDoubanID(s string) string {
	s = strings.TrimSpace(s)
	if doubanBareRe.MatchString(s) {
		return s
	}
	if m := doubanIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}
