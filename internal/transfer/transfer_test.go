package transfer

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// ---------- 假下载器 ----------

// fakeDL 是内存里的下载器：只实现转移用到的几个方法，别的方法调用会 panic。
type fakeDL struct {
	downloader.Downloader
	mu       sync.Mutex
	torrents map[string]*downloader.Torrent
	files    map[string][]byte
	removed  []string // "hash:removeData"
	resumed  []string
	recheck  []string
	listErr  error
	// afterRecheck 决定校验后的进度与状态（默认 1.0、暂停）
	afterRecheck func(t *downloader.Torrent)
}

func newFakeDL() *fakeDL {
	return &fakeDL{torrents: map[string]*downloader.Torrent{}, files: map[string][]byte{}}
}

func (f *fakeDL) put(t downloader.Torrent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t.ID == "" {
		t.ID = t.InfoHash
	}
	f.torrents[t.InfoHash] = &t
}

func (f *fakeDL) get(hash string) (downloader.Torrent, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.torrents[hash]
	if !ok {
		return downloader.Torrent{}, false
	}
	return *t, true
}

func (f *fakeDL) set(hash string, fn func(t *downloader.Torrent)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.torrents[hash])
}

func (f *fakeDL) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := []downloader.Torrent{}
	for _, h := range filter.Hashes {
		if t, ok := f.torrents[h]; ok {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []downloader.Torrent{}
	for _, t := range f.torrents {
		out = append(out, *t)
	}
	return out, nil
}

func (f *fakeDL) CheckTorrentExists(hash string) (bool, error) {
	_, ok := f.get(hash)
	return ok, nil
}

func (f *fakeDL) RecheckTorrent(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recheck = append(f.recheck, id)
	t := f.torrents[id]
	t.State = downloader.TorrentChecking
	return nil
}

func (f *fakeDL) finishCheck(hash string) {
	f.set(hash, func(t *downloader.Torrent) {
		t.State, t.Progress, t.IsCompleted = downloader.TorrentPaused, 1, true
		if f.afterRecheck != nil {
			f.afterRecheck(t)
		}
	})
}

func (f *fakeDL) ResumeTorrent(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resumed = append(f.resumed, id)
	f.torrents[id].State = downloader.TorrentSeeding
	return nil
}

func (f *fakeDL) RemoveTorrent(id string, removeData bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.torrents[id]; !ok {
		return downloader.ErrTorrentNotFound
	}
	delete(f.torrents, id)
	f.removed = append(f.removed, id+map[bool]string{true: ":data", false: ":keep"}[removeData])
	return nil
}

// exporterDL 能导出种子（像 qBittorrent）。
type exporterDL struct {
	*fakeDL
	exportErr error
	onExport  func()
}

func (e *exporterDL) ExportTorrent(_ context.Context, hash string) ([]byte, error) {
	if e.onExport != nil {
		e.onExport()
	}
	if e.exportErr != nil {
		return nil, e.exportErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	data, ok := e.files[hash]
	if !ok {
		return nil, downloader.ErrTorrentNotFound
	}
	return data, nil
}

// fakeSite 只实现 Download。
type fakeSite struct {
	v2.Site
	data map[string][]byte
}

func (s *fakeSite) Download(_ context.Context, torrentID string) ([]byte, error) {
	if d, ok := s.data[torrentID]; ok {
		return d, nil
	}
	return nil, errors.New("404")
}

// ---------- 测试环境 ----------

type env struct {
	t        *testing.T
	db       *gorm.DB
	svc      *Service
	src      *exporterDL
	srcTR    *fakeDL // 不能导出（像 Transmission）
	dst      *fakeDL
	now      time.Time
	pushes   []ptinternal.PushTorrentRequest
	pushRes  *ptinternal.PushTorrentResult
	pushErr  error
	addOnErr bool
	site     *fakeSite
	// onPush 不为空时代替默认的推送行为
	onPush func(req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error)
}

const (
	srcID   = 1
	dstID   = 2
	srcTRID = 3
	offID   = 4
)

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.DownloaderPathMap{}, &models.TransferRule{}, &models.TorrentInfo{}))
	e := &env{
		t: t, db: db, src: &exporterDL{fakeDL: newFakeDL()}, srcTR: newFakeDL(), dst: newFakeDL(),
		now: time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC), site: &fakeSite{data: map[string][]byte{}},
	}
	dls := map[uint]struct {
		dl  downloader.Downloader
		set models.DownloaderSetting
	}{
		srcID:   {e.src, models.DownloaderSetting{ID: srcID, Name: "qb-src"}},
		dstID:   {e.dst, models.DownloaderSetting{ID: dstID, Name: "tr-dst"}},
		srcTRID: {e.srcTR, models.DownloaderSetting{ID: srcTRID, Name: "tr-src"}},
	}
	e.svc = New(Config{
		DB: db,
		Downloaders: DownloadersFunc(func(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
			d, ok := dls[id]
			if !ok {
				return nil, models.DownloaderSetting{}, errors.New("下载器未启用")
			}
			return d.dl, d.set, nil
		}),
		Sites: func(name string) (v2.Site, bool) {
			if name == "hdsky" {
				return e.site, true
			}
			return nil, false
		},
		Push: func(_ context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
			e.pushes = append(e.pushes, req)
			if e.onPush != nil {
				return e.onPush(req)
			}
			if e.pushErr != nil {
				if e.addOnErr {
					e.addToTarget(req)
				}
				return nil, e.pushErr
			}
			if e.pushRes != nil {
				return e.pushRes, nil
			}
			e.addToTarget(req)
			return &ptinternal.PushTorrentResult{Success: true}, nil
		},
		Resolver: v2.NewTrackerResolverFrom(&v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}}),
		Now:      func() time.Time { return e.now },
	})
	return e
}

