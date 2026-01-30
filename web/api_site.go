package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteValidationRequest 站点验证请求
type SiteValidationRequest struct {
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	AuthMethod string `json:"auth_method"` // cookie, api_key
	Cookie     string `json:"cookie,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	APIURL     string `json:"api_url,omitempty"`
}

// SiteValidationResponse 站点验证响应
type SiteValidationResponse struct {
	Valid        bool     `json:"valid"`
	Message      string   `json:"message"`
	FreeTorrents []string `json:"free_torrents,omitempty"` // 免费种子预览
}

// DynamicSiteRequest 动态站点请求
type DynamicSiteRequest struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	BaseURL      string `json:"base_url"`
	AuthMethod   string `json:"auth_method"`
	Cookie       string `json:"cookie,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	APIURL       string `json:"api_url,omitempty"`
	DownloaderID *uint  `json:"downloader_id,omitempty"`
	ParserConfig string `json:"parser_config,omitempty"`
}

// DynamicSiteResponse 动态站点响应
type DynamicSiteResponse struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	DisplayName       string `json:"display_name"`
	BaseURL           string `json:"base_url"`
	Enabled           bool   `json:"enabled"`
	AuthMethod        string `json:"auth_method"`
	DownloaderID      *uint  `json:"downloader_id,omitempty"`
	IsBuiltin         bool   `json:"is_builtin"`
	Unavailable       bool   `json:"unavailable,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// TemplateResponse 模板响应
type TemplateResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	BaseURL     string `json:"base_url"`
	AuthMethod  string `json:"auth_method"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Author      string `json:"author,omitempty"`
}

// TemplateImportRequest 模板导入请求
type TemplateImportRequest struct {
	Template json.RawMessage `json:"template"`
	Cookie   string          `json:"cookie,omitempty"`
	APIKey   string          `json:"api_key,omitempty"`
}

// apiSiteValidate 验证站点配置
// POST /api/sites/validate
func (s *Server) apiSiteValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req SiteValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证必填字段
	if req.Name == "" {
		writeJSON(w, SiteValidationResponse{Valid: false, Message: "站点名称不能为空"})
		return
	}
	if req.AuthMethod == "" {
		writeJSON(w, SiteValidationResponse{Valid: false, Message: "认证方式不能为空"})
		return
	}

	// 根据认证方式验证凭据
	switch req.AuthMethod {
	case "cookie":
		if req.Cookie == "" {
			writeJSON(w, SiteValidationResponse{Valid: false, Message: "Cookie不能为空"})
			return
		}
	case "api_key":
		if req.APIKey == "" {
			writeJSON(w, SiteValidationResponse{Valid: false, Message: "API Key不能为空"})
			return
		}
	default:
		writeJSON(w, SiteValidationResponse{Valid: false, Message: "不支持的认证方式"})
		return
	}

	// TODO: 实际验证逻辑 - 尝试连接站点并获取免费种子
	// 这里返回模拟结果
	response := SiteValidationResponse{
		Valid:        true,
		Message:      "站点配置验证成功",
		FreeTorrents: []string{}, // 实际实现时填充免费种子列表
	}

	global.GetSlogger().Infof("[Site] 验证站点配置: name=%s, auth_method=%s", req.Name, req.AuthMethod)

	writeJSON(w, response)
}

