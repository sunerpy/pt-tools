package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sunerpy/pt-tools/internal/scraper/mcp"
)

// ErrNoMCPSession 没有 MCP 会话上下文。
var ErrNoMCPSession = errors.New("no MCP session in context (sampling requires tool-call context)")

// ErrSamplingNotSupported client 不支持 sampling。
var ErrSamplingNotSupported = errors.New("MCP client does not support sampling capability")

// SamplingProvider 通过 MCP sampling/createMessage 反向请求客户端 LLM。
type SamplingProvider struct {
	ModelPreference string
}

func NewSamplingProvider() *SamplingProvider {
	return &SamplingProvider{}
}

func (p *SamplingProvider) Name() string {
	return "mcp-sampling"
}

func (p *SamplingProvider) Kind() ProviderKind {
	return KindMCPSampling
}

func (p *SamplingProvider) Close() error {
	return nil
}

func (p *SamplingProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	session := mcp.SessionFromContext(ctx)
	if session == nil {
		return nil, ErrNoMCPSession
	}
	if caps := session.InitializeParams().Capabilities; caps == nil || caps.Sampling == nil {
		return nil, ErrSamplingNotSupported
	}
	result, err := session.CreateMessage(ctx, &mcpsdk.CreateMessageParams{
		MaxTokens:    512,
		SystemPrompt: buildSystemPrompt() + "\n\nRespond with JSON only.",
		Messages: []*mcpsdk.SamplingMessage{{
			Role:    "user",
			Content: &mcpsdk.TextContent{Text: buildUserPrompt(req)},
		}},
		Temperature: 0,
	})
	if err != nil {
		return nil, err
	}
	text, ok := result.Content.(*mcpsdk.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected sampling content type %T", result.Content)
	}
	var nfo NFOResult
	if err := json.Unmarshal([]byte(text.Text), &nfo); err != nil {
		decoded, decodeErr := decodeResultPayload(text.Text)
		if decodeErr != nil {
			return nil, err
		}
		nfo = *decoded
	}
	sanitize(&nfo)
	return &nfo, nil
}
