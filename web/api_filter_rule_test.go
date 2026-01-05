package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func setupFilterRuleTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir, err := os.MkdirTemp("", "filter_rule_api_test")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate tables
	err = db.AutoMigrate(&models.FilterRule{}, &models.TorrentInfo{}, &models.SiteSetting{})
	require.NoError(t, err)

	// Set global DB
	global.GlobalDB = &models.TorrentDB{DB: db}

	// Initialize logger for tests
	zapLogger, _ := zap.NewDevelopment()
	global.GlobalLogger = zapLogger

	server := &Server{
		sessions: map[string]string{"test-session": "admin"},
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tmpDir)
	}

	return server, cleanup
}

func TestFilterRuleAPI(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	var createdRuleID uint

	t.Run("Create filter rule", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test Rule",
			Pattern:     "test*",
			PatternType: "wildcard",
			RequireFree: true,
			Enabled:     true,
			Priority:    50,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Test Rule", resp.Name)
		assert.Equal(t, "test*", resp.Pattern)
		assert.Equal(t, "wildcard", resp.PatternType)
		assert.True(t, resp.RequireFree)
		assert.True(t, resp.Enabled)
		assert.Equal(t, 50, resp.Priority)

		createdRuleID = resp.ID
	})

	t.Run("Create filter rule with duplicate name fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test Rule",
			Pattern:     "another*",
			PatternType: "wildcard",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "已存在")
	})

	t.Run("Create filter rule with invalid regex fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Invalid Regex Rule",
			Pattern:     "[invalid",
			PatternType: "regex",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("List filter rules", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp []FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp), 1)
	})

	t.Run("Get filter rule by ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/1", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.getFilterRule(w, req, createdRuleID)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Test Rule", resp.Name)
	})

	t.Run("Update filter rule", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Updated Rule",
			Pattern:     "updated*",
			PatternType: "wildcard",
			RequireFree: false,
			Enabled:     true,
			Priority:    25,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.updateFilterRule(w, req, createdRuleID)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Updated Rule", resp.Name)
		assert.Equal(t, "updated*", resp.Pattern)
		assert.False(t, resp.RequireFree)
		assert.Equal(t, 25, resp.Priority)
	})

	t.Run("Delete filter rule", func(t *testing.T) {
		// First create a rule to delete
		reqBody := FilterRuleRequest{
			Name:        "To Delete",
			Pattern:     "delete*",
			PatternType: "wildcard",
		}
		body, _ := json.Marshal(reqBody)

		createReq := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		createW := httptest.NewRecorder()
		server.apiFilterRules(createW, createReq)
		require.Equal(t, http.StatusOK, createW.Code)

		var createResp FilterRuleResponse
		json.Unmarshal(createW.Body.Bytes(), &createResp)

		// Now delete it
		deleteReq := httptest.NewRequest(http.MethodDelete, "/api/filter-rules/"+strconv.FormatUint(uint64(createResp.ID), 10), nil)
		deleteReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		deleteW := httptest.NewRecorder()
		server.deleteFilterRule(deleteW, deleteReq, createResp.ID)

		assert.Equal(t, http.StatusOK, deleteW.Code)

		// Verify it's deleted
		getReq := httptest.NewRequest(http.MethodGet, "/api/filter-rules/"+strconv.FormatUint(uint64(createResp.ID), 10), nil)
		getReq.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		getW := httptest.NewRecorder()
		server.getFilterRule(getW, getReq, createResp.ID)

		assert.Equal(t, http.StatusNotFound, getW.Code)
	})

	t.Run("Get non-existent filter rule returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/filter-rules/9999", nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.getFilterRule(w, req, 9999)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestFilterRuleTestAPI(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add some test torrents
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Game of Thrones S01E01"},
		{SiteName: "test", TorrentID: "2", Title: "Game of Thrones S01E02"},
		{SiteName: "test", TorrentID: "3", Title: "Breaking Bad S01E01"},
		{SiteName: "test", TorrentID: "4", Title: "The Office S01E01"},
	}
	for _, t := range torrents {
		db.Create(&t)
	}

	t.Run("Test keyword pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "Game of Thrones",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.MatchCount)
	})

	t.Run("Test wildcard pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "*S01E01*",
			PatternType: "wildcard",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount)
	})

	t.Run("Test regex pattern", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "S\\d{2}E\\d{2}",
			PatternType: "regex",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 4, resp.MatchCount)
	})

	t.Run("Test invalid regex returns error", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "[invalid",
			PatternType: "regex",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestFilterRuleValidation(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	t.Run("Empty name fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:    "",
			Pattern: "test",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "名称")
	})

	t.Run("Empty pattern fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:    "Test",
			Pattern: "",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "模式")
	})

	t.Run("Invalid pattern type fails", func(t *testing.T) {
		reqBody := FilterRuleRequest{
			Name:        "Test",
			Pattern:     "test",
			PatternType: "invalid",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.apiFilterRules(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "模式类型")
	})
}

func TestFilterRuleTestAPI_MatchField(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents with tags
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action,thriller"},
		{SiteName: "test", TorrentID: "2", Title: "Movie 4K HDR", Tag: "comedy,action"},
		{SiteName: "test", TorrentID: "3", Title: "TV Show S01E01", Tag: "drama"},
		{SiteName: "test", TorrentID: "4", Title: "Documentary", Tag: "action,documentary"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("Match title only", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "1080p",
			PatternType: "keyword",
			MatchField:  "title",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount)
		assert.Equal(t, 4, resp.TotalCount)
	})

	t.Run("Match tag only", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "action",
			PatternType: "keyword",
			MatchField:  "tag",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount) // 3 torrents have "action" in tag
	})

	t.Run("Match both title and tag", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "drama",
			PatternType: "keyword",
			MatchField:  "both",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount) // 1 torrent has "drama" in tag
	})

	t.Run("Default match field is both", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "action",
			PatternType: "keyword",
			// MatchField not specified, should default to "both"
			Limit: 10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 3, resp.MatchCount) // 3 torrents have "action" in tag
	})
}

func TestFilterRuleTestAPI_NoMatches(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action"},
		{SiteName: "test", TorrentID: "2", Title: "Movie 4K HDR", Tag: "comedy"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("No matches returns empty array", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "nonexistent_pattern_xyz",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 0, resp.MatchCount)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Empty(t, resp.Matches)
	})
}

func TestFilterRuleTestAPI_ResponseFormat(t *testing.T) {
	server, cleanup := setupFilterRuleTestServer(t)
	defer cleanup()

	// Add test torrents with tags
	db := global.GlobalDB.DB
	torrents := []models.TorrentInfo{
		{SiteName: "test", TorrentID: "1", Title: "Movie 1080p BluRay", Tag: "action,thriller"},
	}
	for _, torrent := range torrents {
		db.Create(&torrent)
	}

	t.Run("Response includes title and tag", func(t *testing.T) {
		reqBody := FilterRuleTestRequest{
			Pattern:     "1080p",
			PatternType: "keyword",
			Limit:       10,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/filter-rules/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session", Value: "test-session"})

		w := httptest.NewRecorder()
		server.testFilterRule(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp FilterRuleTestResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.MatchCount)
		require.Len(t, resp.Matches, 1)
		assert.Equal(t, "Movie 1080p BluRay", resp.Matches[0].Title)
		assert.Equal(t, "action,thriller", resp.Matches[0].Tag)
	})
}
