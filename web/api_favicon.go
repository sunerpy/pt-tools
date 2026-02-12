// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sunerpy/requests"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// FaviconService 管理站点图标缓存（数据库存储）
type FaviconService struct {
	// 缓存刷新间隔（默认 12 小时）
	refreshInterval time.Duration
	// 后台刷新任务停止信号
	stopCh chan struct{}
}

// faviconService 全局图标服务实例
var faviconService *FaviconService

// initFaviconService 初始化图标服务
func initFaviconService() {
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
		stopCh:          make(chan struct{}),
	}
	// 启动后台刷新任务
	go faviconService.backgroundRefresh()
}

// backgroundRefresh 后台定时刷新所有图标
func (fs *FaviconService) backgroundRefresh() {
	// 启动时先刷新一次过期的图标
	fs.refreshExpiredFavicons()

	ticker := time.NewTicker(fs.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fs.refreshExpiredFavicons()
		case <-fs.stopCh:
			return
		}
	}
}

// refreshExpiredFavicons 刷新所有过期的图标
func (fs *FaviconService) refreshExpiredFavicons() {
	if global.GlobalDB == nil {
		return
	}

	db := global.GlobalDB.DB
	expiredTime := time.Now().Add(-fs.refreshInterval)

	// 查找所有过期的缓存
	var expiredCaches []models.FaviconCache
	if err := db.Where("last_fetched < ? OR last_fetched IS NULL", expiredTime).Find(&expiredCaches).Error; err != nil {
		global.GetSlogger().Warnf("[Favicon] 查询过期缓存失败: %v", err)
		return
	}

	// 获取所有注册的站点定义
	definitions := v2.GetDefinitionRegistry().GetAll()
	defMap := make(map[string]*v2.SiteDefinition)
	for _, def := range definitions {
		defMap[strings.ToLower(def.ID)] = def
	}

	// 刷新过期的缓存
	for _, cache := range expiredCaches {
		def, ok := defMap[strings.ToLower(cache.SiteID)]
		if !ok {
			continue
		}

		faviconURL := def.FaviconURL
		if faviconURL == "" && len(def.URLs) > 0 {
			faviconURL = strings.TrimSuffix(def.URLs[0], "/") + "/favicon.ico"
		}
		if faviconURL == "" {
			continue
		}

		if err := fs.fetchAndSave(cache.SiteID, def.Name, faviconURL); err != nil {
			global.GetSlogger().Warnf("[Favicon] 刷新图标失败: site=%s, err=%v", cache.SiteID, err)
		} else {
			global.GetSlogger().Infof("[Favicon] 刷新图标成功: site=%s", cache.SiteID)
		}

		// 避免请求过于频繁
		time.Sleep(2 * time.Second)
	}

	// 检查是否有新站点需要添加缓存
	for _, def := range definitions {
		var count int64
		db.Model(&models.FaviconCache{}).Where("site_id = ?", def.ID).Count(&count)
		if count == 0 {
			faviconURL := def.FaviconURL
			if faviconURL == "" && len(def.URLs) > 0 {
				faviconURL = strings.TrimSuffix(def.URLs[0], "/") + "/favicon.ico"
			}
			if faviconURL == "" {
				continue
			}

			if err := fs.fetchAndSave(def.ID, def.Name, faviconURL); err != nil {
				global.GetSlogger().Warnf("[Favicon] 新站点图标获取失败: site=%s, err=%v", def.ID, err)
			} else {
				global.GetSlogger().Infof("[Favicon] 新站点图标已缓存: site=%s", def.ID)
			}

			time.Sleep(2 * time.Second)
		}
	}
}

