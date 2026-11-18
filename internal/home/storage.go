package home

import (
	"github.com/sunerpy/pt-tools/internal/fsutil"
)

// 存储里一块盘是什么（Disk.Kind）。
const (
	DiskDownload = "download"
	DiskLibrary  = "library"
)

// diskUsage 读一个目录所在的盘（测试里换掉）。
var diskUsage = fsutil.DiskUsage

// Disk 是首页存储里的一块盘：Label 写它是哪几个目录（下载盘 · qb、媒体库 · 电影），Path 是第一个目录。
// Total 为 0 是不知道总量（下载器只报剩余）；Low 同体检的警告线（剩不到 5% 或 20 GB）；Error 是取不到的原因。
type Disk struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Path  string `json:"path"`
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
	Low   bool   `json:"low"`
	Error string `json:"error,omitempty"`
}

// DownloadDir 是一台下载器的默认保存路径：Path 是下载器里看到的，Local 是按路径映射换成 pt-tools 里看到的
// （没有映射时同 Path）；Free 是下载器报的剩余空间（小于 0 是没报），Err 是取不到的原因。
type DownloadDir struct {
	Downloader string
	Path       string
	Local      string
	Free       int64
	Err        error
}

// LibraryDir 是一个媒体库的目录。
type LibraryDir struct {
	Name string
	Path string
}

// Storage 列出下载盘与媒体库所在的盘，同一块盘（同一个设备）只列一次。pt-tools 读得到下载目录时按本机读到的算
// （能知道总量）；读不到时用下载器报的剩余空间。
func Storage(downloads []DownloadDir, libs []LibraryDir) []Disk {
	out := []Disk{}
	seen := map[string]int{}
	add := func(kind, label, path string, u fsutil.Usage) {
		if i, ok := seen[u.Device]; ok && u.Device != "" {
			out[i].Label += "、" + label
			return
		}
		if u.Device != "" {
			seen[u.Device] = len(out)
		}
		out = append(out, Disk{Kind: kind, Label: label, Path: path, Free: u.Free, Total: u.Total, Low: fsutil.LowSpace(u.Free, u.Total)})
	}
	for _, d := range downloads {
		label := "下载盘 · " + d.Downloader
		if d.Err != nil {
			out = append(out, Disk{Kind: DiskDownload, Label: label, Path: d.Path, Error: clean(d.Err.Error())})
			continue
		}
		if u, err := diskUsage(d.Local); err == nil {
			add(DiskDownload, label, d.Path, u)
			continue
		}
		if d.Free < 0 {
			out = append(out, Disk{Kind: DiskDownload, Label: label, Path: d.Path, Error: "下载器没报剩余空间，pt-tools 里也读不到这个目录"})
			continue
		}
		add(DiskDownload, label, d.Path, fsutil.Usage{Free: uint64(d.Free)})
	}
	for _, l := range libs {
		label := "媒体库 · " + l.Name
		u, err := diskUsage(l.Path)
		if err != nil {
			out = append(out, Disk{Kind: DiskLibrary, Label: label, Path: l.Path, Error: clean(err.Error())})
			continue
		}
		add(DiskLibrary, label, l.Path, u)
	}
	return out
}
