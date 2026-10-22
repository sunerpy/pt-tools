package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// MissingCredential 按认证方式返回站点还缺的那项凭据：cookie、api_key 或 passkey；齐了返回空串。
// cookie_and_api_key 两样都缺时先报 api_key（浏览器扩展同步的是 Cookie，缺的多半是 API Key）。
func MissingCredential(authMethod, cookie, apiKey, passkey string) string {
	has := func(v string) bool { return strings.TrimSpace(v) != "" }
	switch strings.ToLower(strings.TrimSpace(authMethod)) {
	case "cookie":
		if !has(cookie) {
			return "cookie"
		}
	case "api_key":
		if !has(apiKey) {
			return "api_key"
		}
	case "cookie_and_api_key":
		if !has(apiKey) {
			return "api_key"
		}
		if !has(cookie) {
			return "cookie"
		}
	case "passkey":
		if !has(passkey) {
			return "passkey"
		}
	}
	return ""
}

// SaveSiteCredentials 只写传进来的凭据列（Cookie 加密后存），不改启用状态、地址和 RSS。
// 给凭据还没配齐、站点暂时不能启用的情况用：比如 HDDolby 要 Cookie 和 API Key，扩展先同步来了 Cookie。
func (s *ConfigStore) SaveSiteCredentials(site models.SiteGroup, cookie, apiKey, passkey *string) error {
	cols := map[string]any{}
	if cookie != nil {
		cols["cookie"] = ""
		cols["cookie_encrypted"] = ""
		if strings.TrimSpace(*cookie) != "" {
			cipherText, err := s.EncryptCookie(*cookie)
			if err != nil {
				return err
			}
			cols["cookie_encrypted"] = cipherText
		}
	}
	if apiKey != nil {
		cols["api_key"] = *apiKey
	}
	if passkey != nil {
		cols["passkey"] = *passkey
	}
	if len(cols) == 0 {
		return nil
	}
	res := s.db.DB.Model(&models.SiteSetting{}).Where("name = ?", string(site)).Updates(cols)
	if res.Error != nil {
		return fmt.Errorf("保存站点凭据失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("站点不存在: %s", site)
	}
	events.Publish(events.Event{Type: events.ConfigChanged, Version: time.Now().UnixNano(), Source: "sites", At: time.Now()})
	return nil
}
