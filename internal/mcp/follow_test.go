package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// followDetail 是假的订阅详情：第 4 季，按播出时间、锁住 HHWEB 1080p；第 5 集在等版本；两个种子（一个有失败的文件）。
const followDetail = `{"id":3,"media_type":"tv","season":4,"title":"现在就出发","year":2023,"status":"active","upgrade":false,
"profile_id":0,"sites":[],"downloader_id":0,"category":"","tags":"","save_path":"",
"schedule":{"kind":"airtime","at":"20:00"},"lock_version":true,"lock_group":"HHWEB","lock_resolution":"1080p",
"start_episode":0,"follow_next_season":false,"library_id":0,"total_episodes":0,
"progress":{"in_library":2,"missing":[4,5]},
"episodes":[{"number":4,"name":"第2期下","state":"missing"},{"number":5,"name":"第3期上","state":"waiting_version"}],
"torrents":[
 {"id":8,"site":"hdsky","title":"Show.S04.1080p.WEB-DL-HHWEB","status":"done","files":[
   {"name":"E01.mkv","status":"done","scrape_status":"partial"},{"name":"E02.mkv","status":"failed","message":"目标已有同名文件"}],
  "can_organize":true,"can_rescrape":true},
 {"id":9,"site":"hdsky","title":"Show.S04E03.720p","status":"replaced","files":[],"can_organize":false,"can_rescrape":false},
 {"id":10,"site":"hdsky","title":"Show.S04E06.1080p","status":"failed","files":[],"can_organize":true,"can_rescrape":false}]}`

func followReply(req Request) Response {
	ok := func(body string) Response { return Response{Status: http.StatusOK, Body: []byte(body)} }
	switch {
	case req.Method == http.MethodGet && req.Path == "/subscriptions/3":
		return ok(followDetail)
	case req.Method == http.MethodGet && req.Path == "/subscriptions/4":
		return ok(strings.Replace(strings.Replace(followDetail, `"id":3`, `"id":4`, 1), `"status":"active"`, `"status":"done"`, 1))
	case req.Method == http.MethodGet && strings.HasPrefix(req.Path, "/subscriptions/99"):
		return Response{Status: http.StatusNotFound, Body: []byte(`{"error":"not_found","message":"订阅不存在"}`)}
	case req.Path == "/subscriptions/schedule/preview":
		b, _ := json.Marshal(req.Body)
		if strings.Contains(string(b), "0 25") {
			return Response{Status: http.StatusBadRequest, Body: []byte(`{"error":"invalid_argument","message":"cron「0 25 * * *」第 2 段（小时）要在 0 到 23 之间"}`)}
		}
		return ok(`{"next":[1792944000,1793548800,1794153600]}`)
	case req.Method == http.MethodGet && req.Path == "/subscriptions/3/episode-rules":
		return ok(`[{"group":"PTerWEB","mode":"period_part","offset":0,"source":"manual"}]`)
	case req.Method == http.MethodGet && strings.HasPrefix(req.Path, "/media/history"):
		return ok(`{"items":[{"id":31,"media_type":"tv","title":"现在就出发","library":"综艺","status":"done"},{"id":32,"title":"现在就出发","status":"failed"}]}`)
	}
	return ok(`{"ok":true}`)
}

