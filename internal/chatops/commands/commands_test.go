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

type mockTaskService struct {
	jobs []app.JobStatusDTO
	err  error
}

func (m *mockTaskService) ListJobs(_ context.Context) ([]app.JobStatusDTO, error) {
	return m.jobs, m.err
}

func (m *mockTaskService) StartJob(_ context.Context, _, _ string) error { return nil }
func (m *mockTaskService) StopJob(_ context.Context, _, _ string) error  { return nil }

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

func (m *mockBindingService) IssueCode(_ context.Context, _ uint, _ string) (app.BindCodeDTO, error) {
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