// apiDynamicSites 处理动态站点
// GET /api/sites/dynamic - 列出动态站点
// POST /api/sites/dynamic - 创建动态站点
func (s *Server) apiDynamicSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listDynamicSites(w, r)
	case http.MethodPost:
		s.createDynamicSite(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// listDynamicSites 列出动态站点
func (s *Server) listDynamicSites(w http.ResponseWriter, _ *http.Request) {
	db := global.GlobalDB.DB
	var sites []models.SiteSetting
	if err := db.Find(&sites).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defRegistry := v2.GetDefinitionRegistry()
	responses := make([]DynamicSiteResponse, len(sites))
	var sitesToDisable []models.SiteGroup
	for i, site := range sites {
		resp := DynamicSiteResponse{
			ID:           site.ID,
			Name:         site.Name,
			DisplayName:  site.DisplayName,
			BaseURL:      site.BaseURL,
			Enabled:      site.Enabled,
			AuthMethod:   site.AuthMethod,
			DownloaderID: site.DownloaderID,
			IsBuiltin:    site.IsBuiltin,
		}
		if def, ok := defRegistry.Get(site.Name); ok {
			resp.Unavailable = def.Unavailable
			resp.UnavailableReason = def.UnavailableReason
			if def.Unavailable {
				resp.Enabled = false
				if site.Enabled {
					sitesToDisable = append(sitesToDisable, models.SiteGroup(site.Name))
				}
			}
		}
		responses[i] = resp
	}
	if len(sitesToDisable) > 0 {
		go s.disableUnavailableSites(sitesToDisable)
	}

	writeJSON(w, responses)
}

func (s *Server) createDynamicSite(w http.ResponseWriter, r *http.Request) {
	var req DynamicSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "站点名称不能为空", http.StatusBadRequest)
		return
	}
	if req.AuthMethod == "" {
		http.Error(w, "认证方式不能为空", http.StatusBadRequest)
		return
	}

	repo := models.NewSiteRepository(global.GlobalDB.DB)
	siteID, err := repo.CreateSite(models.SiteData{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		BaseURL:      req.BaseURL,
		Enabled:      true,
		AuthMethod:   req.AuthMethod,
		Cookie:       req.Cookie,
		APIKey:       req.APIKey,
		APIURL:       req.APIURL,
		DownloaderID: req.DownloaderID,
		ParserConfig: req.ParserConfig,
		IsBuiltin:    false,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	site, _ := repo.GetSiteByID(siteID)
	global.GetSlogger().Infof("[Site] 创建动态站点: name=%s, auth_method=%s", req.Name, req.AuthMethod)

	writeJSON(w, DynamicSiteResponse{
		ID:           site.ID,
		Name:         site.Name,
		DisplayName:  site.DisplayName,
		BaseURL:      site.BaseURL,
		Enabled:      site.Enabled,
		AuthMethod:   site.AuthMethod,
		DownloaderID: site.DownloaderID,
		IsBuiltin:    site.IsBuiltin,
	})
}

// apiSiteTemplates 处理站点模板
// GET /api/sites/templates - 列出模板
func (s *Server) apiSiteTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	db := global.GlobalDB.DB
	var templates []models.SiteTemplate
	if err := db.Find(&templates).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responses := make([]TemplateResponse, len(templates))
	for i, tpl := range templates {
		responses[i] = TemplateResponse{
			ID:          tpl.ID,
			Name:        tpl.Name,
			DisplayName: tpl.DisplayName,
			BaseURL:     tpl.BaseURL,
			AuthMethod:  tpl.AuthMethod,
			Description: tpl.Description,
			Version:     tpl.Version,
			Author:      tpl.Author,
		}
	}

	writeJSON(w, responses)
}

