package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func setupTestDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	global.InitLogger(zap.NewNop())
	return db
}

func TestGetRSSFilterAssociation(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建过滤规则
	rule := models.FilterRule{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 创建关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 测试获取关联
	req := httptest.NewRequest(http.MethodGet, "/api/rss/1/filter-rules", nil)
	w := httptest.NewRecorder()
	server.getRSSFilterAssociation(w, req, rss.ID)

	require.Equal(t, http.StatusOK, w.Code)

	var resp RSSFilterAssociationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, rss.ID, resp.RSSID)
	require.Len(t, resp.FilterRuleIDs, 1)
	require.Equal(t, rule.ID, resp.FilterRuleIDs[0])
	require.Len(t, resp.FilterRules, 1)
	require.Equal(t, "test-rule", resp.FilterRules[0].Name)
}

func TestUpdateRSSFilterAssociation(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建过滤规则
	rule1 := models.FilterRule{
		Name:        "rule1",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule1).Error)

	rule2 := models.FilterRule{
		Name:        "rule2",
		Pattern:     ".*4K.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    2,
	}
	require.NoError(t, db.DB.Create(&rule2).Error)

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 测试更新关联
	reqBody := RSSFilterAssociationRequest{
		FilterRuleIDs: []uint{rule1.ID, rule2.ID},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateRSSFilterAssociation(w, req, rss.ID)

	require.Equal(t, http.StatusOK, w.Code)

	var resp RSSFilterAssociationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, rss.ID, resp.RSSID)
	require.Len(t, resp.FilterRuleIDs, 2)
}

func TestUpdateRSSFilterAssociation_ClearAssociations(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建过滤规则
	rule := models.FilterRule{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 创建初始关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 测试清空关联
	reqBody := RSSFilterAssociationRequest{
		FilterRuleIDs: []uint{},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateRSSFilterAssociation(w, req, rss.ID)

	require.Equal(t, http.StatusOK, w.Code)

	var resp RSSFilterAssociationResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.FilterRuleIDs)
}

func TestGetRSSFilterAssociation_RSSNotFound(t *testing.T) {
	_ = setupTestDB(t)

	// 创建服务器
	store := core.NewConfigStore(global.GlobalDB)
	server := NewServer(store, nil)

	// 测试获取不存在的 RSS
	req := httptest.NewRequest(http.MethodGet, "/api/rss/999/filter-rules", nil)
	w := httptest.NewRecorder()
	server.getRSSFilterAssociation(w, req, 999)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateRSSFilterAssociation_InvalidFilterRule(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 测试关联不存在的过滤规则
	reqBody := RSSFilterAssociationRequest{
		FilterRuleIDs: []uint{999},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateRSSFilterAssociation(w, req, rss.ID)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateRSSFilterAssociation_DisabledRuleRejected(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建过滤规则（先创建，再禁用，避免 GORM default:true 问题）
	rule := models.FilterRule{
		Name:        "disabled-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)
	// 显式禁用规则（绕过 GORM default:true 的零值问题）
	require.NoError(t, db.DB.Model(&rule).Update("enabled", false).Error)

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 测试关联禁用的过滤规则应该被拒绝
	reqBody := RSSFilterAssociationRequest{
		FilterRuleIDs: []uint{rule.ID},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/rss/1/filter-rules", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateRSSFilterAssociation(w, req, rss.ID)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "无法关联禁用的过滤规则")
}

func TestDisableFilterRule_ClearsAssociations(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建启用的过滤规则
	rule := models.FilterRule{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 创建关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))

	// 验证关联存在
	ruleIDs, err := assocDB.GetByRSSID(rss.ID)
	require.NoError(t, err)
	require.Len(t, ruleIDs, 1)

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 禁用规则
	reqBody := FilterRuleRequest{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     false, // 禁用
		Priority:    1,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateFilterRule(w, req, rule.ID)

	require.Equal(t, http.StatusOK, w.Code)

	// 验证关联已被清理
	ruleIDs, err = assocDB.GetByRSSID(rss.ID)
	require.NoError(t, err)
	require.Empty(t, ruleIDs)
}

func TestReenableFilterRule_DoesNotRestoreAssociations(t *testing.T) {
	db := setupTestDB(t)

	// 创建 RSS 订阅
	rss := models.RSSSubscription{
		Name:            "test-rss",
		URL:             "http://example.com/rss",
		IntervalMinutes: 10,
	}
	require.NoError(t, db.DB.Create(&rss).Error)

	// 创建启用的过滤规则
	rule := models.FilterRule{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     true,
		Priority:    1,
	}
	require.NoError(t, db.DB.Create(&rule).Error)

	// 创建关联
	assocDB := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assocDB.SetFilterRulesForRSS(rss.ID, []uint{rule.ID}))

	// 创建服务器
	store := core.NewConfigStore(db)
	server := NewServer(store, nil)

	// 禁用规则
	reqBody := FilterRuleRequest{
		Name:        "test-rule",
		Pattern:     ".*1080p.*",
		PatternType: "regex",
		Enabled:     false,
		Priority:    1,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.updateFilterRule(w, req, rule.ID)
	require.Equal(t, http.StatusOK, w.Code)

	// 验证关联已被清理
	ruleIDs, err := assocDB.GetByRSSID(rss.ID)
	require.NoError(t, err)
	require.Empty(t, ruleIDs)

	// 重新启用规则
	reqBody.Enabled = true
	body, _ = json.Marshal(reqBody)
	req = httptest.NewRequest(http.MethodPut, "/api/filter-rules/1", bytes.NewReader(body))
	w = httptest.NewRecorder()
	server.updateFilterRule(w, req, rule.ID)
	require.Equal(t, http.StatusOK, w.Code)

	// 验证关联没有恢复
	ruleIDs, err = assocDB.GetByRSSID(rss.ID)
	require.NoError(t, err)
	require.Empty(t, ruleIDs)
}