func (e *env) addToTarget(req ptinternal.PushTorrentRequest) {
	h, err := qbit.ComputeTorrentHash(req.TorrentData)
	require.NoError(e.t, err)
	e.dst.put(downloader.Torrent{InfoHash: h, Name: req.Title, SavePath: req.SavePath, State: downloader.TorrentPaused, Tags: req.Tags})
}

// seed 在源下载器里放一个已下完的种子，返回 hash。
func (e *env) seed(dl *fakeDL, name string, size int64, done bool) string {
	e.t.Helper()
	data := torrentFile(e.t, name)
	h, err := qbit.ComputeTorrentHash(data)
	require.NoError(e.t, err)
	p := 1.0
	if !done {
		p = 0.5
	}
	dl.put(downloader.Torrent{
		InfoHash: h, Name: name, TotalSize: size, SavePath: "/downloads/movies", Progress: p, IsCompleted: done,
		State: downloader.TorrentSeeding, Category: "movies", Tags: "hdsky,4k", Tracker: "https://tracker.hdsky.me/announce.php?passkey=x",
	})
	dl.mu.Lock()
	dl.files[h] = data
	dl.mu.Unlock()
	return h
}

func torrentFile(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"announce": "https://tracker.hdsky.me/announce.php?passkey=x",
		"info":     map[string]any{"name": name, "length": 1 << 20, "piece length": 16384, "pieces": "01234567890123456789"},
	}))
	return buf.Bytes()
}

func (e *env) job(id uint) models.TorrentTransferJob {
	e.t.Helper()
	var j models.TorrentTransferJob
	require.NoError(e.t, e.db.First(&j, id).Error)
	return j
}

func (e *env) create(items ...Item) []models.TorrentTransferJob {
	e.t.Helper()
	jobs, _, err := e.svc.Create(context.Background(), dstID, items, nil)
	require.NoError(e.t, err)
	return jobs
}

// ---------- 测试 ----------

