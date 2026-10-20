package web

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

// 配置网关对各个服务的接口（多数是严格解析请求体的）：交上去的只有接口认的字段，改一项以后读回来，别的字段不变；
// 密钥给了才改，没给保持原样。

// configRoundTrip 改一项再读回来，回读到的值。
func configRoundTrip(t *testing.T, srv *Server, name string, ch ConfigChange) ConfigChangeResult {
	t.Helper()
	res, err := srv.ConfigApply(context.Background(), name, ch)
	require.NoError(t, err, "%s %s", name, ch.Op)
	require.True(t, res.Applied)
	return res
}

func TestConfigOrganizeResources(t *testing.T) {
	e := newOrganizeServer(t)
	e.srv.SetMediaService(e.rec)
	e.srv.buildHandler()
	ctx := context.Background()

	res := configRoundTrip(t, e.srv, "organize", ConfigChange{Op: "set", Values: map[string]any{"min_video_mb": 300, "scan_interval_min": 60}})
	assert.Equal(t, json.Number("300"), res.Item["min_video_mb"])
	res = configRoundTrip(t, e.srv, "organize", ConfigChange{Op: "set", Values: map[string]any{"tags": []any{"pt"}}})
	assert.Equal(t, json.Number("300"), res.Item["min_video_mb"], "没提到的字段不变")

	lib := configRoundTrip(t, e.srv, "libraries", ConfigChange{Op: "add", Values: map[string]any{
		"name": "电影", "kind": "movie", "path": e.library, "mode": "copy", "enabled": true,
	}})
	require.NotEmpty(t, lib.ID)
	lib = configRoundTrip(t, e.srv, "libraries", ConfigChange{Op: "update", ID: lib.ID, Values: map[string]any{"scrape": true}})
	assert.Equal(t, true, lib.Item["scrape"])
	assert.Equal(t, "电影", lib.Item["name"])

	srvRes := configRoundTrip(t, e.srv, "media_servers", ConfigChange{Op: "add", Values: map[string]any{
		"name": "emby", "kind": "emby", "url": "http://127.0.0.1:8096", "token": "emby-token-123", "enabled": true,
	}})
	assert.Equal(t, ConfigSecretSet, srvRes.Item["token"])
	assert.NotContains(t, configJSON(t, srvRes), "emby-token-123")
	srvRes = configRoundTrip(t, e.srv, "media_servers", ConfigChange{Op: "update", ID: srvRes.ID, Values: map[string]any{"name": "Emby"}})
	assert.Equal(t, ConfigSecretSet, srvRes.Item["token"], "没给令牌：保持原样")
	var ms models.MediaServer
	require.NoError(t, global.GlobalDB.DB.First(&ms, srvRes.ID).Error)
	assert.NotEmpty(t, ms.TokenEncrypted)

	wd := configRoundTrip(t, e.srv, "watch_dirs", ConfigChange{Op: "add", Values: map[string]any{"path": t.TempDir(), "enabled": true}})
	_ = configRoundTrip(t, e.srv, "watch_dirs", ConfigChange{Op: "delete", ID: wd.ID})

	var dl models.DownloaderSetting
	require.NoError(t, global.GlobalDB.DB.First(&dl).Error)
	pm := configRoundTrip(t, e.srv, "media_path_maps", ConfigChange{Op: "add", Values: map[string]any{
		"downloader_id": dl.ID, "downloader_prefix": "/downloads", "local_prefix": e.dlDir,
	}})
	require.NotEmpty(t, pm.ID)

	cls := configRoundTrip(t, e.srv, "classify", ConfigChange{Op: "set", Values: map[string]any{
		"tv": []any{map[string]any{"name": "日剧", "languages": []any{"ja"}}},
	}})
	assert.NotEmpty(t, cls.Item["tv"])
	cls = configRoundTrip(t, e.srv, "classify", ConfigChange{Op: "set", Values: map[string]any{
		"movie": []any{map[string]any{"name": "动画电影", "genres": []any{16}}},
	}})
	assert.NotEmpty(t, cls.Item["movie"])
	assert.NotEmpty(t, cls.Item["tv"], "只改电影的规则：剧集的规则照原样交回")

	// 识别纠正要按 TMDB 核对编号（这个环境的假 TMDB 认识 278）
	ov := configRoundTrip(t, e.srv, "media_overrides", ConfigChange{Op: "add", Values: map[string]any{"title": "某个标题", "tmdb_id": 278, "media_type": "movie"}})
	require.NotEmpty(t, ov.ID)
	_ = configRoundTrip(t, e.srv, "media_overrides", ConfigChange{Op: "delete", ID: ov.ID})

	_, err := e.srv.ConfigApply(ctx, "libraries", ConfigChange{Op: "add", Values: map[string]any{"name": "x", "kind": "music", "path": "/x"}})
	require.Error(t, err, "接口自己的校验照样生效")
}

func TestConfigMediaResources(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	srv.SetMediaService(recognize.New(recognize.Config{DB: db, Cipher: apiTestCipher{}}))
	srv.buildHandler()

	res := configRoundTrip(t, srv, "media", ConfigChange{Op: "set", Values: map[string]any{"tmdb_api_key": "tmdb-key-0123456789", "language": "zh-CN"}})
	assert.Equal(t, ConfigSecretSet, res.Item["tmdb_api_key"])
	assert.NotContains(t, configJSON(t, res), "tmdb-key-0123456789")
	res = configRoundTrip(t, srv, "media", ConfigChange{Op: "set", Values: map[string]any{"language": "en-US"}})
	assert.Equal(t, ConfigSecretSet, res.Item["tmdb_api_key"], "没给 Key：保持原样")
	assert.Equal(t, "en-US", res.Item["language"])

	word := configRoundTrip(t, srv, "media_words", ConfigChange{Op: "add", Values: map[string]any{"kind": "block", "pattern": "国语配音", "enabled": true}})
	word = configRoundTrip(t, srv, "media_words", ConfigChange{Op: "update", ID: word.ID, Values: map[string]any{"note": "备注"}})
	assert.Equal(t, "国语配音", word.Item["pattern"])
}

