package downloader

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/sunerpy/requests"

	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// HTTPDoer defines the minimal contract used by downloader clients.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RequestsHTTPDoer adapts github.com/sunerpy/requests Session to HTTPDoer.
type RequestsHTTPDoer struct {
	session requests.Session
}

func NewRequestsHTTPDoer(baseURL string, timeout time.Duration) *RequestsHTTPDoer {
	session := requests.NewSession().WithTimeout(timeout)
	// 始终显式设置代理（空字符串表示清除），避免 requests 库复用带有陈旧代理的
	// 池化 *http.Transport 时，把本应直连（如环回地址命中 NO_PROXY）的请求错误地
	// 路由到上一次请求残留的代理上。仅在 proxyURL 非空时才 WithProxy 会遗留污染。
	session = session.WithProxy(httpclient.ResolveProxyFromEnvironment(baseURL))
	return &RequestsHTTPDoer{session: session}
}

func (d *RequestsHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	builder := requests.NewRequestBuilder(requests.Method(req.Method), req.URL.String())

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		builder = builder.WithBody(bytes.NewReader(bodyBytes))
	}

	r, err := builder.Build()
	if err != nil {
		return nil, err
	}

	for key, values := range req.Header {
		for _, value := range values {
			r.AddHeader(key, value)
		}
	}

	resp, err := d.session.DoWithContext(req.Context(), r)
	if err != nil {
		return nil, err
	}

	return &http.Response{
		StatusCode:    resp.StatusCode,
		Header:        resp.Headers,
		Body:          io.NopCloser(bytes.NewReader(resp.Bytes())),
		ContentLength: int64(len(resp.Bytes())),
		Request:       req,
	}, nil
}

func (d *RequestsHTTPDoer) Close() error {
	if d.session != nil {
		return d.session.Close()
	}
	return nil
}
