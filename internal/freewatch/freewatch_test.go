package freewatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeSite 是能读详情的站点：Download 回种子文件（失败时回 dlErr）。
type fakeSite struct {
	v2.Site
	detail bool
	dlErr  error
	mu     sync.Mutex
	got    []string
}

func (f *fakeSite) GetDetailFetcher() v2.TorrentDetailFetcher {
	if !f.detail {
		return nil
	}
	return fakeFetcher{}
}

func (f *fakeSite) Download(_ context.Context, id string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, id)
	if f.dlErr != nil {
		return nil, f.dlErr
	}
	return []byte("torrent:" + id), nil
}

type fakeFetcher struct{ v2.TorrentDetailFetcher }

type env struct {
	t     *testing.T
	ctx   context.Context
	db    *gorm.DB
	svc   *Service
	sites map[string]*fakeSite
	mu    sync.Mutex
	now   time.Time
	// snap 是每个种子编号读到的优惠（没有时读不到）
	snap    map[string]*v2.PromotionSnapshot
	reads   int
	pushes  []ptinternal.PushTorrentRequest
	pushRes *ptinternal.PushTorrentResult
	pushErr error
	msgs    []Message
	dlID    uint
}

// sameInstant 比的是时刻，不比时区：服务写进库的时间换成了本地时区（见 New）。
func sameInstant(t *testing.T, want, got time.Time, msg ...any) {
	t.Helper()
	assert.Truef(t, want.Equal(got), "want %s, got %s %v", want, got, msg)
}

func (e *env) Now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&models.FreeWatch{}, &models.DownloaderSetting{}))
	dl := models.DownloaderSetting{Name: "家里的 qB", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	e := &env{
		t: t, ctx: context.Background(), db: db, now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC),
		sites: map[string]*fakeSite{"hdtime": {detail: true}, "mteam": {detail: true}, "unit3d": {}},
		snap:  map[string]*v2.PromotionSnapshot{}, dlID: dl.ID,
	}
	e.svc = New(Config{
		DB: db,
		Site: func(name string) v2.Site {
			if s, ok := e.sites[name]; ok {
				return s
			}
			return nil
		},
		Fresh: func(_ context.Context, _ v2.Site, _, id string, attempt time.Time) *v2.PromotionSnapshot {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.reads++
			s := e.snap[id]
			if s == nil {
				return nil
			}
			out := *s
			out.VerifiedAt = attempt
			return &out
		},
		Push: func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.pushes = append(e.pushes, req)
			if e.pushErr != nil {
				return nil, e.pushErr
			}
			if e.pushRes != nil {
				return e.pushRes, nil
			}
			return &ptinternal.PushTorrentResult{Success: true, TorrentHash: "ABCDEF", Status: ptinternal.PushStatusPushedPaused}, nil
		},
		Notify: func(_ context.Context, m Message) {
			e.mu.Lock()
			e.msgs = append(e.msgs, m)
			e.mu.Unlock()
		},
		SafetyMargin: func() time.Duration { return 10 * time.Minute },
		Now:          e.Now,
	})
	return e
}

func (e *env) setSnap(id string, level v2.DiscountLevel, validity v2.FreeValidity, endsIn time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := &v2.PromotionSnapshot{Level: level, Validity: validity}
	if endsIn > 0 {
		end := e.now.Add(endsIn)
		s.EndsAt = &end
	}
	e.snap[id] = s
}

func (e *env) create(id string, mod func(*CreateInput)) *models.FreeWatch {
	e.t.Helper()
	in := CreateInput{Site: "hdtime", TorrentID: id, Title: "Fake.Movie." + id, SizeBytes: 1 << 30, DownloaderID: e.dlID}
	if mod != nil {
		mod(&in)
	}
	w, err := e.svc.Create(e.ctx, in)
	require.NoError(e.t, err)
	return w
}

func (e *env) row(id uint) models.FreeWatch {
	e.t.Helper()
	var w models.FreeWatch
	require.NoError(e.t, e.db.First(&w, id).Error)
	return w
}