// fetchAndSave 下载并保存图标到数据库
func (fs *FaviconService) fetchAndSave(siteID, siteName, faviconURL string) error {
	if global.GlobalDB == nil {
		return fmt.Errorf("数据库未初始化")
	}

	// 下载图标
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := requests.NewSession().WithTimeout(30 * time.Second)
	if proxyURL := httpclient.ResolveProxyFromEnvironment(faviconURL); proxyURL != "" {
		session = session.WithProxy(proxyURL)
	}
	defer func() { _ = session.Close() }()

	req, err := requests.NewGet(faviconURL).Build()
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.AddHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := session.DoWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("下载图标失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载图标失败: HTTP %d", resp.StatusCode)
	}

	// 限制读取大小（最大 1MB）
	limitedReader := io.LimitReader(bytes.NewReader(resp.Bytes()), 1024*1024)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("读取图标数据失败: %w", err)
	}

	if len(data) == 0 {
		return fmt.Errorf("图标数据为空")
	}

	// 检测 Content-Type
	contentType := http.DetectContentType(data)
	if strings.HasPrefix(contentType, "text/") {
		contentType = "image/x-icon"
	}

	// 计算 ETag
	hashSum := sha256.Sum256(data)
	etag := hex.EncodeToString(hashSum[:8])

	// 保存到数据库
	db := global.GlobalDB.DB
	cache := models.FaviconCache{
		SiteID:      strings.ToLower(siteID),
		SiteName:    siteName,
		FaviconURL:  faviconURL,
		Data:        data,
		ContentType: contentType,
		ETag:        etag,
		LastFetched: time.Now(),
	}

	// 使用 upsert 逻辑
	var existing models.FaviconCache
	if err := db.Where("site_id = ?", cache.SiteID).First(&existing).Error; err == nil {
		// 更新现有记录
		cache.ID = existing.ID
		cache.CreatedAt = existing.CreatedAt
	}

	if err := db.Save(&cache).Error; err != nil {
		return fmt.Errorf("保存到数据库失败: %w", err)
	}

	return nil
}

// GetFavicon 从数据库获取图标
func (fs *FaviconService) GetFavicon(siteID string) (*models.FaviconCache, error) {
	if global.GlobalDB == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	db := global.GlobalDB.DB
	var cache models.FaviconCache
	if err := db.Where("site_id = ?", strings.ToLower(siteID)).First(&cache).Error; err != nil {
		return nil, err
	}

	return &cache, nil
}

// SiteFaviconInfo 站点图标信息
type SiteFaviconInfo struct {
	SiteID      string `json:"site_id"`
	SiteName    string `json:"site_name"`
	FaviconURL  string `json:"favicon_url,omitempty"`
	CacheURL    string `json:"cache_url,omitempty"`
	HasCache    bool   `json:"has_cache"`
	LastFetched int64  `json:"last_fetched,omitempty"`
}

// apiFavicon 处理站点图标请求
// GET /api/favicon/:siteID - 获取站点图标（从数据库缓存）
// POST /api/favicon/:siteID/refresh - 强制刷新缓存
func (s *Server) apiFavicon(w http.ResponseWriter, r *http.Request) {
	// 解析路径
	path := strings.TrimPrefix(r.URL.Path, "/api/favicon/")
	path = strings.TrimSuffix(path, "/")

	// 检查是否是 refresh 请求
	if strings.HasSuffix(path, "/refresh") {
		s.apiFaviconRefresh(w, r)
		return
	}

	// 只允许 GET 方法获取图标
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	siteID := path
	if siteID == "" {
		http.Error(w, "站点ID不能为空", http.StatusBadRequest)
		return
	}

	// 初始化服务（如果尚未初始化）
	if faviconService == nil {
		initFaviconService()
	}

	// 从数据库获取缓存
	cache, err := faviconService.GetFavicon(siteID)
	if err != nil || cache == nil || len(cache.Data) == 0 {
		// 缓存不存在，尝试获取
		def := v2.GetDefinitionRegistry().GetOrDefault(strings.ToLower(siteID))
		if def == nil {
			http.Error(w, "站点不存在", http.StatusNotFound)
			return
		}

		faviconURL := def.FaviconURL
		if faviconURL == "" && len(def.URLs) > 0 {
			faviconURL = strings.TrimSuffix(def.URLs[0], "/") + "/favicon.ico"
		}
		if faviconURL == "" {
			http.Error(w, "站点未配置图标", http.StatusNotFound)
			return
		}

		// 同步获取并缓存
		if fetchErr := faviconService.fetchAndSave(siteID, def.Name, faviconURL); fetchErr != nil {
			global.GetSlogger().Warnf("[Favicon] 获取图标失败: site=%s, err=%v", siteID, fetchErr)
			http.Error(w, "获取图标失败", http.StatusNotFound)
			return
		}

		// 重新获取
		cache, err = faviconService.GetFavicon(siteID)
		if err != nil || cache == nil {
			http.Error(w, "获取图标失败", http.StatusNotFound)
			return
		}
	}

	s.serveFaviconData(w, r, cache)
}

