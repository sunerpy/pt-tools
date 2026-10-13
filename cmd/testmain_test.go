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
	// 初始化只跑一次（sync.Once）：第一次就出错时后面的测试拿到的都是 nil，所以在这里检查
	if _, err := core.InitRuntime(); err != nil {
		fmt.Fprintln(os.Stderr, "初始化运行时失败:", err)
		_ = os.RemoveAll(home)
		os.Exit(1)
	}
	// 记下这次初始化打开的库：测试会把 global.GlobalDB 换成自己的库，结束时要关的是这一个
	runtimeDB := global.GlobalDB
	code := m.Run()
	if runtimeDB != nil {
		core.CloseDB(runtimeDB)
	}
	_ = config.CloseLogFiles()
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, "删除临时家目录失败:", err)
	}
	os.Exit(code)
}
