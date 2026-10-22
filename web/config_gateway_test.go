package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 配置网关：经真的路由（buildHandler）读改配置。密钥只写不读；改动并进现状，再交给网页用的同一个接口保存；
// 预览列出改以前、改以后；执行以后读回改完的样子。

// newConfigEnv 起一个接好令牌、通知通道与消息模板的服务（handler 是完整的路由，配置网关经它调接口）。
func newConfigEnv(t *testing.T) *appEnv {
	t.Helper()
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	srv.mgr.StopAll()
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.APIToken{}, &models.NotificationConf{}, &models.NotificationTemplate{}, &models.SiteLoginState{}))
	e := &appEnv{t: t, srv: srv, tokens: apitoken.New(global.GlobalDB.DB), audit: &fakeAppAudit{}}
	srv.SetAPITokens(e.tokens, e.audit)
	srv.SetChatOpsDeps(&ChatOpsDeps{
		NotificationSvc: app.NewNotificationService(global.GlobalDB.DB, nil, 0),
		BindingSvc:      &stubBindingSvc{}, AuditSvc: &stubAuditSvc{},
		TemplateSvc: app.NewTemplateService(global.GlobalDB.DB, notify.NewTemplates(nil)),
	})
	srv.sessions.put("sess-app", "admin")
	enableMCPForTest(t)
	e.handler = srv.buildHandler()
	return e
}

// configStatus 是网关错误的状态码（不是网关错误时为 0）。
func configStatus(err error) int {
	var ce *configError
	if errors.As(err, &ce) {
		return ce.Status
	}
	return 0
}

func configJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// 全局设置：只给看它自己的字段；预览不改东西；只交并进改动以后的整份设置，没提到的字段不变；字符串数字按现状换成数字；
// 不认识的字段、和现在一样的值、对象类做不了的操作都说清楚；接口自己的校验（下载目录不能为空）照样生效。
func TestConfigGlobal(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	st, err := e.srv.ConfigRead(ctx, "global", "")
	require.NoError(t, err)
	assert.Equal(t, configObject, st.Kind)
	assert.Equal(t, "download", st.Item["download_dir"])
	assert.NotContains(t, st.Item, "attendance_window_start", "签到的字段归 attendance")
	assert.NotContains(t, st.Item, "id")

	_, err = e.srv.ConfigApply(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"cleanup_enabled": true, "default_interval_minutes": "30"}})
	require.NoError(t, err)
	dir := t.TempDir()
	pv, err := e.srv.ConfigPreview(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"download_dir": dir}})
	require.NoError(t, err)
	assert.False(t, pv.Applied)
	assert.Equal(t, []ConfigDiff{{Label: "种子下载目录（download_dir）", Field: "download_dir", Before: "download", After: dir}}, pv.Changes)
	assert.Equal(t, "改全局设置的 1 项", pv.Summary)
	assert.NotEmpty(t, pv.Impact)
	gs, err := e.srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, "download", gs.DownloadDir, "预览不改")

	res, err := e.srv.ConfigApply(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"download_dir": dir}})
	require.NoError(t, err)
	assert.True(t, res.Applied)
	assert.Equal(t, dir, res.Item["download_dir"])
	gs, err = e.srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, dir, gs.DownloadDir)
	assert.True(t, gs.CleanupEnabled, "没提到的字段不变")
	assert.EqualValues(t, 30, gs.DefaultIntervalMinutes)

	_, err = e.srv.ConfigPreview(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"nope": 1, "id": 3}})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, configStatus(err))
	assert.Contains(t, err.Error(), "不能改 id、nope")
	assert.Contains(t, err.Error(), "download_dir", "写明能改的字段")
	_, err = e.srv.ConfigPreview(ctx, "global", ConfigChange{Op: "add", Values: map[string]any{"download_dir": "/x"}})
	assert.Contains(t, err.Error(), "只能 set")
	_, err = e.srv.ConfigPreview(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"download_dir": dir}})
	assert.Contains(t, err.Error(), "没有要改的")
	_, err = e.srv.ConfigApply(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"download_dir": " "}})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, configStatus(err))
	assert.Contains(t, err.Error(), "下载目录不能为空")
	_, err = e.srv.ConfigRead(ctx, "nope", "")
	assert.Equal(t, http.StatusNotFound, configStatus(err))
}

