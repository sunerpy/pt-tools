package commands

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/models"
)

func TestDelrssHandler_ServiceGuards(t *testing.T) {
	t.Run("no sessions", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "delrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "会话存储未初始化")
	})
	t.Run("no site service", func(t *testing.T) {
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s})
		reply, err := handler(t, "delrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "站点服务不可用")
	})
	t.Run("no rss wizard", func(t *testing.T) {
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s, Site: &mockSiteService{}})
		reply, err := handler(t, "delrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "RSS 向导服务不可用")
	})
}

func TestAddrssHandler_ServiceGuards(t *testing.T) {
	t.Run("no sessions", func(t *testing.T) {
		setupServices(t, &Services{})
		reply, err := handler(t, "addrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "会话存储未初始化")
	})
	t.Run("no site service", func(t *testing.T) {
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s})
		reply, err := handler(t, "addrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "站点服务不可用")
	})
	t.Run("no rss wizard", func(t *testing.T) {
		s := chatops.NewSessionStore()
		t.Cleanup(s.Stop)
		setupServices(t, &Services{Sessions: s, Site: &mockSiteService{}})
		reply, err := handler(t, "addrss")(context.Background(), nil, chatops.Source{ReplyLang: "zh"})
		require.NoError(t, err)
		assert.Contains(t, reply.Text, "RSS 向导服务不可用")
	})
}

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

func TestHandleAddrssShortcut_ErrorPaths(t *testing.T) {
	newHarness := func(t *testing.T, wizard *addrssFakeRSSWizard) *addrssHarness {
		return newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)
	}

	t.Run("too few segments", func(t *testing.T) {
		h := newHarness(t, newAddrssFakeRSSWizard())
		reply := h.send(t, "/addrss hdsky | onlyname")
		assert.Contains(t, reply.Text, "快捷格式有误")
	})

	t.Run("unknown site", func(t *testing.T) {
		h := newHarness(t, newAddrssFakeRSSWizard())
		reply := h.send(t, "/addrss nosite | feed | https://rss.example/x.xml")
		assert.Contains(t, reply.Text, "站点不存在或未启用")
	})

	t.Run("invalid url", func(t *testing.T) {
		h := newHarness(t, newAddrssFakeRSSWizard())
		reply := h.send(t, "/addrss hdsky | feed | not-a-url")
		assert.Contains(t, reply.Text, "RSS URL 无效")
	})

	t.Run("unknown downloader", func(t *testing.T) {
		h := newHarness(t, newAddrssFakeRSSWizard())
		reply := h.send(t, "/addrss hdsky | feed | https://rss.example/x.xml | nope-dl")
		assert.Contains(t, reply.Text, "下载器不存在")
	})

	t.Run("happy path with named downloader", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		h := newHarness(t, wizard)
		reply := h.send(t, "/addrss hdsky | feed | https://rss.example/x.xml | tr-backup")
		assert.Contains(t, reply.Text, "已添加 RSS 订阅")
		calls := wizard.calls()
		require.Len(t, calls, 1)
		require.NotNil(t, calls[0].entry.DownloaderID)
		assert.Equal(t, uint(23), *calls[0].entry.DownloaderID)
	})

	t.Run("happy path default downloader", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		h := newHarness(t, wizard)
		reply := h.send(t, "/addrss hdsky | feed | https://rss.example/x.xml | default")
		assert.Contains(t, reply.Text, "已添加 RSS 订阅")
		calls := wizard.calls()
		require.Len(t, calls, 1)
		assert.Nil(t, calls[0].entry.DownloaderID, "default keyword should not pin a downloader")
	})

	t.Run("append error", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		wizard.appendErr = errors.New("db down")
		h := newHarness(t, wizard)
		reply := h.send(t, "/addrss hdsky | feed | https://rss.example/x.xml")
		assert.Contains(t, reply.Text, "添加 RSS 订阅失败")
	})
}

