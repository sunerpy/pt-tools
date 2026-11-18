package reseed

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/utils"
)

// FileEntry 是种子里的一个文件：相对保存路径的路径（用 / 分隔）与大小。
type FileEntry struct {
	Path string
	Size int64
}

type torrentInfo struct {
	Name   string `bencode:"name"`
	Length int64  `bencode:"length"`
	Files  []struct {
		Length int64    `bencode:"length"`
		Path   []string `bencode:"path"`
	} `bencode:"files"`
}

// TorrentFiles 列出 .torrent 文件里的文件：单文件种子是 name；多文件种子是 name/path...（与下载器的文件列表同一写法）。
func TorrentFiles(data []byte) ([]FileEntry, error) {
	var meta struct {
		Info torrentInfo `bencode:"info"`
	}
	if err := bencode.NewDecoder(bytes.NewReader(data)).Decode(&meta); err != nil {
		return nil, fmt.Errorf("解析种子文件失败: %w", err)
	}
	info := meta.Info
	if info.Name == "" {
		return nil, errors.New("种子文件缺少 name")
	}
	if len(info.Files) == 0 {
		return []FileEntry{{Path: info.Name, Size: info.Length}}, nil
	}
	out := make([]FileEntry, 0, len(info.Files))
	for _, f := range info.Files {
		parts := append([]string{info.Name}, f.Path...)
		out = append(out, FileEntry{Path: path.Join(parts...), Size: f.Length})
	}
	return out, nil
}

// DownloaderFiles 把下载器的文件列表换成 FileEntry（分隔符统一成 /）。
func DownloaderFiles(files []downloader.TorrentFile) []FileEntry {
	out := make([]FileEntry, 0, len(files))
	for _, f := range files {
		out = append(out, FileEntry{Path: path.Clean(strings.ReplaceAll(f.Name, "\\", "/")), Size: f.Size})
	}
	return out
}

// SameFiles 报告两份文件列表是否完全一样（路径与大小逐个相同，顺序无关）。不一样时第二个返回值写明第一处差别。
// 辅种只有在文件完全一样时才加：同一份数据才能被新种子校验到 100%。
func SameFiles(a, b []FileEntry) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("文件数不同（%d 与 %d）", len(a), len(b))
	}
	x, y := sorted(a), sorted(b)
	for i := range x {
		if x[i].Path != y[i].Path {
			return false, fmt.Sprintf("文件不同（%s 与 %s）", x[i].Path, y[i].Path)
		}
		if x[i].Size != y[i].Size {
			return false, fmt.Sprintf("%s 的大小不同（%s 与 %s）", x[i].Path, readableSize(x[i].Size), readableSize(y[i].Size))
		}
	}
	return true, ""
}

// readableSize 写成 GiB/MiB 这样的大小，后面带上字节数，差一点的也看得出来。
func readableSize(n int64) string {
	return fmt.Sprintf("%s，%d 字节", utils.FormatBytes(n), n)
}

func sorted(in []FileEntry) []FileEntry {
	out := append([]FileEntry(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