// 下载器：新增时密码只写不读（预览写「已更新」，读出来没有这一项）；改的时候交回占位不算改；删掉的写明不能恢复。
func TestConfigDownloaders(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	const pw = "secret-pass-1"
	first, err := e.srv.ConfigApply(ctx, "downloaders", ConfigChange{Op: "add", Values: map[string]any{
		"name": "qb1", "type": "qbittorrent", "url": "http://127.0.0.1:18080", "enabled": true,
	}})
	require.NoError(t, err)
	require.NotEmpty(t, first.ID)

	add := ConfigChange{Op: "add", Values: map[string]any{
		"name": "qb2", "type": "qbittorrent", "url": "http://127.0.0.1:18081", "username": "admin", "password": pw, "enabled": true,
	}}
	pv, err := e.srv.ConfigPreview(ctx, "downloaders", add)
	require.NoError(t, err)
	assert.Equal(t, "新增下载器「qb2」", pv.Summary)
	assert.Contains(t, pv.Changes, ConfigDiff{Label: "密码（password）", Field: "password", After: "已更新（不显示）", Secret: true})
	res, err := e.srv.ConfigApply(ctx, "downloaders", add)
	require.NoError(t, err)
	assert.Equal(t, "qb2", res.Item["name"])
	assert.NotContains(t, res.Item, "password")
	assert.NotContains(t, configJSON(t, res), pw)

	var row models.DownloaderSetting
	require.NoError(t, global.GlobalDB.DB.Where("name = ?", "qb2").First(&row).Error)
	assert.Equal(t, pw, row.Password)
	assert.False(t, row.IsDefault, "第一台才自动设为默认")

	_, err = e.srv.ConfigPreview(ctx, "downloaders", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{"password": ConfigSecretSet}})
	assert.Contains(t, err.Error(), "没有要改的", "交回占位表示不改")
	_, err = e.srv.ConfigApply(ctx, "downloaders", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{"password": ConfigSecretSet, "auto_start": true}})
	require.NoError(t, err)
	require.NoError(t, global.GlobalDB.DB.First(&row, row.ID).Error)
	assert.True(t, row.AutoStart)
	assert.Equal(t, pw, row.Password, "密码没动")
	assert.Equal(t, "admin", row.Username, "没提到的字段照原样交回")

	st, err := e.srv.ConfigRead(ctx, "downloaders", "")
	require.NoError(t, err)
	assert.Len(t, st.Items, 2)
	assert.NotContains(t, configJSON(t, st), pw)

	_, err = e.srv.ConfigPreview(ctx, "downloaders", ConfigChange{Op: "update", ID: "999", Values: map[string]any{"enabled": false}})
	assert.Equal(t, http.StatusNotFound, configStatus(err))
	_, err = e.srv.ConfigPreview(ctx, "downloaders", ConfigChange{Op: "add", Values: map[string]any{"name": "x"}})
	assert.Contains(t, err.Error(), "要给 type、url")
	_, err = e.srv.ConfigPreview(ctx, "downloaders", ConfigChange{Op: "update", Values: map[string]any{"enabled": false}})
	assert.Contains(t, err.Error(), "要给 id")

	del, err := e.srv.ConfigPreview(ctx, "downloaders", ConfigChange{Op: "delete", ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, "删除下载器「qb2」", del.Summary)
	assert.Contains(t, strings.Join(del.Impact, "\n"), "不能恢复")
	_, err = e.srv.ConfigApply(ctx, "downloaders", ConfigChange{Op: "delete", ID: res.ID})
	require.NoError(t, err)
	st, err = e.srv.ConfigRead(ctx, "downloaders", "")
	require.NoError(t, err)
	assert.Len(t, st.Items, 1)
}

