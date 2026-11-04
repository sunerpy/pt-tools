package organize

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// fakeJellyfin 是假的 Jellyfin：/System/Info、整个库的 /Items、/Users 与继续观看。items 是 /Items 回的项（JSON），
// resume 是 u1 的继续观看。
type fakeJellyfin struct {
	*httptest.Server
	mu     sync.Mutex
	items  []string
	resume []string
	uid    string
	lists  int
	down   bool
}

const jellyfinKey = "jfkey"

func newFakeJellyfin(t *testing.T) *fakeJellyfin {
	f := &fakeJellyfin{uid: "jf-uid"}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), `Token="`+jellyfinKey+`"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.down {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/System/Info":
			_, _ = fmt.Fprintf(w, `{"ServerName":"jf","Version":"12.2.0","Id":%q}`, f.uid)
		case "/Items":
			f.lists++
			_, _ = fmt.Fprintf(w, `{"Items":[%s],"TotalRecordCount":%d}`, strings.Join(f.items, ","), len(f.items))
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"admin"},{"Id":"u2","Name":"家人"}]`))
		case "/Users/u1/Items/Resume":
			_, _ = fmt.Fprintf(w, `{"Items":[%s]}`, strings.Join(f.resume, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeJellyfin) set(items ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = items
}

func (f *fakeJellyfin) listCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lists
}

const (
	jfMovie   = `{"Id":"m1","Name":"奥本海默","Type":"Movie","ProductionYear":2023,"Path":"/data/movies/奥本海默 (2023)/奥本海默 (2023).mkv","ProviderIds":{"Tmdb":"872585"}}`
	jfSeries  = `{"Id":"s1","Name":"最后生还者","Type":"Series","ProviderIds":{"Tmdb":"100088"}}`
	jfEpisode = `{"Id":"e1","Name":"当你迷失在黑暗中","Type":"Episode","SeriesId":"s1","ParentIndexNumber":1,"IndexNumber":1,"Path":"/data/tv/最后生还者 (2023)/Season 01/S01E01.mkv"}`
)

func ptr[T any](v T) *T { return &v }

// saveJellyfin 存一台 Jellyfin（路径前缀 /media → /data）。
func (e *env) saveJellyfin(f *fakeJellyfin, in ServerInput) ServerView {
	e.t.Helper()
	if in.Name == "" {
		in.Name = "Jellyfin"
	}
	in.Kind, in.URL, in.Token = models.MediaServerJellyfin, f.URL, ptr(jellyfinKey)
	if in.LocalPrefix == "" {
		in.LocalPrefix, in.ServerPrefix = "/media", "/data"
	}
	in.Enabled = true
	v, err := e.svc.SaveServer(e.ctx, 0, in)
	require.NoError(e.t, err)
	return v
}

