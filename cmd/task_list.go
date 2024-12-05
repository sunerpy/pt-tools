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
	"time"

	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Display daily task details (not implemented)",
	Long: `The 'list' subcommand displays detailed information about the tasks processed
today, including pushed torrents, skipped torrents, and other task-related statistics.`,
	Example: `  pt-tools task list
  pt-tools task list --date 2024-12-05`,
	Run: func(cmd *cobra.Command, args []string) {
		date, _ := cmd.Flags().GetString("date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		fmt.Printf("Fetching task details for date: %s...\n", date)
		// 模拟展示任务信息
		pushedTorrents := []string{"Torrent1", "Torrent2"}
		skippedTorrents := []string{"Torrent3", "Torrent4"}
		fmt.Println("Pushed Torrents:")
		for _, torrent := range pushedTorrents {
			fmt.Printf("  - %s\n", torrent)
		}
		fmt.Println("Skipped Torrents:")
		for _, torrent := range skippedTorrents {
			fmt.Printf("  - %s\n", torrent)
		}
	},
}

func init() {
	taskCmd.AddCommand(listCmd)
	// Here you will define your flags and configuration settings.
	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// listCmd.PersistentFlags().String("foo", "", "A help for foo")
	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// listCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
