package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

type backupEnv struct {
	srv     *Server
	handler http.Handler
	home    string
	sid     string
}

// setupBackupServer 建一个备份测试用的服务：HOME 指到临时目录，secret.key 是固定的一把（结束时原样换回进程里的密钥状态），
// 库里有一条加密的站点 Cookie；请求经完整的路由走，带着网页会话。
func setupBackupServer(t *testing.T) *backupEnv {
	t.Helper()
	snap := crypto.SnapshotKeyState()
	t.Cleanup(func() { crypto.RestoreKeyState(snap) })
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	dir := filepath.Join(home, ".pt-tools")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.key"), []byte(strings.Repeat("11", 32)), 0o600))
	crypto.ResetForTest()

	srv, db := setupTestServer(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.BackupSetting{}, &models.AppMeta{}, &models.SchemaVersion{}))
	enc, err := crypto.Encrypt([]byte("uid=1"))
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", CookieEncrypted: enc}).Error)
	return &backupEnv{srv: srv, handler: srv.buildHandler(), home: home, sid: srv.sessions.create("admin")}
}

func (e *backupEnv) call(t *testing.T, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.AddCookie(&http.Cookie{Name: "session", Value: e.sid})
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	return w
}

func (e *backupEnv) view(t *testing.T, w *httptest.ResponseRecorder) BackupsView {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v BackupsView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

// 没有会话时一律 401：备份文件里有数据库与密钥，只认网页登录。
func TestBackupAPI_RequiresSession(t *testing.T) {
	e := setupBackupServer(t)
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/backups"},
		{http.MethodPost, "/api/backups"},
		{http.MethodPut, "/api/backups/settings"},
		{http.MethodGet, "/api/backups/file/pt-tools-backup-x.tar.gz"},
		{http.MethodDelete, "/api/backups/file/pt-tools-backup-x.tar.gz"},
		{http.MethodPost, "/api/backups/file/pt-tools-backup-x.tar.gz/restore"},
		{http.MethodPost, "/api/backups/upload"},
		{http.MethodDelete, "/api/backups/pending"},
	} {
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, httptest.NewRequest(r.method, r.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", r.method, r.path)
	}
	found := false
	for _, x := range configExcluded {
		if strings.Contains(x, "备份") {
			found = true
		}
	}
	assert.True(t, found, "App API 与 MCP 的配置网关读改不到备份与恢复")
}

// 默认设置、立即备份、列出、下载、删除。
func TestBackupAPI_CreateListDownloadDelete(t *testing.T) {
	e := setupBackupServer(t)
	v := e.view(t, e.call(t, http.MethodGet, "/api/backups", nil))
	assert.True(t, v.Settings.Enabled)
	assert.Equal(t, "04:30", v.Settings.Time)
	assert.Equal(t, 7, v.Settings.Keep)
	assert.Equal(t, "", v.Settings.Dir)
	assert.Equal(t, filepath.Join(e.home, ".pt-tools", "backups"), v.Settings.DefaultDir)
	assert.Equal(t, v.Settings.DefaultDir, v.Dir)
	assert.True(t, v.Settings.IncludeSecret)
	assert.Equal(t, backup.MinKeep, v.Settings.MinKeep)
	assert.Equal(t, backup.MaxKeep, v.Settings.MaxKeep)
	assert.False(t, v.EnvKey)
	require.NotNil(t, v.NextAt, "开着时有下一次备份的时间")
	assert.Empty(t, v.Backups)
	assert.Nil(t, v.Last)
	assert.Nil(t, v.Pending)

	v = e.view(t, e.call(t, http.MethodPost, "/api/backups", nil))
	require.Len(t, v.Backups, 1)
	b := v.Backups[0]
	assert.Equal(t, string(backup.ReasonManual), b.Reason)
	assert.Equal(t, "手动备份", b.ReasonLabel)
	assert.True(t, b.HasSecret)
	assert.Equal(t, backup.SecretFromFile, b.SecretSource)
	require.NotNil(t, b.CreatedAt)
	require.NotNil(t, v.Last)
	assert.Equal(t, b.Name, v.Last.Name)
	assert.Empty(t, v.Last.Error)
	require.NotNil(t, v.Last.LastOKAt)

	w := e.call(t, http.MethodGet, "/api/backups/file/"+b.Name, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), b.Name)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.EqualValues(t, b.Size, w.Body.Len())
	dl := filepath.Join(t.TempDir(), b.Name)
	require.NoError(t, os.WriteFile(dl, w.Body.Bytes(), 0o600))
	m, err := backup.ReadManifest(dl)
	require.NoError(t, err)
	assert.True(t, m.HasSecret)

	assert.Equal(t, http.StatusNotFound, e.call(t, http.MethodGet, "/api/backups/file/pt-tools-backup-nope.tar.gz", nil).Code)
	assert.Equal(t, http.StatusNotFound, e.call(t, http.MethodGet, "/api/backups/file/torrents.db", nil).Code, "只认这里起的文件名")
	v = e.view(t, e.call(t, http.MethodDelete, "/api/backups/file/"+b.Name, nil))
	assert.Empty(t, v.Backups)
	assert.Equal(t, http.StatusNotFound, e.call(t, http.MethodDelete, "/api/backups/file/"+b.Name, nil).Code)
}

