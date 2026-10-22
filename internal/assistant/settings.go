package assistant

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 设置的默认值与取值范围。
const (
	DefaultMaxSteps       = 8
	DefaultConfirmMinutes = 10
	DefaultRetentionDays  = 30
	maxStepsLimit         = 20
	maxConfirmMinutes     = 60
	maxRetentionDays      = 3650
	maxKeyLen             = 512
	// minKeyLen 是 API Key 的最短长度：回答里回显的 Key 按原文替换，太短的串会误伤正常文字
	minKeyLen     = 8
	maxModelLen   = 128
	maxBaseURLLen = 512
	// DefaultCompactPercent 是默认在上下文用到窗口的百分之几时自动压缩
	DefaultCompactPercent = 80
	minCompactPercent     = 50
	maxCompactPercent     = 95
	// 上下文窗口能填的范围（token）；0 是自动
	minContextWindow = 4096
	maxContextWindow = 10_000_000
)

// Settings 是界面上看到的设置：API Key 只写不读（只回有没有），代理地址里的密码换成 ***。
type Settings struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	// HasAPIKey 报告存没存 API Key
	HasAPIKey      bool   `json:"has_api_key"`
	ProxyURL       string `json:"proxy_url"`
	ChatOps        bool   `json:"chatops"`
	MaxSteps       int    `json:"max_steps"`
	ConfirmMinutes int    `json:"confirm_minutes"`
	// RetentionDays 是会话保留几天，0 表示不清理
	RetentionDays int `json:"retention_days"`
	// ContextWindow 是上下文窗口（token），0 是自动
	ContextWindow int `json:"context_window"`
	// AutoCompact 为真时上下文用到 CompactPercent% 自动压缩
	AutoCompact    bool `json:"auto_compact"`
	CompactPercent int  `json:"compact_percent"`
	// 语音输入（Voltip）：访问令牌只写不读（只回有没有）
	VoiceBaseURL  string `json:"voice_base_url"`
	HasVoiceToken bool   `json:"has_voice_token"`
	VoiceModel    string `json:"voice_model"`
	VoiceLanguage string `json:"voice_language"`
}

// SettingsInput 是保存设置的参数。APIKey、ProxyURL 为 nil 时不改，为空串时清除；ProxyURL 与读到的（密码换成 *** 的）
// 地址相同时也不改。改了接口地址又没给新的 API Key 时，存着的 Key 清掉：Key 不会发到保存它时以外的地址。
// MaxSteps、ConfirmMinutes 为 0 时用默认值；RetentionDays、ContextWindow、AutoCompact、CompactPercent 为 nil 时不改
// （ContextWindow 为 0 是自动，CompactPercent 为 0 是默认的 80）。
type SettingsInput struct {
	Enabled        bool    `json:"enabled"`
	Provider       string  `json:"provider"`
	BaseURL        string  `json:"base_url"`
	Model          string  `json:"model"`
	APIKey         *string `json:"api_key,omitempty"`
	ProxyURL       *string `json:"proxy_url,omitempty"`
	ChatOps        bool    `json:"chatops"`
	MaxSteps       int     `json:"max_steps"`
	ConfirmMinutes int     `json:"confirm_minutes"`
	RetentionDays  *int    `json:"retention_days,omitempty"`
	ContextWindow  *int    `json:"context_window,omitempty"`
	AutoCompact    *bool   `json:"auto_compact,omitempty"`
	CompactPercent *int    `json:"compact_percent,omitempty"`
	// 语音输入：都是 nil 时不改。VoiceToken 为空串时清除；改了地址又没给新令牌时存着的令牌清掉（同 API Key）
	VoiceBaseURL  *string `json:"voice_base_url,omitempty"`
	VoiceToken    *string `json:"voice_token,omitempty"`
	VoiceModel    *string `json:"voice_model,omitempty"`
	VoiceLanguage *string `json:"voice_language,omitempty"`
}

// llmConfig 是调模型用的设置（明文，只在内存里）。
type llmConfig struct {
	BaseURL, Model, APIKey, Proxy string
}

