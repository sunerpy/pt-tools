package scrape

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
)

// 刮削内容：媒体库里可以分别开关。
const (
	ItemNFO          = "nfo"           // 电影、剧集、季与每一集的 NFO
	ItemPoster       = "poster"        // 电影与剧集的海报
	ItemFanart       = "fanart"        // 背景图
	ItemLogo         = "logo"          // 透明背景的标题图（clearlogo.png）
	ItemSeasonPoster = "season_poster" // 季海报
	ItemThumb        = "thumb"         // 每一集的截图
)

// AllItems 是全部刮削内容（界面上的顺序）。
var AllItems = []string{ItemNFO, ItemPoster, ItemFanart, ItemLogo, ItemSeasonPoster, ItemThumb}

// DefaultItems 是没选过时写的内容：Logo 以外都写，和能选以前一样。
var DefaultItems = []string{ItemNFO, ItemPoster, ItemFanart, ItemSeasonPoster, ItemThumb}

// Items 是要写的刮削内容。
type Items struct {
	NFO, Poster, Fanart, Logo, SeasonPoster, Thumb bool
}

// ParseItems 读媒体库里存的刮削内容（逗号分隔）：空串是 DefaultItems，不认识的名字忽略。
func ParseItems(s string) Items {
	names := DefaultItems
	if strings.TrimSpace(s) != "" {
		names = strings.Split(s, ",")
	}
	var it Items
	for _, n := range names {
		switch strings.TrimSpace(n) {
		case ItemNFO:
			it.NFO = true
		case ItemPoster:
			it.Poster = true
		case ItemFanart:
			it.Fanart = true
		case ItemLogo:
			it.Logo = true
		case ItemSeasonPoster:
			it.SeasonPoster = true
		case ItemThumb:
			it.Thumb = true
		}
	}
	return it
}

// List 是选中的内容，按 AllItems 的顺序。
func (it Items) List() []string {
	on := map[string]bool{
		ItemNFO: it.NFO, ItemPoster: it.Poster, ItemFanart: it.Fanart, ItemLogo: it.Logo,
		ItemSeasonPoster: it.SeasonPoster, ItemThumb: it.Thumb,
	}
	out := make([]string, 0, len(AllItems))
	for _, n := range AllItems {
		if on[n] {
			out = append(out, n)
		}
	}
	return out
}

// Any 报告这种媒体库（tv 为真是剧集）至少会写一样东西：季海报与集截图只有剧集有。
func (it Items) Any(tv bool) bool {
	return it.NFO || it.Poster || it.Fanart || it.Logo || (tv && (it.SeasonPoster || it.Thumb))
}

// NormalizeItems 检查要存的刮削内容：去掉重复、按 AllItems 排好后用逗号连起来。不认识的名字报错。
func NormalizeItems(names []string) (string, error) {
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if !slices.Contains(AllItems, n) {
			return "", fmt.Errorf("不认识的刮削内容 %q", n)
		}
		seen[n] = true
	}
	var out []string
	for _, n := range AllItems {
		if seen[n] {
			out = append(out, n)
		}
	}
	return strings.Join(out, ","), nil
}

// 图片语言：海报与 Logo 优先用图上文字是这种语言的那张。
const (
	ImageLangAuto = ""     // 用 TMDB 给的那张（跟着识别语言）
	ImageLangNone = "none" // 没有文字的
)

// ImageLangs 是可选的图片语言。
var ImageLangs = []string{ImageLangAuto, "zh", "en", "ja", ImageLangNone}

// Art 是要下载的 TMDB 图片路径；空串不写。
type Art struct {
	Poster, Fanart, Logo string
}

// ChooseArt 按图片语言挑海报、背景与 Logo。imgs 是条目的图片列表（没取或取不到时为空，这时用 TMDB 给的那张、不写 Logo）：
//   - 海报：选了语言时优先用这种语言的（none 是没有文字的），没有就用 TMDB 给的那张；
//   - 背景：选了语言时优先用没有文字的，没有就用 TMDB 给的那张；
//   - Logo：依次找选的语言、识别语言（recogLang，例如 zh-CN）、英文、没有文字的，都没有时用评分最高的一张。
func ChooseArt(r tmdb.Result, imgs *tmdb.Images, lang, recogLang string) Art {
	art := Art{Poster: r.PosterPath, Fanart: r.BackdropPath}
	if imgs == nil {
		return art
	}
	if lang != ImageLangAuto {
		want := lang
		if lang == ImageLangNone {
			want = ""
		}
		if p, ok := firstIn(imgs.Posters, want); ok {
			art.Poster = p
		}
		if p, ok := firstIn(imgs.Backdrops, ""); ok {
			art.Fanart = p
		}
	}
	var prefs []string
	if lang != ImageLangAuto && lang != ImageLangNone {
		prefs = append(prefs, lang)
	}
	recog, _, _ := strings.Cut(recogLang, "-")
	prefs = append(prefs, strings.ToLower(recog), "en", "")
	for _, l := range prefs {
		if p, ok := firstIn(imgs.Logos, l); ok {
			art.Logo = p
			return art
		}
	}
	if len(imgs.Logos) > 0 {
		art.Logo = imgs.Logos[0].Path
	}
	return art
}

// firstIn 是列表里第一张（评分最高的）文字语言是 lang 的图片。
func firstIn(list []tmdb.Image, lang string) (string, bool) {
	for _, im := range list {
		if im.Lang == lang {
			return im.Path, true
		}
	}
	return "", false
}