// 保存设置：校验不过回 400 和原因；通过后读回新设置。
func TestBackupAPI_Settings(t *testing.T) {
	e := setupBackupServer(t)
	w := e.call(t, http.MethodPut, "/api/backups/settings", strings.NewReader(`{"enabled":true,"time":"4:30","keep":7}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "HH:MM")
	assert.NotContains(t, w.Body.String(), "备份设置有误", "只回原因")
	w = e.call(t, http.MethodPut, "/api/backups/settings", strings.NewReader(`{"enabled":true,"time":"04:30","keep":99}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, http.StatusBadRequest, e.call(t, http.MethodPut, "/api/backups/settings", strings.NewReader(`nope`)).Code)

	dir := filepath.Join(t.TempDir(), "bk")
	body, err := json.Marshal(BackupSettingsRequest{Enabled: false, Time: "23:15", Keep: 3, Dir: dir, IncludeSecret: false})
	require.NoError(t, err)
	v := e.view(t, e.call(t, http.MethodPut, "/api/backups/settings", bytes.NewReader(body)))
	assert.False(t, v.Settings.Enabled)
	assert.Equal(t, "23:15", v.Settings.Time)
	assert.Equal(t, 3, v.Settings.Keep)
	assert.Equal(t, dir, v.Settings.Dir)
	assert.Equal(t, dir, v.Dir)
	assert.False(t, v.Settings.IncludeSecret)
	assert.Nil(t, v.NextAt, "关着时没有下一次")

	v = e.view(t, e.call(t, http.MethodPost, "/api/backups", nil))
	require.Len(t, v.Backups, 1)
	assert.False(t, v.Backups[0].HasSecret, "按设置不带密钥")
	_, err = os.Stat(filepath.Join(dir, v.Backups[0].Name))
	assert.NoError(t, err, "放在设置的目录里")
}

// 从列表里恢复：放进待恢复、能取消；坏文件回 400，原因写清楚。
func TestBackupAPI_RestoreAndCancel(t *testing.T) {
	e := setupBackupServer(t)
	v := e.view(t, e.call(t, http.MethodPost, "/api/backups", nil))
	name := v.Backups[0].Name

	v = e.view(t, e.call(t, http.MethodPost, "/api/backups/file/"+name+"/restore", nil))
	require.NotNil(t, v.Pending)
	assert.Equal(t, name, v.Pending.Name)
	assert.Equal(t, string(backup.KeyKeepFile), v.Pending.KeyAction)
	assert.False(t, v.Pending.NoCipher)
	assert.Equal(t, "手动备份", v.Pending.ReasonLabel)
	_, err := os.Stat(filepath.Join(e.home, ".pt-tools", backup.PendingDir, "ready"))
	assert.NoError(t, err)

	v = e.view(t, e.call(t, http.MethodDelete, "/api/backups/pending", nil))
	assert.Nil(t, v.Pending)

	broken := filepath.Join(v.Dir, "pt-tools-backup-broken.tar.gz")
	require.NoError(t, os.WriteFile(broken, []byte("junk"), 0o600))
	v = e.view(t, e.call(t, http.MethodGet, "/api/backups", nil))
	var listed bool
	for _, b := range v.Backups {
		if b.Name == "pt-tools-backup-broken.tar.gz" {
			listed = true
			assert.NotEmpty(t, b.Error, "坏文件也列出来，让人能删掉")
		}
	}
	assert.True(t, listed)
	w := e.call(t, http.MethodPost, "/api/backups/file/pt-tools-backup-broken.tar.gz/restore", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不是 tar.gz")
	assert.Equal(t, http.StatusNotFound, e.call(t, http.MethodPost, "/api/backups/file/pt-tools-backup-nope.tar.gz/restore", nil).Code)
}

// 上传恢复：请求体就是备份文件，名字在 ?name= 里；用完删掉上传的临时文件。
func TestBackupAPI_UploadRestore(t *testing.T) {
	e := setupBackupServer(t)
	v := e.view(t, e.call(t, http.MethodPost, "/api/backups", nil))
	raw, err := os.ReadFile(filepath.Join(v.Dir, v.Backups[0].Name))
	require.NoError(t, err)

	v = e.view(t, e.call(t, http.MethodPost, "/api/backups/upload?name="+url.QueryEscape(`C:\Users\me\家里的备份.tar.gz`), bytes.NewReader(raw)))
	require.NotNil(t, v.Pending)
	assert.Equal(t, "家里的备份.tar.gz", v.Pending.Name)
	entries, err := os.ReadDir(filepath.Join(e.home, ".pt-tools"))
	require.NoError(t, err)
	for _, x := range entries {
		assert.False(t, strings.HasPrefix(x.Name(), ".pt-tools-upload"), "上传的临时文件删掉了：%s", x.Name())
	}

	w := e.call(t, http.MethodPost, "/api/backups/upload", strings.NewReader("junk"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	req := httptest.NewRequest(http.MethodPost, "/api/backups/upload", strings.NewReader("x"))
	req.AddCookie(&http.Cookie{Name: "session", Value: e.sid})
	req.ContentLength = backup.MaxUploadSize + 1
	w = httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

// 正在备份或恢复时，再点一次回 409。
func TestBackupAPI_Busy(t *testing.T) {
	e := setupBackupServer(t)
	e.srv.backupMu.Lock()
	defer e.srv.backupMu.Unlock()
	assert.Equal(t, http.StatusConflict, e.call(t, http.MethodPost, "/api/backups", nil).Code)
	assert.Equal(t, http.StatusConflict, e.call(t, http.MethodPost, "/api/backups/upload", strings.NewReader("x")).Code)
	assert.Equal(t, http.StatusConflict, e.call(t, http.MethodPost, "/api/backups/file/pt-tools-backup-x.tar.gz/restore", nil).Code)
}

// 读不出备份目录时设置照常返回，原因写在 list_error。
func TestBackupAPI_ListError(t *testing.T) {
	e := setupBackupServer(t)
	dir := filepath.Join(t.TempDir(), "bk")
	body, err := json.Marshal(BackupSettingsRequest{Enabled: true, Time: "04:30", Keep: 7, Dir: dir, IncludeSecret: true})
	require.NoError(t, err)
	e.view(t, e.call(t, http.MethodPut, "/api/backups/settings", bytes.NewReader(body)))
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("x"), 0o600))
	v := e.view(t, e.call(t, http.MethodGet, "/api/backups", nil))
	assert.NotEmpty(t, v.ListError)
	assert.Empty(t, v.Backups)
	assert.Equal(t, dir, v.Settings.Dir)
}

// 最近一次备份失败与启动时恢复的结果都回给界面。
func TestBackupAPI_ResultsAndUploadName(t *testing.T) {
	e := setupBackupServer(t)
	at := time.Date(2026, 11, 2, 4, 30, 0, 0, time.Local)
	require.NoError(t, e.srv.store.RecordBackupResult(backup.ReasonPreUpgrade, "", assert.AnError, at))
	require.NoError(t, e.srv.store.RecordRestoreResult("a.tar.gz", "", assert.AnError, at))
	v := e.view(t, e.call(t, http.MethodGet, "/api/backups", nil))
	require.NotNil(t, v.Last)
	assert.Equal(t, "升级前备份", v.Last.ReasonLabel)
	assert.NotEmpty(t, v.Last.Error)
	assert.Nil(t, v.Last.LastOKAt)
	require.NotNil(t, v.Restore)
	assert.Equal(t, "a.tar.gz", v.Restore.Name)
	assert.NotEmpty(t, v.Restore.Error)

	assert.Equal(t, "上传的备份", uploadName(""))
	assert.Equal(t, "上传的备份", uploadName("/"))
	assert.Equal(t, "x.tar.gz", uploadName("../../x.tar.gz"))
	assert.Equal(t, "ab.tar.gz", uploadName("a\x00b.tar.gz"))
	assert.Len(t, []rune(uploadName(strings.Repeat("长", 300))), 200)
}
