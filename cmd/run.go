/*
Copyright © 2024 sunerpy <nkuzhangshn@gmail.com>
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:
The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/utils"
)

var (
	modeFlag string // 用于存储运行模式的标志值
	runCmd   = &cobra.Command{
		Use:   "run",
		Short: "以脚本模式运行（建议使用 Web）",
		Long: `优先推荐通过 'pt-tools web' 进行管理与启动。
在需要脚本化或后台执行的场景下，可使用 run：
- single: 单次运行并退出
- persistent: 持续运行（按配置间隔执行任务）
示例：
  pt-tools run --mode=single
  pt-tools run --mode=persistent
`,
		Run:       runCmdFunc,
		PreRun:    PersistentCheckCfg,
		ValidArgs: []string{"single", "persistent"},
		Hidden:    true,
	}
)

func init() {
	// run 子命令已废弃，仅保留 Web 方式运行
	// 定义 mode 标志
	runCmd.Flags().StringVarP(&modeFlag, "mode", "m", "single", "Mode to run: single or persistent")
	runCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err.Error() == "flag needs an argument: 'm' in -m" {
			fmt.Println("Error: The '-m' flag requires a value. Use '--mode=single' or '--mode=persistent'.")
			fmt.Println()
			_ = cmd.Usage() // 显示帮助信息
			os.Exit(1)
		}
		return err
	})
}

func runCmdFunc(cmd *cobra.Command, args []string) {
	// 读取 mode 标志值
	mode, _ := cmd.Flags().GetString("mode")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 信号监听
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		sLogger().Warn("收到退出信号，正在退出...")
		cancel()
	}()
	lockPath := "/tmp/pt-tools.lock" // Windows 下自动映射为 C:\tmp\...
	locker, err := utils.NewLocker(lockPath)
	if err != nil {
		color.Red("无法打开锁文件: %v", err)
		os.Exit(1)
	}
	if err := locker.Lock(); err != nil {
		color.Red("已有实例正在运行，请勿重复启动")
		os.Exit(1)
	}
	defer func() { _ = locker.Unlock() }()
	switch mode {
	case "single":
		sLogger().Info("运行模式: 单次运行")
		if err := genTorrentsWithRSSOnce(ctx); err != nil {
			color.Red("Error: %v", err)
			os.Exit(1)
		}
		color.Green("程序已成功完成单次运行！")
	case "persistent":
		sLogger().Info("运行模式: 持续运行")
		if err := genTorrentsWithRSS(ctx); err != nil {
			color.Red("Error: %v", err)
			os.Exit(1)
		}
		color.Green("程序已成功退出！")
	default:
		color.Red("Error: 无效的运行模式 '%s'，仅支持 'single' 或 'persistent'", mode)
		_ = cmd.Usage()
		os.Exit(1)
	}
}
