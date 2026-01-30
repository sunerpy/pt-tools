// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// DownloaderDirectoryRequest 下载器目录请求结构
type DownloaderDirectoryRequest struct {
	Path      string `json:"path"`
	Alias     string `json:"alias"`
	IsDefault bool   `json:"is_default"`
}

// DownloaderDirectoryResponse 下载器目录响应结构
type DownloaderDirectoryResponse struct {
	ID           uint   `json:"id"`
	DownloaderID uint   `json:"downloader_id"`
	Path         string `json:"path"`
	Alias        string `json:"alias"`
	IsDefault    bool   `json:"is_default"`
}

// apiDownloaderRouter 路由分发器，处理下载器相关的所有路由
func (s *Server) apiDownloaderRouter(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/downloaders/")
	parts := strings.Split(path, "/")

	// 检查是否是目录相关的路由
	if len(parts) >= 2 && parts[1] == "directories" {
		if len(parts) == 2 {
			// /api/downloaders/:id/directories
			s.apiDownloaderDirectories(w, r)
		} else {
			// /api/downloaders/:id/directories/:dirId 或 /api/downloaders/:id/directories/:dirId/set-default
			s.apiDownloaderDirectoryDetail(w, r)
		}
		return
	}

	// 检查是否是应用到站点的路由
	if len(parts) == 2 && parts[1] == "apply-to-sites" {
		s.applyDownloaderToSites(w, r, parts[0])
		return
	}

	// 其他情况交给原有的下载器详情处理
	s.apiDownloaderDetail(w, r)
}

// apiDownloaderDirectories 处理下载器目录列表和创建
// GET /api/downloaders/:id/directories - 列出下载器的所有目录
// POST /api/downloaders/:id/directories - 创建新目录
func (s *Server) apiDownloaderDirectories(w http.ResponseWriter, r *http.Request) {
	// 解析下载器ID
	path := strings.TrimPrefix(r.URL.Path, "/api/downloaders/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "directories" {
		http.Error(w, "无效的路径", http.StatusBadRequest)
		return
	}

	downloaderID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "无效的下载器ID", http.StatusBadRequest)
		return
	}

	// 检查下载器是否存在
	db := global.GlobalDB.DB
	var downloader models.DownloaderSetting
	if err := db.First(&downloader, uint(downloaderID)).Error; err != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listDownloaderDirectories(w, r, uint(downloaderID))
	case http.MethodPost:
		s.createDownloaderDirectory(w, r, uint(downloaderID))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiDownloaderDirectoryDetail 处理单个下载器目录的操作
// PUT /api/downloaders/:id/directories/:dirId - 更新目录
// DELETE /api/downloaders/:id/directories/:dirId - 删除目录
// POST /api/downloaders/:id/directories/:dirId/set-default - 设置为默认目录
func (s *Server) apiDownloaderDirectoryDetail(w http.ResponseWriter, r *http.Request) {
	// 解析路径
	path := strings.TrimPrefix(r.URL.Path, "/api/downloaders/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "directories" {
		http.Error(w, "无效的路径", http.StatusBadRequest)
		return
	}

	downloaderID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "无效的下载器ID", http.StatusBadRequest)
		return
	}

	// 检查是否是设置默认路径的操作
	if len(parts) >= 4 && parts[3] == "set-default" {
		setDefaultDirID, parseErr := strconv.ParseUint(parts[2], 10, 64)
		if parseErr != nil {
			http.Error(w, "无效的目录ID", http.StatusBadRequest)
			return
		}
		s.setDefaultDirectory(w, r, uint(downloaderID), uint(setDefaultDirID))
		return
	}

	dirID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "无效的目录ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.updateDownloaderDirectory(w, r, uint(downloaderID), uint(dirID))
	case http.MethodDelete:
		s.deleteDownloaderDirectory(w, r, uint(downloaderID), uint(dirID))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// listDownloaderDirectories 列出下载器的所有目录
