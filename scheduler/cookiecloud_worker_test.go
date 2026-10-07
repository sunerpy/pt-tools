package scheduler

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

// encryptFixedForTest 按 CookieCloud 的 aes-128-cbc-fixed 方式加密（key = md5(uuid-password) 的前 16 个字符，iv 全 0）。
func encryptFixedForTest(t *testing.T, uuid, password string, plain []byte) string {
	t.Helper()
	sum := md5.Sum([]byte(uuid + "-" + password))
	block, err := aes.NewCipher([]byte(hex.EncodeToString(sum[:])[:16]))
	require.NoError(t, err)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	data := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, data)
	return base64.StdEncoding.EncodeToString(out)
}

// 同步写完 Cookie 后会回调 Manager（取登录探测）：这时程序退出，StopAll 在锁外停 CookieCloud 后台，不会互相等待。
func TestCookieCloudWorker_StopAllWhileApplying(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "c.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CookieCloudSetting{}))
	enc := encryptFixedForTest(t, "u1", "pw", []byte(`{"cookie_data":{"hdsky.me":[{"name":"uid","value":"1","domain":".hdsky.me","path":"/"}]}}`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"encrypted":"` + enc + `","crypto_type":"aes-128-cbc-fixed"}`))
	}))
	t.Cleanup(srv.Close)

	m := NewManager()
	entered, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	svc := cookiecloud.New(cookiecloud.Config{
		DB: db, Cipher: testCipher{}, HTTP: srv.Client(),
		Sites: func(context.Context) ([]cookiecloud.SiteState, error) {
			return []cookiecloud.SiteState{{Name: "hdsky", BaseURL: "https://hdsky.me/", Enabled: true, Cookie: "old=1"}}, nil
		},
		Apply: func(context.Context, map[string]string) map[string]error {
			once.Do(func() { close(entered) })
			<-proceed
			_ = m.GetLoginReminderMonitor()
			return nil
		},
	})
	pw := "pw"
	_, err = svc.SaveSettings(context.Background(), cookiecloud.SettingsUpdate{ServerURL: srv.URL, UUID: "u1", Password: &pw, AutoSync: true})
	require.NoError(t, err)
	w := NewCookieCloudWorker(CookieCloudWorkerConfig{Service: svc, StartupDelay: 10 * time.Millisecond, Tick: 20 * time.Millisecond})
	m.SetCookieCloudWorker(w)
	w.Start()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("同步没有走到写入")
	}
	done := make(chan struct{})
	go func() {
		m.StopAll()
		close(done)
	}()
	time.Sleep(50 * time.Millisecond) // 让 StopAll 先开始等后台
	close(proceed)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("StopAll 卡住：在锁里等后台，后台又在等锁")
	}
	assert.Nil(t, m.GetCookieCloudWorker())
}