func (e *env) kinds() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, m := range e.msgs {
		out = append(out, m.Kind+":"+m.Watch.TorrentID)
	}
	return out
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	cases := map[string]CreateInput{
		"没有编号":     {Site: "hdtime", DownloaderID: e.dlID},
		"没有下载器":    {Site: "hdtime", TorrentID: "1"},
		"天数太多":     {Site: "hdtime", TorrentID: "1", DownloaderID: e.dlID, Days: MaxDays + 1},
		"天数是负的":    {Site: "hdtime", TorrentID: "1", DownloaderID: e.dlID, Days: -1},
		"读不了详情的站点": {Site: "unit3d", TorrentID: "1", DownloaderID: e.dlID},
		"没有的站点":    {Site: "nope", TorrentID: "1", DownloaderID: e.dlID},
		"没有的下载器":   {Site: "hdtime", TorrentID: "1", DownloaderID: e.dlID + 9},
		"条目类型不对":   {Site: "hdtime", TorrentID: "1", DownloaderID: e.dlID, TMDBID: 5, MediaType: "music"},
	}
	for name, in := range cases {
		_, err := e.svc.Create(e.ctx, in)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	off := models.DownloaderSetting{Name: "停用的", Type: "qbittorrent", URL: "http://x", Enabled: true}
	require.NoError(t, e.db.Create(&off).Error)
	require.NoError(t, e.db.Model(&off).Update("enabled", false).Error)
	_, err := e.svc.Create(e.ctx, CreateInput{Site: "hdtime", TorrentID: "1", DownloaderID: off.ID})
	require.ErrorIs(t, err, ErrInvalid, "下载器没启用")

	w := e.create("1", func(in *CreateInput) {
		in.MediaType, in.TMDBID, in.Season = models.MediaKindMovie, 693134, 3
	})
	assert.Equal(t, models.FreeWatchWatching, w.Status)
	sameInstant(t, e.Now().Add(DefaultDays*24*time.Hour), w.ExpiresAt, "默认等 7 天")
	require.NotNil(t, w.NextCheckAt)
	sameInstant(t, e.Now(), *w.NextCheckAt, "建好马上检查")
	assert.Zero(t, w.Season, "电影不记季")

	_, err = e.svc.Create(e.ctx, CreateInput{Site: "hdtime", TorrentID: "1", DownloaderID: e.dlID})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "已经在等免费了")
	assert.True(t, e.svc.Supports("hdtime"))
	assert.False(t, e.svc.Supports("unit3d"))
	assert.False(t, New(Config{DB: e.db}).Supports("hdtime"), "没有站点时都不支持")
}

func TestCreateLimit(t *testing.T) {
	e := newEnv(t)
	rows := make([]models.FreeWatch, MaxWatching)
	for i := range rows {
		rows[i] = models.FreeWatch{SiteName: "hdtime", TorrentID: "x" + string(rune('a'+i%26)) + strings.Repeat("y", i/26), Status: models.FreeWatchWatching, ExpiresAt: e.Now().Add(time.Hour)}
	}
	require.NoError(t, e.db.CreateInBatches(rows, 100).Error)
	_, err := e.svc.Create(e.ctx, CreateInput{Site: "hdtime", TorrentID: "new", DownloaderID: e.dlID})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "200")
}