func TestPreview(t *testing.T) {
	e := newEnv(t)
	ok := e.seed(e.src.fakeDL, "Movie.A", 10<<30, true)
	partial := e.seed(e.src.fakeDL, "Movie.B", 1<<30, false)
	inTarget := e.seed(e.src.fakeDL, "Movie.C", 1<<30, true)
	e.dst.put(downloader.Torrent{InfoHash: inTarget})
	busy := e.seed(e.src.fakeDL, "Movie.D", 1<<30, true)
	require.NoError(t, e.db.Create(&models.TorrentTransferJob{InfoHash: busy, State: models.TransferChecking}).Error)
	trWithRecord := e.seed(e.srcTR, "Show.E", 1<<30, true)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "77", TorrentHash: &trWithRecord}).Error)
	trNoRecord := e.seed(e.srcTR, "Show.F", 1<<30, true)
	require.NoError(t, e.db.Create(&models.DownloaderPathMap{SourceDownloaderID: srcID, TargetDownloaderID: dstID, SourcePrefix: "/downloads", TargetPrefix: "/data"}).Error)

	rows, err := e.svc.Preview(context.Background(), dstID, []Item{
		{srcID, ok},
		{srcID, partial},
		{srcID, inTarget},
		{srcID, busy},
		{srcID, "ffff"},
		{srcTRID, trWithRecord},
		{srcTRID, trNoRecord},
		{dstID, "eeee"},
		{offID, "dddd"},
		{srcID, ok}, // 重复的只算一次
	})
	require.NoError(t, err)
	require.Len(t, rows, 9)
	by := map[string]PreviewItem{}
	for _, r := range rows {
		by[r.Hash] = r
	}
	assert.True(t, by[ok].OK)
	assert.Equal(t, SourceExport, by[ok].Source)
	assert.Equal(t, "/data/movies", by[ok].TargetPath)
	assert.True(t, by[ok].Mapped)
	assert.Equal(t, "hdsky", by[ok].SiteName, "没有记录时按 tracker 认出站点")
	assert.Equal(t, "qb-src", by[ok].SourceName)
	assert.Equal(t, "还没下载完", by[partial].Reason)
	assert.Equal(t, "目标下载器里已经有这个种子", by[inTarget].Reason)
	assert.Equal(t, "已经有进行中的转移任务", by[busy].Reason)
	assert.Equal(t, "源下载器里没有这个种子", by["ffff"].Reason)
	assert.True(t, by[trWithRecord].OK)
	assert.Equal(t, SourceSite, by[trWithRecord].Source)
	assert.Equal(t, "/downloads/movies", by[trWithRecord].TargetPath, "没有映射时路径不变")
	assert.False(t, by[trWithRecord].Mapped)
	assert.Contains(t, by[trNoRecord].Reason, "拿不到种子文件")
	assert.Equal(t, "源和目标是同一台下载器", by["eeee"].Reason)
	assert.Contains(t, by["dddd"].Reason, "源下载器不可用")

	_, err = e.svc.Preview(context.Background(), dstID, nil)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Preview(context.Background(), offID, []Item{{srcID, ok}})
	assert.ErrorIs(t, err, ErrTargetUnavailable)
	assert.ErrorContains(t, err, "下载器未启用")
	many := make([]Item, MaxItems+1)
	for i := range many {
		many[i] = Item{srcID, string(rune('a'+i%26)) + time.Duration(i).String()}
	}
	_, err = e.svc.Preview(context.Background(), dstID, many)
	assert.ErrorIs(t, err, ErrInvalid)
}

