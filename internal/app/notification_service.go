package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// Notification 是 NotificationService 与 NotifyManager 之间传递的最小消息载荷。
// TODO(T15): 替换为 internal/notify 包内的 notify.Notification 完整结构。
type Notification struct {
	Title        string            `json:"title"`
	Text         string            `json:"text"`
	SourceConfID uint              `json:"source_conf_id,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	Targets      map[string]string `json:"targets,omitempty"`
}

// NotifyManager 抽象底层投递。
// TODO(T15): 替换为 internal/notify.Manager 接口的真实实现。
type NotifyManager interface {
	Send(ctx context.Context, confID uint, n Notification) error
}

// NotificationConfDTO 是对 models.NotificationConf 的对外只读视图，不含密文字段。
type NotificationConfDTO struct {
	ID          uint            `json:"id"`
	ChannelType string          `json:"channel_type"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	ConfigJSON  json.RawMessage `json:"config_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CreateConfReq 创建通知通道的请求；ConfigJSON 为通道原始配置，进入 service 后会被 AES-GCM 加密落库。
type CreateConfReq struct {
	ChannelType string
	Name        string
	ConfigJSON  json.RawMessage
	Enabled     bool
}

// UpdateConfReq 更新请求，ConfigJSON 为空时不更新对应字段。
type UpdateConfReq struct {
	ChannelType *string
	Name        *string
	ConfigJSON  json.RawMessage
	Enabled     *bool
}

// NotificationService 管理通知通道配置与消息投递。
type NotificationService interface {
	ListConfs(ctx context.Context) ([]NotificationConfDTO, error)
	GetConf(ctx context.Context, id uint) (NotificationConfDTO, error)
	CreateConf(ctx context.Context, req CreateConfReq) (NotificationConfDTO, error)
	UpdateConf(ctx context.Context, id uint, req UpdateConfReq) error
	DeleteConf(ctx context.Context, id uint) error
	TestConf(ctx context.Context, id uint) error
	Push(ctx context.Context, n Notification) error
	Enqueue(ctx context.Context, n Notification, confID uint) error
}

// ErrConfNotFound 当 conf id 不存在时返回。
var ErrConfNotFound = errors.New("notification conf not found")

type notificationService struct {
	db          *gorm.DB
	manager     NotifyManager
	pushTimeout time.Duration
}

// NewNotificationService 构造一个 NotificationService。pushTimeout 为同步投递的最长等待时间，
// 超时后会落到 notification_outbox 表由后台 worker 异步重试。
func NewNotificationService(db *gorm.DB, manager NotifyManager, pushTimeout time.Duration) NotificationService {
	if pushTimeout <= 0 {
		pushTimeout = 5 * time.Second
	}
	return &notificationService{db: db, manager: manager, pushTimeout: pushTimeout}
}

func (s *notificationService) ListConfs(ctx context.Context) ([]NotificationConfDTO, error) {
	var rows []models.NotificationConf
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询通知通道列表失败: %w", err)
	}
	out := make([]NotificationConfDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, NotificationConfDTO{
			ID:          r.ID,
			ChannelType: r.ChannelType,
			Name:        r.Name,
			Enabled:     r.Enabled,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		})
	}
	return out, nil
}

// GetConf 返回单个通知通道配置；config_json 会被解密成原始 JSON 对象，供前端详情页渲染。
func (s *notificationService) GetConf(ctx context.Context, id uint) (NotificationConfDTO, error) {
	if id == 0 {
		return NotificationConfDTO{}, errors.New("id 不能为零")
	}
	var row models.NotificationConf
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotificationConfDTO{}, ErrConfNotFound
	}
	if err != nil {
		return NotificationConfDTO{}, fmt.Errorf("查询通道详情失败: %w", err)
	}
	dto := NotificationConfDTO{
		ID:          row.ID,
		ChannelType: row.ChannelType,
		Name:        row.Name,
		Enabled:     row.Enabled,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if row.ConfigJSON != "" {
		plain, derr := crypto.Decrypt(row.ConfigJSON)
		if derr != nil {
			return NotificationConfDTO{}, fmt.Errorf("解密 config_json 失败: %w", derr)
		}
		if json.Valid(plain) {
			dto.ConfigJSON = json.RawMessage(plain)
		}
	}
	return dto, nil
}

func (s *notificationService) CreateConf(ctx context.Context, req CreateConfReq) (NotificationConfDTO, error) {
	if req.ChannelType == "" {
		return NotificationConfDTO{}, errors.New("channel_type 不能为空")
	}
	if req.Name == "" {
		return NotificationConfDTO{}, errors.New("name 不能为空")
	}
	if len(req.ConfigJSON) == 0 {
		return NotificationConfDTO{}, errors.New("config_json 不能为空")
	}

	cipherStr, err := crypto.Encrypt([]byte(req.ConfigJSON))
	if err != nil {
		return NotificationConfDTO{}, fmt.Errorf("加密通道配置失败: %w", err)
	}

	row := models.NotificationConf{
		ChannelType: req.ChannelType,
		Name:        req.Name,
		ConfigJSON:  cipherStr,
		Enabled:     req.Enabled,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return NotificationConfDTO{}, fmt.Errorf("创建通知通道失败: %w", err)
	}
	return NotificationConfDTO{
		ID:          row.ID,
		ChannelType: row.ChannelType,
		Name:        row.Name,
		Enabled:     row.Enabled,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

func (s *notificationService) UpdateConf(ctx context.Context, id uint, req UpdateConfReq) error {
	if id == 0 {
		return errors.New("id 不能为零")
	}
	updates := map[string]any{}
	if req.ChannelType != nil {
		updates["channel_type"] = *req.ChannelType
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(req.ConfigJSON) > 0 {
		cipherStr, err := crypto.Encrypt([]byte(req.ConfigJSON))
		if err != nil {
			return fmt.Errorf("加密通道配置失败: %w", err)
		}
		updates["config_json"] = cipherStr
	}
	if len(updates) == 0 {
		return nil
	}
	res := s.db.WithContext(ctx).Model(&models.NotificationConf{}).
		Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("更新通知通道失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrConfNotFound
	}
	return nil
}

func (s *notificationService) DeleteConf(ctx context.Context, id uint) error {
	if id == 0 {
		return errors.New("id 不能为零")
	}
	res := s.db.WithContext(ctx).Delete(&models.NotificationConf{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除通知通道失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrConfNotFound
	}
	return nil
}

func (s *notificationService) TestConf(ctx context.Context, id uint) error {
	var row models.NotificationConf
	err := s.db.WithContext(ctx).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrConfNotFound
	}
	if err != nil {
		return fmt.Errorf("查询通知通道失败: %w", err)
	}
	chatID, err := s.testChatID(ctx, row)
	if err != nil {
		return err
	}
	targets := map[string]string(nil)
	if chatID != "" {
		targets = map[string]string{"chat_id": chatID}
		if row.ChannelType == "qq_onebot" {
			targets["message_type"] = "private"
		}
	}
	return s.Push(ctx, Notification{
		Title:        "pt-tools 测试通知",
		Text:         "如果你看到此消息，说明通道配置正常。",
		SourceConfID: row.ID,
		UserID:       chatID,
		Targets:      targets,
	})
}

func (s *notificationService) testChatID(ctx context.Context, row models.NotificationConf) (string, error) {
	switch row.ChannelType {
	case "qq_onebot":
		chatID, err := qqTestChatID(row)
		if err != nil {
			return "", err
		}
		if chatID != "" {
			return chatID, nil
		}
		if bindingChatID, err := s.firstBindingChatID(ctx, row.ID); err != nil || bindingChatID != "" {
			return bindingChatID, err
		}
		return "", errors.New("QQ 通道无可用收件人：请配置至少一个 admin_qq_users 或先完成一次绑定")
	case "telegram":
		chatID, err := telegramTestChatID(row)
		if err != nil {
			return "", err
		}
		if chatID != "" {
			return chatID, nil
		}
		if bindingChatID, err := s.firstBindingChatID(ctx, row.ID); err != nil || bindingChatID != "" {
			return bindingChatID, err
		}
		return "", errors.New("Telegram 通道无可用收件人：请配置 default_chat_id、至少一个 admin_users 或先完成一次绑定")
	default:
		return "", nil
	}
}

func qqTestChatID(row models.NotificationConf) (string, error) {
	var cfg struct {
		AdminQQUsers   []int64 `json:"admin_qq_users"`
		AllowedQQUsers []int64 `json:"allowed_qq_users"`
	}
	if err := decryptConfigJSON(row.ConfigJSON, &cfg); err != nil {
		return "", err
	}
	if len(cfg.AdminQQUsers) > 0 {
		return strconv.FormatInt(cfg.AdminQQUsers[0], 10), nil
	}
	if len(cfg.AllowedQQUsers) > 0 {
		return strconv.FormatInt(cfg.AllowedQQUsers[0], 10), nil
	}
	return "", nil
}

func telegramTestChatID(row models.NotificationConf) (string, error) {
	var cfg map[string]json.RawMessage
	if err := decryptConfigJSON(row.ConfigJSON, &cfg); err != nil {
		return "", err
	}
	if raw := cfg["default_chat_id"]; len(raw) > 0 {
		chatID, err := rawStringOrInt64(raw)
		if err != nil {
			return "", fmt.Errorf("解析 telegram default_chat_id 失败: %w", err)
		}
		if chatID != "" {
			return chatID, nil
		}
	}
	if raw := cfg["admin_users"]; len(raw) > 0 {
		users, err := rawStringOrInt64Slice(raw)
		if err != nil {
			return "", fmt.Errorf("解析 telegram admin_users 失败: %w", err)
		}
		if len(users) > 0 {
			return users[0], nil
		}
	}
	return "", nil
}

func decryptConfigJSON(configJSON string, dst any) error {
	if configJSON == "" {
		return nil
	}
	plain, err := crypto.Decrypt(configJSON)
	if err != nil {
		return fmt.Errorf("解密 config_json 失败: %w", err)
	}
	if err := json.Unmarshal(plain, dst); err != nil {
		return fmt.Errorf("解析 config_json 失败: %w", err)
	}
	return nil
}

func rawStringOrInt64(raw json.RawMessage) (string, error) {
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str, nil
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err == nil {
		if id == 0 {
			return "", nil
		}
		return strconv.FormatInt(id, 10), nil
	}
	return "", errors.New("值必须是字符串或整数")
}

func rawStringOrInt64Slice(raw json.RawMessage) ([]string, error) {
	var strings []string
	if err := json.Unmarshal(raw, &strings); err == nil {
		return strings, nil
	}
	var ids []int64
	if err := json.Unmarshal(raw, &ids); err == nil {
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, strconv.FormatInt(id, 10))
		}
		return out, nil
	}
	return nil, errors.New("值必须是字符串数组或整数数组")
}

func (s *notificationService) firstBindingChatID(ctx context.Context, confID uint) (string, error) {
	var binding models.ChannelBinding
	err := s.db.WithContext(ctx).
		Where("notification_conf_id = ?", confID).
		Order("pt_admin DESC, allowed DESC, id ASC").
		First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("查询 channel_binding 失败: %w", err)
	}
	return binding.ChannelUserID, nil
}

// Push 同步投递：尝试在 pushTimeout 内调用 NotifyManager.Send；超时或失败则转为 outbox 异步队列。
func (s *notificationService) Push(ctx context.Context, n Notification) error {
	if s.manager == nil {
		return s.Enqueue(ctx, n, n.SourceConfID)
	}
	sendCtx, cancel := context.WithTimeout(ctx, s.pushTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.manager.Send(sendCtx, n.SourceConfID, n)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			return nil
		}
		return s.Enqueue(ctx, n, n.SourceConfID)
	case <-sendCtx.Done():
		return s.Enqueue(ctx, n, n.SourceConfID)
	}
}

func (s *notificationService) Enqueue(ctx context.Context, n Notification, confID uint) error {
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("序列化通知载荷失败: %w", err)
	}
	row := models.NotificationOutbox{
		NotificationConfID: confID,
		PayloadJSON:        string(payload),
		Status:             "pending",
		NextRetryAt:        time.Now(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("写入 outbox 失败: %w", err)
	}
	return nil
}
