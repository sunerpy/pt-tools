package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

// 语音输入：对话页录一段话（16 kHz 单声道 WAV），pt-tools 交给 Voltip 的本机服务识别成文字，填进输入框。
// Voltip 的本机服务是 OpenAI 兼容的 POST /v1/audio/transcriptions（multipart：file、model、language），
// 每个请求带访问令牌（Authorization: Bearer），回 {"text": "…"}。令牌只在 pt-tools 后端，浏览器看不到。

// MaxVoiceBytes 是一段录音最大多少字节：16 kHz 单声道 16 位约 10 分钟（页面最长录 5 分钟）。
const MaxVoiceBytes = 20 << 20

// 等 Voltip 的上限：识别（加 AI 润色）一段 5 分钟的口述实测十几秒，长的、本地模型慢的要更久；测试连接只等一会儿。
const (
	voiceTimeout     = 3 * time.Minute
	voiceTestTimeout = 15 * time.Second
	// DefaultVoiceModel 是 Voltip 的默认处理方式（应用里设的预设与场景）
	DefaultVoiceModel = "voltip"
)

// ErrVoiceDisabled 是没有配语音输入（Voltip 的地址或访问令牌没填）。
var ErrVoiceDisabled = errors.New("没有配语音输入：在「AI 助手设置」里填 Voltip 的地址与访问令牌")

var voiceLangRe = regexp.MustCompile(`^(auto|[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})?)$`)

// voiceConfig 是调 Voltip 用的设置（明文，只在内存里）。
type voiceConfig struct {
	BaseURL, Token, Model, Language string
}

// mergeVoice 按输入改语音输入的几项（都是 nil 的不改）。改了地址又没给新令牌时清掉存着的：令牌只发到保存它时的地址。
func (s *Service) mergeVoice(row *models.AssistantSetting, stored models.AssistantSetting, in SettingsInput) error {
	if in.VoiceBaseURL != nil {
		base, err := normalizeBaseURL(*in.VoiceBaseURL)
		if err != nil {
			return fmt.Errorf("%w（Voltip 的地址，例如 http://127.0.0.1:47840/v1）", err)
		}
		if base != stored.VoiceBaseURL && in.VoiceToken == nil {
			row.VoiceTokenEncrypted = ""
		}
		row.VoiceBaseURL = base
	}
	if in.VoiceToken != nil {
		tok := strings.TrimSpace(*in.VoiceToken)
		switch {
		case tok == "":
			row.VoiceTokenEncrypted = ""
		case len(tok) < minKeyLen || len(tok) > maxKeyLen || hasSpace(tok):
			return fmt.Errorf("%w: Voltip 的访问令牌格式不对（%d 到 %d 个字符，不能有空格）", ErrInvalid, minKeyLen, maxKeyLen)
		default:
			enc, err := s.cfg.Cipher.Encrypt(tok)
			if err != nil {
				return fmt.Errorf("加密访问令牌失败: %w", err)
			}
			row.VoiceTokenEncrypted = enc
		}
	}
	if in.VoiceModel != nil {
		m := strings.TrimSpace(*in.VoiceModel)
		if len(m) > maxModelLen || hasSpace(m) {
			return fmt.Errorf("%w: 语音识别的模型名不对（不能有空格，最长 %d 个字符）", ErrInvalid, maxModelLen)
		}
		row.VoiceModel = m
	}
	if in.VoiceLanguage != nil {
		lang := strings.TrimSpace(*in.VoiceLanguage)
		if lang != "" && !voiceLangRe.MatchString(lang) {
			return fmt.Errorf("%w: 识别语言写 zh、en 这样的代码，或者 auto（自动判断）", ErrInvalid)
		}
		row.VoiceLanguage = lang
	}
	return nil
}

// voiceSettings 是存着的语音输入设置；地址或令牌没有时回 ErrVoiceDisabled。AI 助手没启用时也回它（对话页上没有入口）。
func (s *Service) voiceSettings(ctx context.Context) (voiceConfig, error) {
	row, _, err := s.loadSettings(ctx)
	if err != nil {
		return voiceConfig{}, err
	}
	if !row.Enabled || row.VoiceBaseURL == "" || row.VoiceTokenEncrypted == "" {
		return voiceConfig{}, ErrVoiceDisabled
	}
	tok, err := s.cfg.Cipher.Decrypt(row.VoiceTokenEncrypted)
	if err != nil {
		return voiceConfig{}, errors.New("解密 Voltip 的访问令牌失败：密钥换过的话要重新填")
	}
	return voiceConfig{BaseURL: row.VoiceBaseURL, Token: tok, Model: row.VoiceModel, Language: row.VoiceLanguage}, nil
}

// voiceClient 是调 Voltip 用的 HTTP 客户端：不走代理（Voltip 在本机或者局域网里）。测试里换成假服务的客户端。
func (s *Service) voiceClient(timeout time.Duration) *http.Client {
	if s.cfg.VoiceHTTPClient != nil {
		c := *s.cfg.VoiceHTTPClient
		c.Timeout = timeout
		return &c
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	return &http.Client{Timeout: timeout, Transport: tr}
}

// Transcribe 把一段 WAV 录音交给 Voltip 识别成文字。language 不为空时用它（不然用设置里的）；录音里没有说话时回空串。
func (s *Service) Transcribe(ctx context.Context, wav []byte, language string) (string, error) {
	vc, err := s.voiceSettings(ctx)
	if err != nil {
		return "", err
	}
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return "", fmt.Errorf("%w: 录音不是 WAV", ErrInvalid)
	}
	if len(wav) > MaxVoiceBytes {
		return "", fmt.Errorf("%w: 录音太长（最多约 10 分钟）", ErrInvalid)
	}
	lang := strings.TrimSpace(language)
	if lang == "" {
		lang = vc.Language
	}
	if lang != "" && !voiceLangRe.MatchString(lang) {
		return "", fmt.Errorf("%w: 识别语言不对", ErrInvalid)
	}
	model := vc.Model
	if model == "" {
		model = DefaultVoiceModel
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "speech.wav")
	if err != nil {
		return "", err
	}
	if _, err = part.Write(wav); err != nil {
		return "", err
	}
	_ = mw.WriteField("model", model)
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	_ = mw.WriteField("response_format", "json")
	if err = mw.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vc.BaseURL+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+vc.Token)
	res, err := s.voiceClient(voiceTimeout).Do(req)
	if err != nil {
		return "", voiceError(err, vc.Token)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", voiceStatusError(res.StatusCode, raw, vc.Token)
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("Voltip 回的不是识别结果（地址是不是填成了别的服务？）")
	}
	return strings.TrimSpace(out.Text), nil
}

