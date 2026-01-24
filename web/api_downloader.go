package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/transmission"
)

// DownloaderRequest 下载器请求结构
type DownloaderRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // qbittorrent, transmission
	URL         string `json:"url"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	IsDefault   bool   `json:"is_default"`
	Enabled     bool   `json:"enabled"`
	AutoStart   bool   `json:"auto_start"` // 推送种子后自动开始下载
	ExtraConfig string `json:"extra_config,omitempty"`
}

// DownloaderResponse 下载器响应结构
type DownloaderResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Username    string `json:"username"`
	IsDefault   bool   `json:"is_default"`
	Enabled     bool   `json:"enabled"`
	AutoStart   bool   `json:"auto_start"` // 推送种子后自动开始下载
	ExtraConfig string `json:"extra_config,omitempty"`
}

// HealthCheckResponse 健康检查响应
type HealthCheckResponse struct {
	Name      string `json:"name"`
	IsHealthy bool   `json:"is_healthy"`
	Message   string `json:"message,omitempty"`
}

// apiDownloaders 处理下载器列表和创建
// GET /api/downloaders - 列出所有下载器
// POST /api/downloaders - 创建新下载器
func (s *Server) apiDownloaders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listDownloaders(w, r)
	case http.MethodPost:
		s.createDownloader(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// apiDownloaderDetail 处理单个下载器的操作
// GET /api/downloaders/:id - 获取下载器详情
// PUT /api/downloaders/:id - 更新下载器
// DELETE /api/downloaders/:id - 删除下载器
// POST /api/downloaders/:id/set-default - 设置为默认下载器
func (s *Server) apiDownloaderDetail(w http.ResponseWriter, r *http.Request) {
	// 解析ID
	path := strings.TrimPrefix(r.URL.Path, "/api/downloaders/")
	// 检查是否是健康检查路径
	if strings.HasSuffix(path, "/health") {
		idStr := strings.TrimSuffix(path, "/health")
		s.downloaderHealthCheck(w, r, idStr)
		return
	}
	// 检查是否是设置默认路径
	if strings.HasSuffix(path, "/set-default") {
		idStr := strings.TrimSuffix(path, "/set-default")
		s.setDefaultDownloader(w, r, idStr)
		return
	}

	idStr := path
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的下载器ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getDownloader(w, r, uint(id))
	case http.MethodPut:
		s.updateDownloader(w, r, uint(id))
	case http.MethodDelete:
		s.deleteDownloader(w, r, uint(id))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// listDownloaders 列出所有下载器
func (s *Server) listDownloaders(w http.ResponseWriter, r *http.Request) {
	db := global.GlobalDB.DB
	var downloaders []models.DownloaderSetting
	if err := db.Find(&downloaders).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 转换为响应格式（隐藏密码）
	responses := make([]DownloaderResponse, len(downloaders))
	for i, dl := range downloaders {
		responses[i] = DownloaderResponse{
			ID:          dl.ID,
			Name:        dl.Name,
			Type:        dl.Type,
			URL:         dl.URL,
			Username:    dl.Username,
			IsDefault:   dl.IsDefault,
			Enabled:     dl.Enabled,
			AutoStart:   dl.AutoStart,
			ExtraConfig: dl.ExtraConfig,
		}
	}

	writeJSON(w, responses)
}

// createDownloader 创建新下载器
func (s *Server) createDownloader(w http.ResponseWriter, r *http.Request) {
	var req DownloaderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 验证必填字段
	if req.Name == "" {
		http.Error(w, "名称不能为空", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		http.Error(w, "类型不能为空", http.StatusBadRequest)
		return
	}
	if req.Type != "qbittorrent" && req.Type != "transmission" {
		http.Error(w, "不支持的下载器类型", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "URL不能为空", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB

	// 检查名称是否已存在
	var nameCount int64
	db.Model(&models.DownloaderSetting{}).Where("name = ?", req.Name).Count(&nameCount)
	if nameCount > 0 {
		http.Error(w, "下载器名称已存在", http.StatusBadRequest)
		return
	}

	// 检查是否是第一个下载器，如果是则自动设为默认
	var totalCount int64
	db.Model(&models.DownloaderSetting{}).Count(&totalCount)
	if totalCount == 0 {
		req.IsDefault = true
	}

	// 如果设置为默认，先清除其他默认（确保只有一个默认）
	if req.IsDefault {
		db.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Update("is_default", false)
	}

	downloader := models.DownloaderSetting{
		Name:        req.Name,
		Type:        req.Type,
		URL:         req.URL,
		Username:    req.Username,
		Password:    req.Password,
		IsDefault:   req.IsDefault,
		Enabled:     req.Enabled,
		AutoStart:   req.AutoStart,
		ExtraConfig: req.ExtraConfig,
	}

	if err := db.Create(&downloader).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Downloader] 创建下载器: name=%s, type=%s, is_default=%v", req.Name, req.Type, req.IsDefault)

	writeJSON(w, DownloaderResponse{
		ID:          downloader.ID,
		Name:        downloader.Name,
		Type:        downloader.Type,
		URL:         downloader.URL,
		Username:    downloader.Username,
		IsDefault:   downloader.IsDefault,
		Enabled:     downloader.Enabled,
		AutoStart:   downloader.AutoStart,
		ExtraConfig: downloader.ExtraConfig,
	})
}

// getDownloader 获取下载器详情
func (s *Server) getDownloader(w http.ResponseWriter, r *http.Request, id uint) {
	db := global.GlobalDB.DB
	var downloader models.DownloaderSetting
	if err := db.First(&downloader, id).Error; err != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	writeJSON(w, DownloaderResponse{
		ID:          downloader.ID,
		Name:        downloader.Name,
		Type:        downloader.Type,
		URL:         downloader.URL,
		Username:    downloader.Username,
		IsDefault:   downloader.IsDefault,
		Enabled:     downloader.Enabled,
		AutoStart:   downloader.AutoStart,
		ExtraConfig: downloader.ExtraConfig,
	})
}

// updateDownloader 更新下载器
func (s *Server) updateDownloader(w http.ResponseWriter, r *http.Request, id uint) {
	var req DownloaderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB
	var downloader models.DownloaderSetting
	if err := db.First(&downloader, id).Error; err != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	// 如果要取消默认状态，检查是否是唯一的默认下载器
	if downloader.IsDefault && !req.IsDefault {
		var otherDefaultCount int64
		db.Model(&models.DownloaderSetting{}).Where("is_default = ? AND id != ?", true, id).Count(&otherDefaultCount)
		if otherDefaultCount == 0 {
			http.Error(w, "必须保留至少一个默认下载器，请先将其他下载器设为默认", http.StatusBadRequest)
			return
		}
	}

	// 如果设置为默认，先清除其他默认
	if req.IsDefault && !downloader.IsDefault {
		db.Model(&models.DownloaderSetting{}).Where("is_default = ? AND id != ?", true, id).Update("is_default", false)
	}

	// 更新字段
	if req.Name != "" {
		downloader.Name = req.Name
	}
	if req.Type != "" {
		downloader.Type = req.Type
	}
	if req.URL != "" {
		downloader.URL = req.URL
	}
	downloader.Username = req.Username
	if req.Password != "" {
		downloader.Password = req.Password
	}
	downloader.IsDefault = req.IsDefault
	downloader.Enabled = req.Enabled
	downloader.AutoStart = req.AutoStart
	downloader.ExtraConfig = req.ExtraConfig

	if err := db.Save(&downloader).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Downloader] 更新下载器: id=%d, name=%s", id, downloader.Name)

	writeJSON(w, DownloaderResponse{
		ID:          downloader.ID,
		Name:        downloader.Name,
		Type:        downloader.Type,
		URL:         downloader.URL,
		Username:    downloader.Username,
		IsDefault:   downloader.IsDefault,
		Enabled:     downloader.Enabled,
		AutoStart:   downloader.AutoStart,
		ExtraConfig: downloader.ExtraConfig,
	})
}

// deleteDownloader 删除下载器
func (s *Server) deleteDownloader(w http.ResponseWriter, r *http.Request, id uint) {
	db := global.GlobalDB.DB
	var downloader models.DownloaderSetting
	if err := db.First(&downloader, id).Error; err != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	// 如果删除的是默认下载器，检查是否还有其他下载器
	if downloader.IsDefault {
		var count int64
		db.Model(&models.DownloaderSetting{}).Where("id != ?", id).Count(&count)
		if count > 0 {
			// 还有其他下载器，提示用户需要先设置新的默认下载器
			http.Error(w, "不能删除默认下载器，请先将其他下载器设为默认", http.StatusBadRequest)
			return
		}
	}

	if err := db.Delete(&downloader).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Downloader] 删除下载器: id=%d, name=%s", id, downloader.Name)

	writeJSON(w, map[string]string{"status": "deleted"})
}

// downloaderHealthCheck 下载器健康检查（真正测试连接）
func (s *Server) downloaderHealthCheck(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的下载器ID", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB
	var dlSetting models.DownloaderSetting
	if err := db.First(&dlSetting, uint(id)).Error; err != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	response := HealthCheckResponse{
		Name:      dlSetting.Name,
		IsHealthy: false,
	}

	if !dlSetting.Enabled {
		response.Message = "下载器未启用"
		writeJSON(w, response)
		return
	}

	switch dlSetting.Type {
	case "qbittorrent":
		config := qbit.NewQBitConfig(dlSetting.URL, dlSetting.Username, dlSetting.Password)
		dl, err := qbit.NewQbitClient(config, dlSetting.Name)
		if err != nil {
			response.Message = err.Error()
			writeJSON(w, response)
			return
		}
		defer dl.Close()
		response.IsHealthy = true
		response.Message = "连接正常"

	case "transmission":
		config := transmission.NewTransmissionConfigWithAutoStart(dlSetting.URL, dlSetting.Username, dlSetting.Password, dlSetting.AutoStart)
		dl, err := transmission.NewTransmissionClient(config, dlSetting.Name)
		if err != nil {
			response.Message = err.Error()
			writeJSON(w, response)
			return
		}
		defer dl.Close()
		response.IsHealthy = true
		response.Message = "连接正常"

	default:
		response.Message = "不支持的下载器类型: " + dlSetting.Type
	}

	writeJSON(w, response)
}

// setDefaultDownloader 设置默认下载器
func (s *Server) setDefaultDownloader(w http.ResponseWriter, r *http.Request, idStr string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "无效的下载器ID", http.StatusBadRequest)
		return
	}

	db := global.GlobalDB.DB
	var downloader models.DownloaderSetting
	if findErr := db.First(&downloader, uint(id)).Error; findErr != nil {
		http.Error(w, "下载器不存在", http.StatusNotFound)
		return
	}

	// 使用事务确保原子性
	err = db.Transaction(func(tx *gorm.DB) error {
		// 清除所有其他默认
		if clearErr := tx.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Update("is_default", false).Error; clearErr != nil {
			return clearErr
		}
		// 设置当前为默认
		if setErr := tx.Model(&downloader).Update("is_default", true).Error; setErr != nil {
			return setErr
		}
		return nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	global.GetSlogger().Infof("[Downloader] 设置默认下载器: id=%d, name=%s", id, downloader.Name)

	// 重新查询以获取最新状态
	db.First(&downloader, uint(id))

	writeJSON(w, DownloaderResponse{
		ID:          downloader.ID,
		Name:        downloader.Name,
		Type:        downloader.Type,
		URL:         downloader.URL,
		Username:    downloader.Username,
		IsDefault:   downloader.IsDefault,
		Enabled:     downloader.Enabled,
		AutoStart:   downloader.AutoStart,
		ExtraConfig: downloader.ExtraConfig,
	})
}
