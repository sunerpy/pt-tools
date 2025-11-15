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
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sunerpy/pt-tools/models"
)

const (
	defaultDowndir = "downloads"
)

// initCmd represents the init command
var configInitCmd = &cobra.Command{
	Use:     "init",
	Short:   "初始化运行所需目录",
	Long:    "创建 ~/.pt-tools 及 downloads 目录用于持久化运行所需数据",
	Example: `  pt-tools config init`,
	Run:     initConfigAndDBFile,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	// Here you will define your flags and configuration settings.
	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")
	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func chekcAndInitDownloadPath(dir string) error {
	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 创建目录
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建工作目录: %v", err)
		}
		color.Green("创建配置目录: %s", dir)
	}
	downDir := filepath.Join(dir, defaultDowndir)
	if _, err := os.Stat(downDir); os.IsNotExist(err) {
		// 创建目录
		if err := os.MkdirAll(downDir, 0o755); err != nil {
			return fmt.Errorf("无法创建下载目录: %v", err)
		}
		color.Green("创建下载目录: %s", downDir)
	}
	return nil
}

func initConfigAndDBFile(cmd *cobra.Command, args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		color.Red("无法获取用户主目录: %v", err)
		os.Exit(1)
	}
	if err := chekcAndInitDownloadPath(filepath.Join(home, models.WorkDir)); err != nil {
		color.Red("初始化配置失败: %v", err)
		os.Exit(1)
	}
	color.Green("目录初始化成功！可直接运行 pt-tools 进入 Web 完成配置")
}
