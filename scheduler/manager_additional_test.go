package scheduler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
)

func TestStartAll_UnknownSiteHandled(t *testing.T) {
	m := NewManager()
	global.InitLogger(zap.NewNop())
	cfg := &models.Config{Sites: map[models.SiteGroup]models.SiteConfig{models.SiteGroup("unknown"): {Enabled: ptr(true)}}}
	require.NotPanics(t, func() { m.StartAll(cfg) })
}