// 一路走完：导出 → 暂停加入目标（路径按映射）→ 校验 → 恢复目标 → 从源移除（不删数据）→ 种子记录改到目标。
func TestTransferHappyPath(t *testing.T) {
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 10<<30, true)
	require.NoError(t, e.db.Create(&models.DownloaderPathMap{SourceDownloaderID: srcID, TargetDownloaderID: dstID, SourcePrefix: "/downloads", TargetPrefix: "/data"}).Error)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "9", TorrentHash: &h, DownloaderName: "qb-src"}).Error)
	jobs := e.create(Item{srcID, h})
	require.Len(t, jobs, 1)
	id := jobs[0].ID
	assert.Equal(t, models.TransferPending, jobs[0].State)
	assert.Equal(t, "9", jobs[0].TorrentID)

	ctx := context.Background()
	assert.Equal(t, 1, e.svc.RunOnce(ctx))
	j := e.job(id)
	assert.Equal(t, models.TransferExported, j.State)
	assert.NotEmpty(t, j.TorrentData)

	e.svc.RunOnce(ctx)
	j = e.job(id)
	require.Equal(t, models.TransferChecking, j.State, j.Message)
	require.Len(t, e.pushes, 1)
	p := e.pushes[0]
	assert.True(t, p.ReuseExistingData)
	assert.Equal(t, "/data/movies", p.SavePath)
	assert.Equal(t, TransferSource, p.Source)
	assert.Equal(t, "hdsky", p.SiteID)
	assert.Equal(t, "movies", p.Category)
	assert.Equal(t, "hdsky,4k,"+models.TransferTag, p.Tags, "原有标签加上转移标签")
	require.NotNil(t, j.Deadline)
	assert.Equal(t, e.now.Add(30*time.Minute+512*time.Second), *j.Deadline, "30 分钟 + 10 GiB / 20 MiB/s")

	e.svc.RunOnce(ctx) // 让目标校验
	assert.Equal(t, []string{h}, e.dst.recheck)
	e.now = e.now.Add(time.Minute)
	e.svc.RunOnce(ctx) // 还在校验
	assert.Equal(t, models.TransferChecking, e.job(id).State)
	assert.Contains(t, e.job(id).Message, "正在校验")

	e.dst.finishCheck(h)
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferVerified, e.job(id).State)
	e.svc.RunOnce(ctx)
	j = e.job(id)
	assert.Equal(t, models.TransferDone, j.State, j.Message)
	assert.Empty(t, j.TorrentData, "结束后清掉种子文件")
	assert.NotNil(t, j.FinishedAt)
	assert.Equal(t, []string{h}, e.dst.resumed)
	assert.Equal(t, []string{h + ":keep"}, e.src.removed, "从源移除，数据保留")
	var rec models.TorrentInfo
	require.NoError(t, e.db.Where("torrent_id = ?", "9").First(&rec).Error)
	assert.Equal(t, "tr-dst", rec.DownloaderName)
	require.NotNil(t, rec.DownloaderID)
	assert.EqualValues(t, dstID, *rec.DownloaderID)
	assert.Zero(t, e.svc.RunOnce(ctx), "结束的任务不再处理")
}

// 校验没到 100%：等过宽限时间才认定，然后从目标移除（不删数据），源不动。
func TestTransferRollbackWhenIncomplete(t *testing.T) {
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	ctx := context.Background()
	e.svc.RunOnce(ctx) // 导出
	e.svc.RunOnce(ctx) // 加入
	e.svc.RunOnce(ctx) // 让目标校验
	e.dst.afterRecheck = func(t *downloader.Torrent) { t.Progress, t.IsCompleted = 0.42, false }
	e.dst.finishCheck(h)
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferChecking, e.job(id).State, "宽限时间之内再等等")
	e.now = e.now.Add(3 * time.Minute)
	e.svc.RunOnce(ctx)
	j := e.job(id)
	assert.Equal(t, models.TransferRolledBack, j.State)
	assert.Contains(t, j.Message, "42.0%")
	assert.Equal(t, []string{h + ":keep"}, e.dst.removed)
	assert.Empty(t, e.src.removed, "源不动")
	_, still := e.src.get(h)
	assert.True(t, still)
}

// 校验一直不结束：过了时限回滚。
func TestTransferRollbackOnTimeout(t *testing.T) {
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	ctx := context.Background()
	for range 3 {
		e.svc.RunOnce(ctx)
	}
	e.now = e.now.Add(31*time.Minute + time.Minute)
	e.svc.RunOnce(ctx)
	j := e.job(id)
	assert.Equal(t, models.TransferRolledBack, j.State)
	assert.Contains(t, j.Message, "超过时限")
}

