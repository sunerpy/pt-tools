package httpclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/requests"
)

func getOK(t *testing.T, s requests.Session, url string) {
	t.Helper()
	req, err := requests.NewGet(url).Build()
	require.NoError(t, err)
	resp, err := s.Do(req)
	require.NoError(t, err, "没配代理的 Session 被带进了别人的代理")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// 带代理的 Session 关掉后 Transport 连同 Proxy 回到池里；NewSession 与 AcquireSession 拿到它时先把代理清掉。
func TestNewSessionClearsPooledProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	for range 10 {
		proxied := requests.NewSession().WithProxy("http://127.0.0.1:9")
		require.NoError(t, proxied.Close())
		s := NewSession()
		getOK(t, s, srv.URL)
		require.NoError(t, s.Close())

		proxied = requests.NewSession().WithProxy("http://127.0.0.1:9")
		require.NoError(t, proxied.Close())
		a := AcquireSession()
		getOK(t, a, srv.URL)
		ReleaseSession(a)
	}
}

// 钉子：项目代码不直接调用 requests.NewSession，一律走这里的 NewSession。
func TestNoDirectRequestsNewSession(t *testing.T) {
	root := filepath.Join("..", "..")
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "vendor", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, filepath.Join("httpclient", "session.go")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "requests.NewSession(") {
			offenders = append(offenders, path)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, offenders, "改用 httpclient.NewSession()：requests.NewSession 可能拿到带着别人代理的 Transport")
}
