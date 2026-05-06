package llm

import (
	"context"
	"errors"
	"fmt"
)

// ChainedProvider 按顺序尝试 providers，第一个成功就返回。
type ChainedProvider struct {
	providers []Provider
}

func NewChainedProvider(providers ...Provider) *ChainedProvider {
	return &ChainedProvider{providers: providers}
}

func (c *ChainedProvider) Name() string {
	return "chained"
}

func (c *ChainedProvider) Kind() ProviderKind {
	return KindChained
}

func (c *ChainedProvider) Close() error {
	var errs []error
	for _, p := range c.providers {
		if err := p.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *ChainedProvider) Extract(ctx context.Context, req ExtractRequest) (*NFOResult, error) {
	var errs []error
	for _, p := range c.providers {
		result, err := p.Extract(ctx, req)
		if err == nil {
			return result, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", p.Name(), err))
	}
	return nil, fmt.Errorf("all providers failed: %w", errors.Join(errs...))
}
