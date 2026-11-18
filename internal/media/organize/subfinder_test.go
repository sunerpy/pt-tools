package organize

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/csf"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// fakeCSF 是假的 ChineseSubFinder：认 Bearer csf-key；reply 为空时都回 ok。记下收到的 add-job。
type fakeCSF struct {
	*httptest.Server
	mu    sync.Mutex
	jobs  []map[string]any
	reply func(path string) (int, string)
}

func newFakeCSF(t *testing.T) *fakeCSF {
	f := &fakeCSF{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer csf-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"AccessToken Error"}`))
			return
		}
		switch r.URL.Path {
		case "/api/v1/job-status":
			_, _ = w.Write([]byte(`{"message":"job_id is empty"}`))
		case "/api/v1/add-job":
			b, _ := io.ReadAll(r.Body)
			var body map[string]any
			require.NoError(t, json.Unmarshal(b, &body))
			f.mu.Lock()
			f.jobs = append(f.jobs, body)
			reply := f.reply
			f.mu.Unlock()
			code, msg := http.StatusOK, `{"job_id":"j","message":"ok"}`
			if reply != nil {
				code, msg = reply(body["physical_video_file_full_path"].(string))
			}
			w.WriteHeader(code)
			_, _ = w.Write([]byte(msg))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeCSF) got() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.jobs...)
}

func TestChineseSubFinderMovie(t *testing.T) {
	e := newEnv(t)
	f := newFakeCSF(t)
	e.svc.cfg.CSFHTTP = f.Client()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, CSFURL: f.URL, CSFKey: ptr("csf-key"), CSFPath: "/csf/movies"})
	e.bareOpp()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, res.Messages)
	jobs := f.got()
	require.Len(t, jobs, 1)
	assert.Equal(t, map[string]any{
		"video_type": float64(csf.VideoMovie), "physical_video_file_full_path": "/csf/movies/奥本海默 (2023)/" + oppStem + ".mkv",
		"task_priority_level": float64(csf.Priority), "media_server_inside_video_id": "", "is_bluray": false,
	}, jobs[0])
	assert.Empty(t, e.history()[0].Message)

	// 再整理一次（库里已经有了）：不再交
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Len(t, f.got(), 1)
}

func TestChineseSubFinderSeason(t *testing.T) {
	e := newEnv(t)
	f := newFakeCSF(t)
	e.svc.cfg.CSFHTTP = f.Client()
	e.settings(SettingsInput{MinVideoMB: 1})
	// 没填「ChineseSubFinder 里的库目录」：路径和 pt-tools 里的一样
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, CSFURL: f.URL + "/", CSFKey: ptr("csf-key")})
	e.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
		tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
	f.reply = func(p string) (int, string) {
		if strings.Contains(p, "S01E02") {
			return http.StatusOK, `{"message":"physical video file not found"}`
		}
		// 交的时候这一集的 NFO 已经写好了（刮削在前面）
		assert.True(t, exists(strings.TrimSuffix(p, filepath.Ext(p))+".nfo"), p)
		return http.StatusOK, `{"job_id":"j1","message":"job is already in queue"}`
	}
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created)
	jobs := f.got()
	require.Len(t, jobs, 2)
	for _, j := range jobs {
		assert.Equal(t, float64(csf.VideoSeries), j["video_type"])
		assert.True(t, strings.HasPrefix(j["physical_video_file_full_path"].(string), e.tv+string(filepath.Separator)))
	}
	byEp := map[int]models.MediaTransferHistory{}
	for _, r := range e.history() {
		byEp[r.Episode] = r
	}
	assert.Empty(t, byEp[1].Message)
	assert.Contains(t, byEp[2].Message, csf.ErrFileNotFound.Error()+"：")
	assert.Contains(t, byEp[2].Message, "看看媒体库里「ChineseSubFinder 里的库目录」填得对不对")
	require.Len(t, res.Messages, 1)
	assert.True(t, strings.HasPrefix(res.Messages[0], filepath.Base(byEp[2].TargetPath)+"："+csf.ErrFileNotFound.Error()), res.Messages[0])
}

func TestChineseSubFinderStopsWhenUnavailable(t *testing.T) {
	e := newEnv(t)
	f := newFakeCSF(t)
	e.svc.cfg.CSFHTTP = f.Client()
	e.settings(SettingsInput{MinVideoMB: 1})
	// 剧集没开刮削：ChineseSubFinder 读不到 NFO 时写上原因
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, CSFURL: f.URL, CSFKey: ptr("csf-key")})
	e.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
		tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
	f.reply = func(string) (int, string) {
		return http.StatusInternalServerError, `{"message":"open tvshow.nfo: no such file"}`
	}
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Len(t, f.got(), 2, "这种错误只是这一个视频的，接着交下一个")
	for _, r := range e.history() {
		assert.Contains(t, r.Message, "no such file")
		assert.Contains(t, r.Message, "剧集要开着刮削")
	}

	// 连不上：第一个失败后面的不再试，都写上
	e2 := newEnv(t)
	e2.settings(SettingsInput{MinVideoMB: 1})
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	e2.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e2.tv, CSFURL: deadURL, CSFKey: ptr("csf-key")})
	e2.addTorrent(tlouHash, tlouName, map[string]int{
		tlouName + "/The.Last.of.Us.S01E01.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
		tlouName + "/The.Last.of.Us.S01E02.2160p.WEB-DL.DV.H.265-HHWEB.mkv": 5,
	}, func(t *downloader.Torrent) { t.Category = "tv" })
	res, err := e2.svc.Organize(e2.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created)
	require.Len(t, res.Messages, 1)
	assert.True(t, strings.HasPrefix(res.Messages[0], csf.ErrUnavailable.Error()), res.Messages[0])
	for _, r := range e2.history() {
		assert.Contains(t, r.Message, csf.ErrUnavailable.Error())
		assert.NotContains(t, r.Message, "剧集要开着刮削", "连不上不是 NFO 的问题")
		assert.Equal(t, models.MediaTransferDone, r.Status)
	}

	// Key 不对：不再试，写上去哪改
	e3 := newEnv(t)
	f3 := newFakeCSF(t)
	e3.svc.cfg.CSFHTTP = f3.Client()
	e3.settings(SettingsInput{MinVideoMB: 1})
	e3.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e3.movies, CSFURL: f3.URL, CSFKey: ptr("wrong")})
	e3.bareOpp()
	_, err = e3.svc.Organize(e3.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, csf.ErrUnauthorized.Error()+"：到媒体库里重新填", e3.history()[0].Message)
	assert.NotContains(t, e3.history()[0].Message, "wrong")
}

func TestChineseSubFinderBadKey(t *testing.T) {
	// 保存的 Key 解不开（换过密钥）：写上要重新填
	e := newEnv(t)
	f := newFakeCSF(t)
	e.svc.cfg.CSFHTTP = f.Client()
	e.settings(SettingsInput{MinVideoMB: 1})
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, CSFURL: f.URL, CSFKey: ptr("csf-key")})
	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("id = ?", lib.ID).Update("csf_key_encrypted", "garbage").Error)
	e.bareOpp()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Empty(t, f.got())
	require.Len(t, res.Messages, 1)
	assert.Contains(t, res.Messages[0], "解不开保存的 ChineseSubFinder 的 API Key")
	assert.Contains(t, e.history()[0].Message, "解不开保存的 ChineseSubFinder 的 API Key")
}

func TestLibraryCSFSettings(t *testing.T) {
	e := newEnv(t)
	base := LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Enabled: true}

	in := base
	in.CSFURL = "http://csf.lan:19035"
	_, err := e.svc.SaveLibrary(e.ctx, 0, in)
	require.ErrorIs(t, err, ErrInvalid, "填了地址要有 Key")
	for _, bad := range []LibraryInput{
		{CSFURL: "csf.lan:19035", CSFKey: ptr("k")},
		{CSFURL: "http://csf.lan/?k=1", CSFKey: ptr("k")},
		{CSFURL: "http://csf.lan/" + strings.Repeat("a", 512), CSFKey: ptr("k")},
		{CSFURL: "http://csf.lan/", CSFKey: ptr("k"), CSFPath: "relative/dir"},
		{CSFURL: "http://csf.lan/", CSFKey: ptr("k"), CSFPath: "/" + strings.Repeat("长", 400)},
	} {
		c := base
		c.CSFURL, c.CSFKey, c.CSFPath = bad.CSFURL, bad.CSFKey, bad.CSFPath
		_, err = e.svc.SaveLibrary(e.ctx, 0, c)
		require.ErrorIs(t, err, ErrInvalid, bad.CSFURL+" "+bad.CSFPath)
	}

	in = base
	in.SiteSubtitles, in.CSFURL, in.CSFKey, in.CSFPath = true, " http://csf.lan:19035 ", ptr(" k1 "), ` D:\Media\Movies `
	v, err := e.svc.SaveLibrary(e.ctx, 0, in)
	require.NoError(t, err)
	assert.True(t, v.SiteSubtitles)
	assert.Equal(t, "http://csf.lan:19035/", v.CSFURL)
	assert.Equal(t, `D:\Media\Movies`, v.CSFPath)
	assert.True(t, v.HasCSFKey)
	b, _ := json.Marshal(v)
	assert.NotContains(t, string(b), "k1", "Key 不给界面")
	assert.NotContains(t, string(b), "csf_key_encrypted")
	var row models.MediaLibrary
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, "enc:k1", row.CSFKeyEncrypted, "加密保存")

	// 改别的不带 Key：Key 留着；带空串也留着
	in = base
	in.Name, in.CSFURL, in.CSFPath = "电影库", "http://csf2.lan/", "/mnt/movies"
	v, err = e.svc.SaveLibrary(e.ctx, v.ID, in)
	require.NoError(t, err)
	assert.True(t, v.HasCSFKey)
	assert.False(t, v.SiteSubtitles)
	in.CSFKey = ptr(" ")
	_, err = e.svc.SaveLibrary(e.ctx, v.ID, in)
	require.NoError(t, err)
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, "enc:k1", row.CSFKeyEncrypted)
	// 换 Key
	in.CSFKey = ptr("k2")
	_, err = e.svc.SaveLibrary(e.ctx, v.ID, in)
	require.NoError(t, err)
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Equal(t, "enc:k2", row.CSFKeyEncrypted)
	// 清掉地址：Key 一起清掉
	in.CSFURL, in.CSFKey = "", nil
	v, err = e.svc.SaveLibrary(e.ctx, v.ID, in)
	require.NoError(t, err)
	assert.False(t, v.HasCSFKey)
	assert.Empty(t, v.CSFURL)
	require.NoError(t, e.db.First(&row, v.ID).Error)
	assert.Empty(t, row.CSFKeyEncrypted)
	// 再填地址又要 Key
	in.CSFURL = "http://csf.lan/"
	_, err = e.svc.SaveLibrary(e.ctx, v.ID, in)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.SaveLibrary(e.ctx, 99, in)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestTestCSF(t *testing.T) {
	e := newEnv(t)
	f := newFakeCSF(t)
	e.svc.cfg.CSFHTTP = f.Client()
	require.NoError(t, e.svc.TestCSF(e.ctx, CSFTestInput{URL: f.URL, Key: ptr("csf-key")}))
	err := e.svc.TestCSF(e.ctx, CSFTestInput{URL: f.URL, Key: ptr("nope")})
	require.ErrorIs(t, err, csf.ErrUnauthorized)
	err = e.svc.TestCSF(e.ctx, CSFTestInput{URL: f.URL})
	require.ErrorIs(t, err, ErrInvalid, "没有 Key")
	err = e.svc.TestCSF(e.ctx, CSFTestInput{URL: "csf.lan", Key: ptr("k")})
	require.ErrorIs(t, err, ErrInvalid)

	// 用保存的 Key（地址可以先改）
	lib := e.library(LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, CSFURL: "http://old.lan/", CSFKey: ptr("csf-key")})
	require.NoError(t, e.svc.TestCSF(e.ctx, CSFTestInput{ID: lib.ID, URL: f.URL}))
	require.NoError(t, e.svc.TestCSF(e.ctx, CSFTestInput{ID: lib.ID, URL: f.URL, Key: ptr("")}))
	err = e.svc.TestCSF(e.ctx, CSFTestInput{ID: 99, URL: f.URL})
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, e.db.Model(&models.MediaLibrary{}).Where("id = ?", lib.ID).Update("csf_key_encrypted", "garbage").Error)
	err = e.svc.TestCSF(e.ctx, CSFTestInput{ID: lib.ID, URL: f.URL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解密 API Key 失败")
	assert.Empty(t, f.got(), "测试不建任务")
}
