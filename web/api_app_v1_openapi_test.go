package web

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// App API 的契约（docs/reference/app-api-v1.yaml）：App 的 Dart 模型由它生成，这里用它校验真实 handler 的回应。
// App API 的测试都经 appAs / appAsWith / appWithID 调处理函数，回应在那里交给 validateAppResponse。

var appSpec = sync.OnceValues(func() (routers.Router, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../docs/reference/app-api-v1.yaml")
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(context.Background()); err != nil {
		return nil, err
	}
	return legacy.NewRouter(doc)
})

// validateAppResponse 用契约校验一个 App API 回应：状态码有定义，Content-Type 是契约里写的，内容合乎 schema（图片按二进制，只核对类型）。
func validateAppResponse(t testing.TB, req *http.Request, w *httptest.ResponseRecorder) {
	t.Helper()
	router, err := appSpec()
	require.NoError(t, err, "读 docs/reference/app-api-v1.yaml")
	// 契约里的地址带 /api/app/v1 前缀；路由按方法与路径找
	r := req.Clone(context.Background())
	r.Body = http.NoBody
	route, params, err := router.FindRoute(r)
	if !assert.NoError(t, err, "契约里没有 %s %s", req.Method, req.URL.Path) {
		return
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: r, PathParams: params, Route: route},
		Status:                 w.Code,
		Header:                 w.Header(),
		Body:                   io.NopCloser(bytes.NewReader(w.Body.Bytes())),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	assert.NoError(t, openapi3filter.ValidateResponse(context.Background(), in), "%s %s 的回应（%d）不合契约：%s", req.Method, req.URL.Path, w.Code, w.Body.String())
}

// 路由表里的每条 App API 都在契约里，契约里也没有多出来的接口
func TestAppOpenAPICoversRoutes(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../docs/reference/app-api-v1.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	inSpec := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			inSpec[method+" "+path] = true
		}
	}
	srv := &Server{}
	var missing, extra []string
	inRoutes := map[string]bool{}
	for _, r := range srv.appRoutes() {
		k := r.Method + " " + r.Path
		inRoutes[k] = true
		if !inSpec[k] {
			missing = append(missing, k)
		}
	}
	for k := range inSpec {
		if !inRoutes[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	assert.Empty(t, missing, "路由表里有、契约里没有")
	assert.Empty(t, extra, "契约里有、路由表里没有")
	for _, op := range []string{"getMeta", "listTorrents", "getTmdbImage"} {
		found := false
		for _, item := range doc.Paths.Map() {
			for _, o := range item.Operations() {
				found = found || o.OperationID == op
			}
		}
		assert.True(t, found, op)
	}
}