func (s *Server) listDownloaderDirectories(w http.ResponseWriter, _ *http.Request, downloaderID uint) {
	db := global.GlobalDB.DB
	var directories []models.DownloaderDirectory
	if err := db.Where("downloader_id = ?", downloaderID).Find(&directories).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responses := make([]DownloaderDirectoryResponse, len(directories))
	for i, dir := range directories {
		responses[i] = DownloaderDirectoryResponse{
			ID:           dir.ID,
			DownloaderID: dir.DownloaderID,
			Path:         dir.Path,
			Alias:        dir.Alias,
			IsDefault:    dir.IsDefault,
		}
	}

	writeJSON(w, responses)
}

// createDownloaderDirectory 创建新目录
func (s *Server) createDownloaderDirectory(w http.ResponseWriter, r *http.Request, downloaderID uint) {
	var req DownloaderDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证必填字段
	if req.Path == "" {
		http.Error(w, "目录路径不能为空", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB

	// 检查路径是否已存在
	var pathCount int64
	db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND path = ?", downloaderID, req.Path).Count(&pathCount)
	if pathCount > 0 {
		http.Error(w, "该路径已存在", http.StatusBadRequest)
		return
	}

	// 检查是否是第一个目录，如果是则自动设为默认
	var totalCount int64
	db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ?", downloaderID).Count(&totalCount)
	if totalCount == 0 {
		req.IsDefault = true
	}

	// 如果设置为默认，先清除其他默认
	if req.IsDefault {
		db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND is_default = ?", downloaderID, true).Update("is_default", false)
	}

	directory := models.DownloaderDirectory{
		DownloaderID: downloaderID,
		Path:         req.Path,
		Alias:        req.Alias,
		IsDefault:    req.IsDefault,
	}

	if err := db.Create(&directory).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[DownloaderDirectory] 创建目录: downloader_id=%d, path=%s, alias=%s", downloaderID, req.Path, req.Alias)

	writeJSON(w, DownloaderDirectoryResponse{
		ID:           directory.ID,
		DownloaderID: directory.DownloaderID,
		Path:         directory.Path,
		Alias:        directory.Alias,
		IsDefault:    directory.IsDefault,
	})
}

// updateDownloaderDirectory 更新目录
func (s *Server) updateDownloaderDirectory(w http.ResponseWriter, r *http.Request, downloaderID, dirID uint) {
	var req DownloaderDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB
	var directory models.DownloaderDirectory
	if err := db.Where("id = ? AND downloader_id = ?", dirID, downloaderID).First(&directory).Error; err != nil {
		http.Error(w, "目录不存在", http.StatusNotFound)
		return
	}

	// 如果要取消默认状态，检查是否是唯一的默认目录
	if directory.IsDefault && !req.IsDefault {
		var otherDefaultCount int64
		db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND is_default = ? AND id != ?", downloaderID, true, dirID).Count(&otherDefaultCount)
		if otherDefaultCount == 0 {
			http.Error(w, "必须保留至少一个默认目录，请先将其他目录设为默认", http.StatusBadRequest)
			return
		}
	}

	// 如果设置为默认，先清除其他默认
	if req.IsDefault && !directory.IsDefault {
		db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND is_default = ? AND id != ?", downloaderID, true, dirID).Update("is_default", false)
	}

	// 更新字段
	if req.Path != "" {
		directory.Path = req.Path
	}
	directory.Alias = req.Alias
	directory.IsDefault = req.IsDefault

	if err := db.Save(&directory).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[DownloaderDirectory] 更新目录: id=%d, path=%s, alias=%s", dirID, directory.Path, directory.Alias)

	writeJSON(w, DownloaderDirectoryResponse{
		ID:           directory.ID,
		DownloaderID: directory.DownloaderID,
		Path:         directory.Path,
		Alias:        directory.Alias,
		IsDefault:    directory.IsDefault,
	})
}

// deleteDownloaderDirectory 删除目录
func (s *Server) deleteDownloaderDirectory(w http.ResponseWriter, _ *http.Request, downloaderID, dirID uint) {
	db := global.GlobalDB.DB
	var directory models.DownloaderDirectory
	if err := db.Where("id = ? AND downloader_id = ?", dirID, downloaderID).First(&directory).Error; err != nil {
		http.Error(w, "目录不存在", http.StatusNotFound)
		return
	}

	// 如果删除的是默认目录，检查是否还有其他目录
	if directory.IsDefault {
		var count int64
		db.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND id != ?", downloaderID, dirID).Count(&count)
		if count > 0 {
			http.Error(w, "不能删除默认目录，请先将其他目录设为默认", http.StatusBadRequest)
			return
		}
	}

	if err := db.Delete(&directory).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[DownloaderDirectory] 删除目录: id=%d, path=%s", dirID, directory.Path)

	writeJSON(w, map[string]string{"status": "deleted"})
}

// setDefaultDirectory 设置默认目录
func (s *Server) setDefaultDirectory(w http.ResponseWriter, r *http.Request, downloaderID, dirID uint) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	db := global.GlobalDB.DB
	var directory models.DownloaderDirectory
	if err := db.Where("id = ? AND downloader_id = ?", dirID, downloaderID).First(&directory).Error; err != nil {
		http.Error(w, "目录不存在", http.StatusNotFound)
		return
	}

	// 使用事务确保原子性
	err := db.Transaction(func(tx *gorm.DB) error {
		// 清除所有其他默认
		if clearErr := tx.Model(&models.DownloaderDirectory{}).Where("downloader_id = ? AND is_default = ?", downloaderID, true).Update("is_default", false).Error; clearErr != nil {
			return clearErr
		}
		// 设置当前为默认
		if setErr := tx.Model(&directory).Update("is_default", true).Error; setErr != nil {
			return setErr
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[DownloaderDirectory] 设置默认目录: id=%d, path=%s", dirID, directory.Path)

	// 重新查询以获取最新状态
	db.First(&directory, dirID)

	writeJSON(w, DownloaderDirectoryResponse{
		ID:           directory.ID,
		DownloaderID: directory.DownloaderID,
		Path:         directory.Path,
		Alias:        directory.Alias,
		IsDefault:    directory.IsDefault,
	})
}

// getAllDownloaderDirectories 获取所有下载器及其目录（用于前端选择）
// GET /api/downloaders/all-directories
func (s *Server) apiAllDownloaderDirectories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	db := global.GlobalDB.DB

	// 获取所有启用的下载器
	var downloaders []models.DownloaderSetting
	if err := db.Where("enabled = ?", true).Find(&downloaders).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type DownloaderWithDirectories struct {
		ID          uint                          `json:"id"`
		Name        string                        `json:"name"`
		Type        string                        `json:"type"`
		IsDefault   bool                          `json:"is_default"`
		AutoStart   bool                          `json:"auto_start"`
		Directories []DownloaderDirectoryResponse `json:"directories"`
	}

	result := make([]DownloaderWithDirectories, 0, len(downloaders))
	for _, dl := range downloaders {
		var directories []models.DownloaderDirectory
		db.Where("downloader_id = ?", dl.ID).Find(&directories)

		dirResponses := make([]DownloaderDirectoryResponse, len(directories))
		for i, dir := range directories {
			dirResponses[i] = DownloaderDirectoryResponse{
				ID:           dir.ID,
				DownloaderID: dir.DownloaderID,
				Path:         dir.Path,
				Alias:        dir.Alias,
				IsDefault:    dir.IsDefault,
			}
		}

		result = append(result, DownloaderWithDirectories{
			ID:          dl.ID,
			Name:        dl.Name,
			Type:        dl.Type,
			IsDefault:   dl.IsDefault,
			AutoStart:   dl.AutoStart,
			Directories: dirResponses,
		})
	}

	writeJSON(w, result)
}