// 站点：读出来的 Passkey、Cookie 只说有没有；启用时要凭据齐全；改站点不重建 RSS（编号不变）。
// RSS：地址只写不读；新增、修改（编号不变，没给的字段不变）、删除都走 ConfigStore；过滤规则、站点要存在。
func TestConfigSitesAndRSS(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	db := global.GlobalDB.DB
	site := models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", IsBuiltin: true, Passkey: "pk-secret-123456"}
	require.NoError(t, db.Create(&site).Error)
	dl := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, IsDefault: true}
	require.NoError(t, db.Create(&dl).Error)
	rule := models.FilterRule{Name: "free", Pattern: "*", Enabled: true}
	require.NoError(t, db.Create(&rule).Error)

	st, err := e.srv.ConfigRead(ctx, "sites", "hdsky")
	require.NoError(t, err)
	require.Len(t, st.Items, 1)
	assert.Equal(t, ConfigSecretSet, st.Items[0]["passkey"])
	assert.Equal(t, "", st.Items[0]["cookie"])
	assert.Equal(t, false, st.Items[0]["enabled"])

	_, err = e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"enabled": true}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cookie", "没有 Cookie 不能启用")

	const cookie = "uid=1; pass=abcdef0123"
	res, err := e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{
		"enabled": true, "cookie": cookie, "upload_limit_kbs": 100,
	}})
	require.NoError(t, err)
	assert.Equal(t, ConfigSecretSet, res.Item["cookie"])
	assert.NotContains(t, configJSON(t, res), "pass=abcdef")
	assert.NotContains(t, configJSON(t, res), "pk-secret")
	require.NoError(t, db.First(&site, site.ID).Error)
	assert.True(t, site.Enabled)
	assert.Equal(t, 100, site.UploadLimitKBs)
	plain, err := e.srv.store.DecryptCookie(site.CookieEncrypted)
	require.NoError(t, err)
	assert.Equal(t, cookie, plain)

	def, _ := v2.GetDefinitionRegistry().Get("hdsky")
	supported, _ := v2.AttendanceSupport(def)
	_, err = e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"attendance_enabled": true}})
	if supported {
		require.NoError(t, err)
		require.NoError(t, db.First(&site, site.ID).Error)
		assert.True(t, site.AttendanceEnabled)
	} else {
		assert.Contains(t, err.Error(), "不能开启自动签到")
	}

	const rssURL = "https://hdsky.me/torrentrss.php?rows=10&passkey=abcdef0123456789"
	added, err := e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "add", Values: map[string]any{
		"site": "hdsky", "name": "电影", "url": rssURL, "downloader_id": dl.ID, "filter_rule_ids": []any{rule.ID}, "notify_conf_ids": []any{1, 2},
	}})
	require.NoError(t, err)
	require.NotEmpty(t, added.ID)
	assert.Equal(t, ConfigSecretSet, added.Item["url"])
	assert.Equal(t, "[1,2]", added.Item["notify_conf_ids"], "数组写成 JSON 字符串")
	assert.NotContains(t, configJSON(t, added), "abcdef0123456789")

	_, err = e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "update", ID: added.ID, Values: map[string]any{"interval_minutes": 30, "filter_rule_ids": []any{}}})
	require.NoError(t, err)
	var sub models.RSSSubscription
	require.NoError(t, db.Where("site_id = ?", site.ID).First(&sub).Error)
	assert.Equal(t, added.ID, strconv.FormatUint(uint64(sub.ID), 10), "编号不变")
	assert.EqualValues(t, 30, sub.IntervalMinutes)
	assert.Equal(t, rssURL, sub.URL, "地址没动")
	ids, err := models.NewRSSFilterAssociationDB(db).GetFilterRuleIDsForRSS(sub.ID)
	require.NoError(t, err)
	assert.Empty(t, ids)

	_, err = e.srv.ConfigApply(ctx, "sites", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"download_limit_kbs": 50}})
	require.NoError(t, err)
	var again models.RSSSubscription
	require.NoError(t, db.First(&again, sub.ID).Error, "改站点不重建 RSS")

	_, err = e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "add", Values: map[string]any{"site": "hdsky", "name": "x", "url": "https://hdsky.me/a", "filter_rule_ids": []any{999}}})
	assert.Contains(t, err.Error(), "过滤规则不存在")
	_, err = e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "add", Values: map[string]any{"site": "nope-site", "name": "x", "url": "https://x/a"}})
	require.Error(t, err)
	_, err = e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "update", ID: added.ID, Values: map[string]any{"site": "mteam"}})
	assert.Contains(t, err.Error(), "不能改 site", "站点只在新增时给")

	st, err = e.srv.ConfigRead(ctx, "rss", "")
	require.NoError(t, err)
	assert.NotContains(t, configJSON(t, st), "abcdef0123456789")
	_, err = e.srv.ConfigApply(ctx, "rss", ConfigChange{Op: "delete", ID: added.ID})
	require.NoError(t, err)
	var n int64
	require.NoError(t, db.Model(&models.RSSSubscription{}).Count(&n).Error)
	assert.Zero(t, n)
}

