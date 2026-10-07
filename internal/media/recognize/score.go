package recognize

import (
	"crypto/sha1"
	"encoding/hex"
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
const AcceptScore = 0.95

var foldMarks = runes.Remove(runes.In(unicode.Mn))

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

// titleScore 比较解析出的中英文名与条目的名字、原名：相同 1.0；一个包含另一个且长度相近（短的不少于长的六成）0.75。
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
			if (strings.Contains(na, nb) || strings.Contains(nb, na)) && short*10 >= long*6 {
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

// accepted 判断一个候选是否可以认定为匹配。
func accepted(m meta.Meta, c Candidate) bool {
	return titleScore(m, c.Result) >= 0.75 && c.Score >= AcceptScore
}

// overrideKey 是手动纠正用的键：类型、中英文名，电影另加年份（同名的翻拍片按年份区分）；剧集不加年份（各季年份不同）。
// 解析不出名字时为空串。
func overrideKey(m meta.Meta) string {
	en, cn := normalize(m.NameEN), normalize(m.NameCN)
	if en == "" && cn == "" {
		return ""
	}
	k := string(m.Type) + "|" + en + "|" + cn
	if m.Type != meta.TypeTV && m.Year > 0 {
		k += "|" + strconv.Itoa(m.Year)
	}
	if len(k) > 200 {
		sum := sha1.Sum([]byte(k))
		k = "h|" + hex.EncodeToString(sum[:])
	}
	return k
}
