package subscribe

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 下载时按条目建子目录（媒体中心 M9b）：订阅设置里打开以后，订阅推送的种子放进保存路径（订阅没填时是下载器的默认保存路径）下
// 按模板起名的一级子目录，例如 /downloads/最后生还者 (2023)；按条目搜索的推送弹窗里也可以选。这时要下载器照这个目录放
// （qBittorrent 另外传 autoTMM=false，否则默认用自动管理时会忽略保存路径）。整理入库按下载器给的内容路径读，不受影响。

// DefaultSubdirTemplate 是子目录名的默认模板。
const DefaultSubdirTemplate = "{title} ({year})"

// maxSubdirTemplate 是模板的长度上限（字节）；maxSubdirName 是算出的子目录名的上限（字节）。
const (
	maxSubdirTemplate = 255
	maxSubdirName     = 200
)

// subdirVarRe 是模板里的变量：{title}、{original_title}、{year}、{season}、{tmdb_id}。
var subdirVarRe = regexp.MustCompile(`\{[^{}]*\}`)

var subdirVars = map[string]bool{"{title}": true, "{original_title}": true, "{year}": true, "{season}": true, "{tmdb_id}": true}

// cleanSubdirTemplate 检查子目录名的模板：空的按默认模板；只认上面那几个变量，要至少有一个（不然所有条目都放进同一个目录）。
func cleanSubdirTemplate(tpl string) (string, error) {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		return DefaultSubdirTemplate, nil
	}
	if len(tpl) > maxSubdirTemplate {
		return "", fmt.Errorf("%w: 子目录名的模板太长", ErrInvalid)
	}
	found := subdirVarRe.FindAllString(tpl, -1)
	for _, v := range found {
		if !subdirVars[v] {
			return "", fmt.Errorf("%w: 子目录名的模板里有不认识的变量 %s（可以用 {title}、{original_title}、{year}、{season}、{tmdb_id}）", ErrInvalid, v)
		}
	}
	if len(found) == 0 {
		return "", fmt.Errorf("%w: 子目录名的模板里要有 {title} 这类变量，不然所有条目都放进同一个目录", ErrInvalid)
	}
	return tpl, nil
}

// subdirItem 是起子目录名用的条目信息。
type subdirItem struct {
	Title, OriginalTitle string
	Year, Season, TMDBID int
}

// emptyParensRe 是年份不知道时留下的空括号。
var emptyParensRe = regexp.MustCompile(`\(\s*\)|（\s*）|\[\s*\]`)

// subdirName 按模板算子目录名：变量换成条目的信息（不知道的是空的，空括号去掉），去掉路径里不能用的字符（不会有第二级目录），
// 最多 200 字节。算出是空的时为空串（不建子目录）。
func subdirName(tpl string, it subdirItem) string {
	num := func(n int) string {
		if n <= 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	vals := map[string]string{
		"{title}": it.Title, "{original_title}": it.OriginalTitle, "{year}": num(it.Year), "{season}": num(it.Season), "{tmdb_id}": num(it.TMDBID),
	}
	name := subdirVarRe.ReplaceAllStringFunc(tpl, func(v string) string { return vals[v] })
	name = transfer.CleanName(emptyParensRe.ReplaceAllString(name, ""))
	for len(name) > maxSubdirName {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return strings.TrimRight(name, " .")
}

// joinSavePath 把子目录接到下载器看到的保存路径后面：保存路径像 Windows 路径（只有 \）时用 \，否则用 /。
func joinSavePath(base, sub string) string {
	sep := "/"
	if strings.Contains(base, `\`) && !strings.Contains(base, "/") {
		sep = `\`
	}
	return strings.TrimRight(base, `/\`) + sep + sub
}

// errNoDefaultPath 是读不到下载器的默认保存路径。
var errNoDefaultPath = errors.New("下载器没有给出默认保存路径")

// defaultSavePath 读下载器的默认保存路径（qBittorrent 设置里的默认保存路径、Transmission 的 download-dir）。
func (s *Service) defaultSavePath(ctx context.Context, dlID uint) (string, error) {
	if s.cfg.Downloaders == nil {
		return "", errors.New("下载器服务没有启动")
	}
	dl, _, err := s.cfg.Downloaders.Get(ctx, dlID)
	if err != nil {
		return "", err
	}
	paths, err := dl.GetClientPaths()
	if err != nil {
		return "", err
	}
	if len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
		return "", errNoDefaultPath
	}
	return strings.TrimSpace(paths[0]), nil
}

// itemSubdir 是一个条目的子目录名（按订阅设置里的模板）。剧集的年份用首播年份（订阅记的是这一季的），取不到时用 year。
func (s *Service) itemSubdir(ctx context.Context, set Settings, kind string, tmdbID, season int, title, original string, year int) string {
	if kind == models.MediaKindTV {
		if c, err := s.cfg.Recognizer.TMDB(ctx); err == nil {
			if d, derr := c.Details(ctx, kind, tmdbID); derr == nil && d.Year > 0 {
				year = d.Year
			}
		}
	} else {
		season = 0
	}
	return subdirName(set.SubdirTemplate, subdirItem{Title: title, OriginalTitle: original, Year: year, Season: season, TMDBID: tmdbID})
}

// savePathFor 是订阅这次推送的保存路径，以及要不要让下载器照这个目录放：没开「下载时按条目建子目录」时是订阅的保存路径（不强制）；
// 开了时是订阅的保存路径（没有时是下载器的默认保存路径）加上子目录。读不到下载器的默认保存路径、算不出子目录名时不加子目录，
// 写进运行记录。
func (s *Service) savePathFor(ctx context.Context, sub *models.MediaSubscription, set Settings, dlID uint) (string, bool) {
	base := strings.TrimSpace(sub.SavePath)
	if !set.SubdirEnabled {
		return base, false
	}
	name := s.itemSubdir(ctx, set, sub.MediaType, sub.TMDBID, sub.Season, sub.Title, sub.OriginalTitle, sub.Year)
	if name == "" {
		step(ctx, stepInfo, "按模板「%s」算出的子目录名是空的，这次不建子目录", set.SubdirTemplate)
		return base, false
	}
	if base == "" {
		p, err := s.defaultSavePath(ctx, dlID)
		if err != nil {
			step(ctx, stepInfo, "读不到下载器的默认保存路径（%v），这次不建子目录「%s」", err, name)
			return "", false
		}
		base = p
	}
	return joinSavePath(base, name), true
}

// ItemDownloadDir 是按条目推送时「按条目建子目录」的保存路径：base（空的是下载器的默认保存路径）加上按订阅设置里的模板起的子目录。
// 读不到下载器的默认保存路径、算不出子目录名时返回错误（调用方照原来的保存路径推送并写明）。
func (s *Service) ItemDownloadDir(ctx context.Context, kind string, tmdbID, season int, dlID uint, base string) (string, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return "", fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return "", err
	}
	d, err := c.Details(ctx, kind, tmdbID)
	if err != nil {
		return "", fmt.Errorf("取 TMDB 条目失败: %w", err)
	}
	name := s.itemSubdir(ctx, set, kind, tmdbID, season, d.Title, d.OriginalTitle, d.Year)
	if name == "" {
		return "", fmt.Errorf("按模板「%s」算出的子目录名是空的", set.SubdirTemplate)
	}
	if base = strings.TrimSpace(base); base == "" {
		if base, err = s.defaultSavePath(ctx, dlID); err != nil {
			return "", fmt.Errorf("读不到下载器的默认保存路径: %w", err)
		}
	}
	return joinSavePath(base, name), nil
}