// 改订阅：预览只列改了的字段（改以前、改以后），改定时时写明接下来几次搜索；执行只发给了的字段，审计只记字段名
func TestUpdateSubscription(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	args := map[string]any{
		"id": 3, "schedule": map[string]any{"kind": "cron", "cron": "0 22 * * 5"}, "lock_version": true, "sites": []string{"hdsky"}, "start_episode": 4,
	}
	text, isErr, out := e.call("update_subscription", args)
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Equal(t, "修改订阅「现在就出发」（2023）第 4 季的设置", p.Summary)
	assert.Equal(t, []Change{
		{Label: "在哪些站点找", Before: "全部站点", After: "hdsky"},
		{Label: "定时", Before: "按每一集的播出时间", After: "按 cron：0 22 * * 5"},
		{Label: "起始集", Before: "整季", After: "从第 4 集开始"},
	}, p.Changes, "版本锁本来就开着，不列")
	impact := impactText(p)
	assert.Contains(t, impact, "重新算每一集")
	assert.Contains(t, impact, "接下来几次搜索："+timeText(1792944000))
	assert.Equal(t, true, out["pending"])
	for _, r := range e.backend.got() {
		assert.False(t, r.Write, "预览只读")
	}

	args["confirm"] = true
	_, isErr, _ = e.call("update_subscription", args)
	require.False(t, isErr)
	reqs := e.backend.got()
	last := reqs[len(reqs)-1]
	assert.Equal(t, http.MethodPut, last.Method)
	assert.Equal(t, "/subscriptions/3", last.Path)
	assert.True(t, last.Write)
	body, _ := json.Marshal(last.Body)
	assert.JSONEq(t, `{"schedule":{"kind":"cron","cron":"0 22 * * 5"},"lock_version":true,"sites":["hdsky"],"start_episode":4}`, string(body))
	a := e.audit.all()
	require.Len(t, a, 1)
	assert.Equal(t, "update_subscription", a[0].Command)
	assert.Equal(t, []string{"lock_version", "schedule", "sites", "start_episode"}, a[0].Args["fields"])

	// 空数组是全部站点；清掉锁住的版本
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 3, "lock_group": "", "lock_resolution": ""})
	require.False(t, isErr, text)
	assert.Equal(t, []Change{{Label: "锁住的版本", Before: "HHWEB 1080p", After: "还没定（按下一个下载的版本定）"}}, decodePreview(t, text).Changes)

	// 已经完成的订阅：不会搜索，预览照实说（不说「尽快搜索」，也不列接下来几次）；写错的定时照样拦
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 4, "schedule": map[string]any{"kind": "interval", "interval_minutes": 180}})
	require.False(t, isErr, text)
	impact = impactText(decodePreview(t, text))
	assert.Contains(t, impact, "已经完成，不会再搜索")
	assert.NotContains(t, impact, "尽快搜索")
	assert.NotContains(t, impact, "接下来几次搜索")
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 4, "schedule": map[string]any{"kind": "cron", "cron": "0 25 * * *"}})
	assert.True(t, isErr)
	assert.Contains(t, text, "第 2 段（小时）")
	assert.Equal(t, "订阅暂停着：恢复以后才按新的设置搜索", notSearchingNote("paused"))
	assert.Contains(t, notSearchingNote("pending"), "还没开始找")

	// 定时写错了：预览就说哪里不对
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 3, "schedule": map[string]any{"kind": "cron", "cron": "0 25 * * *"}})
	assert.True(t, isErr)
	assert.Contains(t, text, "第 2 段（小时）")
	// 和现在一样：照实说
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 3, "lock_version": true})
	require.False(t, isErr, text)
	assert.Contains(t, impactText(decodePreview(t, text)), "和现在一样")
	// 没给字段、没有这个订阅
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 3})
	assert.True(t, isErr)
	assert.Contains(t, text, "没有要改的字段")
	text, isErr, _ = e.call("update_subscription", map[string]any{"id": 99, "upgrade": true})
	assert.True(t, isErr)
	assert.Contains(t, text, "list_subscriptions")
	_, isErr, _ = e.call("update_subscription", map[string]any{"id": 3, "confirm": true})
	assert.True(t, isErr)
	assert.Equal(t, "error:invalid_argument", e.audit.all()[1].Result)
}

// 每一集：跳过与换版本的预览写明那一集现在怎样；不在这一季的直接说
func TestEpisodeTools(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	text, isErr, _ := e.call("set_episode_skip", map[string]any{"id": 3, "episode": 4, "skip": true})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Equal(t, []Change{{Label: "第 4 集「第2期下」", Before: "缺", After: "不追（跳过）"}}, p.Changes)

	text, isErr, _ = e.call("unlock_episode", map[string]any{"id": 3, "episode": 5, "unlock": true})
	require.False(t, isErr, text)
	p = decodePreview(t, text)
	assert.Equal(t, "只要锁住的版本（HHWEB 1080p）", p.Changes[0].Before)
	assert.Contains(t, impactText(p), "马上搜一次")

	text, isErr, _ = e.call("set_episode_skip", map[string]any{"id": 3, "episode": 9, "skip": true})
	assert.True(t, isErr)
	assert.Contains(t, text, "没有第 9 集")

	_, isErr, _ = e.call("unlock_episode", map[string]any{"id": 3, "episode": 5, "unlock": false, "confirm": true})
	require.False(t, isErr)
	last := e.backend.got()[len(e.backend.got())-1]
	assert.Equal(t, "/subscriptions/3/episodes/5/unlock", last.Path)
	body, _ := json.Marshal(last.Body)
	assert.JSONEq(t, `{"unlock":false}`, string(body))
}

