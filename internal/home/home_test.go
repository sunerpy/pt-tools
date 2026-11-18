package home

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/fsutil"
	"github.com/sunerpy/pt-tools/models"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{}, &models.SiteLoginState{}, &models.SiteAttendanceLog{}, &models.MediaTransferHistory{},
		&models.MediaSubscription{}, &models.MediaSubscriptionEpisode{}, &models.MediaIssue{}, &models.MediaServer{},
	))
	return db
}

// 各块同时取：取到的给数据，出错的写原因（去掉凭证），意外（panic）与超时也只影响那一块。
func TestCollect(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	got := Collect(context.Background(), []Part{
		{Name: "ok", Get: func(context.Context) (any, error) { return []int{1, 2}, nil }},
		{Name: "err", Get: func(context.Context) (any, error) {
			return nil, errors.New("GET http://qb.local/api?passkey=0123456789abcdef0123456789abcdef: 502")
		}},
		{Name: "panic", Get: func(context.Context) (any, error) { panic("坏了") }},
		{Name: "slow", Get: func(context.Context) (any, error) {
			<-block // 不理会 ctx 的块：照样按时给结果
			return 1, nil
		}},
	}, 200*time.Millisecond)
	require.Len(t, got, 4)
	assert.Equal(t, Block{Data: []int{1, 2}}, got["ok"])
	assert.Nil(t, got["err"].Data)
	assert.Contains(t, got["err"].Error, "502")
	assert.NotContains(t, got["err"].Error, "0123456789abcdef")
	assert.Equal(t, Block{Error: "出错了：坏了"}, got["panic"])
	assert.Equal(t, Block{Error: "0 秒内没取到，稍后刷新"}, got["slow"])
}

// stubDisks 把读盘换成固定的结果（路径 → 用量；不在表里的读不到）。
func stubDisks(t *testing.T, disks map[string]fsutil.Usage) {
	t.Helper()
	old := diskUsage
	diskUsage = func(p string) (fsutil.Usage, error) {
		if u, ok := disks[p]; ok {
			return u, nil
		}
		return fsutil.Usage{}, errors.New("读不到 " + p)
	}
	t.Cleanup(func() { diskUsage = old })
}

func TestStorage(t *testing.T) {
	const gb = 1 << 30
	stubDisks(t, map[string]fsutil.Usage{
		"/data/downloads": {Free: 900 * gb, Total: 4000 * gb, Device: "1"},
		"/data/media":     {Free: 900 * gb, Total: 4000 * gb, Device: "1"},
		"/tv":             {Free: 10 * gb, Total: 1000 * gb, Device: "2"},
		"/nas":            {Free: 300 * gb, Device: ""},
	})
	got := Storage([]DownloadDir{
		{Downloader: "qb", Path: "/downloads", Local: "/data/downloads", Free: 1},
		{Downloader: "tr", Path: "/remote", Local: "/remote", Free: 15 * gb},
		{Downloader: "qb2", Path: "/x", Local: "/x", Free: -1},
		{Downloader: "qb3", Err: errors.New("连不上")},
		{Downloader: "nas", Path: "/nas", Local: "/nas", Free: 1},
	}, []LibraryDir{{Name: "电影", Path: "/data/media"}, {Name: "剧集", Path: "/tv"}, {Name: "动画", Path: "/gone"}})
	require.Len(t, got, 7)
	assert.Equal(t, Disk{Kind: DiskDownload, Label: "下载盘 · qb、媒体库 · 电影", Path: "/downloads", Free: 900 * gb, Total: 4000 * gb}, got[0],
		"pt-tools 读得到时按本机读到的算；同一块盘只列一次")
	assert.Equal(t, Disk{Kind: DiskDownload, Label: "下载盘 · tr", Path: "/remote", Free: 15 * gb, Low: true}, got[1], "读不到时用下载器报的剩余空间")
	assert.Equal(t, "下载器没报剩余空间，pt-tools 里也读不到这个目录", got[2].Error)
	assert.Equal(t, Disk{Kind: DiskDownload, Label: "下载盘 · qb3", Error: "连不上"}, got[3])
	assert.Equal(t, Disk{Kind: DiskDownload, Label: "下载盘 · nas", Path: "/nas", Free: 300 * gb}, got[4], "没有设备号的不合并")
	assert.Equal(t, Disk{Kind: DiskLibrary, Label: "媒体库 · 剧集", Path: "/tv", Free: 10 * gb, Total: 1000 * gb, Low: true}, got[5])
	assert.Equal(t, Disk{Kind: DiskLibrary, Label: "媒体库 · 动画", Path: "/gone", Error: "读不到 /gone"}, got[6])
	assert.Empty(t, Storage(nil, nil))
	assert.NotNil(t, Storage(nil, nil))
}
