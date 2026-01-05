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
	v2 "github.com/sunerpy/pt-tools/site/v2"
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

func TestUnifiedSiteImpl_MapDiscountLevels(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
	if err != nil {
		t.Fatalf("NewUnifiedSiteImpl() error = %v", err)
	}

	// Test MT discount mapping
	mtTests := []struct {
		input string
		want  v2.DiscountLevel
	}{
		{"FREE", v2.DiscountFree},
		{"_2X_FREE", v2.Discount2xFree},
		{"2XFREE", v2.Discount2xFree},
		{"PERCENT_50", v2.DiscountPercent50},
		{"50%", v2.DiscountPercent50},
		{"PERCENT_30", v2.DiscountPercent30},
		{"30%", v2.DiscountPercent30},
		{"PERCENT_70", v2.DiscountPercent70},
		{"70%", v2.DiscountPercent70},
		{"_2X_UP", v2.Discount2xUp},
		{"2XUP", v2.Discount2xUp},
		{"_2X_PERCENT_50", v2.Discount2x50},
		{"2X50%", v2.Discount2x50},
		{"NORMAL", v2.DiscountNone},
		{"", v2.DiscountNone},
	}

	for _, tt := range mtTests {
		t.Run("MT_"+tt.input, func(t *testing.T) {
			got := impl.mapMTDiscountLevel(tt.input)
			if got != tt.want {
				t.Errorf("mapMTDiscountLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	// Test PHP discount mapping
	phpTests := []struct {
		input models.DiscountType
		want  v2.DiscountLevel
	}{
		{models.DISCOUNT_FREE, v2.DiscountFree},
		{models.DISCOUNT_TWO_X_FREE, v2.Discount2xFree},
		{models.DISCOUNT_TWO_X, v2.Discount2xUp},
		{models.DISCOUNT_FIFTY, v2.DiscountPercent50},
		{models.DISCOUNT_THIRTY, v2.DiscountPercent30},
		{models.DISCOUNT_TWO_X_FIFTY, v2.Discount2x50},
		{models.DISCOUNT_NONE, v2.DiscountNone},
		{models.DISCOUNT_CUSTOM, v2.DiscountNone},
	}

	for _, tt := range phpTests {
		t.Run("PHP_"+string(tt.input), func(t *testing.T) {
			got := impl.mapPHPDiscountLevel(tt.input)
			if got != tt.want {
				t.Errorf("mapPHPDiscountLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestUnifiedSiteImpl_ConvertMTTorrentToItem(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.MTEAM)
	if err != nil {
		t.Fatalf("NewUnifiedSiteImpl() error = %v", err)
	}

	detail := &models.MTTorrentDetail{
		ID:         "12345",
		Name:       "Test Torrent",
		SmallDescr: "Test Description",
		Size:       "1073741824", // 1GB in bytes
		Status: &models.Status{
			Discount:        "FREE",
			DiscountEndTime: "2026-01-10 12:00:00",
		},
	}

	item := impl.convertMTTorrentToItem(detail)

	if item.ID != "12345" {
		t.Errorf("ID = %v, want %v", item.ID, "12345")
	}
	if item.Title != "Test Torrent" {
		t.Errorf("Title = %v, want %v", item.Title, "Test Torrent")
	}
	if item.DiscountLevel != v2.DiscountFree {
		t.Errorf("DiscountLevel = %v, want %v", item.DiscountLevel, v2.DiscountFree)
	}
	if len(item.Tags) != 1 || item.Tags[0] != "Test Description" {
		t.Errorf("Tags = %v, want [Test Description]", item.Tags)
	}
	if item.DiscountEndTime.IsZero() {
		t.Error("DiscountEndTime should not be zero")
	}
}

func TestUnifiedSiteImpl_ConvertPHPTorrentToItem(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	impl, err := NewUnifiedSiteImpl(context.Background(), models.HDSKY)
	if err != nil {
		t.Fatalf("NewUnifiedSiteImpl() error = %v", err)
	}

	endTime := time.Now().Add(24 * time.Hour)
	info := &models.PHPTorrentInfo{
		TorrentID: "67890",
		Title:     "PHP Test Torrent",
		SubTitle:  "PHP Test Subtitle",
		SizeMB:    1024, // 1GB
		Seeders:   10,
		Leechers:  5,
		Discount:  models.DISCOUNT_TWO_X_FREE,
		EndTime:   endTime,
		HR:        true,
	}

	item := impl.convertPHPTorrentToItem(info)

	if item.ID != "67890" {
		t.Errorf("ID = %v, want %v", item.ID, "67890")
	}
	if item.Title != "PHP Test Torrent" {
		t.Errorf("Title = %v, want %v", item.Title, "PHP Test Torrent")
	}
	if item.SizeBytes != 1024*1024*1024 {
		t.Errorf("SizeBytes = %v, want %v", item.SizeBytes, 1024*1024*1024)
	}
	if item.Seeders != 10 {
		t.Errorf("Seeders = %v, want %v", item.Seeders, 10)
	}
	if item.Leechers != 5 {
		t.Errorf("Leechers = %v, want %v", item.Leechers, 5)
	}
	if item.DiscountLevel != v2.Discount2xFree {
		t.Errorf("DiscountLevel = %v, want %v", item.DiscountLevel, v2.Discount2xFree)
	}
	if !item.HasHR {
		t.Error("HasHR should be true")
	}
	if len(item.Tags) != 1 || item.Tags[0] != "PHP Test Subtitle" {
		t.Errorf("Tags = %v, want [PHP Test Subtitle]", item.Tags)
	}
}
