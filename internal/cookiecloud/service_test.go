package cookiecloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

type plainCipher struct{}

func (plainCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (plainCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// 向量明文里 hdsky.me 的 Cookie 拼成的请求头
const hdskyHeader = "c_secure_pass=s3cret; c_secure_uid=42"

type env struct {
	t       *testing.T
	db      *gorm.DB
	svc     *Service
	srv     *httptest.Server
	now     time.Time
	sites   []SiteState
	mu      sync.Mutex
	applied []map[string]string
	logs    *observer.ObservedLogs
	// payloads 是假 CookieCloud 服务上按 UUID 存的密文
	payloads map[string]string
	// applyErr 让这些站点写入失败
	applyErr map[string]error
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "c.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CookieCloudSetting{}))
	e := &env{
		t: t, db: db, now: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC),
		payloads: map[string]string{vecUUID: `{"encrypted":"` + vecLegacy + `"}`},
		applyErr: map[string]error{},
		sites: []SiteState{
			{Name: "hdsky", DisplayName: "HDSky", BaseURL: "https://hdsky.me/", Enabled: true, Cookie: "old=1"},
			{Name: "example", DisplayName: "Example", BaseURL: "https://example.org/"},
			{Name: "ourbits", DisplayName: "OurBits", BaseURL: "https://ourbits.club/", Enabled: true},
		},
	}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		body, ok := e.payloads[strings.TrimPrefix(r.URL.Path, "/get/")]
		e.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(e.srv.Close)
	core, logs := observer.New(zapcore.DebugLevel)
	e.logs = logs
	e.svc = New(Config{
		DB: db, Cipher: plainCipher{}, HTTP: e.srv.Client(),
		Sites: func(context.Context) ([]SiteState, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			return append([]SiteState(nil), e.sites...), nil
		},
		Apply: func(_ context.Context, cookies map[string]string) map[string]error {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.applied = append(e.applied, cookies)
			failed := map[string]error{}
			for i := range e.sites {
				c, ok := cookies[e.sites[i].Name]
				if !ok {
					continue
				}
				if err := e.applyErr[e.sites[i].Name]; err != nil {
					failed[e.sites[i].Name] = err
					continue
				}
				e.sites[i].Cookie, e.sites[i].Enabled = c, true
			}
			return failed
		},
		Now:    func() time.Time { return e.now },
		Logger: zap.New(core).Sugar(),
	})
	return e
}

func (e *env) configure(mod func(*SettingsUpdate)) {
	e.t.Helper()
	pw := vecPassword
	u := SettingsUpdate{ServerURL: e.srv.URL + "/", UUID: " " + vecUUID + " ", Password: &pw}
	if mod != nil {
		mod(&u)
	}
	_, err := e.svc.SaveSettings(context.Background(), u)
	require.NoError(e.t, err)
}

func TestSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, Settings{IntervalHours: models.CookieCloudDefaultIntervalHours}, got)

	for _, u := range []SettingsUpdate{
		{ServerURL: "ftp://x"},
		{ServerURL: "http://user:pass@x"},
		{ServerURL: "http://x", IntervalHours: 999},
		{ServerURL: "http://x", UUID: "u", AutoSync: true}, // 没有密码
	} {
		_, saveErr := e.svc.SaveSettings(ctx, u)
		assert.ErrorIs(t, saveErr, ErrInvalid, u.ServerURL)
	}

	e.configure(func(u *SettingsUpdate) { u.AutoSync, u.IntervalHours = true, 6 })
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, e.srv.URL, got.ServerURL, "去掉末尾的 /")
	assert.Equal(t, vecUUID, got.UUID)
	assert.True(t, got.HasPassword)
	assert.True(t, got.AutoSync)
	var row models.CookieCloudSetting
	require.NoError(t, e.db.First(&row, 1).Error)
	assert.Equal(t, "enc:"+vecPassword, row.PasswordEncrypted, "密码加密保存")

	// 不带密码修改：保留原来的；记下的同步结果不受影响
	require.NoError(t, e.svc.RecordSync(ctx, e.now, "更新了 1 个站点的 Cookie"))
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e.srv.URL, UUID: vecUUID, IntervalHours: 12})
	require.NoError(t, err)
	assert.True(t, got.HasPassword)
	assert.False(t, got.AutoSync)
	assert.Equal(t, "更新了 1 个站点的 Cookie", got.LastResult)
	empty := ""
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e.srv.URL, UUID: vecUUID, Password: &empty})
	require.NoError(t, err)
	assert.False(t, got.HasPassword, "空串清除密码")
}

