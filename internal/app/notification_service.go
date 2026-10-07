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
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// publishNotificationConfigChanged 通知订阅方（如 cmd/web.go 中的热重载 goroutine）
// 通道配置发生变化，需重建对应通道实例以加载最新的解密配置（proxy_url、bot_token 等）。
func publishNotificationConfigChanged() {
	events.Publish(events.Event{
		Type:    events.ConfigChanged,
		Version: time.Now().UnixNano(),
		Source:  "notification",
		At:      time.Now(),
	})
}

// Notification 是 NotificationService 与 NotifyManager 之间传递的最小消息载荷。
// TODO(T15): 替换为 internal/notify 包内的 notify.Notification 完整结构。
type Notification struct {
	Title        string            `json:"title"`
	Text         string            `json:"text"`
	SourceConfID uint              `json:"source_conf_id,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	Targets      map[string]string `json:"targets,omitempty"`
	Buttons      [][]notify.Button `json:"buttons,omitempty"`
}

// NotifyManager 抽象底层投递。
// TODO(T15): 替换为 internal/notify.Manager 接口的真实实现。
type NotifyManager interface {
	Send(ctx context.Context, confID uint, n Notification) error
}

/*
 * 运行态取值。落到前端是通知通道页画板 22 那条分段器。
 *
 * 四档而不是三档，是因为「在跑」和「接上了」不是一回事：
 * 四个适配器的 Healthy() 含义都只是**构造/启动成功**（QQ 绑上端口就 true，而 NapCat
 * 没握手时发送会明确失败；Telegram 造出 bot 就 true，没做任何网络确认；Webhook 只判
 * config != nil；WeCom 恒 true）。把 Healthy() 直接说成「已连接」就是在界面上说假话 ——
 * 这正是上一轮评审查出的缺陷。
 *
 * 所以只有实现了 notify.LinkStater 并确认对端接上的通道才配 Connected；
 * 其余在跑的一律 Running（界面写「运行中」，不承诺连通性）。
 */
const (
	// ChannelStateConnected 实例在跑，且适配器确认对端真的接上了。
	ChannelStateConnected = "connected"
	// ChannelStateRunning 实例在跑，但连通性未知（适配器给不出这个判断）。
	ChannelStateRunning = "running"
	// ChannelStateError 该跑却没跑起来，或实例自报不健康。
	ChannelStateError = "error"
	// ChannelStateDisabled 配置本身停用了 —— 没跑不是故障。
	ChannelStateDisabled = "disabled"
)

// ChannelStater 由持有实时通道实例的一方实现（cmd 层的 liveNotifyManager）。
// 返回 Connected / Running / Error，或空串表示这一条没有实例在跑。
//
// 做成可选能力而不是塞进 NotifyManager：测试里的假 manager 只关心 Send，
// 不该被迫编造运行态。
type ChannelStater interface {
	ChannelState(confID uint) string
}

// channelRuntimeState 把「配置启用与否」和「实例处于什么状态」合成一个对外状态。
//
// 关键的一档是 enabled 却没有实例：启动时逐条 Init，失败的那条只记日志然后跳过
// （见 cmd.initEnabledChannels），所以「启用了但不在 map 里」正是初始化失败的样子，
// 对用户来说就是异常，不是「未运行」。
func channelRuntimeState(enabled bool, live string) string {
	if !enabled {
		return ChannelStateDisabled
	}
	switch live {
	case ChannelStateConnected:
		return ChannelStateConnected
	case ChannelStateRunning:
		return ChannelStateRunning
	default:
		/* 包括 Error 与空串：启用了却问不到实例，就是没起来 */
		return ChannelStateError
	}
}

// confRuntimeState 是 DTO 用的那一份：manager 不提供这个信号时给空串
// （不能一律按「异常」报 —— 那会把「问不到」说成「坏了」）。
func (s *notificationService) confRuntimeState(enabled bool, confID uint) string {
	stater, ok := s.manager.(ChannelStater)
	if !ok || stater == nil {
		return ""
	}
	return channelRuntimeState(enabled, stater.ChannelState(confID))
}

// NotificationConfDTO 是对 models.NotificationConf 的对外只读视图，不含密文字段。
type NotificationConfDTO struct {
	ID              uint            `json:"id"`
	ChannelType     string          `json:"channel_type"`
	Name            string          `json:"name"`
	Enabled         bool            `json:"enabled"`
	QuietHoursStart string          `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd   string          `json:"quiet_hours_end,omitempty"`
	ConfigJSON      json.RawMessage `json:"config_json,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	// RuntimeState 是进程内的实时运行态：connected / error / disabled。
	// 不落库 —— 它描述的是「现在这个进程里跑着什么」，重启即重算。
	// manager 不提供这个信号时为空串（前端据此不画那枚状态点）。
	RuntimeState string `json:"runtime_state,omitempty"`
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
	ChannelType     *string
	Name            *string
	ConfigJSON      json.RawMessage
	Enabled         *bool
	QuietHoursStart *string
	QuietHoursEnd   *string
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
	PushSync(ctx context.Context, n Notification) error
	Enqueue(ctx context.Context, n Notification, confID uint) error
}

// ErrConfNotFound 当 conf id 不存在时返回。
var ErrConfNotFound = errors.New("notification conf not found")

// ErrInvalidConf 表示通道配置没通过适配器的检查：保存前就拦下，不等热重载时才失败。
var ErrInvalidConf = errors.New("通道配置无效")

type notificationService struct {
	db          *gorm.DB
	manager     NotifyManager
	pushTimeout time.Duration
	registry    *notify.Registry // 检查配置用；默认是 notify.DefaultRegistry()
}

// NewNotificationService 构造一个 NotificationService。pushTimeout 为同步投递的最长等待时间，
// 超时后会落到 notification_outbox 表由后台 worker 异步重试。
func NewNotificationService(db *gorm.DB, manager NotifyManager, pushTimeout time.Duration) NotificationService {
	if pushTimeout <= 0 {
		pushTimeout = 5 * time.Second
	}
	return &notificationService{db: db, manager: manager, pushTimeout: pushTimeout, registry: notify.DefaultRegistry()}
}

// checkConf 用通道类型对应的适配器检查明文配置（不发请求）。类型没注册、或适配器不支持检查时放行。
func (s *notificationService) checkConf(typ string, plain []byte) error {
	ch, err := s.registry.Make(typ)
	if err != nil {
		return nil
	}
	checker, ok := ch.(notify.ConfigChecker)
	if !ok {
		return nil
	}
	if err := checker.CheckConfig(&models.NotificationConf{ChannelType: typ, ConfigJSON: string(plain)}); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConf, err.Error())
	}
	return nil
}

func (s *notificationService) ListConfs(ctx context.Context) ([]NotificationConfDTO, error) {
	var rows []models.NotificationConf
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询通知通道列表失败: %w", err)
	}
	out := make([]NotificationConfDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, NotificationConfDTO{
			ID:              r.ID,
			ChannelType:     r.ChannelType,
			Name:            r.Name,
			Enabled:         r.Enabled,
			QuietHoursStart: r.QuietHoursStart,
			QuietHoursEnd:   r.QuietHoursEnd,
			CreatedAt:       r.CreatedAt,
			UpdatedAt:       r.UpdatedAt,
			RuntimeState:    s.confRuntimeState(r.Enabled, r.ID),
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
		ID:              row.ID,
		ChannelType:     row.ChannelType,
		Name:            row.Name,
		Enabled:         row.Enabled,
		QuietHoursStart: row.QuietHoursStart,
		QuietHoursEnd:   row.QuietHoursEnd,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		RuntimeState:    s.confRuntimeState(row.Enabled, row.ID),
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
	if err := s.checkConf(req.ChannelType, req.ConfigJSON); err != nil {
		return NotificationConfDTO{}, err
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
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		// Enabled 列带 gorm 默认值 true：Create 会跳过零值 false，库里存成启用。停用的要显式写一次
		if !req.Enabled {
			if err := tx.Model(&row).Update("enabled", false).Error; err != nil {
				return err
			}
			row.Enabled = false
		}
		return nil
	}); err != nil {
		return NotificationConfDTO{}, fmt.Errorf("创建通知通道失败: %w", err)
	}
	publishNotificationConfigChanged()
	return NotificationConfDTO{
		ID:              row.ID,
		ChannelType:     row.ChannelType,
		Name:            row.Name,
		Enabled:         row.Enabled,
		QuietHoursStart: row.QuietHoursStart,
		QuietHoursEnd:   row.QuietHoursEnd,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
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
	if req.QuietHoursStart != nil {
		updates["quiet_hours_start"] = *req.QuietHoursStart
	}
	if req.QuietHoursEnd != nil {
		updates["quiet_hours_end"] = *req.QuietHoursEnd
	}
	// 改了配置或类型时，用合并后的配置按（新的）类型检查一遍再存
	if req.ChannelType != nil || len(req.ConfigJSON) > 0 {
		// 合并策略：解密 DB 现有配置 → 与 partial 新值 merge（新值覆盖、缺失键保留）→ 重新加密。
		// 避免前端只发部分字段时把其他原有字段（admin_users / default_chat_id 等）覆盖掉。
		var existingRow models.NotificationConf
		if err := s.db.WithContext(ctx).First(&existingRow, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrConfNotFound
			}
			return fmt.Errorf("查询通道失败: %w", err)
		}

		existingMap := map[string]json.RawMessage{}
		if existingRow.ConfigJSON != "" {
			plain, derr := crypto.Decrypt(existingRow.ConfigJSON)
			if derr != nil {
				return fmt.Errorf("解密旧配置失败: %w", derr)
			}
			if len(plain) > 0 {
				if perr := json.Unmarshal(plain, &existingMap); perr != nil {
					return fmt.Errorf("解析旧配置失败: %w", perr)
				}
			}
		}

		if len(req.ConfigJSON) > 0 {
			var newMap map[string]json.RawMessage
			if perr := json.Unmarshal([]byte(req.ConfigJSON), &newMap); perr != nil {
				return fmt.Errorf("解析新配置失败: %w", perr)
			}
			for k, v := range newMap {
				existingMap[k] = v
			}
		}

		merged, merr := json.Marshal(existingMap)
		if merr != nil {
			return fmt.Errorf("合并配置失败: %w", merr)
		}
		typ := existingRow.ChannelType
		if req.ChannelType != nil {
			typ = *req.ChannelType
		}
		if err := s.checkConf(typ, merged); err != nil {
			return err
		}
		if len(req.ConfigJSON) > 0 {
			cipherStr, err := crypto.Encrypt(merged)
			if err != nil {
				return fmt.Errorf("加密通道配置失败: %w", err)
			}
			updates["config_json"] = cipherStr
		}
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
	publishNotificationConfigChanged()
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
	publishNotificationConfigChanged()
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
	return s.PushSync(ctx, Notification{
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

// PushSync 仅尝试同步投递：超时或失败时不会写入 outbox，错误原样返回给调用方。
// 用于 TestConf 等需要"立即验证通道是否可用"的语义场景；业务通知请使用 Push（含 outbox fallback）。
func (s *notificationService) PushSync(ctx context.Context, n Notification) error {
	if s.manager == nil {
		return errors.New("通知管理器未初始化")
	}
	sendCtx, cancel := context.WithTimeout(ctx, s.pushTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.manager.Send(sendCtx, n.SourceConfID, n)
	}()

	select {
	case err := <-errCh:
		return err
	case <-sendCtx.Done():
		return fmt.Errorf("通道未在 %s 内响应: %w", s.pushTimeout, sendCtx.Err())
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
