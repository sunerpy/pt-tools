package scheduler

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

func TestEffectiveLastActiveV2Semantic(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	lastAccessAt := now.Add(-7 * 24 * time.Hour)
	apiLastLoginAt := now.Add(-2 * 24 * time.Hour)
	cookieLastLoginAt := now.Add(-1 * 24 * time.Hour)
	legacyLastLoginAt := now.Add(-3 * 24 * time.Hour)
	lastVisitAt := now.Add(-4 * 24 * time.Hour)
	staleProbeAt := now.Add(-13 * time.Hour)
	freshProbeAt := now.Add(-2 * time.Hour)

	cases := []struct {
		name  string
		state *models.SiteLoginState
		want  time.Time
	}{
		{
			name:  "last_access_only_returns_last_access",
			state: &models.SiteLoginState{LastAccessAt: &lastAccessAt},
			want:  lastAccessAt,
		},
		{
			name:  "last_access_wins_over_api_last_login",
			state: &models.SiteLoginState{LastAccessAt: &lastAccessAt, ApiLastLoginAt: &apiLastLoginAt},
			want:  lastAccessAt,
		},
		{
			name:  "api_last_login_only_returns_api_last_login",
			state: &models.SiteLoginState{ApiLastLoginAt: &apiLastLoginAt},
			want:  apiLastLoginAt,
		},
		{
			name:  "cookie_last_login_only_returns_cookie_last_login",
			state: &models.SiteLoginState{CookieLastLoginAt: &cookieLastLoginAt},
			want:  cookieLastLoginAt,
		},
		{
			name:  "cookie_newer_than_api_returns_cookie",
			state: &models.SiteLoginState{ApiLastLoginAt: &apiLastLoginAt, CookieLastLoginAt: &cookieLastLoginAt},
			want:  cookieLastLoginAt,
		},
		{
			name:  "legacy_last_login_only_returns_legacy_last_login",
			state: &models.SiteLoginState{LastLoginAt: &legacyLastLoginAt},
			want:  legacyLastLoginAt,
		},
		{
			name: "last_visit_returns_only_after_probe_failures_stale",
			state: &models.SiteLoginState{
				LastVisitAt:              &lastVisitAt,
				LastProbeAt:              &staleProbeAt,
				ConsecutiveProbeFailures: 1,
			},
			want: lastVisitAt,
		},
		{
			name:  "last_visit_healthy_probe_returns_zero",
			state: &models.SiteLoginState{LastVisitAt: &lastVisitAt, ConsecutiveProbeFailures: 0},
			want:  time.Time{},
		},
		{
			name:  "all_empty_returns_zero",
			state: &models.SiteLoginState{},
			want:  time.Time{},
		},
		{
			name: "last_access_wins_over_last_visit_when_probe_healthy",
			state: &models.SiteLoginState{
				LastAccessAt:             &lastAccessAt,
				LastVisitAt:              &lastVisitAt,
				LastProbeAt:              &freshProbeAt,
				ConsecutiveProbeFailures: 0,
			},
			want: lastAccessAt,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EffectiveLastActive(tc.state, now)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReminderMessageContainsLoginSuggestion(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	lastAccessAt := now.Add(-5 * 24 * time.Hour)
	channel := &capturingReminderChannel{}

	db := newReminderTestDB(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "HDSKY", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName:         "HDSKY",
		BanThresholdDays: 30,
		RemindBeforeDays: 30,
		ReminderCron:     "* * * * *",
		LastReminderTier: tierNone,
		LastAccessAt:     &lastAccessAt,
	}).Error)

	monitor := newReminderMonitorForTest(db, now)
	monitor.router = newReminderRouterForTest(channel)
	monitor.RunReminderOnce(context.Background())

	notificationText := channel.lastNotificationText(t)
	for _, phrase := range []string{"建议", "访问", "浏览", "页面", "刷新", lastAccessAt.Format(time.RFC3339)} {
		assert.Contains(t, notificationText, phrase)
	}
}

func TestEffectiveLastActiveV1Compat(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		banDays  int
		remind   int
		ageDays  int
		wantTier string
	}{
		{name: "none beyond pre-warn", banDays: 30, remind: 10, ageDays: 5, wantTier: tierNone},
		{name: "7d band", banDays: 30, remind: 10, ageDays: 25, wantTier: tier7d},
		{name: "custom threshold 60 within pre-warn", banDays: 60, remind: 14, ageDays: 50, wantTier: tier14d},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastAccessAt := now.Add(-time.Duration(tc.ageDays) * 24 * time.Hour)
			state := &models.SiteLoginState{
				BanThresholdDays: tc.banDays,
				RemindBeforeDays: tc.remind,
				LastAccessAt:     &lastAccessAt,
			}

			effective := EffectiveLastActive(state, now)
			gotTier := ComputeTier(state, effective, now)
			assert.Equal(t, tc.wantTier, gotTier)
		})
	}
}

type reminderConfLister struct{}

func (reminderConfLister) ListNotificationConfs(context.Context) ([]models.NotificationConf, error) {
	return []models.NotificationConf{{ID: 1, ChannelType: "capture-reminder", Name: "capture", Enabled: true}}, nil
}

type capturingReminderChannel struct {
	mu           sync.Mutex
	notification notify.Notification
}

func (c *capturingReminderChannel) Type() string { return "capture-reminder" }

func (c *capturingReminderChannel) Init(context.Context, *models.NotificationConf) error { return nil }

func (c *capturingReminderChannel) SupportsInbound() bool { return false }

func (c *capturingReminderChannel) Send(_ context.Context, notification notify.Notification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.notification = notification
	return nil
}

func (c *capturingReminderChannel) OnInbound(notify.InboundHandler) {}

func (c *capturingReminderChannel) Close(context.Context) error { return nil }

func (c *capturingReminderChannel) Healthy() bool { return true }

func (c *capturingReminderChannel) lastNotificationText(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.False(t, strings.TrimSpace(c.notification.Text) == "", "expected reminder notification text")
	return c.notification.Text
}

func newReminderRouterForTest(channel *capturingReminderChannel) *notify.Router {
	registry := notify.NewRegistry()
	registry.Register("capture-reminder", func() notify.Channel { return channel })
	return notify.NewRouter(registry, nil, reminderConfLister{})
}
