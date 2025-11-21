package site

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/models"
)

func TestNewSiteMapConfig(t *testing.T) {
	conf := models.SiteConfig{}
	m := NewSiteMapConfig(models.CMCT, "cookie=1", conf, NewCMCTParser())
	require.NotNil(t, m.SharedConfig)
	require.Equal(t, "cookie=1", m.SharedConfig.Cookie)
	require.NotEmpty(t, m.SharedConfig.SiteCfg.RefererConf.GetReferer())
}
