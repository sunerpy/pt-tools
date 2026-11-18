package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// #542：SiteHRUploadRatio 读站点设置里填的「H&R 上传量达标」；没填、站点不存在、库不可用时为 0（不按上传量）。
func TestSiteHRUploadRatio(t *testing.T) {
	setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", HRUploadRatio: 0.9}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "btschool", AuthMethod: "cookie"}).Error)

	assert.InDelta(t, 0.9, SiteHRUploadRatio("hdsky"), 1e-9)
	assert.Zero(t, SiteHRUploadRatio("btschool"), "没填")
	assert.Zero(t, SiteHRUploadRatio("nosuchsite"), "站点不存在")
	assert.Zero(t, SiteHRUploadRatio(""))

	global.GlobalDB = nil
	assert.Zero(t, SiteHRUploadRatio("hdsky"), "库不可用")
}
