package webhook

import (
	"github.com/sunerpy/pt-tools/internal/notify"
)

func init() {
	notify.RegisterChannel("webhook", func() notify.Channel {
		return &WebhookChannel{}
	})
}
