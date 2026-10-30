package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

/*
 * 站点上已经没有的种子（2026-10-30 生产日志，IYUU 辅种）：IYUU 的索引里还有 M-Team 的 504022，取到签名下载地址以后，
 * 下载节点回的是 {"code":"1","data":null,"message":"種子未找到"}，不是种子文件。以前报「not bencode format」、记错误日志，
 * 辅种记为可重试，过 7 天还去下。现在认出是 ErrTorrentNotFound：辅种不再重试，日志是警告。
 */

const mteamNotFoundBody = `{"code":"1","data":null,"message":"種子未找到"}`

func TestMTorrentDriver_ParseDownload_TorrentNotFound(t *testing.T) {
	quickDownloadRetry(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mteamNotFoundBody))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/api/rss/dlv2?sign=TOPSECRET&tid=504022&uid=340429"`)})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTorrentNotFound)
	assert.Contains(t, err.Error(), "種子未找到", "带上站点的原话")
	assert.NotContains(t, err.Error(), "bencode")
	assert.NotContains(t, err.Error(), "TOPSECRET")
	assert.NotContains(t, err.Error(), "340429", "不写下载地址（里面有账号的 uid）")
	assert.Equal(t, int32(1), hits.Load(), "种子不在了不重试")
}

// 下载节点回了别的 JSON 错误：写站点的原话，不当成种子不在了，也不说「格式不对」。
func TestMTorrentDriver_ParseDownload_JSONErrorKeepsSiteMessage(t *testing.T) {
	quickDownloadRetry(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"401","data":null,"message":"簽名已過期"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/api/rss/dlv2?sign=TOPSECRET"`)})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrTorrentNotFound))
	assert.Contains(t, err.Error(), "簽名已過期")
	assert.Contains(t, err.Error(), "401")
	assert.NotContains(t, err.Error(), "bencode")
	assert.NotContains(t, err.Error(), "TOPSECRET")
}

// 取下载地址那一步就说种子不在了（简体、繁体、英文）：同样是 ErrTorrentNotFound，不去拉第二段。
func TestMTorrentDriver_ParseDownload_TokenSaysNotFound(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	for _, msg := range []string{"種子未找到", "种子未找到", "Torrent not found"} {
		_, err := d.ParseDownload(MTorrentResponse{Code: "1", Message: msg, RawBody: []byte(`{"code":"1","message":"` + msg + `"}`)})
		require.Error(t, err, msg)
		assert.ErrorIs(t, err, ErrTorrentNotFound, msg)
	}
	_, err := d.ParseDownload(MTorrentResponse{Code: "1", Message: "fail", RawBody: []byte("errbody")})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrTorrentNotFound), "别的错误照旧")
}

// NexusPHP：种子删了以后详情页是「没有该ID的种子」（上游 lang_details.php 的 std_no_torrent_id，繁体与英文同理），
// 认出是 ErrTorrentNotFound；详情页上别的原因没有下载链接时，错误里写页面标题，看得出回的是什么页面。
func TestNexusPHPDriver_ParseDownload_TorrentNotFound(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://example.test", Cookie: "c=1"})
	page := func(html string) NexusPHPResponse {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		require.NoError(t, err)
		return NexusPHPResponse{Document: doc}
	}
	for _, text := range []string{"没有该ID的种子", "沒有該ID的種子", "No torrent with this ID"} {
		_, err := d.ParseDownloadContext(context.Background(), page(
			`<html><head><title>错误</title></head><body><h2>错误</h2><table><tr><td class="text">`+text+`</td></tr></table></body></html>`,
		))
		require.Error(t, err, text)
		assert.ErrorIs(t, err, ErrTorrentNotFound, text)
	}

	_, err := d.ParseDownloadContext(context.Background(), page(`<html><head><title>维护中</title></head><body>站点维护</body></html>`))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrTorrentNotFound))
	assert.Contains(t, err.Error(), "no download URL found in detail page")
	assert.Contains(t, err.Error(), "页面标题：维护中")
}

// BaseSite：种子不在了记一条警告（带种子 ID），不是错误；别的解析失败照旧记错误。
func TestBaseSite_Download_TorrentNotFoundIsWarning(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	driver := &MockDriver{}
	driver.On("PrepareDownload", "504022").Return("req", nil)
	driver.On("PrepareDownload", "7").Return("req7", nil)
	driver.On("Execute", mock.Anything, "req").Return("resp", nil)
	driver.On("Execute", mock.Anything, "req7").Return("resp7", nil)
	driver.On("ParseDownload", "resp").Return([]byte(nil), errors.Join(ErrTorrentNotFound, errors.New("M-Team：種子未找到")))
	driver.On("ParseDownload", "resp7").Return([]byte(nil), errors.New("invalid torrent file: not bencode format"))
	site := NewBaseSite(driver, BaseSiteConfig{ID: "mteam", Name: "M-Team", Kind: SiteMTorrent, RateLimit: 100, RateBurst: 100, Logger: zap.New(core)})

	_, err := site.Download(context.Background(), "504022")
	require.ErrorIs(t, err, ErrTorrentNotFound)
	assert.Zero(t, logs.FilterLevelExact(zapcore.ErrorLevel).Len(), "种子不在了不记错误")
	warns := logs.FilterLevelExact(zapcore.WarnLevel).All()
	require.Len(t, warns, 1)
	assert.Equal(t, "504022", warns[0].ContextMap()["torrentID"])

	_, err = site.Download(context.Background(), "7")
	require.Error(t, err)
	errs := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	require.Len(t, errs, 1, "别的解析失败照旧是错误")
	assert.Equal(t, "7", errs[0].ContextMap()["torrentID"])
}