// 进程在推送途中退出：重启后目标里有就接着校验，没有就重新推送。
func TestTransferResumesAfterRestart(t *testing.T) {
	e := newEnv(t)
	added := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	notAdded := e.seed(e.src.fakeDL, "Movie.B", 1<<30, true)
	data := torrentFile(t, "Movie.B")
	e.dst.put(downloader.Torrent{InfoHash: added, State: downloader.TorrentPaused, Tags: models.TransferTag}) // 退出前已经加进去了
	j1 := models.TorrentTransferJob{SourceDownloaderID: srcID, TargetDownloaderID: dstID, InfoHash: added, State: models.TransferAdding}
	j2 := models.TorrentTransferJob{SourceDownloaderID: srcID, TargetDownloaderID: 99, InfoHash: notAdded, State: models.TransferAdding, TorrentData: data}
	require.NoError(t, e.db.Create(&j1).Error)
	require.NoError(t, e.db.Create(&j2).Error)
	e.svc.RunOnce(context.Background())
	assert.Equal(t, models.TransferChecking, e.job(j1.ID).State)
	assert.Equal(t, models.TransferAdding, e.job(j2.ID).State, "目标下载器不可用时留着下一轮再试")
	assert.Contains(t, e.job(j2.ID).Message, "目标下载器不可用")
	require.NoError(t, e.db.Model(&j2).Update("target_downloader_id", dstID).Error)
	e.svc.RunOnce(context.Background())
	assert.Equal(t, models.TransferExported, e.job(j2.ID).State, "目标里没有：重新推送")
}

// 同一台目标下载器同一时间只有一个任务在加入或校验。
func TestTransferOneCheckPerTarget(t *testing.T) {
	e := newEnv(t)
	a := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	b := e.seed(e.src.fakeDL, "Movie.B", 1<<30, true)
	jobs := e.create(Item{srcID, a}, Item{srcID, b})
	require.Len(t, jobs, 2)
	ctx := context.Background()
	e.svc.RunOnce(ctx) // 两个都导出
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferChecking, e.job(jobs[0].ID).State)
	assert.Equal(t, models.TransferExported, e.job(jobs[1].ID).State, "等前一个校验完")
	e.svc.RunOnce(ctx)
	e.dst.finishCheck(a)
	e.svc.RunOnce(ctx) // a verified
	e.svc.RunOnce(ctx) // a done，b 开始
	assert.Equal(t, models.TransferDone, e.job(jobs[0].ID).State)
	assert.Equal(t, models.TransferChecking, e.job(jobs[1].ID).State)
}

// 推送被拒（如站点容量已满）、推送报错：任务失败，源不动；推送报错但其实加进去了：接着校验。
func TestTransferPushOutcomes(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	e.svc.RunOnce(ctx)
	e.pushRes = &ptinternal.PushTorrentResult{Success: false, Message: "站点容量已满"}
	e.svc.RunOnce(ctx)
	j := e.job(id)
	assert.Equal(t, models.TransferFailed, j.State)
	assert.Contains(t, j.Message, "站点容量已满")
	assert.Empty(t, e.src.removed)

	e2 := newEnv(t)
	h2 := e2.seed(e2.src.fakeDL, "Movie.B", 1<<30, true)
	id2 := e2.create(Item{srcID, h2})[0].ID
	e2.svc.RunOnce(ctx)
	e2.pushErr, e2.addOnErr = errors.New("timeout"), true
	e2.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferChecking, e2.job(id2).State, "其实已经加进去了")

	e3 := newEnv(t)
	h3 := e3.seed(e3.src.fakeDL, "Movie.C", 1<<30, true)
	id3 := e3.create(Item{srcID, h3})[0].ID
	e3.svc.RunOnce(ctx)
	e3.pushErr = errors.New("boom")
	e3.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e3.job(id3).State)
	assert.Contains(t, e3.job(id3).Message, "boom")

	e4 := newEnv(t)
	h4 := e4.seed(e4.src.fakeDL, "Movie.D", 1<<30, true)
	id4 := e4.create(Item{srcID, h4})[0].ID
	e4.svc.RunOnce(ctx)
	e4.dst.put(downloader.Torrent{InfoHash: h4}) // 导出之后有人把它加进了目标
	e4.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e4.job(id4).State)
	assert.Contains(t, e4.job(id4).Message, "已经有这个种子")
	assert.Empty(t, e4.pushes)
}

