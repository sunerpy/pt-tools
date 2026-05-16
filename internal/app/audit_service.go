package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

const (
	defaultRowCap        int64 = 500_000
	defaultRetentionDays       = 90 * 24 * time.Hour
	redactedPlaceholder        = "[REDACTED]"
)

var sensitiveKeys = []string{"token", "passkey", "cookie", "password", "secret", "apikey"}

type AuditEntry struct {
	NotificationConfID uint
	ChannelType        string
	ChannelUserID      string
	Command            string
	Args               map[string]any
	Result             string
	LatencyMs          int64
}

type AuditQuery struct {
	Since         time.Time
	Until         time.Time
	ChannelUserID string
	Command       string
	Result        string
	Page          int
	PageSize      int
}

type AuditDTO struct {
	ID            uint      `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	ChannelType   string    `json:"channel_type"`
	ChannelUserID string    `json:"channel_user_id"`
	Command       string    `json:"command"`
	ArgsJSON      string    `json:"args_json"`
	Result        string    `json:"result"`
	LatencyMs     int64     `json:"latency_ms"`
}

// AuditStatsDTO 聚合指标，用于 ChatOps 审计页顶部的 stat chip。
// SuccessRate 单位为百分比 (0..100)，已四舍五入到 2 位小数。
type AuditStatsDTO struct {
	TodayCount   int64   `json:"today_count"`
	TotalCount   int64   `json:"total_count"`
	SuccessRate  float64 `json:"success_rate"`
	MaxLatencyMs int64   `json:"max_latency_ms"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

type AuditService interface {
	Record(ctx context.Context, e AuditEntry) error
	Query(ctx context.Context, q AuditQuery) (items []AuditDTO, total int, err error)
	Stats(ctx context.Context) (AuditStatsDTO, error)
	Prune(ctx context.Context) (deleted int64, err error)
}

type auditService struct {
	db        *gorm.DB
	rowCap    int64
	retention time.Duration
}

func NewAuditService(db *gorm.DB) AuditService {
	return &auditService{
		db:        db,
		rowCap:    defaultRowCap,
		retention: defaultRetentionDays,
	}
}

// NewAuditServiceWithCap 暴露 rowCap / retention 供测试注入小阈值；生产请用 NewAuditService。
func NewAuditServiceWithCap(db *gorm.DB, rowCap int64, retention time.Duration) AuditService {
	if rowCap <= 0 {
		rowCap = defaultRowCap
	}
	if retention <= 0 {
		retention = defaultRetentionDays
	}
	return &auditService{
		db:        db,
		rowCap:    rowCap,
		retention: retention,
	}
}

func (s *auditService) Record(ctx context.Context, e AuditEntry) error {
	safeArgs := redact(e.Args)
	argsJSON := "{}"
	if safeArgs != nil {
		b, err := json.Marshal(safeArgs)
		if err != nil {
			return fmt.Errorf("序列化 audit args 失败: %w", err)
		}
		argsJSON = string(b)
	}
	row := models.ActionAudit{
		NotificationConfID: e.NotificationConfID,
		ChannelType:        e.ChannelType,
		ChannelUserID:      e.ChannelUserID,
		Command:            e.Command,
		ArgsJSON:           argsJSON,
		Result:             e.Result,
		LatencyMs:          e.LatencyMs,
		CreatedAt:          time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("写入 action_audit 失败: %w", err)
	}
	return nil
}

func (s *auditService) Query(ctx context.Context, q AuditQuery) ([]AuditDTO, int, error) {
	page := q.Page
	if page <= 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}

	tx := s.db.WithContext(ctx).Model(&models.ActionAudit{})
	if !q.Since.IsZero() {
		tx = tx.Where("created_at >= ?", q.Since)
	}
	if !q.Until.IsZero() {
		tx = tx.Where("created_at < ?", q.Until)
	}
	if q.ChannelUserID != "" {
		tx = tx.Where("channel_user_id = ?", q.ChannelUserID)
	}
	if q.Command != "" {
		tx = tx.Where("command = ?", q.Command)
	}
	if q.Result != "" {
		tx = tx.Where("result = ?", q.Result)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计 action_audit 失败: %w", err)
	}

	var rows []models.ActionAudit
	if err := tx.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("查询 action_audit 失败: %w", err)
	}

	items := make([]AuditDTO, 0, len(rows))
	for _, r := range rows {
		items = append(items, AuditDTO{
			ID:            r.ID,
			CreatedAt:     r.CreatedAt,
			ChannelType:   r.ChannelType,
			ChannelUserID: r.ChannelUserID,
			Command:       r.Command,
			ArgsJSON:      r.ArgsJSON,
			Result:        r.Result,
			LatencyMs:     r.LatencyMs,
		})
	}
	return items, int(total), nil
}

