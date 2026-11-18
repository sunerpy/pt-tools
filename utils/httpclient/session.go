package httpclient

import "github.com/sunerpy/requests"

// NewSession 是 requests.NewSession 的替身，新建时先把代理清掉。
//
// requests 的 Session 共用一个 http.Transport 池：带代理的 Session 关掉后，它的 Transport 连同 Proxy 一起回到池里，
// 下一个 NewSession 可能拿到它，本该直连的请求（没配代理的站点、NO_PROXY 里的主机）就走了别人的代理。
// 需要代理的调用方在这之后照常 WithProxy。项目里不要直接用 requests.NewSession。
func NewSession() requests.Session {
	return requests.NewSession().WithProxy("")
}