// 通知通道：配置里的令牌、地址这类键只写不读；改配置只交改了的键（服务端按键合并），也收 {"config": {...}} 的写法。
func TestConfigNotifyChannels(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	const endpoint = "https://hooks.example.com/abc?token=t0p-secret-value"
	res, err := e.srv.ConfigApply(ctx, "notify_channels", ConfigChange{Op: "add", Values: map[string]any{
		"channel_type": "webhook", "name": "hook", "enabled": true,
		"config": map[string]any{"endpoint_url": endpoint, "method": "POST"},
	}})
	require.NoError(t, err)
	require.NotEmpty(t, res.ID)
	assert.Equal(t, ConfigSecretSet, res.Item["config.endpoint_url"])
	assert.Equal(t, "POST", res.Item["config.method"])
	assert.NotContains(t, configJSON(t, res), "t0p-secret")

	_, err = e.srv.ConfigApply(ctx, "notify_channels", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{
		"config.method": "PUT", "config.endpoint_url": ConfigSecretSet, "name": "钩子",
	}})
	require.NoError(t, err)
	cfg := e.channelConfig(t, res.ID)
	assert.Equal(t, "PUT", cfg["method"])
	assert.Equal(t, endpoint, cfg["endpoint_url"], "交回占位：地址不变")

	pv, err := e.srv.ConfigPreview(ctx, "notify_channels", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{
		"config": map[string]any{"endpoint_url": "https://hooks.example.com/new"},
	}})
	require.NoError(t, err)
	require.Len(t, pv.Changes, 1)
	assert.True(t, pv.Changes[0].Secret)
	assert.Equal(t, "已设置", pv.Changes[0].Before)
	_, err = e.srv.ConfigApply(ctx, "notify_channels", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{
		"config": map[string]any{"endpoint_url": "https://hooks.example.com/new"},
	}})
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.example.com/new", e.channelConfig(t, res.ID)["endpoint_url"])

	st, err := e.srv.ConfigRead(ctx, "notify_channels", res.ID)
	require.NoError(t, err)
	assert.Equal(t, "钩子", st.Items[0]["name"])
	assert.NotContains(t, configJSON(t, st), "hooks.example.com")
	_, err = e.srv.ConfigApply(ctx, "notify_channels", ConfigChange{Op: "update", ID: res.ID, Values: map[string]any{"channel_type": "telegram"}})
	assert.Contains(t, err.Error(), "不能改 channel_type", "类型只在新增时给")
	_, err = e.srv.ConfigApply(ctx, "notify_channels", ConfigChange{Op: "delete", ID: res.ID})
	require.NoError(t, err)
}

// channelConfig 读通知通道解密以后的配置。
func (e *appEnv) channelConfig(t *testing.T, id string) map[string]any {
	t.Helper()
	var row models.NotificationConf
	require.NoError(t, global.GlobalDB.DB.First(&row, id).Error)
	plain, err := crypto.Decrypt(row.ConfigJSON)
	require.NoError(t, err)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(plain, &cfg))
	return cfg
}

