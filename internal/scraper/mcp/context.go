package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionKey struct{}

func WithSession(ctx context.Context, s *mcpsdk.ServerSession) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

func SessionFromContext(ctx context.Context) *mcpsdk.ServerSession {
	s, _ := ctx.Value(sessionKey{}).(*mcpsdk.ServerSession)
	return s
}
