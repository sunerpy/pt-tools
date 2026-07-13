// MIT License
// Copyright (c) 2025 pt-tools

package commands

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestBindHandler_Branches(t *testing.T) {
	t.Run("usage on missing code", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "bind")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "用法")
	})
	t.Run("service unavailable", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "bind")(context.Background(), []string{"CODE"}, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "绑定服务不可用")
	})
	t.Run("consume error", func(t *testing.T) {
		setupServices(t, &Services{Binding: &mockBindingService{consumeErr: errors.New("bad")}})
		reply, err := handler(t, "bind")(context.Background(), []string{"CODE"}, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "绑定失败")
	})
	t.Run("success", func(t *testing.T) {
		setupServices(t, &Services{Binding: &mockBindingService{}})
		reply, err := handler(t, "bind")(context.Background(), []string{"CODE"}, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "绑定成功")
	})
}

func TestDeleteHandler_Branches(t *testing.T) {
	t.Run("usage on missing id", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "delete")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "用法")
	})
	t.Run("no sessions", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "delete")(context.Background(), []string{"tid"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "会话存储未初始化")
	})
	t.Run("prompts for confirm with --with-data + downloader", func(t *testing.T) {
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s, Torrent: &mockTorrentService{}})
		reply, err := handler(t, "delete")(context.Background(),
			[]string{"tid", "--with-data", "qb"}, chatops.Source{ReplyLang: "zh", IsAdmin: true, ChannelType: "tg", ChannelUserID: "u"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "确认删除")
		assert.Contains(t, reply.Text, "含数据")
		st, ok := s.Pending("tg", 0, "u")
		require.True(t, ok)
		require.NotNil(t, st.Handler)
	})
}

// TestDeleteHandler_ConfirmFlow drives the confirm handler stored in the
// session: cancel, success, and torrent-service-unavailable branches.
func TestDeleteHandler_ConfirmFlow(t *testing.T) {
	newSess := func(t *testing.T, torrent app.TorrentService) (*chatops.SessionStore, chatops.Source) {
		t.Helper()
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s, Torrent: torrent})
		src := chatops.Source{ReplyLang: "zh", IsAdmin: true, ChannelType: "tg", ChannelUserID: "u"}
		_, err := handler(t, "delete")(context.Background(), []string{"tid"}, src)
		require.NoError(t, err)
		return s, src
	}

	t.Run("cancel on non-YES", func(t *testing.T) {
		tor := &mockTorrentService{}
		s, src := newSess(t, tor)
		st, ok := s.Pending("tg", 0, "u")
		require.True(t, ok)
		reply, err := st.Handler(context.Background(), []string{"no"}, src)
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "已取消删除")
		assert.Empty(t, tor.deleteCalls)
	})

	t.Run("delete success on YES", func(t *testing.T) {
		tor := &mockTorrentService{}
		s, src := newSess(t, tor)
		st, ok := s.Pending("tg", 0, "u")
		require.True(t, ok)
		reply, err := st.Handler(context.Background(), []string{"YES"}, src)
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "已删除")
		require.Len(t, tor.deleteCalls, 1)
		assert.Equal(t, "tid", tor.deleteCalls[0].id)
	})

	t.Run("torrent not found", func(t *testing.T) {
		tor := &notFoundTorrentService{}
		s, src := newSess(t, tor) //nolint:govet
		st, ok := s.Pending("tg", 0, "u")
		require.True(t, ok)
		reply, err := st.Handler(context.Background(), []string{"YES"}, src)
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "种子不存在")
	})
}

type notFoundTorrentService struct{ mockTorrentService }

func (m *notFoundTorrentService) Delete(context.Context, string, string, bool) error {
	return app.ErrTorrentNotFound
}

func TestUnbindHandler_Success(t *testing.T) {
	mb := &mockBindingService{}
	setupServices(t, &Services{Binding: mb, Bindings: &mockBindingResolver{id: 42, ok: true}})
	reply, err := handler(t, "unbind")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已解绑")
	require.Len(t, mb.revokeCalls, 1)
	assert.Equal(t, uint(42), mb.revokeCalls[0])
}

