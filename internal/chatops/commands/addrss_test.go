// MIT License
// Copyright (c) 2025 pt-tools

package commands

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

type addrssAppendCall struct {
	site  string
	entry models.RSSConfig
}

type addrssFakeRSSWizard struct {
	mu            sync.Mutex
	downloaders   []DownloaderOption
	filterRules   []IDNameOption
	notifications []IDNameOption
	rssList       []models.RSSConfig
	appendCalls   []addrssAppendCall
	deleteCalls   []addrssDeleteCall
	appendErr     error
	listErr       error
	deleteErr     error
}

type addrssDeleteCall struct {
	site  string
	rssID uint
}

func newAddrssFakeRSSWizard() *addrssFakeRSSWizard {
	return &addrssFakeRSSWizard{
		downloaders:   []DownloaderOption{{ID: 22, Name: "qb-main", IsDefault: true}, {ID: 23, Name: "tr-backup"}},
		filterRules:   []IDNameOption{{ID: 1, Name: "FreeRule"}, {ID: 2, Name: "MovieRule"}},
		notifications: []IDNameOption{{ID: 10, Name: "tg-alert"}, {ID: 11, Name: "qq-alert"}},
	}
}

func (f *addrssFakeRSSWizard) AppendRSSToSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appendErr != nil {
		return models.RSSConfig{}, f.appendErr
	}
	entry.ID = uint(len(f.appendCalls) + 100)
	f.appendCalls = append(f.appendCalls, addrssAppendCall{site: siteName, entry: entry})
	return entry, nil
}

func (f *addrssFakeRSSWizard) ListDownloaders(context.Context) ([]DownloaderOption, error) {
	return append([]DownloaderOption(nil), f.downloaders...), nil
}

func (f *addrssFakeRSSWizard) ListFilterRules(context.Context) ([]IDNameOption, error) {
	return append([]IDNameOption(nil), f.filterRules...), nil
}

func (f *addrssFakeRSSWizard) ListNotificationChannels(context.Context) ([]IDNameOption, error) {
	return append([]IDNameOption(nil), f.notifications...), nil
}

func (f *addrssFakeRSSWizard) ListRSSForSite(string) ([]models.RSSConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]models.RSSConfig(nil), f.rssList...), nil
}

func (f *addrssFakeRSSWizard) DeleteRSSFromSite(siteName string, rssID uint) (models.RSSConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return models.RSSConfig{}, f.deleteErr
	}
	var found models.RSSConfig
	for _, r := range f.rssList {
		if r.ID == rssID {
			found = r
		}
	}
	f.deleteCalls = append(f.deleteCalls, addrssDeleteCall{site: siteName, rssID: rssID})
	return found, nil
}

func (f *addrssFakeRSSWizard) deleteCallsList() []addrssDeleteCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]addrssDeleteCall, len(f.deleteCalls))
	copy(out, f.deleteCalls)
	return out
}

func (f *addrssFakeRSSWizard) calls() []addrssAppendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]addrssAppendCall, len(f.appendCalls))
	copy(out, f.appendCalls)
	return out
}

type addrssStoreRSSWizard struct {
	*addrssFakeRSSWizard
	store *core.ConfigStore
}

func (w *addrssStoreRSSWizard) AppendRSSToSite(siteName string, entry models.RSSConfig) (models.RSSConfig, error) {
	created, err := w.store.AppendRSSToSite(siteName, entry)
	if err != nil {
		return models.RSSConfig{}, err
	}
	w.mu.Lock()
	w.appendCalls = append(w.appendCalls, addrssAppendCall{site: siteName, entry: entry})
	w.mu.Unlock()
	return created, nil
}

type addrssBindingLookup struct {
	info chatops.BindingInfo
	ok   bool
}

func (l *addrssBindingLookup) FindByChannelUser(context.Context, string, string) (chatops.BindingInfo, bool, error) {
	return l.info, l.ok, nil
}

type addrssBindCoder struct{}

func (addrssBindCoder) ConsumeCode(context.Context, string, string, string) error { return nil }

type addrssAuditRecorder struct{}

func (addrssAuditRecorder) Record(context.Context, chatops.AuditEntry) error { return nil }

type addrssAllowAllRateLimiter struct{}

