package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requests 的 Session 共用一个 http.Transport 池：带代理的 Session 关掉后，它的 Transport 连同 Proxy 一起回到池里
// （站点重新注册时旧实例关闭、每个走环境代理的请求用完的临时 Session 都是这样）。之后新建的客户端拿到它，
// 本该直连的请求就走了那个代理 —— QA 里指向 127.0.0.1 的假站点全部报 proxyconnect；真实场景是配了代理、
// 又把内网 tracker 或局域网服务写进 NO_PROXY 的用户。
func TestSiteHTTPClient_ClosedProxiedClientDoesNotLeakProxy(t *testing.T) {
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer direct.Close()
	t.Setenv("NO_PROXY", "127.0.0.1,localhost")
	t.Setenv("no_proxy", "127.0.0.1,localhost")

	ctx := context.Background()
	for i := range 20 {
		cfg := DefaultSiteHTTPClientConfig()
		cfg.ProxyURL = "http://127.0.0.1:9" // 不可达的代理
		proxied := NewSiteHTTPClient(cfg)
		_, err := proxied.Get(ctx, direct.URL, nil)
		require.Error(t, err, "显式配置的代理应当生效")
		require.NoError(t, proxied.Close())

		fresh := NewSiteHTTPClient(DefaultSiteHTTPClientConfig())
		resp, err := fresh.Get(ctx, direct.URL, nil)
		require.NoError(t, err, "第 %d 次：没配代理的客户端被带进了别人的代理", i+1)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, fresh.Close())
	}
}
