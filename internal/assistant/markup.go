package assistant

import (
	"strings"
	"unicode"
)

// toolMarkup 是模型自己的工具调用标记的开头。部分 OpenAI 兼容服务（例如 DeepSeek 经某些网关）把调用解析成
// tool_calls 以后，正文里还留着这些标记：从第一个标记起都不是给人看的，也不回传给模型。
// DeepSeek V3.2 是 DSML（<｜DSML｜function_calls>…），更早的 V3、R1 是 <｜tool▁calls▁begin｜>…。
var toolMarkup = []string{"<｜DSML｜", "<｜tool▁calls▁begin｜>", "<｜tool▁call▁begin｜>"}

// cutToolMarkup 从第一个工具调用标记起截掉（不去空白，流式增量用）。
func cutToolMarkup(s string) string {
	cut := len(s)
	for _, m := range toolMarkup {
		if i := strings.Index(s, m); i >= 0 && i < cut {
			cut = i
		}
	}
	return s[:cut]
}

// stripToolMarkup 去掉正文里的工具调用标记与它前面的空白（落库、回传给模型的回答用）。
func stripToolMarkup(s string) string {
	cut := cutToolMarkup(s)
	if len(cut) == len(s) {
		return s
	}
	return strings.TrimRightFunc(cut, unicode.IsSpace)
}

// markupHold 是流式增量可以先发的长度：末尾可能是标记开头的一段（例如刚收到「<｜」）先不发。
func markupHold(s string) int {
	keep := len(s)
	for _, m := range toolMarkup {
		for n := min(len(m)-1, len(s)); n > 0; n-- {
			if strings.HasSuffix(s, m[:n]) {
				keep = min(keep, len(s)-n)
				break
			}
		}
	}
	return keep
}
