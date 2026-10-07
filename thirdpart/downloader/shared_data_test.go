package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 辅种与原种子共用同一份数据：删其中一个时不能连数据一起删，否则另一个就坏了。
func TestSharesData(t *testing.T) {
	orig := Torrent{InfoHash: "A", Name: "Movie", SavePath: "/data", ContentPath: "/data/Movie"}
	reseed := Torrent{InfoHash: "B", Name: "Movie", SavePath: "/data/", ContentPath: "/data/Movie/"}
	other := Torrent{InfoHash: "C", Name: "Other", SavePath: "/data", ContentPath: "/data/Other"}
	// 不建子文件夹的多文件种子：qBittorrent 报的内容路径就是保存路径
	flat := Torrent{InfoHash: "D", Name: "Pack", SavePath: "/data", ContentPath: "/data"}
	noPath := Torrent{InfoHash: "E", Name: "Show", SavePath: "/tv"}
	sameNoPath := Torrent{InfoHash: "F", Name: "Show", SavePath: "/tv/"}
	win := Torrent{InfoHash: "G", ContentPath: `D:\PT\Movie`}
	winSame := Torrent{InfoHash: "H", ContentPath: `d:/pt/movie`}
	all := []Torrent{orig, reseed, other, flat, noPath, sameNoPath, win, winSame}

	assert.True(t, SharesData(all, orig, nil), "同一个内容路径")
	assert.False(t, SharesData(all, other, nil))
	assert.False(t, SharesData(all, flat, nil), "保存路径下别的种子不算共用")
	assert.True(t, SharesData(all, noPath, nil), "没有内容路径时按保存路径 + 名称")
	assert.True(t, SharesData(all, win, nil), "Windows 路径分隔符、大小写不同")
	assert.False(t, SharesData(all, orig, map[string]bool{"b": true}), "同一批一起删的不算")
	assert.False(t, SharesData([]Torrent{orig, {InfoHash: "a", ContentPath: "/data/Movie"}}, orig, nil), "只有自己（hash 大小写不同）")
	assert.False(t, SharesData(all, Torrent{InfoHash: "Z"}, nil), "没有路径时说不清，不拦")

	// 没有 hash 时按 ID 认
	x := Torrent{ID: "1", ContentPath: "/m/X"}
	y := Torrent{ID: "2", ContentPath: "/m/X"}
	assert.True(t, SharesData([]Torrent{x, y}, x, nil))
	assert.False(t, SharesData([]Torrent{x, y}, x, map[string]bool{TorrentKey(y): true}))
	assert.Equal(t, "a", TorrentKey(Torrent{ID: "1", InfoHash: " A "}))
	assert.Equal(t, "id:1", TorrentKey(x))
}

func TestKeepSharedData(t *testing.T) {
	a := Torrent{InfoHash: "A", ContentPath: "/d/Movie"}
	b := Torrent{InfoHash: "B", ContentPath: "/d/Movie"}
	c := Torrent{InfoHash: "C", ContentPath: "/d/Other"}
	withData, keep := KeepSharedData([]Torrent{a, b, c}, []Torrent{a, c})
	assert.Equal(t, []Torrent{c}, withData)
	assert.Equal(t, []Torrent{a}, keep, "b 还用着 a 的数据")
	withData, keep = KeepSharedData([]Torrent{a, b, c}, []Torrent{a, b})
	assert.Len(t, withData, 2, "两个一起删：数据可以一起删")
	assert.Empty(t, keep)
}