// serveFaviconData 提供数据库中的图标数据
func (s *Server) serveFaviconData(w http.ResponseWriter, r *http.Request, cache *models.FaviconCache) {
	etag := fmt.Sprintf(`"%s"`, cache.ETag)

	// 设置缓存头
	w.Header().Set("Content-Type", cache.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // 浏览器缓存 1 天
	w.Header().Set("ETag", etag)

	// 检查 If-None-Match
	if match := r.Header.Get("If-None-Match"); match != "" {
		if match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cache.Data)
}

// apiFaviconList 获取所有站点的图标信息
// GET /api/favicons - 获取所有站点图标信息列表
func (s *Server) apiFaviconList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 初始化服务（如果尚未初始化）
	if faviconService == nil {
		initFaviconService()
	}

	if global.GlobalDB == nil {
		http.Error(w, "数据库未初始化", http.StatusInternalServerError)
		return
	}

	// 获取所有注册的站点定义
	definitions := v2.GetDefinitionRegistry().GetAll()

	// 获取数据库中的缓存信息
	db := global.GlobalDB.DB
	var caches []models.FaviconCache
	db.Find(&caches)

	cacheMap := make(map[string]*models.FaviconCache)
	for i := range caches {
		cacheMap[strings.ToLower(caches[i].SiteID)] = &caches[i]
	}

	result := make([]SiteFaviconInfo, 0, len(definitions))

	for _, def := range definitions {
		info := SiteFaviconInfo{
			SiteID:     def.ID,
			SiteName:   def.Name,
			FaviconURL: def.FaviconURL,
			CacheURL:   fmt.Sprintf("/api/favicon/%s", def.ID),
		}

		if cache, ok := cacheMap[strings.ToLower(def.ID)]; ok && len(cache.Data) > 0 {
			info.HasCache = true
			info.LastFetched = cache.LastFetched.Unix()
		}

		result = append(result, info)
	}

	writeJSON(w, result)
}

// apiFaviconRefresh 强制刷新站点图标缓存
// POST /api/favicon/:siteID/refresh - 强制刷新缓存
func (s *Server) apiFaviconRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析 siteID
	path := strings.TrimPrefix(r.URL.Path, "/api/favicon/")
	path = strings.TrimSuffix(path, "/refresh")
	siteID := path
	if siteID == "" {
		http.Error(w, "站点ID不能为空", http.StatusBadRequest)
		return
	}

	// 初始化服务（如果尚未初始化）
	if faviconService == nil {
		initFaviconService()
	}

	// 获取站点定义
	def := v2.GetDefinitionRegistry().GetOrDefault(strings.ToLower(siteID))
	if def == nil {
		http.Error(w, "站点不存在", http.StatusNotFound)
		return
	}

	// 确定 favicon URL
	faviconURL := def.FaviconURL
	if faviconURL == "" && len(def.URLs) > 0 {
		faviconURL = strings.TrimSuffix(def.URLs[0], "/") + "/favicon.ico"
	}

	if faviconURL == "" {
		http.Error(w, "站点未配置图标URL", http.StatusBadRequest)
		return
	}

	// 重新下载并保存
	if err := faviconService.fetchAndSave(siteID, def.Name, faviconURL); err != nil {
		global.GetSlogger().Warnf("[Favicon] 刷新图标失败: site=%s, err=%v", siteID, err)
		http.Error(w, fmt.Sprintf("刷新图标失败: %v", err), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Favicon] 刷新图标成功: site=%s", siteID)
	writeJSON(w, map[string]any{
		"status":    "ok",
		"site_id":   siteID,
		"has_cache": true,
		"cache_url": fmt.Sprintf("/api/favicon/%s", siteID),
	})
}
