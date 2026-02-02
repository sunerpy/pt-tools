package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
)

func setupTestDB(t *testing.T) func() {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	return func() {
		// cleanup if needed
	}
}

func TestNewUnifiedSiteImpl(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name      string
		siteGroup models.SiteGroup
		wantErr   bool
	}{
		{
			name:      "MTEAM site",
			siteGroup: models.MTEAM,
			wantErr:   false,
		},
		{
			name:      "HDSKY site",
			siteGroup: models.HDSKY,
			wantErr:   false,
		},
		{
			name:      "CMCT site",
			siteGroup: models.SpringSunday,
			wantErr:   false,
		},
		{
			name:      "Unknown site",
			siteGroup: models.SiteGroup("unknown"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), tt.siteGroup)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewUnifiedSiteImpl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && impl == nil {
				t.Error("NewUnifiedSiteImpl() returned nil without error")
			}
			if !tt.wantErr {
				if impl.SiteGroup() != tt.siteGroup {
					t.Errorf("SiteGroup() = %v, want %v", impl.SiteGroup(), tt.siteGroup)
				}
				if impl.Context() == nil {
					t.Error("Context() returned nil")
				}
				if impl.MaxRetries() != maxRetries {
					t.Errorf("MaxRetries() = %v, want %v", impl.MaxRetries(), maxRetries)
				}
				if impl.RetryDelay() != retryDelay {
					t.Errorf("RetryDelay() = %v, want %v", impl.RetryDelay(), retryDelay)
				}
			}
		})
	}
}

func TestUnifiedSiteImpl_IsEnabled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// 创建站点配置
	enabled := true
	disabled := false

	// 插入启用的站点
	global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:       string(models.MTEAM),
		Enabled:    true,
		AuthMethod: "api_key",
		APIKey:     "test-key",
		APIUrl:     "https://api.m-team.cc",
	})

	// 插入禁用的站点
	global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:       string(models.HDSKY),
		Enabled:    false,
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
	})

	tests := []struct {
		name      string
		siteGroup models.SiteGroup
		want      bool
	}{
		{
			name:      "Enabled site",
			siteGroup: models.MTEAM,
			want:      true,
		},
		{
			name:      "Disabled site",
			siteGroup: models.HDSKY,
			want:      false,
		},
		{
			name:      "Non-existent site",
			siteGroup: models.SpringSunday,
			want:      false,
		},
	}

	_ = enabled
	_ = disabled

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), tt.siteGroup)
			if err != nil {
				t.Fatalf("NewUnifiedSiteImpl() error = %v", err)
			}
			if got := impl.IsEnabled(); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnifiedSiteImpl_RateLimiter(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
	require.NoError(t, err)
	require.NotNil(t, impl)
	require.NotNil(t, impl.limiter, "limiter should be initialized")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	for i := 0; i < 3; i++ {
		err := impl.waitForRateLimit(ctx)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Logf("Rate limit applied, elapsed: %v (burst allowed initial requests)", elapsed)
	}
}

func TestUnifiedSiteImpl_RateLimiter_ContextCanceled(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
	require.NoError(t, err)

	for i := 0; i < 200; i++ {
		_ = impl.limiter.Allow()
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = impl.waitForRateLimit(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}
