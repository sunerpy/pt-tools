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
)

const (
	defaultDowndir = "downloads"
	defaultWorkdir = ".pt-tools"
)

// initCmd represents the init command
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:
Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: initConfigAndDBFile,
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

const defaultConfigContent = `
[pt_sites.mteam]
name = "mteam"
rss_url = "https://rss.m-team.cc/api/rss/xxxx"
download_dir = "./downloads/mteam"
interval_minutes = 5
enabled = true
[pt_sites.ptsite2]
name = "ptsite2"
rss_url = "https://rss.ptsite2.com/api/rss/yyyy"
download_dir = "./downloads/ptsite2"
interval_minutes = 5
enabled = false
`

// 创建默认配置文件
func createDefaultConfigFile(path string) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// 如果文件已存在，提示用户
		return fmt.Errorf("配置文件已存在: %s", path)
	}
	// 写入默认配置内容
	if err := os.WriteFile(path, []byte(defaultConfigContent), 0o644); err != nil {
		return fmt.Errorf("无法创建默认配置文件: %v", err)
	}
	return nil
}

// 检查并初始化配置目录
func checkAndInitConfigDir(dir, configFileName string, force bool) error {
	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 创建目录
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建配置目录: %v", err)
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
	// 生成配置文件路径
	configFilePath := filepath.Join(dir, configFileName)
	// 如果配置文件已存在，处理逻辑
	if _, err := os.Stat(configFilePath); !os.IsNotExist(err) {
		if force {
			color.Yellow("配置文件已存在，强制覆盖: %s", configFilePath)
		} else {
			return nil
		}
	}
	// 创建默认配置文件
	if err := createDefaultConfigFile(configFilePath); err != nil {
		return err
	}
	color.Green("创建默认配置文件: %s", configFilePath)
	return nil
}

func initConfigAndDBFile(cmd *cobra.Command, args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		color.Red("无法获取用户主目录: %v", err)
		os.Exit(1)
	}
	if err := checkAndInitConfigDir(filepath.Join(home, defaultWorkdir), configName, false); err != nil {
		color.Red("初始化配置失败: %v", err)
		os.Exit(1)
	}
	color.Green("配置初始化成功！")
}
