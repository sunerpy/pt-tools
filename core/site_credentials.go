package core

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

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

// SaveRenewedSiteCookie 存站点续期换发的 Cookie（BYR，见 site/v2 的 SessionCookies）：chain 是这个会话用过的那些 Cookie，
// 站点设置里存着的是其中一个时换成 renewed；已经是别的（扩展推来了新的、用户改了、验证时的候选会话）就不动、返回 false。
// 只写 Cookie 密文、不发 ConfigChanged：在用的站点实例已经换上新值，不用重建实例、重载调度；重启以后从这里接着用。
func (s *ConfigStore) SaveRenewedSiteCookie(site string, chain []string, renewed string) (bool, error) {
	if strings.TrimSpace(renewed) == "" {
		return false, nil
	}
	stored := false
	err := s.db.DB.Transaction(func(tx *gorm.DB) error {
		var row models.SiteSetting
		if err := tx.Where("name = ?", site).First(&row).Error; err != nil {
			return fmt.Errorf("读取站点 %s 失败: %w", site, err)
		}
		current, err := s.cookiePlaintextForSite(row)
		if err != nil {
			return err
		}
		if !slices.Contains(chain, current) {
			return nil
		}
		cipherText, err := s.EncryptCookie(renewed)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.SiteSetting{}).Where("id = ?", row.ID).
			Updates(map[string]any{"cookie_encrypted": cipherText, "cookie": ""}).Error; err != nil {
			return fmt.Errorf("保存站点 %s 续期的 Cookie 失败: %w", site, err)
		}
		stored = true
		return nil
	})
	return stored, err
}