func TestTasksHandler_Branches(t *testing.T) {
	t.Run("service unavailable", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "tasks")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "任务服务不可用")
	})
	t.Run("list error", func(t *testing.T) {
		setupServices(t, &Services{Task: &mockTaskService{err: errors.New("boom")}})
		reply, err := handler(t, "tasks")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "查询任务失败")
	})
	t.Run("empty jobs", func(t *testing.T) {
		setupServices(t, &Services{Task: &mockTaskService{}})
		reply, err := handler(t, "tasks")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "无 RSS 任务")
	})
	t.Run("running + stopped jobs", func(t *testing.T) {
		setupServices(t, &Services{Task: &mockTaskService{jobs: []app.JobStatusDTO{
			{SiteName: "hdsky", RSSName: "a", Running: true},
			{SiteName: "mteam", RSSName: "b", Running: false},
		}}})
		reply, err := handler(t, "tasks")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "running")
		assert.Contains(t, reply.Text, "stopped")
	})
}

type errSiteService struct {
	sites   []app.SiteSummaryDTO
	infoErr error
	listErr error
}

func (m *errSiteService) ListSites(_ context.Context) ([]app.SiteSummaryDTO, error) {
	return m.sites, m.listErr
}

func (m *errSiteService) GetSiteUserInfo(_ context.Context, name string) (app.UserInfoDTO, error) {
	if m.infoErr != nil {
		return app.UserInfoDTO{}, m.infoErr
	}
	return app.UserInfoDTO{SiteName: name, Username: "u", Uploaded: "1TB"}, nil
}

type errTorrentService struct {
	mockTorrentService
	pauseErr  error
	resumeErr error
}

func (m *errTorrentService) Pause(_ context.Context, _, _ string) error { return m.pauseErr }

func (m *errTorrentService) Resume(_ context.Context, _, _ string) error { return m.resumeErr }

func handler(t *testing.T, name string) chatops.CommandHandler {
	t.Helper()
	spec, ok := chatops.DefaultRegistry().Get(name)
	require.True(t, ok, "command %s must be registered", name)
	return spec.Handler
}

func TestSitesServiceUnavailable(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "sites")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "不可用")
}

func TestSitesUserInfoSuccess(t *testing.T) {
	setupServices(t, &Services{Site: &errSiteService{}})
	reply, err := handler(t, "sites")(context.Background(), []string{"hdsky"}, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "hdsky")
	assert.Contains(t, reply.Text, "Uploaded")
}

func TestSitesUserInfoErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"not_found", app.ErrSiteNotFound, "站点不存在"},
		{"unavailable", app.ErrUserInfoUnavailable, "暂无用户信息"},
		{"generic", errors.New("boom"), "查询失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupServices(t, &Services{Site: &errSiteService{infoErr: tc.err}})
			reply, err := handler(t, "sites")(context.Background(), []string{"x"}, chatops.Source{ReplyLang: "zh"})
			require.NoError(t, err)
			assert.Contains(t, reply.Text, tc.want)
		})
	}
}

func TestSitesListError(t *testing.T) {
	setupServices(t, &Services{Site: &errSiteService{listErr: errors.New("db")}})
	reply, err := handler(t, "sites")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "查询站点失败")
}

func TestSitesEmpty(t *testing.T) {
	setupServices(t, &Services{Site: &errSiteService{sites: nil}})
	reply, err := handler(t, "sites")(context.Background(), nil, chatops.Source{ReplyLang: "en"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "no sites configured")
}

func TestPauseUsageAndUnavailable(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "pause")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法")

	setupServices(t, &Services{})
	reply, err = handler(t, "pause")(context.Background(), []string{"id"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "不可用")
}

func TestPauseErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"notfound", app.ErrTorrentNotFound, "种子不存在"},
		{"dl_notfound", app.ErrDownloaderNotFound, "下载器不存在"},
		{"generic", errors.New("x"), "暂停失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupServices(t, &Services{Torrent: &errTorrentService{pauseErr: tc.err}})
			reply, err := handler(t, "pause")(context.Background(), []string{"id", "qb"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
			require.NoError(t, err)
			assert.Contains(t, reply.Text, tc.want)
		})
	}
}