func TestConfigSubscribeResources(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	srv.buildHandler()
	prof := configRoundTrip(t, srv, "quality_profiles", ConfigChange{Op: "add", Values: map[string]any{"name": "1080p", "resolutions": []any{"1080p"}}})
	require.NotEmpty(t, prof.ID)
	prof = configRoundTrip(t, srv, "quality_profiles", ConfigChange{Op: "update", ID: prof.ID, Values: map[string]any{"min_seeders": 3}})
	assert.Equal(t, json.Number("3"), prof.Item["min_seeders"])
	assert.Equal(t, "1080p", prof.Item["name"])

	res := configRoundTrip(t, srv, "subscribe", ConfigChange{Op: "set", Values: map[string]any{"search_interval_hours": 6}})
	assert.Equal(t, json.Number("6"), res.Item["search_interval_hours"])

	src := configRoundTrip(t, srv, "douban_sources", ConfigChange{Op: "add", Values: map[string]any{"user_id": "123456", "name": "我的想看", "enabled": false}})
	src = configRoundTrip(t, srv, "douban_sources", ConfigChange{Op: "update", ID: src.ID, Values: map[string]any{"name": "想看"}})
	assert.Equal(t, "123456", src.Item["user_id"])
}

func TestConfigBrushAndTransfer(t *testing.T) {
	srv, _, ds := newBrushServer(t)
	srv.buildHandler()
	task := configRoundTrip(t, srv, "brush_tasks", ConfigChange{Op: "add", Values: map[string]any{
		"name": "刷流", "site_name": "hdsky", "downloader_id": ds.ID, "enabled": false, "interval_min": 30, "max_downloading": 2,
	}})
	require.NotEmpty(t, task.ID)
	task = configRoundTrip(t, srv, "brush_tasks", ConfigChange{Op: "update", ID: task.ID, Values: map[string]any{"max_seeders": 5}})
	assert.Equal(t, json.Number("5"), task.Item["max_seeders"])
	assert.Equal(t, json.Number("30"), task.Item["interval_min"], "没提到的字段不变")
	_ = configRoundTrip(t, srv, "brush_tasks", ConfigChange{Op: "delete", ID: task.ID})

	tsrv, _, src, dst := newTransferServer(t)
	tsrv.buildHandler()
	rule := configRoundTrip(t, tsrv, "transfer_rules", ConfigChange{Op: "add", Values: map[string]any{
		"name": "转移", "source_downloader_id": src.ID, "target_downloader_id": dst.ID, "min_seeding_hours": 24, "enabled": false,
	}})
	require.NotEmpty(t, rule.ID)
	rule = configRoundTrip(t, tsrv, "transfer_rules", ConfigChange{Op: "update", ID: rule.ID, Values: map[string]any{"max_per_run": 3}})
	assert.Equal(t, json.Number("24"), rule.Item["min_seeding_hours"])
}

// 辅种与 CookieCloud：令牌、密码给了才改；CookieCloud 的 UUID 读出来遮住，但整份保存时照原值交回（不交就清掉了）。
func TestConfigReseedAndCookieCloud(t *testing.T) {
	srv, _ := newReseedServer(t)
	srv.buildHandler()
	res := configRoundTrip(t, srv, "reseed", ConfigChange{Op: "set", Values: map[string]any{"token": "iyuu-token-0123", "interval_hours": 12}})
	assert.Equal(t, ConfigSecretSet, res.Item["token"])
	res = configRoundTrip(t, srv, "reseed", ConfigChange{Op: "set", Values: map[string]any{"interval_hours": 24}})
	assert.Equal(t, ConfigSecretSet, res.Item["token"], "没给令牌：保持原样")
	assert.NotContains(t, configJSON(t, res), "iyuu-token")

	cc := newCookieCloudServer(t)
	cc.srv.buildHandler()
	got := configRoundTrip(t, cc.srv, "cookiecloud", ConfigChange{Op: "set", Values: map[string]any{
		"server_url": cc.server.URL, "uuid": "u1", "password": "pw", "interval_hours": 6,
	}})
	assert.Equal(t, ConfigSecretSet, got.Item["uuid"])
	assert.Equal(t, ConfigSecretSet, got.Item["password"])
	got = configRoundTrip(t, cc.srv, "cookiecloud", ConfigChange{Op: "set", Values: map[string]any{"interval_hours": 12}})
	assert.Equal(t, ConfigSecretSet, got.Item["uuid"], "UUID 照原值交回")
	var row models.CookieCloudSetting
	require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
	assert.Equal(t, "u1", row.UUID)
	assert.NotContains(t, configJSON(t, got), `"u1"`)
}

func TestConfigCloakDailyAndDeadScan(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()
	srv.buildHandler()
	res := configRoundTrip(t, srv, "cloak", ConfigChange{Op: "set", Values: map[string]any{"endpoint": "http://127.0.0.1:8080", "token": "cloak-token-1"}})
	assert.Equal(t, ConfigSecretSet, res.Item["token"])
	res = configRoundTrip(t, srv, "cloak", ConfigChange{Op: "set", Values: map[string]any{"profile_id": "p1"}})
	assert.Equal(t, ConfigSecretSet, res.Item["token"], "没给令牌：保持原样")
	assert.Equal(t, "http://127.0.0.1:8080", res.Item["endpoint"])
}
