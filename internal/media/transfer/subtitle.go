package transfer

import (
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var subTokenRe = regexp.MustCompile(`[\s._\-\[\]()&+,]+`)

// 字幕语言的写法（比较前转小写）。顺序就是优先级：简体 → 繁体 → 中文 → 其它；双语字幕取第一种。
var subLangs = []struct {
	tag    string
	tokens []string
	words  []string // 直接在文件名里找的中文写法
}{
	{"zh-CN", []string{"chs", "sc", "gb", "hans", "zhcn", "zh-cn", "zh-hans", "简体", "简中"}, []string{"简体", "简中", "简日", "简英", "简繁"}},
	{"zh-TW", []string{"cht", "tc", "big5", "hant", "zhtw", "zh-tw", "zh-hk", "zh-hant", "繁体", "繁中"}, []string{"繁体", "繁體", "繁中", "繁日", "繁英"}},
	{"zh", []string{"chi", "zho", "zh", "chinese", "中文", "中字"}, []string{"中文", "中字", "双语"}},
	{"en", []string{"eng", "en", "english"}, []string{"英文"}},
	{"ja", []string{"jpn", "ja", "jp", "japanese"}, []string{"日文", "日语"}},
	{"ko", []string{"kor", "ko", "kr", "korean"}, []string{"韩文", "韩语"}},
}

// SubtitleLang 从字幕文件名里认出语言，返回放进库里时加的后缀（zh-CN、zh-TW、zh、en、ja、ko）；认不出返回空串。
func SubtitleLang(name string) string {
	stem := strings.ToLower(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	// zh-CN 这种带连字符的写法先整体找，再按分隔符拆开找
	tokens := map[string]bool{}
	for _, t := range subTokenRe.Split(stem, -1) {
		if t != "" {
			tokens[t] = true
		}
	}
	for _, l := range subLangs {
		for _, t := range l.tokens {
			if strings.Contains(t, "-") {
				if strings.Contains(stem, t) {
					return l.tag
				}
				continue
			}
			if tokens[t] {
				return l.tag
			}
		}
		for _, w := range l.words {
			if strings.Contains(stem, w) {
				return l.tag
			}
		}
	}
	return ""
}

// SubtitleTarget 是字幕放进库里的路径：视频的目标路径去掉扩展名，加上语言后缀与字幕的扩展名。
// n 大于 1 时在语言后面加序号（同一种语言有几份字幕时）。
func SubtitleTarget(videoTarget, subName string, n int) string {
	base := strings.TrimSuffix(videoTarget, filepath.Ext(videoTarget))
	if lang := SubtitleLang(subName); lang != "" {
		base += "." + lang
	}
	if n > 1 {
		base += "." + strconv.Itoa(n)
	}
	return base + strings.ToLower(path.Ext(subName))
}

// MatchSubtitles 把字幕分给视频：只有一个视频时全给它；否则先按文件名前缀（字幕名以视频名开头），
// 再按 key（季与集）一样来分。分不出去的字幕不整理。返回值的下标对应 videos。
func MatchSubtitles(videos, subs []File, key func(File) string) [][]File {
	out := make([][]File, len(videos))
	if len(videos) == 0 {
		return out
	}
	if len(videos) == 1 {
		out[0] = append(out[0], subs...)
		return out
	}
	stems := make([]string, len(videos))
	keys := make([]string, len(videos))
	for i, v := range videos {
		stems[i] = strings.ToLower(strings.TrimSuffix(path.Base(v.Rel), path.Ext(v.Rel)))
		if key != nil {
			keys[i] = key(v)
		}
	}
	for _, s := range subs {
		sub := strings.ToLower(path.Base(s.Rel))
		best := -1
		for i, st := range stems {
			if strings.HasPrefix(sub, st) && (best < 0 || len(st) > len(stems[best])) {
				best = i
			}
		}
		if best < 0 && key != nil {
			if k := key(s); k != "" {
				for i := range keys {
					if keys[i] == k {
						if best >= 0 { // 两个视频的季集一样，分不清
							best = -1
							break
						}
						best = i
					}
				}
			}
		}
		if best >= 0 {
			out[best] = append(out[best], s)
		}
	}
	return out
}
