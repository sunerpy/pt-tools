package cmd

import (
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
)

func PersistentCheckCfg(cmd *cobra.Command, args []string) {
	if err := initTools(); err != nil {
		color.Red("初始化失败: %v", err)
		_ = cmd.Usage()
		os.Exit(1)
	}
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	if strings.TrimSpace(gl.DownloadDir) == "" {
		color.Yellow("未检测到全局配置，请先通过 Web 完成初始化")
		os.Exit(1)
	}
}
