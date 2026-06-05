package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestValidateAndNormalizeRSS(t *testing.T) {
	existing := []models.RSSConfig{{Name: "old", URL: " HTTP://RSS.EXAMPLE/FEED "}}

	tests := []struct {
		name     string
		existing []models.RSSConfig
		entry    models.RSSConfig
		want     models.RSSConfig
		wantErr  string
	}{
		{
			name:     "name empty",
			existing: nil,
			entry:    models.RSSConfig{Name: " ", URL: "http://rss.example/feed", IntervalMinutes: 10},
			wantErr:  "RSS 的 name 不能为空",
		},
		{
			name:     "url empty",
			existing: nil,
			entry:    models.RSSConfig{Name: "rss", URL: " ", IntervalMinutes: 10},
			wantErr:  "RSS 的 url 不能为空",
		},
		{
			name:     "duplicate url trims and ignores case",
			existing: existing,
			entry:    models.RSSConfig{Name: "new", URL: "http://rss.example/feed", IntervalMinutes: 10},
			wantErr:  "RSS 的 URL 与已有订阅重复: http://rss.example/feed",
		},
		{
			name:     "interval clamped to min",
			existing: nil,
			entry: models.RSSConfig{
				Name:            "rss",
				URL:             "http://rss.example/feed",
				IntervalMinutes: 3,
			},
			want: models.RSSConfig{
				Name:            "rss",
				URL:             "http://rss.example/feed",
				IntervalMinutes: models.MinIntervalMinutes,
			},
		},
		{
			name:     "valid unique entry",
			existing: existing,
			entry: models.RSSConfig{
				Name:            "rss",
				URL:             "http://rss.example/ok",
				Category:        "movie",
				IntervalMinutes: 10,
			},
			want: models.RSSConfig{
				Name:            "rss",
				URL:             "http://rss.example/ok",
				Category:        "movie",
				IntervalMinutes: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndNormalizeRSS(tt.existing, tt.entry)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Equal(t, models.RSSConfig{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