func TestHandleAddrssPickSite_ListError(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &errSiteService{listErr: errors.New("db down")}, RSSWizard: newAddrssFakeRSSWizard(), Sessions: s})

	reply, err := handleAddrssPickSite(context.Background(), chatops.Source{ReplyLang: "zh"}, addrssWizardState{Step: addrssStepPickSite}, "hdsky")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "查询站点失败")
}

func TestHandleDelrssPickSite_ListError(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &errSiteService{listErr: errors.New("db down")}, RSSWizard: delrssWizardWithRSS(), Sessions: s})

	reply, err := handleDelrssPickSite(context.Background(), chatops.Source{ReplyLang: "zh"}, delrssWizardState{Step: delrssStepPickSite}, "hdsky")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "查询站点失败")
}

func TestDelrssConfirm_DeleteError(t *testing.T) {
	wizard := delrssWizardWithRSS()
	wizard.deleteErr = errors.New("delete boom")
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runDelrssToConfirm(t, h, "hdsky", "beta")
	reply := h.send(t, "YES")
	assert.Contains(t, reply.Text, "删除 RSS 订阅失败")
}

func TestDelrssStepHandler_InvalidStateAndUnknownStep(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &mockSiteService{sites: addrssEnabledSites()}, RSSWizard: delrssWizardWithRSS(), Sessions: s})
	src := chatops.Source{ReplyLang: "zh", ChannelType: "tg", ChannelUserID: "u"}

	bad, err := delrssStepHandler(context.Background(), []string{"x"}, src, "{not-json")
	require.NoError(t, err)
	assert.Contains(t, bad.Text, "向导状态已损坏")

	unknown, err := delrssStepHandler(context.Background(), []string{"x"}, src, `{"step":"bogus"}`)
	require.NoError(t, err)
	assert.Contains(t, unknown.Text, "未知向导步骤")
}

func TestHandleDelrssConfirm_DeletedNameFallback(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	// rssList empty, so DeleteRSSFromSite returns an empty-Name config; the
	// handler must fall back to st.RSSName.
	setupServices(t, &Services{RSSWizard: wizard})
	reply, err := handleDelrssConfirm(context.Background(), chatops.Source{ReplyLang: "zh"},
		delrssWizardState{Step: delrssStepConfirm, Site: "hdsky", RSSID: 7, RSSName: "fallback-name"}, "YES")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "fallback-name")
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

func TestHandleAddrssRSSName_Empty(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &mockSiteService{sites: addrssEnabledSites()}, RSSWizard: newAddrssFakeRSSWizard(), Sessions: s})

	reply, err := handleAddrssRSSName(context.Background(),
		chatops.Source{ReplyLang: "zh", ChannelType: "tg", ChannelUserID: "u"},
		addrssWizardState{Step: addrssStepRSSName, Site: "hdsky"}, "")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "RSS 订阅名不能为空")
}

func TestHandleAddrssFilterMode_Invalid(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &mockSiteService{sites: addrssEnabledSites()}, RSSWizard: newAddrssFakeRSSWizard(), Sessions: s})

	reply, err := handleAddrssFilterMode(context.Background(),
		chatops.Source{ReplyLang: "zh", ChannelType: "tg", ChannelUserID: "u"},
		addrssWizardState{Step: addrssStepFilterMode, Site: "hdsky"}, "bogus_mode")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "过滤模式无效")
}

func TestHandleAddrssMaxNotify_Invalid(t *testing.T) {
	s := chatops.NewSessionStore()
	t.Cleanup(s.Stop)
	setupServices(t, &Services{Site: &mockSiteService{sites: addrssEnabledSites()}, RSSWizard: newAddrssFakeRSSWizard(), Sessions: s})

	reply, err := handleAddrssMaxNotify(context.Background(),
		chatops.Source{ReplyLang: "zh", ChannelType: "tg", ChannelUserID: "u"},
		addrssWizardState{Step: addrssStepMaxNotify, Site: "hdsky"}, "-3")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "每小时通知上限")
}

