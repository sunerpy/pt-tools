package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/internal/crypto"
)

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "管理 AES 密钥（备份与恢复）",
	Long:  `pt-tools secret 命令用于导出和导入 AES 加密密钥，支持密钥备份和恢复。`,
}

var secretExportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出加密密钥",
	Long:  `导出 AES-256 密钥为 base64 编码，输出到标准输出。请妥善保管输出内容。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSecretExport()
	},
}

var secretImportCmd = &cobra.Command{
	Use:   "import",
	Short: "导入加密密钥",
	Long:  `从标准输入读取 base64 编码的密钥，验证长度（32 字节），原子性写入 ~/.pt-tools/secret.key。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return runSecretImport(force)
	},
}

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretExportCmd, secretImportCmd)
	secretImportCmd.Flags().Bool("force", false, "覆盖现有密钥文件")
}

func runSecretExport() error {
	key, err := crypto.ExportKey()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 读取密钥失败: %v\n", err)
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(key)
	fmt.Println(encoded)
	fmt.Fprintf(os.Stderr, "⚠ output contains AES key — keep secret\n")

	return nil
}

func runSecretImport(force bool) error {
	buf := make([]byte, 4096)
	n, err := os.Stdin.Read(buf)
	if err != nil && err.Error() != "EOF" {
		fmt.Fprintf(os.Stderr, "错误: 读取标准输入失败: %v\n", err)
		return err
	}

	b64 := string(buf[:n])
	for len(b64) > 0 && (b64[len(b64)-1] == '\n' || b64[len(b64)-1] == '\r' || b64[len(b64)-1] == ' ' || b64[len(b64)-1] == '\t') {
		b64 = b64[:len(b64)-1]
	}

	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: base64 解码失败: %v\n", err)
		return err
	}

	if len(key) != 32 {
		fmt.Fprintf(os.Stderr, "错误: 密钥长度无效，期望 32 字节，实际 %d 字节\n", len(key))
		return fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(key))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 获取主目录失败: %v\n", err)
		return err
	}

	keyDir := filepath.Join(home, ".pt-tools")
	keyPath := filepath.Join(keyDir, "secret.key")

	if _, err := os.Stat(keyPath); err == nil && !force {
		fmt.Fprintf(os.Stderr, "~/.pt-tools/secret.key already exists; use --force to overwrite\n")
		return fmt.Errorf("key file exists")
	}

	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 创建目录失败: %v\n", err)
		return err
	}

	tmpPath := filepath.Join(keyDir, ".secret.key.tmp."+fmt.Sprintf("%d", os.Getpid()))
	if err := os.WriteFile(tmpPath, key, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 写入临时文件失败: %v\n", err)
		return err
	}

	if err := os.Rename(tmpPath, keyPath); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 原子写入失败: %v\n", err)
		os.Remove(tmpPath)
		return err
	}

	fmt.Fprintf(os.Stderr, "✓ 密钥已成功导入到 %s\n", keyPath)
	return nil
}
