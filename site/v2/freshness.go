package v2

import (
	"context"
	"time"
)

type freshAfterKey struct{}

// WithFreshAfter 要求取详情时只用 t 之后取得的数据。详情来自缓存的驱动（HDDolby 的详情是最近一次列表结果的缓存）
// 发现缓存早于 t 时先刷新再回答；刷新失败就返回错误，不拿旧数据充数。实时请求的驱动本来就满足，不受影响。
func WithFreshAfter(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, freshAfterKey{}, t)
}

// FreshAfterFrom 取出 WithFreshAfter 设置的时间；没有设置（或设置的是零时间）时 ok 为 false。
func FreshAfterFrom(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(freshAfterKey{}).(time.Time)
	if !ok || t.IsZero() {
		return time.Time{}, false
	}
	return t, true
}
