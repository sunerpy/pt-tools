package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// buttonDL 记下按钮推送用到的下载器、加入选项与开始调用。
type buttonDL struct {
	mu      sync.Mutex
	used    []string
	adds    []downloader.AddTorrentOptions
	resumes []string
}

func (b *buttonDL) mock(t *testing.T, name string) downloader.Downloader {
	t.Helper()
	m := sm.NewMockDownloader(gomock.NewController(t))
	m.EXPECT().GetName().Return(name).AnyTimes()
	m.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	m.EXPECT().Close().Return(nil).AnyTimes()
	m.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	m.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(1)<<50, nil).AnyTimes()
	m.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	m.EXPECT().GetAllTorrents().Return(nil, nil).AnyTimes()
	m.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).DoAndReturn(func(data []byte, o downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
		h, err := qbit.ComputeTorrentHash(data)
		if err != nil {
			return downloader.AddTorrentResult{}, err
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.used = append(b.used, name)
		b.adds = append(b.adds, o)
		return downloader.AddTorrentResult{Success: true, Hash: h}, nil
	}).AnyTimes()
	m.EXPECT().ResumeTorrent(gomock.Any()).DoAndReturn(func(id string) error {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.resumes = append(b.resumes, id)
		return nil
	}).AnyTimes()
	return m
}

type buttonRig struct {
	db       *models.TorrentDB
	dl       *buttonDL
	sub      models.RSSSubscription
	row      models.RSSNotificationLog
	boundID  uint
	actions  *RSSCallbackActions
	lookups  int
	lookupFn func() (*v2.TorrentItem, error)
}

// newButtonRig：站点绑定了下载器 bound（RSS 没有指定），另有默认下载器 default；RSS 的到期暂停按 pause。
func newButtonRig(t *testing.T, pause bool) *buttonRig {
	t.Helper()
	db, err := core.OpenTestDB(t)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.AutoMigrate(&models.RSSNotificationLog{}))
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	internal.GetDiskBudget().Reset()
	t.Cleanup(func() { internal.GetDiskBudget().Reset() })

	r := &buttonRig{db: db, dl: &buttonDL{}}
	def := models.DownloaderSetting{Name: "default", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true, AutoStart: true}
	bound := models.DownloaderSetting{Name: "bound", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true, AutoStart: true}
	require.NoError(t, db.DB.Create(&def).Error)
	require.NoError(t, db.DB.Create(&bound).Error)
	r.boundID = bound.ID
	site := models.SiteSetting{Name: "springsunday", Enabled: true, DownloaderID: &bound.ID}
	require.NoError(t, db.DB.Create(&site).Error)
	r.sub = models.RSSSubscription{SiteID: site.ID, Name: "r", URL: "http://x/rss", IntervalMinutes: 10, Category: "cat", Tag: "movie", DownloadPath: "/data/r", PauseOnFreeEnd: pause}
	require.NoError(t, db.DB.Create(&r.sub).Error)
	r.row = models.RSSNotificationLog{
		RSSID: r.sub.ID, SiteName: "springsunday", TorrentID: "77", NotifyKind: "filtered",
		NotificationConfID: 1, Result: "sent", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, db.DB.Create(&r.row).Error)

	t.Cleanup(internal.SwapPushDownloaderFactory(func(s models.DownloaderSetting) (downloader.Downloader, error) {
		return r.dl.mock(t, s.Name), nil
	}))
	fetcher := func(_ context.Context, _, id string) ([]byte, error) {
		name := "button-" + id
		return fmt.Appendf(nil, "d4:infod6:lengthi%de4:name%d:%s12:piece lengthi16384eee", int64(1)<<30, len(name), name), nil
	}
	r.actions = NewRSSCallbackActions(db.DB, fetcher)
	r.actions.SetDetailLookup(func(_ context.Context, siteName, torrentID string) (*v2.TorrentItem, error) {
		r.lookups++
		if siteName != "springsunday" || torrentID != "77" {
			return nil, fmt.Errorf("查错了种子: %s/%s", siteName, torrentID)
		}
		return r.lookupFn()
	})
	return r
}

