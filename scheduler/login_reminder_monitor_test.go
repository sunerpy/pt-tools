package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func newReminderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteLoginState{}, &models.MigrationState{}))
	return db
}

type fakeReminderSite struct {
	info v2.UserInfo
	err  error
}

func (f *fakeReminderSite) ID() string                                  { return "fake" }
func (f *fakeReminderSite) Name() string                                { return "Fake" }
func (f *fakeReminderSite) Kind() v2.SiteKind                           { return v2.SiteNexusPHP }
func (f *fakeReminderSite) Login(context.Context, v2.Credentials) error { return nil }
func (f *fakeReminderSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return nil, nil
}

func (f *fakeReminderSite) GetUserInfo(context.Context) (v2.UserInfo, error) { return f.info, f.err }
func (f *fakeReminderSite) Download(context.Context, string) ([]byte, error) { return nil, nil }
func (f *fakeReminderSite) Close() error                                     { return nil }

type fakeReminderResolver struct {
	def  *v2.SiteDefinition
	site v2.Site
	err  error
}

func (f *fakeReminderResolver) Resolve(setting models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	return f.def, f.site, f.err
}

type fakeDecryptor struct{}

func (fakeDecryptor) Decrypt(setting models.SiteSetting) (string, error) {
	return setting.Cookie, nil
}

type captureRouter struct {
	count atomic.Int32
	last  notify.Notification
}

func (c *captureRouter) ListNotificationConfs(context.Context) ([]models.NotificationConf, error) {
	return []models.NotificationConf{{ID: 1, ChannelType: "capture", Name: "capture", Enabled: true}}, nil
}

type captureChannel struct {
	sink *captureRouter
}

func (c *captureChannel) Type() string { return "capture" }

func (c *captureChannel) Init(context.Context, *models.NotificationConf) error { return nil }

func (c *captureChannel) SupportsInbound() bool { return false }

func (c *captureChannel) Send(_ context.Context, n notify.Notification) error {
	c.sink.last = n
	c.sink.count.Add(1)
	return nil
}

func (c *captureChannel) OnInbound(notify.InboundHandler) {}

func (c *captureChannel) Close(context.Context) error { return nil }

func (c *captureChannel) Healthy() bool { return true }

func newCapturingRouter(c *captureRouter) *notify.Router {
	registry := notify.NewRegistry()
	registry.Register("capture", func() notify.Channel { return &captureChannel{sink: c} })
	return notify.NewRouter(registry, nil, c)
}

type spyMonitor struct {
	*LoginReminderMonitor
	calls atomic.Int32
}

func TestComputeTier(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		banDays  int
		remind   int
		ageDays  int
		expected string
	}{
		{"none beyond pre-warn", 30, 10, 5, tierNone},
		{"30d band at edge of pre-warn", 30, 30, 5, tier30d},
		{"14d band edge", 30, 30, 17, tier14d},
		{"7d band", 30, 10, 25, tier7d},
		{"3d band", 30, 10, 28, tier3d},
		{"imminent at 1d", 30, 10, 29, tierImminent},
		{"imminent past deadline", 30, 10, 31, tierImminent},
		{"custom threshold 60 within pre-warn", 60, 14, 50, tier14d},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastActive := now.Add(-time.Duration(tc.ageDays) * 24 * time.Hour)
			state := &models.SiteLoginState{
				BanThresholdDays: tc.banDays,
				RemindBeforeDays: tc.remind,
				LastAccessAt:     &lastActive,
			}
			effective := EffectiveLastActive(state, now)
			tier := ComputeTier(state, effective, now)
			assert.Equal(t, tc.expected, tier, "remaining=%d", DaysRemaining(state, effective, now))
		})
	}
}

func TestEffectiveLastActiveProbeWins(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-3 * 24 * time.Hour)
	visit := now.Add(-1 * 24 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		ConsecutiveProbeFailures: 0,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "fresh probe should win even though visit is newer")
}

func TestEffectiveLastActiveFallbackToVisit(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-30 * 24 * time.Hour)
	visit := now.Add(-2 * 24 * time.Hour)
	staleProbeAt := now.Add(-20 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		LastProbeAt:              &staleProbeAt,
		ConsecutiveProbeFailures: 5,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "last_access should win over extension visit even after stale probe failures")
}