// 免费了就推送：要求免费、免费结束时暂停、带资格快照与条目；记成已推送并通知（带下载器名与免费结束时间）。
func TestCheckPushesWhenFree(t *testing.T) {
	e := newEnv(t)
	w := e.create("300101", func(in *CreateInput) {
		in.SavePath, in.Category, in.Tags, in.HasHR, in.HRSeedTimeH = "/dl", "movies", "a,b", true, 72
		in.MediaType, in.TMDBID, in.Season = models.MediaKindTV, 231620, 4
	})
	e.setSnap("300101", v2.DiscountFree, v2.FreeUntil, 24*time.Hour)
	assert.Equal(t, 1, e.svc.RunOnce(e.ctx))

	require.Len(t, e.pushes, 1)
	req := e.pushes[0]
	assert.Equal(t, []any{"hdtime", "300101", "Fake.Movie.300101", "/dl", "movies", "a,b", e.dlID, SourceKind()},
		[]any{req.SiteID, req.TorrentID, req.Title, req.SavePath, req.Category, req.Tags, req.DownloaderID, req.Source})
	assert.Equal(t, []byte("torrent:300101"), req.TorrentData)
	assert.True(t, req.Policy.RequireFree)
	assert.True(t, req.Policy.PauseOnFreeEnd)
	assert.Equal(t, ptinternal.SourceKindFreeWatch, req.Policy.SourceKind)
	assert.Equal(t, 10*time.Minute, req.Policy.SafetyMargin)
	sameInstant(t, e.Now(), req.Policy.AttemptStart)
	require.NotNil(t, req.Snapshot)
	assert.Equal(t, v2.DiscountFree, req.Snapshot.Level)
	assert.Equal(t, &ptinternal.PushMedia{Type: models.MediaKindTV, TMDBID: 231620, Season: 4}, req.Media)
	assert.Equal(t, &ptinternal.PushTorrentMeta{SizeBytes: 1 << 30, HasHR: true, HRSeedTimeH: 72}, req.Meta)

	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchPushed, got.Status)
	assert.Equal(t, "abcdef", got.InfoHash)
	assert.Equal(t, "免费了，已推送", got.Message)
	assert.Nil(t, got.NextCheckAt)
	assert.Equal(t, 1, got.CheckCount)
	require.Len(t, e.msgs, 1)
	m := e.msgs[0]
	assert.Equal(t, KindPushed, m.Kind)
	assert.Equal(t, "家里的 qB", m.Downloader)
	require.NotNil(t, m.Snapshot)

	assert.Zero(t, e.svc.RunOnce(e.ctx), "推送了的不再检查")
}

func SourceKind() string { return ptinternal.SourceKindFreeWatch }

// 还没免费、免费剩得太少、结束时间不知道：30 分钟后再看，连续失败清零；说明写现在的优惠。
func TestCheckNotFreeYet(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", nil)
	e.setSnap("1", v2.DiscountPercent50, v2.FreeNotFree, 0)
	e.svc.RunOnce(e.ctx)
	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchWatching, got.Status)
	assert.Equal(t, "还没免费（现在是 50%）", got.Message)
	require.NotNil(t, got.NextCheckAt)
	sameInstant(t, e.Now().Add(NotFreeRetry), *got.NextCheckAt)
	assert.Empty(t, e.pushes)

	assert.Zero(t, e.svc.RunOnce(e.ctx), "没到点不检查")
	e.advance(NotFreeRetry)
	e.setSnap("1", v2.DiscountNone, v2.FreeNotFree, 0)
	require.NoError(t, e.db.Model(&models.FreeWatch{}).Where("id = ?", w.ID).Update("failures", 3).Error)
	e.svc.RunOnce(e.ctx)
	got = e.row(w.ID)
	assert.Equal(t, "还没免费", got.Message)
	assert.Zero(t, got.Failures, "读到了就清零")
	assert.Equal(t, 2, got.CheckCount)

	e.advance(NotFreeRetry)
	e.setSnap("1", v2.DiscountFree, v2.FreeUntil, 5*time.Minute)
	e.svc.RunOnce(e.ctx)
	assert.Contains(t, e.row(w.ID).Message, "免费剩余时间不超过 10 分钟")

	e.advance(NotFreeRetry)
	e.setSnap("1", v2.DiscountFree, v2.FreeUnknown, 0)
	e.svc.RunOnce(e.ctx)
	assert.Equal(t, "免费结束时间未知，暂不推送", e.row(w.ID).Message)
	assert.Empty(t, e.pushes)

	// 长期免费：推送，通知里没有结束时间
	e.advance(NotFreeRetry)
	e.setSnap("1", v2.DiscountFree, v2.FreePermanent, 0)
	e.svc.RunOnce(e.ctx)
	assert.Equal(t, models.FreeWatchPushed, e.row(w.ID).Status)
	_, text, ev := Notice(e.msgs[0], e.Now())
	assert.NotContains(t, text, "免费到")
	assert.Equal(t, "", ev.Fields["free_end"])
}

