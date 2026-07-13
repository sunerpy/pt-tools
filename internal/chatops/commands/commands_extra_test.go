package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

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

func (m *errTorrentService) Pause(_ context.Context, _, _ string) error  { return m.pauseErr }
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