func TestEffectiveLastActiveFallbackOnlyAfterStaleness(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	probe := now.Add(-30 * 24 * time.Hour)
	visit := now.Add(-2 * 24 * time.Hour)
	freshProbeAt := now.Add(-1 * time.Hour)

	state := &models.SiteLoginState{
		LastAccessAt:             &probe,
		LastVisitAt:              &visit,
		LastProbeAt:              &freshProbeAt,
		ConsecutiveProbeFailures: 1,
	}
	got := EffectiveLastActive(state, now)
	assert.Equal(t, probe, got, "1h failure shouldn't trigger fallback (need 12h+)")
}

func TestRunReminderOnceCronWindowDedup(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	state := &models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tierNone,
	}
	expired := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	state.LastAccessAt = &expired
	require.NoError(t, db.Create(state).Error)

	monitor.RunReminderOnce(context.Background())

	var first models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&first).Error)
	require.NotNil(t, first.LastReminderSentAt, "first run within cron window should fire")
	firstSentAt := *first.LastReminderSentAt

	advanceClock(monitor, 30*time.Minute)
	monitor.RunReminderOnce(context.Background())

	var second models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&second).Error)
	require.NotNil(t, second.LastReminderSentAt)
	assert.Equal(t, firstSentAt, *second.LastReminderSentAt, "same cron window should NOT re-fire (dedup)")
}

func TestRunReminderOnceTierEscalationFiresImmediately(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))

	stateAccess := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "0 10,22 * * *",
		LastReminderTier: tier7d,
		LastAccessAt:     &stateAccess,
	}).Error)

	advanceClock(monitor, 30*time.Minute)

	advanceClock(monitor, 4*24*time.Hour)
	monitor.RunReminderOnce(context.Background())

	var after models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&after).Error)
	require.NotNil(t, after.LastReminderSentAt, "tier escalation 7d→3d should fire even off cron tick")
	assert.Equal(t, tier3d, after.LastReminderTier)
}

func TestRunReminderOnceQuietHoursOverride(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	imminent := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	monitor := newReminderMonitorForTest(db, imminent)
	monitor.quietHours = QuietHours{Start: "23:00", End: "08:00"}

	stateAccess := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var imminentRow models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&imminentRow).Error)
	require.NotNil(t, imminentRow.LastReminderSentAt, "banned-imminent must override quiet hours")
	assert.Equal(t, tierImminent, imminentRow.LastReminderTier)
}

func TestRunReminderOnceQuietHoursNonImminent(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	atQuiet := time.Date(2026, 5, 18, 3, 0, 0, 0, time.UTC)
	monitor := newReminderMonitorForTest(db, atQuiet)
	monitor.quietHours = QuietHours{Start: "23:00", End: "08:00"}

	stateAccess := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	assert.Nil(t, row.LastReminderSentAt, "non-imminent tier in quiet hours must NOT fire")
}

func TestReminderSilenceWindowAfterMigration(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	migrationCompletedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.MigrationState{SchemaVersion: 10, CompletedAt: migrationCompletedAt}).Error)

	monitor := newReminderMonitorForTest(db, migrationCompletedAt)
	advanceClock(monitor, 12*time.Hour)

	stateAccess := migrationCompletedAt.Add(-25 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	assert.Nil(t, row.LastReminderSentAt, "migration silence window should block non-imminent reminders within 24h")
	assert.Equal(t, tierNone, row.LastReminderTier)
}

func TestReminderResumesAfter24h(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	migrationCompletedAt := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.MigrationState{SchemaVersion: 10, CompletedAt: migrationCompletedAt}).Error)

	monitor := newReminderMonitorForTest(db, migrationCompletedAt)
	advanceClock(monitor, 25*time.Hour)

	stateAccess := migrationCompletedAt.Add(-25 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 10,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &stateAccess,
	}).Error)

	monitor.RunReminderOnce(context.Background())

	var row models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&row).Error)
	require.NotNil(t, row.LastReminderSentAt, "reminders should resume after the 24h migration silence window")
	assert.Equal(t, migrationCompletedAt.Add(25*time.Hour), *row.LastReminderSentAt)
	assert.Equal(t, tier3d, row.LastReminderTier)
}

