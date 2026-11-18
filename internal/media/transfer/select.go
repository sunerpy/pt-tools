// Package transfer 是整理入库的文件操作：从种子的文件里挑出要整理的视频与字幕、按模板算出库里的路径，
// 再用硬链接、复制、软链接或移动放进媒体库。不访问网络，也不读写数据库。
package transfer

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// File 是种子里的一个文件。Rel 是下载器给的、相对保存目录的路径（用 / 分隔），Path 是 pt-tools 里的绝对路径。
type File struct {
	Rel  string `json:"rel"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Skip 是没有选上的文件与原因。
type Skip struct {
	File   File   `json:"file"`
	Reason string `json:"reason"`
}

// Selection 是挑选的结果。种子里有原盘（BDMV、VIDEO_TS 目录）时只挑原盘：Discs 列出每一张，Disc 是第一张的种类，
// 不再挑散的视频与字幕。
type Selection struct {
	Videos    []File `json:"videos"`
	Subtitles []File `json:"subtitles"`
	Skipped   []Skip `json:"skipped"`
	Disc      string `json:"disc,omitempty"`
	Discs     []Disc `json:"discs,omitempty"`
}

// Disc 是种子里的一张原盘：蓝光的 BDMV 或 DVD 的 VIDEO_TS 目录，以及它们的上一级（原盘根目录）。
type Disc struct {
	// Kind 是 BDMV 或 VIDEO_TS（同一个根目录里两种都有时是 BDMV）
	Kind string `json:"kind"`
	// Root 是原盘根目录相对保存目录的路径（用 / 分隔）；BDMV 直接在保存目录里时为空
	Root string `json:"root"`
	// Path 是原盘根目录在 pt-tools 里的路径
	Path string `json:"path"`
	// Files 是根目录下原盘结构目录（BDMV、CERTIFICATE 等）里的文件，按种子里的顺序
	Files []DiscFile `json:"files"`
	// Main 是最大的视频流（BDMV/STREAM 里的 .m2ts，或 VIDEO_TS 里的 VTS_*.VOB）在 Files 里的下标；没有时为 -1
	Main int `json:"main"`
	// Size 是 Files 的大小合计
	Size int64 `json:"size"`
}

// DiscFile 是原盘里的一个文件：In 是相对原盘根目录的路径（用 / 分隔，例如 BDMV/STREAM/00001.m2ts）。
type DiscFile struct {
	File
	In string `json:"in"`
}

// discDirs 是原盘根目录下属于原盘的目录（比较前转大写）：蓝光的 BDMV、CERTIFICATE、AACS、BDSVM，DVD 的 VIDEO_TS、AUDIO_TS。
var discDirs = []string{"BDMV", "CERTIFICATE", "AACS", "BDSVM", "VIDEO_TS", "AUDIO_TS"}

// DefaultMinVideoBytes 是视频文件的默认最小体积：更小的当作样片或花絮。
const DefaultMinVideoBytes = 50 << 20

var (
	videoExts = []string{
		".mkv", ".mp4", ".m4v", ".avi", ".ts", ".m2ts", ".mts", ".mov", ".wmv", ".flv",
		".rmvb", ".rm", ".webm", ".mpg", ".mpeg",
	}
	subtitleExts = []string{".srt", ".ass", ".ssa", ".sub", ".idx", ".sup", ".vtt"}

	// extraDirs 是放花絮、样片、特典的目录名（比较前转小写、去掉空格与 ._-）。
	// 不含 Specials：那里是第 0 季的正片。
	extraDirs = []string{
		"sample", "samples", "trailer", "trailers", "featurette", "featurettes", "extra", "extras",
		"behindthescenes", "deletedscenes", "interview", "interviews", "bonus", "bonusfeatures",
		"sp", "sps", "cd", "cds", "scan", "scans", "menu", "menus", "pv", "cm", "nc", "ncop", "nced",
		"花絮", "特典", "预告", "预告片", "幕后", "映像特典",
	}
	extraTokenRe = regexp.MustCompile(`(?i)(?:^|[\s._\-\[(])(?:ncop|nced)\d*(?:$|[\s._\-\])])`)
	// 结尾的 sample、trailer、featurette，或开头的 sample（片名以 Trailer 开头的不算，如 Trailer Park Boys）
	sampleRe  = regexp.MustCompile(`(?i)(?:^|[\s._\-])(?:sample|trailer|featurette)$|^sample(?:$|[\s._\-])`)
	dirNormRe = regexp.MustCompile(`[\s._\-]+`)
)

// IsVideo 报告文件名是不是视频。
func IsVideo(name string) bool {
	return slices.Contains(videoExts, strings.ToLower(path.Ext(name)))
}

// IsISO 报告文件名是不是 ISO 镜像（电影当作一个视频文件整理，剧集里的不整理）。
func IsISO(name string) bool {
	return strings.EqualFold(path.Ext(name), ".iso")
}

// IsLibraryVideo 报告媒体库里的这个文件是不是视频：普通的视频文件、ISO 镜像与 DVD 原盘的 VOB（蓝光原盘的 .m2ts 本来就是视频）。
// 删除库里的文件时用它看目录里还有没有视频。
func IsLibraryVideo(name string) bool {
	return IsVideo(name) || IsISO(name) || strings.EqualFold(path.Ext(name), ".vob")
}

// IsSubtitle 报告文件名是不是字幕。
func IsSubtitle(name string) bool {
	return slices.Contains(subtitleExts, strings.ToLower(path.Ext(name)))
}

// discOf 报告路径是不是在原盘目录里：返回原盘的种类与原盘根目录（第一个 BDMV、VIDEO_TS 那一级的上一级，用 / 分隔）。
func discOf(rel string) (kind, root string, ok bool) {
	segs := strings.Split(rel, "/")
	for i, seg := range segs[:len(segs)-1] {
		switch up := strings.ToUpper(seg); up {
		case "BDMV", "VIDEO_TS":
			return up, strings.Join(segs[:i], "/"), true
		}
	}
	return "", "", false
}

// mainStream 报告原盘里的这个文件（相对原盘根目录）能不能当主文件：BDMV/STREAM 下的 .m2ts、VIDEO_TS 下的 VTS_*.VOB。
func mainStream(in string) bool {
	up := strings.ToUpper(in)
	switch {
	case strings.HasPrefix(up, "BDMV/STREAM/") && strings.Count(up, "/") == 2:
		return strings.HasSuffix(up, ".M2TS")
	case strings.HasPrefix(up, "VIDEO_TS/VTS_") && strings.Count(up, "/") == 1:
		return strings.HasSuffix(up, ".VOB")
	}
	return false
}

// rootPath 是文件 f 所在原盘根目录 root（相对保存目录）在 pt-tools 里的路径：f.Path 去掉相对根目录的那一段。
func rootPath(f File, root string) string {
	in := f.Rel
	if root != "" {
		in = strings.TrimPrefix(f.Rel, root+"/")
	}
	// 从 f.Path 的末尾去掉 in 那么多段：f.Path 的分隔符可能是 / 也可能是 \（Windows），不按字面去掉 in
	p := f.Path
	for range strings.Count(in, "/") + 1 {
		switch i := strings.LastIndexAny(p, `/\`); {
		case i < 0:
			return p
		case i == 0:
			return p[:1] // 原盘根目录就是文件系统的根
		default:
			p = p[:i]
		}
	}
	return p
}

// selectDiscs 按原盘根目录挑出每一张原盘；不属于任何一张的文件进 skipped。没有原盘时返回 nil。
func selectDiscs(files []File) ([]Disc, []Skip) {
	byRoot := map[string]*Disc{}
	for _, f := range files {
		kind, root, ok := discOf(f.Rel)
		if !ok {
			continue
		}
		d := byRoot[root]
		if d == nil {
			d = &Disc{Kind: kind, Root: root, Path: rootPath(f, root), Main: -1}
			byRoot[root] = d
		}
		if kind == "BDMV" {
			d.Kind = kind
		}
	}
	if len(byRoot) == 0 {
		return nil, nil
	}
	roots := make([]string, 0, len(byRoot))
	for r := range byRoot {
		roots = append(roots, r)
	}
	// 一个文件归到最近的（最长的）那个根目录
	sort.Slice(roots, func(i, j int) bool { return len(roots[i]) > len(roots[j]) })
	var skipped []Skip
	for _, f := range files {
		var d *Disc
		var in string
		for _, r := range roots {
			switch {
			case r == "":
				d, in = byRoot[r], f.Rel
			case strings.HasPrefix(f.Rel, r+"/"):
				d, in = byRoot[r], strings.TrimPrefix(f.Rel, r+"/")
			}
			if d != nil {
				break
			}
		}
		first, _, _ := strings.Cut(in, "/")
		if d == nil || !strings.Contains(in, "/") || !slices.Contains(discDirs, strings.ToUpper(first)) {
			skipped = append(skipped, Skip{File: f, Reason: "原盘以外的文件不整理"})
			continue
		}
		d.Files = append(d.Files, DiscFile{File: f, In: in})
		d.Size += f.Size
		if mainStream(in) && (d.Main < 0 || f.Size > d.Files[d.Main].Size) {
			d.Main = len(d.Files) - 1
		}
	}
	sort.Strings(roots)
	out := make([]Disc, 0, len(roots))
	for _, r := range roots {
		out = append(out, *byRoot[r])
	}
	return out, skipped
}

// extraReason 报告文件是不是花絮、样片这类附带内容，返回原因；正片返回空串。
func extraReason(rel string) string {
	segs := strings.Split(rel, "/")
	for _, d := range segs[:len(segs)-1] {
		if slices.Contains(extraDirs, strings.ToLower(dirNormRe.ReplaceAllString(d, ""))) {
			return fmt.Sprintf("在「%s」目录里（花絮、样片或特典）", d)
		}
	}
	base := segs[len(segs)-1]
	stem := strings.TrimSuffix(base, path.Ext(base))
	if sampleRe.MatchString(stem) || extraTokenRe.MatchString(stem) {
		return "文件名表明是样片、预告或片头片尾"
	}
	return ""
}

// Select 从种子的文件里挑出要整理的视频与字幕。minSize 为 0 时用 DefaultMinVideoBytes。ISO 镜像当作视频挑。
// 有原盘目录（BDMV、VIDEO_TS）的种子只挑原盘（见 Disc），Disc 写明第一张的种类。
func Select(files []File, minSize int64) Selection {
	if minSize <= 0 {
		minSize = DefaultMinVideoBytes
	}
	var sel Selection
	if discs, skipped := selectDiscs(files); len(discs) > 0 {
		return Selection{Disc: discs[0].Kind, Discs: discs, Skipped: skipped}
	}
	for _, f := range files {
		switch {
		case IsSubtitle(f.Rel) && extraReason(f.Rel) != "":
			// 花絮、样片目录里的字幕不跟着正片走
			sel.Skipped = append(sel.Skipped, Skip{File: f, Reason: extraReason(f.Rel)})
		case IsSubtitle(f.Rel):
			sel.Subtitles = append(sel.Subtitles, f)
		case !IsVideo(f.Rel) && !IsISO(f.Rel):
			// 图片、NFO、种子说明一类，不整理也不列出
		case extraReason(f.Rel) != "":
			sel.Skipped = append(sel.Skipped, Skip{File: f, Reason: extraReason(f.Rel)})
		case f.Size < minSize:
			sel.Skipped = append(sel.Skipped, Skip{File: f, Reason: fmt.Sprintf("小于 %d MB，当作样片", minSize>>20)})
		default:
			sel.Videos = append(sel.Videos, f)
		}
	}
	return sel
}
