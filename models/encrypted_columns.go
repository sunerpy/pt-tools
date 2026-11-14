package models

// EncryptedColumn 是一个以 AES-GCM 密文保存的列。
type EncryptedColumn struct {
	Table, Column string
}

// EncryptedColumns 列出以 AES-GCM 密文保存的列（都用 secret.key 这一把密钥）：非空即说明库里有用某个密钥
// 加密过的数据。启动检查（core 的 checkGeneratedSecretKey）与备份校验（internal/backup）用同一份清单；
// 以 Encrypted 结尾的字段都要列在这里（TestEncryptedColumnsCoverEveryEncryptedField 会检查）。
var EncryptedColumns = []EncryptedColumn{
	{"site_settings", "cookie_encrypted"},
	{"notification_conf", "config_json"},
	{"cloak_settings", "token_encrypted"},
	// ChineseSubFinder 的 API Key
	{"media_libraries", "csf_key_encrypted"},
	// NAS 监控来源的密钥与刷新出的凭据
	{"nas_sources", "secret_encrypted"},
	{"nas_sources", "extra_encrypted"},
	// AI 助手的 API Key、代理与语音输入的令牌
	{"assistant_settings", "api_key_encrypted"},
	{"assistant_settings", "proxy_encrypted"},
	{"assistant_settings", "voice_token_encrypted"},
	// 媒体识别与刮削：TMDB Key、代理、Fanart.tv 的项目 Key 与个人 Key、OMDb Key、TheTVDB 的 Key 与订阅 PIN
	{"media_settings", "tmdb_key_encrypted"},
	{"media_settings", "proxy_encrypted"},
	{"media_settings", "fanart_key_encrypted"},
	{"media_settings", "fanart_client_key_encrypted"},
	{"media_settings", "omdb_key_encrypted"},
	{"media_settings", "tvdb_key_encrypted"},
	{"media_settings", "tvdb_pin_encrypted"},
	// 媒体服务器（Jellyfin、Emby、Plex）的令牌
	{"media_servers", "token_encrypted"},
	// IYUU 令牌
	{"reseed_settings", "token_encrypted"},
	// CookieCloud 的密码
	{"cookiecloud_settings", "password_encrypted"},
	// 站点代理
	{"site_proxy_settings", "proxy_encrypted"},
	// 远程访问的主机密钥
	{"remote_settings", "host_keys_encrypted"},
}
