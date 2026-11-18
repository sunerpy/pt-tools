package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	assert.Equal(t, http.StatusBadRequest, e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: `{"listen_addr":"8081"}`}).Code, "监听地址要写成地址:端口")
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

type fakeCompatControl struct {
	rt      QbitCompatRuntime
	applied []string
	err     error
}

func (f *fakeCompatControl) Runtime() QbitCompatRuntime { return f.rt }

func (f *fakeCompatControl) Apply(addr string) error {
	f.applied = append(f.applied, addr)
	if f.err != nil {
		f.rt = QbitCompatRuntime{Want: addr, Source: "settings", Error: f.err.Error()}
		return f.err
	}
	f.rt = QbitCompatRuntime{Addr: addr, Want: addr, Source: "settings"}
	if addr == "" {
		f.rt = QbitCompatRuntime{}
	}
	return nil
}

// 网页上设监听地址：存进设置、马上换监听；监听失败也算保存了，回应里写明原因；不带地址时不换
func TestQbitCompatListenSettings(t *testing.T) {
	e := newAppEnv(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}))
	ctl := &fakeCompatControl{}
	e.srv.SetQbitCompatControl(ctl)
	put := func(body string) QbitCompatView {
		w := e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: body})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var v QbitCompatView
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
		return v
	}
	v := put(`{"downloader_id":0,"full_control":false,"listen_addr":" 0.0.0.0:18081 "}`)
	assert.Equal(t, []string{"0.0.0.0:18081"}, ctl.applied)
	assert.True(t, v.Listening)
	assert.Equal(t, "0.0.0.0:18081", v.SettingsAddr)
	assert.Equal(t, "settings", v.ListenSource)

	v = put(`{"downloader_id":0,"full_control":true}`)
	assert.Len(t, ctl.applied, 1, "没带地址：不换监听")
	assert.Equal(t, "0.0.0.0:18081", v.SettingsAddr, "没带地址：存着的地址不变")

	ctl.err = errors.New("bind: address already in use")
	v = put(`{"downloader_id":0,"full_control":true,"listen_addr":"127.0.0.1:18082"}`)
	assert.False(t, v.Listening)
	assert.Equal(t, "127.0.0.1:18082", v.ConfiguredAddr)
	assert.Contains(t, v.ListenError, "address already in use")
	assert.Equal(t, "127.0.0.1:18082", v.SettingsAddr, "监听失败也存下了")

	ctl.err = nil
	v = put(`{"downloader_id":0,"full_control":true,"listen_addr":""}`)
	assert.False(t, v.Listening)
	assert.Empty(t, v.SettingsAddr)
	assert.Equal(t, "", ctl.applied[len(ctl.applied)-1])
}

// 选的下载器用不了时写明原因：删掉了、停用了、没有启用的下载器
func TestQbitCompatDownloaderIssue(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}))
	get := func() QbitCompatView {
		w := e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", session: true})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var v QbitCompatView
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
		return v
	}
	assert.Contains(t, get().DownloaderIssue, "没有启用的下载器")
	off := models.DownloaderSetting{Name: "qb-off", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&off).Error)
	require.NoError(t, db.Model(&off).Update("enabled", false).Error)
	_, err := qbitcompat.SaveSettings(t.Context(), db, qbitcompat.Settings{DownloaderID: off.ID})
	require.NoError(t, err)
	v := get()
	assert.Contains(t, v.DownloaderIssue, "「qb-off」停用了")
	assert.Empty(t, v.Downloader)
	require.NoError(t, db.Delete(&off).Error)
	assert.Contains(t, get().DownloaderIssue, "删掉了")
}

// 最近的登录：成功的写令牌的名字，失败的写原因；登录着的会话数
func TestQbitCompatLogins(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}))
	qc := qbitcompat.New(qbitcompat.Deps{DB: db, Tokens: e.tokens})
	e.srv.SetQbitCompat(qc)
	login := func(pass string) {
		form := url.Values{"username": {"moviepilot"}, "password": {pass}}
		req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		qc.Handler().ServeHTTP(httptest.NewRecorder(), req)
	}
	login(e.token(apitoken.ScopeQbitCompat))
	login("ptt_bad")
	login(e.token(apitoken.ScopeAppRead))
	w := e.do(appReq{method: http.MethodGet, path: "/api/qbit-compat", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v QbitCompatView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	assert.Equal(t, 1, v.Sessions)
	require.NotNil(t, v.LastLogin)
	assert.Equal(t, "测试 qbit:compat", v.LastLogin.Token)
	assert.Equal(t, "moviepilot", v.LastLogin.Username)
	assert.Equal(t, "ok", v.LastLogin.Result)
	require.NotNil(t, v.LastFailure)
	assert.Equal(t, "scope", v.LastFailure.Result, "最近一次失败是令牌没有 qB 兼容权限")
}

type blockingCompatControl struct {
	fakeCompatControl
	gate chan struct{}
}

func (b *blockingCompatControl) Apply(addr string) error {
	if addr == "127.0.0.1:18091" {
		<-b.gate
	}
	return b.fakeCompatControl.Apply(addr)
}

// 两个页面同时保存：存设置与换监听一次只做一个，存下的地址与最后一次换的监听是同一个
func TestQbitCompatSaveSerialized(t *testing.T) {
	e := newAppEnv(t)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.QbitCompatSetting{}, &models.QbitCompatTorrent{}, &models.DownloaderSetting{}))
	ctl := &blockingCompatControl{gate: make(chan struct{})}
	e.srv.SetQbitCompatControl(ctl)
	put := func(addr string, done chan<- int) {
		w := e.do(appReq{method: http.MethodPut, path: "/api/qbit-compat", session: true, body: `{"downloader_id":0,"full_control":false,"listen_addr":"` + addr + `"}`})
		done <- w.Code
	}
	// 读的是开头拿到的库，不用 t.Context()：Eventually 起的检查不在测试的 goroutine 上，不在里面 require
	db := global.GlobalDB.DB
	stored := func() (string, error) {
		row, err := qbitcompat.LoadSettings(context.Background(), db)
		if err != nil {
			return "", err
		}
		return row.ListenAddr, nil
	}
	// 失败退出时也放行卡住的那次保存，不留 goroutine
	release := sync.OnceFunc(func() { close(ctl.gate) })
	t.Cleanup(release)
	a, b := make(chan int, 1), make(chan int, 1)
	go put("127.0.0.1:18091", a)
	require.Eventually(t, func() bool {
		v, err := stored()
		return err == nil && v == "127.0.0.1:18091"
	}, 2*time.Second, 10*time.Millisecond)
	go put("127.0.0.1:18092", b)
	// 前一次还在换监听：后一次等着，不先存。在测试自己的 goroutine 上逐次查，读失败直接算失败：
	// 原来用 assert.Never，它到时就返回，最后一次检查可能在测试结束以后才跑完（Windows CI 上偶发 context canceled）
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		v, err := stored()
		require.NoError(t, err)
		require.NotEqual(t, "127.0.0.1:18092", v, "前一次还在换监听：后一次等着，不先存")
	}
	release()
	assert.Equal(t, http.StatusOK, <-a)
	assert.Equal(t, http.StatusOK, <-b)
	v, err := stored()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:18092", v)
	assert.Equal(t, []string{"127.0.0.1:18091", "127.0.0.1:18092"}, ctl.applied)
	assert.Equal(t, "127.0.0.1:18092", ctl.Runtime().Addr)
}
