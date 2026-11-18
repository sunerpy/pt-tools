package transfer

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode/utf8"
)

// 默认命名模板（Emby、Jellyfin、Plex 都认）：每部电影、每部剧集一个目录，剧集再按季分目录。模板不写扩展名。
const (
	DefaultMovieTemplate = `{{.Title}}{{if .Year}} ({{.Year}}){{end}}/{{.Title}}{{if .Year}} ({{.Year}}){{end}}{{if .Quality}} - {{.Quality}}{{end}}`
	DefaultTVTemplate    = `{{.Title}}{{if .Year}} ({{.Year}}){{end}}/Season {{.Season}}/{{.Title}} - {{.SeasonEpisode}}{{if .EpisodeTitle}} - {{.EpisodeTitle}}{{end}}`
)

// Vars 是模板里能用的变量。字符串在渲染前已经去掉了路径里不能用的字符。
type Vars struct {
	Title         string // 标题（TMDB，按设置的语言）
	OriginalTitle string // 原名
	Year          int    // 年份；剧集是首播年份
	TMDBID        int
	IMDbID        string
	Season        int    // 季（电影为 0）
	Episode       int    // 集（电影为 0）
	EpisodeEnd    int    // 一个文件里有多集时的最后一集
	SeasonEpisode string // S01E02，多集时 S01E02-E03
	EpisodeTitle  string // 这一集的标题（TMDB 有时才有）
	Resolution    string // 2160p 一类
	Source        string // WEB-DL、BluRay 一类，Remux 时带上 Remux
	VideoCodec    string
	HDR           string // 多个时用空格隔开
	Audio         string // 多个时用空格隔开
	Group         string // 制作组
	Edition       string // 版本说明，多个时用空格隔开
	Quality       string // 分辨率、来源、HDR、编码连在一起，如「2160p WEB-DL HDR10 H.265」
}

// maxRendered 是模板渲染结果的长度上限（字节）；maxSegment 是一级目录或文件名的上限（留出扩展名与字幕后缀）。
const (
	maxRendered = 2048
	maxSegment  = 200
)

var (
	// ErrTemplate 表示模板写错了或渲染出的路径不能用（错误信息写明原因）。
	ErrTemplate = errors.New("命名模板不能用")
	spacesRe    = regexp.MustCompile(`\s{2,}`)
)

// CleanName 把一段文字变成能放进路径的名字：去掉 \ / * ? " < > | 与控制字符，冒号换成「 - 」，合并空格，去掉首尾的空格与点。
func CleanName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteRune(' ')
		case r == ':' || r == '：':
			b.WriteString(" - ")
		case strings.ContainsRune(`\/*?"<>|`, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Trim(spacesRe.ReplaceAllString(b.String(), " "), " .")
}

func (v Vars) cleaned() Vars {
	for _, p := range []*string{
		&v.Title, &v.OriginalTitle, &v.IMDbID, &v.SeasonEpisode, &v.EpisodeTitle, &v.Resolution, &v.Source,
		&v.VideoCodec, &v.HDR, &v.Audio, &v.Group, &v.Edition, &v.Quality,
	} {
		*p = CleanName(*p)
	}
	return v
}

// SeasonEpisode 写出 S01E02，多集时 S01E02-E03；没有集数时只写季。
func SeasonEpisode(season, episode, end int) string {
	switch {
	case episode <= 0:
		return fmt.Sprintf("S%02d", season)
	case end > episode:
		return fmt.Sprintf("S%02dE%02d-E%02d", season, episode, end)
	default:
		return fmt.Sprintf("S%02dE%02d", season, episode)
	}
}

var funcs = template.FuncMap{
	// pad 把数字补零到 width 位：{{pad .Season 2}} 写出 01
	"pad": func(n, width int) string {
		if width < 1 || width > 6 {
			width = 2
		}
		return fmt.Sprintf("%0*d", width, n)
	},
}

// limitWriter 写满上限后报错，模板里写了循环也不会无限输出。
type limitWriter struct {
	b strings.Builder
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w.b.Len()+len(p) > maxRendered {
		return 0, fmt.Errorf("%w: 渲染结果超过 %d 字节", ErrTemplate, maxRendered)
	}
	return w.b.Write(p)
}

// Render 按模板算出库里的相对路径（不含扩展名，用 / 分隔）。tpl 为空时报错，由调用方先换成默认模板。
// 渲染出的每一级都去掉首尾的空格与点，空的一级（包括 . 与 ..）跳过，每级最多 200 字节。
func Render(tpl string, v Vars) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		return "", fmt.Errorf("%w: 模板为空", ErrTemplate)
	}
	t, err := template.New("name").Funcs(funcs).Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrTemplate, err)
	}
	var w limitWriter
	if err := t.Execute(&w, v.cleaned()); err != nil {
		if errors.Is(err, ErrTemplate) {
			return "", err
		}
		return "", fmt.Errorf("%w: %w", ErrTemplate, err)
	}
	var segs []string
	for seg := range strings.SplitSeq(strings.ReplaceAll(w.b.String(), `\`, "/"), "/") {
		seg = strings.Trim(spacesRe.ReplaceAllString(seg, " "), " .")
		switch {
		case seg == "":
			continue
		case len(seg) > maxSegment:
			return "", fmt.Errorf("%w: 有一级名字超过 %d 字节", ErrTemplate, maxSegment)
		case !utf8.ValidString(seg):
			return "", fmt.Errorf("%w: 渲染结果不是有效的 UTF-8", ErrTemplate)
		}
		segs = append(segs, seg)
	}
	if len(segs) == 0 {
		return "", fmt.Errorf("%w: 渲染结果为空", ErrTemplate)
	}
	return strings.Join(segs, "/"), nil
}

// SampleMovie 与 SampleEpisode 是检查模板与界面预览用的示例。
var (
	SampleMovie = Vars{
		Title: "奥本海默", OriginalTitle: "Oppenheimer", Year: 2023, TMDBID: 872585, IMDbID: "tt15398776",
		Resolution: "2160p", Source: "BluRay", VideoCodec: "H.265", HDR: "HDR10", Audio: "DTS-HD MA", Group: "FRDS",
		Quality: "2160p BluRay HDR10 H.265",
	}
	SampleEpisode = Vars{
		Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, TMDBID: 100088, IMDbID: "tt3581920",
		Season: 1, Episode: 2, SeasonEpisode: "S01E02", EpisodeTitle: "感染",
		Resolution: "2160p", Source: "WEB-DL", VideoCodec: "H.265", HDR: "DV", Group: "HHWEB",
		Quality: "2160p WEB-DL DV H.265",
	}
)

// Check 用示例渲染一次模板，报告能不能用；返回示例渲染的结果给界面看。
func Check(tpl, kind string) (string, error) {
	v := SampleMovie
	if kind == "tv" {
		v = SampleEpisode
	}
	return Render(tpl, v)
}
