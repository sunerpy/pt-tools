package assistant

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const voiceToken = "voltip-token-0123456789"

// fakeVoltip 是假的 Voltip 本机服务：认令牌，记下收到的模型与语言，回固定的文字。
type fakeVoltip struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    []map[string]string
	status int
}

func newFakeVoltip(t *testing.T) *fakeVoltip {
	f := &fakeVoltip{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+voiceToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		status := f.status
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"忙"}}`)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"voltip","name":"默认处理方式"},{"id":"voltip:raw","name":"  不使用   AI 润色 "},{"id":"voltip:scene=coding"},{"id":"has space"},{"id":""}]}`)
		case "/v1/audio/transcriptions":
			require.NoError(t, r.ParseMultipartForm(32<<20))
			file, hdr, err := r.FormFile("file")
			require.NoError(t, err)
			data, _ := io.ReadAll(file)
			f.mu.Lock()
			f.got = append(f.got, map[string]string{
				"model": r.FormValue("model"), "language": r.FormValue("language"), "name": hdr.Filename, "riff": string(data[:4]),
			})
			f.mu.Unlock()
			_, _ = io.WriteString(w, `{"text":"  把沙丘重新整理一下  "}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// wavOf 是一段很短的 16 kHz 单声道 WAV。
func wavOf(samples int) []byte {
	data := make([]byte, samples*2)
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 32000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	return append(h, data...)
}

func (e *env) voiceSettings(base, token, model, lang string) Settings {
	e.t.Helper()
	key := testKey
	got, err := e.svc.SaveSettings(context.Background(), SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", APIKey: &key,
		VoiceBaseURL: &base, VoiceToken: &token, VoiceModel: &model, VoiceLanguage: &lang,
	})
	require.NoError(e.t, err)
	return got
}

// 语音输入：配了 Voltip 的地址与令牌才有；录音交给 /audio/transcriptions（带令牌、模型与语言），回的文字去掉首尾空白。
func TestVoiceTranscribe(t *testing.T) {
	e := newEnv(t)
	v := newFakeVoltip(t)
	e.svc.cfg.VoiceHTTPClient = v.srv.Client()
	ctx := context.Background()

	_, err := e.svc.Transcribe(ctx, wavOf(160), "")
	require.ErrorIs(t, err, ErrVoiceDisabled)
	st, err := e.svc.Status(ctx, admin)
	require.NoError(t, err)
	assert.False(t, st.Voice)

	got := e.voiceSettings(v.srv.URL+"/v1/", voiceToken, "", "zh")
	assert.Equal(t, v.srv.URL+"/v1", got.VoiceBaseURL, "末尾的 / 去掉")
	assert.True(t, got.HasVoiceToken)
	assert.NotContains(t, got.VoiceBaseURL, voiceToken)
	st, _ = e.svc.Status(ctx, admin)
	assert.True(t, st.Voice)

	text, err := e.svc.Transcribe(ctx, wavOf(1600), "")
	require.NoError(t, err)
	assert.Equal(t, "把沙丘重新整理一下", text)
	text, err = e.svc.Transcribe(ctx, wavOf(1600), "en")
	require.NoError(t, err)
	assert.NotEmpty(t, text)
	v.mu.Lock()
	assert.Equal(t, []map[string]string{
		{"model": DefaultVoiceModel, "language": "zh", "name": "speech.wav", "riff": "RIFF"},
		{"model": DefaultVoiceModel, "language": "en", "name": "speech.wav", "riff": "RIFF"},
	}, v.got)
	v.mu.Unlock()

	_, err = e.svc.Transcribe(ctx, []byte("not a wav file at all, definitely not RIFF"), "")
	require.ErrorIs(t, err, ErrInvalid)
	_, err = e.svc.Transcribe(ctx, wavOf(16), "zh;rm")
	require.ErrorIs(t, err, ErrInvalid)

	v.mu.Lock()
	v.status = http.StatusServiceUnavailable
	v.mu.Unlock()
	_, err = e.svc.Transcribe(ctx, wavOf(16), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "正忙")
}

// 换了地址又没给新令牌：存着的令牌清掉（只发到保存它时的地址）；令牌写不读；错的令牌说清楚。
func TestVoiceSettingsAndTest(t *testing.T) {
	e := newEnv(t)
	v := newFakeVoltip(t)
	e.svc.cfg.VoiceHTTPClient = v.srv.Client()
	ctx := context.Background()
	base := v.srv.URL + "/v1"
	e.voiceSettings(base, voiceToken, "voltip:raw", "")

	res, err := e.svc.TestVoice(ctx, SettingsInput{})
	require.NoError(t, err)
	assert.Equal(t, []VoiceModel{
		{ID: "voltip", Name: "默认处理方式"},
		{ID: "voltip:raw", Name: "不使用 AI 润色"},
		{ID: "voltip:scene=coding", Name: "voltip:scene=coding"},
	}, res.Models, "没有名字的用 ID；带空格的、空的丢掉")

	other := "http://127.0.0.1:1/v1"
	got, err := e.svc.SaveSettings(ctx, SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", VoiceBaseURL: &other,
	})
	require.NoError(t, err)
	assert.False(t, got.HasVoiceToken, "换了地址：令牌清掉")
	_, err = e.svc.TestVoice(ctx, SettingsInput{})
	require.ErrorIs(t, err, ErrInvalid)

	bad := "wrong-token-000000"
	_, err = e.svc.TestVoice(ctx, SettingsInput{VoiceBaseURL: &base, VoiceToken: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不认这个访问令牌")
	short := "x"
	_, err = e.svc.SaveSettings(ctx, SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", VoiceToken: &short,
	})
	require.ErrorIs(t, err, ErrInvalid)
	lang := "中文"
	_, err = e.svc.SaveSettings(ctx, SettingsInput{
		Enabled: true, Provider: "custom", BaseURL: e.llm.srv.URL + "/v1", Model: "fake-model", VoiceLanguage: &lang,
	})
	require.ErrorIs(t, err, ErrInvalid)
	assert.False(t, strings.Contains(err.Error(), voiceToken))
}
