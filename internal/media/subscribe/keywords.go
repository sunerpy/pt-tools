package subscribe

import (
	"strings"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// maxKeywordsLen 是包含词、排除词各自最长多少字节（与列宽相同）。
const maxKeywordsLen = 1024

// keywordSeparators 把空格、点、下划线当作一样：同一个种子在各站点的标题有的用空格、有的用点。
var keywordSeparators = strings.NewReplacer(".", " ", "_", " ")

// normalizeKeywordText 转小写，空格、点、下划线连在一起的算一个空格。
func normalizeKeywordText(s string) string {
	return strings.Join(strings.Fields(keywordSeparators.Replace(strings.ToLower(s))), " ")
}

// splitKeywords 按换行与逗号拆关键词（与刷流任务的写法相同），去空、规整。
func splitKeywords(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' || r == '，' }) {
		if kw := normalizeKeywordText(part); kw != "" {
			out = append(out, kw)
		}
	}
	return out
}

// keywordReject 是候选种子不合订阅的包含词、排除词时的原因（写进运行记录）；合的时候是空串。
func keywordReject(sub *models.MediaSubscription, it v2.TorrentItem) string {
	text := normalizeKeywordText(it.Title + " " + it.Subtitle)
	if inc := splitKeywords(sub.IncludeKeywords); len(inc) > 0 {
		hit := false
		for _, kw := range inc {
			if strings.Contains(text, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return "标题不含任何包含词"
		}
	}
	for _, kw := range splitKeywords(sub.ExcludeKeywords) {
		if strings.Contains(text, kw) {
			return "标题含排除词 " + kw
		}
	}
	return ""
}
