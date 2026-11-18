package cmd

import (
	"github.com/sunerpy/pt-tools/global"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// renewedCookieStore 是存续期 Cookie 的地方（core.ConfigStore）。
type renewedCookieStore interface {
	SaveRenewedSiteCookie(site string, chain []string, renewed string) (bool, error)
}

// renewedCookieSink 把会续期的站点（BYR）换发的 Cookie 存回站点设置，存没存上都记一笔日志（不写 Cookie 本身）。
func renewedCookieSink(store renewedCookieStore) v2.SessionCookieSink {
	return func(siteID string, chain []string, renewed string) (bool, error) {
		stored, err := store.SaveRenewedSiteCookie(siteID, chain, renewed)
		if logger := global.GetSloggerSafe(); logger != nil {
			switch {
			case err != nil:
				logger.Warnf("[Site] 站点 %s 续期换发的 Cookie 没存上（这个进程里接着用新的）: %v", siteID, err)
			case stored:
				logger.Infof("[Site] 站点 %s 续期换发了 Cookie，已存回站点设置", siteID)
			default:
				logger.Infof("[Site] 站点 %s 设置里存的不是这个会话的 Cookie（扩展推来了新的，或者是验证候选 Cookie 的会话），续期换发的不存", siteID)
			}
		}
		return stored, err
	}
}
