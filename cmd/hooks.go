package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/sunerpy/pt-tools/global"
)

func PersistentCheckCfg(cmd *cobra.Command, args []string) {
	if err := initTools(); err != nil {
		color.Red("Load Config Error: %v", err)
		color.Yellow("Try to run 'pt-tools config init' to initialize the configuration file.")
		cmd.Usage()
		os.Exit(1)
	}
	configFilePath := global.GlobalViper.ConfigFileUsed()
	if configFilePath == "" {
		color.Red("Error: Configuration file not found.")
		fmt.Println("Please run 'pt-tools config init' to generate a default configuration file.")
		os.Exit(1)
	}
	// 检查数据库文件是否存在
	dbFile := filepath.Join(global.GlobalDirCfg.WorkDir, dbFile)
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		color.Red("Error: Database file not found.")
		fmt.Println("Please run 'pt-tools config init' to initialize the database.")
		os.Exit(1)
	}
	// 检查downloadDir
	home, err := os.UserHomeDir()
	if err != nil {
		color.Red("无法获取用户主目录: %v", err)
		os.Exit(1)
	}
	chekcAndInitDownloadPath(filepath.Join(home, defaultWorkdir))
	if _, err := os.Stat(global.GlobalDirCfg.DownloadDir); os.IsNotExist(err) {
		color.Red("Error: Download directory not found.")
		fmt.Println("Please run 'pt-tools config init' to initialize the database.")
		os.Exit(1)
	}
}
