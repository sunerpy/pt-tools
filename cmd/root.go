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
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
)

const (
	dbFile = "torrents.db"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pt-tools",
	Short: "pt-tools: Web 管理与任务自动化",
	Long:  `pt-tools 提供 Web 管理界面与命令行工具，支持任务运行、数据库管理与配置；直接运行将启动 Web 服务。`,
	Example: `  直接启动 Web
  pt-tools
  指定地址与端口
  pt-tools web --host=0.0.0.0 --port=8080
  文档
  https://github.com/sunerpy/pt-tools#readme
  https://raw.githubusercontent.com/sunerpy/pt-tools/main/examples/binary-run.md`,
	Run: func(cmd *cobra.Command, args []string) {
		webCmd.Run(cmd, args)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// initTools()
	// cobra.OnInitialize(initTools)
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// 无需根级 toggle
	// if err := initTools(); err != nil {
	// 	color.Red("Failed to load configuration: %s\n",cfgFile)
	// 	panic(err)
	// }
}

func initTools() error {
	logger, err := core.InitRuntime()
	if err != nil {
		color.Red("Failed to load configuration\n")
		return err
	}
	global.InitLogger(logger)
	return nil
}
