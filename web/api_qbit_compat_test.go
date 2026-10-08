package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/qbitcompat"
)

// qB 兼容入口的设置接口：只认 session；读写下载器与完全控制；回监听状态与实际用的下载器
func TestQbitCompatSettingsAPI(t *testing.T) {
	e := newAppEnv(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}, &models.TorrentInfo{}))
	def := models.DownloaderSetting{Name: "qb-default", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}
	other := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&def).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&other).Error)

	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat"}).Code)
	tok := e.token(apitoken.ScopeQbitCompat, apitoken.ScopeAppRead, apitoken.ScopeAppWrite)
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", bearer: tok}).Code, "令牌碰不到")

	w := e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v QbitCompatView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.False(t, v.Listening)
	assert.Equal(t, "qb-default", v.Downloader, "没指定时用默认下载器")

	body := `{"downloader_id":` + strconv.FormatUint(uint64(other.ID), 10) + `,"full_control":true}`
	w = e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.Equal(t, other.ID, v.DownloaderID)
	assert.True(t, v.FullControl)
	assert.Equal(t, "tr", v.Downloader)

	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: `{"downloader_id":999}`}).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: `{"listen_addr":"0.0.0.0:1"}`}).Code, "监听地址不能从这里改")
	assert.Equal(t, http.StatusUnauthorized, e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", bearer: tok, body: `{}`}).Code)

	qc := qbitcompat.New(qbitcompat.Deps{DB: global.GlobalDB.DB})
	qc.SetAddr("127.0.0.1:18080")
	e.srv.SetQbitCompat(qc)
	w = e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", session: true})
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.True(t, v.Listening)
	assert.Equal(t, "127.0.0.1:18080", v.ListenAddr)
}

// 设置接口的错误：数据库没初始化回 503；设置、下载器或所有权表读不出来、存不进去时回 500
func TestQbitCompatSettingsAPIErrors(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}).Error)
	get := func() int { return e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", session: true}).Code }
	put := func() int {
		return e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: `{}`}).Code
	}

	require.NoError(t, db.Migrator().DropTable(&models.QbitCompatTorrent{}))
	assert.Equal(t, http.StatusInternalServerError, get(), "所有权表读不出来")
	require.NoError(t, db.Migrator().DropTable(&models.DownloaderSetting{}))
	assert.Equal(t, http.StatusInternalServerError, get(), "下载器读不出来")
	assert.Equal(t, http.StatusInternalServerError, put(), "存进去了，但状态读不出来")
	require.NoError(t, db.Migrator().DropTable(&models.QbitCompatSetting{}))
	assert.Equal(t, http.StatusInternalServerError, get(), "设置读不出来")
	assert.Equal(t, http.StatusInternalServerError, put(), "设置存不进去")

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, get())
	assert.Equal(t, http.StatusServiceUnavailable, put())
}
