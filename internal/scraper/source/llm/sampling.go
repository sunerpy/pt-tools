package llm

import (
	"context"
	"errors"
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
	_ = ctx
	_ = req
	return nil, ErrNoMCPSession
}