func (s *auditService) Stats(ctx context.Context) (AuditStatsDTO, error) {
	var dto AuditStatsDTO

	if err := s.db.WithContext(ctx).Model(&models.ActionAudit{}).Count(&dto.TotalCount).Error; err != nil {
		return dto, fmt.Errorf("audit stats total: %w", err)
	}

	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := s.db.WithContext(ctx).Model(&models.ActionAudit{}).
		Where("created_at >= ?", midnight).
		Count(&dto.TodayCount).Error; err != nil {
		return dto, fmt.Errorf("audit stats today: %w", err)
	}

	var successCount int64
	if err := s.db.WithContext(ctx).Model(&models.ActionAudit{}).
		Where("result = ?", "success").
		Count(&successCount).Error; err != nil {
		return dto, fmt.Errorf("audit stats success: %w", err)
	}
	if dto.TotalCount > 0 {
		rate := float64(successCount) / float64(dto.TotalCount) * 100.0
		dto.SuccessRate = math.Round(rate*100) / 100
	}

	type latRow struct {
		Max int64
		Avg float64
	}
	var lat latRow
	if err := s.db.WithContext(ctx).Model(&models.ActionAudit{}).
		Select("COALESCE(MAX(latency_ms), 0) as max, COALESCE(AVG(latency_ms), 0) as avg").
		Scan(&lat).Error; err != nil {
		return dto, fmt.Errorf("audit stats latency: %w", err)
	}
	dto.MaxLatencyMs = lat.Max
	dto.AvgLatencyMs = math.Round(lat.Avg*100) / 100

	return dto, nil
}

func (s *auditService) Prune(ctx context.Context) (int64, error) {
	var totalDeleted int64

	cutoff := time.Now().Add(-s.retention)
	res := s.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&models.ActionAudit{})
	if res.Error != nil {
		return 0, fmt.Errorf("按时间清理 action_audit 失败: %w", res.Error)
	}
	totalDeleted += res.RowsAffected

	var count int64
	if err := s.db.WithContext(ctx).Model(&models.ActionAudit{}).Count(&count).Error; err != nil {
		return totalDeleted, fmt.Errorf("统计 action_audit 行数失败: %w", err)
	}

	if count > s.rowCap {
		excess := count - s.rowCap
		// 子查询选出最早 excess 条主键再删除，避免依赖方言相关的 ORDER BY DELETE。
		var ids []uint
		if err := s.db.WithContext(ctx).
			Model(&models.ActionAudit{}).
			Order("created_at ASC").
			Limit(int(excess)).
			Pluck("id", &ids).Error; err != nil {
			return totalDeleted, fmt.Errorf("选取超额 action_audit 主键失败: %w", err)
		}
		if len(ids) > 0 {
			res2 := s.db.WithContext(ctx).
				Where("id IN ?", ids).
				Delete(&models.ActionAudit{})
			if res2.Error != nil {
				return totalDeleted, fmt.Errorf("按行数上限清理 action_audit 失败: %w", res2.Error)
			}
			totalDeleted += res2.RowsAffected
		}
	}

	return totalDeleted, nil
}

// redact 递归遍历 args，对键名（小写）包含敏感关键词的字段，将其值替换为 [REDACTED]。
// 返回新的 map，不修改输入；嵌套 map / slice 同样不可变。
func redact(args map[string]any) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if isSensitiveKey(k) {
			out[k] = redactedPlaceholder
			continue
		}
		out[k] = redactValue(v)
	}
	return out
}

func redactValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return redact(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = redactValue(item)
		}
		return out
	default:
		return v
	}
}

func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(lower)
	for _, kw := range sensitiveKeys {
		if strings.Contains(normalized, kw) {
			return true
		}
	}
	return false
}