// 消息模板：改了标为自定义，模板写错时接口回原因；「删除」是恢复默认。
func TestConfigTemplates(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	st, err := e.srv.ConfigRead(ctx, "notify_templates", "rss.matched")
	require.NoError(t, err)
	require.Len(t, st.Items, 1)
	assert.Equal(t, false, st.Items[0]["customized"])
	vars, _ := st.Items[0]["vars"].([]any)
	assert.Contains(t, vars, "site", "变量只列名字")

	res, err := e.srv.ConfigApply(ctx, "notify_templates", ConfigChange{Op: "update", ID: "rss.matched", Values: map[string]any{"text": "{{.torrent_title}} 命中"}})
	require.NoError(t, err)
	assert.Equal(t, true, res.Item["customized"])
	_, err = e.srv.ConfigApply(ctx, "notify_templates", ConfigChange{Op: "update", ID: "rss.matched", Values: map[string]any{"text": "{{.nope"}})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, configStatus(err))

	pv, err := e.srv.ConfigPreview(ctx, "notify_templates", ConfigChange{Op: "delete", ID: "rss.matched"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pv.Summary, "恢复默认"), pv.Summary)
	assert.NotContains(t, strings.Join(pv.Impact, ""), "不能恢复")
	_, err = e.srv.ConfigApply(ctx, "notify_templates", ConfigChange{Op: "delete", ID: "rss.matched"})
	require.NoError(t, err)
	st, err = e.srv.ConfigRead(ctx, "notify_templates", "rss.matched")
	require.NoError(t, err)
	assert.Equal(t, false, st.Items[0]["customized"])
}

// 签到、战报、日志级别：对象类设置照现状合并后保存；登录提醒只交改了的几项。
func TestConfigSmallObjects(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	// 签到的时间窗存在全局设置那一行里：先保存过一次全局设置（装好以后都有）
	_, err := e.srv.ConfigApply(ctx, "global", ConfigChange{Op: "set", Values: map[string]any{"download_dir": t.TempDir()}})
	require.NoError(t, err)
	_, err = e.srv.ConfigApply(ctx, "attendance", ConfigChange{Op: "set", Values: map[string]any{"window_start": "07:30"}})
	require.NoError(t, err)
	start, end, err := e.srv.store.AttendanceWindow()
	require.NoError(t, err)
	assert.Equal(t, "07:30", start)
	assert.Equal(t, "10:00", end, "没提到的字段不变")

	_, err = e.srv.ConfigApply(ctx, "log_level", ConfigChange{Op: "set", Values: map[string]any{"level": "nope"}})
	assert.Equal(t, http.StatusBadRequest, configStatus(err))

	daily, err := e.srv.ConfigApply(ctx, "daily_report", ConfigChange{Op: "set", Values: map[string]any{"time": "21:30"}})
	require.NoError(t, err)
	assert.Equal(t, "21:30", daily.Item["time"])
	dead, err := e.srv.ConfigApply(ctx, "dead_scan", ConfigChange{Op: "set", Values: map[string]any{"interval_hours": 48}})
	require.NoError(t, err)
	assert.Equal(t, json.Number("48"), dead.Item["interval_hours"])
	_, err = e.srv.ConfigApply(ctx, "dead_scan", ConfigChange{Op: "set", Values: map[string]any{"enabled": true}})
	assert.Equal(t, http.StatusBadRequest, configStatus(err), "打开时要选通知通道：接口的校验照样生效")

	rule, err := e.srv.ConfigApply(ctx, "filter_rules", ConfigChange{Op: "add", Values: map[string]any{"name": "4K", "pattern": "2160p", "pattern_type": "keyword", "enabled": true}})
	require.NoError(t, err)
	rule, err = e.srv.ConfigApply(ctx, "filter_rules", ConfigChange{Op: "update", ID: rule.ID, Values: map[string]any{"priority": 5}})
	require.NoError(t, err)
	assert.Equal(t, "2160p", rule.Item["pattern"], "没提到的字段不变")
	assert.Equal(t, json.Number("5"), rule.Item["priority"])

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie", Enabled: true, CookieEncrypted: "x", IsBuiltin: true}).Error)
	_, err = e.srv.ConfigApply(ctx, "login_reminders", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"remind_before_days": 3}})
	require.NoError(t, err)
	st, err := e.srv.ConfigRead(ctx, "login_reminders", "hdsky")
	require.NoError(t, err)
	assert.EqualValues(t, json.Number("3"), st.Items[0]["remind_before_days"])
	assert.NotContains(t, st.Items[0], "last_login_at", "只给看配置与几项状态")
	_, err = e.srv.ConfigApply(ctx, "login_reminders", ConfigChange{Op: "update", ID: "hdsky", Values: map[string]any{"probe_mode": "sometimes"}})
	assert.Equal(t, http.StatusBadRequest, configStatus(err))
}

