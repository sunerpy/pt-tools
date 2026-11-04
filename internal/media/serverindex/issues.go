package serverindex

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
)

// ErrIssueNotFound 表示问题不存在。
var ErrIssueNotFound = errors.New("问题不存在")

// IssueKey 是一个问题的唯一键：不适用的列写 0 或空串（server_removed 的 HistoryID 是 0，library_missing 的服务器两列是 0 与空串）。
type IssueKey struct {
	Kind         string
	ServerID     uint
	ServerItemID string
	HistoryID    uint
}

// IssueInfo 是问题说的是哪一部、哪一集，以及说明。
type IssueInfo struct {
	MediaType string
	TMDBID    int
	Season    int
	Episode   int
	Title     string
	Detail    string
}

func removedKey(serverID uint, itemID string) IssueKey {
	return IssueKey{Kind: models.MediaIssueServerRemoved, ServerID: serverID, ServerItemID: itemID}
}

func (k IssueKey) where(tx *gorm.DB) *gorm.DB {
	return tx.Where("kind = ? AND server_id = ? AND server_item_id = ? AND history_id = ?", k.Kind, k.ServerID, k.ServerItemID, k.HistoryID)
}

// Raise 记下发现了一个问题：没有就新建（open）；解决过的（resolved）改回 open；还开着的更新说明；用户忽略的（dismissed）不动。
func Raise(tx *gorm.DB, key IssueKey, info IssueInfo, now time.Time) error {
	row := models.MediaIssue{
		Kind: key.Kind, Status: models.MediaIssueOpen, ServerID: key.ServerID, ServerItemID: key.ServerItemID, HistoryID: key.HistoryID,
		MediaType: info.MediaType, TMDBID: info.TMDBID, Season: info.Season, Episode: info.Episode, Title: info.Title, Detail: info.Detail,
		CreatedAt: now, UpdatedAt: now,
	}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return fmt.Errorf("记下问题失败: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	err := key.where(tx.Model(&models.MediaIssue{})).Where("status <> ?", models.MediaIssueDismissed).Updates(map[string]any{
		"status": models.MediaIssueOpen, "media_type": info.MediaType, "tmdb_id": info.TMDBID, "season": info.Season, "episode": info.Episode,
		"title": info.Title, "detail": info.Detail, "updated_at": now, "resolved_at": nil,
	}).Error
	if err != nil {
		return fmt.Errorf("记下问题失败: %w", err)
	}
	return nil
}

// Clear 记下问题没有了：还开着的与用户忽略的改成解决了（resolved），写上说明；没有这个问题或已经解决的不动。
func Clear(tx *gorm.DB, key IssueKey, detail string, now time.Time) error {
	err := key.where(tx.Model(&models.MediaIssue{})).Where("status IN ?", []string{models.MediaIssueOpen, models.MediaIssueDismissed}).
		Updates(map[string]any{"status": models.MediaIssueResolved, "detail": truncate(detail, 1024), "updated_at": now, "resolved_at": now}).Error
	if err != nil {
		return fmt.Errorf("记下问题解决失败: %w", err)
	}
	return nil
}

// Dismiss 是用户忽略一个还开着的问题。问题不存在时返回 ErrIssueNotFound；已经不是开着的不动，返回它现在的样子。
func Dismiss(ctx context.Context, db *gorm.DB, id uint, now time.Time) (models.MediaIssue, error) {
	var row models.MediaIssue
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.MediaIssue{}).Where("id = ? AND status = ?", id, models.MediaIssueOpen).
			Updates(map[string]any{"status": models.MediaIssueDismissed, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("忽略问题失败: %w", err)
		}
		if err := tx.Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
			return fmt.Errorf("读取问题失败: %w", err)
		}
		if row.ID == 0 {
			return ErrIssueNotFound
		}
		return nil
	})
	return row, err
}

// IssueFilter 是列问题的条件；空的一项不限。
type IssueFilter struct {
	Status    string
	Kind      string
	MediaType string
	TMDBID    int
	Limit     int
}

// Issues 列出问题，最新的在前；Limit 为 0 时最多 500 条。
func Issues(ctx context.Context, db *gorm.DB, f IssueFilter) ([]models.MediaIssue, error) {
	q := db.WithContext(ctx).Model(&models.MediaIssue{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.MediaType != "" {
		q = q.Where("media_type = ?", f.MediaType)
	}
	if f.TMDBID > 0 {
		q = q.Where("tmdb_id = ?", f.TMDBID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	out := []models.MediaIssue{}
	if err := q.Order("julianday(updated_at) DESC, id DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取问题失败: %w", err)
	}
	return out, nil
}
