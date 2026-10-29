package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

/*
 * 绑定页的「通道」一栏：每个能收命令的通道（Telegram、QQ）名单里有谁、谁已经绑定、每个账号最近一次和机器人的往来，
 * 以及没有指定收件人的通知（每日战报、签到、追剧…）发给谁。
 *
 * 用户（2026-10-28）：「我的 tg 机器人已经和 pt-tools 绑定了，但是却没有显示」。生产上机器人在线、通知照常发到默认 Chat ID，
 * 而账号在 10-21 发过 /unbind，命令要重新绑定才能用 —— 页面只列绑定行，看不出这三件事的区别。
 */

var (
	// ErrBindUnsupportedChannel 表示这个通道不收命令（Webhook、钉钉…），没有绑定这回事。
	ErrBindUnsupportedChannel = errors.New("这个通道不收命令，不用绑定")
	// ErrBindNotInWhitelist 表示账号不在通道的名单里：名单外的人发的消息通道直接丢掉，绑定了也用不了。
	ErrBindNotInWhitelist = errors.New("这个账号不在通道的名单里：先在通道设置里把它加进管理员或允许名单")
	// ErrAlreadyBound 表示这个账号在这个通道上已经绑定了。
	ErrAlreadyBound = errors.New("这个账号在这个通道上已经绑定了")
)

// commandChannel 报告这种通道能不能收命令（只有它们需要绑定）。
func commandChannel(channelType string) bool {
	return channelType == "telegram" || channelType == "qq_onebot"
}

// 账号在通道名单里的身份。
const (
	RoleAdmin   = "admin"
	RoleAllowed = "allowed"
)

// 没有指定收件人的通知从哪来的收件人（见 defaultRecipient）。
const (
	NotifyViaDefaultChat = "default_chat_id"
	NotifyViaAdmin       = "admin"
	NotifyViaAllowed     = "allowed"
	NotifyViaBinding     = "binding"
)

// ChannelUserActivity 是一个账号最近一次和机器人的往来（操作审计里最近的一条命令）。
type ChannelUserActivity struct {
	Command string    `json:"command"`
	Result  string    `json:"result"`
	At      time.Time `json:"at"`
}

// ChannelBindingUser 是通道名单里的一个账号，或者已经绑定、却不在名单里的账号（这样的账号发的消息通道会丢掉）。
type ChannelBindingUser struct {
	UserID string `json:"user_id"`
	// Roles 是它在名单里的身份（admin、allowed）；不在名单里时为空数组
	Roles     []string `json:"roles"`
	Bound     bool     `json:"bound"`
	BindingID uint     `json:"binding_id,omitempty"`
	// Last 是它最近一次和机器人的往来；没有记录时没有
	Last *ChannelUserActivity `json:"last,omitempty"`
}

// ChannelBindingView 是一个能收命令的通道在绑定页上的样子。
type ChannelBindingView struct {
	ConfID      uint   `json:"conf_id"`
	Name        string `json:"name"`
	ChannelType string `json:"channel_type"`
	Enabled     bool   `json:"enabled"`
	// RuntimeState 是通道实例此刻的状态（connected、running、error、disabled），由接口层补上；问不到时为空
	RuntimeState string `json:"runtime_state,omitempty"`
	// NotifyTarget 是没有指定收件人的通知发给谁；没有收件人时为空，NotifyError 写原因
	NotifyTarget string `json:"notify_target,omitempty"`
	// NotifyVia 是这个收件人从哪来：default_chat_id、admin、allowed、binding
	NotifyVia   string               `json:"notify_via,omitempty"`
	NotifyError string               `json:"notify_error,omitempty"`
	Users       []ChannelBindingUser `json:"users"`
	// ConfigError 是读不出通道配置时的原因（这时 Users 只有已经绑定的账号）
	ConfigError string `json:"config_error,omitempty"`
}

// channelLists 是通道配置里的名单与默认 Chat ID（只有 Telegram 有）。
type channelLists struct {
	admins      []string
	allowed     []string
	defaultChat string
}

