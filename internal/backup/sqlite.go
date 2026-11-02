package backup

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// openReadOnly 只读打开一个数据库文件。immutable=1 表示不加锁、不建 -wal 与 -shm：
// 校验的是快照、解开的临时文件，或者启动时刚换上、还没人用的数据库，旁边不能留下多余的文件。
func openReadOnly(path string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(sqlite.Open(sqliteURI(path, "mode=ro&immutable=1")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, func() {}, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, func() {}, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, func() { _ = sqlDB.Close() }, nil
}

// sqliteURI 把文件路径写成 SQLite 的 URI 文件名（file:///绝对路径?参数），路径里的空格等字符按 URI 转义；
// Windows 的 C:\x 写成 file:///C:/x。
func sqliteURI(path, query string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p, RawQuery: query}).String()
}

// snapshotSchemaVersion 读快照里的数据库版本（schema_versions 里最大的版本号；没有这张表时是 0）。
func snapshotSchemaVersion(path string) (int, error) {
	db, done, err := openReadOnly(path)
	if err != nil {
		return 0, err
	}
	defer done()
	return schemaVersion(db)
}

func schemaVersion(db *gorm.DB) (int, error) {
	ok, err := models.TableExists(db, "schema_versions")
	if err != nil || !ok {
		return 0, err
	}
	var v *int
	if err := db.Raw("SELECT MAX(version) FROM schema_versions").Scan(&v).Error; err != nil {
		return 0, fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if v == nil {
		return 0, nil
	}
	return *v, nil
}

// integrityCheck 跑 PRAGMA integrity_check，结果不是 ok 时报错。
func integrityCheck(db *gorm.DB) error {
	var rows []string
	if err := db.Raw("PRAGMA integrity_check").Scan(&rows).Error; err != nil {
		return fmt.Errorf("%w：数据库检查失败：%v", ErrInvalid, err)
	}
	if len(rows) != 1 || rows[0] != "ok" {
		detail := strings.Join(rows, "；")
		if len(detail) > 200 {
			detail = detail[:200] + "…"
		}
		return fmt.Errorf("%w：数据库损坏：%s", ErrInvalid, detail)
	}
	return nil
}

// cipherCheck 用 decrypt 解库里每个加密列的密文：每一列取最多 3 条非空的值，有一条解得开就算这一列通过
// （密钥不对时一条也解不开；只有个别值坏了不至于整份备份不能用）。库里一条密文都没有时 noCipher 为真。
func cipherCheck(db *gorm.DB, decrypt func(string) ([]byte, error)) (noCipher bool, err error) {
	noCipher = true
	for _, c := range models.EncryptedColumns {
		ok, err := models.TableExists(db, c.Table)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		var values []string
		if err := db.Table(c.Table).Where(c.Column+" IS NOT NULL AND "+c.Column+" != ''").Limit(3).Pluck(c.Column, &values).Error; err != nil {
			return false, fmt.Errorf("读取 %s.%s 失败: %w", c.Table, c.Column, err)
		}
		if len(values) == 0 {
			continue
		}
		noCipher = false
		opened := false
		for _, v := range values {
			if _, derr := decrypt(v); derr == nil {
				opened = true
				break
			}
		}
		if !opened {
			return false, fmt.Errorf("%w：%s", errCannotDecrypt, columnLabel(c.Table))
		}
	}
	return noCipher, nil
}

var errCannotDecrypt = errors.New("解不开")

func columnLabel(table string) string {
	switch table {
	case "site_settings":
		return "站点 Cookie"
	case "notification_conf":
		return "通知通道的凭证"
	case "cloak_settings":
		return "CloakBrowser 令牌"
	}
	return table
}

// withKey 把密钥做成 cipherCheck 用的解密函数。
func withKey(key []byte) func(string) ([]byte, error) {
	return func(c string) ([]byte, error) { return crypto.DecryptWithKey(key, c) }
}
