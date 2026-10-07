package reseed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// ---------- 假 IYUU ----------

type fakeIYUU struct {
	mu      sync.Mutex
	sites   []map[string]any
	results map[string]any // 源 hash → {torrent: [...]}
	// queryCodes 依次作为 /reseed/index/index 的 code 返回（用完后返回 0）
	queryCodes []int
	retryAfter int
	reports    int
	queries    int
}

func (f *fakeIYUU) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		write := func(code int, data any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "m", "data": data})
		}
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/reseed/sites/index":
			write(0, map[string]any{"sites": f.sites})
		case "/reseed/sites/reportExisting":
			f.reports++
			write(0, map[string]any{"sid_sha1": "sum-" + strings.Join(r.PostForm["sid_list[]"], ",")})
		case "/reseed/index/index":
			f.queries++
			if len(f.queryCodes) > 0 {
				code := f.queryCodes[0]
				f.queryCodes = f.queryCodes[1:]
				if code != 0 {
					write(code, map[string]any{"Retry-After": f.retryAfter})
					return
				}
			}
			var asked []string
			_ = json.Unmarshal([]byte(r.PostForm.Get("hash")), &asked)
			out := map[string]any{}
			for _, h := range asked {
				if v, ok := f.results[h]; ok {
					out[h] = v
				}
			}
			write(0, out)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------- 假下载器与站点 ----------

type fakeDL struct {
	downloader.Downloader
	torrents []downloader.Torrent
	files    map[string][]downloader.TorrentFile
}

func (f *fakeDL) GetAllTorrents() ([]downloader.Torrent, error) { return f.torrents, nil }

func (f *fakeDL) GetTorrentFiles(id string) ([]downloader.TorrentFile, error) {
	if fs, ok := f.files[id]; ok {
		return fs, nil
	}
	return nil, errors.New("no files")
}

type fakeSite struct {
	v2.Site
	data map[string][]byte
}

func (s *fakeSite) Download(_ context.Context, id string) ([]byte, error) {
	if d, ok := s.data[id]; ok {
		return d, nil
	}
	return nil, errors.New("404")
}

type plainCipher struct{}

func (plainCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (plainCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// torrentWith 生成一个多文件种子，返回内容与 info hash。
func torrentWith(t *testing.T, name string, files map[string]int64, salt string) ([]byte, string) {
	t.Helper()
	list := []map[string]any{}
	for p, size := range files {
		list = append(list, map[string]any{"length": size, "path": strings.Split(p, "/")})
	}
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"announce": "https://tracker.example/" + salt,
		"info":     map[string]any{"name": name, "files": list, "piece length": 16384, "pieces": "01234567890123456789", "source": salt},
	}))
	h, err := qbit.ComputeTorrentHash(buf.Bytes())
	require.NoError(t, err)
	return buf.Bytes(), h
}