// 源不能导出时按 pt-tools 的记录从站点重新下载；拿到的种子对不上就失败。
func TestTransferSiteFallback(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.srcTR, "Show.E", 1<<30, true)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "77", TorrentHash: &h}).Error)
	e.site.data["77"] = torrentFile(t, "Show.E")
	id := e.create(Item{srcTRID, h})[0].ID
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferExported, e.job(id).State, e.job(id).Message)

	e2 := newEnv(t)
	h2 := e2.seed(e2.srcTR, "Show.F", 1<<30, true)
	require.NoError(t, e2.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "78", TorrentHash: &h2}).Error)
	e2.site.data["78"] = torrentFile(t, "Something.Else")
	id2 := e2.create(Item{srcTRID, h2})[0].ID
	e2.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e2.job(id2).State)
	assert.Contains(t, e2.job(id2).Message, "不符")

	// 导出失败、也没有记录：失败
	e3 := newEnv(t)
	h3 := e3.seed(e3.src.fakeDL, "Movie.G", 1<<30, true)
	id3 := e3.create(Item{srcID, h3})[0].ID
	e3.src.exportErr = downloader.ErrCapabilityUnsupported
	e3.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e3.job(id3).State)
	assert.Contains(t, e3.job(id3).Message, "导出种子失败")
}

// 源里的种子在导出前没了或者变成没下完：失败。源下载器连不上：留着下一轮再试。
func TestTransferExportChecksSource(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	gone := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	partial := e.seed(e.src.fakeDL, "Movie.B", 1<<30, true)
	jobs := e.create(Item{srcID, gone}, Item{srcID, partial})
	require.NoError(t, e.src.RemoveTorrent(gone, false))
	e.src.set(partial, func(t *downloader.Torrent) { t.Progress, t.IsCompleted = 0.3, false })
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e.job(jobs[0].ID).State)
	assert.Equal(t, "源种子还没下载完", e.job(jobs[1].ID).Message)

	e2 := newEnv(t)
	h := e2.seed(e2.src.fakeDL, "Movie.C", 1<<30, true)
	id := e2.create(Item{srcID, h})[0].ID
	e2.src.listErr = errors.New("connection refused")
	e2.svc.RunOnce(ctx)
	j := e2.job(id)
	assert.Equal(t, models.TransferPending, j.State)
	assert.Contains(t, j.Message, "connection refused")
}

// 只能取消还没加入目标的任务；导出期间被取消的，导出的结果不覆盖取消。
func TestTransferCancel(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	a := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	b := e.seed(e.src.fakeDL, "Movie.B", 1<<30, true)
	jobs := e.create(Item{srcID, a})
	require.NoError(t, e.svc.Cancel(ctx, jobs[0].ID))
	assert.Equal(t, models.TransferCanceled, e.job(jobs[0].ID).State)
	assert.ErrorIs(t, e.svc.Cancel(ctx, 999), ErrJobNotFound)

	jb := e.create(Item{srcID, b})[0]
	e.src.onExport = func() { require.NoError(t, e.svc.Cancel(ctx, jb.ID)) }
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferCanceled, e.job(jb.ID).State, "导出期间被取消")
	e.src.onExport = nil

	c := e.seed(e.src.fakeDL, "Movie.C", 1<<30, true)
	jc := e.create(Item{srcID, c})[0]
	e.svc.RunOnce(ctx)
	e.svc.RunOnce(ctx)
	require.Equal(t, models.TransferChecking, e.job(jc.ID).State)
	assert.ErrorIs(t, e.svc.Cancel(ctx, jc.ID), ErrNotCancelable)

	n, err := e.svc.ClearFinished(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	// 已经有进行中的任务：不重复建
	_, skipped, err := e.svc.Create(ctx, dstID, []Item{{srcID, c}}, nil)
	require.NoError(t, err)
	require.Len(t, skipped, 1)
	assert.Equal(t, "已经有进行中的转移任务", skipped[0].Reason)
}

