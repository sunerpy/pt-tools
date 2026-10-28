package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
)

// writeTools 是会改东西的工具：其余都是只读的。加工具时要在这里写明是哪一种（TestToolsReadWrite 会提醒）。
var writeTools = map[string]bool{
	"pause_torrent": true, "resume_torrent": true, "delete_torrent": true, "push_torrent": true,
	"add_subscription": true, "set_subscription_status": true, "search_subscription": true, "delete_subscription": true,
	"organize_media": true, "retry_media_organize": true, "change_brush_task": true,
	"change_config":       true,
	"update_subscription": true, "set_episode_skip": true, "unlock_episode": true, "save_episode_rule": true,
	"delete_episode_rule": true, "reorganize_subscription_torrent": true, "rescrape_subscription_torrent": true, "rescrape_media": true,
}

// 会改动下载器、订阅、媒体库或配置的工具都是写工具（ReadOnlyHint 为假）：执行要 mcp:write 与 confirm=true；
// 只读的标 ReadOnlyHint。新加的工具要在 writeTools 里写明是哪一种。
func TestToolsReadWrite(t *testing.T) {
	e := newEnv(t)
	res, err := e.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range res.Tools {
		require.NotNil(t, tool.Annotations, tool.Name)
		assert.Equal(t, writeTools[tool.Name], !tool.Annotations.ReadOnlyHint, "%s 是不是写工具", tool.Name)
	}
}

func decodePreview(t *testing.T, text string) Preview {
	t.Helper()
	var p Preview
	require.NoError(t, json.Unmarshal([]byte(text), &p), text)
	require.True(t, p.Pending, text)
	return p
}

func impactText(p Preview) string { return strings.Join(p.Impact, "\n") }

// 种子的预览：按下载器里的现状写改以前、改以后，删文件的写明释放多少、不能恢复；找不到的直接说，不让确认。
func TestTorrentPreviews(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		if strings.HasPrefix(req.Path, "/torrents?") {
			return Response{Status: http.StatusOK, Body: []byte(`{"items":[` +
				`{"downloader_id":1,"downloader":"qb","task_id":"aaa","title":"Dune","progress":1,"size":4294967296,"state":"seeding","ratio":2.5,"save_path":"/downloads"},` +
				`{"downloader_id":1,"downloader":"qb","task_id":"bbb","title":"Arrival","progress":0.25,"size":4294967296,"state":"paused"}]}`)}
		}
		return Response{Status: http.StatusOK, Body: []byte(`{}`)}
	}
	text, isErr, _ := e.call("pause_torrent", map[string]any{"downloader_id": 1, "task_id": "aaa"})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Equal(t, "pause_torrent", p.Tool)
	assert.Contains(t, impactText(p), "停止做种")
	assert.Contains(t, impactText(p), "2.50")

	text, _, _ = e.call("resume_torrent", map[string]any{"downloader_id": 1, "task_id": "bbb"})
	p = decodePreview(t, text)
	assert.Equal(t, []Change{{Label: "状态", Before: "已暂停", After: "下载中"}}, p.Changes)
	assert.Contains(t, impactText(p), "还要写入约 3.0 GB")

	text, _, _ = e.call("delete_torrent", map[string]any{"downloader_id": 1, "task_id": "aaa", "remove_data": true})
	p = decodePreview(t, text)
	require.Len(t, p.Changes, 2)
	assert.Equal(t, "一起删除", p.Changes[1].After)
	assert.Contains(t, impactText(p), "不能恢复，释放约 4.0 GB")
	assert.Contains(t, impactText(p), "H&R")
	text, _, _ = e.call("delete_torrent", map[string]any{"downloader_id": 1, "task_id": "aaa"})
	p = decodePreview(t, text)
	assert.Equal(t, "保留在 /downloads", p.Changes[1].After)

	text, isErr, _ = e.call("pause_torrent", map[string]any{"downloader_id": 1, "task_id": "zzz"})
	assert.True(t, isErr)
	assert.Contains(t, text, "没有任务 zzz")

	// 下载器连不上：照样给预览，写明读不到
	e.backend.reply = func(Request) Response {
		return Response{Status: http.StatusInternalServerError, Body: []byte(`{"error":"internal","message":"连接被拒绝"}`)}
	}
	text, isErr, _ = e.call("pause_torrent", map[string]any{"downloader_id": 1, "task_id": "aaa"})
	require.False(t, isErr, text)
	assert.Contains(t, impactText(decodePreview(t, text)), "读不到这个种子的现状")
}

// 推送的预览：推到哪台下载器、目录，以及那台下载器的剩余空间。
func TestPushPreview(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		return Response{Status: http.StatusOK, Body: []byte(`{"items":[{"id":1,"name":"qb","default":true,"reachable":true,"free_space":107374182400},{"id":2,"name":"tr","reachable":false,"error":"超时"}]}`)}
	}
	text, isErr, _ := e.call("push_torrent", map[string]any{"site": "hdsky", "torrent_id": "123", "save_path": "/movies"})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Contains(t, p.Summary, "推送到「qb」")
	assert.Contains(t, p.Changes, Change{Label: "保存目录", After: "/movies"})
	assert.Contains(t, impactText(p), "剩余 100.0 GB")
	text, _, _ = e.call("push_torrent", map[string]any{"site": "hdsky", "torrent_id": "123", "downloader_id": 2})
	assert.Contains(t, impactText(decodePreview(t, text)), "连不上")
	text, isErr, _ = e.call("push_torrent", map[string]any{"site": "hdsky"})
	assert.True(t, isErr)
	assert.Contains(t, text, "torrent_id")
}