func TestDue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	assert.False(t, e.svc.Due(ctx), "没设置")
	e.configure(nil)
	assert.False(t, e.svc.Due(ctx), "没打开定时同步")
	e.configure(func(u *SettingsUpdate) { u.AutoSync, u.IntervalHours = true, 6 })
	assert.True(t, e.svc.Due(ctx), "没同步过")
	require.NoError(t, e.svc.RecordSync(ctx, e.now, "ok"))
	assert.False(t, e.svc.Due(ctx))
	e.now = e.now.Add(6 * time.Hour)
	assert.True(t, e.svc.Due(ctx))
}

// 预览：只列 CookieCloud 里有对站点地址有效的 Cookie 的站点，写明 Cookie 名、是否启用、有没有变化；不含 Cookie 的值，不写任何东西。
func TestPreview(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, err := e.svc.Preview(ctx)
	assert.ErrorIs(t, err, ErrNotConfigured)
	e.configure(nil)
	p, err := e.svc.Preview(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, p.Domains)
	require.Len(t, p.Items, 2)
	assert.Equal(t, PreviewItem{Site: "example", SiteName: "Example", Host: "example.org", CookieNames: []string{"x"}, Changed: true}, p.Items[0])
	assert.Equal(t, PreviewItem{Site: "hdsky", SiteName: "HDSky", Host: "hdsky.me", CookieNames: []string{"c_secure_pass", "c_secure_uid"}, Enabled: true, Changed: true}, p.Items[1])
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "s3cret", "预览不带 Cookie 的值")
	assert.Empty(t, e.applied)

	// 两种加密方式都认
	e.payloads[vecUUID] = `{"encrypted":"` + vecFixed + `","crypto_type":"aes-128-cbc-fixed"}`
	p, err = e.svc.Preview(ctx)
	require.NoError(t, err)
	assert.Len(t, p.Items, 2)
}

func TestImport(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(nil)
	_, err := e.svc.Import(ctx, nil)
	assert.ErrorIs(t, err, ErrInvalid)

	res, err := e.svc.Import(ctx, []string{"hdsky", "example", "ourbits", "hdsky", " "})
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky", "example"}, res.Imported)
	assert.Equal(t, []string{"ourbits"}, res.Missing, "CookieCloud 里没有它的 Cookie")
	require.Len(t, e.applied, 1)
	assert.Equal(t, map[string]string{"hdsky": hdskyHeader, "example": "x=y"}, e.applied[0], "没启用的站点也可以导入")
	assert.Contains(t, res.Summary(), "更新了 2 个站点的 Cookie")

	// 再导一次：都没变化，不写
	res, err = e.svc.Import(ctx, []string{"hdsky"})
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky"}, res.Unchanged)
	assert.Len(t, e.applied, 1)

	// 写不进去的站点单独列出，其余的照常写
	e.sites[0].Cookie, e.sites[1].Cookie = "", ""
	e.applyErr["example"] = errors.New("保存站点 example 失败")
	res, err = e.svc.Import(ctx, []string{"hdsky", "example"})
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky"}, res.Imported)
	assert.Equal(t, []SiteError{{Site: "example", Error: "保存站点 example 失败"}}, res.Failed)
	assert.Contains(t, res.Summary(), "1 个写入失败（example）")
}

