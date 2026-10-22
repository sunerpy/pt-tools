package metadata

import (
	"cmp"
	"slices"
	"strings"
)

// 海报与 Logo 优先的语言（媒体库的「图片语言」）。
const (
	LangAuto = ""     // 跟识别语言：用来源自己挑的那张（TMDB 按识别语言给的）
	LangNone = "none" // 没有文字的
)

// FanartOnly 报告这种图片只有 Fanart.tv 有（TMDB 没有）。
func FanartOnly(kind string) bool {
	switch kind {
	case ArtClearart, ArtBanner, ArtLandscape, ArtDisc:
		return true
	}
	return false
}

// sortArt 把同一来源同一种图片按评分从高到低排（同分保持原来的顺序）。
func sortArt(in []Artwork) {
	slices.SortStableFunc(in, func(a, b Artwork) int { return cmp.Compare(b.Likes, a.Likes) })
}

// choose 从一个来源的候选（同一种、同一季同一集，已按评分排好）里挑一张：
//   - 海报与季海报：选了图片语言时优先用这种语言的（none 是没有文字的），其次是来源自己挑的那张（Default），
//     再按识别语言、英文、没有文字的找，最后是评分最高的；
//   - 背景：选了图片语言时优先用没有文字的，其次是来源自己挑的那张，再优先没有文字的，最后是评分最高的；
//   - Logo 与其它：依次找选的语言（none 是没有文字的）、识别语言、英文、没有文字的，最后是评分最高的；
//   - 集截图：来源给的那张。
//
// 这样 TMDB 的海报与背景在默认设置下仍是 TMDB 按识别语言给的那张，和以前一样。
func choose(cands []Artwork, kind, imageLang, recogLang string) *Artwork {
	if len(cands) == 0 {
		return nil
	}
	want := imageLang
	if imageLang == LangNone {
		want = ""
	}
	recog, _, _ := strings.Cut(strings.ToLower(recogLang), "-")
	byLang := func(lang string) *Artwork {
		for i := range cands {
			if !cands[i].Default && cands[i].Lang == lang {
				return &cands[i]
			}
		}
		return nil
	}
	def := func() *Artwork {
		for i := range cands {
			if cands[i].Default {
				return &cands[i]
			}
		}
		return nil
	}
	first := func(langs ...string) *Artwork {
		for _, l := range langs {
			if a := byLang(l); a != nil {
				return a
			}
		}
		return &cands[0]
	}
	switch kind {
	case ArtThumb:
		if a := def(); a != nil {
			return a
		}
		return &cands[0]
	case ArtPoster, ArtSeasonPoster:
		if imageLang != LangAuto {
			if a := byLang(want); a != nil {
				return a
			}
		}
		if a := def(); a != nil {
			return a
		}
		return first(recog, "en", "")
	case ArtFanart:
		if imageLang != LangAuto {
			if a := byLang(""); a != nil {
				return a
			}
		}
		if a := def(); a != nil {
			return a
		}
		return first("")
	}
	var prefs []string
	if imageLang != LangAuto {
		prefs = append(prefs, want)
	}
	prefs = append(prefs, recog, "en", "")
	return first(prefs...)
}
