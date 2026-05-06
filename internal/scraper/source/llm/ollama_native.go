package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	ollamaapi "github.com/ollama/ollama/api"
)

const defaultOllamaBaseURL = "http://localhost:11434"

// OllamaConfig Ollama provider 配置。
type OllamaConfig struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// OllamaNativeProvider 使用原生 format schema。
type OllamaNativeProvider struct {
	client *ollamaapi.Client
	model  string
	schema []byte
}

func NewOllamaNativeProvider(cfg OllamaConfig) (*OllamaNativeProvider, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse ollama base url: %w", err)
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = Presets["ollama"].DefaultModels[0]
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	schema, err := jsonMarshalSchema(generateNFOSchema())
	if err != nil {
		return nil, err
	}
	return &OllamaNativeProvider{
		client: ollamaapi.NewClient(parsed, httpClient),
		model:  model,
		schema: schema,
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
	stream := false
	chatReq := &ollamaapi.ChatRequest{
		Model:  p.model,
		Stream: &stream,
		Format: p.schema,
		Options: map[string]any{
			"temperature": 0,
			"num_predict": defaultMaxTokens,
		},
		Messages: []ollamaapi.Message{
			{Role: "system", Content: buildSystemPrompt()},
			{Role: "user", Content: buildUserPrompt(req)},
		},
	}

	var final ollamaapi.ChatResponse
	if err := p.client.Chat(ctx, chatReq, func(resp ollamaapi.ChatResponse) error {
		final = resp
		return nil
	}); err != nil {
		return nil, fmt.Errorf("ollama native call: %w", err)
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