func (addrssAllowAllRateLimiter) Allow(string, string, string) bool { return true }

type addrssReplyRecorder struct {
	mu      sync.Mutex
	replies []chatops.Reply
}

func (r *addrssReplyRecorder) Reply(_ context.Context, _ notify.InboundMessage, reply chatops.Reply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replies = append(r.replies, reply)
	return nil
}

func (r *addrssReplyRecorder) last() chatops.Reply {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.replies) == 0 {
		return chatops.Reply{}
	}
	return r.replies[len(r.replies)-1]
}

func (r *addrssReplyRecorder) texts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.replies))
	for _, reply := range r.replies {
		out = append(out, reply.Text)
	}
	return out
}

type addrssHarness struct {
	chain    *chatops.MessageChain
	sessions *chatops.SessionStore
	replies  *addrssReplyRecorder
	wizard   *addrssFakeRSSWizard
	channel  string
	confID   uint
	userID   string
}

func newAddrssHarness(t *testing.T, channel string, admin bool, sites []app.SiteSummaryDTO, wizard RSSWizardService) *addrssHarness {
	t.Helper()
	sessions := chatops.NewSessionStore()
	t.Cleanup(sessions.Stop)
	fakeWizard, _ := wizard.(*addrssFakeRSSWizard)
	if fakeWizard == nil {
		if wrapped, ok := wizard.(*addrssStoreRSSWizard); ok {
			fakeWizard = wrapped.addrssFakeRSSWizard
		}
	}
	setupServices(t, &Services{
		Site:      &mockSiteService{sites: sites},
		RSSWizard: wizard,
		Sessions:  sessions,
	})
	replies := &addrssReplyRecorder{}
	chain := chatops.NewMessageChain(
		chatops.DefaultRegistry(),
		&addrssBindingLookup{ok: true, info: chatops.BindingInfo{ID: 1, ConfID: 77, ChannelType: channel, ChannelUserID: "user-1", ReplyLang: "zh", PtAdmin: admin, Allowed: true}},
		addrssBindCoder{},
		addrssAuditRecorder{},
		addrssAllowAllRateLimiter{},
		sessions,
		replies,
	)
	return &addrssHarness{chain: chain, sessions: sessions, replies: replies, wizard: fakeWizard, channel: channel, confID: 77, userID: "user-1"}
}

func (h *addrssHarness) send(t *testing.T, text string) chatops.Reply {
	t.Helper()
	err := h.chain.Process(context.Background(), notify.InboundMessage{
		ChannelType:   h.channel,
		SourceConfID:  h.confID,
		ChannelUserID: h.userID,
		Username:      "alice",
		ChatID:        "chat-1",
		Text:          text,
	})
	require.NoError(t, err)
	reply := h.replies.last()
	require.Empty(t, reply.Buttons)
	return reply
}

func addrssEnabledSites() []app.SiteSummaryDTO {
	return []app.SiteSummaryDTO{{Name: "hdsky", Enabled: true}, {Name: "mteam", Enabled: false}}
}

func runAddrssToConfirm(t *testing.T, h *addrssHarness, name, rawURL, downloader string, optionals []string) chatops.Reply {
	t.Helper()
	assert.Contains(t, h.send(t, "/addrss").Text, "请选择要添加 RSS 的站点")
	assert.Contains(t, h.send(t, "hdsky").Text, "请输入 RSS 订阅名")
	assert.Contains(t, h.send(t, name).Text, "请粘贴完整 RSS URL")
	assert.Contains(t, h.send(t, rawURL).Text, "请选择下载器")
	assert.Contains(t, h.send(t, downloader).Text, "分类")
	for _, input := range optionals {
		h.send(t, input)
	}
	reply := h.replies.last()
	assert.Contains(t, reply.Text, "回复 YES")
	return reply
}

func runAddrssSkipPathToConfirm(t *testing.T, h *addrssHarness, name, rawURL, downloader string) chatops.Reply {
	t.Helper()
	return runAddrssToConfirm(t, h, name, rawURL, downloader, []string{"skip", "skip", "skip", "skip", "skip", "skip", "skip", "skip"})
}

