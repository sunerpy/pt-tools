package meta

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// cnMarkerRe 是不属于名字的中文词：季与集、语言与字幕、发布说明
	cnMarkerRe = regexp.MustCompile(`^(?:第[\d一二三四五六七八九十]+[季部集期]|全\d+集|\d+集全|` +
		`(?:国语|国配|粤语|粤配|普通话|中字|中英|简繁|繁简|简中|繁中|中文|字幕|双语|官方|首发|禁转|独占|内封|内嵌|特效|` +
		`无水印|高码|杜比|视界|完结|更新至?|连载|版本|[国粤日英韩泰]|[\d\-~～至第集全季部期])+)$`)
	cnTrailRe = regexp.MustCompile(`(?:第[\d一二三四五六七八九十]+[季部集期]|全\d+集)+$`)
	// 副标题里分隔名字与其它信息的符号
	cnSepRe = regexp.MustCompile(`[|｜/／\[\]【】*丨]`)
)

// cnName 规整一段中文名：去掉末尾的季与集（庆余年第二季 → 庆余年）。
func cnName(s string) string {
	s = strings.TrimSpace(s)
	s = cnTrailRe.ReplaceAllString(s, "")
	return strings.Trim(s, " ：:·-")
}

// firstCN 是名字区里第一段中文名：连续的中文词（跳过季、集、语言等标记），遇到英文词就停。
func firstCN(tokens []string) string {
	var parts []string
	for _, tk := range tokens {
		switch {
		case hasHan(tk) && !cnMarkerRe.MatchString(tk):
			parts = append(parts, tk)
		case len(parts) > 0:
			return cnName(strings.Join(parts, " "))
		}
	}
	return cnName(strings.Join(parts, " "))
}

// subtitleCN 从副标题里取中文名：按 | / 【】 等分段，取第一段里开头连续的中文词。
func subtitleCN(sub string) string {
	for _, seg := range cnSepRe.Split(sub, -1) {
		var parts []string
		for _, tk := range strings.Fields(seg) {
			if !hasHan(tk) || cnMarkerRe.MatchString(tk) {
				break
			}
			parts = append(parts, tk)
		}
		if name := cnName(strings.Join(parts, " ")); name != "" && hasHan(name) {
			return name
		}
	}
	return ""
}

var cnDigits = map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}

// chineseNumber 把 1–99 的中文数字（或阿拉伯数字）转成整数；转不了时为 0。
func chineseNumber(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	rs := []rune(s)
	switch {
	case len(rs) == 1 && rs[0] == '十':
		return 10
	case len(rs) == 1:
		return cnDigits[rs[0]]
	case len(rs) == 2 && rs[0] == '十':
		return 10 + cnDigits[rs[1]]
	case len(rs) == 2 && rs[1] == '十':
		return cnDigits[rs[0]] * 10
	case len(rs) == 3 && rs[1] == '十':
		return cnDigits[rs[0]]*10 + cnDigits[rs[2]]
	}
	return 0
}
