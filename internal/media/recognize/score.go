package recognize

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// AcceptScore 是认定为匹配的最低分；名字至少要部分相同（titleScore ≥ 0.75）。
// 名字只是部分相同时，第一名还要比第二名高出 AcceptMargin，否则只列候选。
const (
	AcceptScore  = 0.95
	AcceptMargin = 0.1
)

var foldMarks = runes.Remove(runes.In(unicode.Mn))

// Normalize 规整名字用于比较（订阅对种子标题时也用），规则见 normalize。
func Normalize(s string) string { return normalize(s) }

// normalize 规整名字用于比较：去掉重音（Shōgun → shogun）、转小写、只留字母和数字（含汉字）。
func normalize(s string) string {
	t := transform.Chain(norm.NFD, foldMarks, norm.NFC)
	folded, _, err := transform.String(t, s)
	if err != nil {
		folded = s
	}
	var b strings.Builder
	for _, r := range strings.ToLower(folded) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	digitsRe = regexp.MustCompile(`\d+`)
	wordsRe  = regexp.MustCompile(`[\p{L}\p{N}]+`)
	romanRe  = regexp.MustCompile(`^(ii|iii|iv|v|vi|vii|viii|ix|x)$`)
)

// sequelNumbers 是名字里的数字与罗马数字（续集编号）：Inside Out 2 → "2"，The Wandering Earth II → "ii"。
func sequelNumbers(s string) string {
	low := strings.ToLower(s)
	nums := digitsRe.FindAllString(low, -1)
	for _, w := range wordsRe.FindAllString(low, -1) {
		if romanRe.MatchString(w) {
			nums = append(nums, w)
		}
	}
	sort.Strings(nums)
	return strings.Join(nums, " ")
}

// titleScore 比较解析出的中英文名与条目的名字、原名：相同 1.0；一个包含另一个、长度相近（短的不少于长的六成）
// 且数字一样（续集编号对得上，Inside Out 不算包含于 Inside Out 2）0.75。
func titleScore(m meta.Meta, r tmdb.Result) float64 {
	best := 0.0
	for _, a := range []string{m.NameCN, m.NameEN} {
		na := normalize(a)
		if na == "" {
			continue
		}
		for _, b := range []string{r.Title, r.OriginalTitle} {
			nb := normalize(b)
			if nb == "" {
				continue
			}
			if na == nb {
				return 1
			}
			la, lb := len([]rune(na)), len([]rune(nb))
			short, long := min(la, lb), max(la, lb)
			if (strings.Contains(na, nb) || strings.Contains(nb, na)) && short*10 >= long*6 && sequelNumbers(a) == sequelNumbers(b) {
				best = 0.75
			}
		}
	}
	return best
}

// score 给一个候选打分：名字（1 / 0.75 / 0）+ 年份（相同 +0.25，差一年 +0.1，对不上 -0.3）
// + 类型相同 +0.05 + 搜索排名（第一 +0.1，第二 +0.05）。剧集后面几季的年份晚于首播年份，不扣分。
func score(m meta.Meta, r tmdb.Result, rank int) float64 {
	s := titleScore(m, r)
	if m.Year > 0 && r.Year > 0 {
		d := m.Year - r.Year
		switch {
		case d == 0:
			s += 0.25
		case d == 1 || d == -1:
			s += 0.1
		case r.MediaType == tmdb.KindTV && d > 0:
		default:
			s -= 0.3
		}
	}
	if m.Type != meta.TypeUnknown && string(m.Type) == r.MediaType {
		s += 0.05
	}
	switch rank {
	case 0:
		s += 0.1
	case 1:
		s += 0.05
	}
	return s
}

// pick 返回可以认定的候选（候选已按分数从高到低排好）：名字至少部分相同、总分过线；
// 名字只是部分相同时还要比第二名高出 AcceptMargin。没有时返回 nil。
func pick(m meta.Meta, cands []Candidate) *Candidate {
	if len(cands) == 0 {
		return nil
	}
	ts := titleScore(m, cands[0].Result)
	if ts < 0.75 || cands[0].Score < AcceptScore {
		return nil
	}
	if ts < 1 && len(cands) > 1 && cands[0].Score-cands[1].Score < AcceptMargin {
		return nil
	}
	return &cands[0]
}

// overrideKey 是用一个名字算出的纠正键：类型、名字，电影另加年份（同名的翻拍片按年份区分）；
// 剧集不加年份（各季年份不同）。名字为空时为空串。
func overrideKey(m meta.Meta, name string) string {
	n := normalize(name)
	if n == "" {
		return ""
	}
	k := string(m.Type) + "|" + n
	if m.Type != meta.TypeTV && m.Year > 0 {
		k += "|" + strconv.Itoa(m.Year)
	}
	if len(k) > 200 {
		sum := sha1.Sum([]byte(k))
		k = "h|" + hex.EncodeToString(sum[:])
	}
	return k
}

// overrideKeys 是保存纠正用的主键与别名：有英文名时主键用英文名、中文名作别名，否则主键用中文名。
// 这样同一个标题有没有中文副标题都能命中。解析不出名字时主键为空串。
func overrideKeys(m meta.Meta) (primary, alt string) {
	en, cn := overrideKey(m, m.NameEN), overrideKey(m, m.NameCN)
	if en != "" {
		return en, cn
	}
	return cn, ""
}