func TestAddrssWizardHappyPathFullFields(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runAddrssToConfirm(t, h, "Weekly Movies", "https://rss.example/full.xml", "qb-main", []string{
		"movies", "uhd", "/data/movies", "filter_only", "FreeRule, 2", "match", "tg-alert, 11", "7",
	})
	finalReply := h.send(t, "YES")
	assert.Contains(t, finalReply.Text, "已添加 RSS 订阅")

	calls := wizard.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "hdsky", calls[0].site)
	entry := calls[0].entry
	assert.Equal(t, "Weekly Movies", entry.Name)
	assert.Equal(t, "https://rss.example/full.xml", entry.URL)
	assert.Equal(t, "movies", entry.Category)
	assert.Equal(t, "uhd", entry.Tag)
	assert.Equal(t, "/data/movies", entry.DownloadPath)
	require.NotNil(t, entry.DownloaderID)
	assert.Equal(t, uint(22), *entry.DownloaderID)
	assert.Equal(t, models.FilterModeFilterOnly, entry.FilterMode)
	assert.Equal(t, []uint{1, 2}, entry.FilterRuleIDs)
	assert.Equal(t, "match", entry.NotifyMode)
	assert.Equal(t, "[10,11]", entry.NotifyConfIDs)
	assert.Equal(t, 7, entry.MaxNotificationsPerHour)
}

func TestAddrssWizardAllSkipPath(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runAddrssSkipPathToConfirm(t, h, "Skip Feed", "https://rss.example/skip.xml", "qb-main")
	assert.Contains(t, h.send(t, "YES").Text, "已添加 RSS 订阅")

	calls := wizard.calls()
	require.Len(t, calls, 1)
	entry := calls[0].entry
	assert.Equal(t, "hdsky", calls[0].site)
	assert.Equal(t, "Skip Feed", entry.Name)
	assert.Equal(t, "https://rss.example/skip.xml", entry.URL)
	require.NotNil(t, entry.DownloaderID)
	assert.Empty(t, entry.Category)
	assert.Empty(t, entry.Tag)
	assert.Empty(t, entry.DownloadPath)
	assert.Empty(t, entry.FilterMode)
	assert.Empty(t, entry.FilterRuleIDs)
	assert.Empty(t, entry.NotifyMode)
	assert.Equal(t, "[]", entry.NotifyConfIDs)
	assert.Zero(t, entry.MaxNotificationsPerHour)
}

func TestAddrssWizardDownloaderDefault(t *testing.T) {
	for _, input := range []string{"", "默认"} {
		t.Run("input="+input, func(t *testing.T) {
			wizard := newAddrssFakeRSSWizard()
			h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

			runAddrssSkipPathToConfirm(t, h, "Default DL", "https://rss.example/default.xml", input)
			assert.Contains(t, h.send(t, "YES").Text, "已添加 RSS 订阅")

			calls := wizard.calls()
			require.Len(t, calls, 1)
			assert.Nil(t, calls[0].entry.DownloaderID)
		})
	}
}

func TestAddrssWizardNoEnabledSites(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, []app.SiteSummaryDTO{{Name: "hdsky", Enabled: false}}, wizard)

	reply := h.send(t, "/addrss")
	assert.Contains(t, reply.Text, "Web 界面配置并启用站点")
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
	assert.Empty(t, wizard.calls())
}

func TestAddrssWizardDuplicateURLStaysOnURLStep(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	db := setupAddrssGlobalDB(t)
	seedAddrssSite(t, db, "hdsky")
	var site models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "hdsky").First(&site).Error)
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: site.ID, Name: "exists", URL: "https://rss.example/dup.xml", IntervalMinutes: models.MinIntervalMinutes}).Error)
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	assert.Contains(t, h.send(t, "/addrss").Text, "请选择")
	assert.Contains(t, h.send(t, "hdsky").Text, "订阅名")
	assert.Contains(t, h.send(t, "Dup Feed").Text, "RSS URL")
	reply := h.send(t, " https://rss.example/DUP.xml ")
	assert.Contains(t, reply.Text, "已存在相同 RSS URL")
	assert.Empty(t, wizard.calls())
	state, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	require.True(t, pending)
	assert.Equal(t, addrssStepRSSURL, state.Step)
}