// apiSiteTemplateImport 导入站点模板
// POST /api/sites/templates/import
func (s *Server) apiSiteTemplateImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req TemplateImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 解析模板
	var templateExport models.SiteTemplateExport
	if err := json.Unmarshal(req.Template, &templateExport); err != nil {
		http.Error(w, "无效的模板格式", http.StatusBadRequest)
		return
	}

	// 验证模板
	if templateExport.Name == "" {
		http.Error(w, "模板名称不能为空", http.StatusBadRequest)
		return
	}
	if templateExport.AuthMethod == "" {
		http.Error(w, "认证方式不能为空", http.StatusBadRequest)
		return
	}

	// 验证凭据
	switch templateExport.AuthMethod {
	case "cookie":
		if req.Cookie == "" {
			http.Error(w, "Cookie不能为空", http.StatusBadRequest)
			return
		}
	case "api_key":
		if req.APIKey == "" {
			http.Error(w, "API Key不能为空", http.StatusBadRequest)
			return
		}
	}

	db := global.GlobalDB.DB

	template := models.SiteTemplate{}
	_ = template.FromExport(&templateExport)
	if err := db.Create(&template).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			db.Where("name = ?", template.Name).Updates(&template)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	repo := models.NewSiteRepository(db)
	siteID, err := repo.CreateSite(models.SiteData{
		Name:         templateExport.Name,
		DisplayName:  templateExport.DisplayName,
		BaseURL:      templateExport.BaseURL,
		Enabled:      true,
		AuthMethod:   templateExport.AuthMethod,
		Cookie:       req.Cookie,
		APIKey:       req.APIKey,
		ParserConfig: string(templateExport.ParserConfig),
		IsBuiltin:    false,
		TemplateID:   &template.ID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	site, _ := repo.GetSiteByID(siteID)
	global.GetSlogger().Infof("[Site] 导入模板: name=%s", templateExport.Name)

	writeJSON(w, DynamicSiteResponse{
		ID:          site.ID,
		Name:        site.Name,
		DisplayName: site.DisplayName,
		BaseURL:     site.BaseURL,
		Enabled:     site.Enabled,
		AuthMethod:  site.AuthMethod,
		IsBuiltin:   site.IsBuiltin,
	})
}

// apiSiteTemplateExport 导出站点模板
// GET /api/sites/templates/:id/export
func (s *Server) apiSiteTemplateExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 解析ID
	path := strings.TrimPrefix(r.URL.Path, "/api/sites/templates/")
	path = strings.TrimSuffix(path, "/export")
	id, err := strconv.ParseUint(path, 10, 64)
	if err != nil {
		http.Error(w, "无效的模板ID", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB
	var template models.SiteTemplate
	if err := db.First(&template, uint(id)).Error; err != nil {
		http.Error(w, "模板不存在", http.StatusNotFound)
		return
	}

	// 导出模板（不包含敏感信息）
	export := template.ToExport()

	global.GetSlogger().Infof("[Site] 导出模板: id=%d, name=%s", id, template.Name)

	writeJSON(w, export)
}

// ============================================================================
// Free Torrent Batch Download API
// ============================================================================

// FreeTorrentBatchRequest represents a batch download request
type FreeTorrentBatchRequest struct {
	ArchiveType string `json:"archiveType"` // "tar.gz" or "zip"
}

// FreeTorrentBatchResponse represents a batch download response
type FreeTorrentBatchResponse struct {
	ArchivePath  string                `json:"archivePath"`
	ArchiveType  string                `json:"archiveType"`
	TorrentCount int                   `json:"torrentCount"`
	TotalSize    int64                 `json:"totalSize"`
	Manifest     []TorrentManifestItem `json:"manifest"`
}

// TorrentManifestItem represents a torrent in the manifest
type TorrentManifestItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	SizeBytes     int64  `json:"sizeBytes"`
	DiscountLevel string `json:"discountLevel"`
	DownloadURL   string `json:"downloadUrl"`
	Category      string `json:"category,omitempty"`
	Seeders       int    `json:"seeders,omitempty"`
	Leechers      int    `json:"leechers,omitempty"`
}

// apiSiteFreeTorrentsDownload handles GET /api/site/{site_id}/free-torrents/download
// Downloads all free torrents from a site and packages them into an archive
func (s *Server) apiSiteFreeTorrentsDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract site ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/site/")
	path = strings.TrimSuffix(path, "/free-torrents/download")
	siteID := path

	if siteID == "" {
		http.Error(w, "Site ID required", http.StatusBadRequest)
		return
	}

	// Get archive type from query parameter or request body
	archiveType := r.URL.Query().Get("type")
	if archiveType == "" {
		archiveType = "tar.gz"
	}

	// Validate archive type
	if archiveType != "tar.gz" && archiveType != "zip" {
		http.Error(w, "Invalid archive type. Use 'tar.gz' or 'zip'", http.StatusBadRequest)
		return
	}

	global.GetSlogger().Infof("[Site] Batch download free torrents: site=%s, type=%s", siteID, archiveType)

	// TODO: Implement actual batch download using BatchDownloadService
	// This requires getting the site instance from the registry
	// For now, return a placeholder response

	response := FreeTorrentBatchResponse{
		ArchivePath:  "",
		ArchiveType:  archiveType,
		TorrentCount: 0,
		TotalSize:    0,
		Manifest:     []TorrentManifestItem{},
	}

	// Return JSON response with download info
	// In actual implementation, this would return the file for download
	writeJSON(w, response)
}

// apiSiteFreeTorrentsList handles GET /api/site/{site_id}/free-torrents
// Lists all free torrents from a site without downloading
func (s *Server) apiSiteFreeTorrentsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Extract site ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/site/")
	path = strings.TrimSuffix(path, "/free-torrents")
	siteID := path

	if siteID == "" {
		http.Error(w, "Site ID required", http.StatusBadRequest)
		return
	}

	global.GetSlogger().Infof("[Site] List free torrents: site=%s", siteID)

	// TODO: Implement actual free torrent listing
	// This requires getting the site instance from the registry

	// Return empty list for now
	writeJSON(w, []TorrentManifestItem{})
}