// 定时同步只更新已经启用、Cookie 有变化的站点，不启用新站点。
func TestSync(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(func(u *SettingsUpdate) { u.AutoSync = true })
	res, err := e.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky"}, res.Imported, "example 没启用，不碰")
	require.Len(t, e.applied, 1)
	assert.Equal(t, map[string]string{"hdsky": hdskyHeader}, e.applied[0])
	res, err = e.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Empty(t, res.Imported)
	assert.Equal(t, []string{"hdsky"}, res.Unchanged)
	assert.Equal(t, "1 个没有变化", res.Summary())
}

func TestLoadErrors(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(func(u *SettingsUpdate) { wrong := "wrong"; u.Password = &wrong })
	_, err := e.svc.Preview(ctx)
	assert.ErrorIs(t, err, ErrDecrypt, "密码不对")
	e.configure(func(u *SettingsUpdate) { u.UUID = "missing" })
	_, err = e.svc.Sync(ctx)
	assert.ErrorIs(t, err, ErrNotFound)

	e.configure(nil)
	e.svc.runMu.Lock()
	_, err = e.svc.Import(ctx, []string{"hdsky"})
	assert.ErrorIs(t, err, ErrBusy)
	_, err = e.svc.Preview(ctx)
	assert.ErrorIs(t, err, ErrBusy)
	e.svc.runMu.Unlock()

	// 连不上：错误里不带地址（地址里有 UUID）
	e.srv.Close()
	_, err = e.svc.Preview(ctx)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), vecUUID)
}

// 只跟随同一主机内的重定向。
func TestFetchRedirects(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"encrypted":"x"}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/get/away":
			http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+r.URL.Path, http.StatusFound)
		case "/get/here":
			http.Redirect(w, r, "/v2/get/here", http.StatusFound)
		case "/v2/get/here":
			_, _ = w.Write([]byte(`{"encrypted":"x"}`))
		}
	}))
	defer srv.Close()
	_, err := Fetch(context.Background(), srv.Client(), srv.URL, "away")
	assert.ErrorContains(t, err, "别的主机")
	p, err := Fetch(context.Background(), srv.Client(), srv.URL, "here")
	require.NoError(t, err)
	assert.Equal(t, "x", p.Encrypted)
}

// 日志里没有 Cookie 的值。
func TestNoCookieValuesInLogs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(nil)
	_, err := e.svc.Preview(ctx)
	require.NoError(t, err)
	_, err = e.svc.Import(ctx, []string{"hdsky", "example"})
	require.NoError(t, err)
	e.configure(func(u *SettingsUpdate) { u.AutoSync = true })
	_, err = e.svc.Sync(ctx)
	require.NoError(t, err)
	require.NotZero(t, e.logs.Len())
	for _, entry := range e.logs.All() {
		line := entry.Message
		for _, f := range entry.Context {
			line += " " + f.String
		}
		assert.NotContains(t, line, "s3cret")
		assert.NotContains(t, line, vecPassword)
	}
}

func TestSameCookie(t *testing.T) {
	assert.True(t, SameCookie("a=1; b=2", "b=2;a=1 ;"))
	assert.False(t, SameCookie("a=1; b=2", "a=1"))
	assert.False(t, SameCookie("a=1", "a=2"))
	assert.True(t, SameCookie("", " ; "))
}

// 读取期间改了设置（换了账户、关了定时同步）：读到的是旧设置的数据，不写。
func TestImportAbortsWhenSettingsChange(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(func(u *SettingsUpdate) { u.AutoSync = true })
	other := "other-pass"
	e.svc.cfg.Sites = func(context.Context) ([]SiteState, error) {
		// 取数和解密已经做完，这时用户保存了新密码
		_, saveErr := e.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e.srv.URL, UUID: vecUUID, Password: &other})
		require.NoError(t, saveErr)
		return append([]SiteState(nil), e.sites...), nil
	}
	_, err := e.svc.Import(ctx, []string{"hdsky"})
	assert.ErrorIs(t, err, ErrSettingsChanged)
	assert.Empty(t, e.applied)

	// 定时同步：读取期间关掉了定时同步
	e2 := newEnv(t)
	e2.configure(func(u *SettingsUpdate) { u.AutoSync = true })
	e2.svc.cfg.Sites = func(context.Context) ([]SiteState, error) {
		pw := vecPassword
		_, saveErr := e2.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e2.srv.URL, UUID: vecUUID, Password: &pw})
		require.NoError(t, saveErr)
		return append([]SiteState(nil), e2.sites...), nil
	}
	_, err = e2.svc.Sync(ctx)
	assert.ErrorIs(t, err, ErrSettingsChanged)
	assert.Empty(t, e2.applied)
}