type env struct {
	t     *testing.T
	db    *gorm.DB
	svc   *Service
	iyuu  *fakeIYUU
	dl    *fakeDL
	site  *fakeSite
	now   time.Time
	slept []time.Duration
	// 源种子
	srcHash string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "r.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ReseedSetting{}, &models.ReseedRecord{}, &models.TorrentTransferJob{},
		&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.DownloaderPathMap{}, &models.TransferRule{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{ID: 1, Name: "qb", Type: "qbittorrent", URL: "http://x", Enabled: true}).Error)
	e := &env{t: t, db: db, now: time.Date(2026, 10, 7, 10, 0, 0, 0, time.Local), site: &fakeSite{data: map[string][]byte{}}}
	e.iyuu = &fakeIYUU{sites: []map[string]any{
		{"id": 1, "site": "hdsky", "nickname": "天空", "base_url": "hdsky.me"},
		{"id": 2, "site": "ourbits", "nickname": "我堡", "base_url": "ourbits.club"},
		{"id": 3, "site": "nowhere", "nickname": "无", "base_url": "nowhere.example"},
	}, results: map[string]any{}}
	srv := e.iyuu.server(t)
	_, src := torrentWith(t, "Movie", map[string]int64{"a.mkv": 100, "sub/b.srt": 1}, "src")
	e.srcHash = src
	e.dl = &fakeDL{
		torrents: []downloader.Torrent{
			{ID: src, InfoHash: strings.ToUpper(src), Name: "Movie", SavePath: "/data/movies", TotalSize: 101, Progress: 1, IsCompleted: true, State: downloader.TorrentSeeding, Category: "movies"},
			{ID: "partial", InfoHash: "partial", Progress: 0.5, State: downloader.TorrentDownloading},
		},
		files: map[string][]downloader.TorrentFile{src: {{Name: "Movie/a.mkv", Size: 100}, {Name: "Movie\\sub\\b.srt", Size: 1}}},
	}
	dls := transfer.DownloadersFunc(func(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
		if id != 1 {
			return nil, models.DownloaderSetting{}, errors.New("下载器未启用")
		}
		return e.dl, models.DownloaderSetting{ID: 1, Name: "qb"}, nil
	})
	resolver := v2.NewTrackerResolverFrom(
		&v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}},
		&v2.SiteDefinition{ID: "ourbits", Schema: v2.SchemaNexusPHP, URLs: []string{"https://ourbits.club/"}},
	)
	tr := transfer.New(transfer.Config{
		DB: db, Downloaders: dls, Resolver: resolver, Now: func() time.Time { return e.now },
		Push: func(context.Context, ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error) {
			return &ptinternal.PushTorrentResult{Success: true}, nil
		},
	})
	e.svc = New(Config{
		DB: db, Cipher: plainCipher{}, Transfer: tr, Downloaders: dls,
		Sites: func(name string) (v2.Site, bool) {
			if name == "hdsky" {
				return e.site, true
			}
			return nil, false
		},
		SiteIDs:  func() []string { return []string{"hdsky"} },
		Resolver: resolver,
		NewClient: func(token string) *iyuu.Client {
			c := iyuu.New(token)
			c.BaseURL = srv.URL
			return c
		},
		Now: func() time.Time { return e.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			e.slept = append(e.slept, d)
			return nil
		},
	})
	return e
}

func (e *env) enable(mod func(*SettingsUpdate)) {
	e.t.Helper()
	tok := "tok"
	u := SettingsUpdate{Enabled: true, Token: &tok}
	if mod != nil {
		mod(&u)
	}
	_, err := e.svc.SaveSettings(context.Background(), u)
	require.NoError(e.t, err)
}

// candidate 在 hdsky 上放一个可辅种的种子（files 与源一致时能通过核对），返回它的 info hash。
func (e *env) candidate(id string, files map[string]int64) string {
	data, h := torrentWith(e.t, "Movie", files, "c"+id)
	e.site.data[id] = data
	return h
}

func (e *env) setResults(cands ...map[string]any) {
	e.iyuu.results[e.srcHash] = map[string]any{"torrent": cands}
}

// ---------- 测试 ----------

func TestSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.False(t, got.HasToken)
	assert.Equal(t, models.ReseedDefaultIntervalHours, got.IntervalHours)
	assert.Equal(t, models.ReseedDefaultMaxPerSitePerDay, got.MaxPerSitePerDay)

	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true})
	assert.ErrorIs(t, err, ErrInvalid, "开启要先填 token")
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{IntervalHours: 200})
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{MaxPerSitePerDay: -1})
	assert.ErrorIs(t, err, ErrInvalid)

	tok := " tok "
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true, Token: &tok, IntervalHours: 6, DownloaderIDs: []uint{1, 1, 0}, SiteNames: []string{"hdsky", " hdsky"}, MaxPerSitePerDay: 3})
	require.NoError(t, err)
	assert.True(t, got.HasToken)
	assert.Equal(t, []uint{1}, got.DownloaderIDs)
	assert.Equal(t, []string{"hdsky"}, got.SiteNames)
	var row models.ReseedSetting
	require.NoError(t, e.db.First(&row, 1).Error)
	assert.Equal(t, "enc:tok", row.TokenEncrypted, "token 加密保存")

	// 不带 token 修改：保留原来的
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: false, IntervalHours: 6})
	require.NoError(t, err)
	assert.True(t, got.HasToken)
	assert.False(t, got.Enabled, "关掉的开关原样保存")
	empty := ""
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{Token: &empty})
	require.NoError(t, err)
	assert.False(t, got.HasToken, "空串清除 token")

	assert.False(t, e.svc.Due(ctx), "没开启")
	e.enable(nil)
	assert.True(t, e.svc.Due(ctx), "没运行过")
	require.NoError(t, e.svc.RecordRun(ctx, e.now, "ok"))
	assert.False(t, e.svc.Due(ctx))
	e.now = e.now.Add(13 * time.Hour)
	assert.True(t, e.svc.Due(ctx))
}

