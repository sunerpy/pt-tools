package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/models"
)

const (
	maxActiveCodesPerAdm = 3
	defaultReplyLang     = "zh"
)

var (
	ErrTooManyActiveCodes = errors.New("too many active bind codes for this admin")
	ErrCodeUsedOrExpired  = errors.New("bind code is invalid, expired, or already used")
	ErrInvalidReplyLang   = errors.New("invalid reply_lang; only 'zh' or 'en' allowed")
)

type BindCodeDTO struct {
	Code      string     `json:"code"`
	ConfID    uint       `json:"conf_id"`
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type BindingDTO struct {
	ID            uint      `json:"id"`
	ConfID        uint      `json:"conf_id"`
	ChannelType   string    `json:"channel_type"`
	ChannelUserID string    `json:"channel_user_id"`
	Label         string    `json:"label"`
	ReplyLang     string    `json:"reply_lang"`
	PtAdmin       bool      `json:"admin"`
	Allowed       bool      `json:"allowed"`
	CreatedAt     time.Time `json:"created_at"`
	LastActiveAt  time.Time `json:"last_active"`
}

type BindingService interface {
	IssueCode(ctx context.Context, confID uint, label string, ttl time.Duration) (BindCodeDTO, error)
	ListPendingCodes(ctx context.Context) ([]BindCodeDTO, error)
	ConsumeCode(ctx context.Context, code, channelType, channelUserID string) (BindingDTO, error)
	ListBindings(ctx context.Context) ([]BindingDTO, error)
	Revoke(ctx context.Context, bindingID uint) error
	SetReplyLang(ctx context.Context, bindingID uint, lang string) error
}

type bindingService struct {
	db        *gorm.DB
	createdBy string
	now       func() time.Time
	gen       func() (string, error)
}

func NewBindingService(db *gorm.DB, createdBy string) BindingService {
	return &bindingService{
		db:        db,
		createdBy: createdBy,
		now:       time.Now,
		gen:       chatops.GenerateBindCode,
	}
}

func (s *bindingService) IssueCode(ctx context.Context, confID uint, label string, ttl time.Duration) (BindCodeDTO, error) {
	now := s.now()

	var active int64
	if err := s.db.WithContext(ctx).Model(&models.BotToken{}).
		Where("kind = ? AND created_by = ? AND used_at IS NULL AND (expires_at IS NULL OR expires_at > ?)",
			"bind_code", s.createdBy, now).
		Count(&active).Error; err != nil {
		return BindCodeDTO{}, fmt.Errorf("count active bind codes: %w", err)
	}
	if active >= maxActiveCodesPerAdm {
		return BindCodeDTO{}, ErrTooManyActiveCodes
	}

	code, err := s.gen()
	if err != nil {
		return BindCodeDTO{}, fmt.Errorf("generate bind code: %w", err)
	}

	row := models.BotToken{
		Kind:            "bind_code",
		CodeOrTokenHash: code,
		Scope:           encodeBindScope(confID, label),
		CreatedBy:       s.createdBy,
	}
	var expiresPtr *time.Time
	if ttl > 0 {
		expires := now.Add(ttl)
		row.ExpiresAt = &expires
		expiresPtr = &expires
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return BindCodeDTO{}, fmt.Errorf("persist bind code: %w", err)
	}

	return BindCodeDTO{
		Code:      code,
		ConfID:    confID,
		Label:     label,
		ExpiresAt: expiresPtr,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *bindingService) ListPendingCodes(ctx context.Context) ([]BindCodeDTO, error) {
	now := s.now()
	var rows []models.BotToken
	if err := s.db.WithContext(ctx).
		Where("kind = ? AND used_at IS NULL AND (expires_at IS NULL OR expires_at > ?)",
			"bind_code", now).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list pending codes: %w", err)
	}

	out := make([]BindCodeDTO, 0, len(rows))
	for _, r := range rows {
		confID, label := parseBindScope(r.Scope)
		dto := BindCodeDTO{
			Code:      r.CodeOrTokenHash,
			ConfID:    confID,
			Label:     label,
			CreatedAt: r.CreatedAt,
		}
		if r.ExpiresAt != nil {
			t := *r.ExpiresAt
			dto.ExpiresAt = &t
		}
		out = append(out, dto)
	}
	return out, nil
}

func (s *bindingService) ConsumeCode(ctx context.Context, code, channelType, channelUserID string) (BindingDTO, error) {
	if code == "" || channelType == "" || channelUserID == "" {
		return BindingDTO{}, ErrCodeUsedOrExpired
	}

	var binding models.ChannelBinding
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now()

		var token models.BotToken
		if err := tx.Where(
			"kind = ? AND code_or_token_hash = ? AND used_at IS NULL AND (expires_at IS NULL OR expires_at > ?)",
			"bind_code", code, now,
		).First(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCodeUsedOrExpired
			}
			return fmt.Errorf("lookup bind code: %w", err)
		}

		// Atomic mark-used: race-safe via WHERE used_at IS NULL.
		res := tx.Model(&models.BotToken{}).
			Where("id = ? AND used_at IS NULL", token.ID).
			UpdateColumn("used_at", now)
		if res.Error != nil {
			return fmt.Errorf("mark code used: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrCodeUsedOrExpired
		}

		confID, label := parseBindScope(token.Scope)
		binding = models.ChannelBinding{
			NotificationConfID: confID,
			ChannelType:        channelType,
			ChannelUserID:      channelUserID,
			PtAdmin:            true,
			Allowed:            true,
			Label:              label,
			ReplyLang:          defaultReplyLang,
		}
		if err := tx.Create(&binding).Error; err != nil {
			return fmt.Errorf("create channel binding: %w", err)
		}
		return nil
	})
	if err != nil {
		return BindingDTO{}, err
	}

	return toBindingDTO(binding), nil
}