func TestServerIndexSettings(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	v := e.saveJellyfin(f, ServerInput{})
	assert.Equal(t, DefaultIndexHours, v.IndexHours, "新建时默认 6 小时同步一次")
	assert.False(t, v.Webhook)
	assert.False(t, v.CountLibrary)
	assert.Empty(t, v.PlayURL)
	assert.Equal(t, []uint{}, v.PlaybackChannels)

	in := ServerInput{
		Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: f.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data",
		PlayURL: ptr(" https://jf.example.com/ "), IndexHours: ptr(12), CountLibrary: ptr(true),
		ResumeUserID: ptr("u1"), ResumeUserName: ptr("admin"), PlaybackChannels: ptr([]uint{3, 3, 5}),
	}
	v, err := e.svc.SaveServer(e.ctx, v.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "https://jf.example.com", v.PlayURL)
	assert.Equal(t, 12, v.IndexHours)
	assert.True(t, v.CountLibrary)
	assert.Equal(t, [2]string{"u1", "admin"}, [2]string{v.ResumeUserID, v.ResumeUserName})
	assert.Equal(t, []uint{3, 5}, v.PlaybackChannels, "去重")

	// 不给的那几项不改（旧页面、只改开关时）
	v, err = e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: f.URL, Enabled: false, LocalPrefix: "/media", ServerPrefix: "/data"})
	require.NoError(t, err)
	assert.False(t, v.Enabled)
	assert.Equal(t, [4]any{"https://jf.example.com", 12, true, "u1"}, [4]any{v.PlayURL, v.IndexHours, v.CountLibrary, v.ResumeUserID})
	assert.Equal(t, []uint{3, 5}, v.PlaybackChannels)

	// 0 是关掉索引；清掉用户时用户名也清掉；通道清空
	in.IndexHours, in.ResumeUserID, in.ResumeUserName, in.PlaybackChannels, in.PlayURL = ptr(0), ptr(""), ptr("admin"), ptr([]uint{}), ptr("")
	v, err = e.svc.SaveServer(e.ctx, v.ID, in)
	require.NoError(t, err)
	assert.Equal(t, 0, v.IndexHours)
	assert.True(t, v.IndexOff)
	assert.Empty(t, v.ResumeUserName)
	assert.Empty(t, v.PlaybackChannels)
	assert.Empty(t, v.PlayURL)

	for name, bad := range map[string]ServerInput{
		"间隔太长":     {IndexHours: ptr(MaxIndexHours + 1)},
		"间隔是负的":    {IndexHours: ptr(-1)},
		"打开链接地址不对": {PlayURL: ptr("ftp://x")},
		"用户编号带斜杠":  {ResumeUserID: ptr("a/b")},
		"用户名太长":    {ResumeUserID: ptr("u1"), ResumeUserName: ptr(strings.Repeat("名", 129))},
		"通道编号是 0":  {PlaybackChannels: ptr([]uint{0})},
	} {
		bad.Name, bad.Kind, bad.URL = "Jellyfin", models.MediaServerJellyfin, f.URL
		_, err = e.svc.SaveServer(e.ctx, v.ID, bad)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	_, err = e.svc.SaveServer(e.ctx, 0, ServerInput{Name: "Plex", Kind: models.MediaServerPlex, URL: "http://plex:32400", Token: ptr("t"), ResumeUserID: ptr("u1")})
	require.ErrorIs(t, err, ErrInvalid, "Plex 不支持继续观看")
}

func TestServerKindChangeClearsResumeUser(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	v := e.saveJellyfin(f, ServerInput{ResumeUserID: ptr("u1"), ResumeUserName: ptr("admin")})
	v, err := e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Jellyfin", Kind: models.MediaServerPlex, URL: "http://plex:32400", Token: ptr("t")})
	require.NoError(t, err)
	assert.Empty(t, v.ResumeUserID, "换成 Plex 时清掉继续观看的用户")
}

