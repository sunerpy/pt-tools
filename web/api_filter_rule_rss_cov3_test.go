package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
)

func TestTestFilterRuleWithRSS_RequireFreeAndMatch(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssFeedXML))
	}))
	defer ts.Close()

	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSSubscription{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "unknownrss", Enabled: true, BaseURL: "http://e"}).Error)
	rss := models.RSSSubscription{Name: "r1", URL: ts.URL, SiteID: 1, IntervalMinutes: 10}
	require.NoError(t, global.GlobalDB.DB.Create(&rss).Error)

	m, err := filter.NewMatcher(filter.PatternKeyword, "Cool")
	require.NoError(t, err)

	t.Run("require free filters out non-free", func(t *testing.T) {
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, true, rss.ID, 20)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("match both fields", func(t *testing.T) {
		mb, err := filter.NewMatcher(filter.PatternWildcard, "*")
		require.NoError(t, err)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, mb, models.MatchFieldBoth, false, rss.ID, 1)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("site not found", func(t *testing.T) {
		require.NoError(t, global.GlobalDB.DB.Create(&models.RSSSubscription{
			Name: "orphan", URL: ts.URL, SiteID: 9999, IntervalMinutes: 10,
		}).Error)
		w := httptest.NewRecorder()
		server.testFilterRuleWithRSS(w, m, models.MatchFieldTitle, false, 2, 20)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestMatchesField_AllBranches(t *testing.T) {
	m, err := filter.NewMatcher(filter.PatternKeyword, "hit")
	require.NoError(t, err)

	assert.True(t, matchesField(m, models.MatchFieldTitle, "a hit b", "tag"))
	assert.False(t, matchesField(m, models.MatchFieldTitle, "nope", "hit"))
	assert.True(t, matchesField(m, models.MatchFieldTag, "nope", "hit tag"))
	assert.True(t, matchesField(m, models.MatchFieldBoth, "hit", "x"))
	assert.True(t, matchesField(m, models.MatchField("unknown"), "x", "hit"))
}
