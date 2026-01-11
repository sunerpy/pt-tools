package internal

import (
	"testing"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
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