// VoiceModel 是 Voltip 列出的一种处理方式：ID 原样填进「模型」，Name 是 Voltip 给的名字（预设、场景在应用里的名称）。
type VoiceModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// VoiceTestResult 是测试语音输入的结果：Voltip 列出的处理方式（GET /models 的每一项都能当模型填）。
type VoiceTestResult struct {
	Models    []VoiceModel `json:"models"`
	LatencyMs int64        `json:"latency_ms"`
}

// maxVoiceModelName 是处理方式的名字最多显示多少个字。
const maxVoiceModelName = 64

// TestVoice 按输入（还没保存的也行）读一次 Voltip 的 GET /models，看地址与令牌能不能用。输入没给令牌时用存着的，
// 但地址变了就不带。
func (s *Service) TestVoice(ctx context.Context, in SettingsInput) (VoiceTestResult, error) {
	stored, _, err := s.loadSettings(ctx)
	if err != nil {
		return VoiceTestResult{}, err
	}
	row := stored
	if err = s.mergeVoice(&row, stored, in); err != nil {
		return VoiceTestResult{}, err
	}
	if row.VoiceBaseURL == "" {
		return VoiceTestResult{}, fmt.Errorf("%w: 先填 Voltip 的地址", ErrInvalid)
	}
	if row.VoiceTokenEncrypted == "" {
		return VoiceTestResult{}, fmt.Errorf("%w: 先填 Voltip 的访问令牌（voltip-server --print-token，或者应用里「复制令牌」）", ErrInvalid)
	}
	tok, err := s.cfg.Cipher.Decrypt(row.VoiceTokenEncrypted)
	if err != nil {
		return VoiceTestResult{}, errors.New("解密 Voltip 的访问令牌失败：重新填一遍")
	}
	ctx, cancel := context.WithTimeout(ctx, voiceTestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, row.VoiceBaseURL+"/models", nil)
	if err != nil {
		return VoiceTestResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	start := time.Now()
	res, err := s.voiceClient(voiceTestTimeout).Do(req)
	if err != nil {
		return VoiceTestResult{}, &TestError{Message: voiceError(err, tok).Error()}
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return VoiceTestResult{}, &TestError{Message: voiceStatusError(res.StatusCode, raw, tok).Error()}
	}
	var page struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return VoiceTestResult{}, &TestError{Message: "回的不是模型列表：地址是不是填成了别的服务？Voltip 的地址以 /v1 结尾"}
	}
	out := VoiceTestResult{Models: []VoiceModel{}, LatencyMs: time.Since(start).Milliseconds()}
	for _, m := range page.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" || len(id) > maxModelLen || hasSpace(id) || strings.Contains(id, tok) {
			continue
		}
		name := strings.Join(strings.Fields(m.Name), " ")
		if r := []rune(name); len(r) > maxVoiceModelName {
			name = string(r[:maxVoiceModelName]) + "…"
		}
		if name == "" || strings.Contains(name, tok) {
			name = id
		}
		out.Models = append(out.Models, VoiceModel{ID: id, Name: name})
	}
	return out, nil
}

// voiceError 把连 Voltip 的网络错误换成给人看的一句话（不带令牌）。
func voiceError(err error, token string) error {
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return errors.New("识别取消了")
	case errors.As(err, &ne) && ne.Timeout():
		return errors.New("Voltip 太久没有回应：录音太长或者本地模型太慢，可以短一点再试")
	case strings.Contains(err.Error(), "connection refused"):
		return errors.New("连不上 Voltip：确认 Voltip 的本机服务开着（应用里「设置 → 本机服务」，或者 voltip-server 在运行），地址与端口对")
	}
	msg := err.Error()
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "***")
	}
	return fmt.Errorf("连不上 Voltip：%s", redact.Secrets(msg))
}

// voiceStatusError 把 Voltip 回的错误状态换成给人看的一句话。
func voiceStatusError(status int, body []byte, token string) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("Voltip 不认这个访问令牌：重新复制一遍令牌（Voltip 里重新生成过的话旧令牌就失效了）")
	case http.StatusServiceUnavailable:
		return errors.New("Voltip 正忙（同时识别的请求满了），过几秒再试")
	case http.StatusRequestEntityTooLarge:
		return errors.New("录音太长，Voltip 不收：短一点再试")
	case http.StatusNotFound:
		return errors.New("Voltip 没有这个接口：地址要以 /v1 结尾，例如 http://127.0.0.1:47840/v1")
	}
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(body, &e) == nil {
		msg = e.Error.Message
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "***")
	}
	if r := []rune(msg); len(r) > 200 {
		msg = string(r[:200]) + "…"
	}
	return fmt.Errorf("Voltip 回 %d：%s", status, redact.Secrets(msg))
}
