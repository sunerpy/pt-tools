// Package cachectl 是媒体数据来源（TMDB、豆瓣、Bangumi、Fanart.tv、OMDb）的响应缓存共用的开关：
// 显式重新刮削时不读缓存，直接问来源要现在的数据（取到的照样写进缓存，替换旧的）。
package cachectl

import "context"

type bypassKey struct{}

// Bypass 返回一个不读缓存的 ctx：用它发的请求跳过缓存里的旧响应，取到的新响应照样写进缓存。
func Bypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, bypassKey{}, true)
}

// Bypassed 报告 ctx 是不是要跳过缓存读。
func Bypassed(ctx context.Context) bool {
	v, _ := ctx.Value(bypassKey{}).(bool)
	return v
}
