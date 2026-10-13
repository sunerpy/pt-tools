package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
