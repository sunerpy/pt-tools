package assistant

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

// 配置工具只给有配置权限的入口：没有时连讲怎么用的技能也不给；能看不能改时只有 get_config；提示词说清楚。
func TestConfigToolsFollowPermission(t *testing.T) {
	cases := []struct {
		name          string
		actor         Actor
		read, change  bool
		prompt, never string
	}{
		{"网页登录", admin, true, true, "get_skill", "不能改配置"},
		{"只读令牌", Actor{Owner: "token:7", Entry: models.AssistantEntryApp, Name: "r"}, false, false, "", "get_config"},
		{"能看配置的只读令牌", Actor{Owner: "token:8", Entry: models.AssistantEntryApp, Name: "c", CanConfig: true}, true, false, "不能改配置", "change_config"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.llm.push(reply{text: "好的"})
			conv := e.conversation(tc.actor)
			res := e.send(tc.actor, conv, "看看配置")
			fin, _ := e.waitRun(tc.actor, conv, res.RunID)
			require.Equal(t, RunDone, fin.Status, fin.Error)
			req := e.llm.requests()[0]
			names := req.toolNames()
			assert.Equal(t, tc.read, slices.Contains(names, "get_config"))
			assert.Equal(t, tc.read, slices.Contains(names, mcp.SkillTool))
			assert.Equal(t, tc.change, slices.Contains(names, "change_config"))
			system, _ := req.Messages[0]["content"].(string)
			if tc.prompt != "" {
				assert.Contains(t, system, tc.prompt)
			}
			assert.NotContains(t, system, tc.never)
		})
	}
}

// change_config 和别的写工具一样先预览、等确认：卡片用配置网关的差异与影响；落库与下发的参数里像密钥的值是 ***，
// 执行时交给工具的还是原值；运行在 MCP 里带着 app:config。
func TestChangeConfigConfirmation(t *testing.T) {
	e := newEnv(t)
	args := `{"resource":"downloaders","op":"update","id":"1","values":{"enabled":false,"password":"hunter2-secret-pw"}}`
	e.llm.push(call("call_c", "change_config", args), reply{text: "改好了。"})
	conv := e.conversation(admin)
	res := e.send(admin, conv, "停用下载器 1，密码改成 hunter2-secret-pw")
	events, _ := e.until(admin, conv, 0, pendingConfirmation)
	c := *events[len(events)-1].Confirmation
	assert.Equal(t, "change_config", c.Tool)
	assert.Equal(t, "改下载器「qb」的 2 项", c.Summary)
	assert.Equal(t, []mcp.Change{
		{Label: "启用（enabled）", Before: "是", After: "否"},
		{Label: "密码（password）", Before: "（不显示）", After: "已更新（不显示）", Secret: true},
	}, c.Changes)
	values, _ := c.Arguments["values"].(map[string]any)
	assert.Equal(t, "***", values["password"], "确认卡片的参数里不写密钥")
	assert.Equal(t, false, values["enabled"])

	var row models.AssistantConfirmation
	require.NoError(t, e.db.First(&row, c.ID).Error)
	assert.NotContains(t, row.Arguments, "hunter2")
	assert.NotContains(t, row.Preview, "hunter2")
	var stored []models.AssistantMessage
	require.NoError(t, e.db.Where("conversation_id = ?", conv).Find(&stored).Error)
	for _, m := range stored {
		assert.NotContains(t, m.ToolCalls, "hunter2", "落库的工具调用参数里也没有")
	}

	_, err := e.svc.Decide(context.Background(), admin, c.ID, true)
	require.NoError(t, err)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	require.Equal(t, RunDone, fin.Status, fin.Error)
	writes := e.backend.writes()
	require.Len(t, writes, 1)
	assert.Equal(t, "/config/downloaders/changes", writes[0].Path)
	body, _ := json.Marshal(writes[0].Body)
	assert.Contains(t, string(body), "hunter2-secret-pw", "执行时交给工具的是原值")
	for _, cl := range e.backend.callers {
		assert.Contains(t, cl.Scopes, apitoken.ScopeAppConfig)
	}
}

// maskConfigValues 只遮 values 里像密钥的键，别的参数不动；没有 values 时原样返回。
func TestMaskConfigValues(t *testing.T) {
	got := maskConfigValues(map[string]any{
		"resource": "notify_channels", "op": "update", "id": "3",
		"values": map[string]any{"config.bot_token": "123:abc", "config.default_chat_id": "-100", "name": "tg", "url": "http://x"},
	})
	assert.Equal(t, map[string]any{"config.bot_token": "***", "config.default_chat_id": "-100", "name": "tg", "url": "***"}, got["values"])
	assert.Equal(t, "notify_channels", got["resource"])
	plain := map[string]any{"resource": "global"}
	assert.Equal(t, plain, maskConfigValues(plain))
}