func TestSiteMap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.SiteMap(ctx)
	assert.ErrorIs(t, err, iyuu.ErrNoToken)
	e.enable(nil)
	items, err := e.svc.SiteMap(ctx)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, SiteMapItem{SID: 1, IYUUSite: "hdsky", Nickname: "天空", Host: "hdsky.me", SiteName: "hdsky", Configured: true, Selected: true}, items[0])
	byName := map[string]SiteMapItem{}
	for _, it := range items {
		byName[it.IYUUSite] = it
	}
	assert.Equal(t, "ourbits", byName["ourbits"].SiteName, "认得出来")
	assert.False(t, byName["ourbits"].Configured, "pt-tools 里没配置")
	assert.Empty(t, byName["nowhere"].SiteName)

	e.enable(func(u *SettingsUpdate) { u.SiteNames = []string{"ourbits"} })
	items, err = e.svc.SiteMap(ctx)
	require.NoError(t, err)
	assert.False(t, items[0].Selected, "选了别的站点时 hdsky 不在范围内")
}

// 一轮辅种：文件一致的建任务、文件不一致的记失败、站点不在范围 / 下载器里已有的跳过；第二轮不重复尝试。
func TestRun(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("11", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	bad := e.candidate("12", map[string]int64{"a.mkv": 100})
	e.setResults(
		map[string]any{"sid": 1, "torrent_id": 11, "info_hash": strings.ToUpper(good)},
		map[string]any{"sid": 1, "torrent_id": "12", "info_hash": bad},
		map[string]any{"sid": 2, "torrent_id": 13, "info_hash": "ffff"},
		map[string]any{"sid": 1, "torrent_id": 14, "info_hash": "partial"},
		map[string]any{"sid": 1, "torrent_id": 15, "info_hash": "eeee"},
	)
	res, err := e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Downloaders)
	assert.Equal(t, 1, res.Hashes, "只查已经下完的种子")
	assert.Equal(t, 5, res.Candidates)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 2, res.Failed, "文件不一致、下载失败")
	assert.Equal(t, 1, res.Skipped[SkipNotSelected])
	assert.Equal(t, 1, res.Skipped[SkipExists])
	assert.Contains(t, res.Summary(), "加入 1 个")

	var jobs []models.TorrentTransferJob
	require.NoError(t, e.db.Find(&jobs).Error)
	require.Len(t, jobs, 1)
	j := jobs[0]
	assert.Equal(t, models.JobKindReseed, j.Kind)
	assert.Equal(t, good, j.InfoHash)
	assert.Equal(t, models.TransferExported, j.State)
	assert.Equal(t, "/data/movies", j.TargetSavePath)
	assert.Equal(t, "hdsky", j.Tags)
	assert.Equal(t, "11", j.TorrentID)

	recs, err := e.svc.Records(ctx)
	require.NoError(t, err)
	require.Len(t, recs, 3)
	states := map[string]RecordView{}
	for _, r := range recs {
		states[r.TorrentID] = r
	}
	assert.Equal(t, models.ReseedQueued, states["11"].State)
	assert.Equal(t, models.TransferExported, states["11"].JobState)
	assert.Contains(t, states["12"].Message, "文件列表与原种子不一致")
	assert.Contains(t, states["15"].Message, "下载种子失败")

	res, err = e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Zero(t, res.Created)
	assert.Equal(t, 3, res.Skipped[SkipTried], "同一站点同一种子只尝试一次")
	assert.Equal(t, 1, e.iyuu.reports, "7 天内站点没变：sid_sha1 用缓存的")
}

// 每个站点每天有上限；过了午夜重新计数。
func TestRunDailyLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.MaxPerSitePerDay = 1 })
	a := e.candidate("21", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	b := e.candidate("22", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 21, "info_hash": a}, map[string]any{"sid": 1, "torrent_id": 22, "info_hash": b})
	res, err := e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 1, res.Skipped[SkipDailyLimit])
	e.now = e.now.Add(24 * time.Hour)
	res, err = e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "第二天接着加")
}

