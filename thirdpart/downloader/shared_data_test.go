package downloader

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 辅种与原种子共用同一份数据：删其中一个时不能连数据一起删，否则另一个就坏了。
func TestDataSharing(t *testing.T) {
	orig := Torrent{InfoHash: "A", Name: "Movie", SavePath: "/data", ContentPath: "/data/Movie"}
	reseed := Torrent{InfoHash: "B", Name: "Movie", SavePath: "/data/", ContentPath: "/data/Movie/"}
	other := Torrent{InfoHash: "C", Name: "Other", SavePath: "/data", ContentPath: "/data/Other"}
	noPath := Torrent{InfoHash: "E", Name: "Show", SavePath: "/tv"}
	sameNoPath := Torrent{InfoHash: "F", Name: "Show", SavePath: "/tv/"}
	win := Torrent{InfoHash: "G", ContentPath: `D:\PT\Movie`}
	winSame := Torrent{InfoHash: "H", ContentPath: `d:/pt/movie`}
	s := NewDataSharing([]Torrent{orig, reseed, other, noPath, sameNoPath, win, winSame}, nil)

	assert.True(t, s.Shares(orig, nil), "同一个内容路径")
	assert.Equal(t, []Torrent{reseed}, s.Sharers(orig))
	assert.False(t, s.Shares(other, nil))
	assert.True(t, s.Shares(noPath, nil), "没有内容路径时按保存路径 + 名称")
	assert.True(t, s.Shares(win, nil), "Windows 路径分隔符、大小写不同")
	assert.False(t, s.Shares(orig, map[string]bool{"b": true}), "同一批一起删的不算")
	assert.False(t, NewDataSharing([]Torrent{orig, {InfoHash: "a", ContentPath: "/data/Movie"}}, nil).Shares(orig, nil), "只有自己（hash 大小写不同）")
	assert.False(t, s.Shares(Torrent{InfoHash: "Z"}, nil), "没有路径时说不清，不拦")

	// 没有 hash 时按 ID 认
	x := Torrent{ID: "1", ContentPath: "/m/X"}
	y := Torrent{ID: "2", ContentPath: "/m/X"}
	assert.True(t, NewDataSharing([]Torrent{x, y}, nil).Shares(x, nil))
	assert.False(t, NewDataSharing([]Torrent{x, y}, nil).Shares(x, map[string]bool{TorrentKey(y): true}))
	assert.Equal(t, "a", TorrentKey(Torrent{ID: "1", InfoHash: " A "}))
	assert.Equal(t, "id:1", TorrentKey(x))
}

// qBittorrent 不建子文件夹的多文件种子：内容路径就是保存路径，同一目录下的种子按文件判断。
func TestDataSharingFlat(t *testing.T) {
	pack := Torrent{ID: "p", InfoHash: "p", Name: "Pack", SavePath: "/data", ContentPath: "/data", TotalSize: 30}
	packReseed := Torrent{ID: "pr", InfoHash: "pr", Name: "Pack", SavePath: "/data/", ContentPath: "/data/", TotalSize: 30}
	unrelated := Torrent{ID: "u", InfoHash: "u", Name: "Other", SavePath: "/data", ContentPath: "/data", TotalSize: 30}
	partial := Torrent{ID: "b", InfoHash: "b", Name: "Part", SavePath: "/data", ContentPath: "/data", TotalSize: 20}
	single := Torrent{ID: "s", InfoHash: "s", Name: "a.mkv", SavePath: "/data", ContentPath: "/data/a.mkv", TotalSize: 20}
	files := map[string][]TorrentFile{
		"p":  {{Name: "a.mkv", Size: 20}, {Name: "a.srt", Size: 10}},
		"pr": {{Name: "a.mkv", Size: 20}, {Name: "a.srt", Size: 10}},
		"u":  {{Name: "x.mkv", Size: 30}},
		"b":  {{Name: "a.mkv", Size: 20}},
	}
	var calls []string
	lister := func(t Torrent) ([]TorrentFile, error) {
		calls = append(calls, t.ID)
		if fs, ok := files[t.ID]; ok {
			return fs, nil
		}
		return nil, errors.New("读不到")
	}
	s := NewDataSharing([]Torrent{pack, packReseed, unrelated, partial, single}, lister)
	assert.Equal(t, []Torrent{packReseed, partial}, s.Sharers(pack), "文件相同或有重叠的算；同目录文件不同的不算")
	assert.False(t, s.Shares(unrelated, nil), "同目录、文件不同")
	assert.True(t, s.Shares(partial, map[string]bool{"pr": true}), "只有部分文件相同也算共用")
	assert.False(t, s.Shares(single, nil), "单文件种子的数据路径不同")
	n := len(calls)
	s.Shares(pack, nil)
	assert.Len(t, calls, n, "文件列表读一次就记住")

	// 读不到文件列表：当作共用，宁可留下数据
	ghost := Torrent{ID: "g", InfoHash: "g", Name: "Ghost", SavePath: "/data", ContentPath: "/data", TotalSize: 30}
	s2 := NewDataSharing([]Torrent{pack, ghost}, lister)
	assert.True(t, s2.Shares(pack, nil))
	assert.True(t, NewDataSharing([]Torrent{pack, unrelated}, nil).Shares(pack, nil), "没有 files 时也当作共用")
}

func TestKeepSharedData(t *testing.T) {
	a := Torrent{InfoHash: "A", ContentPath: "/d/Movie"}
	b := Torrent{InfoHash: "B", ContentPath: "/d/Movie"}
	c := Torrent{InfoHash: "C", ContentPath: "/d/Other"}
	s := NewDataSharing([]Torrent{a, b, c}, nil)
	withData, keep := s.KeepSharedData([]Torrent{a, c})
	assert.Equal(t, []Torrent{c}, withData)
	assert.Equal(t, []Torrent{a}, keep, "b 还用着 a 的数据")
	withData, keep = s.KeepSharedData([]Torrent{a, b})
	assert.Len(t, withData, 2, "两个一起删：数据可以一起删")
	assert.Empty(t, keep)
}