// 集号规则：设之前写明这个组原来的规则；offset 方式要给偏移；删没有的规则直接说
func TestEpisodeRuleTools(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	text, isErr, _ := e.call("save_episode_rule", map[string]any{"id": 3, "group": "PTerWEB", "mode": "offset", "offset": 1})
	require.False(t, isErr, text)
	assert.Equal(t, []Change{{Label: "制作组 PTerWEB的集号", Before: "按第几期与上下", After: "集号偏移 +1"}}, decodePreview(t, text).Changes)
	text, isErr, _ = e.call("save_episode_rule", map[string]any{"id": 3, "mode": "offset"})
	assert.True(t, isErr)
	assert.Contains(t, text, "offset")
	text, isErr, _ = e.call("save_episode_rule", map[string]any{"id": 3, "mode": "guess"})
	assert.True(t, isErr, "mode 只收那四种")
	assert.Contains(t, text, "参数不对")

	text, isErr, _ = e.call("delete_episode_rule", map[string]any{"id": 3, "group": "XX"})
	assert.True(t, isErr)
	assert.Contains(t, text, "get_episode_rules")
	text, isErr, _ = e.call("delete_episode_rule", map[string]any{"id": 3, "group": "PTerWEB"})
	require.False(t, isErr, text)
	assert.Equal(t, "删除", decodePreview(t, text).Changes[0].After)

	_, isErr, _ = e.call("delete_episode_rule", map[string]any{"id": 3, "group": "PTerWEB", "confirm": true})
	require.False(t, isErr)
	last := e.backend.got()[len(e.backend.got())-1]
	assert.Equal(t, http.MethodDelete, last.Method)
	assert.Equal(t, "/subscriptions/3/episode-rules?group=PTerWEB", last.Path)
	_, isErr, _ = e.call("save_episode_rule", map[string]any{"id": 3, "group": " XX ", "mode": "date", "note": " 按日期 ", "confirm": true})
	require.False(t, isErr)
	last = e.backend.got()[len(e.backend.got())-1]
	body, _ := json.Marshal(last.Body)
	assert.JSONEq(t, `{"group":"XX","mode":"date","offset":0,"note":"按日期"}`, string(body))
}

// 定时预览：时间另外写成 pt-tools 所在时区的文字
func TestPreviewSchedule(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	text, isErr, out := e.call("preview_schedule", map[string]any{"schedule": map[string]any{"kind": "weekly", "days": []int{5}, "at": "22:00"}, "count": 3})
	require.False(t, isErr, text)
	assert.Equal(t, []any{timeText(1792944000), timeText(1793548800), timeText(1794153600)}, out["next_text"])
	got := e.backend.got()[0]
	assert.False(t, got.Write)
	body, _ := json.Marshal(got.Body)
	assert.JSONEq(t, `{"schedule":{"kind":"weekly","days":[5],"at":"22:00"},"media_type":"","subscription_id":0,"count":3}`, string(body))
	assert.Equal(t, time.Unix(1792944000, 0).In(time.Local).Format("2006-01-02"), strings.Fields(timeText(1792944000))[0])
}

// 订阅的种子：重新整理的预览列出每个文件现在怎样；洗版换掉的、还没有整理好的不能刮削；执行发到这个种子的接口
func TestSubscriptionTorrentTools(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	text, isErr, _ := e.call("reorganize_subscription_torrent", map[string]any{"id": 3, "subscription_torrent_id": 8})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Equal(t, []Change{
		{Label: "E01.mkv", Before: "已整理", After: "不动"},
		{Label: "E02.mkv", Before: "失败：目标已有同名文件", After: "按订阅的条目重新整理"},
	}, p.Changes)
	assert.Contains(t, impactText(p), "不靠识别")

	text, isErr, _ = e.call("reorganize_subscription_torrent", map[string]any{"id": 3, "subscription_torrent_id": 10})
	require.False(t, isErr, text)
	assert.Equal(t, "还没有整理记录", decodePreview(t, text).Changes[0].Before)

	for _, c := range []struct {
		tool string
		tid  int
		want string
	}{
		{"reorganize_subscription_torrent", 9, "洗版换掉"},
		{"rescrape_subscription_torrent", 10, "先用 reorganize_subscription_torrent"},
		{"rescrape_subscription_torrent", 77, "get_subscription"},
	} {
		text, isErr, _ = e.call(c.tool, map[string]any{"id": 3, "subscription_torrent_id": c.tid})
		assert.True(t, isErr, c.tool)
		assert.Contains(t, text, c.want, c.tool)
	}

	text, isErr, _ = e.call("rescrape_subscription_torrent", map[string]any{"id": 3, "subscription_torrent_id": 8})
	require.False(t, isErr, text)
	assert.Contains(t, impactText(decodePreview(t, text)), "这部剧在这个媒体库里整理好的每一集")

	_, isErr, _ = e.call("rescrape_subscription_torrent", map[string]any{"id": 3, "subscription_torrent_id": 8, "confirm": true})
	require.False(t, isErr)
	last := e.backend.got()[len(e.backend.got())-1]
	assert.Equal(t, "/subscriptions/3/torrents/8/scrape", last.Path)
	assert.True(t, last.Write)
	a := e.audit.all()
	require.Len(t, a, 1)
	assert.Equal(t, uint(3), a[0].Args["id"], "审计记订阅与种子")
	assert.Equal(t, uint(8), a[0].Args["torrent"])
}

