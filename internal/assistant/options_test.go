package assistant

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 推理内容的几种来处、推理等级、选模型、附件与对话页的模型列表。

func TestThinkSplitterAcrossChunks(t *testing.T) {
	cases := []struct {
		name          string
		chunks        []string
		text, thought string
	}{
		{"标签切开", []string{" \n<th", "ink>想一", "想</thi", "nk>\n\n答", "案"}, "答案", "想一想"},
		{"没有标签", []string{"直接", "回答"}, "直接回答", ""},
		{"像标签开头但不是", []string{"<thx", "y>"}, "<thxy>", ""},
		{"标签不在开头不认", []string{"先说 <think>不是思考</think>"}, "先说 <think>不是思考</think>", ""},
		{"没收尾的思考", []string{"<think>想到一半"}, "", "想到一半"},
		{"只有空白", []string{"  "}, "  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var sp thinkSplitter
			var text, thought string
			for _, ch := range c.chunks {
				a, b := sp.add(ch)
				text, thought = text+a, thought+b
			}
			a, b := sp.flush()
			assert.Equal(t, c.text, text+a)
			assert.Equal(t, c.thought, thought+b)
		})
	}
}

func TestReasoningFieldShownNotReplayed(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{reasoning: "这是 reasoning 字段", reasoningKey: "reasoning", text: "答一。"}, reply{text: "答二。"})
	conv := e.conversation(admin)
	e.waitRun(admin, conv, e.send(admin, conv, "问一").RunID)
	e.waitRun(admin, conv, e.send(admin, conv, "问二").RunID)
	msgs := e.messages(conv)
	require.Equal(t, models.AssistantRoleAssistant, msgs[1].Role)
	assert.Equal(t, "这是 reasoning 字段", msgs[1].Reasoning)
	assert.Equal(t, fieldReasoning, msgs[1].ReasoningField)
	second := e.llm.requests()[1]
	assert.NotContains(t, second.Raw, "reasoning_content", "不是 reasoning_content 来的不回传：不认这个字段的服务会拒绝请求")
	assert.NotContains(t, second.Raw, "这是 reasoning 字段")
	d, err := e.svc.Conversation(context.Background(), admin, conv)
	require.NoError(t, err)
	assert.Equal(t, "这是 reasoning 字段", d.Messages[1].Reasoning)
}

func TestThinkTagsSplitOutOfAnswer(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "<think>先查一下站点</think>\n\n你的站点都正常。"}, reply{text: "好的。"})
	conv := e.conversation(admin)
	_, all := e.waitRun(admin, conv, e.send(admin, conv, "站点怎么样").RunID)
	partial, reasoning := "", ""
	for _, ev := range all {
		switch {
		case ev.Type == EventDelta && ev.Replace:
			partial = ev.Text
		case ev.Type == EventDelta:
			partial += ev.Text
		case ev.Type == EventReasoning:
			reasoning += ev.Text
		}
	}
	assert.Equal(t, "你的站点都正常。", partial, "增量里没有 <think> 那一段")
	assert.Equal(t, "先查一下站点", reasoning)
	msgs := e.messages(conv)
	assert.Equal(t, "你的站点都正常。", msgs[1].Content)
	assert.Equal(t, "先查一下站点", msgs[1].Reasoning)
	assert.Equal(t, fieldThink, msgs[1].ReasoningField)
	e.waitRun(admin, conv, e.send(admin, conv, "谢谢").RunID)
	second := e.llm.requests()[1]
	assert.NotContains(t, second.Raw, "先查一下站点", "正文里的思考不回传")
}