func readChannelLists(row models.NotificationConf) (channelLists, error) {
	var out channelLists
	switch row.ChannelType {
	case "qq_onebot":
		var cfg struct {
			AdminQQUsers   []int64 `json:"admin_qq_users"`
			AllowedQQUsers []int64 `json:"allowed_qq_users"`
		}
		if err := decryptConfigJSON(row.ConfigJSON, &cfg); err != nil {
			return out, err
		}
		for _, id := range cfg.AdminQQUsers {
			out.admins = append(out.admins, strconv.FormatInt(id, 10))
		}
		for _, id := range cfg.AllowedQQUsers {
			out.allowed = append(out.allowed, strconv.FormatInt(id, 10))
		}
	case "telegram":
		var cfg map[string]json.RawMessage
		if err := decryptConfigJSON(row.ConfigJSON, &cfg); err != nil {
			return out, err
		}
		var err error
		if raw := cfg["admin_users"]; len(raw) > 0 {
			if out.admins, err = rawStringOrInt64Slice(raw); err != nil {
				return out, fmt.Errorf("解析 telegram admin_users 失败: %w", err)
			}
		}
		if raw := cfg["allowed_users"]; len(raw) > 0 {
			if out.allowed, err = rawStringOrInt64Slice(raw); err != nil {
				return out, fmt.Errorf("解析 telegram allowed_users 失败: %w", err)
			}
		}
		if raw := cfg["default_chat_id"]; len(raw) > 0 {
			if out.defaultChat, err = rawStringOrInt64(raw); err != nil {
				return out, fmt.Errorf("解析 telegram default_chat_id 失败: %w", err)
			}
		}
	}
	return out, nil
}

// rolesOf 是账号在名单里的身份。
func (l channelLists) rolesOf(userID string) []string {
	roles := []string{}
	if slices.Contains(l.admins, userID) {
		roles = append(roles, RoleAdmin)
	}
	if slices.Contains(l.allowed, userID) {
		roles = append(roles, RoleAllowed)
	}
	return roles
}