// 校验完了目标里的种子却没了、恢复或从源移除失败。
func TestTransferFinishEdgeCases(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	for range 3 {
		e.svc.RunOnce(ctx)
	}
	e.dst.finishCheck(h)
	e.svc.RunOnce(ctx)
	require.Equal(t, models.TransferVerified, e.job(id).State)
	require.NoError(t, e.src.RemoveTorrent(h, false)) // 用户已经自己从源删了
	e.src.removed = nil
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferDone, e.job(id).State, "源里已经没有了也算完成")

	e2 := newEnv(t)
	h2 := e2.seed(e2.src.fakeDL, "Movie.B", 1<<30, true)
	id2 := e2.create(Item{srcID, h2})[0].ID
	for range 3 {
		e2.svc.RunOnce(ctx)
	}
	e2.dst.finishCheck(h2)
	e2.svc.RunOnce(ctx)
	require.NoError(t, e2.dst.RemoveTorrent(h2, false))
	e2.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e2.job(id2).State)
	_, still := e2.src.get(h2)
	assert.True(t, still, "源不动")

	// 加入之后目标里一直找不到：过了宽限失败
	e3 := newEnv(t)
	h3 := e3.seed(e3.src.fakeDL, "Movie.C", 1<<30, true)
	id3 := e3.create(Item{srcID, h3})[0].ID
	e3.svc.RunOnce(ctx)
	e3.svc.RunOnce(ctx)
	require.NoError(t, e3.dst.RemoveTorrent(h3, false))
	e3.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferChecking, e3.job(id3).State)
	e3.now = e3.now.Add(3 * time.Minute)
	e3.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e3.job(id3).State)
}

func TestCheckTimeout(t *testing.T) {
	assert.Equal(t, 30*time.Minute, checkTimeout(0))
	assert.Equal(t, 30*time.Minute, checkTimeout(-1))
	assert.Equal(t, 30*time.Minute+51*time.Second, checkTimeout(1<<30))
}

// 校验完成之后、收尾之前目标又开始校验或数据不完整：不收尾，退回校验；源绝不移除。
func TestTransferFinishRechecksTarget(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	for range 3 {
		e.svc.RunOnce(ctx)
	}
	e.dst.finishCheck(h)
	e.svc.RunOnce(ctx)
	require.Equal(t, models.TransferVerified, e.job(id).State)
	e.dst.set(h, func(t *downloader.Torrent) { t.Progress, t.IsCompleted = 0.6, false })
	e.svc.RunOnce(ctx)
	j := e.job(id)
	assert.Equal(t, models.TransferChecking, j.State, "目标不再是 100%：退回校验")
	assert.Empty(t, e.src.removed, "源不动")
	assert.Empty(t, e.dst.resumed, "也不恢复目标")
	e.dst.set(h, func(t *downloader.Torrent) { t.Progress, t.IsCompleted, t.State = 1, true, downloader.TorrentPaused })
	e.svc.RunOnce(ctx)
	e.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferDone, e.job(id).State)
}

