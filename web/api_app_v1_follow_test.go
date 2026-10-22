package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// appWithEpisode 调带 {id} 与 {n} 的处理函数。
func appWithEpisode(t testing.TB, h http.HandlerFunc, target, id, n, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.SetPathValue("id", id)
	req.SetPathValue("n", n)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read", "app:write"}}))
	w := httptest.NewRecorder()
	h(w, req)
	validateAppResponse(t, req, w)
	return w
}

// 生成的 Dart 客户端只改定时时发的就是 testdata/app_subscription_update_schedule_only.json（没设的字段都是 null；
// apps/mobile/test/app/subscription_update_test.dart 断言客户端发出的和这个文件逐字一样）：站点限制、下载器、保存路径都不变
// （以前客户端把 sites 默认成空数组，等于改成全部站点）
func TestAppSubscriptionUpdateFromGeneratedClient(t *testing.T) {
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.SiteNames = func() []string { return []string{"hdsky"} } })
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"tv","tmdb_id":1001,"season":1}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	id := strconv.Itoa(int(sub.ID))
	dl := models.DownloaderSetting{Name: "qb-dart", Type: "qbittorrent", URL: "http://qb.local", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&dl).Error)
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id,
		`{"sites":["hdsky"],"downloader_id":`+strconv.Itoa(int(dl.ID))+`,"save_path":"/downloads/tv"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 两边共用的这份 JSON：Dart 的测试断言生成的客户端发出的就是它（逐字一样）
	dart, err := os.ReadFile("testdata/app_subscription_update_schedule_only.json")
	require.NoError(t, err)
	dartScheduleOnly := strings.TrimSpace(string(dart))
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, dartScheduleOnly)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, []string{"hdsky"}, got.Sites, "null 是不改，不是全部站点")
	assert.Equal(t, dl.ID, got.DownloaderID)
	assert.Equal(t, "/downloads/tv", got.SavePath)
	assert.Equal(t, "cron", got.Schedule.Kind)
	assert.Equal(t, "0 22 * * 5", got.Schedule.Cron)
}

// 追剧：新建按默认设置、修改只改给了的字段、每一集、跳过与换版本、运行记录（去掉下载器地址）、集号规则
func TestAppFollow(t *testing.T) {
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.SiteNames = func() []string { return []string{"hdsky"} } })
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"tv","tmdb_id":1001,"season":1}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, "airtime", sub.Schedule.Kind, "新建的剧集按播出时间搜索")
	assert.True(t, sub.LockVersion, "新建的剧集开着版本锁")
	assert.Equal(t, 2, sub.Progress.Required)
	assert.Equal(t, []int{1, 2}, sub.Progress.Missing)
	id := strconv.Itoa(int(sub.ID))

	// 指定了的不按默认
	w = appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions",
		`{"media_type":"tv","tmdb_id":1001,"season":1,"schedule":{"kind":"daily","at":"7:00"},"lock_version":false}`, "app:write")
	assert.Equal(t, http.StatusBadRequest, w.Code, "同一季已经订阅了")
	for _, body := range []string{
		`{"media_type":"movie","tmdb_id":278,"schedule":{"kind":"airtime"}}`,
		`{"media_type":"tv","tmdb_id":1001,"from_torrent":{"site":"","torrent_id":"1"}}`,
		`{"media_type":"tv","tmdb_id":1001,"start_episode":-1}`,
	} {
		assert.Equal(t, http.StatusBadRequest, appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", body, "app:write").Code, body)
	}

	// 修改：只改给了的字段，钟点存成 HH:MM
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id,
		`{"schedule":{"kind":"weekly","at":"9:30","days":[6,0]},"lock_group":"HHWEB","start_episode":2}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, AppSchedule{Kind: "weekly", At: "09:30", Days: []int{0, 6}}, sub.Schedule)
	assert.Equal(t, "HHWEB", sub.LockGroup)
	assert.Equal(t, 2, sub.StartEpisode)
	assert.True(t, sub.LockVersion, "没给的不变")
	assert.Equal(t, 1, sub.Progress.Required, "从第 2 集开始追")
	for _, body := range []string{`{"schedule":{"kind":"hourly"}}`, `{"sites":["x"]}`, `{"total_episodes":2001}`, `{"library_id":99}`} {
		assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, body).Code, body)
	}
	assert.Equal(t, http.StatusNotFound, appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/99", "99", `{}`).Code)

	// 在哪找、推到哪：站点与下载器要存在，没给的不变，空数组是全部站点
	dl := models.DownloaderSetting{Name: "qb-follow", Type: "qbittorrent", URL: "http://qb.local", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&dl).Error)
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, `{"downloader_id":999}`).Code)
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id,
		`{"sites":["hdsky"],"downloader_id":`+strconv.Itoa(int(dl.ID))+`,"category":"tv","tags":"追剧,综艺","save_path":"/downloads/tv"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var placed AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &placed))
	assert.Equal(t, []string{"hdsky"}, placed.Sites)
	assert.Equal(t, dl.ID, placed.DownloaderID)
	assert.Equal(t, "tv", placed.Category)
	assert.Equal(t, "追剧,综艺", placed.Tags)
	assert.Equal(t, "/downloads/tv", placed.SavePath)
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, `{"sites":[],"lock_group":"HHWEB"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	placed = AppSubscription{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &placed))
	assert.Empty(t, placed.Sites, "空数组是全部站点")
	assert.Equal(t, dl.ID, placed.DownloaderID, "没给的不变")
	assert.Equal(t, "/downloads/tv", placed.SavePath)
	for _, body := range []string{
		`{"media_type":"tv","tmdb_id":1002,"season":1,"sites":["nope"]}`,
		`{"media_type":"tv","tmdb_id":1002,"season":1,"downloader_id":999}`,
	} {
		assert.Equal(t, http.StatusBadRequest, appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", body, "app:write").Code, body)
	}
	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, `{"start_episode":0,"lock_group":"","lock_resolution":""}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var cleared AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cleared))
	assert.Empty(t, cleared.LockGroup)
	assert.Zero(t, cleared.StartEpisode)
	assert.Equal(t, "weekly", cleared.Schedule.Kind)

	// 每一集；跳过第 1 集、第 2 集换版本
	w = appWithID(t, srv.appSubscriptionEpisodes, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/episodes", id, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var eps []AppEpisode
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &eps))
	require.Len(t, eps, 2)
	assert.Equal(t, "第1期上", eps[0].Name)
	assert.Equal(t, "missing", eps[0].State)

	for _, c := range []struct{ n, body string }{{"1", `{}`}, {"0", `{"skip":true}`}, {"x", `{"skip":true}`}, {"9", `{"skip":true}`}, {"1", `{"skip":true,"x":1}`}} {
		assert.Equal(t, http.StatusBadRequest, appWithEpisode(t, srv.appSubscriptionEpisodeSkip, "/api/app/v1/subscriptions/"+id+"/episodes/"+c.n+"/skip", id, c.n, c.body).Code, c)
	}
	w = appWithEpisode(t, srv.appSubscriptionEpisodeSkip, "/api/app/v1/subscriptions/"+id+"/episodes/1/skip", id, "1", `{"skip":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &eps))
	assert.Equal(t, "skipped", eps[0].State)
	assert.True(t, eps[0].ManualSkip)
	assert.Equal(t, http.StatusBadRequest, appWithEpisode(t, srv.appSubscriptionEpisodeUnlock, "/api/app/v1/subscriptions/"+id+"/episodes/2/unlock", id, "2", `{}`).Code)
	w = appWithEpisode(t, srv.appSubscriptionEpisodeUnlock, "/api/app/v1/subscriptions/"+id+"/episodes/2/unlock", id, "2", `{"unlock":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &eps))
	assert.True(t, eps[1].ManualUnlock)

	// 集号规则
	w = appWithID(t, srv.appEpisodeRuleSave, http.MethodPut, "/api/app/v1/subscriptions/"+id+"/episode-rules", id, `{"group":"XX","mode":"offset","offset":1,"note":"E02 是第 3 集"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var rules []AppEpisodeRule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rules))
	require.Len(t, rules, 1)
	assert.Equal(t, AppEpisodeRule{Group: "XX", Mode: "offset", Offset: 1, Source: "manual", Note: "E02 是第 3 集", UpdatedAt: rules[0].UpdatedAt}, rules[0])
	for _, body := range []string{`{"mode":"guess"}`, `{"mode":"offset"}`, `{"mode":"tmdb","source":"ai"}`} {
		assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appEpisodeRuleSave, http.MethodPut, "/api/app/v1/subscriptions/"+id+"/episode-rules", id, body).Code, body)
	}
	w = appWithID(t, srv.appEpisodeRules, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/episode-rules", id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"group":"XX"`)
	w = appWithID(t, srv.appEpisodeRuleDelete, http.MethodDelete, "/api/app/v1/subscriptions/"+id+"/episode-rules?group=XX", id, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "[]\n", w.Body.String())
	assert.Equal(t, http.StatusNotFound, appWithID(t, srv.appEpisodeRuleDelete, http.MethodDelete, "/api/app/v1/subscriptions/"+id+"/episode-rules?group=XX", id, "").Code)

	// 运行记录：新的在前，分页；交出去时去掉下载器的地址
	require.NoError(t, global.GlobalDB.DB.Create(&models.MediaSubscriptionRun{
		SubscriptionID: sub.ID, Trigger: models.MediaRunSchedule, StartedAt: time.Now(), Status: models.MediaRunFailed,
		Summary: `推送失败: Post "http://10.0.0.5:8080/api/v2/torrents/add": connection refused`,
		Steps:   `[{"kind":"fail","text":"推送失败: dial tcp 10.0.0.5:8080: connect: connection refused"}]`,
	}).Error)
	w = appWithID(t, srv.appSubscriptionRuns, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/runs?page_size=2", id, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
	var runs AppPage[AppSubscriptionRun]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &runs))
	// 建立、两次修改、跳过、换版本、设规则、删规则，加上面那条
	assert.Equal(t, 8, runs.Total)
	require.Len(t, runs.Items, 2)
	assert.Equal(t, models.MediaRunFailed, runs.Items[0].Status)
	require.Len(t, runs.Items[0].Steps, 1)
	assert.Contains(t, runs.Items[0].Steps[0].Text, "connection refused")
	w = appWithID(t, srv.appSubscriptionRuns, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/runs?page=4&page_size=2", id, "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &runs))
	require.Len(t, runs.Items, 2)
	assert.Equal(t, models.MediaRunCreate, runs.Items[1].Trigger, "最早的一条是建立")
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionRuns, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/runs?page=0", id, "").Code)
	assert.Equal(t, http.StatusNotFound, appWithID(t, srv.appSubscriptionRuns, http.MethodGet, "/api/app/v1/subscriptions/99/runs", "99", "").Code)

	// 电影没有分集与集号规则
	w = appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"movie","tmdb_id":278}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var movie AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &movie))
	assert.False(t, movie.LockVersion)
	assert.Empty(t, movie.Schedule.Kind)
	mid := strconv.Itoa(int(movie.ID))
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionEpisodes, http.MethodGet, "/api/app/v1/subscriptions/"+mid+"/episodes", mid, "").Code)
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appEpisodeRuleSave, http.MethodPut, "/api/app/v1/subscriptions/"+mid+"/episode-rules", mid, `{"mode":"tmdb"}`).Code)
	assert.Equal(t, http.StatusBadRequest, appWithEpisode(t, srv.appSubscriptionEpisodeSkip, "/api/app/v1/subscriptions/"+mid+"/episodes/1/skip", mid, "1", `{"skip":true}`).Code)
}

