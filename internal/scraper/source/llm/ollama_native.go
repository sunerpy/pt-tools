package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultOllamaBaseURL = "http://localhost:11434"

// OllamaConfig Ollama provider 配置。
type OllamaConfig struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// OllamaNativeProvider 使用 Ollama 原生 /api/chat endpoint 的 format 字段来实现严格 JSON schema 输出。
// 直接手写 HTTP 调用以避免依赖 github.com/ollama/ollama（GO-2025-4251 安全漏洞未修复）。
type OllamaNativeProvider struct {
	baseURL    string
	model      string
	schema     json.RawMessage
	httpClient *http.Client
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Format   json.RawMessage     `json:"format,omitempty"`
	Stream   bool                `json:"stream"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Model     string            `json:"model"`
	CreatedAt string            `json:"created_at"`
	Message   ollamaChatMessage `json:"message"`
	Done      bool              `json:"done"`
}

func NewOllamaNativeProvider(cfg OllamaConfig) (*OllamaNativeProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = Presets["ollama"].DefaultModels[0]
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	schemaBytes, err := jsonMarshalSchema(generateNFOSchema())
	if err != nil {
		return nil, err
	}
	return &OllamaNativeProvider{
		baseURL:    baseURL,
		model:      model,
		schema:     schemaBytes,
		httpClient: httpClient,
	}, nil
}

func (p *OllamaNativeProvider) Name() string {
	return "ollama-native"
}

func (p *OllamaNativeProvider) Kind() ProviderKind {
	return KindOllamaNative
}

func (p *OllamaNativeProvider) Close() error {
	return nil
}

func (p *OllamaNativeProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	body := ollamaChatRequest{
		Model: p.model,
		Messages: []ollamaChatMessage{
			{Role: "system", Content: buildSystemPrompt()},
			{Role: "user", Content: buildUserPrompt(req)},
		},
		Format: p.schema,
		Stream: false,
		Options: map[string]any{
			"temperature": 0,
			"num_predict": defaultMaxTokens,
		},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama http call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ollama http %d: %s", resp.StatusCode, strings.TrimSpace(string(slurp)))
	}

	var final ollamaChatResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&final); decodeErr != nil {
		return nil, fmt.Errorf("decode ollama response: %w", decodeErr)
	}

	result, err := decodeResultPayload(final.Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse ollama response: %w", err)
	}
	sanitize(result)
	result.GeneratedBy = p.Name()
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}

func jsonMarshalSchema(schema map[string]any) ([]byte, error) {
	buf, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	return buf, nil
}