func TestAddrssWizardInvalidURLStaysOnURLStep(t *testing.T) {
	for _, input := range []string{"not-a-url", "ftp://x"} {
		t.Run(input, func(t *testing.T) {
			wizard := newAddrssFakeRSSWizard()
			h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

			h.send(t, "/addrss")
			h.send(t, "hdsky")
			h.send(t, "Invalid Feed")
			reply := h.send(t, input)
			assert.Contains(t, reply.Text, "RSS URL 无效")
			assert.Empty(t, wizard.calls())
			state, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
			require.True(t, pending)
			assert.Equal(t, addrssStepRSSURL, state.Step)
		})
	}
}

func TestAddrssWizardSessionTimeoutMidWizard(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)
	h.send(t, "/addrss")
	state, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	require.True(t, pending)
	h.sessions.Set(h.channel, h.confID, h.userID, state, time.Nanosecond)
	time.Sleep(time.Millisecond)

	reply := h.send(t, "hdsky")
	assert.Contains(t, reply.Text, "/help")
	assert.Empty(t, wizard.calls())
	_, pending = h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestAddrssWizardCancelAtConfirmClearsSession(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runAddrssSkipPathToConfirm(t, h, "Cancel Feed", "https://rss.example/cancel.xml", "qb-main")
	reply := h.send(t, "NO")
	assert.Contains(t, reply.Text, "已取消")
	assert.Empty(t, wizard.calls())
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func TestAddrssWizardAtomicAppendKeepsConcurrentRSS(t *testing.T) {
	db := setupAddrssGlobalDB(t)
	store := core.NewConfigStore(db)
	site := seedAddrssSite(t, db, "hdsky")
	wizard := &addrssStoreRSSWizard{addrssFakeRSSWizard: newAddrssFakeRSSWizard(), store: store}
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	runAddrssSkipPathToConfirm(t, h, "Atomic Feed", "https://rss.example/atomic.xml", "默认")
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: site.ID, Name: "concurrent", URL: "https://rss.example/concurrent.xml", IntervalMinutes: models.MinIntervalMinutes}).Error)
	reply := h.send(t, "YES")
	assert.Contains(t, reply.Text, "已添加 RSS 订阅")

	var rows []models.RSSSubscription
	require.NoError(t, db.DB.Where("site_id = ?", site.ID).Order("name asc").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.ElementsMatch(t, []string{"Atomic Feed", "concurrent"}, []string{rows[0].Name, rows[1].Name})
}

func TestAddrssWizardCrossChannelParity(t *testing.T) {
	run := func(t *testing.T, channel string) []string {
		wizard := newAddrssFakeRSSWizard()
		h := newAddrssHarness(t, channel, true, addrssEnabledSites(), wizard)
		runAddrssSkipPathToConfirm(t, h, "Parity Feed", "https://rss.example/parity.xml", "默认")
		assert.Contains(t, h.send(t, "YES").Text, "已添加 RSS 订阅")
		for _, reply := range h.replies.replies {
			assert.Empty(t, reply.Buttons)
		}
		return h.replies.texts()
	}

	telegramReplies := run(t, "telegram")
	qqReplies := run(t, "qq")
	assert.Equal(t, telegramReplies, qqReplies)
}

func TestAddrssWizardAuthGateRequiresAdmin(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", false, addrssEnabledSites(), wizard)

	reply := h.send(t, "/addrss")
	assert.Contains(t, reply.Text, "管理员权限")
	assert.Empty(t, wizard.calls())
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)
}

func addrssMultiEnabledSites() []app.SiteSummaryDTO {
	return []app.SiteSummaryDTO{
		{Name: "hdsky", Enabled: true},
		{Name: "hddolby", Enabled: true},
		{Name: "mteam", Enabled: false},
	}
}

func TestAddrssWizardNumericSiteSelection(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssMultiEnabledSites(), wizard)

	prompt := h.send(t, "/addrss").Text
	assert.Contains(t, prompt, "回复站点名或序号")
	assert.Contains(t, prompt, "1. hddolby")
	assert.Contains(t, prompt, "2. hdsky")

	assert.Contains(t, h.send(t, "1").Text, "请输入 RSS 订阅名")
	assert.Contains(t, h.send(t, "Numeric Site").Text, "请粘贴完整 RSS URL")
	assert.Contains(t, h.send(t, "https://rss.example/numsite.xml").Text, "请选择下载器")
	for _, in := range []string{"默认", "skip", "skip", "skip", "skip", "skip", "skip", "skip", "skip"} {
		h.send(t, in)
	}
	assert.Contains(t, h.send(t, "YES").Text, "已添加 RSS 订阅")

	calls := wizard.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "hddolby", calls[0].site)
}