// 定时：按间隔、cron 能存能读；预览给接下来几次的时间（cron 不对时 400 并说清楚哪一段）
func TestAppScheduleIntervalCronAndPreview(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions",
		`{"media_type":"tv","tmdb_id":1001,"season":1,"schedule":{"kind":"cron","cron":"0 22 * * 5 ;0 9,13 * * 6"}}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, AppSchedule{Kind: "cron", Cron: "0 22 * * 5; 0 9,13 * * 6"}, sub.Schedule)
	id := strconv.Itoa(int(sub.ID))

	w = appWithID(t, srv.appSubscriptionUpdate, http.MethodPut, "/api/app/v1/subscriptions/"+id, id, `{"schedule":{"kind":"interval","interval_minutes":120}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var updated AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, AppSchedule{Kind: "interval", IntervalMinutes: 120}, updated.Schedule, "换成按间隔：cron 清掉")
	require.NotNil(t, updated.NextSearchAt)

	preview := func(body string) (int, AppSchedulePreview, string) {
		w := appAsWith(t, srv.appSchedulePreview, http.MethodPost, "/api/app/v1/subscriptions/schedule/preview", body, "app:read")
		var out AppSchedulePreview
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		}
		return w.Code, out, w.Body.String()
	}
	code, got, _ := preview(`{"schedule":{"kind":"interval","interval_minutes":90},"count":3}`)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, got.Next, 3)
	assert.Equal(t, int64(90*60), got.Next[1]-got.Next[0])

	code, got, _ = preview(`{"schedule":{"kind":"cron","cron":"*/10 * * * *"},"count":4}`)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, got.Next, 4)
	assert.GreaterOrEqual(t, got.Next[2]-got.Next[1], int64(30*60), "两次之间至少隔 30 分钟")

	code, got, _ = preview(`{"schedule":{"kind":"daily","at":"20:00"},"subscription_id":` + id + `}`)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, got.Next, 5)
	assert.Equal(t, int64(24*3600), got.Next[1]-got.Next[0])
	assert.Equal(t, 20, time.Unix(got.Next[0], 0).In(time.Local).Hour())

	code, got, _ = preview(`{"schedule":{"kind":"airtime"}}`)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, got.Next)
	assert.Contains(t, got.Note, "播出时间")

	code, _, text := preview(`{"schedule":{"kind":"cron","cron":"0 25 * * *"}}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, text, "第 2 段（小时）")
	code, _, _ = preview(`{"schedule":{"kind":"interval","interval_minutes":10}}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _, _ = preview(`{"schedule":{"kind":"daily"},"count":11}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _, _ = preview(`{"schedule":{"kind":"daily"},"subscription_id":999}`)
	assert.Equal(t, http.StatusNotFound, code)
}
