// MIT License
// Copyright (c) 2025 pt-tools

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

func TestHandleDelrssConfirm_Cancel(t *testing.T) {
	setupServices(t, &Services{RSSWizard: delrssWizardWithRSS()})
	reply, err := handleDelrssConfirm(context.Background(), chatops.Source{ReplyLang: "zh"},
		delrssWizardState{Step: delrssStepConfirm}, "no")
	require.NoError(t, err)
	assert.Contains(t, reply.Text, "已取消删除")
}

func delrssWizardWithRSS() *addrssFakeRSSWizard {
	w := newAddrssFakeRSSWizard()
	w.rssList = []models.RSSConfig{
		{ID: 5, Name: "alpha", URL: "https://rss.example/alpha.xml"},
		{ID: 9, Name: "beta", URL: "https://rss.example/beta.xml"},
	}
	return w
}

func runDelrssToConfirm(t *testing.T, h *addrssHarness, sitePick, rssPick string) {
	t.Helper()
	assert.Contains(t, h.send(t, "/delrss").Text, "请选择要删除 RSS 的站点")
	assert.Contains(t, h.send(t, sitePick).Text, "请选择要删除的 RSS 订阅")
	assert.Contains(t, h.send(t, rssPick).Text, "回复 YES")
}

func TestDelrssWizardPickByNameAndConfirm(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runDelrssToConfirm(t, h, "hdsky", "beta")
	confirm := h.replies.last()
	assert.Contains(t, confirm.Text, "名称: beta")
	assert.Contains(t, confirm.Text, "ID: 9")
	assert.Contains(t, h.send(t, "YES").Text, "已删除 RSS 订阅")

	calls := wizard.deleteCallsList()
	require.Len(t, calls, 1)
	assert.Equal(t, "hdsky", calls[0].site)
	assert.Equal(t, uint(9), calls[0].rssID)
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestDelrssWizardPickByListNumber(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	listPrompt := h.send(t, "/delrss")
	assert.Contains(t, listPrompt.Text, "请选择要删除 RSS 的站点")
	rssPrompt := h.send(t, "hdsky").Text
	assert.Contains(t, rssPrompt, "1. alpha（ID: 5）")
	assert.Contains(t, rssPrompt, "2. beta（ID: 9）")

	assert.Contains(t, h.send(t, "1").Text, "回复 YES")
	assert.Contains(t, h.send(t, "YES").Text, "已删除 RSS 订阅")

	calls := wizard.deleteCallsList()
	require.Len(t, calls, 1)
	assert.Equal(t, uint(5), calls[0].rssID)
}

func TestDelrssWizardPickByID(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runDelrssToConfirm(t, h, "1", "9")
	assert.Contains(t, h.send(t, "YES").Text, "已删除 RSS 订阅")

	calls := wizard.deleteCallsList()
	require.Len(t, calls, 1)
	assert.Equal(t, uint(9), calls[0].rssID)
}

func TestDelrssWizardCancelKeepsRSS(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runDelrssToConfirm(t, h, "hdsky", "alpha")
	assert.Contains(t, h.send(t, "NO").Text, "已取消删除")
	assert.Empty(t, wizard.deleteCallsList())
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestDelrssWizardInvalidRSSStaysOnStep(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	h.send(t, "/delrss")
	h.send(t, "hdsky")
	reply := h.send(t, "nonexistent")
	assert.Contains(t, reply.Text, "RSS 订阅不存在")
	assert.Empty(t, wizard.deleteCallsList())
	state, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	require.True(t, pending)
	assert.Equal(t, delrssStepPickRSS, state.Step)
}

func TestDelrssWizardEmptyListEndsWizard(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	h.send(t, "/delrss")
	reply := h.send(t, "hdsky")
	assert.Contains(t, reply.Text, "没有 RSS 订阅")
	assert.Empty(t, wizard.deleteCallsList())
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestDelrssWizardNoEnabledSites(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", true, []app.SiteSummaryDTO{{Name: "hdsky", Enabled: false}}, wizard)

	reply := h.send(t, "/delrss")
	assert.Contains(t, reply.Text, "Web 界面配置并启用站点")
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
	assert.Empty(t, wizard.deleteCallsList())
}

func TestDelrssWizardAuthGateRequiresAdmin(t *testing.T) {
	wizard := delrssWizardWithRSS()
	h := newAddrssHarness(t, "telegram", false, addrssEnabledSites(), wizard)

	reply := h.send(t, "/delrss")
	assert.Contains(t, reply.Text, "管理员权限")
	assert.Empty(t, wizard.deleteCallsList())
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestDelrssWizardCrossChannelParity(t *testing.T) {
	run := func(t *testing.T, channel string) []string {
		wizard := delrssWizardWithRSS()
		h := newAddrssHarness(t, channel, true, addrssEnabledSites(), wizard)
		runDelrssToConfirm(t, h, "hdsky", "alpha")
		assert.Contains(t, h.send(t, "YES").Text, "已删除 RSS 订阅")
		for _, reply := range h.replies.replies {
			assert.Empty(t, reply.Buttons)
		}
		return h.replies.texts()
	}
	telegramReplies := run(t, "telegram")
	qqReplies := run(t, "qq")
	assert.Equal(t, telegramReplies, qqReplies)
}