// 加入目标时带上 pt-tools-transfer 标签；目标里同 hash 的种子没有这个标签时不是这次加的：不接管、不移除。
func TestTransferOwnership(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	id := e.create(Item{srcID, h})[0].ID
	e.svc.RunOnce(ctx)
	e.svc.RunOnce(ctx)
	require.Len(t, e.pushes, 1)
	assert.Contains(t, e.pushes[0].Tags, models.TransferTag)
	assert.Equal(t, models.TransferChecking, e.job(id).State)

	// 重启前停在 adding：目标里有同 hash 的种子，但不是带标签加的
	e2 := newEnv(t)
	h2 := e2.seed(e2.src.fakeDL, "Movie.B", 1<<30, true)
	e2.dst.put(downloader.Torrent{InfoHash: h2, Tags: "user"})
	j2 := models.TorrentTransferJob{SourceDownloaderID: srcID, TargetDownloaderID: dstID, InfoHash: h2, State: models.TransferAdding}
	require.NoError(t, e2.db.Create(&j2).Error)
	e2.svc.RunOnce(ctx)
	got := e2.job(j2.ID)
	assert.Equal(t, models.TransferFailed, got.State)
	assert.Contains(t, got.Message, "不是这次转移加的")
	_, still := e2.dst.get(h2)
	assert.True(t, still, "别人的种子不动")

	// 推送报错，目标里却有一个不带标签的同 hash 种子：不接管
	e3 := newEnv(t)
	h3 := e3.seed(e3.src.fakeDL, "Movie.C", 1<<30, true)
	id3 := e3.create(Item{srcID, h3})[0].ID
	e3.svc.RunOnce(ctx)
	e3.onPush = func(ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
		e3.dst.put(downloader.Torrent{InfoHash: h3, Tags: "user"})
		return nil, errors.New("timeout")
	}
	e3.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e3.job(id3).State)
	_, still = e3.dst.get(h3)
	assert.True(t, still)

	// 校验期间目标里的种子被换成了别人加的：失败、不移除
	e4 := newEnv(t)
	h4 := e4.seed(e4.src.fakeDL, "Movie.D", 1<<30, true)
	id4 := e4.create(Item{srcID, h4})[0].ID
	for range 3 {
		e4.svc.RunOnce(ctx)
	}
	e4.dst.set(h4, func(t *downloader.Torrent) { t.Tags = "user"; t.State, t.Progress = downloader.TorrentPaused, 0.1 })
	e4.now = e4.now.Add(10 * time.Minute)
	e4.svc.RunOnce(ctx)
	assert.Equal(t, models.TransferFailed, e4.job(id4).State)
	assert.Empty(t, e4.dst.removed, "不是这次加的不移除")
	assert.Empty(t, e4.src.removed)
}

// 完成后只改源下载器上的那条种子记录；别的下载器上的同 hash 记录不动。
func TestTransferFinishUpdatesSourceRecordsOnly(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h := e.seed(e.src.fakeDL, "Movie.A", 1<<30, true)
	src, other := uint(srcID), uint(77)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "hdsky", TorrentID: "1", TorrentHash: &h, DownloaderID: &src, DownloaderName: "qb-src"}).Error)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "ourbits", TorrentID: "2", TorrentHash: &h, DownloaderID: &other, DownloaderName: "elsewhere"}).Error)
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "audiences", TorrentID: "3", TorrentHash: &h, DownloaderName: "qb-src"}).Error)
	id := e.create(Item{srcID, h})[0].ID
	for range 3 {
		e.svc.RunOnce(ctx)
	}
	e.dst.finishCheck(h)
	e.svc.RunOnce(ctx)
	e.svc.RunOnce(ctx)
	require.Equal(t, models.TransferDone, e.job(id).State)
	var rows []models.TorrentInfo
	require.NoError(t, e.db.Order("torrent_id").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, "tr-dst", rows[0].DownloaderName, "源下载器上的记录改到目标")
	assert.Equal(t, "elsewhere", rows[1].DownloaderName, "别的下载器上的记录不动")
	assert.Equal(t, "tr-dst", rows[2].DownloaderName, "只有名称的旧记录按源下载器名称认")
}