// 订阅的预览：状态改以前、改以后；删除写明已下载的不受影响；没有这个订阅的直接说。
func TestSubscriptionPreviews(t *testing.T) {
	e := newEnv(t)
	e.backend.reply = func(req Request) Response {
		if req.Path == "/subscriptions/5" {
			return Response{Status: http.StatusOK, Body: []byte(`{"id":5,"media_type":"tv","season":2,"title":"沙丘：预言","year":2024,"status":"active","upgrade":true,"progress":{"in_library":3,"missing":[4,5]}}`)}
		}
		return Response{Status: http.StatusNotFound, Body: []byte(`{"error":"not_found","message":"没有这个订阅"}`)}
	}
	text, _, _ := e.call("set_subscription_status", map[string]any{"id": 5, "status": "paused"})
	p := decodePreview(t, text)
	assert.Equal(t, "暂停订阅「沙丘：预言」（2024）第 2 季", p.Summary)
	assert.Equal(t, []Change{{Label: "状态", Before: "订阅中", After: "已暂停"}}, p.Changes)
	text, _, _ = e.call("search_subscription", map[string]any{"id": 5})
	assert.Contains(t, impactText(decodePreview(t, text)), "还缺的 2 集")
	text, _, _ = e.call("delete_subscription", map[string]any{"id": 5})
	p = decodePreview(t, text)
	assert.Contains(t, impactText(p), "媒体库里的文件留着")
	assert.Contains(t, impactText(p), "洗版")
	text, isErr, _ := e.call("delete_subscription", map[string]any{"id": 9})
	assert.True(t, isErr)
	assert.Contains(t, text, "没有编号 9 的订阅")

	text, isErr, _ = e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 438631, "upgrade": true})
	require.False(t, isErr, text)
	p = decodePreview(t, text)
	assert.Contains(t, p.Summary, "TMDB 438631")
	assert.Contains(t, impactText(p), "替换媒体库里的文件")
}

// 整理的预览：照整理计划列出每个文件放到哪儿与整理方式的影响；整理不了的直接说。
func TestOrganizePreviews(t *testing.T) {
	e := newEnv(t)
	plan := `{"downloader":"qb","name":"Dune.2021.2160p","library":{"name":"电影"},"mode":"copy","items":[` +
		`{"rel":"Dune.mkv","size":21474836480,"target":"/media/电影/沙丘 (2021)/沙丘 (2021).mkv","status":"pending"},` +
		`{"rel":"Dune.sample.mkv","size":1024,"target":"/media/x","status":"exists"}],"skipped":1}`
	e.backend.reply = func(req Request) Response {
		switch {
		case req.Path == "/media/organize/preview":
			return Response{Status: http.StatusOK, Body: []byte(plan)}
		case strings.HasPrefix(req.Path, "/media/history"):
			return Response{Status: http.StatusOK, Body: []byte(`{"items":[{"id":9,"title":"沙丘","torrent_name":"Dune.2021","mode":"hardlink","status":"failed","message":"找不到文件","target_path":"/media/电影/沙丘 (2021)"}]}`)}
		}
		return Response{Status: http.StatusOK, Body: []byte(`{}`)}
	}
	text, isErr, _ := e.call("organize_media", map[string]any{"downloader_id": 1, "hash": "abc"})
	require.False(t, isErr, text)
	p := decodePreview(t, text)
	assert.Contains(t, p.Summary, "整理进媒体库「电影」")
	require.Len(t, p.Changes, 2)
	assert.Equal(t, Change{Label: "整理方式", After: "复制"}, p.Changes[0])
	assert.Equal(t, "/media/电影/沙丘 (2021)/沙丘 (2021).mkv", p.Changes[1].After)
	assert.Contains(t, impactText(p), "多占约 20.0 GB")
	assert.Contains(t, impactText(p), "1 个文件的目标位置已经有同名文件")
	assert.True(t, e.backend.got()[len(e.backend.got())-1].Preview)

	text, _, _ = e.call("retry_media_organize", map[string]any{"id": 9})
	p = decodePreview(t, text)
	assert.Contains(t, p.Summary, "重新整理「沙丘」")
	assert.Contains(t, p.Changes[0].Before, "找不到文件")
	assert.Contains(t, impactText(p), "硬链接")

	plan = `{"name":"Dune","items":[],"problem":"认不出是什么片"}`
	text, isErr, _ = e.call("organize_media", map[string]any{"downloader_id": 1, "hash": "abc"})
	assert.True(t, isErr)
	assert.Contains(t, text, "认不出是什么片")
}

// 只有 mcp:write 的令牌也能看写工具的预览（预览的读带着 Preview，App API 那边按读放行）。
func TestPreviewNeedsOnlyWriteScope(t *testing.T) {
	e := newEnv(t)
	e.caller.Scopes = []string{apitoken.ScopeMCPWrite}
	text, isErr, _ := e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 1})
	require.False(t, isErr, text)
	assert.True(t, decodePreview(t, text).Pending)
	e.caller.Scopes = []string{apitoken.ScopeMCPRead}
	text, isErr, _ = e.call("add_subscription", map[string]any{"media_type": "movie", "tmdb_id": 1})
	assert.True(t, isErr)
	assert.Contains(t, text, "mcp:write")
}