// effective 是一次运行用的设置。
type effective struct {
	llm            llmConfig
	maxSteps       int
	confirmMinutes int
	chatOps        bool
	// contextWindow 是设置里填的上下文窗口（0 是自动，见 windowFor）
	contextWindow  int
	autoCompact    bool
	compactPercent int
}

func (s *Service) loadSettings(ctx context.Context) (models.AssistantSetting, bool, error) {
	var rows []models.AssistantSetting
	if err := s.db(ctx).Where("id = 1").Limit(1).Find(&rows).Error; err != nil {
		return models.AssistantSetting{}, false, fmt.Errorf("读取 AI 助手设置失败: %w", err)
	}
	if len(rows) == 0 {
		return models.AssistantSetting{ID: 1, RetentionDays: DefaultRetentionDays}, false, nil
	}
	return rows[0], true, nil
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// maskProxy 把代理地址里的密码换成 ***。
func maskProxy(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return strings.Replace(u.String(), "%2A%2A%2A", "***", 1)
}

func (s *Service) view(row models.AssistantSetting) Settings {
	out := Settings{
		Enabled: row.Enabled, Provider: row.Provider, BaseURL: row.BaseURL, Model: row.Model,
		HasAPIKey: row.APIKeyEncrypted != "", ChatOps: row.ChatOps,
		MaxSteps: orDefault(row.MaxSteps, DefaultMaxSteps), ConfirmMinutes: orDefault(row.ConfirmMinutes, DefaultConfirmMinutes),
		RetentionDays: row.RetentionDays, ContextWindow: row.ContextWindow, AutoCompact: !row.AutoCompactOff,
		CompactPercent: orDefault(row.CompactPercent, DefaultCompactPercent),
		VoiceBaseURL:   row.VoiceBaseURL, HasVoiceToken: row.VoiceTokenEncrypted != "", VoiceModel: row.VoiceModel,
		VoiceLanguage: row.VoiceLanguage,
	}
	if row.ProxyEncrypted != "" {
		if p, err := s.cfg.Cipher.Decrypt(row.ProxyEncrypted); err == nil {
			out.ProxyURL = maskProxy(p)
		}
	}
	return out
}

// Settings 读设置。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, _, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return s.view(row), nil
}

// normalizeBaseURL 校验接口地址：http 或 https，有主机，不带用户信息、查询串与片段；去掉末尾的 /。
func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > maxBaseURLLen {
		return "", fmt.Errorf("%w: 接口地址太长", ErrInvalid)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("%w: 接口地址要以 http:// 或 https:// 开头", ErrInvalid)
	}
	if u.User != nil {
		return "", fmt.Errorf("%w: 接口地址里不要带用户名或密码（API Key 填在下面）", ErrInvalid)
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(raw, "?") {
		return "", fmt.Errorf("%w: 接口地址里不要带查询参数", ErrInvalid)
	}
	return strings.TrimRight(raw, "/"), nil
}

func hasSpace(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0
}

