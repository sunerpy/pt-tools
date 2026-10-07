package scheduler

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
)

type testCipher struct{}

func (testCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (testCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

func newReseedWorkerForTest(t *testing.T) (*ReseedWorker, *reseed.Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "r.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ReseedSetting{}, &models.ReseedRecord{}, &models.TorrentTransferJob{}, &models.DownloaderSetting{}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sites": []any{}}})
	}))
	t.Cleanup(srv.Close)
	svc := reseed.New(reseed.Config{
		DB: db, Cipher: testCipher{},
		NewClient: func(token string) *iyuu.Client {
			c := iyuu.New(token)
			c.BaseURL = srv.URL
			return c
		},
	})
	w := NewReseedWorker(ReseedWorkerConfig{Service: svc, StartupDelay: 10 * time.Millisecond, Tick: 20 * time.Millisecond})
	return w, svc, db
}

func lastReseedResult(t *testing.T, db *gorm.DB) string {
	var r models.ReseedSetting
	if err := db.First(&r, 1).Error; err != nil {
		return ""
	}
	return r.LastResult
}

// 开启并到期时定时跑一轮，把结果（这里是没有可辅种的站点）记下来。
func TestReseedWorker_RunsWhenDue(t *testing.T) {
	w, svc, db := newReseedWorkerForTest(t)
	tok := "tok"
	_, err := svc.SaveSettings(context.Background(), reseed.SettingsUpdate{Enabled: true, Token: &tok})
	require.NoError(t, err)
	w.Start()
	w.Start()
	require.Eventually(t, func() bool { return strings.Contains(lastReseedResult(t, db), "没有可以辅种的站点") }, 5*time.Second, 20*time.Millisecond)
	w.Stop()
	assert.False(t, w.Running())
	assert.Error(t, w.RunNow(), "停止之后不再运行")
	w.Start()
	assert.Same(t, svc, w.Service())
}

// 「立即运行」在后台跑；正在跑时再点返回 ErrReseedBusy。
func TestReseedWorker_RunNow(t *testing.T) {
	w, _, db := newReseedWorkerForTest(t)
	require.NoError(t, w.claim())
	assert.True(t, w.Running())
	assert.ErrorIs(t, w.RunNow(), ErrReseedBusy)
	w.mu.Lock()
	w.busy = false
	w.mu.Unlock()
	w.wg.Done()

	require.NoError(t, w.RunNow())
	require.Eventually(t, func() bool { return strings.Contains(lastReseedResult(t, db), "没有设置 IYUU token") }, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	w.Stop()

	var nilWorker *ReseedWorker
	nilWorker.Start()
	nilWorker.Stop()
	assert.False(t, nilWorker.Running())
	assert.Nil(t, nilWorker.Service())
	assert.Error(t, nilWorker.RunNow())

	mgr := &Manager{}
	w2, _, _ := newReseedWorkerForTest(t)
	mgr.SetReseedWorker(w2)
	assert.Same(t, w2, mgr.GetReseedWorker())
	w3, _, _ := newReseedWorkerForTest(t)
	mgr.SetReseedWorker(w3)
	mgr.StopAll()
	assert.Nil(t, mgr.GetReseedWorker())
}
