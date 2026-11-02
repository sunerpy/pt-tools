package models

// EncryptedColumn 是一个以 AES-GCM 密文保存的列。
type EncryptedColumn struct {
	Table, Column string
}

// EncryptedColumns 列出以 AES-GCM 密文保存的列（站点 Cookie、通知配置、CloakBrowser 令牌）：非空即说明库里有用某个密钥
// 加密过的数据。启动检查（core 的 checkGeneratedSecretKey）与备份校验（internal/backup）用同一份清单。
var EncryptedColumns = []EncryptedColumn{
	{"site_settings", "cookie_encrypted"},
	{"notification_conf", "config_json"},
	{"cloak_settings", "token_encrypted"},
}