func (s *bindingService) ListBindings(ctx context.Context) ([]BindingDTO, error) {
	var rows []models.ChannelBinding
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	out := make([]BindingDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBindingDTO(r))
	}
	return out, nil
}

func (s *bindingService) Revoke(ctx context.Context, bindingID uint) error {
	res := s.db.WithContext(ctx).Delete(&models.ChannelBinding{}, bindingID)
	if res.Error != nil {
		return fmt.Errorf("revoke binding: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *bindingService) SetReplyLang(ctx context.Context, bindingID uint, lang string) error {
	if lang != "zh" && lang != "en" {
		return ErrInvalidReplyLang
	}
	res := s.db.WithContext(ctx).Model(&models.ChannelBinding{}).
		Where("id = ?", bindingID).
		UpdateColumn("reply_lang", lang)
	if res.Error != nil {
		return fmt.Errorf("update reply_lang: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// encodeBindScope serializes (confID, label) to "bind:<confID>" or
// "bind:<confID>|label:<label>" if label is non-empty. Decoded by parseBindScope.
func encodeBindScope(confID uint, label string) string {
	if label == "" {
		return fmt.Sprintf("bind:%d", confID)
	}
	return fmt.Sprintf("bind:%d|label:%s", confID, label)
}

// parseBindScope decodes "bind:<confID>|label:<label>" → (confID, label).
// Tolerant: returns zeroes if format unexpected.
func parseBindScope(scope string) (uint, string) {
	var confID uint
	var label string
	const prefix = "bind:"
	if len(scope) < len(prefix) || scope[:len(prefix)] != prefix {
		return 0, ""
	}
	rest := scope[len(prefix):]
	pipe := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '|' {
			pipe = i
			break
		}
	}
	idStr := rest
	if pipe >= 0 {
		idStr = rest[:pipe]
		labelPart := rest[pipe+1:]
		const lp = "label:"
		if len(labelPart) >= len(lp) && labelPart[:len(lp)] == lp {
			label = labelPart[len(lp):]
		}
	}
	var n uint
	for i := 0; i < len(idStr); i++ {
		c := idStr[i]
		if c < '0' || c > '9' {
			return 0, label
		}
		n = n*10 + uint(c-'0')
	}
	confID = n
	return confID, label
}

func toBindingDTO(r models.ChannelBinding) BindingDTO {
	dto := BindingDTO{
		ID:            r.ID,
		ConfID:        r.NotificationConfID,
		ChannelType:   r.ChannelType,
		ChannelUserID: r.ChannelUserID,
		Label:         r.Label,
		ReplyLang:     r.ReplyLang,
		PtAdmin:       r.PtAdmin,
		Allowed:       r.Allowed,
		CreatedAt:     r.CreatedAt,
	}
	if r.LastActiveAt != nil {
		dto.LastActiveAt = *r.LastActiveAt
	}
	return dto
}
