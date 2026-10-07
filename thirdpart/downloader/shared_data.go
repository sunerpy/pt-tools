package downloader

import "strings"

// SharesData 报告下载器里有没有别的种子用着 t 的数据：数据路径与 t 相同（辅种与原种子就是这样：文件完全一样，放在同一个位置）。
// deleting 里的种子（键见 TorrentKey）不算：它们与 t 同一批删，或者已经删掉了。t 没有路径可比时返回 false。
// 自动删种在删数据之前要先问它：有别的种子用着时只删种子、保留数据，否则另一个种子就坏了。
// 只比较数据路径、不比较文件列表，所以一个种子的文件放在另一个种子目录里的情况认不出来。
func SharesData(all []Torrent, t Torrent, deleting map[string]bool) bool {
	p := DataPath(t)
	if p == "" {
		return false
	}
	self := TorrentKey(t)
	for _, o := range all {
		k := TorrentKey(o)
		if k == self || deleting[k] {
			continue
		}
		if DataPath(o) == p {
			return true
		}
	}
	return false
}

// TorrentKey 是 SharesData 认种子用的键：小写 info hash，没有 hash 时用 ID。
func TorrentKey(t Torrent) string {
	if h := strings.ToLower(strings.TrimSpace(t.InfoHash)); h != "" {
		return h
	}
	return "id:" + t.ID
}

// DataPath 是种子数据的路径（统一成 / 分隔、去掉末尾的 /；Windows 路径不区分大小写）。没有内容路径时用保存路径 + 名称。
func DataPath(t Torrent) string {
	p := t.ContentPath
	if strings.TrimSpace(p) == "" {
		if strings.TrimSpace(t.SavePath) == "" || t.Name == "" {
			return ""
		}
		p = strings.TrimRight(t.SavePath, "/\\") + "/" + t.Name
	}
	win := strings.Contains(p, "\\") || (len(p) >= 2 && p[1] == ':')
	p = strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
	if win {
		p = strings.ToLower(p)
	}
	return p
}

// KeepSharedData 把要删的种子分成两组：没有别的种子用着数据的（可以连数据删），和数据还被别的种子用着的（只删种子）。
func KeepSharedData(all, toDelete []Torrent) (withData, keepData []Torrent) {
	deleting := make(map[string]bool, len(toDelete))
	for _, t := range toDelete {
		deleting[TorrentKey(t)] = true
	}
	for _, t := range toDelete {
		if SharesData(all, t, deleting) {
			keepData = append(keepData, t)
		} else {
			withData = append(withData, t)
		}
	}
	return withData, keepData
}
