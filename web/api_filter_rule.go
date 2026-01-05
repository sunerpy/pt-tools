package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
)

// FilterRuleRequest 过滤规则请求结构
type FilterRuleRequest struct {
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	PatternType string `json:"pattern_type"` // keyword, wildcard, regex
	MatchField  string `json:"match_field"`  // title, tag, both
	RequireFree bool   `json:"require_free"`
	Enabled     bool   `json:"enabled"`
	SiteID      *uint  `json:"site_id"`
	RSSID       *uint  `json:"rss_id"`
	Priority    int    `json:"priority"`
}

// FilterRuleResponse 过滤规则响应结构
type FilterRuleResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	PatternType string `json:"pattern_type"`
	MatchField  string `json:"match_field"`
	RequireFree bool   `json:"require_free"`
	Enabled     bool   `json:"enabled"`
	SiteID      *uint  `json:"site_id"`
	RSSID       *uint  `json:"rss_id"`
	Priority    int    `json:"priority"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// FilterRuleTestRequest 过滤规则测试请求
type FilterRuleTestRequest struct {
	Pattern     string `json:"pattern"`
	PatternType string `json:"pattern_type"`
	MatchField  string `json:"match_field"`  // title, tag, both
	RequireFree bool   `json:"require_free"` // 是否仅匹配免费种子
	SiteID      *uint  `json:"site_id"`
	RSSID       *uint  `json:"rss_id"`
	Limit       int    `json:"limit"` // 最多返回多少条匹配结果
}

// FilterRuleTestMatch 单个匹配结果
type FilterRuleTestMatch struct {
	Title  string `json:"title"`
	Tag    string `json:"tag"`
	IsFree bool   `json:"is_free"` // 是否免费
}

// FilterRuleTestResponse 过滤规则测试响应
type FilterRuleTestResponse struct {
	MatchCount int                   `json:"match_count"`
	TotalCount int                   `json:"total_count"` // 总种子数
	Matches    []FilterRuleTestMatch `json:"matches"`     // 匹配的种子详情
}

// apiFilterRules 处理过滤规则列表和创建
// GET /api/filter-rules - 列出所有过滤规则
// POST /api/filter-rules - 创建新过滤规则
func (s *Server) apiFilterRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listFilterRules(w, r)
	case http.MethodPost:
		s.createFilterRule(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiFilterRuleDetail 处理单个过滤规则的操作
// GET /api/filter-rules/:id - 获取过滤规则详情
// PUT /api/filter-rules/:id - 更新过滤规则
// DELETE /api/filter-rules/:id - 删除过滤规则
func (s *Server) apiFilterRuleDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/filter-rules/")

	// 检查是否是测试路径
	if path == "test" {
		s.testFilterRule(w, r)
		return
	}

	id, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		http.Error(w, "无效的过滤规则ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getFilterRule(w, r, uint(id))
	case http.MethodPut:
		s.updateFilterRule(w, r, uint(id))
	case http.MethodDelete:
		s.deleteFilterRule(w, r, uint(id))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// listFilterRules 列出所有过滤规则
func (s *Server) listFilterRules(w http.ResponseWriter, r *http.Request) {
	filterDB := models.NewFilterRuleDB(global.GlobalDB)
	rules, err := filterDB.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responses := make([]FilterRuleResponse, len(rules))
	for i, rule := range rules {
		responses[i] = toFilterRuleResponse(rule)
	}

	writeJSON(w, responses)
}

// createFilterRule 创建新过滤规则
func (s *Server) createFilterRule(w http.ResponseWriter, r *http.Request) {
	var req FilterRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证必填字段
	if req.Name == "" {
		http.Error(w, "名称不能为空", http.StatusBadRequest)
		return
	}
	if req.Pattern == "" {
		http.Error(w, "匹配模式不能为空", http.StatusBadRequest)
		return
	}

	// 验证模式类型
	patternType := models.PatternType(req.PatternType)
	if patternType == "" {
		patternType = models.PatternKeyword
	}
	if patternType != models.PatternKeyword && patternType != models.PatternWildcard && patternType != models.PatternRegex {
		http.Error(w, "不支持的模式类型", http.StatusBadRequest)
		return
	}

	// 验证模式是否有效
	if err := validatePattern(patternType, req.Pattern); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filterDB := models.NewFilterRuleDB(global.GlobalDB)

	// 检查名称是否已存在
	exists, err := filterDB.Exists(req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exists {
		http.Error(w, "过滤规则名称已存在", http.StatusBadRequest)
		return
	}

	// 设置默认优先级
	priority := req.Priority
	if priority <= 0 {
		priority = 100
	}

	// 设置匹配字段，默认为 both
	matchField := models.MatchField(req.MatchField)
	if matchField == "" {
		matchField = models.MatchFieldBoth
	}
	if matchField != models.MatchFieldTitle && matchField != models.MatchFieldTag && matchField != models.MatchFieldBoth {
		http.Error(w, "不支持的匹配字段类型", http.StatusBadRequest)
		return
	}

	rule := &models.FilterRule{
		Name:        req.Name,
		Pattern:     req.Pattern,
		PatternType: patternType,
		MatchField:  matchField,
		RequireFree: req.RequireFree,
		Enabled:     req.Enabled,
		SiteID:      req.SiteID,
		RSSID:       req.RSSID,
		Priority:    priority,
	}

	if err := filterDB.Create(rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[FilterRule] 创建过滤规则: name=%s, pattern=%s, type=%s", req.Name, req.Pattern, req.PatternType)

	writeJSON(w, toFilterRuleResponse(*rule))
}

// getFilterRule 获取过滤规则详情
func (s *Server) getFilterRule(w http.ResponseWriter, r *http.Request, id uint) {
	filterDB := models.NewFilterRuleDB(global.GlobalDB)
	rule, err := filterDB.GetByID(id)
	if err != nil {
		http.Error(w, "过滤规则不存在", http.StatusNotFound)
		return
	}

	writeJSON(w, toFilterRuleResponse(*rule))
}

// updateFilterRule 更新过滤规则
func (s *Server) updateFilterRule(w http.ResponseWriter, r *http.Request, id uint) {
	var req FilterRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filterDB := models.NewFilterRuleDB(global.GlobalDB)
	rule, err := filterDB.GetByID(id)
	if err != nil {
		http.Error(w, "过滤规则不存在", http.StatusNotFound)
		return
	}

	// 记录原始启用状态，用于判断是否需要清理关联
	wasEnabled := rule.Enabled

	// 如果名称变更，检查是否与其他规则冲突
	if req.Name != "" && req.Name != rule.Name {
		exists, err := filterDB.ExistsExcluding(req.Name, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if exists {
			http.Error(w, "过滤规则名称已存在", http.StatusBadRequest)
			return
		}
		rule.Name = req.Name
	}

	// 验证并更新模式
	if req.Pattern != "" {
		patternType := models.PatternType(req.PatternType)
		if patternType == "" {
			patternType = rule.PatternType
		}
		if err := validatePattern(patternType, req.Pattern); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rule.Pattern = req.Pattern
		rule.PatternType = patternType
	} else if req.PatternType != "" {
		// 只更新类型，需要验证现有模式
		patternType := models.PatternType(req.PatternType)
		if err := validatePattern(patternType, rule.Pattern); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rule.PatternType = patternType
	}

	// 更新匹配字段
	if req.MatchField != "" {
		matchField := models.MatchField(req.MatchField)
		if matchField != models.MatchFieldTitle && matchField != models.MatchFieldTag && matchField != models.MatchFieldBoth {
			http.Error(w, "不支持的匹配字段类型", http.StatusBadRequest)
			return
		}
		rule.MatchField = matchField
	}

	rule.RequireFree = req.RequireFree
	rule.Enabled = req.Enabled
	rule.SiteID = req.SiteID
	rule.RSSID = req.RSSID
	if req.Priority > 0 {
		rule.Priority = req.Priority
	}

	if err := filterDB.Update(rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 如果规则从启用变为禁用，清理所有 RSS 关联
	if wasEnabled && !req.Enabled {
		assocDB := models.NewRSSFilterAssociationDB(global.GlobalDB.DB)
		if err := assocDB.DeleteByFilterRuleID(id); err != nil {
			global.GetSlogger().Warnf("[FilterRule] 清理 RSS 关联失败: id=%d, error=%v", id, err)
		} else {
			global.GetSlogger().Infof("[FilterRule] 规则禁用，已清理 RSS 关联: id=%d, name=%s", id, rule.Name)
		}
	}

	global.GetSlogger().Infof("[FilterRule] 更新过滤规则: id=%d, name=%s", id, rule.Name)

	writeJSON(w, toFilterRuleResponse(*rule))
}

// deleteFilterRule 删除过滤规则
func (s *Server) deleteFilterRule(w http.ResponseWriter, r *http.Request, id uint) {
	filterDB := models.NewFilterRuleDB(global.GlobalDB)

	rule, err := filterDB.GetByID(id)
	if err != nil {
		http.Error(w, "过滤规则不存在", http.StatusNotFound)
		return
	}

	if err := filterDB.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[FilterRule] 删除过滤规则: id=%d, name=%s", id, rule.Name)

	writeJSON(w, map[string]string{"status": "deleted"})
}

// testFilterRule 测试过滤规则
func (s *Server) testFilterRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req FilterRuleTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 收到测试请求: pattern=%s, type=%s, field=%s, require_free=%v", req.Pattern, req.PatternType, req.MatchField, req.RequireFree)

	if req.Pattern == "" {
		http.Error(w, "匹配模式不能为空", http.StatusBadRequest)
		return
	}

	// 验证模式类型
	patternType := filter.PatternType(req.PatternType)
	if patternType == "" {
		patternType = filter.PatternKeyword
	}

	// 创建匹配器
	matcher, err := filter.NewMatcher(patternType, req.Pattern)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 设置默认限制
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 设置匹配字段，默认为 both
	matchField := models.MatchField(req.MatchField)
	if matchField == "" {
		matchField = models.MatchFieldBoth
	}

	// 如果指定了 RSS ID，从 RSS 订阅获取种子
	if req.RSSID != nil {
		s.testFilterRuleWithRSS(w, matcher, matchField, req.RequireFree, *req.RSSID, limit)
		return
	}

	// 否则从数据库获取种子列表进行匹配测试
	db := global.GlobalDB.DB
	tx := db.Model(&models.TorrentInfo{})

	if req.SiteID != nil {
		var site models.SiteSetting
		if err := db.First(&site, *req.SiteID).Error; err == nil {
			tx = tx.Where("site_name = ?", site.Name)
		}
	}

	var torrents []models.TorrentInfo
	if err := tx.Order("created_at DESC").Limit(500).Find(&torrents).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 匹配种子
	var matches []FilterRuleTestMatch
	global.GetSlogger().Debugf("[FilterRuleTest] 开始匹配种子，总数: %d", len(torrents))
	for _, t := range torrents {
		isMatch := matchesField(matcher, matchField, t.Title, t.Tag)
		global.GetSlogger().Debugf("[FilterRuleTest] 尝试匹配: title=%s, tag=%s, matched=%v", t.Title, t.Tag, isMatch)
		if isMatch {
			matches = append(matches, FilterRuleTestMatch{
				Title: t.Title,
				Tag:   t.Tag,
			})
			if len(matches) >= limit {
				break
			}
		}
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 匹配完成: 匹配数=%d, 总数=%d", len(matches), len(torrents))
	writeJSON(w, FilterRuleTestResponse{
		MatchCount: len(matches),
		TotalCount: len(torrents),
		Matches:    matches,
	})
}

// testFilterRuleWithRSS 使用 RSS 订阅测试过滤规则
func (s *Server) testFilterRuleWithRSS(w http.ResponseWriter, matcher filter.PatternMatcher, matchField models.MatchField, requireFree bool, rssID uint, limit int) {
	db := global.GlobalDB.DB

	// 获取 RSS 订阅
	var rss models.RSSSubscription
	if err := db.First(&rss, rssID).Error; err != nil {
		http.Error(w, "RSS 订阅不存在", http.StatusNotFound)
		return
	}

	// 获取站点信息用于解析
	var site models.SiteSetting
	if err := db.First(&site, rss.SiteID).Error; err != nil {
		http.Error(w, "站点不存在", http.StatusNotFound)
		return
	}

	// 获取 RSS feed
	feed, err := fetchRSSFeedForTest(rss.URL)
	if err != nil {
		http.Error(w, "获取 RSS 失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 根据站点类型获取种子详情并匹配
	siteName := models.SiteGroup(site.Name)
	totalCount := len(feed.Items)

	global.GetSlogger().Debugf("[FilterRuleTest] 站点: %s, RSS: %s, 总条目数: %d, requireFree: %v", siteName, rss.Name, totalCount, requireFree)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 使用并发获取种子详情
	type detailResult struct {
		item   *gofeed.Item
		detail *internal.TorrentDetailForTest
		err    error
	}

	resultChan := make(chan detailResult)
	matchChan := make(chan FilterRuleTestMatch, limit)
	var wg, processingWg sync.WaitGroup

	// 控制并发数，避免过多请求
	maxConcurrent := 10
	semaphore := make(chan struct{}, maxConcurrent)

	// 启动结果处理 goroutine
	processingWg.Add(1)
	go func() {
		defer processingWg.Done()
		matchCount := 0
		for result := range resultChan {
			if result.err != nil {
				global.GetSlogger().Debugf("[FilterRuleTest] 获取种子详情失败: %v", result.err)
				continue
			}

			title := result.detail.Title
			tag := result.detail.Tag
			isFree := result.detail.IsFree

			global.GetSlogger().Debugf("[FilterRuleTest] 种子: GUID=%s, Title=%s, Tag=%s, IsFree=%v", result.item.GUID, title, tag, isFree)

			// 检查模式匹配
			if !matchesField(matcher, matchField, title, tag) {
				continue
			}

			// 检查免费筛选
			if requireFree && !isFree {
				global.GetSlogger().Debugf("[FilterRuleTest] 跳过非免费种子: %s", title)
				continue
			}

			if matchCount < limit {
				matchChan <- FilterRuleTestMatch{
					Title:  title,
					Tag:    tag,
					IsFree: isFree,
				}
				matchCount++
			}
		}
		close(matchChan)
	}()

	// 启动种子详情获取
	for _, item := range feed.Items {
		wg.Add(1)
		go func(item *gofeed.Item) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			detail, err := internal.GetTorrentDetailForTest(ctx, siteName, item)
			resultChan <- detailResult{
				item:   item,
				detail: detail,
				err:    err,
			}
		}(item)
	}

	// 等待所有详情获取完成
	wg.Wait()
	close(resultChan)

	// 等待处理完成
	processingWg.Wait()

	// 收集匹配结果
	var matches []FilterRuleTestMatch
	for match := range matchChan {
		matches = append(matches, match)
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 匹配结果: %d/%d", len(matches), totalCount)

	writeJSON(w, FilterRuleTestResponse{
		MatchCount: len(matches),
		TotalCount: totalCount,
		Matches:    matches,
	})
}

// matchesField 根据匹配字段配置进行匹配
func matchesField(matcher filter.PatternMatcher, matchField models.MatchField, title, tag string) bool {
	switch matchField {
	case models.MatchFieldTitle:
		return matcher.Match(title)
	case models.MatchFieldTag:
		return matcher.Match(tag)
	case models.MatchFieldBoth:
		fallthrough
	default:
		return matcher.Match(title) || matcher.Match(tag)
	}
}

// fetchRSSFeedForTest 获取 RSS feed 用于测试
func fetchRSSFeedForTest(url string) (*gofeed.Feed, error) {
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %v", err)
	}
	return feed, nil
}

// validatePattern 验证模式是否有效
func validatePattern(patternType models.PatternType, pattern string) error {
	filterPatternType := filter.PatternType(patternType)
	_, err := filter.NewMatcher(filterPatternType, pattern)
	return err
}

// toFilterRuleResponse 转换为响应格式
func toFilterRuleResponse(rule models.FilterRule) FilterRuleResponse {
	matchField := string(rule.MatchField)
	if matchField == "" {
		matchField = string(models.MatchFieldBoth)
	}
	return FilterRuleResponse{
		ID:          rule.ID,
		Name:        rule.Name,
		Pattern:     rule.Pattern,
		PatternType: string(rule.PatternType),
		MatchField:  matchField,
		RequireFree: rule.RequireFree,
		Enabled:     rule.Enabled,
		SiteID:      rule.SiteID,
		RSSID:       rule.RSSID,
		Priority:    rule.Priority,
		CreatedAt:   rule.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   rule.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
