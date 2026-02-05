package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSettingsGlobal_Defaults(t *testing.T) {
	var g SettingsGlobal
	require.Equal(t, 0, g.MaxRetry)
	require.Equal(t, 0, g.RetainHours)
}

func TestSettingsGlobal_GetEffectiveIntervalMinutes(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected int32
	}{
		{"零值返回默认值", 0, DefaultIntervalMinutes},
		{"负值返回默认值", -5, DefaultIntervalMinutes},
		{"小于最小值返回最小值", 2, MinIntervalMinutes},
		{"正常值返回原值", 15, 15},
		{"大于最大值返回最大值", 2000, MaxIntervalMinutes},
		{"边界值-最小值", MinIntervalMinutes, MinIntervalMinutes},
		{"边界值-最大值", MaxIntervalMinutes, MaxIntervalMinutes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := SettingsGlobal{DefaultIntervalMinutes: tt.input}
			result := g.GetEffectiveIntervalMinutes()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSettingsGlobal_GetEffectiveConcurrency(t *testing.T) {
	tests := []struct {
		name     string
		input    int32
		expected int32
	}{
		{"零值返回默认值", 0, DefaultConcurrency},
		{"负值返回默认值", -1, DefaultConcurrency},
		{"正常值返回原值", 5, 5},
		{"大于最大值返回最大值", 20, MaxConcurrency},
		{"边界值-最小值", MinConcurrency, MinConcurrency},
		{"边界值-最大值", MaxConcurrency, MaxConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := SettingsGlobal{DefaultConcurrency: tt.input}
			result := g.GetEffectiveConcurrency()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSConfig_GetEffectiveIntervalMinutes(t *testing.T) {
	globalSettings := &SettingsGlobal{DefaultIntervalMinutes: 20}

	tests := []struct {
		name           string
		rssInterval    int32
		globalSettings *SettingsGlobal
		expected       int32
	}{
		{"RSS配置优先", 15, globalSettings, 15},
		{"RSS为0使用全局配置", 0, globalSettings, 20},
		{"RSS为负使用全局配置", -5, globalSettings, 20},
		{"全局配置为nil使用默认值", 0, nil, DefaultIntervalMinutes},
		{"RSS小于最小值返回最小值", 2, globalSettings, MinIntervalMinutes},
		{"RSS大于最大值返回最大值", 2000, globalSettings, MaxIntervalMinutes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RSSConfig{IntervalMinutes: tt.rssInterval}
			result := r.GetEffectiveIntervalMinutes(tt.globalSettings)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRSSConfig_GetEffectiveConcurrency(t *testing.T) {
	globalSettings := &SettingsGlobal{DefaultConcurrency: 5}

	tests := []struct {
		name           string
		rssConcurrency int32
		globalSettings *SettingsGlobal
		expected       int32
	}{
		{"RSS配置优先", 8, globalSettings, 8},
		{"RSS为0使用全局配置", 0, globalSettings, 5},
		{"RSS为负使用全局配置", -1, globalSettings, 5},
		{"全局配置为nil使用默认值", 0, nil, DefaultConcurrency},
		{"RSS大于最大值返回最大值", 20, globalSettings, MaxConcurrency},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RSSConfig{Concurrency: tt.rssConcurrency}
			result := r.GetEffectiveConcurrency(tt.globalSettings)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestRSSConfig_ShouldSkip 测试 ShouldSkip 方法
func TestRSSConfig_ShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		rss      RSSConfig
		wantSkip bool
	}{
		{
			name:     "示例配置应该跳过",
			rss:      RSSConfig{URL: "https://example.com/rss", IsExample: true},
			wantSkip: true,
		},
		{
			name:     "空URL应该跳过",
			rss:      RSSConfig{URL: "", IsExample: false},
			wantSkip: true,
		},
		{
			name:     "示例配置且空URL应该跳过",
			rss:      RSSConfig{URL: "", IsExample: true},
			wantSkip: true,
		},
		{
			name:     "正常配置不应该跳过",
			rss:      RSSConfig{URL: "https://example.com/rss", IsExample: false},
			wantSkip: false,
		},
		{
			name:     "有URL的非示例配置不应该跳过",
			rss:      RSSConfig{URL: "https://test.com/feed", IsExample: false, Name: "test"},
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rss.ShouldSkip()
			require.Equal(t, tt.wantSkip, result)
		})
	}
}

// TestRSSConfig_Fields 测试 RSSConfig 结构体字段
func TestRSSConfig_Fields(t *testing.T) {
	downloaderID := uint(1)
	rss := RSSConfig{
		Name:            "test-site",
		URL:             "https://example.com/rss",
		IntervalMinutes: 15,
		Concurrency:     5,
		DownloadSubPath: "/downloads/test",
		DownloaderID:    &downloaderID,
		IsExample:       false,
	}

	require.Equal(t, "test-site", rss.Name)
	require.Equal(t, "https://example.com/rss", rss.URL)
	require.Equal(t, int32(15), rss.IntervalMinutes)
	require.Equal(t, int32(5), rss.Concurrency)
	require.Equal(t, "/downloads/test", rss.DownloadSubPath)
	require.NotNil(t, rss.DownloaderID)
	require.Equal(t, uint(1), *rss.DownloaderID)
	require.False(t, rss.IsExample)
}

// TestSettingsGlobal_Fields 测试 SettingsGlobal 结构体字段
func TestSettingsGlobal_Fields(t *testing.T) {
	g := SettingsGlobal{
		DefaultIntervalMinutes: 10,
		DefaultConcurrency:     3,
		MaxRetry:               5,
		RetainHours:            48,
	}

	require.Equal(t, int32(10), g.DefaultIntervalMinutes)
	require.Equal(t, int32(3), g.DefaultConcurrency)
	require.Equal(t, 5, g.MaxRetry)
	require.Equal(t, 48, g.RetainHours)
}

// TestRSSConfig_GetEffectiveConcurrency_EdgeCases 测试并发数边界情况
func TestRSSConfig_GetEffectiveConcurrency_EdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		rssConcurrency int32
		globalSettings *SettingsGlobal
		expected       int32
	}{
		{
			name:           "RSS并发数为最小值",
			rssConcurrency: MinConcurrency,
			globalSettings: &SettingsGlobal{DefaultConcurrency: 5},
			expected:       MinConcurrency,
		},
		{
			name:           "RSS并发数为最大值",
			rssConcurrency: MaxConcurrency,
			globalSettings: &SettingsGlobal{DefaultConcurrency: 5},
			expected:       MaxConcurrency,
		},
		{
			name:           "RSS并发数小于最小值",
			rssConcurrency: MinConcurrency - 1,
			globalSettings: &SettingsGlobal{DefaultConcurrency: 5},
			expected:       5, // 使用全局配置，因为 0 或负数会回退到全局
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RSSConfig{Concurrency: tt.rssConcurrency}
			result := r.GetEffectiveConcurrency(tt.globalSettings)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestPHPTorrentInfo_IsFree 测试 IsFree 方法
func TestPHPTorrentInfo_IsFree(t *testing.T) {
	tests := []struct {
		name     string
		discount DiscountType
		expected bool
	}{
		{"免费种子", DISCOUNT_FREE, true},
		{"2x免费种子", DISCOUNT_TWO_X_FREE, true},
		{"无优惠", DISCOUNT_NONE, false},
		{"2x上传", DISCOUNT_TWO_X, false},
		{"30%优惠", DISCOUNT_THIRTY, false},
		{"50%优惠", DISCOUNT_FIFTY, false},
		{"2x50%优惠", DISCOUNT_TWO_X_FIFTY, false},
		{"自定义优惠", DISCOUNT_CUSTOM, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PHPTorrentInfo{Discount: tt.discount}
			result := p.IsFree()
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestPHPTorrentInfo_CanbeFinished 测试 CanbeFinished 方法
func TestPHPTorrentInfo_CanbeFinished(t *testing.T) {
	logger := zap.NewNop().Sugar()

	tests := []struct {
		name        string
		torrent     PHPTorrentInfo
		enabled     bool
		speedLimit  int
		sizeLimitGB int
		expected    bool
	}{
		{
			name:        "限制未启用",
			torrent:     PHPTorrentInfo{SizeMB: 1024, EndTime: time.Now().Add(1 * time.Hour)},
			enabled:     false,
			speedLimit:  10,
			sizeLimitGB: 1,
			expected:    true,
		},
		{
			name:        "种子大小超过限制",
			torrent:     PHPTorrentInfo{SizeMB: 2048, EndTime: time.Now().Add(1 * time.Hour)},
			enabled:     true,
			speedLimit:  10,
			sizeLimitGB: 1,
			expected:    false,
		},
		{
			name:        "种子大小在限制内",
			torrent:     PHPTorrentInfo{SizeMB: 512, EndTime: time.Now().Add(1 * time.Hour)},
			enabled:     true,
			speedLimit:  10,
			sizeLimitGB: 1,
			expected:    true,
		},
		{
			name:        "免费时间不足",
			torrent:     PHPTorrentInfo{SizeMB: 1024 * 1024 * 10, EndTime: time.Now().Add(1 * time.Second)},
			enabled:     true,
			speedLimit:  1,
			sizeLimitGB: 100,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.torrent.CanbeFinished(logger, tt.enabled, tt.speedLimit, tt.sizeLimitGB)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestPHPTorrentInfo_GetFreeEndTime 测试 GetFreeEndTime 方法
func TestPHPTorrentInfo_GetFreeEndTime(t *testing.T) {
	endTime := time.Now().Add(1 * time.Hour)
	p := PHPTorrentInfo{EndTime: endTime}
	result := p.GetFreeEndTime()
	require.NotNil(t, result)
	require.Equal(t, endTime.Unix(), result.Unix())
}

// TestPHPTorrentInfo_GetFreeLevel 测试 GetFreeLevel 方法
func TestPHPTorrentInfo_GetFreeLevel(t *testing.T) {
	tests := []struct {
		name     string
		discount DiscountType
		expected string
	}{
		{"免费", DISCOUNT_FREE, "free"},
		{"2x免费", DISCOUNT_TWO_X_FREE, "2xfree"},
		{"无优惠", DISCOUNT_NONE, "none"},
		{"空优惠", "", "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PHPTorrentInfo{Discount: tt.discount}
			result := p.GetFreeLevel()
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestRSSConfig_DownloadPath 测试 DownloadPath 字段和相关方法
func TestRSSConfig_DownloadPath(t *testing.T) {
	tests := []struct {
		name                  string
		downloadPath          string
		expectedEffectivePath string
		expectedHasCustomPath bool
	}{
		{
			name:                  "空下载路径",
			downloadPath:          "",
			expectedEffectivePath: "",
			expectedHasCustomPath: false,
		},
		{
			name:                  "自定义下载路径",
			downloadPath:          "/data/downloads/movies",
			expectedEffectivePath: "/data/downloads/movies",
			expectedHasCustomPath: true,
		},
		{
			name:                  "相对路径",
			downloadPath:          "downloads/tv",
			expectedEffectivePath: "downloads/tv",
			expectedHasCustomPath: true,
		},
		{
			name:                  "Windows风格路径",
			downloadPath:          "D:\\Downloads\\Torrents",
			expectedEffectivePath: "D:\\Downloads\\Torrents",
			expectedHasCustomPath: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rss := RSSConfig{
				Name:         "test-rss",
				URL:          "https://example.com/rss",
				DownloadPath: tt.downloadPath,
			}

			require.Equal(t, tt.expectedEffectivePath, rss.GetEffectiveDownloadPath())
			require.Equal(t, tt.expectedHasCustomPath, rss.HasCustomDownloadPath())
		})
	}
}

// TestRSSConfig_FilterRuleIDs 测试 FilterRuleIDs 字段
func TestRSSConfig_FilterRuleIDs(t *testing.T) {
	tests := []struct {
		name          string
		filterRuleIDs []uint
		expectedLen   int
	}{
		{
			name:          "空过滤规则列表",
			filterRuleIDs: nil,
			expectedLen:   0,
		},
		{
			name:          "单个过滤规则",
			filterRuleIDs: []uint{1},
			expectedLen:   1,
		},
		{
			name:          "多个过滤规则",
			filterRuleIDs: []uint{1, 2, 3, 5, 8},
			expectedLen:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rss := RSSConfig{
				Name:          "test-rss",
				URL:           "https://example.com/rss",
				FilterRuleIDs: tt.filterRuleIDs,
			}

			if tt.filterRuleIDs == nil {
				require.Nil(t, rss.FilterRuleIDs)
			} else {
				require.Len(t, rss.FilterRuleIDs, tt.expectedLen)
				require.Equal(t, tt.filterRuleIDs, rss.FilterRuleIDs)
			}
		})
	}
}

// TestRSSConfig_FullFields 测试 RSSConfig 所有字段
func TestRSSConfig_FullFields(t *testing.T) {
	downloaderID := uint(2)
	rss := RSSConfig{
		ID:              1,
		Name:            "full-test",
		URL:             "https://example.com/rss/full",
		Category:        "movies",
		Tag:             "hd",
		IntervalMinutes: 15,
		Concurrency:     5,
		DownloadSubPath: "subpath",
		DownloadPath:    "/custom/download/path",
		DownloaderID:    &downloaderID,
		FilterRuleIDs:   []uint{1, 2, 3},
		IsExample:       false,
	}

	require.Equal(t, uint(1), rss.ID)
	require.Equal(t, "full-test", rss.Name)
	require.Equal(t, "https://example.com/rss/full", rss.URL)
	require.Equal(t, "movies", rss.Category)
	require.Equal(t, "hd", rss.Tag)
	require.Equal(t, int32(15), rss.IntervalMinutes)
	require.Equal(t, int32(5), rss.Concurrency)
	require.Equal(t, "subpath", rss.DownloadSubPath)
	require.Equal(t, "/custom/download/path", rss.DownloadPath)
	require.NotNil(t, rss.DownloaderID)
	require.Equal(t, uint(2), *rss.DownloaderID)
	require.Len(t, rss.FilterRuleIDs, 3)
	require.False(t, rss.IsExample)
	require.True(t, rss.HasCustomDownloadPath())
	require.Equal(t, "/custom/download/path", rss.GetEffectiveDownloadPath())
}
