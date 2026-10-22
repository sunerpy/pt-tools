package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

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
	MinSizeGB   int    `json:"min_size_gb"`
	MaxSizeGB   int    `json:"max_size_gb"`
	Enabled     bool   `json:"enabled"`
	SiteID      *uint  `json:"site_id"`
	RSSID       *uint  `json:"rss_id"`
	Priority    int    `json:"priority"`
	// Purpose 是规则用途：download（下载，默认）、notify（只用于命中规则的通知）、both。更新时空着表示不改
	Purpose string `json:"purpose"`
}

// FilterRuleResponse 过滤规则响应结构
type FilterRuleResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Pattern     string `json:"pattern"`
	PatternType string `json:"pattern_type"`
	MatchField  string `json:"match_field"`
	RequireFree bool   `json:"require_free"`
	MinSizeGB   int    `json:"min_size_gb"`
	MaxSizeGB   int    `json:"max_size_gb"`
	Enabled     bool   `json:"enabled"`
	SiteID      *uint  `json:"site_id"`
	RSSID       *uint  `json:"rss_id"`
	Priority    int    `json:"priority"`
	Purpose     string `json:"purpose"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// FilterRuleTestRequest 过滤规则测试请求
type FilterRuleTestRequest struct {
	Pattern     string  `json:"pattern"`
	PatternType string  `json:"pattern_type"`
	MatchField  string  `json:"match_field"`  // title, tag, both
	RequireFree bool    `json:"require_free"` // 是否仅匹配免费种子
	MinSizeGB   int     `json:"min_size_gb"`
	MaxSizeGB   int     `json:"max_size_gb"`
	TestSizeGB  float64 `json:"test_size_gb"` // 模拟种子大小，用于完整决策测试
	TestIsFree  *bool   `json:"test_is_free"` // 覆盖 is_free 状态（nil=使用真实值）
	GlobalSize  int     `json:"global_size"`  // 模拟全局大小上限
	FilterMode  string  `json:"filter_mode"`  // 模拟 FilterMode
	SiteID      *uint   `json:"site_id"`
	RSSID       *uint   `json:"rss_id"`
	Limit       int     `json:"limit"` // 最多返回多少条匹配结果
}

// FilterRuleTestMatch 单个匹配结果
type FilterRuleTestMatch struct {
	Title    string  `json:"title"`
	Tag      string  `json:"tag"`
	IsFree   bool    `json:"is_free"`  // 是否免费
	SizeGB   float64 `json:"size_gb"`  // 种子大小（GB）
	Decision string  `json:"decision"` // 完整决策结果: downloaded / skipped
	Reason   string  `json:"reason"`   // 跳过原因（如果有）
	Source   string  `json:"source"`   // 下载通道: filter_rule / free_download / ""
	// DetailMissing 是这个种子的详情没取到（RSS 试跑）：标题按 RSS 里的，免费状态不知道（按非免费算）
	DetailMissing bool `json:"detail_missing,omitempty"`
}

// FilterRuleTestResponse 过滤规则测试响应
type FilterRuleTestResponse struct {
	MatchCount int                   `json:"match_count"`
	TotalCount int                   `json:"total_count"` // 总种子数
	Matches    []FilterRuleTestMatch `json:"matches"`     // 匹配的种子详情
	// Source 是这次试跑用的数据：history（最近入库的种子记录）或 rss（现拉的 RSS）
	Source string `json:"source"`
	// SourceName 是数据源的说明：RSS 的名称与站点，或「最近入库的种子记录（最多 N 条）」
	SourceName string `json:"source_name"`
	// DetailFailed 是 RSS 里详情没取到的条目数：它们按 RSS 里的标题算，免费状态不知道
	DetailFailed int `json:"detail_failed,omitempty"`
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

	purpose, ok := rulePurpose(req.Purpose)
	if !ok {
		http.Error(w, errRulePurpose, http.StatusBadRequest)
		return
	}

	rule := &models.FilterRule{
		Name:        req.Name,
		Pattern:     req.Pattern,
		PatternType: patternType,
		MatchField:  matchField,
		RequireFree: req.RequireFree,
		MinSizeGB:   sanitizeRuleSize(req.MinSizeGB),
		MaxSizeGB:   sanitizeRuleSize(req.MaxSizeGB),
		Enabled:     req.Enabled,
		SiteID:      req.SiteID,
		RSSID:       req.RSSID,
		Priority:    priority,
		Purpose:     purpose,
	}

	// require_free 与 enabled 的列默认是 true：GORM 建记录时把 false 当成没给、写成默认值，不勾的在同一个事务里再写一次
	if err := global.GlobalDB.DB.Transaction(func(tx *gorm.DB) error {
		if err := models.NewFilterRuleDB(&models.TorrentDB{DB: tx}).Create(rule); err != nil {
			return err
		}
		if req.RequireFree && req.Enabled {
			return nil
		}
		if err := tx.Model(rule).Updates(map[string]any{"require_free": req.RequireFree, "enabled": req.Enabled}).Error; err != nil {
			return err
		}
		rule.RequireFree, rule.Enabled = req.RequireFree, req.Enabled
		return nil
	}); err != nil {
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

// updateFilterRule 更新过滤规则。请求是整条规则：require_free、enabled、大小与 site_id/rss_id 按请求覆盖，
// 名称、模式、模式类型、匹配范围、用途空着（优先级为 0）时保留原值。
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

	// 更新用途：空着表示不改（老版本的前端不发这个字段）
	if req.Purpose != "" {
		purpose, ok := rulePurpose(req.Purpose)
		if !ok {
			http.Error(w, errRulePurpose, http.StatusBadRequest)
			return
		}
		rule.Purpose = purpose
	}

	rule.RequireFree = req.RequireFree
	rule.MinSizeGB = sanitizeRuleSize(req.MinSizeGB)
	rule.MaxSizeGB = sanitizeRuleSize(req.MaxSizeGB)
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

	rt := ruleTest{
		matcher:    matcher,
		matchField: matchField,
		rule: &models.FilterRule{
			Pattern:     req.Pattern,
			PatternType: models.PatternType(patternType),
			MatchField:  matchField,
			RequireFree: req.RequireFree,
			MinSizeGB:   sanitizeRuleSize(req.MinSizeGB),
			MaxSizeGB:   sanitizeRuleSize(req.MaxSizeGB),
			Enabled:     true,
		},
		mode:       models.NormalizeFilterMode(models.FilterMode(req.FilterMode)),
		globalSize: req.GlobalSize,
		testSizeGB: req.TestSizeGB,
		testIsFree: req.TestIsFree,
		limit:      limit,
	}

	// 指定了 RSS 时现拉一次这个 RSS 来试
	if req.RSSID != nil {
		s.testFilterRuleWithRSS(w, r.Context(), rt, *req.RSSID)
		return
	}

	// 否则用最近入库的种子记录来试
	db := global.GlobalDB.DB
	tx := db.Model(&models.TorrentInfo{})
	sourceName := fmt.Sprintf("最近入库的种子记录（最多 %d 条）", historyTestRows)

	if req.SiteID != nil {
		var site models.SiteSetting
		if err := db.First(&site, *req.SiteID).Error; err == nil {
			tx = tx.Where("site_name = ?", site.Name)
			sourceName = fmt.Sprintf("%s 最近入库的种子记录（最多 %d 条）", site.Name, historyTestRows)
		}
	}

	var torrents []models.TorrentInfo
	if err := tx.Order("created_at DESC").Limit(historyTestRows).Find(&torrents).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 开始匹配种子，总数: %d", len(torrents))
	matches := []FilterRuleTestMatch{}
	for _, t := range torrents {
		m, ok := rt.evaluate(t.Title, t.Tag, t.TorrentSize, t.IsFree)
		if !ok {
			continue
		}
		matches = append(matches, m)
		if len(matches) >= limit {
			break
		}
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 匹配完成: 匹配数=%d, 总数=%d", len(matches), len(torrents))
	writeJSON(w, FilterRuleTestResponse{
		MatchCount: len(matches),
		TotalCount: len(torrents),
		Matches:    matches,
		Source:     testSourceHistory,
		SourceName: sourceName,
	})
}

// historyTestRows 是用历史记录试跑时看最近多少条入库记录。
const historyTestRows = 500

// 试跑用的数据。
const (
	testSourceHistory = "history"
	testSourceRSS     = "rss"
)

// rssTestDetailTimeout 是 RSS 试跑取种子详情的总时限：到点还没取到的按 RSS 里的信息算。
const rssTestDetailTimeout = 2 * time.Minute

// rssTestConcurrency 是 RSS 试跑同时取几个种子的详情。
const rssTestConcurrency = 8

// ruleTest 是一次试跑：规则、模拟的条件与最多返回几条命中。历史记录与 RSS 两条路用同一套判断。
type ruleTest struct {
	matcher    filter.PatternMatcher
	matchField models.MatchField
	rule       *models.FilterRule
	mode       models.FilterMode
	globalSize int
	testSizeGB float64
	testIsFree *bool
	limit      int
}

// evaluate 看一个种子：没命中模式时第二个返回值为假；命中时按完整决策（大小、免费、下载模式、全局上限）给出下不下与原因。
func (t ruleTest) evaluate(title, tag string, sizeBytes int64, isFree bool) (FilterRuleTestMatch, bool) {
	if !matchesField(t.matcher, t.matchField, title, tag) {
		return FilterRuleTestMatch{}, false
	}
	sizeGB := bytesToGB(sizeBytes)
	if t.testSizeGB > 0 {
		sizeGB = t.testSizeGB
	}
	if t.testIsFree != nil {
		isFree = *t.testIsFree
	}
	decision := evaluateTestDecision(t.rule, t.mode, t.globalSize, sizeGB, isFree)
	return FilterRuleTestMatch{
		Title:    title,
		Tag:      tag,
		IsFree:   isFree,
		SizeGB:   sizeGB,
		Decision: decisionLabel(decision.ShouldDownload),
		Reason:   decision.Reason,
		Source:   decision.Source,
	}, true
}

// testFilterRuleWithRSS 现拉一次 RSS 来试：每个条目取一次详情（标题、副标题、免费、大小），和 RSS 任务看到的一样；
// 取不到详情的按 RSS 里的标题算，免费状态不知道（按非免费），并在结果里标出来。只读，不推送不写库。
// rssTestDetail 取试跑用的种子详情（测试里换掉）。
var rssTestDetail = internal.GetTorrentDetailForTest

func (s *Server) testFilterRuleWithRSS(w http.ResponseWriter, ctx context.Context, rt ruleTest, rssID uint) {
	db := global.GlobalDB.DB

	var rss models.RSSSubscription
	if err := db.First(&rss, rssID).Error; err != nil {
		http.Error(w, "RSS 订阅不存在", http.StatusNotFound)
		return
	}
	var site models.SiteSetting
	if err := db.First(&site, rss.SiteID).Error; err != nil {
		http.Error(w, "站点不存在", http.StatusNotFound)
		return
	}
	if rss.IsExample {
		http.Error(w, "这是示例 RSS，不会真的去拉：到站点页添加你自己的 RSS 链接再试", http.StatusBadRequest)
		return
	}

	feed, err := internal.FetchRSSFeed(ctx, rss.URL)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, internal.ErrRSSPlaceholder) {
			status = http.StatusBadRequest
		}
		http.Error(w, "获取 RSS「"+rss.Name+"」失败："+err.Error(), status)
		return
	}

	siteName := models.SiteGroup(site.Name)
	global.GetSlogger().Debugf("[FilterRuleTest] 站点: %s, RSS: %s, 总条目数: %d", siteName, rss.Name, len(feed.Items))

	detailCtx, cancel := context.WithTimeout(ctx, rssTestDetailTimeout)
	defer cancel()
	details := make([]*internal.TorrentDetailForTest, len(feed.Items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, rssTestConcurrency)
	for i, item := range feed.Items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-detailCtx.Done():
				return
			}
			defer func() { <-sem }()
			d, err := rssTestDetail(detailCtx, siteName, item)
			if err == nil {
				details[i] = d
			}
		}()
	}
	wg.Wait()

	// 按 RSS 里的顺序（站点给的通常是新的在前）给出命中
	matches := []FilterRuleTestMatch{}
	missing := 0
	for i, item := range feed.Items {
		d := details[i]
		if d == nil {
			d = &internal.TorrentDetailForTest{Title: item.Title, DetailMissing: true}
			if len(item.Categories) > 0 {
				d.Tag = strings.Join(item.Categories, ",")
			}
		}
		if d.DetailMissing {
			missing++
		}
		if len(matches) >= rt.limit {
			continue
		}
		// 和 RSS 任务一样按 RSS 条目的标题匹配（详情页上的标题有的站点取不到、有的是另一种写法），副标题、免费与大小按详情
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = d.Title
		}
		m, ok := rt.evaluate(title, d.Tag, d.SizeBytes, d.IsFree)
		if !ok {
			continue
		}
		m.DetailMissing = d.DetailMissing
		matches = append(matches, m)
	}

	global.GetSlogger().Debugf("[FilterRuleTest] 匹配结果: %d/%d，详情没取到 %d 条", len(matches), len(feed.Items), missing)
	writeJSON(w, FilterRuleTestResponse{
		MatchCount:   len(matches),
		TotalCount:   len(feed.Items),
		Matches:      matches,
		Source:       testSourceRSS,
		SourceName:   fmt.Sprintf("%s（%s）", rss.Name, site.Name),
		DetailFailed: missing,
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

// validatePattern 验证模式是否有效
func validatePattern(patternType models.PatternType, pattern string) error {
	filterPatternType := filter.PatternType(patternType)
	_, err := filter.NewMatcher(filterPatternType, pattern)
	return err
}

// errRulePurpose 是规则用途不对时的提示
const errRulePurpose = "规则用途只能是 download（下载）、notify（通知）或 both（两者）"

// rulePurpose 检查请求里的规则用途：空着是 download（和库里的默认值、过滤时对空值的理解一致）。
func rulePurpose(v string) (string, bool) {
	switch p := filter.Purpose(strings.TrimSpace(v)); p {
	case "":
		return string(filter.PurposeDownload), true
	case filter.PurposeDownload, filter.PurposeNotify, filter.PurposeBoth:
		return string(p), true
	}
	return "", false
}

// toFilterRuleResponse 转换为响应格式
func toFilterRuleResponse(rule models.FilterRule) FilterRuleResponse {
	matchField := string(rule.MatchField)
	if matchField == "" {
		matchField = string(models.MatchFieldBoth)
	}
	// 和过滤时一样：库里的空值与不认识的值都按下载规则返回，前端回传时也就是合法的值
	purpose := string(filter.NormalizePurpose(rule.Purpose))
	return FilterRuleResponse{
		ID:          rule.ID,
		Name:        rule.Name,
		Pattern:     rule.Pattern,
		PatternType: string(rule.PatternType),
		MatchField:  matchField,
		RequireFree: rule.RequireFree,
		MinSizeGB:   rule.MinSizeGB,
		MaxSizeGB:   rule.MaxSizeGB,
		Enabled:     rule.Enabled,
		SiteID:      rule.SiteID,
		RSSID:       rule.RSSID,
		Priority:    rule.Priority,
		Purpose:     purpose,
		CreatedAt:   rule.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   rule.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

// sanitizeRuleSize clamps negative values to 0 (meaning "no bound").
func sanitizeRuleSize(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// evaluateTestDecision mirrors filter.Decide semantics for the rule-tester UI.
// It returns the same Decision shape but operates on a single in-memory rule
// candidate rather than the DB-backed rule cache.
// Plan A: since the tester is always testing with a rule present, the "rules
// associated" flag is conceptually true — non-matching conditions do NOT fall
// back to the free channel under auto_free.
func evaluateTestDecision(rule *models.FilterRule, mode models.FilterMode, globalSizeGB int, sizeGB float64, isFree bool) filter.Decision {
	if globalSizeGB > 0 && sizeGB > float64(globalSizeGB) {
		return filter.Decision{ShouldDownload: false, Source: filter.SourceNone, Reason: "超出全局大小限制"}
	}
	if mode == models.FilterModeFreeOnly {
		if isFree {
			return filter.Decision{ShouldDownload: true, Source: filter.SourceFreeDownload}
		}
		return filter.Decision{ShouldDownload: false, Source: filter.SourceNone, Reason: "free_only 模式下过滤规则通道已关闭且非免费"}
	}
	if rule.RequireFree && !isFree {
		return filter.Decision{ShouldDownload: false, MatchedRule: rule, Source: filter.SourceNone, Reason: "匹配规则要求免费，但种子非免费"}
	}
	if !rule.MatchesSize(sizeGB) {
		return filter.Decision{ShouldDownload: false, MatchedRule: rule, Source: filter.SourceNone, Reason: "匹配规则但大小不符合规则约束"}
	}
	return filter.Decision{ShouldDownload: true, MatchedRule: rule, Source: filter.SourceFilterRule}
}

func decisionLabel(ok bool) string {
	if ok {
		return "downloaded"
	}
	return "skipped"
}

func bytesToGB(n int64) float64 {
	if n <= 0 {
		return 0
	}
	return float64(n) / 1024 / 1024 / 1024
}

// RSSLinkTestRequest 是 POST /api/rss/test：添加、编辑 RSS 时先试一下链接（只拉一次，不存）。
type RSSLinkTestRequest struct {
	URL string `json:"url"`
}

// RSSLinkTestResponse 是试的结果：RSS 的标题、条目数与最新的几条标题。
type RSSLinkTestResponse struct {
	Title  string   `json:"title"`
	Count  int      `json:"count"`
	Titles []string `json:"titles"`
}

// rssLinkTestTitles 是试链接时回给页面看的标题条数。
const rssLinkTestTitles = 5

func (s *Server) apiRSSLinkTest(w http.ResponseWriter, r *http.Request) {
	var req RSSLinkTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "请求格式不对: "+err.Error(), http.StatusBadRequest)
		return
	}
	feed, err := internal.FetchRSSFeed(r.Context(), req.URL)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, internal.ErrRSSPlaceholder) || errors.Is(err, internal.ErrRSSInvalidURL) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	titles := make([]string, 0, rssLinkTestTitles)
	for _, it := range feed.Items {
		if len(titles) == rssLinkTestTitles {
			break
		}
		if t := strings.TrimSpace(it.Title); t != "" {
			titles = append(titles, t)
		}
	}
	writeJSON(w, RSSLinkTestResponse{Title: strings.TrimSpace(feed.Title), Count: len(feed.Items), Titles: titles})
}