// 限流：等待不长时等一次再查；太长就停下这一轮。IYUU 报错时重新报告站点再查一次。
func TestRunRateLimitAndRefresh(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	e.enable(nil)
	good := e.candidate("31", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 31, "info_hash": good})
	e.iyuu.queryCodes, e.iyuu.retryAfter = []int{429}, 5
	res, err := e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{5 * time.Second}, e.slept)
	assert.Equal(t, 1, res.Created)

	e2 := newEnv(t)
	e2.enable(nil)
	e2.iyuu.queryCodes, e2.iyuu.retryAfter = []int{429}, 600
	res, err = e2.svc.Run(ctx)
	require.NoError(t, err)
	assert.Contains(t, res.Stopped, "600 秒后再试")
	assert.Empty(t, e2.slept, "等太久就不等")

	e3 := newEnv(t)
	e3.enable(nil)
	g3 := e3.candidate("41", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e3.setResults(map[string]any{"sid": 1, "torrent_id": 41, "info_hash": g3})
	e3.iyuu.queryCodes = []int{400}
	res, err = e3.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, e3.iyuu.reports, "报错后重新报告一次站点")
	assert.Equal(t, 1, res.Created)

	e4 := newEnv(t)
	e4.enable(nil)
	e4.iyuu.queryCodes = []int{400, 400}
	res, err = e4.svc.Run(ctx)
	require.NoError(t, err)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "查询 IYUU 失败")
}

func TestRunPreconditions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	_, err := e.svc.Run(ctx)
	assert.ErrorIs(t, err, iyuu.ErrNoToken)
	e.enable(func(u *SettingsUpdate) { u.SiteNames = []string{"ourbits"} })
	_, err = e.svc.Run(ctx)
	assert.ErrorIs(t, err, ErrNoSites, "选中的站点 pt-tools 里没配置")

	e2 := newEnv(t)
	e2.enable(func(u *SettingsUpdate) { u.DownloaderIDs = []uint{1, 9} })
	res, err := e2.svc.Run(ctx)
	require.NoError(t, err)
	require.Len(t, res.Errors, 1)
	assert.Contains(t, res.Errors[0], "下载器 9 不可用")
	assert.Equal(t, 1, res.Downloaders)
}

func TestFiles(t *testing.T) {
	data, _ := torrentWith(t, "Movie", map[string]int64{"a.mkv": 100, "sub/b.srt": 1}, "x")
	got, err := TorrentFiles(data)
	require.NoError(t, err)
	same, _ := SameFiles(got, []FileEntry{{Path: "Movie/sub/b.srt", Size: 1}, {Path: "Movie/a.mkv", Size: 100}})
	assert.True(t, same, "顺序无关")
	same, why := SameFiles(got, []FileEntry{{Path: "Movie/a.mkv", Size: 100}, {Path: "Movie/sub/b.srt", Size: 2}})
	assert.False(t, same)
	assert.Contains(t, why, "大小不同")
	same, why = SameFiles(got, []FileEntry{{Path: "Other/a.mkv", Size: 100}, {Path: "Movie/sub/b.srt", Size: 1}})
	assert.False(t, same)
	assert.Contains(t, why, "文件不同")
	same, why = SameFiles(got, nil)
	assert.False(t, same)
	assert.Contains(t, why, "文件数不同")

	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "one.iso", "length": 7}}))
	single, err := TorrentFiles(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, []FileEntry{{Path: "one.iso", Size: 7}}, single)
	_, err = TorrentFiles([]byte("nope"))
	assert.Error(t, err)
	buf.Reset()
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"length": 7}}))
	_, err = TorrentFiles(buf.Bytes())
	assert.Error(t, err)
}

// 下载种子失败（站点暂时不可用等）7 天后可以再试；核对没通过的不再试。
func TestRunRetriesDownloadFailuresLater(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("51", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	data := e.site.data["51"]
	delete(e.site.data, "51") // 站点暂时下载不到
	bad := e.candidate("52", map[string]int64{"a.mkv": 100})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 51, "info_hash": good}, map[string]any{"sid": 1, "torrent_id": 52, "info_hash": bad})
	res, err := e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Failed)

	e.site.data["51"] = data
	res, err = e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Skipped[SkipTried], "7 天之内不重试")

	e.now = e.now.Add(8 * 24 * time.Hour)
	res, err = e.svc.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "下载失败的过了 7 天再试，这次成功")
	assert.Equal(t, 1, res.Skipped[SkipTried], "文件不一致的不再试")
	var recs []models.ReseedRecord
	require.NoError(t, e.db.Order("torrent_id").Find(&recs).Error)
	require.Len(t, recs, 2, "重试更新原来那条记录")
	assert.Equal(t, models.ReseedQueued, recs[0].State)
	assert.False(t, recs[0].Retryable)
	assert.Equal(t, models.ReseedFailed, recs[1].State)
}
