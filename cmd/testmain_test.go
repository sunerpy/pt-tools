package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
)

// TestMain 让整个包的测试用一个临时的家目录：InitRuntime 只跑一次（sync.Once），它打开的库和日志文件会一直开着，
// 落在某个测试的 t.TempDir() 里的话，Windows 上那个测试的临时目录删不掉（#542）；也不往真正的家目录里写。
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "pt-tools-cmd-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("USERPROFILE", home)
	// 密钥在包初始化时已按原来的家目录读过；换了家目录要重新读（没有就在临时目录里生成），否则 ConfigStore 找不到 secret.key
	crypto.ResetForTest()
	_, _ = core.InitRuntime()
	code := m.Run()
	if global.GlobalDB != nil {
		core.CloseDB(global.GlobalDB)
	}
	_ = config.CloseLogFiles()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