func TestHandleAddrssConfirm_Cancel(t *testing.T) {
	setupServices(t, &Services{RSSWizard: newAddrssFakeRSSWizard()})
	reply, err := handleAddrssConfirm(context.Background(), chatops.Source{ReplyLang: "zh"},
		addrssWizardState{Step: addrssStepConfirm}, "no")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消添加")
}

func TestHandleAddrssConfirm_AppendError(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	wizard.appendErr = errors.New("db down")
	setupServices(t, &Services{RSSWizard: wizard})
	reply, err := handleAddrssConfirm(context.Background(), chatops.Source{ReplyLang: "zh"},
		addrssWizardState{Step: addrssStepConfirm, Site: "hdsky", RSSName: "x", RSSURL: "https://x/f"}, "YES")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "添加 RSS 订阅失败")
}

func TestHandleDelrssConfirm_Cancel(t *testing.T) {
	setupServices(t, &Services{RSSWizard: delrssWizardWithRSS()})
	reply, err := handleDelrssConfirm(context.Background(), chatops.Source{ReplyLang: "zh"},
		delrssWizardState{Step: delrssStepConfirm}, "no")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消删除")
}

func TestResolveDownloaderSelection_Branches(t *testing.T) {
	dls := []DownloaderOption{{ID: 22, Name: "qb-main"}, {ID: 23, Name: "tr-backup"}}

	got, ok := resolveDownloaderSelection("qb-main", dls)
	require.True(t, ok)
	assert.Equal(t, uint(22), got.ID)

	got, ok = resolveDownloaderSelection("23", dls)
	require.True(t, ok)
	assert.Equal(t, uint(23), got.ID)

	got, ok = resolveDownloaderSelection("2", dls)
	require.True(t, ok)
	assert.Equal(t, uint(23), got.ID, "positional index resolves to 2nd downloader")

	_, ok = resolveDownloaderSelection("", dls)
	assert.False(t, ok)

	_, ok = resolveDownloaderSelection("nope", dls)
	assert.False(t, ok)
}

func TestValidAddrssRSSURL_Branches(t *testing.T) {
	assert.False(t, validAddrssRSSURL(""))
	assert.False(t, validAddrssRSSURL("ftp://host/x"))
	assert.False(t, validAddrssRSSURL("http://"))
	assert.False(t, validAddrssRSSURL("https://rss.m-team.xxx"))
	assert.True(t, validAddrssRSSURL("https://rss.example.com/feed.xml"))
	assert.True(t, validAddrssRSSURL("http://host:9090/rss"))
}

func TestDuplicateRSSURL_Branches(t *testing.T) {
	t.Run("nil DB returns false", func(t *testing.T) {
		setupServices(t, &Services{})
		dup, err := duplicateRSSURL(context.Background(), "hdsky", "https://x/feed")
		require.NoError(t, err)
		assert.False(t, dup)
	})

	t.Run("detects duplicate against seeded DB", func(t *testing.T) {
		db := setupAddrssGlobalDB(t)
		site := seedAddrssSite(t, db, "hdsky")
		require.NoError(t, db.DB.Create(&models.RSSSubscription{
			SiteID: site.ID, Name: "feed", URL: "https://rss.example/dup.xml", IntervalMinutes: 5,
		}).Error)

		dup, err := duplicateRSSURL(context.Background(), "hdsky", "https://rss.example/dup.xml")
		require.NoError(t, err)
		assert.True(t, dup)

		notDup, err := duplicateRSSURL(context.Background(), "hdsky", "https://rss.example/other.xml")
		require.NoError(t, err)
		assert.False(t, notDup)
	})

	t.Run("unknown site returns false", func(t *testing.T) {
		setupAddrssGlobalDB(t)
		dup, err := duplicateRSSURL(context.Background(), "no-such-site", "https://x/feed")
		require.NoError(t, err)
		assert.False(t, dup)
	})
}
