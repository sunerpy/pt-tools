package recognize

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/words"
	"github.com/sunerpy/pt-tools/models"
)

// OverrideInput 是手动纠正的参数：这个标题（按解析结果）固定识别成 TMDB 上的这个条目。
type OverrideInput struct {
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	TMDBID    int    `json:"tmdb_id"`
	MediaType string `json:"media_type"`
}

// SetOverride 保存一条手动纠正：先到 TMDB 确认条目存在。同一个解析结果只保留最新的一条。
func (s *Service) SetOverride(ctx context.Context, in OverrideInput) (*models.MediaOverride, error) {
	clean, err := cleanInput(Input{Title: in.Title, Subtitle: in.Subtitle})
	if err != nil {
		return nil, err
	}
	if in.MediaType != tmdb.KindMovie && in.MediaType != tmdb.KindTV {
		return nil, fmt.Errorf("%w: 类型只能是 movie 或 tv", ErrInvalid)
	}
	if in.TMDBID <= 0 {
		return nil, fmt.Errorf("%w: TMDB 编号要是正整数", ErrInvalid)
	}
	m, _, err := s.parse(ctx, clean)
	if err != nil {
		return nil, err
	}
	key := overrideKey(m)
	if key == "" {
		return nil, fmt.Errorf("%w: 解析不出名字，不能纠正", ErrInvalid)
	}
	c, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	d, err := c.Details(ctx, in.MediaType, in.TMDBID)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, fmt.Errorf("%w: TMDB 上没有这个条目（%s %d）", ErrInvalid, in.MediaType, in.TMDBID)
	}
	if err != nil {
		return nil, err
	}
	label := m.String()
	if m.NameCN != "" && m.NameCN != m.NameEN && !strings.Contains(label, m.NameCN) {
		label = m.NameCN + " / " + label
	}
	row := models.MediaOverride{Key: key, Label: label, TMDBID: in.TMDBID, MediaType: in.MediaType, Title: d.Title}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"label", "tmdb_id", "media_type", "title", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("保存手动纠正失败: %w", err)
	}
	var saved models.MediaOverride
	if err := s.cfg.DB.WithContext(ctx).Where("key = ?", key).First(&saved).Error; err != nil {
		return nil, fmt.Errorf("读取手动纠正失败: %w", err)
	}
	return &saved, nil
}

// Overrides 列出手动纠正（新的在前）。
func (s *Service) Overrides(ctx context.Context) ([]models.MediaOverride, error) {
	var rows []models.MediaOverride
	if err := s.cfg.DB.WithContext(ctx).Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取手动纠正失败: %w", err)
	}
	return rows, nil
}

// DeleteOverride 删除一条手动纠正。
func (s *Service) DeleteOverride(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaOverride{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除手动纠正失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Words 列出识别词（按套用顺序）。
func (s *Service) Words(ctx context.Context) ([]models.MediaWordRule, error) {
	var rows []models.MediaWordRule
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取识别词失败: %w", err)
	}
	return rows, nil
}

// SaveWord 新建（ID 为 0）或修改一条识别词。
func (s *Service) SaveWord(ctx context.Context, r models.MediaWordRule) (*models.MediaWordRule, error) {
	r.Kind = strings.TrimSpace(r.Kind)
	r.Note = strings.TrimSpace(r.Note)
	if r.Kind != models.MediaWordReplace {
		r.Replacement = ""
	}
	if r.Kind != models.MediaWordOffset {
		r.Offset = 0
	}
	if err := words.Validate(r); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if len([]rune(r.Note)) > 256 {
		return nil, fmt.Errorf("%w: 备注最多 256 个字", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	if r.ID == 0 {
		var n int64
		if err := db.Model(&models.MediaWordRule{}).Count(&n).Error; err != nil {
			return nil, fmt.Errorf("统计识别词失败: %w", err)
		}
		if n >= maxWords {
			return nil, fmt.Errorf("%w: 识别词最多 %d 条", ErrInvalid, maxWords)
		}
		row := models.MediaWordRule{
			Kind: r.Kind, Pattern: r.Pattern, Replacement: r.Replacement, Offset: r.Offset,
			IsRegex: r.IsRegex, Enabled: r.Enabled, Note: r.Note,
		}
		if err := db.Create(&row).Error; err != nil {
			return nil, fmt.Errorf("保存识别词失败: %w", err)
		}
		return &row, nil
	}
	res := db.Model(&models.MediaWordRule{}).Where("id = ?", r.ID).
		Select("kind", "pattern", "replacement", "offset", "is_regex", "enabled", "note", "updated_at").
		Updates(&models.MediaWordRule{
			Kind: r.Kind, Pattern: r.Pattern, Replacement: r.Replacement, Offset: r.Offset,
			IsRegex: r.IsRegex, Enabled: r.Enabled, Note: r.Note,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("保存识别词失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	var saved models.MediaWordRule
	if err := db.First(&saved, r.ID).Error; err != nil {
		return nil, fmt.Errorf("读取识别词失败: %w", err)
	}
	return &saved, nil
}

// DeleteWord 删除一条识别词。
func (s *Service) DeleteWord(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaWordRule{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除识别词失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