// 登记的资源都对得上接口：读的地址挂着（没启用的服务回 503，不是落到网页的兜底），列表的单项地址挂着；
// 密钥不在普通字段里，必填的都能写，操作与种类认识。
func TestConfigResourcesWired(t *testing.T) {
	e := newConfigEnv(t)
	ctx := context.Background()
	names := map[string]bool{}
	for _, r := range e.srv.configResources() {
		assert.False(t, names[r.Name], "重复的资源 %s", r.Name)
		names[r.Name] = true
		assert.Contains(t, []string{configObject, configList}, r.Kind, r.Name)
		assert.NotEmpty(t, r.Title, r.Name)
		assert.NotEmpty(t, r.Desc, r.Name)
		require.NotEmpty(t, r.Ops, r.Name)
		for _, op := range r.Ops {
			if r.Kind == configObject {
				assert.Equal(t, configOpSet, op, r.Name)
			} else {
				assert.Contains(t, []string{configOpAdd, configOpUpdate, configOpDelete}, op, r.Name)
			}
		}
		assert.NotEmpty(t, r.fieldList(), r.Name)
		for k := range r.Secrets {
			assert.NotContains(t, r.Fields, k, "%s 的密钥 %s 不该在普通字段里", r.Name, k)
		}
		for _, k := range r.Required {
			assert.True(t, r.writable(k, configOpAdd), "%s 的必填字段 %s 要能写", r.Name, k)
		}
		if r.load == nil {
			status, _, err := e.srv.configCall(ctx, http.MethodGet, r.Path, nil)
			require.NoError(t, err)
			assert.NotEqual(t, http.StatusFound, status, "%s 的 %s 没挂上", r.Name, r.Path)
			assert.NotEqual(t, http.StatusNotFound, status, "%s 的 %s 没挂上", r.Name, r.Path)
		}
		if r.Kind == configList && r.ItemPath != "" && r.save == nil {
			path := strings.ReplaceAll(r.ItemPath, "{id}", "987654321")
			status, _, err := e.srv.configCall(ctx, http.MethodDelete, path, nil)
			require.NoError(t, err)
			assert.NotEqual(t, http.StatusFound, status, "%s 的 %s 没挂上", r.Name, path)
		}
	}
	cat := e.srv.ConfigCatalog()
	assert.Len(t, cat.Resources, len(names))
	assert.Equal(t, ConfigSecretSet, cat.SecretPlaceholder)
	assert.Contains(t, strings.Join(cat.Excluded, "、"), "API 令牌")
}

// 网关在进程内调接口不经登录；外面的请求照样要登录（标记只能在进程里放进 context）。
func TestConfigInternalCallsOnly(t *testing.T) {
	e := newConfigEnv(t)
	status, _, err := e.srv.configCall(context.Background(), http.MethodGet, "/api/global", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	w := e.do(appReq{method: http.MethodGet, path: "/api/global"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	bare := setupServer(t)
	_, _, err = bare.configCall(context.Background(), http.MethodGet, "/api/global", nil)
	assert.Equal(t, http.StatusServiceUnavailable, configStatus(err), "路由还没建好")
}