func TestResumeUsageAndErrors(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "resume")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法")

	setupServices(t, &Services{Torrent: &errTorrentService{resumeErr: app.ErrTorrentNotFound}})
	reply, err = handler(t, "resume")(context.Background(), []string{"id"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "种子不存在")

	setupServices(t, &Services{Torrent: &errTorrentService{resumeErr: app.ErrDownloaderNotFound}})
	reply, err = handler(t, "resume")(context.Background(), []string{"id", "qb"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "下载器不存在")

	setupServices(t, &Services{Torrent: &errTorrentService{resumeErr: errors.New("x")}})
	reply, err = handler(t, "resume")(context.Background(), []string{"id"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "恢复失败")
}

func TestTorrentsUsageAndUnavailable(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "torrents")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "不可用")

	setupServices(t, &Services{Torrent: &mockTorrentService{}})
	reply, err = handler(t, "torrents")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "用法")
}

func TestTorrentsListError(t *testing.T) {
	setupServices(t, &Services{Torrent: &mockTorrentService{listErr: errors.New("db")}})
	reply, err := handler(t, "torrents")(context.Background(), []string{"qb"}, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "查询种子失败")
}

func TestTorrentsEmpty(t *testing.T) {
	setupServices(t, &Services{Torrent: &mockTorrentService{}})
	reply, err := handler(t, "torrents")(context.Background(), []string{"qb"}, chatops.Source{ReplyLang: "en"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "no torrents")
}

func TestTruncateHelper(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 5))
	assert.Equal(t, "ab…", truncate("abcdef", 3))
}

func TestFormatBytesHelper(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Contains(t, formatBytes(2048), "KiB")
	assert.Contains(t, formatBytes(5*1024*1024), "MiB")
	assert.Contains(t, formatBytes(3*1024*1024*1024), "GiB")
}

func TestTrHelper(t *testing.T) {
	assert.Equal(t, "英文", tr("zh", "英文", "english"))
	assert.Equal(t, "english", tr("en", "英文", "english"))
	assert.Equal(t, "英文", tr("", "英文", "english"))
}

func TestParseDownloaderArg(t *testing.T) {
	assert.Equal(t, "qb", parseDownloaderArg([]string{"id", "qb"}, 1))
	assert.Equal(t, "", parseDownloaderArg([]string{"id"}, 1))
}

func TestUnbindServiceUnavailable(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "unbind")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "不可用")
}

func TestUnbindLookupError(t *testing.T) {
	setupServices(t, &Services{Binding: &mockBindingService{}, Bindings: &mockBindingResolver{err: errors.New("x")}})
	reply, err := handler(t, "unbind")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "查询绑定失败")
}

func TestUnbindNotFound(t *testing.T) {
	setupServices(t, &Services{Binding: &mockBindingService{}, Bindings: &mockBindingResolver{ok: false}})
	reply, err := handler(t, "unbind")(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "未找到当前绑定")
}

