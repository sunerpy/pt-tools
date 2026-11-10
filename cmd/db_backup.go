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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/utils"
)

// 备份与恢复两个命令不走 db 的 PersistentPreRun（InitRuntime）：服务在运行时也要能用，不能去迁移它正在用的数据库；
// 恢复还要在原来的 secret.key 丢了时也能用（InitRuntime 的启动检查会拒绝）。
func noRuntimeInit(*cobra.Command, []string) {}

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "备份数据库与密钥",
	Long: `把数据库（一致的快照，服务在运行时也能做）和 secret.key 打成一个 tar.gz。

不带 --output 时放进「全局设置 → 备份」里的目录（默认 ~/.pt-tools/backups）。
带不带密钥默认按备份设置，--no-secret 只备份数据库。带上密钥的备份要和数据库一样妥善保管：
拿到它就能解开保存的站点 Cookie、通知凭证和 API Key。恢复用 pt-tools db restore。`,
	Example: `  pt-tools db backup
  pt-tools db backup --output /mnt/backup/
  pt-tools db backup --output /mnt/backup/pt-tools.tar.gz --no-secret`,
	PersistentPreRun: noRuntimeInit,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		noSecret, _ := cmd.Flags().GetBool("no-secret")
		return runDBBackup(cmd.Context(), cmd.OutOrStdout(), output, noSecret)
	},
}

// restoreCmd 校验一份备份并放进待恢复，下次启动 Web 服务时生效。
var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "从备份恢复（重启以后生效）",
	Long: `校验一份备份（pt-tools db backup 或网页上做的），通过后放进 ~/.pt-tools/restore-pending/，
下次启动 pt-tools 的 Web 服务时换上：原来的数据库（要换密钥时还有 secret.key）挪进
~/.pt-tools/backups/pre-restore-<时间>/，换上以后用当前生效的密钥复验，任何一步失败都放回原来的文件、按原来的数据启动。

这个命令不改现在的数据库，服务在运行时也能用；重启以前可以用 --cancel 取消。
备份没带密钥时，要求现在的密钥解得开它；做备份时的密钥可以先用 pt-tools secret import 导入。`,
	Example: `  pt-tools db restore --input /mnt/backup/pt-tools-backup-20261102-043000-scheduled.tar.gz
  pt-tools db restore --cancel`,
	PersistentPreRun: noRuntimeInit,
	RunE: func(cmd *cobra.Command, args []string) error {
		input, _ := cmd.Flags().GetString("input")
		cancel, _ := cmd.Flags().GetBool("cancel")
		return runDBRestore(cmd.OutOrStdout(), input, cancel)
	},
}

func init() {
	dbCmd.AddCommand(backupCmd, restoreCmd)
	backupCmd.Flags().StringP("output", "o", "", "备份文件或目录（默认是备份设置里的目录）")
	backupCmd.Flags().Bool("no-secret", false, "不带上密钥，只备份数据库")
	restoreCmd.Flags().StringP("input", "i", "", "要恢复的备份文件（.tar.gz）")
	restoreCmd.Flags().Bool("cancel", false, "取消尚未生效的恢复")
	restoreCmd.MarkFlagsMutuallyExclusive("input", "cancel")
}

func runDBBackup(ctx context.Context, out io.Writer, output string, noSecret bool) error {
	defer discardGeneratedKey()
	if ctx == nil {
		ctx = context.Background()
	}
	var include *bool
	if noSecret {
		include = new(bool)
	}
	info, err := core.BackupDataDir(ctx, output, include)
	if err != nil {
		return fmt.Errorf("备份失败: %w", err)
	}
	fmt.Fprintf(out, "已备份到 %s（%s）\n", info.Path, utils.FormatBytes(info.Size))
	if info.Manifest.HasSecret {
		fmt.Fprintln(out, "这份备份带了密钥：拿到它就能解开保存的站点 Cookie、通知凭证和 API Key，请妥善保管。")
	} else {
		fmt.Fprintln(out, "这份备份没带密钥：恢复时要有做备份时的 secret.key（或 PT_TOOLS_SECRET_KEY）。")
	}
	return nil
}

func runDBRestore(out io.Writer, input string, cancel bool) error {
	defer discardGeneratedKey()
	if cancel {
		p, err := core.PendingRestore()
		if err != nil {
			return err
		}
		if p == nil {
			fmt.Fprintln(out, "没有待恢复的备份")
			return nil
		}
		if err := core.CancelRestore(); err != nil {
			return err
		}
		fmt.Fprintf(out, "已取消恢复 %s\n", p.Name)
		return nil
	}
	if input == "" {
		return errors.New("用 --input 指定要恢复的备份文件")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	if st, statErr := os.Stat(abs); statErr != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("找不到备份文件 %s", input)
	}
	p, err := core.StageRestore(abs, filepath.Base(abs))
	if err != nil {
		return fmt.Errorf("不能恢复: %w", err)
	}
	m := p.Manifest
	fmt.Fprintf(out, "备份校验通过：%s\n", p.Name)
	fmt.Fprintf(out, "  做备份的时间：%s（%s）\n", m.CreatedAt.Local().Format("2006-01-02 15:04:05"), m.Reason.Label())
	fmt.Fprintf(out, "  程序版本：%s，数据库版本：%d\n", appVersionText(m.AppVersion), m.SchemaVersion)
	fmt.Fprintf(out, "  密钥：%s\n", keyActionText(p))
	pre := "数据目录下的 " + backup.DefaultDirName + "/pre-restore-<时间>/"
	if data, derr := core.DataDir(); derr == nil {
		pre = filepath.Join(data, backup.DefaultDirName, "pre-restore-<时间>") + "/"
	}
	fmt.Fprintf(out, "重启 pt-tools 以后生效；原来的数据库会挪进 %s。重启以前可以用 pt-tools db restore --cancel 取消。\n", pre)
	return nil
}

// appVersionText 是做备份的程序版本（给人看）；从不记版本号的旧版本升上来时的升级前备份写「未知」。
func appVersionText(v string) string {
	if v == "" || v == backup.UnknownAppVersion {
		return "未知"
	}
	return v
}

// keyActionText 是恢复以后用哪把密钥（给人看）。
func keyActionText(p backup.Pending) string {
	switch {
	case p.Key == backup.KeyKeepEnv:
		return "仍用环境变量 PT_TOOLS_SECRET_KEY 的密钥"
	case p.Key == backup.KeyUseBackup:
		return "secret.key 换成备份里的密钥（原来的一起挪进 pre-restore 目录）"
	case p.NoCipher:
		return "备份里没有加密保存的凭证，不需要密钥；secret.key 不变"
	default:
		return "secret.key 不变（现在的密钥解得开这份备份）"
	}
}

// discardGeneratedKey 删掉这个命令启动时新生成的 secret.key：命令行进程启动时找不到 secret.key 会生成一把
// （internal/crypto 的 init），留着它，下次启动的服务会把它当成原来的密钥，而备份与恢复命令不走 InitRuntime 的启动检查。
func discardGeneratedKey() {
	if _, generated, _ := crypto.KeyStatus(); !generated {
		return
	}
	if err := crypto.DiscardGeneratedKey(); err != nil {
		fmt.Fprintf(os.Stderr, "删除这次新生成的 secret.key 失败: %v\n", err)
	}
}
