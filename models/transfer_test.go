package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapTransferPath(t *testing.T) {
	maps := []DownloaderPathMap{
		{SourcePrefix: "/downloads", TargetPrefix: "/data"},
		{SourcePrefix: "/downloads/movies/", TargetPrefix: "/mnt/movies/"},
		{SourcePrefix: `D:\PT`, TargetPrefix: "/volume1/pt"},
		{SourcePrefix: "/srv", TargetPrefix: `E:\srv`},
	}
	cases := []struct {
		src, want string
		ok        bool
	}{
		{"/downloads", "/data", true},
		{"/downloads/tv/A", "/data/tv/A", true},
		{"/downloads/movies/B", "/mnt/movies/B", true}, // 最长前缀
		{"/downloadsX/C", "/downloadsX/C", false},      // 按路径分段匹配
		{`D:\PT\Movies\D`, "/volume1/pt/Movies/D", true},
		{"/srv/a/b", `E:\srv\a\b`, true},
		{"/other", "/other", false},
	}
	for _, c := range cases {
		got, ok := MapTransferPath(maps, c.src)
		assert.Equal(t, c.want, got, c.src)
		assert.Equal(t, c.ok, ok, c.src)
	}

	// Windows 路径不区分大小写，/ 与 \ 都认；Unix 路径仍区分大小写
	win := []DownloaderPathMap{{SourcePrefix: `D:\PT`, TargetPrefix: "/volume1/pt"}, {SourcePrefix: `\\nas\share`, TargetPrefix: "/mnt/share"}}
	for src, want := range map[string]string{
		`d:\pt\Movie`:       "/volume1/pt/Movie",
		`D:/PT/Movie/a.mkv`: "/volume1/pt/Movie/a.mkv",
		`\\NAS\Share\TV`:    "/mnt/share/TV",
		`D:\PTX\Movie`:      `D:\PTX\Movie`,
	} {
		got, _ := MapTransferPath(win, src)
		assert.Equal(t, want, got, src)
	}
	got, ok := MapTransferPath([]DownloaderPathMap{{SourcePrefix: "/downloads", TargetPrefix: "/data"}}, "/Downloads/x")
	assert.False(t, ok, "Unix 路径区分大小写")
	assert.Equal(t, "/Downloads/x", got)

	// 当前盘根目录 \ 与不带盘符的 \Downloads 也是 Windows 写法
	got, ok = MapTransferPath([]DownloaderPathMap{{SourcePrefix: `\`, TargetPrefix: "/mnt/c"}}, `\Downloads\A`)
	assert.True(t, ok)
	assert.Equal(t, "/mnt/c/Downloads/A", got)
	got, ok = MapTransferPath([]DownloaderPathMap{{SourcePrefix: `\Downloads`, TargetPrefix: "/data"}}, `\downloads\a`)
	assert.True(t, ok)
	assert.Equal(t, "/data/a", got)

	root := []DownloaderPathMap{{SourcePrefix: "/", TargetPrefix: "/mnt/nas"}}
	got, ok = MapTransferPath(root, "/downloads/a")
	assert.True(t, ok)
	assert.Equal(t, "/mnt/nas/downloads/a", got)
	got, ok = MapTransferPath(nil, "/x")
	assert.False(t, ok)
	assert.Equal(t, "/x", got)
}

func TestTransferStateFinal(t *testing.T) {
	for _, s := range []string{TransferDone, TransferRolledBack, TransferFailed, TransferCanceled} {
		assert.True(t, TransferStateFinal(s), s)
	}
	for _, s := range TransferActiveStates {
		assert.False(t, TransferStateFinal(s), s)
	}
}
