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
	Short: "Initialize a new configuration file",
	Long: `The 'init' command generates a default configuration file
and sets up the necessary database file if they do not already exist.
This command ensures the application has the required settings to run.`,
	Example: `  pt-tools config init
  pt-tools config init --config /path/to/config.toml`,
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
[global]
default_interval = "5m"       # 默认的任务间隔时间（单位：分钟），例如 "5m" 表示每 5 分钟运行一次任务。
default_enabled = true        # 默认是否启用站点任务，true 为启用，false 为禁用。
download_dir = "downloads"    # 默认的种子下载目录。
download_limit_enabled = true # 是否启用下载限速，true 为启用，false 为禁用。
download_speed_limit = 20     # 下载速度限制（单位：MB/s）。
torrent_size_gb = 500         # 默认的下载种子大小（单位：GB）。
[qbit]
enabled = true                  # 是否启用 qBittorrent 客户端，true 为启用，false 为禁用。
url = "http://xxx.xxx.xxx:8080" # qBittorrent Web UI 的 URL 地址。
user = "admin"                  # qBittorrent 的登录用户名。
password = "adminadmin"         # qBittorrent 的登录密码。
[sites]
    [sites.mteam] # 定义 MTeam 站点的配置信息。
    enabled = false                        # 是否启用 MTeam 站点任务，true 为启用，false 为禁用。
    auth_method = "api_key"                # 认证方式，MT站点支持 "api_key"。
    api_key = "xxx"                        # 如果使用 API 认证，此处填写 API 密钥。
    api_url = "https://api.m-team.xxx/api" # API 地址。
        [[sites.mteam.rss]] # 定义 MTeam 站点的 RSS 订阅信息。
        name = "TMP2"                              # RSS 订阅的名称。
        url = "https://rss.m-team.xxx/api/rss/xxx" # RSS 订阅链接。
        category = "Tv"                            # RSS 订阅分类。
        tag = "MT"                                 # 为任务添加的标记（用于区分）。
        interval_minutes = 10                      # RSS 任务执行间隔时间（单位：分钟）。
        download_sub_path = "mteam/tvs"            # 下载的种子存储的子目录。
    [sites.hdsky] # 定义 HDSky 站点的配置信息。
    enabled = true         # 是否启用 HDSky 站点任务，true 为启用，false 为禁用。
    auth_method = "cookie" # 认证方式，支持 "api_key" 和 "cookie"。
    cookie = "xxx"         # 如果使用 Cookie 认证，此处填写 Cookie。
        [[sites.hdsky.rss]] # 定义 HDSky 站点的 RSS 订阅信息。
        name = "HDSky"                               # RSS 订阅的名称。
        url = "https://hdsky.xxx/torrentrss.php?xxx" # RSS 订阅链接。
        category = "Mv"                              # RSS 订阅分类。
        tag = "HDSKY"                                # 为任务添加的标记（用于区分）。
        interval_minutes = 5                         # RSS 任务执行间隔时间（单位：分钟）。
        download_sub_path = "hdsky/"                 # 下载的种子存储的子目录。
    [sites.cmct] # 定义 CMCT 站点的配置信息。
    enabled = true         # 是否启用 CMCT 站点任务，true 为启用，false 为禁用。
    auth_method = "cookie" # 认证方式，"cookie"。
    cookie = "xxx"         # 如果使用 Cookie 认证，此处填写 Cookie。
        [[sites.cmct.rss]] # 定义 CMCT 站点的 RSS 订阅信息。
        name = "CMCT"                 # RSS 订阅的名称。
        url = "https://springxxx.xxx" # RSS 订阅链接。
        category = "Tv"               # RSS 订阅分类。
        tag = "CMCT"                  # 为任务添加的标记（用于区分）。
        interval_minutes = 5          # RSS 任务执行间隔时间（单位：分钟）。
        download_sub_path = "cmct/"   # 下载的种子存储的子目录。
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

func chekcAndInitDownloadPath(dir string) error {
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
	return nil
}

// 检查并初始化配置目录
func checkAndInitConfigDir(dir, configFileName string, force bool) error {
	// 检查目录是否存在
	if err := chekcAndInitDownloadPath(dir); err != nil {
		return err
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
