package definitions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// Rousi 的驱动工厂把站点 ID 交给了 HTTP 客户端：站点代理选中它时请求经代理，没选中时直连。
func TestRousiFactoryPassesSiteIDToHTTPClient(t *testing.T) {
	var proxied, direct atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxied.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(proxy.Close)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		direct.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(target.Close)
	t.Cleanup(httpclient.SetEnvProxyForTest(""))
	t.Cleanup(func() { httpclient.SetSiteProxyPolicy(nil) })

	opts, err := json.Marshal(map[string]string{"passkey": "p"})
	require.NoError(t, err)
	site, err := createRousiDriver(v2.SiteConfig{ID: "RousiPro", Name: "Rousi", BaseURL: target.URL, Options: opts}, zap.NewNop())
	require.NoError(t, err)
	driver := site.(*v2.BaseSite[rousiRequest, rousiResponse]).GetDriver().(*rousiDriver)

	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"rousipro": true}})
	_, err = driver.Execute(context.Background(), rousiRequest{Endpoint: "/api/v1/me"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), proxied.Load(), "选中：经代理")
	assert.Equal(t, int32(0), direct.Load())

	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: proxy.URL, Mode: httpclient.SiteProxyInclude, Sites: map[string]bool{"hdsky": true}})
	_, err = driver.Execute(context.Background(), rousiRequest{Endpoint: "/api/v1/me"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), proxied.Load())
	assert.Equal(t, int32(1), direct.Load(), "没选中：直连")
}
