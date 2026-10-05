package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/cloakdriver/transport"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/web"
)

func TestWireLoginReminderMonitor_RegistersNonNil(t *testing.T) {
	global.InitLogger(zap.NewNop())

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SiteSetting{},
		&models.SiteLoginState{},
		&models.NotificationConf{},
	))

	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })

	store := core.NewConfigStore(global.GlobalDB)
	siteRegistry := v2.NewSiteRegistry(global.GetLogger())
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)

	require.Nil(t, mgr.GetLoginReminderMonitor(),
		"precondition: monitor must be nil before wiring (this nil is the 503 cause)")

	wireLoginReminderMonitor(mgr, store, siteRegistry, nil, nil)

	require.NotNil(t, mgr.GetLoginReminderMonitor(),
		"after wiring, GetLoginReminderMonitor must be non-nil so the probe endpoint stops returning 503")
}

func TestLoginReminderDecryptor_Decrypt(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	d := loginReminderDecryptor{store: store}

	// Empty cookie -> empty plaintext, no error.
	plain, err := d.Decrypt(models.SiteSetting{})
	require.NoError(t, err)
	assert.Empty(t, plain)

	// Round-trip a real cookie through the store's encrypt/decrypt.
	cipher, err := store.EncryptCookie("session=abc123")
	require.NoError(t, err)
	got, err := d.Decrypt(models.SiteSetting{CookieEncrypted: cipher})
	require.NoError(t, err)
	assert.Equal(t, "session=abc123", got)
}

func TestLoginReminderResolver_Resolve_DecryptError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	resolver := loginReminderResolver{
		registry:  v2.NewSiteRegistry(global.GetLogger()),
		decryptor: loginReminderDecryptor{store: store},
	}

	// A non-base64 / undecryptable cookie must surface as an error from Resolve.
	_, _, err = resolver.Resolve(models.SiteSetting{Name: "hdsky", CookieEncrypted: "not-valid-cipher"})
	require.Error(t, err)
}

func TestLoginReminderResolver_Resolve_UnknownSite(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	global.GlobalDB = &models.TorrentDB{DB: db}
	store := core.NewConfigStore(global.GlobalDB)

	resolver := loginReminderResolver{
		registry:  v2.NewSiteRegistry(global.GetLogger()),
		decryptor: loginReminderDecryptor{store: store},
	}

	// Empty cookie decrypts fine (empty); an unregistered site name makes
	// CreateSite fail, so Resolve returns an error.
	_, _, err = resolver.Resolve(models.SiteSetting{Name: "does-not-exist"})
	require.Error(t, err)
}

// fakeNotificationSvc 只实现 PushSync / Push；其余方法沿用嵌入的 nil 接口，被调用就会 panic，
// 用来证明登录监控只走 PushSync。
type fakeNotificationSvc struct {
	app.NotificationService
	pushSync atomic.Int32
	push     atomic.Int32
	lastConf atomic.Uint32
}

func (f *fakeNotificationSvc) PushSync(_ context.Context, n app.Notification) error {
	f.pushSync.Add(1)
	f.lastConf.Store(uint32(n.SourceConfID))
	return nil
}

func (f *fakeNotificationSvc) Push(context.Context, app.Notification) error {
	f.push.Add(1)
	return nil
}

type countingChannel struct{}

func (countingChannel) Type() string                                         { return "counting" }
func (countingChannel) Init(context.Context, *models.NotificationConf) error { return nil }
func (countingChannel) SupportsInbound() bool                                { return false }
func (countingChannel) Send(context.Context, notify.Notification) error      { return nil }
func (countingChannel) OnInbound(notify.InboundHandler)                      {}
func (countingChannel) Close(context.Context) error                          { return nil }
func (countingChannel) Healthy() bool                                        { return true }

// M1 问题 12：登录监控经 ChatOps 的 live 通道发送，不再用 registry.Make 另起通道实例。
func TestWireLoginReminderMonitor_UsesLiveChannelsNotNewInstances(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.NotificationConf{ChannelType: "counting", Name: "c", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie"}).Error)

	var makes atomic.Int32
	registry := notify.NewRegistry()
	registry.Register("counting", func() notify.Channel { makes.Add(1); return countingChannel{} })
	svc := &fakeNotificationSvc{}
	bs := &chatopsBootstrap{registry: registry, deps: &web.ChatOpsDeps{NotificationSvc: svc}}

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), bs, nil)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)
	require.True(t, mon.Notifier().Enabled())

	require.NoError(t, mon.SendTestReminder(context.Background(), "hdsky"))
	assert.Equal(t, int32(1), svc.pushSync.Load(), "delivered through the live manager")
	assert.Equal(t, int32(0), svc.push.Load(), "Push would fall back to the outbox, which cannot drive QQ/Telegram")
	assert.Equal(t, int32(0), makes.Load(), "no channel instance is created for login notifications")
	assert.Equal(t, uint32(1), svc.lastConf.Load())
}

