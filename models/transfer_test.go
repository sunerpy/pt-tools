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

	root := []DownloaderPathMap{{SourcePrefix: "/", TargetPrefix: "/mnt/nas"}}
	got, ok := MapTransferPath(root, "/downloads/a")
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
