package core

import (
	"errors"
	"fmt"
	"os"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
)

// acceptNewSecretKeyEnv 设为 1 表示确认原密钥已经丢失，接受新生成的密钥并重新填写凭证。
const acceptNewSecretKeyEnv = "PT_TOOLS_ACCEPT_NEW_SECRET_KEY"

var (
	// ErrSecretKeyUnusable 表示 secret.key 存在却无法使用（读不出或格式不对）。
	ErrSecretKeyUnusable = errors.New("secret.key 无法使用")
	// ErrSecretKeyRegenerated 表示 secret.key 是本次新生成的，但数据库里已有用原密钥加密的数据。
	ErrSecretKeyRegenerated = errors.New("secret.key 是新生成的，数据库里却已有加密数据")
)

// encryptedColumns 列出以 AES-GCM 密文保存的列，非空即说明库里有用某个密钥加密过的数据。
var encryptedColumns = []struct{ table, column string }{
	{"site_settings", "cookie_encrypted"},
	{"notification_conf", "config_json"},
	{"cloak_settings", "token_encrypted"},
}

// checkSecretKeyFile 在打开数据库之前检查密钥文件：文件存在却无法使用时拒绝启动，不自动换新密钥。
func checkSecretKeyFile() error {
	_, _, loadErr := crypto.KeyStatus()
	if loadErr == nil {
		return nil
	}
	return fmt.Errorf("%w：%v。为避免覆盖原密钥，没有自动生成新密钥。请恢复正确的 secret.key，"+
		"或用 pt-tools secret import 导入备份的密钥后重启", ErrSecretKeyUnusable, loadErr)
}

// checkGeneratedSecretKey 在打开数据库之后检查：本次新生成了密钥、库里却已有加密数据，
// 说明原来的 secret.key 丢了或没挂载进来。新密钥解不开这些数据，继续运行只会让人以为凭证丢了，
// 所以删掉刚生成的密钥并拒绝启动，除非用户明确确认原密钥已经丢失。
func checkGeneratedSecretKey(db *gorm.DB) error {
	path, generated, _ := crypto.KeyStatus()
	if !generated || db == nil {
		return nil
	}
	found, err := hasEncryptedData(db)
	if err != nil {
		return fmt.Errorf("检查已加密数据失败: %w", err)
	}
	if !found {
		return nil
	}
	if os.Getenv(acceptNewSecretKeyEnv) == "1" {
		if logger := global.GetSloggerSafe(); logger != nil {
			logger.Warnf("已按 %s=1 启用新生成的 %s：原来加密保存的站点 Cookie 和通知凭证无法解密，需要重新填写",
				acceptNewSecretKeyEnv, path)
		}
		return nil
	}
	if derr := crypto.DiscardGeneratedKey(); derr != nil {
		if logger := global.GetSloggerSafe(); logger != nil {
			logger.Warnf("删除新生成的 %s 失败: %v", path, derr)
		}
	}
	return fmt.Errorf("%w：%s 原本不存在，新生成的密钥解不开已保存的站点 Cookie 和通知凭证。"+
		"请把原来的 secret.key 放回 %s（或设置 PT_TOOLS_SECRET_KEY）后重启；"+
		"确认原密钥已经丢失、愿意重新填写这些凭证时，设置 %s=1 启动一次",
		ErrSecretKeyRegenerated, path, path, acceptNewSecretKeyEnv)
}

func hasEncryptedData(db *gorm.DB) (bool, error) {
	for _, c := range encryptedColumns {
		if !db.Migrator().HasTable(c.table) {
			continue
		}
		var n int64
		if err := db.Table(c.table).Where(c.column + " IS NOT NULL AND " + c.column + " != ''").Count(&n).Error; err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}
