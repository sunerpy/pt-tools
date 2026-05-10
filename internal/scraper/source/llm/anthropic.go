package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
)

const anthropicStructuredOutputBeta = "structured-outputs-2025-11-13"

// AnthropicConfig Anthropic Claude provider 配置。
type AnthropicConfig struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// AnthropicProvider Claude 专用 provider。
type AnthropicProvider struct {
	client anthropic.Client
	model  string
	schema map[string]any
}

func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("apikey required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = anthropic.ModelClaudeSonnet4_5
	}

	opts := []aoption.RequestOption{aoption.WithAPIKey(strings.TrimSpace(cfg.APIKey))}
	if cfg.HTTPClient != nil {
		opts = append(opts, aoption.WithHTTPClient(cfg.HTTPClient))
	}

	return &AnthropicProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
		schema: generateNFOSchema(),
	}, nil
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) Kind() ProviderKind {
	return KindAnthropic
}

func (p *AnthropicProvider) Close() error {
	return nil
}

func (p *AnthropicProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	result, err := p.extractStructured(ctx, req)
	if err != nil {
		result, err = p.extractFallback(ctx, req)
		if err != nil {
			return nil, err
		}
	}
	sanitize(result)
	result.GeneratedBy = p.Name()
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}

func (p *AnthropicProvider) extractStructured(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	msg, err := p.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:       p.model,
		MaxTokens:   int64(defaultMaxTokens),
		Temperature: anthropic.Float(0),
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(buildUserPrompt(req))),
		},
		System:       []anthropic.BetaTextBlockParam{{Text: buildSystemPrompt()}},
		OutputFormat: anthropic.BetaJSONSchemaOutputFormat(p.schema),
		Betas:        []anthropic.AnthropicBeta{anthropic.AnthropicBeta(anthropicStructuredOutputBeta)},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic structured call: %w", err)
	}
	result, err := decodeResultPayload(collectAnthropicBetaText(msg.Content))
	if err != nil {
		return nil, fmt.Errorf("parse anthropic structured response: %w", err)
	}
	return result, nil
}

func (p *AnthropicProvider) extractFallback(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:       p.model,
		MaxTokens:   int64(defaultMaxTokens),
		Temperature: anthropic.Float(0),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildUserPrompt(req))),
		},
		System: []anthropic.TextBlockParam{{Text: buildSystemPrompt()}},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic fallback call: %w", err)
	}
	result, err := decodeResultPayload(collectAnthropicText(msg.Content))
	if err != nil {
		return nil, fmt.Errorf("parse anthropic fallback response: %w", err)
	}
	return result, nil
}

func collectAnthropicBetaText(blocks []anthropic.BetaContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func collectAnthropicText(blocks []anthropic.ContentBlockUnion) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
