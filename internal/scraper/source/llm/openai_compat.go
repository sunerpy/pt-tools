package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const defaultMaxTokens = 512

// Config OpenAI-compat provider 配置。
type Config struct {
	PresetName  string
	BaseURL     string
	APIKey      string
	Model       string
	HTTPClient  *http.Client
	Temperature float64
	MaxTokens   int

	SupportsStrictSchema *bool
}

// OpenAICompatProvider 一个 adapter 覆盖 9+ OpenAI-compat provider。
type OpenAICompatProvider struct {
	name        string
	client      openai.Client
	model       string
	strict      bool
	temperature float64
	maxTokens   int
	schema      map[string]any
}

func NewOpenAICompatProvider(cfg Config) (*OpenAICompatProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" && cfg.PresetName != "ollama" {
		return nil, fmt.Errorf("apikey required")
	}

	name := strings.TrimSpace(cfg.PresetName)
	if name == "" {
		name = "openai-compat"
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	strict := false
	var modelsHint []string
	if preset, ok := Presets[cfg.PresetName]; ok {
		if baseURL == "" {
			baseURL = preset.BaseURL
		}
		strict = preset.SupportsStrictSchema
		modelsHint = append(modelsHint, preset.DefaultModels...)
	}
	if cfg.SupportsStrictSchema != nil {
		strict = *cfg.SupportsStrictSchema
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" && len(modelsHint) > 0 {
		model = modelsHint[0]
	}
	if model == "" {
		return nil, fmt.Errorf("model required")
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	opts := make([]option.RequestOption, 0, 3)
	if strings.TrimSpace(cfg.APIKey) != "" {
		opts = append(opts, option.WithAPIKey(strings.TrimSpace(cfg.APIKey)))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}

	return &OpenAICompatProvider{
		name:        name,
		client:      openai.NewClient(opts...),
		model:       model,
		strict:      strict,
		temperature: cfg.Temperature,
		maxTokens:   maxTokens,
		schema:      generateNFOSchema(),
	}, nil
}

func (p *OpenAICompatProvider) Name() string {
	return p.name
}

func (p *OpenAICompatProvider) Kind() ProviderKind {
	return KindOpenAICompat
}

func (p *OpenAICompatProvider) Close() error {
	return nil
}

func (p *OpenAICompatProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	params := openai.ChatCompletionNewParams{
		Model: p.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(buildSystemPrompt()),
			openai.UserMessage(buildUserPrompt(req)),
		},
		Temperature: openai.Float(p.temperature),
		MaxTokens:   openai.Int(int64(p.maxTokens)),
	}

	if p.strict {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "nfo_metadata",
					Schema: p.schema,
					Strict: openai.Bool(true),
				},
			},
		}
	} else {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{Type: "json_object"},
		}
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai compat call: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai compat: no choices")
	}

	result, err := decodeResultPayload(resp.Choices[0].Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	sanitize(result)
	result.GeneratedBy = p.name
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}

// generateNFOSchema 用 invopop/jsonschema 从 NFOResult struct 生成 schema。
func generateNFOSchema() map[string]any {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(&NFOResult{})
	buf, _ := json.Marshal(schema)
	var out map[string]any
	_ = json.Unmarshal(buf, &out)
	return out
}
