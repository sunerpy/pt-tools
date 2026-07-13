package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
)

const rssFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>test</title>
<item><title>Cool Movie 1080p</title><guid>1</guid><link>http://e/1</link><category>movie</category></item>
<item><title>Another Show</title><guid>2</guid><link>http://e/2</link><category>tv</category></item>
</channel></rss>`

func TestTestFilterRuleWithRSS_Cov(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "unknownsite", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "r1", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	m, err := filter.NewMatcher(filter.PatternKeyword, "Cool")
	require.NoError(t, err)

	t.Run("rss not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, 999, 20)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("matches feed items", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, rss.ID, 20)
		require.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 2, resp.TotalCount)
	})
}

func TestFetchRSSFeedForTest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	feed, err := fetchRSSFeedForTest(ts.URL)
	require.NoError(t, err)
	assert.Len(t, feed.Items, 2)

	_, err = fetchRSSFeedForTest("http://127.0.0.1:1/nope")
	assert.Error(t, err)
}
