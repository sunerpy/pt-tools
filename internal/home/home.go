// Package home 是首页（媒体中心 M8）：同时取各块的数据，一块出错不影响别的；另算待处理、存储与状态行。
// 各块的数据由各自的包算（今天播出在 subscribe、最近入库与继续观看在 organize），这里只编排。
package home

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// Block 是首页的一块：Data 是这一块的数据，Error 是没取到的原因（别的块照样给）。
type Block struct {
	Data  any    `json:"data"`
	Error string `json:"error,omitempty"`
}

// Part 是要取的一块。
type Part struct {
	Name string
	Get  func(ctx context.Context) (any, error)
}

// Collect 同时取各块，每块最多等 timeout：出错、超时与意外（panic）写进那一块的 Error（去掉凭证），别的块照样给。
func Collect(ctx context.Context, parts []Part, timeout time.Duration) map[string]Block {
	out := make(map[string]Block, len(parts))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range parts {
		wg.Go(func() {
			b := collect(ctx, p, timeout)
			mu.Lock()
			out[p.Name] = b
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

func collect(ctx context.Context, p Part, timeout time.Duration) Block {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ch := make(chan Block, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- Block{Error: fmt.Sprintf("出错了：%v", r)}
			}
		}()
		v, err := p.Get(ctx)
		if err != nil {
			ch <- Block{Error: clean(err.Error())}
			return
		}
		ch <- Block{Data: v}
	}()
	select {
	case b := <-ch:
		return b
	case <-ctx.Done():
		return Block{Error: fmt.Sprintf("%d 秒内没取到，稍后刷新", int(timeout/time.Second))}
	}
}

// clean 是给人看的错误：去掉凭证。
func clean(s string) string { return strings.TrimSpace(redact.Secrets(s)) }