// 读不到（站点、详情、种子文件、推送出错）：按 1、2、4、4 小时退避，第 5 次不再等并通知。
func TestCheckFailuresBackoff(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", nil)
	waits := []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour}
	for i, wait := range waits {
		e.svc.RunOnce(e.ctx)
		got := e.row(w.ID)
		require.Equal(t, models.FreeWatchWatching, got.Status, i)
		assert.Equal(t, i+1, got.Failures)
		require.NotNil(t, got.NextCheckAt)
		sameInstant(t, e.Now().Add(wait), *got.NextCheckAt, i)
		assert.Contains(t, got.Message, "读不到种子现在的优惠")
		e.advance(wait)
	}
	e.svc.RunOnce(e.ctx)
	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchFailed, got.Status)
	assert.Equal(t, MaxFailures, got.Failures)
	assert.Equal(t, MaxFailures, got.CheckCount)
	assert.Contains(t, got.Message, "连续 5 次，不再等")
	assert.Equal(t, []string{"failed:1"}, e.kinds())
	assert.Equal(t, "读不到种子现在的优惠", e.msgs[0].Error)

	// 站点没了、种子文件下不来、推送出错也算读不到
	w2 := e.create("2", func(in *CreateInput) { in.Site = "mteam" })
	delete(e.sites, "mteam")
	e.svc.RunOnce(e.ctx)
	assert.Contains(t, e.row(w2.ID).Message, "站点没有启用或者不存在")

	w3 := e.create("3", nil)
	e.setSnap("3", v2.DiscountFree, v2.FreeUntil, time.Hour)
	e.sites["hdtime"].dlErr = errors.New("连接超时")
	e.svc.RunOnce(e.ctx)
	assert.Contains(t, e.row(w3.ID).Message, "下载种子文件失败：连接超时")

	e.sites["hdtime"].dlErr = nil
	e.advance(time.Hour)
	e.setSnap("3", v2.DiscountFree, v2.FreeUntil, time.Hour)
	e.pushErr = errors.New("下载器连不上")
	e.svc.RunOnce(e.ctx)
	got = e.row(w3.ID)
	assert.Equal(t, 2, got.Failures)
	assert.Contains(t, got.Message, "推送失败：下载器连不上")
}

// 门禁或闸门没放行：30 分钟后再看；下载器里已经有：记成已推送，不通知。
func TestCheckPushRejectedAndDuplicate(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", nil)
	e.setSnap("1", v2.DiscountFree, v2.FreeUntil, 24*time.Hour)
	e.pushRes = &ptinternal.PushTorrentResult{Status: ptinternal.PushStatusRejectedDisk, Message: "磁盘空间不足"}
	e.svc.RunOnce(e.ctx)
	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchWatching, got.Status)
	assert.Equal(t, "免费了，但这次没推送：磁盘空间不足", got.Message)
	sameInstant(t, e.Now().Add(NotFreeRetry), *got.NextCheckAt)

	e.advance(NotFreeRetry)
	e.pushRes = &ptinternal.PushTorrentResult{Success: true, Skipped: true, Status: ptinternal.PushStatusDuplicate, TorrentHash: "AA"}
	e.svc.RunOnce(e.ctx)
	got = e.row(w.ID)
	assert.Equal(t, models.FreeWatchPushed, got.Status)
	assert.Equal(t, "下载器里已经有这个种子，不用再推", got.Message)
	assert.Equal(t, "aa", got.InfoHash)
	assert.Empty(t, e.kinds(), "不是这次推的，不通知")

	w2 := e.create("2", nil)
	e.setSnap("2", v2.DiscountFree, v2.FreeUntil, 24*time.Hour)
	e.pushRes = &ptinternal.PushTorrentResult{Success: true, Status: ptinternal.PushStatusMonitorDegraded, Message: "登记失败"}
	e.svc.RunOnce(e.ctx)
	assert.Equal(t, "免费了，已推送（免费结束监控没登记上：登记失败）", e.row(w2.ID).Message)

	w3 := e.create("3", nil)
	e.setSnap("3", v2.DiscountFree, v2.FreeUntil, 24*time.Hour)
	e.pushRes = nil
	e.mu.Lock()
	e.pushErr = nil
	e.mu.Unlock()
	svc := e.svc
	svc.cfg.Push = func(context.Context, ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		return nil, nil
	}
	svc.RunOnce(e.ctx)
	assert.Contains(t, e.row(w3.ID).Message, "推送没有结果")
}