func TestModelAndEffortOptions(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "好。"})
	conv := e.conversation(admin)
	res, err := e.svc.SendWith(context.Background(), admin, conv, SendOptions{Text: "你好", Model: "other-model", Effort: "high"})
	require.NoError(t, err)
	assert.Equal(t, "other-model", res.Message.Model)
	assert.Equal(t, "high", res.Message.Effort)
	e.waitRun(admin, conv, res.RunID)
	req := e.llm.requests()[0]
	assert.Equal(t, "other-model", req.Model)
	assert.Equal(t, "high", req.ReasoningEffort)
	msgs := e.messages(conv)
	assert.Equal(t, "other-model", msgs[1].Model, "回答记下实际用的模型")
	assert.Equal(t, "high", msgs[1].Effort)

	// 不选的时候用设置里的模型、不带推理等级
	e.llm.push(reply{text: "好。"})
	e.waitRun(admin, conv, e.send(admin, conv, "再来").RunID)
	req = e.llm.requests()[1]
	assert.Equal(t, "fake-model", req.Model)
	assert.NotContains(t, req.Raw, "reasoning_effort")

	for _, o := range []SendOptions{
		{Text: "x", Model: "有 空格"},
		{Text: "x", Model: strings.Repeat("m", maxModelLen+1)},
		{Text: "x", Effort: "ultra"},
	} {
		_, err := e.svc.SendWith(context.Background(), admin, conv, o)
		assert.ErrorIs(t, err, ErrInvalid, "%+v", o)
	}
}

func TestEffortRejectedRetriesWithout(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusBadRequest, body: "Unsupported value: 'reasoning_effort' does not support 'none' with this model."},
		reply{text: "按默认回答了。"})
	conv := e.conversation(admin)
	res, err := e.svc.SendWith(context.Background(), admin, conv, SendOptions{Text: "你好", Effort: "none"})
	require.NoError(t, err)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunDone, fin.Status)
	reqs := e.llm.requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "none", reqs[0].ReasoningEffort)
	assert.Empty(t, reqs[1].ReasoningEffort, "去掉推理等级重试")
	msgs := e.messages(conv)
	assert.Equal(t, []string{"user", "notice", "assistant"}, roles(msgs))
	assert.Contains(t, msgs[1].Content, "不认推理等级「不思考」")
	assert.Empty(t, msgs[2].Effort, "回答记下实际没带推理等级")

	// 别的 400 不重试
	e.llm.push(reply{status: http.StatusBadRequest, body: "messages too long"})
	res, err = e.svc.SendWith(context.Background(), admin, conv, SendOptions{Text: "再来", Effort: "low"})
	require.NoError(t, err)
	fin, _ = e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunFailed, fin.Status)
	assert.Len(t, e.llm.requests(), 3)
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4))))
	return buf.Bytes()
}