// 重新刮削整理记录：只有整理好的能刮削
func TestRescrapeMedia(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = followReply
	text, isErr, _ := e.call("rescrape_media", map[string]any{"id": 31})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Equal(t, "重新刮削「现在就出发」", p.Summary)
	assert.Contains(t, p.Changes[0].Before, "媒体库「综艺」")
	text, isErr, _ = e.call("rescrape_media", map[string]any{"id": 32})
	assert.True(t, isErr)
	assert.Contains(t, text, "retry_media_organize")
	_, isErr, _ = e.call("rescrape_media", map[string]any{"id": 31, "confirm": true})
	require.False(t, isErr)
	assert.Equal(t, "/media/history/31/scrape", e.backend.got()[len(e.backend.got())-1].Path)
}

// 新建订阅带上追剧的设置：请求体照给的发，预览写明定时、版本锁与从哪个种子开始
func TestAddSubscriptionFollowFields(t *testing.T) {
	e := newEnv(t)
	args := map[string]any{
		"media_type": "tv", "tmdb_id": 231620, "season": 4, "schedule": map[string]any{"kind": "daily", "at": "21:30"},
		"lock_version": false, "start_episode": 3, "from_torrent": map[string]any{"site": "hdsky", "torrent_id": "91"},
		"sites": []string{"hdsky"}, "downloader_id": 2, "save_path": "/tv",
	}
	text, isErr, _ := e.call("add_subscription", args)
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	labels := map[string]string{}
	for _, c := range p.Changes {
		labels[c.Label] = c.After
	}
	assert.Equal(t, "每天 21:30", labels["定时"])
	assert.Equal(t, "关", labels["版本锁"])
	assert.Equal(t, "从第 3 集开始", labels["起始集"])
	assert.Equal(t, "站点 hdsky 的种子 91", labels["从这个种子开始追"])
	assert.Equal(t, "下载器 2", labels["推送到"])
	assert.NotContains(t, impactText(p), "开着版本锁")

	args["confirm"] = true
	_, isErr, _ = e.call("add_subscription", args)
	require.False(t, isErr)
	body, _ := json.Marshal(e.backend.got()[len(e.backend.got())-1].Body)
	assert.JSONEq(t, `{"media_type":"tv","tmdb_id":231620,"season":4,"upgrade":false,"profile_id":0,"sites":["hdsky"],"downloader_id":2,
		"save_path":"/tv","schedule":{"kind":"daily","at":"21:30"},"lock_version":false,"start_episode":3,"from_torrent":{"site":"hdsky","torrent_id":"91"}}`, string(body))

	// 只给条目：请求体和以前一样（默认按播出时间、开着版本锁，由 pt-tools 定）
	_, isErr, _ = e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 278, "confirm": true})
	require.False(t, isErr)
	body, _ = json.Marshal(e.backend.got()[len(e.backend.got())-1].Body)
	assert.JSONEq(t, `{"media_type":"movie","tmdb_id":278,"season":0,"upgrade":false,"profile_id":0}`, string(body))
}

// 追剧技能：列在技能清单里，讲了定时、cron、集号规则与重试
func TestFollowSkill(t *testing.T) {
	e := newEnv(t)
	_, isErr, out := e.call("get_skill", nil)
	require.False(t, isErr)
	names := []string{}
	for _, s := range out["skills"].([]any) {
		names = append(names, s.(map[string]any)["name"].(string))
	}
	assert.Equal(t, []string{SkillConfig, SkillFollow}, names)
	text, isErr, out := e.call("get_skill", map[string]any{"name": SkillFollow})
	require.False(t, isErr)
	assert.Equal(t, SkillFollow, out["name"])
	for _, want := range []string{"add_subscription", "update_subscription", "preview_schedule", "0 22 * * 5", "save_episode_rule", "reorganize_subscription_torrent", "search_subscription"} {
		assert.Contains(t, text, want)
	}
	// 技能里提到的工具都存在
	for _, name := range []string{"set_episode_skip", "unlock_episode", "list_subscription_runs", "get_episode_rules", "delete_episode_rule", "rescrape_subscription_torrent", "rescrape_media", "retry_media_organize"} {
		assert.Contains(t, ToolNames, name)
		assert.Contains(t, followSkillGuide, name)
	}
}