func TestSendTestReminderRoutesWithoutMutatingReminderState(t *testing.T) {
	db := newReminderTestDB(t)
	recorder := &captureRouter{}
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * 24 * time.Hour)
	sentAt := now.Add(-2 * time.Hour)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:               "HDSKY",
		BanThresholdDays:       30,
		RemindBeforeDays:       10,
		ReminderCron:           "0 10,22 * * *",
		NotificationChannelIDs: "[1]",
		LastReminderTier:       tier7d,
		LastReminderSentAt:     &sentAt,
		LastAccessAt:           &recent,
	}).Error)

	monitor := newReminderMonitorForTest(db, now)
	monitor.router = newCapturingRouter(recorder)

	require.NoError(t, monitor.SendTestReminder(context.Background(), "HDSKY"))
	require.NoError(t, monitor.SendTestReminder(context.Background(), "HDSKY"))

	assert.Equal(t, int32(2), recorder.count.Load(), "test reminders should bypass router dedupe")
	assert.Equal(t, "[pt-tools][测试] 站点 HDSKY 登录提醒", recorder.last.Title)
	assert.Contains(t, recorder.last.Text, "这是一条测试提醒")
	assert.Contains(t, recorder.last.Text, "tier=none")

	var after models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&after).Error)
	assert.Equal(t, tier7d, after.LastReminderTier)
	require.NotNil(t, after.LastReminderSentAt)
	assert.Equal(t, sentAt, *after.LastReminderSentAt)
}

func TestRunProbeOnceUpdatesState(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true, Cookie: "abc=1"}).Error)

	access := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC).Unix()
	resolver := &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{info: v2.UserInfo{LastAccess: access}},
	}

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.resolver = resolver

	monitor.RunProbeOnce(context.Background())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	require.NotNil(t, state.LastAccessAt)
	assert.Equal(t, access, state.LastAccessAt.Unix())
	assert.Equal(t, "OK", state.LastProbeStatus)
	assert.Equal(t, 0, state.ConsecutiveProbeFailures)
}

func TestRunProbeOnceCountsFailures(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)

	resolver := &fakeReminderResolver{
		def:  &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP},
		site: &fakeReminderSite{err: fmt.Errorf("boom: %w", v2.ErrSessionExpired)},
	}

	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.resolver = resolver

	monitor.RunProbeOnce(context.Background())
	monitor.RunProbeOnce(context.Background())
	monitor.RunProbeOnce(context.Background())

	var state models.SiteLoginState
	require.NoError(t, db.Where("site_name = ?", "HDSKY").First(&state).Error)
	assert.Equal(t, 3, state.ConsecutiveProbeFailures)
	assert.Equal(t, "SESSION_EXPIRED", state.LastProbeStatus)
}

func TestStartStopIdempotent(t *testing.T) {
	db := newReminderTestDB(t)
	monitor := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	monitor.probeEvery = 24 * time.Hour
	monitor.reminderTick = 24 * time.Hour

	monitor.Start()
	monitor.Start()
	time.Sleep(20 * time.Millisecond)
	monitor.Stop()
	monitor.Stop()
}

func TestDecodeChannelIDs(t *testing.T) {
	cases := []struct {
		raw  string
		want []uint
	}{
		{"", nil},
		{"[]", []uint{}},
		{"[1,3,5]", []uint{1, 3, 5}},
		{"not json", nil},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeChannelIDs(tc.raw))
		})
	}
}

func TestTierIsHigher(t *testing.T) {
	assert.True(t, tierIsHigher(tier3d, tier7d))
	assert.True(t, tierIsHigher(tierImminent, tier3d))
	assert.False(t, tierIsHigher(tier7d, tier3d))
	assert.False(t, tierIsHigher(tierNone, tierNone))
}

func newReminderMonitorForTest(db *gorm.DB, now time.Time) *LoginReminderMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &LoginReminderMonitor{
		ctx:          ctx,
		cancel:       cancel,
		db:           db,
		router:       nil,
		resolver:     &fakeReminderResolver{def: &v2.SiteDefinition{ID: "fake"}, site: &fakeReminderSite{}},
		decryptor:    fakeDecryptor{},
		clock:        sitelogin.NewFakeClock(now),
		logger:       zap.NewNop().Sugar(),
		probeEvery:   6 * time.Hour,
		reminderTick: time.Minute,
		probeLocks:   make(map[string]*probeSlot),
	}
}

func advanceClock(m *LoginReminderMonitor, d time.Duration) {
	if fc, ok := m.clock.(*sitelogin.FakeClock); ok {
		fc.Advance(d)
	}
}

var _ = errors.New
