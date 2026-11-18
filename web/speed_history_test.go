package web

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 一格（30 秒）只记一条，最多留 20 条（丢最旧的）。
func TestSpeedHistory(t *testing.T) {
	var h speedHistory
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	h.record(SpeedSample{At: t0, Down: 1})
	h.record(SpeedSample{At: t0.Add(29 * time.Second), Down: 2})
	h.record(SpeedSample{At: t0.Add(31 * time.Second), Down: 3})
	got := h.snapshot()
	require.Len(t, got, 2, "同一格里只记第一条")
	assert.Equal(t, []int64{1, 3}, []int64{got[0].Down, got[1].Down})

	for i := range 30 {
		h.record(SpeedSample{At: t0.Add(time.Duration(i+2) * speedSlot), Down: int64(100 + i)})
	}
	got = h.snapshot()
	require.Len(t, got, speedHistoryLen)
	assert.Equal(t, int64(110), got[0].Down, "丢掉最旧的")
	assert.Equal(t, int64(129), got[len(got)-1].Down)

	got[0].Down = -1
	assert.Equal(t, int64(110), h.snapshot()[0].Down, "给出去的是副本")
}

// 每算完一次总速度记一条：两台下载器里一台连不上时，那一条只含另一台的速度，answered 是 1、total 是 2。
func TestTransferStatsRecordsSpeed(t *testing.T) {
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	dm := mgr.GetDownloaderManager()
	fakes := map[string]*fakeDownloader{
		"qb1": {status: downloader.ClientStatus{UpSpeed: 5, DlSpeed: 6}},
		"qb2": {statusErr: assertErr("dial tcp: connection refused"), freeSpaceErr: assertErr("dial tcp: connection refused")},
	}
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		f := fakes[name]
		f.name, f.dlType = name, downloader.DownloaderQBittorrent
		return f, nil
	})
	for _, name := range []string{"qb1", "qb2"} {
		require.NoError(t, dm.RegisterConfig(name, downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://"+name, "u", "p", true), name == "qb1"))
		require.NoError(t, db.Create(&models.DownloaderSetting{Name: name, Type: "qbittorrent", Enabled: true}).Error)
	}
	server.mgr = mgr

	records, err := server.listEnabledDownloaderRecords(nil)
	require.NoError(t, err)
	resp := server.collectTransferStats(context.Background(), dm, records)
	assert.Equal(t, int64(6), resp.TotalDownloadSpeed)
	got := server.speeds.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, SpeedSample{At: got[0].At, Down: 6, Up: 5, Answered: 1, Total: 2}, got[0])

	// 没有启用的下载器时不记
	var empty Server
	empty.collectTransferStats(context.Background(), dm, nil)
	assert.Empty(t, empty.speeds.snapshot())
}