func TestAttachmentsReachModel(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	img, err := e.svc.Upload(ctx, admin, `C:\pics\shot.png`, pngBytes(t))
	require.NoError(t, err)
	assert.Equal(t, "shot.png", img.Name, "只留文件名")
	assert.Equal(t, models.AssistantAttachImage, img.Kind)
	assert.Equal(t, "image/png", img.MIME)
	log, err := e.svc.Upload(ctx, admin, "qb.log", []byte("\xef\xbb\xbf错误：tracker https://hdsky.me/announce.php?passkey="+passkey+" 回 403\n```\n"))
	require.NoError(t, err)
	assert.Equal(t, models.AssistantAttachText, log.Kind)

	e.llm.push(reply{text: "图里是个灰色方块。"}, reply{text: "日志里是 403。"})
	conv := e.conversation(admin)
	res, err := e.svc.SendWith(ctx, admin, conv, SendOptions{Text: "看看这两个", Attachments: []uint{img.ID, log.ID, img.ID}})
	require.NoError(t, err)
	require.Len(t, res.Message.Attachments, 2, "重复的编号只算一个")
	e.waitRun(admin, conv, res.RunID)
	first := e.llm.requests()[0].Raw
	assert.Contains(t, first, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngBytes(t))[:16], "这一轮的图片用 data URI 给模型")
	assert.Contains(t, first, "附件「qb.log」")
	assert.Contains(t, first, "回 403")
	assert.NotContains(t, first, passkey, "文本附件脱敏以后才给模型")
	assert.Contains(t, first, "````", "内容里有 ``` 时围栏加长")

	e.waitRun(admin, conv, e.send(admin, conv, "日志说了什么").RunID)
	second := e.llm.requests()[1].Raw
	assert.NotContains(t, second, "data:image/png", "以前的图片不再给")
	assert.Contains(t, second, "附了图片「shot.png」")
	assert.Contains(t, second, "回 403", "以前的文本附件在字数以内照给")

	d, err := e.svc.Conversation(ctx, admin, conv)
	require.NoError(t, err)
	require.Len(t, d.Messages[0].Attachments, 2)
	assert.Equal(t, "shot.png", d.Messages[0].Attachments[0].Name)

	// 发过的不能再发、不能单独删；别人读不到
	_, err = e.svc.SendWith(ctx, admin, conv, SendOptions{Text: "再发一次", Attachments: []uint{img.ID}})
	assert.ErrorIs(t, err, ErrInvalid)
	assert.ErrorIs(t, e.svc.DeleteAttachment(ctx, admin, img.ID), ErrInvalid)
	other := Actor{Owner: "web:other", Entry: models.AssistantEntryWeb, Name: "other", CanWrite: true}
	_, err = e.svc.Attachment(ctx, other, img.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	got, err := e.svc.Attachment(ctx, admin, log.ID)
	require.NoError(t, err)
	assert.NotContains(t, string(got.Data), passkey, "存的是脱敏以后的内容")
	assert.False(t, bytes.HasPrefix(got.Data, []byte("\xef\xbb\xbf")), "去掉 BOM")

	// 删会话连附件一起删
	require.NoError(t, e.svc.DeleteConversation(ctx, admin, conv))
	var n int64
	require.NoError(t, e.db.Model(&models.AssistantAttachment{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestAttachmentOnlyMessage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	img, err := e.svc.Upload(ctx, admin, "截图.png", pngBytes(t))
	require.NoError(t, err)
	e.llm.push(reply{text: "收到图片。"})
	conv := e.conversation(admin)
	res, err := e.svc.SendWith(ctx, admin, conv, SendOptions{Attachments: []uint{img.ID}})
	require.NoError(t, err, "只发附件不写字也行")
	e.waitRun(admin, conv, res.RunID)
	list, err := e.svc.Conversations(ctx, admin)
	require.NoError(t, err)
	assert.Equal(t, "截图.png", list[0].Title, "没写字时会话标题用附件名")
}

func TestUploadRejects(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cases := map[string]struct {
		name string
		data []byte
	}{
		"空文件":      {"a.txt", nil},
		"不认识的类型":   {"a.exe", []byte("MZ\x90\x00")},
		"不是 UTF-8": {"a.txt", []byte{0xff, 0xfe, 'a', 0}},
		"文本太大":     {"a.log", bytes.Repeat([]byte("a"), maxTextBytes+1)},
		"SVG 不收":   {"a.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
	}
	for name, c := range cases {
		_, err := e.svc.Upload(ctx, admin, c.name, c.data)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	big := append(pngBytes(t), bytes.Repeat([]byte{0}, maxImageBytes)...)
	_, err := e.svc.Upload(ctx, admin, "big.png", big)
	assert.ErrorIs(t, err, ErrInvalid, "图片太大")

	var ids []uint
	for range MaxAttachments + 1 {
		v, uerr := e.svc.Upload(ctx, admin, "a.txt", []byte("x"))
		require.NoError(t, uerr)
		ids = append(ids, v.ID)
	}
	conv := e.conversation(admin)
	_, err = e.svc.SendWith(ctx, admin, conv, SendOptions{Text: "x", Attachments: ids})
	assert.ErrorIs(t, err, ErrInvalid, "一条消息最多 4 个附件")
	require.NoError(t, e.svc.DeleteAttachment(ctx, admin, ids[0]), "没发的可以删")
	assert.ErrorIs(t, e.svc.DeleteAttachment(ctx, admin, ids[0]), ErrNotFound)

	for range maxPending - MaxAttachments {
		_, uerr := e.svc.Upload(ctx, admin, "a.txt", []byte("x"))
		require.NoError(t, uerr)
	}
	_, err = e.svc.Upload(ctx, admin, "a.txt", []byte("x"))
	assert.ErrorIs(t, err, ErrInvalid, "没发的附件太多")
}

func TestPendingAttachmentsCleanedAfterADay(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	v, err := e.svc.Upload(ctx, admin, "a.txt", []byte("x"))
	require.NoError(t, err)
	require.NoError(t, e.db.Model(&models.AssistantAttachment{}).Where("id = ?", v.ID).
		Update("created_at", time.Now().Add(-25*time.Hour)).Error)
	fresh, err := e.svc.Upload(ctx, admin, "b.txt", []byte("y"))
	require.NoError(t, err)
	_, err = e.svc.Cleanup(ctx)
	require.NoError(t, err)
	_, err = e.svc.Attachment(ctx, admin, v.ID)
	assert.ErrorIs(t, err, ErrNotFound, "传了一天没发的清掉")
	_, err = e.svc.Attachment(ctx, admin, fresh.ID)
	assert.NoError(t, err)
}

func TestImageRejectedHint(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	img, err := e.svc.Upload(ctx, admin, "a.png", pngBytes(t))
	require.NoError(t, err)
	e.llm.push(reply{status: http.StatusBadRequest, body: "unknown variant `image_url`, expected `text`"})
	conv := e.conversation(admin)
	res, err := e.svc.SendWith(ctx, admin, conv, SendOptions{Text: "看图", Attachments: []uint{img.ID}})
	require.NoError(t, err)
	fin, _ := e.waitRun(admin, conv, res.RunID)
	assert.Equal(t, RunFailed, fin.Status)
	assert.Contains(t, fin.Error, "可能不能看图")
}

func TestChatModels(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.llm.mu.Lock()
	e.llm.models = []string{"b-model", "fake-model", "text-embedding-3", "a-model"}
	e.llm.mu.Unlock()
	got, err := e.svc.ChatModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, "fake-model", got.Default)
	assert.Equal(t, []string{"fake-model", "a-model", "b-model"}, got.Models, "设置里的模型在第一个，去掉不能对话的")
	assert.Empty(t, got.Error)

	e.llm.mu.Lock()
	e.llm.models = []string{"c-model"}
	e.llm.mu.Unlock()
	got, err = e.svc.ChatModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"fake-model", "a-model", "b-model"}, got.Models, "十分钟内用缓存")
	e.llm.mu.Lock()
	n := len(e.llm.modelAuths)
	e.llm.mu.Unlock()
	assert.Equal(t, 1, n)

	// 换了模型设置以后重读；读不到时只有设置里的模型，并写明原因（不带 Key）
	e.svc.modelsMu.Lock()
	e.svc.models = modelCache{}
	e.svc.modelsMu.Unlock()
	e.llm.mu.Lock()
	e.llm.modelStatus = http.StatusUnauthorized
	e.llm.mu.Unlock()
	got, err = e.svc.ChatModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"fake-model"}, got.Models)
	assert.Contains(t, got.Error, "401")
	assert.NotContains(t, got.Error, testKey)

	_, err = e.svc.SaveSettings(ctx, SettingsInput{Enabled: false, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.NoError(t, err)
	_, err = e.svc.ChatModels(ctx)
	assert.True(t, errors.Is(err, ErrDisabled))
}

func TestUserParamShapes(t *testing.T) {
	text := []models.AssistantAttachment{{Kind: models.AssistantAttachText, Name: "a.md", Size: 3, Data: []byte("abc")}}
	p, images := userParam("问", text, true, false)
	assert.False(t, images)
	buf, _ := json.Marshal(p)
	assert.Contains(t, string(buf), `"content":"问`, "没有图片时内容是一段文字")
	p, _ = userParam("问", text, false, true)
	buf, _ = json.Marshal(p)
	assert.Contains(t, string(buf), "太长了")
	assert.NotContains(t, string(buf), "abc")
	img := []models.AssistantAttachment{{Kind: models.AssistantAttachImage, Name: "a.png", MIME: "image/png", Data: []byte("x")}}
	p, images = userParam("", img, true, false)
	assert.True(t, images)
	buf, _ = json.Marshal(p)
	assert.Contains(t, string(buf), `"image_url"`)
	assert.NotContains(t, string(buf), `"text"`, "没有文字时只有图片")
}
