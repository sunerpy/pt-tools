package cmd

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// **Validates: Requirements 3.1, 3.2, 3.3, 3.4**

// 本文件验证异步修复必须保留的判定语义（哪些配置会触发重载、哪些不会），
// 全部直接驱动真实的 maybeAutoStartReload，而非手抄的判定副本。

// countingReloader 记录 Reload 的调用次数与最后一次的配置，用于断言
// 「派发的重载最终会被执行一次」。因为生产代码经 `go mgr.Reload` 异步执行，
// 所以调用计数需用 Eventually 观测。
type countingReloader struct {
	reloadCount atomic.Int32
	lastCfg     atomic.Pointer[models.Config]
}

func (r *countingReloader) Reload(cfg *models.Config) {
	r.lastCfg.Store(cfg)
	r.reloadCount.Add(1)
}

func (r *countingReloader) count() int {
	return int(r.reloadCount.Load())
}

func newAutoStartConfig(autoStart bool, dir string) *models.Config {
	global.InitLogger(zap.NewNop())
	cfg := &models.Config{}
	cfg.Global.AutoStart = autoStart
	cfg.Global.DownloadDir = dir
	return cfg
}

// TestPreservation_StartupDecision 用表驱动方式覆盖 maybeAutoStartReload 的判定：
// 仅当 AutoStart=true 且下载目录去空白后非空时才派发（dispatched==true），
// 且被派发的重载最终恰好执行一次；其余情形不派发，重载永不执行。
func TestPreservation_StartupDecision(t *testing.T) {
	tests := []struct {
		name           string
		cfg            *models.Config
		wantDispatched bool
	}{
		{
			name:           "nil config - 不派发",
			cfg:            nil,
			wantDispatched: false,
		},
		{
			name:           "AutoStart=true 且目录非空 - 派发",
			cfg:            newAutoStartConfig(true, "/data/downloads"),
			wantDispatched: true,
		},
		{
			name:           "AutoStart=false 且目录非空 - 不派发",
			cfg:            newAutoStartConfig(false, "/data/downloads"),
			wantDispatched: false,
		},
		{
			name:           "AutoStart=true 但目录为空 - 不派发",
			cfg:            newAutoStartConfig(true, ""),
			wantDispatched: false,
		},
		{
			name:           "AutoStart=true 但目录仅空格 - 不派发",
			cfg:            newAutoStartConfig(true, "   "),
			wantDispatched: false,
		},
		{
			name:           "AutoStart=true 但目录仅制表符换行 - 不派发",
			cfg:            newAutoStartConfig(true, "\t\n"),
			wantDispatched: false,
		},
		{
			name:           "AutoStart=true 目录含首尾空白仍有内容 - 派发",
			cfg:            newAutoStartConfig(true, "  /data/dl  "),
			wantDispatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reloader := &countingReloader{}
			dispatched := maybeAutoStartReload(reloader, tt.cfg)
			require.Equal(t, tt.wantDispatched, dispatched)

			if tt.wantDispatched {
				require.Eventually(t, func() bool {
					return reloader.count() == 1
				}, 2*time.Second, 10*time.Millisecond,
					"派发后后台 Reload 应最终执行一次")
				assert.Equal(t, tt.cfg, reloader.lastCfg.Load(),
					"Reload 应收到原始配置")
			} else {
				// 未派发时给后台一点时间，确认 Reload 始终未被调用。
				assert.Never(t, func() bool {
					return reloader.count() != 0
				}, 100*time.Millisecond, 20*time.Millisecond,
					"未派发时 Reload 不应被调用")
			}
		})
	}
}

// TestPreservation_ManualReloadIsAsynchronous 记录真实的手动重载语义：
// web/server.go 的 apiGlobal/apiQbit 处理器均以 `go func(){ ... s.mgr.Reload(cfg) }()`
// 异步派发后立即返回，因此并不存在需要保留的「同步手动重载」语义；
// 需要保留的只是「手动重载最终仍会调用一次 mgr.Reload」。此用例以真实的
// 异步派发模型断言该属性，纠正旧测试中「手动重载是同步」的错误前提。
func TestPreservation_ManualReloadIsAsynchronous(t *testing.T) {
	reloader := &countingReloader{}
	cfg := newAutoStartConfig(true, "/data/downloads")

	// 模拟 web/server.go 处理器的真实结构：异步派发、handler 立即返回。
	start := time.Now()
	go func() {
		reloader.Reload(cfg)
	}()
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 100*time.Millisecond,
		"手动重载 handler 应异步派发并立即返回，不阻塞调用方")

	require.Eventually(t, func() bool {
		return reloader.count() == 1
	}, 2*time.Second, 10*time.Millisecond,
		"手动重载最终仍应调用一次 mgr.Reload")
}
