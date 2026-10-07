package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/models"
)

// 打开定时同步并到期时同步一次，把结果记下来（这里服务端没有这个 UUID 的数据，记为失败）；没打开时不同步。
func TestCookieCloudWorker_SyncsWhenDue(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "c.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CookieCloudSetting{}))
	hits := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.URL.Path
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	svc := cookiecloud.New(cookiecloud.Config{
		DB: db, Cipher: testCipher{}, HTTP: srv.Client(),
		Sites: func(context.Context) ([]cookiecloud.SiteState, error) { return nil, nil },
	})
	pw := "pw"
	_, err = svc.SaveSettings(context.Background(), cookiecloud.SettingsUpdate{ServerURL: srv.URL, UUID: "u1", Password: &pw})
	require.NoError(t, err)

	w := NewCookieCloudWorker(CookieCloudWorkerConfig{Service: svc, StartupDelay: 10 * time.Millisecond, Tick: 20 * time.Millisecond})
	w.Start()
	w.Start()
	select {
	case p := <-hits:
		t.Fatalf("没打开定时同步却请求了 %s", p)
	case <-time.After(100 * time.Millisecond):
	}
	_, err = svc.SaveSettings(context.Background(), cookiecloud.SettingsUpdate{ServerURL: srv.URL, UUID: "u1", AutoSync: true, IntervalHours: 6})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var r models.CookieCloudSetting
		return db.First(&r, 1).Error == nil && strings.HasPrefix(r.LastResult, "同步失败") && r.LastSyncAt != nil
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "/get/u1", <-hits)
	w.Stop()
	w.Start()
	assert.Same(t, svc, w.Service())

	var nilWorker *CookieCloudWorker
	assert.Nil(t, nilWorker.Service())
	nilWorker.Start()
	nilWorker.Stop()

	m := newTestManager(t)
	m.SetCookieCloudWorker(w)
	assert.Same(t, w, m.GetCookieCloudWorker())
	other := NewCookieCloudWorker(CookieCloudWorkerConfig{Service: svc})
	m.SetCookieCloudWorker(other)
	assert.Same(t, other, m.GetCookieCloudWorker())
}
