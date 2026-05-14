package notify

import (
	"context"

	"github.com/sunerpy/pt-tools/models"
)

type Channel interface {
	Type() string
	Init(ctx context.Context, conf *models.NotificationConf) error
	SupportsInbound() bool
	Send(ctx context.Context, n Notification) error
	OnInbound(handler InboundHandler)
	Close(ctx context.Context) error
	Healthy() bool
}