// 到期：记成过期并通知（不用等到下次检查）；每个站点每轮最多读 PerSiteRound 个。
func TestExpireAndPerSiteCap(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", func(in *CreateInput) { in.Days = 1 })
	e.setSnap("1", v2.DiscountNone, v2.FreeNotFree, 0)
	e.svc.RunOnce(e.ctx)
	e.advance(24 * time.Hour)
	e.svc.RunOnce(e.ctx)
	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchExpired, got.Status)
	assert.Equal(t, "等到期了还没免费", got.Message)
	assert.Equal(t, []string{"expired:1"}, e.kinds())
	_, text, ev := Notice(e.msgs[0], e.Now())
	assert.Equal(t, "「Fake.Movie.1」（hdtime）等了 1 天还没免费，不再等了。", text)
	assert.Equal(t, int64(1), ev.Fields["days"])

	for i := range PerSiteRound + 2 {
		e.create("h"+string(rune('a'+i)), nil)
	}
	e.create("m1", func(in *CreateInput) { in.Site = "mteam" })
	before := e.reads
	assert.Equal(t, PerSiteRound+1, e.svc.RunOnce(e.ctx), "hdtime 读 10 个，mteam 读 1 个")
	assert.Equal(t, before+PerSiteRound+1, e.reads)
	assert.Equal(t, 2, e.svc.RunOnce(e.ctx), "剩下的下一轮接着读")
}

// 取消与改到期只对在等的；改到期要晚于现在、最多 30 天以后。
func TestCancelAndSetExpiry(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", nil)
	_, err := e.svc.SetExpiry(e.ctx, w.ID, e.Now())
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SetExpiry(e.ctx, w.ID, e.Now().Add(31*24*time.Hour))
	require.ErrorIs(t, err, ErrInvalid)
	at := e.Now().Add(10 * 24 * time.Hour)
	got, err := e.svc.SetExpiry(e.ctx, w.ID, at)
	require.NoError(t, err)
	sameInstant(t, at, got.ExpiresAt)
	assert.Equal(t, at, e.row(w.ID).ExpiresAt.UTC())
	_, err = e.svc.SetExpiry(e.ctx, w.ID+9, at)
	require.ErrorIs(t, err, ErrNotFound)

	c, err := e.svc.Cancel(e.ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FreeWatchCanceled, c.Status)
	assert.Equal(t, "取消了", c.Message)
	_, err = e.svc.Cancel(e.ctx, w.ID)
	require.ErrorIs(t, err, ErrInvalid, "已经不在等了")
	_, err = e.svc.SetExpiry(e.ctx, w.ID, at)
	require.ErrorIs(t, err, ErrInvalid)
	assert.Zero(t, e.svc.RunOnce(e.ctx), "取消的不检查")
	assert.Empty(t, e.kinds())

	again := e.create("1", nil)
	assert.NotEqual(t, w.ID, again.ID, "取消以后同一个种子可以再等")
	rows, counts, err := e.svc.List(e.ctx, "")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, again.ID, rows[0].ID, "新建的在前")
	assert.Equal(t, Counts{models.FreeWatchWatching: 1, models.FreeWatchCanceled: 1}, counts)
	rows, _, err = e.svc.List(e.ctx, models.FreeWatchCanceled)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, w.ID, rows[0].ID)

	// 列表最多列 ListLimit 条，条数照实数
	old := make([]models.FreeWatch, ListLimit)
	for i := range old {
		old[i] = models.FreeWatch{SiteName: "hdtime", TorrentID: fmt.Sprintf("old-%d", i), Status: models.FreeWatchExpired, ExpiresAt: e.Now()}
	}
	require.NoError(t, e.db.CreateInBatches(old, 100).Error)
	rows, counts, err = e.svc.List(e.ctx, "")
	require.NoError(t, err)
	assert.Len(t, rows, ListLimit)
	assert.Equal(t, int64(ListLimit), counts[models.FreeWatchExpired])
}

