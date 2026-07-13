package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/maintenance"
)

// maintenanceCleanRequest 是 POST /api/maintenance/clean 的请求体。
type maintenanceCleanRequest struct {
	// Categories 指定要清理的类别（logs/staging/backups）；为空表示全部三类。
	Categories []string `json:"categories"`
	// DryRun 为 true 时只预览将清理项，不实际删除。
	DryRun bool `json:"dryRun"`
	// KeepBackups 为 backups 类别保留的最近份数；<=0 使用默认值。
	KeepBackups int `json:"keepBackups"`
}

// cleanCategoryDTO 是单个类别的 JSON 友好清理结果。
type cleanCategoryDTO struct {
	Name         string `json:"name"`
	DeletedCount int    `json:"deletedCount"`
	FreedBytes   int64  `json:"freedBytes"`
	FreedHuman   string `json:"freedHuman"`
	SkippedCount int    `json:"skippedCount"`
	Note         string `json:"note,omitempty"`
}

// cleanResultDTO 是整次清理的 JSON 友好汇总。
type cleanResultDTO struct {
	DryRun          bool               `json:"dryRun"`
	Categories      []cleanCategoryDTO `json:"categories"`
	TotalDeleted    int                `json:"totalDeleted"`
	TotalFreedBytes int64              `json:"totalFreedBytes"`
	TotalFreedHuman string             `json:"totalFreedHuman"`
}

// apiMaintenanceClean 处理共享清理服务的 Web 入口（受 auth 保护的破坏性操作）。
//
//	GET  /api/maintenance/clean            → 预览（DryRun=true），返回将清理项
//	POST /api/maintenance/clean            → 按请求体执行；body.dryRun=true 时同样为预览
//
// 预览（GET 或 dryRun:true）绝不删除磁盘文件；只有 POST 且 dryRun:false 才真正删除，
// 且删除一律经底层 maintenance.Cleaner 的白名单 + 红线保护。
func (s *Server) apiMaintenanceClean(w http.ResponseWriter, r *http.Request) {
	var opts maintenance.CleanOptions
	switch r.Method {
	case http.MethodGet:
		opts.DryRun = true
	case http.MethodPost:
		var req maintenanceCleanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "无效的请求格式", http.StatusBadRequest)
			return
		}
		opts.DryRun = req.DryRun
		opts.KeepBackups = req.KeepBackups
		for _, c := range req.Categories {
			opts.Categories = append(opts.Categories, maintenance.CleanCategory(c))
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cleaner := s.newCleaner()
	res, err := cleaner.Clean(r.Context(), opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, toCleanResultDTO(res))
}

// newCleaner 依据服务端配置构造共享清理服务。homeDir 与运行时其它处（apiLogs）
// 一致，取自 os.UserHomeDir()，便于测试通过 t.Setenv("HOME", ...) 指向 fake home。
func (s *Server) newCleaner() *maintenance.Cleaner {
	homeDir, _ := os.UserHomeDir()
	return maintenance.NewCleaner(homeDir, global.GlobalDB, config.DefaultZapConfig)
}

// toCleanResultDTO 将 maintenance.CleanResult 映射为 JSON 友好 DTO 并汇总总量。
func toCleanResultDTO(res *maintenance.CleanResult) cleanResultDTO {
	dto := cleanResultDTO{DryRun: res.DryRun}
	for _, cr := range res.Categories {
		dto.Categories = append(dto.Categories, cleanCategoryDTO{
			Name:         string(cr.Category),
			DeletedCount: len(cr.Deleted),
			FreedBytes:   cr.FreedBytes,
			FreedHuman:   humanBytes(cr.FreedBytes),
			SkippedCount: len(cr.Skipped),
			Note:         cr.Note,
		})
		dto.TotalDeleted += len(cr.Deleted)
		dto.TotalFreedBytes += cr.FreedBytes
	}
	dto.TotalFreedHuman = humanBytes(dto.TotalFreedBytes)
	return dto
}

// humanBytes 将字节数格式化为人类可读字符串（B/KiB/MiB/GiB/TiB）。
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
