package feishuapp

import (
	"errors"
	"fmt"
	"testing"

	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewLarkRunner_BuildsClientWithoutIO(t *testing.T) {
	cfg := &Config{AppID: "cli_runner", AppSecret: "s", Domain: DomainLark}
	r := newLarkRunner(cfg, wsHooks{}, sdkLogger{log: func() *zap.SugaredLogger { return zap.NewNop().Sugar() }})
	_, ok := r.(*larkws.Client)
	assert.True(t, ok)
}

func TestIsClientError(t *testing.T) {
	assert.True(t, isClientError(larkws.NewClientError(1000040346, "app_id is invalid")))
	assert.True(t, isClientError(fmt.Errorf("wrapped: %w", larkws.NewClientError(403, "forbidden"))))
	assert.False(t, isClientError(larkws.NewServerError(1, "system busy")))
	assert.False(t, isClientError(errors.New("dial tcp: refused")))
}
