package scrape

import (
	"fmt"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/metadata"
)

// 刮削内容：媒体库里可以分别开关。
const (
	ItemNFO          = "nfo"           // 电影、剧集、季与每一集的 NFO
	ItemPoster       = "poster"        // 电影与剧集的海报
	ItemFanart       = "fanart"        // 背景图
	ItemLogo         = "logo"          // 透明背景的标题图（clearlogo.png）
	ItemClearart     = "clearart"      // 透明背景的人物与标题图（clearart.png，只有 Fanart.tv 有）
	ItemBanner       = "banner"        // 横幅（banner.jpg，只有 Fanart.tv 有）
	ItemLandscape    = "landscape"     // 横版海报（landscape.jpg，只有 Fanart.tv 有）
	ItemDisc         = "discart"       // 碟片图（discart.png，只有电影有，只有 Fanart.tv 有）
	ItemSeasonPoster = "season_poster" // 季海报
	ItemThumb        = "thumb"         // 每一集的截图
)

// AllItems 是全部刮削内容（界面上的顺序）。
var AllItems = []string{
	ItemNFO, ItemPoster, ItemFanart, ItemLogo, ItemClearart, ItemBanner, ItemLandscape, ItemDisc, ItemSeasonPoster, ItemThumb,
}

// DefaultItems 是没选过时写的内容：Logo 与只有 Fanart.tv 有的那几种以外都写，和能选以前一样。
var DefaultItems = []string{ItemNFO, ItemPoster, ItemFanart, ItemSeasonPoster, ItemThumb}

// Items 是要写的刮削内容。
type Items struct {
	NFO, Poster, Fanart, Logo, Clearart, Banner, Landscape, Disc, SeasonPoster, Thumb bool
}

// ParseItems 读媒体库里存的刮削内容（逗号分隔）：空串是 DefaultItems，不认识的名字忽略。
func ParseItems(s string) Items {
	names := DefaultItems
	if strings.TrimSpace(s) != "" {
		names = strings.Split(s, ",")
	}
	var it Items
	for _, n := range names {
		if p := it.field(strings.TrimSpace(n)); p != nil {
			*p = true
		}
	}
	return it
}

// field 是这一项的开关；不认识的名字返回 nil。
func (it *Items) field(name string) *bool {
	switch name {
	case ItemNFO:
		return &it.NFO
	case ItemPoster:
		return &it.Poster
	case ItemFanart:
		return &it.Fanart
	case ItemLogo:
		return &it.Logo
	case ItemClearart:
		return &it.Clearart
	case ItemBanner:
		return &it.Banner
	case ItemLandscape:
		return &it.Landscape
	case ItemDisc:
		return &it.Disc
	case ItemSeasonPoster:
		return &it.SeasonPoster
	case ItemThumb:
		return &it.Thumb
	}
	return nil
}

// List 是选中的内容，按 AllItems 的顺序。
func (it Items) List() []string {
	out := make([]string, 0, len(AllItems))
	for _, n := range AllItems {
		if *it.field(n) {
			out = append(out, n)
		}
	}
	return out
}

// Any 报告这种媒体库（tv 为真是剧集）至少会写一样东西：季海报与集截图只有剧集有，碟片只有电影有。
func (it Items) Any(tv bool) bool {
	return it.NFO || it.Poster || it.Fanart || it.Logo || it.Clearart || it.Banner || it.Landscape ||
		(!tv && it.Disc) || (tv && (it.SeasonPoster || it.Thumb))
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
	ImageLangAuto = metadata.LangAuto // 用来源给的那张（跟着识别语言）
	ImageLangNone = metadata.LangNone // 没有文字的
)

// ImageLangs 是可选的图片语言。
var ImageLangs = []string{ImageLangAuto, "zh", "en", "ja", ImageLangNone}

// 旧版本的「简介与评分」（媒体库的 ScrapeSource）：现在改成了每一项分别选（见 metadata.Sources），旧的设置照样认。
const (
	SourceTMDB   = ""                    // 用 TMDB 的
	SourceDouban = metadata.LegacyDouban // 简介与评分都用豆瓣的，在豆瓣上找不到时用 TMDB 的
)

// Sources 是旧版本「简介与评分」可选的值。
var Sources = []string{SourceTMDB, SourceDouban}
