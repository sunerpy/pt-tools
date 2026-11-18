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
	// ContextWindow 是模型的上下文窗口（token），0 表示自动（按服务商的模型列表与内置的表）
	ContextWindow int `gorm:"not null;default:0"`
	// CompactPercent 是上下文用到窗口的百分之几时自动压缩，0 表示默认（80）
	CompactPercent int `gorm:"not null;default:0"`
	// AutoCompactOff 为真时不自动压缩（零值是开着）
	AutoCompactOff bool `gorm:"not null;default:false"`
	// 语音输入：Voltip 本机服务（OpenAI 兼容的 /audio/transcriptions）的地址、访问令牌（加密存）、模型与识别语言。
	// 地址与令牌都有时对话页上才有麦克风按钮
	VoiceBaseURL        string `gorm:"size:512;not null;default:''"`
	VoiceTokenEncrypted string `gorm:"type:text;not null;default:''"`
	VoiceModel          string `gorm:"size:128;not null;default:''"`
	VoiceLanguage       string `gorm:"size:16;not null;default:''"`
	UpdatedAt           time.Time
}

// TableName 指定表名。
func (AssistantSetting) TableName() string { return "assistant_settings" }

// AssistantConversation 是一个会话。Owner 是带命名空间的属主（web:<用户名>、token:<编号>、device:<编号>、chatops:<绑定编号>），
// 查询一律按属主过滤。
type AssistantConversation struct {
	ID    uint   `gorm:"primaryKey"`
	Owner string `gorm:"size:64;not null;default:'';index"`
	Entry string `gorm:"size:16;not null;default:''"`
	Title string `gorm:"size:128;not null;default:''"`
	// Summary 是压缩以后前面那段对话的摘要（脱敏以后的）；SummaryUntil 是摘要到哪条消息为止（含），
	// 交给模型的历史从它后面开始，摘要放在系统提示里。都是零值时没有压缩过
	Summary      string `gorm:"type:text;not null;default:''"`
	SummaryUntil uint   `gorm:"not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time `gorm:"index"`
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
	// Reasoning 是模型的推理内容（思考过程）：网页与 App 上脱敏以后给人看；ReasoningField 为空时还要回传给模型
	// （DeepSeek 等的思考模式带工具时要求回传）
	Reasoning string `gorm:"type:text;not null;default:''"`
	// ReasoningField 是推理内容从哪儿来的：空是 reasoning_content 字段；reasoning 是同名字段（OpenRouter、Groq、Ollama 等），
	// think 是正文开头的 <think>…</think>。后两种只给人看、不回传：不认 reasoning_content 的服务会拒绝整个请求
	ReasoningField string `gorm:"size:16;not null;default:''"`
	// Model、Effort 是这一轮的模型与推理等级：用户消息是发的时候选的，助手消息是实际请求的；空的是设置里的模型、不指定推理等级
	Model  string `gorm:"size:128;not null;default:''"`
	Effort string `gorm:"size:16;not null;default:''"`
	// ToolCalls 是助手消息里的工具调用（JSON 数组：编号、工具名、规范化脱敏以后的参数）
	ToolCalls  string `gorm:"type:text;not null;default:''"`
	ToolName   string `gorm:"size:64;not null;default:''"`
	ToolCallID string `gorm:"size:128;not null;default:''"`
	IsError    bool   `gorm:"not null;default:false"`
	// PromptTokens、CompletionTokens 是生成这条消息的那次模型调用用掉的 token（服务商报的；没报时为 0）。
	// 助手消息是这一步的请求；压缩时那条提示记的是写摘要的那次调用
	PromptTokens     int64 `gorm:"not null;default:0"`
	CompletionTokens int64 `gorm:"not null;default:0"`
	CreatedAt        time.Time
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
	// Preview 是建这条记录以前写工具给的预览（mcp.Preview 的 JSON：改以前、改以后与影响），没有时为空
	Preview string `gorm:"type:text;not null;default:''"`
	Status  string `gorm:"size:16;not null;default:'';index"`
	// DecidedBy 是谁定的：属主，或者 timeout、cancel、restart、shutdown
	DecidedBy string `gorm:"size:64;not null;default:''"`
	DecidedAt *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}

// TableName 指定表名。
func (AssistantConfirmation) TableName() string { return "assistant_confirmations" }

// AI 助手附件的种类。
const (
	AssistantAttachImage = "image"
	AssistantAttachText  = "text"
)

// AssistantAttachment 是用户随消息发的附件：图片原样存，文本文件存脱敏以后的内容（交给模型的就是这一份）。
// MessageID 为 0 的是传了还没发的，只有上传的属主能看、能删，一天以后清掉；发出去以后跟着会话删。
type AssistantAttachment struct {
	ID             uint   `gorm:"primaryKey"`
	Owner          string `gorm:"size:64;not null;default:'';index"`
	ConversationID uint   `gorm:"not null;default:0;index"`
	MessageID      uint   `gorm:"not null;default:0;index"`
	Name           string `gorm:"size:255;not null;default:''"`
	MIME           string `gorm:"column:mime;size:64;not null;default:''"`
	// Kind 是 image 或 text
	Kind string `gorm:"size:8;not null;default:''"`
	// Size 是上传的文件大小（字节）
	Size      int64  `gorm:"not null;default:0"`
	Data      []byte `gorm:"type:blob"`
	CreatedAt time.Time
}

// TableName 指定表名。
func (AssistantAttachment) TableName() string { return "assistant_attachments" }