// 服务地址不能带查询参数或 #（否则 /get/{uuid} 落进查询或 # 里，请求会发到别的路径）；
// 重定向不能降级到 http、不能换端口；响应超过上限时拒绝。明确写了 legacy 的不按 fixed 再试。
func TestFetchHardening(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"http://x/admin#ignore", "http://x/admin?a=1", "http://x/?", "mailto:x@example.org", "http://u:p@x"} {
		_, err := Fetch(ctx, nil, bad, "u")
		assert.Error(t, err, bad)
		_, err = normalizeServer(bad)
		assert.ErrorIs(t, err, ErrInvalid, bad)
	}

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.EscapedPath()
		switch r.URL.Path {
		case "/cc/get/port":
			http.Redirect(w, r, "http://127.0.0.1:1"+r.URL.Path, http.StatusFound)
		case "/cc/get/big":
			_, _ = w.Write([]byte(`{"encrypted":"x"}`))
			_, _ = w.Write(bytes.Repeat([]byte(" "), maxBodyBytes))
		default:
			_, _ = w.Write([]byte(`{"encrypted":"x"}`))
		}
	}))
	defer srv.Close()
	_, err := Fetch(ctx, srv.Client(), srv.URL+"/cc/", "a b")
	require.NoError(t, err)
	assert.Equal(t, "/cc/get/a%20b", got, "UUID 按路径的一段转义，接在服务地址的路径后面")
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "port")
	assert.ErrorContains(t, err, "别的端口")
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "big")
	assert.ErrorContains(t, err, "超过")
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/cc"+r.URL.Path, http.StatusFound) // 同一主机的 http 地址
	}))
	defer tlsSrv.Close()
	_, err = Fetch(ctx, tlsSrv.Client(), tlsSrv.URL, "down")
	assert.ErrorContains(t, err, "https 请求重定向到了 http")
	// http 升到别的端口上的 https：不跟随
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, tlsSrv.URL+r.URL.Path, http.StatusFound)
	}))
	defer up.Close()
	_, err = Fetch(ctx, up.Client(), up.URL, "up")
	assert.ErrorContains(t, err, "别的端口")

	_, err = Decrypt(Payload{Encrypted: vecFixed, CryptoType: CryptoLegacy}, vecUUID, vecPassword)
	assert.ErrorIs(t, err, ErrDecrypt, "写明 legacy 的不按 fixed 再试")
}

// 换了数据来源（地址、UUID 或密码）：上次同步的时间与结果清掉，定时同步按新来源马上到期。
func TestSaveSettingsResetsScheduleOnNewSource(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.configure(func(u *SettingsUpdate) { u.AutoSync, u.IntervalHours = true, 24 })
	require.NoError(t, e.svc.RecordSync(ctx, e.now, "更新了 1 个站点的 Cookie（hdsky）"))
	assert.False(t, e.svc.Due(ctx))

	// 只改间隔：保留
	got, err := e.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e.srv.URL, UUID: vecUUID, AutoSync: true, IntervalHours: 12})
	require.NoError(t, err)
	assert.NotNil(t, got.LastSyncAt)
	assert.False(t, e.svc.Due(ctx))

	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{ServerURL: e.srv.URL, UUID: "other-uuid", AutoSync: true, IntervalHours: 12})
	require.NoError(t, err)
	assert.Nil(t, got.LastSyncAt)
	assert.Empty(t, got.LastResult)
	assert.True(t, e.svc.Due(ctx), "新来源马上到期")
}