func TestAddrssWizardNumericDownloaderSelection(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	h.send(t, "/addrss")
	h.send(t, "hdsky")
	h.send(t, "Numeric DL")
	dlPrompt := h.send(t, "https://rss.example/numdl.xml").Text
	assert.Contains(t, dlPrompt, "回复名称或序号")
	assert.Contains(t, dlPrompt, "1. qb-main")
	assert.Contains(t, dlPrompt, "2. tr-backup")

	assert.Contains(t, h.send(t, "2").Text, "分类")
	for _, in := range []string{"skip", "skip", "skip", "skip", "skip", "skip", "skip", "skip"} {
		h.send(t, in)
	}
	assert.Contains(t, h.send(t, "YES").Text, "已添加 RSS 订阅")

	calls := wizard.calls()
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].entry.DownloaderID)
	assert.Equal(t, uint(23), *calls[0].entry.DownloaderID)
}

func TestAddrssShortcutHappyPath(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)

	reply := h.send(t, "/addrss hdsky | Quick Feed | https://rss.example/quick.xml | tr-backup")
	assert.Contains(t, reply.Text, "已添加 RSS 订阅")
	assert.Empty(t, reply.Buttons)
	_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
	assert.False(t, pending)

	calls := wizard.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "hdsky", calls[0].site)
	assert.Equal(t, "Quick Feed", calls[0].entry.Name)
	assert.Equal(t, "https://rss.example/quick.xml", calls[0].entry.URL)
	require.NotNil(t, calls[0].entry.DownloaderID)
	assert.Equal(t, uint(23), *calls[0].entry.DownloaderID)
}

func TestAddrssShortcutNumericSiteAndDownloader(t *testing.T) {
	wizard := newAddrssFakeRSSWizard()
	h := newAddrssHarness(t, "telegram", true, addrssMultiEnabledSites(), wizard)

	reply := h.send(t, "/addrss 1 | Num Quick | https://rss.example/numquick.xml | 1")
	assert.Contains(t, reply.Text, "已添加 RSS 订阅")

	calls := wizard.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "hddolby", calls[0].site)
	require.NotNil(t, calls[0].entry.DownloaderID)
	assert.Equal(t, uint(22), *calls[0].entry.DownloaderID)
}

func TestAddrssShortcutInvalidShowsExample(t *testing.T) {
	t.Run("missing url", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)
		reply := h.send(t, "/addrss hdsky | OnlyName")
		assert.Contains(t, reply.Text, "至少需要")
		assert.Contains(t, reply.Text, "/addrss 站点 | 订阅名 | RSS地址")
		assert.Empty(t, wizard.calls())
		_, pending := h.sessions.Pending(h.channel, h.confID, h.userID)
		assert.False(t, pending)
	})

	t.Run("bad url", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)
		reply := h.send(t, "/addrss hdsky | Name | not-a-url")
		assert.Contains(t, reply.Text, "RSS URL 无效")
		assert.Empty(t, wizard.calls())
	})

	t.Run("unknown site", func(t *testing.T) {
		wizard := newAddrssFakeRSSWizard()
		h := newAddrssHarness(t, "telegram", true, addrssEnabledSites(), wizard)
		reply := h.send(t, "/addrss nosuchsite | Name | https://rss.example/x.xml")
		assert.Contains(t, reply.Text, "站点不存在或未启用")
		assert.Empty(t, wizard.calls())
	})
}

func setupAddrssGlobalDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	prev := global.GlobalDB
	global.GlobalDB = db
	t.Cleanup(func() { global.GlobalDB = prev })
	return db
}

func seedAddrssSite(t *testing.T, db *models.TorrentDB, name string) models.SiteSetting {
	t.Helper()
	site := models.SiteSetting{Name: name, Enabled: true, AuthMethod: "cookie", APIUrl: "https://" + name + ".example"}
	require.NoError(t, db.DB.Create(&site).Error)
	return site
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
