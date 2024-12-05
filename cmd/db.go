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

	"github.com/spf13/cobra"
)

// dbCmd represents the db command
var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Manage database operations",
	Long: `The 'db' command allows you to manage database-related operations
such as initializing the database or creating backups. Use the available
subcommands to perform specific actions.`,
	Example: `  pt-tools db init
  pt-tools db backup --output /path/to/backup.sql`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Please specify a subcommand. Use 'pt-tools db --help' for more information.")
		cmd.Usage()
	},
	PersistentPreRun: PersistentCheckCfg, // 确保在执行所有子命令前进行配置检查
}

func init() {
	rootCmd.AddCommand(dbCmd)
	// Here you will define your flags and configuration settings.
	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dbCmd.PersistentFlags().String("foo", "", "A help for foo")
	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dbCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
