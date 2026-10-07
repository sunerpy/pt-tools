// Package transfer 是整理入库的文件操作：从种子的文件里挑出要整理的视频与字幕、按模板算出库里的路径，
// 再用硬链接、复制、软链接或移动放进媒体库。不访问网络，也不读写数据库。
package transfer

import (
	"fmt"
	"path"
	"regexp"
	"slices"
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

// Selection 是挑选的结果。Disc 不为空时种子是原盘（BDMV、VIDEO_TS），整个种子不整理。
type Selection struct {
	Videos    []File `json:"videos"`
	Subtitles []File `json:"subtitles"`
	Skipped   []Skip `json:"skipped"`
	Disc      string `json:"disc,omitempty"`
}

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

// IsSubtitle 报告文件名是不是字幕。
func IsSubtitle(name string) bool {
	return slices.Contains(subtitleExts, strings.ToLower(path.Ext(name)))
}

// discOf 报告路径是不是在原盘目录里，返回原盘的种类。
func discOf(rel string) string {
	for seg := range strings.SplitSeq(rel, "/") {
		switch strings.ToUpper(seg) {
		case "BDMV":
			return "BDMV"
		case "VIDEO_TS":
			return "VIDEO_TS"
		}
	}
	return ""
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

// Select 从种子的文件里挑出要整理的视频与字幕。minSize 为 0 时用 DefaultMinVideoBytes。
// 原盘目录（BDMV、VIDEO_TS）的种子不挑：整个种子跳过，Disc 写明种类。
func Select(files []File, minSize int64) Selection {
	if minSize <= 0 {
		minSize = DefaultMinVideoBytes
	}
	var sel Selection
	for _, f := range files {
		if d := discOf(f.Rel); d != "" {
			return Selection{Disc: d}
		}
	}
	for _, f := range files {
		switch {
		case IsSubtitle(f.Rel) && extraReason(f.Rel) != "":
			// 花絮、样片目录里的字幕不跟着正片走
			sel.Skipped = append(sel.Skipped, Skip{File: f, Reason: extraReason(f.Rel)})
		case IsSubtitle(f.Rel):
			sel.Subtitles = append(sel.Subtitles, f)
		case strings.EqualFold(path.Ext(f.Rel), ".iso"):
			sel.Skipped = append(sel.Skipped, Skip{File: f, Reason: "ISO 镜像不整理"})
		case !IsVideo(f.Rel):
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
