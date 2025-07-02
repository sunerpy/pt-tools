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
	"github.com/spf13/viper"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
)

const (
	configDir  = ".pt-tools"
	configName = "config.toml"
	dbFile     = "torrents.db"
)

var (
	cfgFile string
	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "pt-tools",
		Short: "pt-tools: A CLI tool for managing and automating PT site tasks",
		Long: `pt-tools is a powerful and flexible command-line tool designed for managing tasks related to private tracker (PT) sites.
It supports running in single execution or continuous monitoring modes, database management, and configuration customization.`,
		Example: `  # Run in single execution mode
  pt-tools run --mode=single
  # Run in persistent mode
  pt-tools run --mode=persistent
  # Generate shell completion for Bash
  pt-tools completion bash
  # Generate shell completion for Zsh
  pt-tools completion zsh
  # Initialize a configuration file
  pt-tools config init
  # Manage database operations
  pt-tools db --help`,
		PreRun: PersistentCheckCfg,
	}
)

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
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.pt-tools/config.toml)")
	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	// if err := initTools(); err != nil {
	// 	color.Red("Failed to load configuration: %s\n",cfgFile)
	// 	panic(err)
	// }
}

func initTools() error {
	if global.GlobalViper == nil {
		global.GlobalViper = viper.New()
	}
	// 尝试加载配置文件
	logger, err := core.InitViper(cfgFile)
	if err != nil {
		color.Red("Failed to load configuration\n")
		return err
	}
	global.InitLogger(logger)
	return nil
}
