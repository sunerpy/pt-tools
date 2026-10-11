package models

import "time"

// AI 助手（路线图 M19）会话的入口。
const (
	AssistantEntryWeb     = "web"
	AssistantEntryApp     = "app"
	AssistantEntryChatOps = "chatops"
)

// AI 助手消息的角色。notice 是 pt-tools 自己写的提示（出错、已停止），不交给模型。
const (
	AssistantRoleUser      = "user"
	AssistantRoleAssistant = "assistant"
	AssistantRoleTool      = "tool"
	AssistantRoleNotice    = "notice"
)

// AI 助手待确认操作的状态：pending 只能经条件更新变成后面三种之一，变了就不再改。
const (
	AssistantConfirmPending  = "pending"
	AssistantConfirmApproved = "approved"
	AssistantConfirmDenied   = "denied"
	AssistantConfirmExpired  = "expired"
)

// AssistantSetting 是 AI 助手的设置，只有一行（ID 为 1）；没有这一行时按默认值（关闭）。列的默认值一律是零值。
type AssistantSetting struct {
	ID      uint `gorm:"primaryKey"`
	Enabled bool `gorm:"not null;default:false"`
	// Provider 是界面上选的服务商预设（deepseek、openai、custom 等），只给界面用
	Provider string `gorm:"size:32;not null;default:''"`
	// BaseURL 是 OpenAI 兼容接口的地址（到 /chat/completions 之前）
	BaseURL         string `gorm:"size:512;not null;default:''"`
	Model           string `gorm:"size:128;not null;default:''"`
	APIKeyEncrypted string `gorm:"type:text;not null;default:''"`
	ProxyEncrypted  string `gorm:"type:text;not null;default:''"`
	// ChatOps 为真时，通知渠道里的非命令消息交给助手
	ChatOps  bool `gorm:"column:chat_ops;not null;default:false"`
	MaxSteps int  `gorm:"not null;default:0"`
	// ConfirmMinutes 是待确认操作等几分钟
	ConfirmMinutes int `gorm:"not null;default:0"`
	// RetentionDays 是会话保留几天，0 表示不清理
	RetentionDays int `gorm:"not null;default:0"`
	UpdatedAt     time.Time
}

// TableName 指定表名。
func (AssistantSetting) TableName() string { return "assistant_settings" }

// AssistantConversation 是一个会话。Owner 是带命名空间的属主（web:<用户名>、token:<编号>、device:<编号>、chatops:<绑定编号>），
// 查询一律按属主过滤。
type AssistantConversation struct {
	ID        uint   `gorm:"primaryKey"`
	Owner     string `gorm:"size:64;not null;default:'';index"`
	Entry     string `gorm:"size:16;not null;default:''"`
	Title     string `gorm:"size:128;not null;default:''"`
	CreatedAt time.Time
	UpdatedAt time.Time `gorm:"index"`
}

// TableName 指定表名。
func (AssistantConversation) TableName() string { return "assistant_conversations" }

// AssistantMessage 是会话里的一条消息，按 ID 排序。文字与工具参数都是脱敏以后的；工具结果来自 App API（本来就不带凭证）。
type AssistantMessage struct {
	ID             uint   `gorm:"primaryKey"`
	ConversationID uint   `gorm:"not null;default:0;index"`
	RunID          string `gorm:"size:32;not null;default:''"`
	Role           string `gorm:"size:16;not null;default:''"`
	Content        string `gorm:"type:text;not null;default:''"`
	// Reasoning 是模型的推理内容：只回传给模型（DeepSeek 等的思考模式带工具时要求回传），不给任何入口看
	Reasoning string `gorm:"type:text;not null;default:''"`
	// ToolCalls 是助手消息里的工具调用（JSON 数组：编号、工具名、规范化脱敏以后的参数）
	ToolCalls  string `gorm:"type:text;not null;default:''"`
	ToolName   string `gorm:"size:64;not null;default:''"`
	ToolCallID string `gorm:"size:128;not null;default:''"`
	IsError    bool   `gorm:"not null;default:false"`
	CreatedAt  time.Time
}

// TableName 指定表名。
func (AssistantMessage) TableName() string { return "assistant_messages" }

// AssistantConfirmation 是一个等人确认的写操作。结论只经 WHERE status='pending' 的条件更新得出，改到一行的才算数。
type AssistantConfirmation struct {
	ID             uint   `gorm:"primaryKey"`
	ConversationID uint   `gorm:"not null;default:0;index"`
	RunID          string `gorm:"size:32;not null;default:''"`
	ToolCallID     string `gorm:"size:128;not null;default:''"`
	ToolName       string `gorm:"size:64;not null;default:''"`
	// Arguments 是规范化脱敏以后的参数（JSON），确认后按它调用工具
	Arguments string `gorm:"type:text;not null;default:''"`
	Status    string `gorm:"size:16;not null;default:'';index"`
	// DecidedBy 是谁定的：属主，或者 timeout、cancel、restart、shutdown
	DecidedBy string `gorm:"size:64;not null;default:''"`
	DecidedAt *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// TableName 指定表名。
func (AssistantConfirmation) TableName() string { return "assistant_confirmations" }
