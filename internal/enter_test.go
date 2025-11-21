package internal

import (
	"testing"

	"github.com/sunerpy/pt-tools/global"
	"go.uber.org/zap"
)

func TestSLoggerFallbackAndGlobal(t *testing.T) {
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