func TestNoticeTexts(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	end := now.Add(23*time.Hour + 59*time.Minute)
	w := models.FreeWatch{ID: 3, SiteName: "hdtime", TorrentID: "9", Title: "T", SizeBytes: 15 << 30, Failures: 5, CreatedAt: now.Add(-3 * 24 * time.Hour)}
	title, text, ev := Notice(Message{Kind: KindPushed, Watch: w, Downloader: "qb", Snapshot: &v2.PromotionSnapshot{EndsAt: &end}}, now)
	assert.Equal(t, "等到免费了，已推送", title)
	assert.Equal(t, "「T」（hdtime）免费了，已推送到 qb。\n免费到 "+end.In(time.Local).Format(time.DateTime)+"（还剩 23h59min），到时候没下完会暂停。", text)
	assert.Equal(t, "15.00 GiB", ev.Fields["size"])
	assert.Equal(t, "qb", ev.Fields["downloader"])
	assert.Equal(t, "23h59min", ev.Fields["free_remaining"])

	_, text, ev = Notice(Message{Kind: KindExpired, Watch: w}, now)
	assert.Equal(t, "「T」（hdtime）等了 3 天还没免费，不再等了。", text)
	assert.Equal(t, int64(3), ev.Fields["days"])

	title, text, ev = Notice(Message{Kind: KindFailed, Watch: w, Error: "读不到"}, now)
	assert.Equal(t, "等免费没成，不再等了", title)
	assert.Equal(t, "「T」（hdtime）连续 5 次没成，不再等了。最近一次：读不到。", text)
	assert.Equal(t, int64(5), ev.Fields["fails"])
	assert.Equal(t, "45min", remaining(45*time.Minute))
}

func TestLevelTextAndClip(t *testing.T) {
	for level, want := range map[v2.DiscountLevel]string{
		v2.DiscountPercent50: "50%", v2.DiscountPercent30: "30%", v2.DiscountPercent70: "70%", v2.Discount2xUp: "2X 上传",
		v2.Discount2x50: "2X 50%", v2.DiscountNone: "", v2.DiscountFree: "",
	} {
		assert.Equal(t, want, levelText(level), level)
	}
	assert.Equal(t, "中", clip(" 中文 ", 4), "按字节截断不切开一个字")
	assert.Equal(t, "ab", clip("ab", 4))
}

// 推送了以后下载器被删掉：通知里写编号；一轮当中取消了上下文就停下。
func TestDownloaderNameAndCancelledRound(t *testing.T) {
	e := newEnv(t)
	e.create("1", nil)
	e.setSnap("1", v2.DiscountFree, v2.FreeUntil, 24*time.Hour)
	require.NoError(t, e.db.Delete(&models.DownloaderSetting{}, e.dlID).Error)
	e.svc.RunOnce(e.ctx)
	require.Len(t, e.msgs, 1)
	assert.Equal(t, fmt.Sprintf("下载器 %d", e.dlID), e.msgs[0].Downloader)

	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	require.NoError(t, e.db.Create(&models.DownloaderSetting{Name: "qb2", Type: "qbittorrent", URL: "http://y", Enabled: true}).Error)
	var dl models.DownloaderSetting
	require.NoError(t, e.db.Where("name = ?", "qb2").First(&dl).Error)
	e.dlID = dl.ID
	e.create("2", nil)
	assert.Zero(t, e.svc.RunOnce(ctx), "上下文取消了不再读")
}

// 库坏了：读写都报错或者只记日志，不 panic。
func TestDBErrors(t *testing.T) {
	e := newEnv(t)
	w := e.create("1", nil)
	closeTestGorm(e.db)
	_, err := e.svc.Create(e.ctx, CreateInput{Site: "hdtime", TorrentID: "2", DownloaderID: e.dlID})
	require.Error(t, err)
	_, _, err = e.svc.List(e.ctx, "")
	require.Error(t, err)
	_, err = e.svc.Cancel(e.ctx, w.ID)
	require.Error(t, err)
	_, err = e.svc.SetExpiry(e.ctx, w.ID, e.Now().Add(time.Hour))
	require.Error(t, err)
	assert.Zero(t, e.svc.RunOnce(e.ctx))
	e.svc.later(e.ctx, *w, "x")
	e.svc.failed(e.ctx, *w, "x")
	_, err = e.svc.finish(e.ctx, *w, models.FreeWatchCanceled, "x", "", false)
	require.Error(t, err)
	assert.Equal(t, "下载器 7", e.svc.downloaderName(e.ctx, 7))
}

