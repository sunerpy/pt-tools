package assistant

import (
	"bytes"
	"context"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 签到认验证码：图片以 data URI 发给看图模型（没单独选时用助手的模型），回答里只取验证码本身。
func TestReadCaptcha(t *testing.T) {
	e := newEnv(t)
	img := renderCaptcha("AB12CD")

	e.llm.push(reply{text: "验证码是：AB12CD"})
	got, err := e.svc.ReadCaptcha(context.Background(), "vision-model", CaptchaRequest{Image: img, MIME: "image/png", Length: 6})
	require.NoError(t, err)
	assert.Equal(t, "AB12CD", got)
	reqs := e.llm.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "vision-model", reqs[0].Model)
	assert.False(t, reqs[0].Stream)
	assert.Contains(t, reqs[0].Raw, `"image_url"`)
	assert.Contains(t, reqs[0].Raw, "data:image/png;base64,")
	assert.Contains(t, reqs[0].Raw, "6 个字符")
	assert.Equal(t, "Bearer "+testKey, reqs[0].Auth, "用 AI 助手的 Key")

	for _, tc := range []struct{ reply, want string }{
		{"The captcha reads AB12CD.", "AB12CD"},
		{"<think>先看第一个字……</think>\nAB12CD", "AB12CD"},
		{"A B 1 2 C D", "AB12CD"},
		{"认不出来", ""},
	} {
		e.llm.push(reply{text: tc.reply})
		read, readErr := e.svc.ReadCaptcha(context.Background(), "", CaptchaRequest{Image: img, MIME: "image/png", Length: 6})
		require.NoError(t, readErr, tc.reply)
		assert.Equal(t, tc.want, read, tc.reply)
	}
	assert.Equal(t, "fake-model", e.llm.requests()[1].Model, "没单独选看图模型时用助手的模型")

	// 只有大写字母和数字的验证码：提示里写明，认成小写的换成大写
	e.llm.push(reply{text: "ab12cd"})
	got, err = e.svc.ReadCaptcha(context.Background(), "", CaptchaRequest{Image: img, MIME: "image/png", Length: 6, UpperOnly: true})
	require.NoError(t, err)
	assert.Equal(t, "AB12CD", got)
	reqs = e.llm.requests()
	assert.Contains(t, reqs[len(reqs)-1].Raw, "大写")
}

func TestReadCaptcha_ModelCannotSeeImages(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{status: http.StatusBadRequest, body: "unknown variant `image_url`, expected `text`"})
	_, err := e.svc.ReadCaptcha(context.Background(), "deepseek-chat", CaptchaRequest{Image: renderCaptcha("AB12CD"), MIME: "image/png", Length: 6})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
	assert.Contains(t, err.Error(), "看图")
	assert.NotContains(t, err.Error(), testKey)
}

func TestSignIn_AssistantDisabled(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.SaveSettings(context.Background(), SettingsInput{Enabled: false, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model"})
	require.NoError(t, err)
	_, err = e.svc.ReadCaptcha(context.Background(), "", CaptchaRequest{Image: renderCaptcha("AB12CD"), MIME: "image/png", Length: 6})
	assert.ErrorIs(t, err, ErrDisabled)
	_, err = e.svc.ChooseAnswer(context.Background(), "题目", []string{"a", "b"}, false)
	assert.ErrorIs(t, err, ErrDisabled)
	assert.ErrorIs(t, e.svc.Ready(context.Background()), ErrDisabled)
	assert.Empty(t, e.llm.requests(), "没启用时不调模型")
}

// 签到答题：题目和带编号的选项发给助手的模型，回答里的编号换成下标（从 0 开始）。
func TestChooseAnswer(t *testing.T) {
	e := newEnv(t)
	opts := []string{"Photoshop", "qBittorrent", "Transmission", "Excel"}

	e.llm.push(reply{text: "2"})
	got, err := e.svc.ChooseAnswer(context.Background(), "以下哪个是 BT 客户端？", opts, false)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, got)
	req := e.llm.requests()[0]
	assert.Equal(t, "fake-model", req.Model)
	for _, w := range []string{"以下哪个是 BT 客户端？", "1. Photoshop", "4. Excel", "单选"} {
		assert.Contains(t, req.Raw, w)
	}

	e.llm.push(reply{text: "答案：2, 3"})
	got, err = e.svc.ChooseAnswer(context.Background(), "以下哪些是 BT 客户端？", opts, true)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, got)
	assert.Contains(t, e.llm.requests()[1].Raw, "多选")

	e.llm.push(reply{text: "<think>想一想</think>选 B"})
	got, err = e.svc.ChooseAnswer(context.Background(), "q", opts, false)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, got, "回答写成字母也认")

	e.llm.push(reply{text: "单选只取第一个：3 或 1"})
	got, err = e.svc.ChooseAnswer(context.Background(), "q", opts, false)
	require.NoError(t, err)
	assert.Equal(t, []int{2}, got)

	e.llm.push(reply{text: "都不对，选 9"})
	_, err = e.svc.ChooseAnswer(context.Background(), "q", opts, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "选项编号")
}

// 「测试看图」：现画一张验证码让模型认，比对认得对不对。
func TestTestVision(t *testing.T) {
	e := newEnv(t)
	e.svc.newCaptchaText = func() string { return "HK7PN3" }

	e.llm.push(reply{text: "HK7PN3"})
	res, err := e.svc.TestVision(context.Background(), "vision-model")
	require.NoError(t, err)
	assert.True(t, res.OK)
	assert.Equal(t, "HK7PN3", res.Expected)
	assert.Equal(t, "HK7PN3", res.Got)
	assert.Equal(t, "vision-model", res.Model)

	e.llm.push(reply{text: "HK1PN3"})
	res, err = e.svc.TestVision(context.Background(), "")
	require.NoError(t, err)
	assert.False(t, res.OK)
	assert.Equal(t, 1, res.Mismatch)
	assert.Equal(t, "fake-model", res.Model)

	e.llm.push(reply{text: "认不出"})
	res, err = e.svc.TestVision(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 6, res.Mismatch)
}

func TestRenderCaptcha(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(renderCaptcha("AB12CD")))
	require.NoError(t, err)
	b := img.Bounds()
	assert.Greater(t, b.Dx(), 100)
	dark := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r < 0x4000 {
				dark++
			}
		}
	}
	assert.Greater(t, dark, 200, "字要画出来")
	text := randomCaptchaText()
	assert.Len(t, text, 6)
	assert.Equal(t, strings.ToUpper(text), text)
	for _, r := range captchaChars {
		_, ok := captchaGlyphs[r]
		assert.True(t, ok, "字符 %c 没有字形", r)
	}
}
