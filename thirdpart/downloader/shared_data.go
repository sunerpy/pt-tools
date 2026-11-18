package downloader

import "strings"

// DataSharing 判断下载器里的种子是不是共用同一份数据（辅种与原种子就是这样：文件完全一样，放在同一个位置）。
// 自动删种在删数据之前要先问它：有别的种子用着时只删种子、保留数据，否则另一个种子就坏了。
//
// 数据路径（内容路径，没有时用保存路径 + 名称）不同的不算共用。路径相同时：
//   - 两个都有自己的文件夹或都是单文件：算共用。
//   - 有一个是 qBittorrent 不建子文件夹的多文件种子（内容路径就是保存路径）：同一目录下的种子不一定是同一份数据，
//     有文件重叠才算共用；读不到文件列表时当作共用（宁可留下数据）。
//
// 只认数据路径相同的种子：一个种子的文件放在另一个种子的目录里（例如单集种子放在季包的目录里）的情况认不出来。
type DataSharing struct {
	all   []Torrent
	files func(Torrent) ([]TorrentFile, error)
	cache map[string]map[string]bool // TorrentKey → 文件的完整路径；nil 表示读不到
}

// NewDataSharing 用下载器里的全部种子建一个 DataSharing。files 读一个种子的文件列表（见 FilesOf），
// 为 nil 时平铺的多文件种子与同目录同大小的种子一律当作共用。
func NewDataSharing(all []Torrent, files func(Torrent) ([]TorrentFile, error)) *DataSharing {
	return &DataSharing{all: all, files: files, cache: map[string]map[string]bool{}}
}

// FilesOf 按下载器读种子的文件列表。
func FilesOf(dl Downloader) func(Torrent) ([]TorrentFile, error) {
	return func(t Torrent) ([]TorrentFile, error) { return dl.GetTorrentFiles(t.ID) }
}

// Shares 报告除了 deleting 里的（键见 TorrentKey：同一批一起删的，或者已经删掉的），还有没有别的种子用着 t 的数据。
func (s *DataSharing) Shares(t Torrent, deleting map[string]bool) bool {
	for _, o := range s.Sharers(t) {
		if !deleting[TorrentKey(o)] {
			return true
		}
	}
	return false
}

// Sharers 返回和 t 共用数据的种子（不含 t 自己）。
func (s *DataSharing) Sharers(t Torrent) []Torrent {
	p := DataPath(t)
	if p == "" {
		return nil
	}
	self := TorrentKey(t)
	var out []Torrent
	for _, o := range s.all {
		if TorrentKey(o) == self || DataPath(o) != p {
			continue
		}
		if s.sameData(t, o) {
			out = append(out, o)
		}
	}
	return out
}

// sameData 判断数据路径相同的两个种子是不是同一份数据。
func (s *DataSharing) sameData(a, b Torrent) bool {
	if !flat(a) && !flat(b) {
		return true
	}
	fa, fb := s.fileSet(a), s.fileSet(b)
	if fa == nil || fb == nil {
		return true // 读不到文件列表：当作共用，宁可留下数据
	}
	for f := range fa {
		if fb[f] {
			return true
		}
	}
	return false
}

// fileSet 是种子文件的完整路径（规范化后），读不到时返回 nil。
func (s *DataSharing) fileSet(t Torrent) map[string]bool {
	k := TorrentKey(t)
	if set, ok := s.cache[k]; ok {
		return set
	}
	var set map[string]bool
	if s.files != nil {
		if fs, err := s.files(t); err == nil && len(fs) > 0 {
			set = make(map[string]bool, len(fs))
			for _, f := range fs {
				set[normPath(strings.TrimRight(t.SavePath, "/\\")+"/"+f.Name)] = true
			}
		}
	}
	s.cache[k] = set
	return set
}

// flat 报告是不是 qBittorrent 不建子文件夹的多文件种子：内容路径就是保存路径。
func flat(t Torrent) bool {
	return strings.TrimSpace(t.ContentPath) != "" && strings.TrimSpace(t.SavePath) != "" &&
		normPath(t.ContentPath) == normPath(t.SavePath)
}

// TorrentKey 是认种子用的键：小写 info hash，没有 hash 时用 ID。
func TorrentKey(t Torrent) string {
	if h := strings.ToLower(strings.TrimSpace(t.InfoHash)); h != "" {
		return h
	}
	return "id:" + t.ID
}

// DataPath 是种子数据的路径（见 normPath）。没有内容路径时用保存路径 + 名称。
func DataPath(t Torrent) string {
	p := t.ContentPath
	if strings.TrimSpace(p) == "" {
		if strings.TrimSpace(t.SavePath) == "" || t.Name == "" {
			return ""
		}
		p = strings.TrimRight(t.SavePath, "/\\") + "/" + t.Name
	}
	return normPath(p)
}

// normPath 统一成 / 分隔、去掉末尾的 /；Windows 路径不区分大小写。
func normPath(p string) string {
	win := strings.Contains(p, "\\") || (len(p) >= 2 && p[1] == ':')
	p = strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
	if win {
		p = strings.ToLower(p)
	}
	return p
}

// KeepSharedData 把要删的种子分成两组：没有别的种子用着数据的（可以连数据删），和数据还被别的种子用着的（只删种子）。
func (s *DataSharing) KeepSharedData(toDelete []Torrent) (withData, keepData []Torrent) {
	deleting := make(map[string]bool, len(toDelete))
	for _, t := range toDelete {
		deleting[TorrentKey(t)] = true
	}
	for _, t := range toDelete {
		if s.Shares(t, deleting) {
			keepData = append(keepData, t)
		} else {
			withData = append(withData, t)
		}
	}
	return withData, keepData
}