// ListChannels 列出能收命令的通道（按编号），每个带上名单里的账号与绑定情况、最近一次往来，以及通知发给谁。
func (s *bindingService) ListChannels(ctx context.Context) ([]ChannelBindingView, error) {
	var confs []models.NotificationConf
	if err := s.db.WithContext(ctx).Where("channel_type IN ?", []string{"telegram", "qq_onebot"}).
		Order("id ASC").Find(&confs).Error; err != nil {
		return nil, fmt.Errorf("读取通知通道失败: %w", err)
	}
	var bindings []models.ChannelBinding
	if err := s.db.WithContext(ctx).Order("id ASC").Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	out := make([]ChannelBindingView, 0, len(confs))
	for _, conf := range confs {
		v := ChannelBindingView{ConfID: conf.ID, Name: conf.Name, ChannelType: conf.ChannelType, Enabled: conf.Enabled, Users: []ChannelBindingUser{}}
		lists, err := readChannelLists(conf)
		if err != nil {
			v.ConfigError = err.Error()
		}
		seen := map[string]int{}
		add := func(id string) {
			if id == "" {
				return
			}
			if _, ok := seen[id]; ok {
				return
			}
			seen[id] = len(v.Users)
			v.Users = append(v.Users, ChannelBindingUser{UserID: id, Roles: lists.rolesOf(id)})
		}
		for _, id := range lists.admins {
			add(id)
		}
		for _, id := range lists.allowed {
			add(id)
		}
		for _, b := range bindings {
			if b.NotificationConfID != conf.ID {
				continue
			}
			add(b.ChannelUserID)
			u := &v.Users[seen[b.ChannelUserID]]
			if !u.Bound {
				u.Bound, u.BindingID = true, b.ID
			}
		}
		for i := range v.Users {
			v.Users[i].Last = s.lastActivity(ctx, conf.ChannelType, v.Users[i].UserID)
		}
		if err == nil {
			target, rerr := defaultRecipient(ctx, s.db, conf)
			switch {
			case rerr != nil:
				v.NotifyError = rerr.Error()
			case target == lists.defaultChat && target != "":
				v.NotifyTarget, v.NotifyVia = target, NotifyViaDefaultChat
			case slices.Contains(lists.admins, target):
				v.NotifyTarget, v.NotifyVia = target, NotifyViaAdmin
			case conf.ChannelType == "qq_onebot" && slices.Contains(lists.allowed, target):
				// 只有 QQ 会退到允许名单（Telegram 没有管理员时直接用绑定的账号）
				v.NotifyTarget, v.NotifyVia = target, NotifyViaAllowed
			case target != "":
				v.NotifyTarget, v.NotifyVia = target, NotifyViaBinding
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// lastActivity 是这个账号最近一次和机器人的往来：操作审计里这个通道类型、这个账号最近的一条命令
// （没绑定时的拒绝、/bind 本身记的是通道 0，所以不按通道编号筛）。读不到时为 nil。
func (s *bindingService) lastActivity(ctx context.Context, channelType, userID string) *ChannelUserActivity {
	var row models.ActionAudit
	err := s.db.WithContext(ctx).Select("command", "result", "created_at").
		Where("channel_type = ? AND channel_user_id = ? AND command NOT LIKE ?", channelType, userID, NotifyCommandPrefix+"%").
		Order("created_at DESC, id DESC").First(&row).Error
	if err != nil {
		return nil
	}
	return &ChannelUserActivity{Command: row.Command, Result: row.Result, At: row.CreatedAt}
}

// BindDirect 在网页上直接绑定通道名单里的账号，效果与用绑定码绑定相同（能执行全部命令，含管理员命令）。
// 只认名单里的账号：名单外的人发的消息通道直接丢掉，绑定了也用不了；名单只有登录网页的人改得了。
// 通道存在时，拒绝的结果里也有通道编号与类型（审计用）。
func (s *bindingService) BindDirect(ctx context.Context, confID uint, channelUserID string) (BindingDTO, error) {
	if channelUserID == "" {
		return BindingDTO{}, ErrBindNotInWhitelist
	}
	var conf models.NotificationConf
	if err := s.db.WithContext(ctx).First(&conf, confID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return BindingDTO{}, ErrConfNotFound
		}
		return BindingDTO{}, fmt.Errorf("查询通知通道失败: %w", err)
	}
	if !commandChannel(conf.ChannelType) {
		return BindingDTO{}, ErrBindUnsupportedChannel
	}
	// 拒绝时也带上通道，接口层要把这次尝试记进审计
	refused := BindingDTO{ConfID: conf.ID, ChannelType: conf.ChannelType, ChannelUserID: channelUserID}
	lists, err := readChannelLists(conf)
	if err != nil {
		return refused, err
	}
	if len(lists.rolesOf(channelUserID)) == 0 {
		return refused, ErrBindNotInWhitelist
	}
	var binding models.ChannelBinding
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if qerr := tx.Model(&models.ChannelBinding{}).
			Where("notification_conf_id = ? AND channel_user_id = ?", conf.ID, channelUserID).
			Count(&n).Error; qerr != nil {
			return fmt.Errorf("查询绑定失败: %w", qerr)
		}
		if n > 0 {
			return ErrAlreadyBound
		}
		binding = models.ChannelBinding{
			NotificationConfID: conf.ID,
			ChannelType:        conf.ChannelType,
			ChannelUserID:      channelUserID,
			PtAdmin:            true,
			Allowed:            true,
			Label:              "网页绑定",
			ReplyLang:          defaultReplyLang,
		}
		if cerr := tx.Create(&binding).Error; cerr != nil {
			return fmt.Errorf("create channel binding: %w", cerr)
		}
		return nil
	})
	if err != nil {
		return refused, err
	}
	return toBindingDTO(binding), nil
}