func TestWireLoginReminderMonitor_WithoutChatOpsRecordsOnly(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), nil, nil)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)
	assert.False(t, mon.Notifier().Enabled())
	assert.Nil(t, loginReminderSender(nil))
	assert.Nil(t, loginReminderSender(&chatopsBootstrap{}))
	assert.ErrorContains(t, mon.SendTestReminder(context.Background(), "hdsky"), "通知")
}

// wiringProbeSite 是注册进 UserInfoService 的假站点，不发网络请求。
type wiringProbeSite struct {
	calls   atomic.Int32
	attends atomic.Int32
	info    v2.UserInfo
}

func (s *wiringProbeSite) ID() string                                       { return "hdsky" }
func (s *wiringProbeSite) Name() string                                     { return "HDSky" }
func (s *wiringProbeSite) Kind() v2.SiteKind                                { return v2.SiteNexusPHP }
func (s *wiringProbeSite) Login(context.Context, v2.Credentials) error      { return nil }
func (s *wiringProbeSite) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (s *wiringProbeSite) Close() error                                     { return nil }

func (s *wiringProbeSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (s *wiringProbeSite) GetUserInfo(context.Context) (v2.UserInfo, error) {
	s.calls.Add(1)
	return s.info, nil
}

func (s *wiringProbeSite) SupportsAttendance() bool { return true }

func (s *wiringProbeSite) Attend(context.Context) (v2.AttendResult, error) {
	s.attends.Add(1)
	return v2.AttendResult{Status: v2.AttendSigned, Message: "签到成功"}, nil
}

// M1b：生产接线把 UserInfoService 交给监控，探测走已注册的共享实例并刷新统计。
func TestWireLoginReminderMonitor_ProbesThroughUserInfoService(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "c=1"}).Error)

	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})
	site := &wiringProbeSite{info: v2.UserInfo{Site: "hdsky", Username: "tester", Uploaded: 7, LastAccess: time.Now().Add(-time.Hour).Unix()}}
	svc.RegisterSite(site)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), nil, svc)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)

	require.True(t, mon.RunProbeOnceForSite(context.Background(), "hdsky"))
	assert.Equal(t, int32(1), site.calls.Load(), "the registered shared instance was used")
	got, err := svc.GetUserInfo(context.Background(), "hdsky")
	require.NoError(t, err)
	assert.Equal(t, int64(7), got.Uploaded)
	var st models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "hdsky").First(&st).Error)
	assert.Equal(t, "OK", st.LastProbeStatus)
}

// M1c：接线同时启动每日签到监控，签到用 UserInfoService 里的共享实例。
func TestWireLoginReminderMonitor_WiresAttendance(t *testing.T) {
	global.InitLogger(zap.NewNop())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}, &models.SiteAttendanceLog{}, &models.SettingsGlobal{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "c=1"}).Error)

	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})
	site := &wiringProbeSite{}
	svc.RegisterSite(site)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, core.NewConfigStore(global.GlobalDB), v2.NewSiteRegistry(global.GetLogger()), nil, svc)
	att := mgr.GetAttendanceMonitor()
	require.NotNil(t, att)

	row, err := att.SignNow(context.Background(), "hdsky")
	require.NoError(t, err)
	assert.Equal(t, models.AttendanceSigned, row.Status)
	assert.Equal(t, int32(1), site.attends.Load())

	_, ok := attendanceSites(nil).AttendanceSite("hdsky")
	assert.False(t, ok)
	_, ok = attendanceSites(svc).AttendanceSite("missing")
	assert.False(t, ok)
}

func TestLoginReminderUserInfo_NilStaysNil(t *testing.T) {
	assert.Nil(t, loginReminderUserInfo(nil), "a nil service must not become a non-nil interface")
	assert.NotNil(t, loginReminderUserInfo(v2.NewUserInfoService(v2.UserInfoServiceConfig{})))
}