// 库里的时间混着时区（网页改的到期时间是 UTC，时钟是本地时区）：到期与到点按真实时刻比，不按文本比。
// 按文本比时 "04:00+00:00" 排在 "11:00+08:00" 前面，没到期的会被当成到期。
func TestTimesComparedByInstant(t *testing.T) {
	e := newEnv(t)
	cst := time.FixedZone("CST", 8*3600)
	e.mu.Lock()
	e.now = e.now.In(cst)
	e.mu.Unlock()
	e.snap["1"] = &v2.PromotionSnapshot{Level: v2.DiscountNone}
	w := e.create("1", nil)
	require.Equal(t, 1, e.svc.RunOnce(e.ctx))
	at := e.Now().Add(time.Hour).UTC()
	_, err := e.svc.SetExpiry(e.ctx, w.ID, at)
	require.NoError(t, err)
	// 下次检查改到 10 分钟以前（存成 UTC），到期在一小时后（UTC）：这一轮要检查，不能当成到期
	require.NoError(t, e.db.Model(&models.FreeWatch{}).Where("id = ?", w.ID).Update("next_check_at", e.Now().Add(-10*time.Minute).UTC()).Error)
	assert.Equal(t, 1, e.svc.RunOnce(e.ctx))
	got := e.row(w.ID)
	assert.Equal(t, models.FreeWatchWatching, got.Status)
	assert.Equal(t, 2, got.CheckCount)
	e.advance(61 * time.Minute)
	assert.Zero(t, e.svc.RunOnce(e.ctx))
	assert.Equal(t, models.FreeWatchExpired, e.row(w.ID).Status)
}

// 库里留着别的时区写的时间（比如换过容器的时区，现在写的一律是本地时区）：+14:00 的文本排在本地时间的文本后面，
// 按文本比会把到点的当成没到点、把过了期的当成没到期。到点与到期按真实时刻比。
func TestTimesComparedByInstantAcrossZones(t *testing.T) {
	e := newEnv(t)
	far := time.FixedZone("+14", 14*3600)
	e.snap["1"] = &v2.PromotionSnapshot{Level: v2.DiscountNone}
	e.snap["2"] = &v2.PromotionSnapshot{Level: v2.DiscountNone}
	due := e.create("1", nil)
	gone := e.create("2", nil)
	require.Equal(t, 2, e.svc.RunOnce(e.ctx), "建好各看一次，30 分钟后再看")
	// 1 的下次检查是 10 分钟以前、2 的到期是一小时以前，都用 +14:00 写
	require.NoError(t, e.db.Model(&models.FreeWatch{}).Where("id = ?", due.ID).
		Update("next_check_at", e.Now().Add(-10*time.Minute).In(far)).Error)
	require.NoError(t, e.db.Model(&models.FreeWatch{}).Where("id = ?", gone.ID).
		Update("expires_at", e.Now().Add(-time.Hour).In(far)).Error)
	var raw []string
	require.NoError(t, e.db.Raw("SELECT next_check_at FROM free_watches WHERE id = ? UNION ALL SELECT expires_at FROM free_watches WHERE id = ?",
		due.ID, gone.ID).Scan(&raw).Error)
	for _, v := range raw {
		require.Contains(t, v, "+14:00", "库里存的是 +14:00 的文本（不然这条测试测不到什么）")
	}
	assert.Equal(t, 1, e.svc.RunOnce(e.ctx), "只有 1 到点了")
	assert.Equal(t, 2, e.row(due.ID).CheckCount)
	assert.Equal(t, models.FreeWatchExpired, e.row(gone.ID).Status)
}
