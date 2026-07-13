package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// syncSignalSite 是一个 mock 站点，其 GetUserInfo 被调用时向 called 发送信号。
// GetUserInfo 在 loginHandler 派生的 goroutine 读取全局 userInfoService 之后才执行，
// 测试通过接收该信号建立 happens-before 边，确保异步读取先于 cleanup 的写入，
// 从而在 -race 下不产生对全局变量的数据竞争（单纯 sleep 不建立同步关系，无法消除竞态）。
type syncSignalSite struct {
	mockSite
	called chan struct{}
}

func (s *syncSignalSite) GetUserInfo(context.Context) (v2.UserInfo, error) {
	s.called <- struct{}{}
	return s.userInfo, nil
}

func TestLoginHandler_TriggersUserInfoSync(t *testing.T) {
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{CacheTTL: 5 * time.Minute})
	site := &syncSignalSite{
		mockSite: mockSite{id: "syncsite", name: "SyncSite", kind: v2.SiteNexusPHP, userInfo: v2.UserInfo{Site: "syncsite"}},
		called:   make(chan struct{}, 1),
	}
	svc.RegisterSite(site)
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	srv := setupServer(t)
	require.NoError(t, srv.store.EnsureAdmin("synced", hashPassword("pw")))

	body, _ := json.Marshal(map[string]string{"username": "synced", "password": "pw"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.loginHandler(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	select {
	case <-site.called:
	case <-time.After(2 * time.Second):
		t.Fatal("异步用户信息同步未触发")
	}
}

func TestRefreshExpiredFavicons_SkipDisabledAndNoURL(t *testing.T) {
	setupFaviconServer(t)

	if v2.GetDefinitionRegistry().GetOrDefault("covnourl") == nil {
		v2.RegisterSiteDefinition(&v2.SiteDefinition{ID: "covnourl", Name: "CovNoURL"})
	}

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covnourl", Enabled: true}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "covdisabled", Enabled: false}).Error)
	old := time.Now().Add(-72 * time.Hour)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "covnourl", SiteName: "CovNoURL", Data: []byte{1}, LastFetched: old,
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.FaviconCache{
		SiteID: "covdisabled", SiteName: "CovDisabled", Data: []byte{1}, LastFetched: old,
	}).Error)

	fs := &FaviconService{refreshInterval: time.Nanosecond}
	fs.refreshExpiredFavicons()
}

func TestApiSiteDetail_PostReloadsConfig(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	body, _ := json.Marshal(models.SiteConfig{
		AuthMethod: "cookie", Cookie: "x=1", APIUrl: "https://hdsky.me",
		RSS: []models.RSSConfig{{Name: "f1", URL: "http://e/rss"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewReader(body))
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
	time.Sleep(50 * time.Millisecond)
}