// merged 是按输入改过的一行设置（还没存）。stored 是现在存着的那一行。
func (s *Service) merged(stored models.AssistantSetting, in SettingsInput) (models.AssistantSetting, error) {
	row := stored
	row.ID = 1
	base, err := normalizeBaseURL(in.BaseURL)
	if err != nil {
		return row, err
	}
	model := strings.TrimSpace(in.Model)
	if len(model) > maxModelLen || hasSpace(model) {
		return row, fmt.Errorf("%w: 模型名不对（不能有空格，最长 %d 个字符）", ErrInvalid, maxModelLen)
	}
	provider := strings.TrimSpace(in.Provider)
	if len(provider) > 32 || hasSpace(provider) {
		return row, fmt.Errorf("%w: 服务商不对", ErrInvalid)
	}
	if in.Enabled && (base == "" || model == "") {
		return row, fmt.Errorf("%w: 启用以前要填接口地址与模型", ErrInvalid)
	}
	steps, minutes := orDefault(in.MaxSteps, DefaultMaxSteps), orDefault(in.ConfirmMinutes, DefaultConfirmMinutes)
	if steps > maxStepsLimit {
		return row, fmt.Errorf("%w: 步数上限在 1 到 %d 之间", ErrInvalid, maxStepsLimit)
	}
	if minutes > maxConfirmMinutes {
		return row, fmt.Errorf("%w: 确认等待在 1 到 %d 分钟之间", ErrInvalid, maxConfirmMinutes)
	}
	if in.RetentionDays != nil {
		if *in.RetentionDays < 0 || *in.RetentionDays > maxRetentionDays {
			return row, fmt.Errorf("%w: 保留天数在 0 到 %d 之间（0 表示不清理）", ErrInvalid, maxRetentionDays)
		}
		row.RetentionDays = *in.RetentionDays
	}
	if in.ContextWindow != nil {
		if w := *in.ContextWindow; w != 0 && (w < minContextWindow || w > maxContextWindow) {
			return row, fmt.Errorf("%w: 上下文窗口在 %d 到 %d 之间（0 表示自动）", ErrInvalid, minContextWindow, maxContextWindow)
		}
		row.ContextWindow = *in.ContextWindow
	}
	if in.CompactPercent != nil {
		if p := *in.CompactPercent; p != 0 && (p < minCompactPercent || p > maxCompactPercent) {
			return row, fmt.Errorf("%w: 自动压缩的阈值在 %d%% 到 %d%% 之间", ErrInvalid, minCompactPercent, maxCompactPercent)
		}
		row.CompactPercent = *in.CompactPercent
	}
	if in.AutoCompact != nil {
		row.AutoCompactOff = !*in.AutoCompact
	}
	if err := s.mergeVoice(&row, stored, in); err != nil {
		return row, err
	}
	if base != stored.BaseURL && in.APIKey == nil {
		// 换了地址：存着的 Key 不跟过去
		row.APIKeyEncrypted = ""
	}
	if in.APIKey != nil {
		key := strings.TrimSpace(*in.APIKey)
		switch {
		case key == "":
			row.APIKeyEncrypted = ""
		case len(key) < minKeyLen || len(key) > maxKeyLen || hasSpace(key):
			return row, fmt.Errorf("%w: API Key 格式不对（%d 到 %d 个字符，不能有空格；本机的 Ollama 不要 Key 就留空）",
				ErrInvalid, minKeyLen, maxKeyLen)
		default:
			enc, err := s.cfg.Cipher.Encrypt(key)
			if err != nil {
				return row, fmt.Errorf("加密 API Key 失败: %w", err)
			}
			row.APIKeyEncrypted = enc
		}
	}
	if in.ProxyURL != nil {
		proxy := strings.TrimSpace(*in.ProxyURL)
		switch {
		case proxy == "":
			row.ProxyEncrypted = ""
		case proxy == s.view(stored).ProxyURL:
			// 界面原样提交了密码换成 *** 的地址：不改
		default:
			if _, err := tmdb.ParseProxyURL(proxy); err != nil {
				return row, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
			}
			enc, err := s.cfg.Cipher.Encrypt(proxy)
			if err != nil {
				return row, fmt.Errorf("加密代理地址失败: %w", err)
			}
			row.ProxyEncrypted = enc
		}
	}
	row.Enabled, row.Provider, row.BaseURL, row.Model = in.Enabled, provider, base, model
	row.ChatOps, row.MaxSteps, row.ConfirmMinutes = in.ChatOps, steps, minutes
	return row, nil
}

// SaveSettings 保存设置。
func (s *Service) SaveSettings(ctx context.Context, in SettingsInput) (Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	stored, _, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.merged(stored, in)
	if err != nil {
		return Settings{}, err
	}
	row.UpdatedAt = s.now()
	if err := s.db(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled", "provider", "base_url", "model", "api_key_encrypted", "proxy_encrypted", "chat_ops",
			"max_steps", "confirm_minutes", "retention_days", "context_window", "compact_percent", "auto_compact_off",
			"voice_base_url", "voice_token_encrypted", "voice_model", "voice_language", "updated_at",
		}),
	}).Create(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("保存 AI 助手设置失败: %w", err)
	}
	return s.view(row), nil
}