func (r *buttonRig) record(t *testing.T) models.TorrentInfo {
	t.Helper()
	ti, err := r.db.GetTorrentBySiteAndID("springsunday", "77")
	require.NoError(t, err)
	require.NotNil(t, ti)
	return *ti
}

// RSS 通知里的「立即下载」：继承这条 RSS 的下载器顺序、路径与到期暂停；明确的用户操作不要求免费。
func TestOnRSSDownload_InheritsRSSPolicy(t *testing.T) {
	r := newButtonRig(t, true)
	end := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	r.lookupFn = func() (*v2.TorrentItem, error) {
		return &v2.TorrentItem{ID: "77", DiscountLevel: v2.DiscountFree, DiscountEndTime: end, FetchedAt: time.Now().UTC()}, nil
	}

	require.NoError(t, r.actions.OnRSSDownload(context.Background(), r.row.ID, 0))

	assert.Equal(t, 1, r.lookups, "推送前读了详情")
	require.Equal(t, []string{"bound"}, r.dl.used, "RSS 没指定下载器时用站点绑定的，与 RSS 推送同一顺序")
	assert.True(t, r.dl.adds[0].AddAtPaused, "继承到期暂停：先暂停加入")
	assert.Equal(t, "/data/r", r.dl.adds[0].SavePath)
	assert.Len(t, r.dl.resumes, 1, "记账与监控登记成功后开始")
	ti := r.record(t)
	assert.True(t, ti.PauseOnFreeEnd)
	assert.False(t, ti.RequireFree, "按钮是明确的用户操作，不要求免费")
	require.NotNil(t, ti.SourceRSSID)
	assert.Equal(t, r.sub.ID, *ti.SourceRSSID)
	require.NotNil(t, ti.DownloaderID)
	assert.Equal(t, r.boundID, *ti.DownloaderID)
	require.NotNil(t, ti.PausePlannedAt)
	assert.Equal(t, string(v2.FreeUntil), ti.FreeValidity)
	require.NotNil(t, ti.FreeEndTime)
	assert.True(t, ti.FreeEndTime.Equal(end))
}

// 继承了到期暂停但读不到结束时间：暂停加入，不自动开始。
func TestOnRSSDownload_PauseWithoutEndStaysPaused(t *testing.T) {
	r := newButtonRig(t, true)
	r.lookupFn = func() (*v2.TorrentItem, error) { return nil, errors.New("站点返回 500") }

	require.NoError(t, r.actions.OnRSSDownload(context.Background(), r.row.ID, 0))

	require.Len(t, r.dl.adds, 1, "不要求免费，照样推送")
	assert.True(t, r.dl.adds[0].AddAtPaused)
	assert.Empty(t, r.dl.resumes, "结束时间未知时不自动开始")
	assert.True(t, r.record(t).PauseOnFreeEnd)
}

// RSS 没开到期暂停：不免费也照常推送、照常开始。
func TestOnRSSDownload_NoPauseNotFreePushes(t *testing.T) {
	r := newButtonRig(t, false)
	r.lookupFn = func() (*v2.TorrentItem, error) {
		return &v2.TorrentItem{ID: "77", DiscountLevel: v2.DiscountNone, FetchedAt: time.Now().UTC()}, nil
	}

	require.NoError(t, r.actions.OnRSSDownload(context.Background(), r.row.ID, 0))

	require.Len(t, r.dl.adds, 1)
	assert.False(t, r.dl.adds[0].AddAtPaused)
	ti := r.record(t)
	assert.False(t, ti.PauseOnFreeEnd)
	assert.Equal(t, string(v2.FreeNotFree), ti.FreeValidity)
	assert.Nil(t, ti.PausePlannedAt)
}