// M1d：生产接线把 CloakBrowser 后备交给监控。主通道用真实的 NexusPHP 驱动，站点返回 Cloudflare 质询页时
// 记为被拦截，按库里的三项配置向 Manager 启动 profile（带上 token）；Manager 拒绝时保留主通道的状态并附上后备的原因。
// 没有 Profile ID 时不走后备。
func TestWireLoginReminderMonitor_UsesCloakFallback(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	t.Setenv("ALL_PROXY", "")
	t.Setenv("all_proxy", "")
	var trackerHits atomic.Int32
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		trackerHits.Add(1)
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!DOCTYPE html><title>Just a moment...</title>`))
	}))
	t.Cleanup(tracker.Close)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.NotificationConf{},
		&models.MigrationState{}, &models.MonitorNotificationLog{}, &models.SiteAttendanceLog{}, &models.SettingsGlobal{},
		&models.CloakSettings{}))
	prevDB := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prevDB })
	require.NoError(t, db.Create(&models.SiteSetting{
		Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "uid=93012; pass=abc", APIUrl: tracker.URL,
	}).Error)

	var launches atomic.Int32
	var gotPath, gotAuth atomic.Value
	manager := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		launches.Add(1)
		gotPath.Store(r.URL.Path)
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(manager.Close)
	store := core.NewConfigStore(global.GlobalDB)
	profile := "profile-1"
	require.NoError(t, store.SaveCloakConfig(manager.URL, "cloak-token", false, &profile))

	repo := v2.NewInMemoryUserInfoRepo()
	require.NoError(t, repo.Save(context.Background(), v2.UserInfo{Site: "hdsky", UserID: "93012", Username: "tester"}))
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: repo})
	registry := v2.NewSiteRegistry(global.GetLogger())
	site, err := registry.CreateSite("hdsky", v2.SiteCredentials{Cookie: "uid=93012; pass=abc"}, tracker.URL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = site.Close() })
	svc.RegisterSite(site)

	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	wireLoginReminderMonitor(mgr, store, registry, nil, svc)
	mon := mgr.GetLoginReminderMonitor()
	require.NotNil(t, mon)

	require.True(t, mon.RunProbeOnceForSite(context.Background(), "hdsky"))
	assert.Positive(t, trackerHits.Load(), "the primary probe reached the site")
	assert.Equal(t, int32(1), launches.Load())
	assert.Equal(t, "/api/profiles/profile-1/launch", gotPath.Load())
	assert.Equal(t, "Bearer cloak-token", gotAuth.Load())
	var st models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "hdsky").First(&st).Error)
	assert.Equal(t, "CHALLENGE", st.LastProbeStatus)
	assert.Contains(t, st.LastProbeError, "cloudflare challenge")
	assert.Contains(t, st.LastProbeError, "CloakBrowser 后备未成功")
	assert.Contains(t, st.LastProbeError, "KEY_ERROR")

	empty := ""
	require.NoError(t, store.SaveCloakConfig(manager.URL, "", false, &empty))
	require.True(t, mon.RunProbeOnceForSite(context.Background(), "hdsky"))
	assert.Equal(t, int32(1), launches.Load(), "no profile ID, no fallback")
	require.NoError(t, db.Where("site_name = ?", "hdsky").First(&st).Error)
	assert.NotContains(t, st.LastProbeError, "CloakBrowser")
}

// 后备提供者的三个依赖都来自库与用户信息服务：Cookie 与主通道同一取法，身份信息缺失时不给 NexusPHP 后备。
func TestLoginReminderFallback_ReadsStoreAndUserInfo(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CloakSettings{}))
	store := core.NewConfigStore(&models.TorrentDB{DB: db})
	assert.Nil(t, loginReminderFallback(nil, nil))

	def, ok := v2.GetDefinitionRegistry().Get("hdsky")
	require.True(t, ok)
	cipher, err := store.EncryptCookie("uid=93012; pass=abc")
	require.NoError(t, err)
	setting := models.SiteSetting{Name: "hdsky", CookieEncrypted: cipher}
	repo := v2.NewInMemoryUserInfoRepo()
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: repo})

	provider := loginReminderFallback(store, svc)
	require.NotNil(t, provider)
	assert.Nil(t, provider.FallbackFor(context.Background(), setting, def), "nothing configured yet")

	profile := "profile-1"
	require.NoError(t, store.SaveCloakConfig("http://cloak:8080", "tok", false, &profile))
	assert.Nil(t, provider.FallbackFor(context.Background(), setting, def), "NexusPHP needs a known user ID")

	require.NoError(t, repo.Save(context.Background(), v2.UserInfo{Site: "hdsky", UserID: "93012"}))
	tr, ok := provider.FallbackFor(context.Background(), setting, def).(*transport.Transport)
	require.True(t, ok)
	assert.Equal(t, strings.TrimRight(def.URLs[0], "/")+"/userdetails.php?id=93012", tr.URL)
	assert.Equal(t, "profile-1", tr.ProfileID)
	require.Len(t, tr.Cookies, 2)
	assert.Equal(t, "uid", tr.Cookies[0].Name)

	assert.Nil(t, loginReminderFallback(store, nil).FallbackFor(context.Background(), setting, def),
		"without the user info service only M-Team could fall back")
}