// decrypted 把一行设置换成调模型用的明文设置。
func (s *Service) decrypted(row models.AssistantSetting) (llmConfig, error) {
	c := llmConfig{BaseURL: row.BaseURL, Model: row.Model}
	var err error
	if row.APIKeyEncrypted != "" {
		if c.APIKey, err = s.cfg.Cipher.Decrypt(row.APIKeyEncrypted); err != nil {
			return c, errors.New("解密 API Key 失败：密钥换过的话要重新填 API Key")
		}
	}
	if row.ProxyEncrypted != "" {
		if c.Proxy, err = s.cfg.Cipher.Decrypt(row.ProxyEncrypted); err != nil {
			return c, errors.New("解密代理地址失败：密钥换过的话要重新填代理地址")
		}
	}
	return c, nil
}

// effectiveSettings 是一次运行用的设置；没有启用或没有配置好时返回 ErrDisabled。
func (s *Service) effectiveSettings(ctx context.Context) (effective, error) {
	row, _, err := s.loadSettings(ctx)
	if err != nil {
		return effective{}, err
	}
	if !row.Enabled || row.BaseURL == "" || row.Model == "" {
		return effective{}, ErrDisabled
	}
	llm, err := s.decrypted(row)
	if err != nil {
		return effective{}, err
	}
	return effective{
		llm: llm, maxSteps: orDefault(row.MaxSteps, DefaultMaxSteps),
		confirmMinutes: orDefault(row.ConfirmMinutes, DefaultConfirmMinutes), chatOps: row.ChatOps,
		contextWindow: row.ContextWindow, autoCompact: !row.AutoCompactOff,
		compactPercent: orDefault(row.CompactPercent, DefaultCompactPercent),
	}, nil
}

// Status 是给各入口看的状态：启用没有、用的模型、这个主体能不能确认改动类操作。不含 Key 与接口地址。
type Status struct {
	Enabled    bool   `json:"enabled"`
	Model      string `json:"model"`
	CanConfirm bool   `json:"can_confirm"`
	// ChatOps 报告通知渠道里的非命令消息交不交给助手
	ChatOps bool `json:"chatops"`
	// Voice 报告配了语音输入（Voltip 的地址与访问令牌都有）。只给网页对话页：识别接口只认网页登录，App API 不回这一项
	Voice bool `json:"voice,omitempty"`
}

// Status 读状态。
func (s *Service) Status(ctx context.Context, a Actor) (Status, error) {
	row, _, err := s.loadSettings(ctx)
	if err != nil {
		return Status{}, err
	}
	enabled := row.Enabled && row.BaseURL != "" && row.Model != ""
	st := Status{
		Enabled: enabled, CanConfirm: enabled && a.CanWrite, ChatOps: enabled && row.ChatOps,
		Voice: enabled && row.VoiceBaseURL != "" && row.VoiceTokenEncrypted != "",
	}
	if enabled {
		st.Model = row.Model
	}
	return st, nil
}

// TestResult 是测试连接的结果。
type TestResult struct {
	Model     string `json:"model"`
	Reply     string `json:"reply"`
	LatencyMs int64  `json:"latency_ms"`
}

// TestSettings 按输入（还没保存的也行）发一次很短的对话，看接口地址、模型与 Key 能不能用。
// 输入没给 Key 时用存着的 Key，但接口地址变了就不带：存着的 Key 只发到保存它时的地址。
func (s *Service) TestSettings(ctx context.Context, in SettingsInput) (TestResult, error) {
	stored, _, err := s.loadSettings(ctx)
	if err != nil {
		return TestResult{}, err
	}
	in.Enabled = true
	row, err := s.merged(stored, in)
	if err != nil {
		return TestResult{}, err
	}
	llm, err := s.decrypted(row)
	if err != nil {
		return TestResult{}, err
	}
	return s.ping(ctx, llm)
}
