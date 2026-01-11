package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// RSSFilterAssociationRequest RSS-Filter 关联请求结构
type RSSFilterAssociationRequest struct {
	FilterRuleIDs []uint `json:"filter_rule_ids"`
}

// RSSFilterAssociationResponse RSS-Filter 关联响应结构
type RSSFilterAssociationResponse struct {
	RSSID         uint                 `json:"rss_id"`
	FilterRuleIDs []uint               `json:"filter_rule_ids"`
	FilterRules   []FilterRuleResponse `json:"filter_rules"`
}

// apiRSSFilterAssociation 处理 RSS-Filter 关联
// GET /api/rss/:id/filter-rules - 获取 RSS 关联的过滤规则
// PUT /api/rss/:id/filter-rules - 更新 RSS 关联的过滤规则
func (s *Server) apiRSSFilterAssociation(w http.ResponseWriter, r *http.Request) {
	// 解析 RSS ID
	path := strings.TrimPrefix(r.URL.Path, "/api/rss/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "filter-rules" {
		http.Error(w, "无效的路径", http.StatusBadRequest)
		return
	}

	rssID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "无效的 RSS ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getRSSFilterAssociation(w, r, uint(rssID))
	case http.MethodPut:
		s.updateRSSFilterAssociation(w, r, uint(rssID))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// getRSSFilterAssociation 获取 RSS 关联的过滤规则
func (s *Server) getRSSFilterAssociation(w http.ResponseWriter, r *http.Request, rssID uint) {
	// 验证 RSS 是否存在
	db := global.GlobalDB.DB
	var rss models.RSSSubscription
	if err := db.First(&rss, rssID).Error; err != nil {
		http.Error(w, "RSS 订阅不存在", http.StatusNotFound)
		return
	}

	// 获取关联的过滤规则
	assocDB := models.NewRSSFilterAssociationDB(db)
	rules, err := assocDB.GetFilterRulesForRSS(rssID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 构建响应
	ruleIDs := make([]uint, len(rules))
	ruleResponses := make([]FilterRuleResponse, len(rules))
	for i, rule := range rules {
		ruleIDs[i] = rule.ID
		ruleResponses[i] = toFilterRuleResponse(rule)
	}

	writeJSON(w, RSSFilterAssociationResponse{
		RSSID:         rssID,
		FilterRuleIDs: ruleIDs,
		FilterRules:   ruleResponses,
	})
}

// updateRSSFilterAssociation 更新 RSS 关联的过滤规则
func (s *Server) updateRSSFilterAssociation(w http.ResponseWriter, r *http.Request, rssID uint) {
	// 验证 RSS 是否存在
	db := global.GlobalDB.DB
	var rss models.RSSSubscription
	if err := db.First(&rss, rssID).Error; err != nil {
		http.Error(w, "RSS 订阅不存在", http.StatusNotFound)
		return
	}

	// 解析请求
	var req RSSFilterAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证过滤规则是否存在且已启用
	if len(req.FilterRuleIDs) > 0 {
		filterDB := models.NewFilterRuleDB(global.GlobalDB)
		for _, ruleID := range req.FilterRuleIDs {
			rule, err := filterDB.GetByID(ruleID)
			if err != nil {
				http.Error(w, "过滤规则不存在: "+strconv.FormatUint(uint64(ruleID), 10), http.StatusBadRequest)
				return
			}
			// 禁止关联禁用的规则
			if !rule.Enabled {
				http.Error(w, "无法关联禁用的过滤规则: "+rule.Name, http.StatusBadRequest)
				return
			}
		}
	}

	// 更新关联
	assocDB := models.NewRSSFilterAssociationDB(db)
	if err := assocDB.SetFilterRulesForRSS(rssID, req.FilterRuleIDs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[RSSFilter] 更新 RSS 过滤规则关联: rss_id=%d, rule_ids=%v", rssID, req.FilterRuleIDs)

	// 返回更新后的关联
	s.getRSSFilterAssociation(w, r, rssID)
}