func TestVersionNoServicesStillWorks(t *testing.T) {
	setupServices(t, &Services{})
	reply, err := handler(t, "version")(context.Background(), nil, chatops.Source{ReplyLang: "en"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "Version:")
}

type mockTaskService struct {
	jobs []app.JobStatusDTO
	err  error
}

func (m *mockTaskService) ListJobs(_ context.Context) ([]app.JobStatusDTO, error) {
	return m.jobs, m.err
}

func (m *mockTaskService) StartJob(_ context.Context, _, _ string) error { return nil }

func (m *mockTaskService) StopJob(_ context.Context, _, _ string) error { return nil }

type mockTorrentService struct {
	mu          sync.Mutex
	listResult  []app.TorrentDTO
	listTotal   int
	listErr     error
	deleteCalls []deleteCall
	pauseCalls  []string
	resumeCalls []string
}

type deleteCall struct {
	name       string
	id         string
	removeData bool
}

func (m *mockTorrentService) ListByDownloader(_ context.Context, _ string, _, _ int) ([]app.TorrentDTO, int, error) {
	return m.listResult, m.listTotal, m.listErr
}

func (m *mockTorrentService) Get(_ context.Context, _, _ string) (app.TorrentDTO, error) {
	return app.TorrentDTO{}, nil
}

func (m *mockTorrentService) Pause(_ context.Context, _, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pauseCalls = append(m.pauseCalls, id)
	return nil
}

func (m *mockTorrentService) Resume(_ context.Context, _, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resumeCalls = append(m.resumeCalls, id)
	return nil
}

func (m *mockTorrentService) Delete(_ context.Context, name, id string, removeData bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls = append(m.deleteCalls, deleteCall{name: name, id: id, removeData: removeData})
	return nil
}

type mockSiteService struct {
	sites []app.SiteSummaryDTO
}

func (m *mockSiteService) ListSites(_ context.Context) ([]app.SiteSummaryDTO, error) {
	return m.sites, nil
}

func (m *mockSiteService) GetSiteUserInfo(_ context.Context, name string) (app.UserInfoDTO, error) {
	return app.UserInfoDTO{SiteName: name, Username: "u"}, nil
}

type mockBindingService struct {
	consumeErr  error
	consumed    []string
	revokeCalls []uint
}

func (m *mockBindingService) IssueCode(_ context.Context, _ uint, _ string, _ time.Duration) (app.BindCodeDTO, error) {
	return app.BindCodeDTO{}, nil
}

func (m *mockBindingService) ListPendingCodes(_ context.Context) ([]app.BindCodeDTO, error) {
	return nil, nil
}

func (m *mockBindingService) ConsumeCode(_ context.Context, code, _, _ string) (app.BindingDTO, error) {
	if m.consumeErr != nil {
		return app.BindingDTO{}, m.consumeErr
	}
	m.consumed = append(m.consumed, code)
	return app.BindingDTO{ID: 1}, nil
}

func (m *mockBindingService) ListBindings(_ context.Context) ([]app.BindingDTO, error) {
	return nil, nil
}

func (m *mockBindingService) Revoke(_ context.Context, id uint) error {
	m.revokeCalls = append(m.revokeCalls, id)
	return nil
}

func (m *mockBindingService) SetReplyLang(_ context.Context, _ uint, _ string) error { return nil }

type mockBindingResolver struct {
	id  uint
	ok  bool
	err error
}

func (m *mockBindingResolver) FindByChannelUser(_ context.Context, _, _ string) (uint, bool, error) {
	return m.id, m.ok, m.err
}

type mockDownloaderStatus struct {
	statuses []downloader.DownloaderStatus
}

func (m *mockDownloaderStatus) GetAllDownloaderStatus() []downloader.DownloaderStatus {
	return m.statuses
}

func setupServices(t *testing.T, s *Services) {
	t.Helper()
	prev := getServices()
	SetServices(s)
	t.Cleanup(func() { SetServices(prev) })
}

func TestHelpCmd_Listing(t *testing.T) {
	setupServices(t, &Services{})
	specs := chatops.DefaultRegistry().List()
	require.GreaterOrEqual(t, len(specs), 11)

	helpSpec, ok := chatops.DefaultRegistry().Get("help")
	require.True(t, ok)

	reply, err := helpSpec.Handler(context.Background(), nil, chatops.Source{IsAdmin: true, ReplyLang: "zh"})
	require.NoError(t, err)
	for _, name := range []string{"help", "status", "tasks", "torrents", "sites", "version", "pause", "resume", "delete", "bind", "unbind"} {
		assert.Contains(t, reply.Text, "/"+name, "missing /%s in help output", name)
	}

	nonAdmin, err := helpSpec.Handler(context.Background(), nil, chatops.Source{IsAdmin: false, ReplyLang: "zh"})
	require.NoError(t, err)
	assert.NotContains(t, nonAdmin.Text, "/pause")
	assert.NotContains(t, nonAdmin.Text, "/delete")
	assert.Contains(t, nonAdmin.Text, "/bind")
}

func TestStatusCmd_OutputShape(t *testing.T) {
	setupServices(t, &Services{
		Task: &mockTaskService{jobs: []app.JobStatusDTO{
			{SiteName: "s", RSSName: "r", Running: true},
			{SiteName: "s2", RSSName: "r2", Running: false},
		}},
		Downloader: &mockDownloaderStatus{statuses: []downloader.DownloaderStatus{
			{Name: "qb1", Type: "qbittorrent", IsHealthy: true},
		}},
	})
	spec, _ := chatops.DefaultRegistry().Get("status")
	reply, err := spec.Handler(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "qb1")
	assert.Contains(t, reply.Text, "1 运行中")
}

func TestTasksCmd_Listing(t *testing.T) {
	setupServices(t, &Services{
		Task: &mockTaskService{jobs: []app.JobStatusDTO{
			{SiteName: "hdsky", RSSName: "main", Running: true},
		}},
	})
	spec, _ := chatops.DefaultRegistry().Get("tasks")
	reply, err := spec.Handler(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "hdsky/main")
	assert.Contains(t, reply.Text, "running")
}

func TestTorrentsCmd_RateLimited(t *testing.T) {
	spec, ok := chatops.DefaultRegistry().Get("torrents")
	require.True(t, ok)
	require.NotNil(t, spec.RateLimit)
	assert.Equal(t, 1, spec.RateLimit.Burst)
	assert.Equal(t, 10*time.Second, spec.RateLimit.Per)

	rl := chatops.NewRateLimiter()
	assert.True(t, rl.Allow("telegram", "u1", "torrents"))
	assert.False(t, rl.Allow("telegram", "u1", "torrents"))
}

func TestTorrentsCmd_Listing(t *testing.T) {
	setupServices(t, &Services{
		Torrent: &mockTorrentService{
			listResult: []app.TorrentDTO{{ID: "h1", Name: "demo", State: "downloading", Progress: 0.42, Size: 2048}},
			listTotal:  1,
		},
	})
	spec, _ := chatops.DefaultRegistry().Get("torrents")
	reply, err := spec.Handler(context.Background(), []string{"qb1"}, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "demo")
}

func TestSitesCmd_List(t *testing.T) {
	setupServices(t, &Services{
		Site: &mockSiteService{sites: []app.SiteSummaryDTO{{Name: "hdsky", Status: "enabled"}}},
	})
	spec, _ := chatops.DefaultRegistry().Get("sites")
	reply, err := spec.Handler(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "hdsky")
}

func TestVersionCmd(t *testing.T) {
	setupServices(t, &Services{})
	spec, _ := chatops.DefaultRegistry().Get("version")
	reply, err := spec.Handler(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "Version:")
	assert.Contains(t, reply.Text, "Commit:")
}

func TestPauseCmd_Success(t *testing.T) {
	mt := &mockTorrentService{}
	setupServices(t, &Services{Torrent: mt})
	spec, _ := chatops.DefaultRegistry().Get("pause")
	require.True(t, spec.AdminOnly)
	_, err := spec.Handler(context.Background(), []string{"abc", "qb1"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, mt.pauseCalls, "abc")
}

func TestResumeCmd_Success(t *testing.T) {
	mt := &mockTorrentService{}
	setupServices(t, &Services{Torrent: mt})
	spec, _ := chatops.DefaultRegistry().Get("resume")
	_, err := spec.Handler(context.Background(), []string{"xyz"}, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, mt.resumeCalls, "xyz")
}

func TestDeleteCmd_NonAdminDenied(t *testing.T) {
	spec, _ := chatops.DefaultRegistry().Get("delete")
	require.True(t, spec.AdminOnly, "delete must be admin only — chain enforces denial")
}

func TestDeleteCmd_RequiresConfirm(t *testing.T) {
	mt := &mockTorrentService{}
	store := chatops.NewSessionStore()
	t.Cleanup(store.Stop)
	setupServices(t, &Services{Torrent: mt, Sessions: store})

	spec, _ := chatops.DefaultRegistry().Get("delete")
	src := chatops.Source{
		ChannelType:   "telegram",
		ChannelConfID: 1,
		ChannelUserID: "u1",
		ReplyLang:     "zh",
		IsAdmin:       true,
	}

	reply, err := spec.Handler(context.Background(), []string{"abc", "--with-data"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "确认删除 abc")
	assert.Contains(t, reply.Text, "YES")

	state, ok := store.Pending(src.ChannelType, src.ChannelConfID, src.ChannelUserID)
	require.True(t, ok)
	require.NotNil(t, state.Handler)
	assert.Equal(t, "confirm_delete", state.Step)

	require.Empty(t, mt.deleteCalls, "no delete should occur before YES")

	confirmReply, err := state.Handler(context.Background(), []string{"YES"}, src)
	require.NoError(t, err)
	assert.Contains(t, confirmReply.Text, "已删除 abc")
	require.Len(t, mt.deleteCalls, 1)
	assert.Equal(t, "abc", mt.deleteCalls[0].id)
	assert.True(t, mt.deleteCalls[0].removeData)
}

func TestDeleteCmd_ConfirmCancelled(t *testing.T) {
	mt := &mockTorrentService{}
	store := chatops.NewSessionStore()
	t.Cleanup(store.Stop)
	setupServices(t, &Services{Torrent: mt, Sessions: store})

	spec, _ := chatops.DefaultRegistry().Get("delete")
	src := chatops.Source{ChannelType: "tg", ChannelConfID: 1, ChannelUserID: "u2", ReplyLang: "zh"}
	_, _ = spec.Handler(context.Background(), []string{"abc"}, src)

	state, ok := store.Pending(src.ChannelType, src.ChannelConfID, src.ChannelUserID)
	require.True(t, ok)
	reply, err := state.Handler(context.Background(), []string{"no"}, src)
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消")
	assert.Empty(t, mt.deleteCalls)
}

func TestBindCmd_Success(t *testing.T) {
	mb := &mockBindingService{}
	setupServices(t, &Services{Binding: mb})
	spec, _ := chatops.DefaultRegistry().Get("bind")
	reply, err := spec.Handler(context.Background(), []string{"ABCD2345"}, chatops.Source{
		ChannelType:   "telegram",
		ChannelUserID: "u1",
		ReplyLang:     "zh",
	})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "成功")
	assert.Equal(t, []string{"ABCD2345"}, mb.consumed)
}

func TestBindCmd_InvalidCode(t *testing.T) {
	mb := &mockBindingService{consumeErr: errors.New("invalid")}
	setupServices(t, &Services{Binding: mb})
	spec, _ := chatops.DefaultRegistry().Get("bind")

	reply, err := spec.Handler(context.Background(), []string{"BADCODE2"}, chatops.Source{ReplyLang: "zh"})
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(reply.Text), "invalid")
}

func TestUnbindCmd_Success(t *testing.T) {
	mb := &mockBindingService{}
	mr := &mockBindingResolver{id: 42, ok: true}
	setupServices(t, &Services{Binding: mb, Bindings: mr})
	spec, _ := chatops.DefaultRegistry().Get("unbind")
	require.True(t, spec.AdminOnly)
	reply, err := spec.Handler(context.Background(), nil, chatops.Source{ReplyLang: "zh", IsAdmin: true})
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已解绑")
	assert.Equal(t, []uint{42}, mb.revokeCalls)
}