func TestServerUsers(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	v := e.saveJellyfin(f, ServerInput{})
	users, err := e.svc.ServerUsers(e.ctx, ServerTestInput{ID: v.ID, Kind: models.MediaServerJellyfin, URL: f.URL})
	require.NoError(t, err, "没填 Token 时用保存的")
	assert.Equal(t, []server.User{{ID: "u1", Name: "admin"}, {ID: "u2", Name: "家人"}}, users)
	_, err = e.svc.ServerUsers(e.ctx, ServerTestInput{Kind: models.MediaServerPlex, URL: "http://plex:32400", Token: ptr("t")})
	require.ErrorIs(t, err, server.ErrUnsupported)
	_, err = e.svc.ServerUsers(e.ctx, ServerTestInput{ID: 99, Kind: models.MediaServerJellyfin, URL: f.URL})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestSyncServerAndIndexLifecycle(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	f.set(jfMovie, jfSeries, jfEpisode)
	v := e.saveJellyfin(f, ServerInput{})

	res, err := e.svc.SyncServer(e.ctx, v.ID)
	require.NoError(t, err)
	assert.Equal(t, [3]int{1, 1, 1}, [3]int{res.Movies, res.Series, res.Episodes})
	list, err := e.svc.Servers(e.ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	got := list[0]
	require.NotNil(t, got.IndexedAt)
	assert.Equal(t, [3]int{1, 1, 1}, [3]int{got.IndexMovies, got.IndexSeries, got.IndexEpisodes})
	var n int64
	require.NoError(t, e.db.Model(&models.MediaServerItem{}).Where("server_id = ?", v.ID).Count(&n).Error)
	assert.EqualValues(t, 3, n)

	// 改了地址：多半换了一台服务器，清掉旧的索引
	other := newFakeJellyfin(t)
	upd, err := e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: other.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data"})
	require.NoError(t, err)
	assert.Nil(t, upd.IndexedAt)
	assert.Zero(t, upd.IndexMovies)
	require.NoError(t, e.db.Model(&models.MediaServerItem{}).Where("server_id = ?", v.ID).Count(&n).Error)
	assert.Zero(t, n)

	// 只改别的（名字、开关）不清
	_, err = e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: f.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data"})
	require.NoError(t, err)
	_, err = e.svc.SyncServer(e.ctx, v.ID)
	require.NoError(t, err)
	_, err = e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "家里的 Jellyfin", Kind: models.MediaServerJellyfin, URL: f.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data"})
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.MediaServerItem{}).Where("server_id = ?", v.ID).Count(&n).Error)
	assert.EqualValues(t, 3, n)

	// 删除服务器连同索引
	require.NoError(t, e.svc.DeleteServer(e.ctx, v.ID))
	require.NoError(t, e.db.Model(&models.MediaServerItem{}).Count(&n).Error)
	assert.Zero(t, n)

	_, err = e.svc.SyncServer(e.ctx, v.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestSyncServerRefusesDisabled(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	v := e.saveJellyfin(f, ServerInput{IndexHours: ptr(0)})
	_, err := e.svc.SyncServer(e.ctx, v.ID)
	require.ErrorIs(t, err, ErrInvalid, "关了索引")
	assert.Zero(t, f.listCount())

	// 同步失败：记下原因
	_, err = e.svc.SaveServer(e.ctx, v.ID, ServerInput{Name: "Jellyfin", Kind: models.MediaServerJellyfin, URL: f.URL, Enabled: true, LocalPrefix: "/media", ServerPrefix: "/data", IndexHours: ptr(6)})
	require.NoError(t, err)
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	_, err = e.svc.SyncServer(e.ctx, v.ID)
	require.ErrorIs(t, err, server.ErrUnavailable)
	list, _ := e.svc.Servers(e.ctx)
	assert.Contains(t, list[0].IndexError, "HTTP 500")
}

func TestIndexDueRules(t *testing.T) {
	st := newIndexState()
	start := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	st.startedAt = start
	never := models.MediaServer{ID: 1}
	assert.False(t, st.due(never, start.Add(30*time.Second)), "启动 1 分钟以内不做第一次")
	assert.True(t, st.due(never, start.Add(time.Minute)))

	synced := start.Add(-5 * time.Hour)
	srv := models.MediaServer{ID: 2, IndexedAt: &synced}
	assert.False(t, st.due(srv, start), "默认 6 小时")
	assert.True(t, st.due(srv, start.Add(time.Hour)))
	srv.IndexHours = 12
	assert.False(t, st.due(srv, start.Add(time.Hour)))

	// 排上的到了时间就做
	st.schedule(2, start.Add(2*time.Minute))
	st.schedule(2, start.Add(5*time.Minute))
	assert.False(t, st.due(srv, start.Add(time.Minute)))
	assert.True(t, st.due(srv, start.Add(2*time.Minute)), "合进更早的那一次")
	st.done(2, start.Add(2*time.Minute), start.Add(2*time.Minute), nil)
	assert.False(t, st.due(srv, start.Add(3*time.Minute)), "做掉了")

	// 同步的时候新排上的（这时来了 Webhook）留着，下一次做
	st.schedule(2, start.Add(5*time.Minute))
	st.done(2, start.Add(3*time.Minute), start.Add(4*time.Minute), nil)
	assert.True(t, st.due(srv, start.Add(5*time.Minute)), "开始同步以后才排上的不算做掉了")
	st.done(2, start.Add(5*time.Minute), start.Add(5*time.Minute), nil)
	assert.False(t, st.due(srv, start.Add(6*time.Minute)))

	// 失败以后 30 分钟再试；间隔更短时按间隔
	st.done(2, start, start, fmt.Errorf("x"))
	assert.False(t, st.due(srv, start.Add(29*time.Minute)))
	assert.True(t, st.due(srv, start.Add(30*time.Minute)))
	short := models.MediaServer{ID: 3, IndexHours: 1, IndexedAt: &synced}
	st.done(3, start, start, fmt.Errorf("x"))
	assert.True(t, st.due(short, start.Add(30*time.Minute)))
	st.done(3, start, start, nil)
	short.IndexedAt = &start
	assert.False(t, st.due(short, start.Add(10*time.Minute)), "成功以后按上次成功的时间")
	assert.True(t, st.due(short, start.Add(time.Hour)))
	assert.Equal(t, 0, indexHours(models.MediaServer{IndexOff: true, IndexHours: 6}))
}

func TestIndexDueSyncsOnlyDueServers(t *testing.T) {
	e := newEnv(t)
	a, b := newFakeJellyfin(t), newFakeJellyfin(t)
	a.set(jfMovie)
	b.set(jfMovie)
	va := e.saveJellyfin(a, ServerInput{Name: "A"})
	e.saveJellyfin(b, ServerInput{Name: "B", IndexHours: ptr(0)})
	e.svc.idx.startedAt = e.Now().Add(-time.Hour)
	e.svc.indexDue(e.ctx)
	assert.Equal(t, 1, a.listCount(), "从没同步过：启动 1 分钟以后同步")
	assert.Zero(t, b.listCount(), "关了索引的不同步")
	e.svc.indexDue(e.ctx)
	assert.Equal(t, 1, a.listCount(), "刚同步过，不到 6 小时")

	e.svc.scheduleIndex(va.ID, webhookSyncDelay)
	e.svc.indexDue(e.ctx)
	assert.Equal(t, 1, a.listCount(), "Webhook 排的 2 分钟后才做")
	e.advance(webhookSyncDelay)
	e.svc.indexDue(e.ctx)
	assert.Equal(t, 2, a.listCount())

	// 手动同步排上马上叫醒节拍
	e.svc.scheduleIndex(va.ID, 0)
	select {
	case <-e.svc.idx.wake:
	default:
		t.Fatal("delay 为 0 时要叫醒后台节拍")
	}
	// 排在以后的：到时间叫醒，不等下一个 30 秒的节拍
	e.svc.scheduleIndex(va.ID, 20*time.Millisecond)
	select {
	case <-e.svc.idx.wake:
	case <-time.After(5 * time.Second):
		t.Fatal("排上的同步到时间要叫醒后台节拍")
	}
}

func TestIndexLoopRunsAndStops(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	f.set(jfMovie)
	v := e.saveJellyfin(f, ServerInput{})
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	e.svc.scheduleIndex(v.ID, 0)
	require.Eventually(t, func() bool { return f.listCount() > 0 }, 10*time.Second, 10*time.Millisecond, "排上马上要做的会叫醒节拍")
	e.svc.Stop()
	assert.False(t, e.svc.isRunning())
}

func TestResume(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	f.set(jfSeries, jfEpisode, jfMovie)
	f.resume = []string{
		`{"Id":"e1","Name":"当你迷失在黑暗中","Type":"Episode","SeriesId":"s1","SeriesName":"最后生还者","ParentIndexNumber":1,"IndexNumber":1,"RunTimeTicks":36000000000,"UserData":{"PlayedPercentage":25,"PlaybackPositionTicks":9000000000}}`,
		`{"Id":"m1","Name":"奥本海默","Type":"Movie","ProviderIds":{"Tmdb":"872585"},"UserData":{"PlayedPercentage":50}}`,
	}
	v := e.saveJellyfin(f, ServerInput{ResumeUserID: ptr("u1"), ResumeUserName: ptr("admin"), PlayURL: ptr("https://jf.example.com")})
	_, err := e.svc.SyncServer(e.ctx, v.ID)
	require.NoError(t, err)
	// 没选用户的、Plex、关着的不取
	e.saveJellyfin(newFakeJellyfin(t), ServerInput{Name: "没选用户"})

	groups, err := e.svc.Resume(e.ctx)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	g := groups[0]
	assert.Equal(t, [3]any{v.ID, "Jellyfin", "admin"}, [3]any{g.ServerID, g.Name, g.User})
	assert.Empty(t, g.Error)
	require.Len(t, g.Items, 2)
	assert.Equal(t, ResumeItem{
		ServerID: v.ID, ItemID: "e1", MediaType: models.MediaKindTV, TMDBID: 100088, Title: "最后生还者", Season: 1, Episode: 1,
		EpisodeName: "当你迷失在黑暗中", StillPath: "/e1.jpg", BackdropPath: "/tlou-bd.jpg", PosterPath: "/tlou.jpg", Percent: 25,
		RemainingSeconds: 2700, OpenURL: "https://jf.example.com/web/#/details?id=e1&serverId=jf-uid",
	}, g.Items[0], "剧的 TMDB 编号从索引里按剧找；剧照取这一集的")
	assert.Equal(t, ResumeItem{
		ServerID: v.ID, ItemID: "m1", MediaType: models.MediaKindMovie, TMDBID: 872585, Title: "奥本海默", BackdropPath: "/opp-bd.jpg",
		PosterPath: "/opp.jpg", Percent: 50, OpenURL: "https://jf.example.com/web/#/details?id=m1&serverId=jf-uid",
	}, g.Items[1])

	// 服务器出错：这一台写原因，别的照样给
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
	groups, err = e.svc.Resume(e.ctx)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.NotEmpty(t, groups[0].Error)
	assert.Empty(t, groups[0].Items)
}

func TestIssuesAndDismiss(t *testing.T) {
	e := newEnv(t)
	f := newFakeJellyfin(t)
	f.set(jfMovie)
	v := e.saveJellyfin(f, ServerInput{})
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{
		Status: models.MediaTransferDone, MediaType: models.MediaKindMovie, TMDBID: 872585, SourcePath: "/downloads/a.mkv",
		TargetPath: "/media/movies/奥本海默 (2023)/奥本海默 (2023).mkv",
	}).Error)
	_, err := e.svc.SyncServer(e.ctx, v.ID)
	require.NoError(t, err)
	f.set()
	_, err = e.svc.SyncServer(e.ctx, v.ID)
	require.NoError(t, err)
	list, err := e.svc.Issues(e.ctx, issueFilterOpen())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Jellyfin", list[0].ServerName)
	assert.Equal(t, models.MediaIssueServerRemoved, list[0].Kind)
	row, err := e.svc.DismissIssue(e.ctx, list[0].ID)
	require.NoError(t, err)
	assert.Equal(t, models.MediaIssueDismissed, row.Status)
	_, err = e.svc.DismissIssue(e.ctx, 999)
	require.ErrorIs(t, err, ErrNotFound)
}

func issueFilterOpen() serverindex.IssueFilter {
	return serverindex.IssueFilter{Status: models.MediaIssueOpen}
}
