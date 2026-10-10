// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestSLoggerFallbackAndGlobal(t *testing.T) {
	// InitLogger 只生效一次：这里清空的全局 logger 要还回去，不然之后走 global.GetSlogger() 的测试会空指针
	orig := global.GlobalLogger
	if orig == nil {
		orig = zap.NewNop()
	}
	t.Cleanup(func() { global.GlobalLogger = orig })
	// fallback when global logger is nil
	global.GlobalLogger = nil
	if sLogger() == nil {
		t.Fatalf("nil")
	}
	// when global logger is set
	global.InitLogger(zap.NewNop())
	if sLogger() == nil {
		t.Fatalf("nil")
	}
}

func TestScheduleTorrentForMonitoring(t *testing.T) {
	// No scheduler registered -> no-op, no panic.
	ScheduleTorrentForMonitoring(models.TorrentInfo{TorrentID: "x"})

	var got string
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { got = ti.TorrentID })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	ScheduleTorrentForMonitoring(models.TorrentInfo{TorrentID: "sched-1"})
	assert.Equal(t, "sched-1", got)
}

func TestSetAndGetRSSNotifier(t *testing.T) {
	// initially unset within a fresh binary is not guaranteed; set then read.
	SetRSSNotifier(nil)
	assert.Nil(t, getRSSNotifier())

	n := stubRSSNotifier{}
	SetRSSNotifier(n)
	t.Cleanup(func() { SetRSSNotifier(nil) })
	assert.NotNil(t, getRSSNotifier())
}
