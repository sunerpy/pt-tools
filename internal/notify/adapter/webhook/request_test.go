package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
	"github.com/sunerpy/pt-tools/models"
)

// received 是假接收方收到的一次请求
type received struct {
	Method, Path, Query, ContentType, Signature string
	Header                                      http.Header
	Body                                        []byte
}

func hookServer(t *testing.T, status int) (*httptest.Server, func() received) {
	t.Helper()
	var got received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = received{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			ContentType: r.Header.Get("Content-Type"), Signature: r.Header.Get("X-PT-Signature"),
			Header: r.Header.Clone(), Body: body,
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() received { return got }
}

func channelFor(t *testing.T, cfg map[string]any) *WebhookChannel {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	w := &WebhookChannel{}
	require.NoError(t, w.Init(context.Background(), &models.NotificationConf{Name: "我的钩子", ConfigJSON: string(raw)}))
	return w
}

func checkConfig(cfg map[string]any) error {
	raw, _ := json.Marshal(cfg)
	return (&WebhookChannel{}).CheckConfig(&models.NotificationConf{ChannelType: "webhook", ConfigJSON: string(raw)})
}

// 不写请求体模板时：原来的字段一个不少、值不变，只多一个 event
func TestWebhook_DefaultPayloadAddsEvent(t *testing.T) {
	srv, got := hookServer(t, http.StatusOK)
	w := channelFor(t, map[string]any{"endpoint_url": srv.URL})
	n := sampleNotification(notify.EventRSSMatched)
	n.Link, n.SourceConfID = "https://hdtime.org/details.php?id=300101", 9
	require.NoError(t, w.Send(context.Background(), n))

	r := got()
	assert.Equal(t, http.MethodPost, r.Method)
	assert.Equal(t, "application/json", r.ContentType)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(r.Body, &body))
	keys := make([]string, 0, len(body))
	for k := range body {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"event_type", "title", "text", "link", "payload", "event"}, keys)
	var old struct {
		EventType string         `json:"event_type"`
		Title     string         `json:"title"`
		Text      string         `json:"text"`
		Link      string         `json:"link"`
		Payload   map[string]any `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &old))
	assert.Equal(t, "notification", old.EventType)
	assert.Equal(t, n.Title, old.Title)
	assert.Equal(t, n.Text, old.Text)
	assert.Equal(t, n.Link, old.Link)
	assert.Equal(t, map[string]any{"channel_type": "webhook", "source_conf_id": float64(9)}, old.Payload)
	var ev notify.Event
	require.NoError(t, json.Unmarshal(body["event"], &ev))
	assert.Equal(t, notify.EventRSSMatched, ev.Type)
	assert.Equal(t, n.Event.Fields, ev.Fields)
}

func TestWebhook_RequestFormats(t *testing.T) {
	n := sampleNotification(notify.EventRSSMatched)
	cases := []struct {
		name               string
		cfg                map[string]any
		method, ct, body   string
		wantPath, wantQury string
	}{
		{
			name:   "json",
			cfg:    map[string]any{"body_template": `{"msg": {{json .title}}, "site": {{json .site}}}`},
			method: http.MethodPost, ct: "application/json",
			body: `{"msg": "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE", "site": "hdtime"}`,
		},
		{
			name:   "text with PUT",
			cfg:    map[string]any{"method": "put", "content_type": "text", "body_template": "{{.site}}：{{.size}}"},
			method: http.MethodPut, ct: "text/plain; charset=utf-8", body: "hdtime：15.00 GB",
		},
		{
			name:   "form with PATCH",
			cfg:    map[string]any{"method": "PATCH", "content_type": "form", "body_template": "site={{urlquery .site}}&t={{urlquery .torrent_title}}"},
			method: http.MethodPatch, ct: "application/x-www-form-urlencoded",
			body: "site=hdtime&t=Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE",
		},
		{
			name: "custom",
			cfg: map[string]any{
				"content_type": "custom", "body_template": "<msg>{{.site}}</msg>",
				"headers": map[string]string{"content-type": "application/xml"},
			},
			method: http.MethodPost, ct: "application/xml", body: "<msg>hdtime</msg>",
		},
		{
			name:   "GET with templated query",
			cfg:    map[string]any{"method": "GET", "path": "/send/{{.event}}?site={{urlquery .site}}&n={{.seeders}}"},
			method: http.MethodGet, ct: "", body: "",
			wantPath: "/send/rss.matched", wantQury: "site=hdtime&n=12",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := hookServer(t, http.StatusNoContent)
			cfg := map[string]any{"endpoint_url": srv.URL + "/hook"}
			for k, v := range tc.cfg {
				if k == "path" {
					cfg["endpoint_url"] = srv.URL + v.(string)
					continue
				}
				cfg[k] = v
			}
			require.NoError(t, checkConfig(cfg))
			w := channelFor(t, cfg)
			require.NoError(t, w.Send(context.Background(), n))
			r := got()
			assert.Equal(t, tc.method, r.Method)
			assert.Equal(t, tc.ct, r.ContentType)
			assert.Equal(t, tc.body, string(r.Body))
			if tc.wantPath != "" {
				assert.Equal(t, tc.wantPath, r.Path)
				assert.Equal(t, tc.wantQury, r.Query)
			}
		})
	}
}

func TestWebhook_CheckConfigRejects(t *testing.T) {
	base := "https://example.com/hook"
	bad := []struct {
		cfg  map[string]any
		want string
	}{
		{map[string]any{"method": "DELETE"}, "请求方法只能是"},
		{map[string]any{"content_type": "xml"}, "请求体格式只能是"},
		{map[string]any{"content_type": "text"}, "就要写请求体模板"},
		{map[string]any{"method": "GET", "body_template": "{{.title}}"}, "GET 请求不带请求体"},
		{map[string]any{"content_type": "custom", "body_template": "x"}, "就要在请求头里写 Content-Type"},
		{map[string]any{"endpoint_url": "ftp://example.com/x"}, "http:// 或 https://"},
		{map[string]any{"endpoint_url": "example.com/hook"}, "http:// 或 https://"},
		{map[string]any{"endpoint_url": "https://{{.site}}.example.com/x"}, "只能写在路径和查询里"},
		{map[string]any{"endpoint_url": "https://example.com{{.site}}"}, "只能写在路径和查询里"},
		{map[string]any{"endpoint_url": "{{.link}}"}, "只能写在路径和查询里"},
		{map[string]any{"body_template": `{"a": {{.title}}}`}, "不是合法的 JSON"},
		{map[string]any{"body_template": `{"a": {{json .titel}}}`}, "没有这个变量：titel"},
		{map[string]any{"body_template": `{{range .timestamp}}{{end}}`}, "range 只能遍历列表变量"},
		{map[string]any{"headers": "nope"}, "headers 要是 JSON 对象"},
	}
	for _, tc := range bad {
		cfg := map[string]any{"endpoint_url": base}
		for k, v := range tc.cfg {
			cfg[k] = v
		}
		err := checkConfig(cfg)
		require.Error(t, err, tc.cfg)
		assert.Contains(t, err.Error(), tc.want, tc.cfg)
	}
	// 变量名写错时写明是哪个事件
	err := checkConfig(map[string]any{"endpoint_url": base, "body_template": `{"a": {{json .titel}}}`})
	assert.Contains(t, err.Error(), "（rss.new）")
	// 只在某种事件里有的变量也能用：其他事件里是零值
	assert.NoError(t, checkConfig(map[string]any{"endpoint_url": base, "body_template": `{"d": {{json .downloader}}, "n": {{.count}}}`}))
}

// 3xx 没有跟到新地址时没送到，算失败（原来只把 400 以上算失败）
func TestWebhook_RedirectStatusIsFailure(t *testing.T) {
	srv, _ := hookServer(t, http.StatusFound)
	w := channelFor(t, map[string]any{"endpoint_url": srv.URL})
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "302")
}

func TestWebhook_HMACOverRenderedBody(t *testing.T) {
	srv, got := hookServer(t, http.StatusOK)
	w := channelFor(t, map[string]any{
		"endpoint_url": srv.URL, "hmac_secret": "topsecret",
		"content_type": "text", "body_template": "{{.event}} {{.channel}}",
	})
	require.NoError(t, w.Send(context.Background(), sampleNotification(notify.EventTest)))
	r := got()
	assert.Equal(t, "test 我的钩子", string(r.Body), "channel 是通道名称")
	assert.Equal(t, computeSig(t, "topsecret", r.Body), r.Signature)
}

// 旧版编辑页把请求头存成了 JSON 文本：照样能用
func TestWebhook_LegacyStringHeaders(t *testing.T) {
	srv, got := hookServer(t, http.StatusOK)
	w := channelFor(t, map[string]any{"endpoint_url": srv.URL, "headers": `{"X-Tenant": "acme"}`})
	require.NoError(t, w.Send(context.Background(), sampleNotification(notify.EventTest)))
	assert.Equal(t, "acme", got().Header.Get("X-Tenant"))
	// 空字符串当没有
	assert.NoError(t, checkConfig(map[string]any{"endpoint_url": srv.URL, "headers": ""}))
}

// 错误与日志里不出现地址的查询部分（可能带 token）
func TestWebhook_ErrorsHideQuery(t *testing.T) {
	w := channelFor(t, map[string]any{"endpoint_url": "http://127.0.0.1:1/x?token=S3CRET", "timeout_seconds": 1})
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "S3CRET")
	assert.NotContains(t, err.Error(), "://")

	// 地址模板渲染错误也不带地址
	_, err = Preview(PreviewInput{Type: notify.EventTest, EndpointURL: "https://example.com/x?token=S3CRET&v={{.nope}}"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "S3CRET")
}

// outbox 重试：通知经 JSON 往返以后渲染出来的请求体和实时发送时完全一样
func TestWebhook_RetryRendersSameBody(t *testing.T) {
	r, err := compile(&Config{
		EndpointURL:  "https://example.com/hook",
		BodyTemplate: `{"bonus": {{.bonus}}, "gt": {{if gt .bonus 1000}}true{{else}}false{{end}}, "up": {{printf "%d" .uploaded_bytes}}, "items": {{json .items}}}`,
	})
	require.NoError(t, err)
	live := sampleNotification(notify.EventReportDaily)
	raw, err := json.Marshal(live)
	require.NoError(t, err)
	var retried notify.Notification
	require.NoError(t, json.Unmarshal(raw, &retried))
	a, err := r.render(live, "x", false)
	require.NoError(t, err)
	b, err := r.render(retried, "x", false)
	require.NoError(t, err)
	assert.Equal(t, string(a.Body), string(b.Body))
	assert.True(t, json.Valid(a.Body), string(a.Body))
}

// 没有事件的通知（升级前留下的重试）也能渲染：事件变量都是零值
func TestWebhook_NoEvent(t *testing.T) {
	srv, got := hookServer(t, http.StatusOK)
	w := channelFor(t, map[string]any{"endpoint_url": srv.URL, "body_template": `{"t": {{json .title}}, "e": {{json .event}}}`})
	require.NoError(t, w.Send(context.Background(), notify.Notification{Title: "旧的", Text: "x"}))
	assert.JSONEq(t, `{"t": "旧的", "e": ""}`, string(got().Body))
}

func TestPreview(t *testing.T) {
	out, err := Preview(PreviewInput{
		Type: notify.EventRSSMatched, EndpointURL: "https://example.com/hook?site={{urlquery .site}}",
		BodyTemplate: `{"t": {{json .torrent_title}}}`, Headers: Headers{"Authorization": "Bearer x"},
	})
	require.NoError(t, err)
	assert.Equal(t, &PreviewResult{
		Method: http.MethodPost, URL: "https://example.com/hook?site=hdtime",
		ContentType: "application/json", Body: `{"t": "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE"}`,
	}, out)

	out, err = Preview(PreviewInput{Type: notify.EventTest, Method: "GET", EndpointURL: "https://example.com/hook"})
	require.NoError(t, err)
	assert.Equal(t, &PreviewResult{Method: http.MethodGet, URL: "https://example.com/hook"}, out)

	_, err = Preview(PreviewInput{Type: "nope", EndpointURL: "https://example.com/hook"})
	assert.ErrorIs(t, err, ErrUnknownEvent)
	_, err = Preview(PreviewInput{Type: notify.EventTest})
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = Preview(PreviewInput{Type: notify.EventTest, EndpointURL: "https://example.com/", BodyTemplate: "{{.nope}}"})
	assert.ErrorIs(t, err, tmpl.ErrInvalid)
}

// 按响应内容判断成败：很多服务出错也回 200，错误写在响应里
func TestCheckResponse(t *testing.T) {
	check := func(src string, status int, body string) error {
		t.Helper()
		r, err := compile(&Config{EndpointURL: "https://example.com/hook", SuccessTemplate: src})
		require.NoError(t, err, src)
		return r.checkResponse(status, []byte(body))
	}
	// 没写成功条件：只看状态码（调用方判过 2xx）
	r, err := compile(&Config{EndpointURL: "https://example.com/hook"})
	require.NoError(t, err)
	assert.NoError(t, r.checkResponse(200, []byte(`{"code":500}`)))

	assert.NoError(t, check(`{{eq .json.code 200}}`, 200, `{"code":200,"msg":"ok"}`))
	assert.ErrorIs(t, check(`{{eq .json.code 200}}`, 200, `{"code":500,"msg":"token 错误"}`), errResponseRejected)
	assert.NoError(t, check(`  TRUE `, 200, ``), "true 不分大小写、去掉空白")
	assert.NoError(t, check(`{{eq .body "ok"}}`, 200, `ok`))
	assert.NoError(t, check(`{{gt .json.id 0}}`, 200, `{"id": 12, "title": "x"}`))
	assert.NoError(t, check(`{{eq .json.data.ok true}}`, 200, `{"data":{"ok":true}}`), "嵌套字段")
	assert.NoError(t, check(`{{eq .status 201}}`, 201, ``))

	// 自己写的失败原因带进错误里（截到 200 个字）
	err = check(`{{if eq .json.code 0}}true{{else}}{{.json.msg}}{{end}}`, 200, `{"code":40001,"msg":"invalid token"}`)
	require.ErrorIs(t, err, errResponseRejected)
	assert.Contains(t, err.Error(), "invalid token")
	// 不在 checkResponse 里截短（先去凭证再截，见 scrubError）
	err = check(`{{.body}}`, 200, strings.Repeat("长", 300))
	assert.Contains(t, err.Error(), strings.Repeat("长", 300))
	assert.Less(t, len([]rune(scrubError(err, nil).Error())), maxReasonRunes+2)

	// 响应不是 JSON：取字段得零值，判断为不成功；算不出来的也算失败
	assert.ErrorIs(t, check(`{{eq .json.code 0}}`, 200, `<html>busy</html>`), errResponseRejected)
	err = check(`{{lt .body 1}}`, 200, `x`)
	require.ErrorIs(t, err, errResponseRejected)
	assert.Contains(t, err.Error(), "算不出来")
}

func TestWebhook_SuccessTemplateOnSend(t *testing.T) {
	var reply string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	w := channelFor(t, map[string]any{
		"endpoint_url":     srv.URL,
		"success_template": `{{if eq .json.code 0}}true{{else}}{{.json.msg}}{{end}}`,
	})
	reply = `{"code":0}`
	require.NoError(t, w.Send(context.Background(), sampleNotification(notify.EventTest)))
	reply = `{"code":1,"msg":"quota exceeded"}`
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")

	// 成功条件写错（没有这个函数）：保存前就被拒
	assert.ErrorContains(t, checkConfig(map[string]any{"endpoint_url": srv.URL, "success_template": `{{nope .json}}`}), "成功条件")
}

func TestPreviewSuccessTemplate(t *testing.T) {
	in := PreviewInput{
		Type: notify.EventTest, EndpointURL: "https://example.com/hook",
		SuccessTemplate: `{{eq .json.code 200}}`, SampleResponse: `{"code":200}`,
	}
	out, err := Preview(in)
	require.NoError(t, err)
	require.NotNil(t, out.Success)
	assert.True(t, *out.Success)

	in.SampleResponse = `{"code":500}`
	out, err = Preview(in)
	require.NoError(t, err)
	assert.False(t, *out.Success)
	assert.Contains(t, out.SuccessReason, "不满足成功条件")

	in.SampleResponse, in.SampleStatus = `{"code":200}`, 302
	out, err = Preview(in)
	require.NoError(t, err)
	assert.False(t, *out.Success, "状态码不是 2xx 就不看成功条件")
	assert.Contains(t, out.SuccessReason, "302")

	// 没写成功条件：不给 success
	out, err = Preview(PreviewInput{Type: notify.EventTest, EndpointURL: "https://example.com/hook"})
	require.NoError(t, err)
	assert.Nil(t, out.Success)
}

// 对方的响应把我们发过去的凭证回显出来（地址查询、路径里的 token、请求头、请求体里的 token 字段、HMAC Secret）：
// 成功条件拿它当失败原因时，写进错误、日志与重试记录以前都要去掉。
func TestWebhook_FailureReasonHidesEchoedSecrets(t *testing.T) {
	var echo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(echo))
	}))
	t.Cleanup(srv.Close)
	pathToken := "AbCdEfGhIjKlMnOpQrStUvWx"
	w := channelFor(t, map[string]any{
		"endpoint_url":     srv.URL + "/api/webhooks/123/" + pathToken + "?token=S3CRET-Q",
		"headers":          map[string]string{"Authorization": "Bearer S3CRET-H"},
		"hmac_secret":      "S3CRET-M",
		"body_template":    `{"token": "S3CRET-B", "content": {{json .title}}}`,
		"success_template": `{{if eq .json.code 0}}true{{else}}{{.json.msg}}{{end}}`,
	})
	echo = `{"code":1,"msg":"bad: S3CRET-Q S3CRET-H S3CRET-B S3CRET-M ` + pathToken + ` pt-tools 测试通知"}`
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.Error(t, err)
	assert.ErrorIs(t, err, errResponseRejected)
	for _, secret := range []string{"S3CRET-Q", "S3CRET-H", "S3CRET-B", "S3CRET-M", pathToken} {
		assert.NotContains(t, err.Error(), secret)
	}
	assert.Contains(t, err.Error(), "pt-tools 测试通知", "不是凭证的内容照样留着")
}

// 响应只读前 64 KiB（对方回几百 MB 也不会都读进内存）；不跟随跳转。
func TestWebhook_ResponseCapAndNoRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4<<20)))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusFound)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	w := channelFor(t, map[string]any{"endpoint_url": srv.URL + "/big", "success_template": `{{eq (len .body) 65536}}`})
	require.NoError(t, w.Send(context.Background(), sampleNotification(notify.EventTest)))

	w = channelFor(t, map[string]any{"endpoint_url": srv.URL + "/moved"})
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "302")
}

// 太短的凭证（Bearer abc 里的 abc）没法放心替换：原因里出现它时整段不显示；普通的短查询参数（v=1）不算凭证
func TestWebhook_FailureReasonShortSecret(t *testing.T) {
	var echo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(echo))
	}))
	t.Cleanup(srv.Close)
	w := channelFor(t, map[string]any{
		"endpoint_url":     srv.URL + "/hook?v=1",
		"headers":          map[string]string{"Authorization": "Bearer abc"},
		"success_template": `{{.json.msg}}`,
	})
	echo = `{"msg":"token abc is invalid"}`
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.ErrorIs(t, err, errResponseRejected)
	assert.NotContains(t, err.Error(), "abc")
	assert.Contains(t, err.Error(), "不显示")

	echo = `{"msg":"quota 1 left"}`
	err = w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.ErrorIs(t, err, errResponseRejected)
	assert.Contains(t, err.Error(), "quota 1 left", "普通的短查询参数不触发")
}

// 凭证正好横跨截断处：先去凭证再截短，截下来的半截凭证不会漏出去
func TestWebhook_FailureReasonSecretAcrossCut(t *testing.T) {
	var echo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(echo))
	}))
	t.Cleanup(srv.Close)
	w := channelFor(t, map[string]any{
		"endpoint_url":     srv.URL + "/hook",
		"headers":          map[string]string{"X-Key": "LONGSECRETVALUE1"},
		"success_template": `{{.json.msg}}`,
	})
	prefix := "webhook 响应不满足成功条件："
	pad := strings.Repeat("x", maxReasonRunes-len([]rune(prefix))-4)
	echo = `{"msg":"` + pad + `LONGSECRETVALUE1 tail"}`
	err := w.Send(context.Background(), sampleNotification(notify.EventTest))
	require.ErrorIs(t, err, errResponseRejected)
	assert.NotContains(t, err.Error(), "LONG")
}
