package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// 站点代理（全局设置）：哪些站点的请求经哪个代理发出。设置存在 site_proxy_settings 那一行，代理地址加密存；
// 保存成功以后马上换上判定用的快照（httpclient.SetSiteProxyPolicy），下一个站点请求就按新设置走，不用重启。

// SiteProxySettings 是站点代理的设置。ProxyURL 是明文（可能带密码），只在进程里用；给界面看要先 httpclient.MaskProxyURL。
type SiteProxySettings struct {
	Enabled  bool
	ProxyURL string
	Mode     string
	// Sites 是选中的站点 ID（规范化后的小写 ID，排好序）
	Sites []string
	// ProxyUnreadable 是存着的代理地址解不开（加密密钥换过）：当作没填，要重新填
	ProxyUnreadable bool
	// UpdatedAt 是上次保存的时间，没保存过时为 nil
	UpdatedAt *time.Time
}

// SiteProxyInput 是保存站点代理的参数。ProxyURL 与读到的（密码换成 *** 的）地址相同表示不改，空串表示清除。
type SiteProxyInput struct {
	Enabled  bool
	ProxyURL string
	Mode     string
	Sites    []string
}

// ErrSiteProxyInvalid 是站点代理设置不对（地址、模式或站点）；错误文本里有给人看的原因，不带代理地址。
var ErrSiteProxyInvalid = errors.New("站点代理设置有误")

// siteProxyMu 让保存与装载串行：判定用的快照总是最后一次保存的内容。
var siteProxyMu sync.Mutex

// SiteProxy 读站点代理设置；没有这一行时返回默认值（关闭、全部站点、没有选站点）。
func (s *ConfigStore) SiteProxy() (SiteProxySettings, error) {
	out := SiteProxySettings{Mode: httpclient.SiteProxyAll, Sites: []string{}}
	var rows []models.SiteProxySetting
	if err := s.db.DB.Where("id = 1").Limit(1).Find(&rows).Error; err != nil {
		return out, fmt.Errorf("读取站点代理设置失败: %w", err)
	}
	if len(rows) == 0 {
		return out, nil
	}
	row := rows[0]
	out.Enabled = row.Enabled
	if validSiteProxyMode(row.Mode) {
		out.Mode = row.Mode
	}
	out.Sites = parseSiteProxySites(row.SitesJSON)
	updated := row.UpdatedAt
	out.UpdatedAt = &updated
	if row.ProxyEncrypted != "" {
		plain, err := s.DecryptCookie(row.ProxyEncrypted)
		if err != nil {
			out.ProxyUnreadable = true
		} else {
			out.ProxyURL = plain
		}
	}
	return out, nil
}

// SaveSiteProxy 校验并保存站点代理设置，成功以后换上新的判定快照。站点 ID 认不认识由调用方先查（它有站点注册表）。
func (s *ConfigStore) SaveSiteProxy(in SiteProxyInput) (SiteProxySettings, error) {
	siteProxyMu.Lock()
	defer siteProxyMu.Unlock()
	stored, err := s.SiteProxy()
	if err != nil {
		return SiteProxySettings{}, err
	}
	mode := strings.TrimSpace(in.Mode)
	if mode == "" {
		mode = httpclient.SiteProxyAll
	}
	if !validSiteProxyMode(mode) {
		return SiteProxySettings{}, fmt.Errorf("%w：不认识的模式 %q", ErrSiteProxyInvalid, mode)
	}
	sites := normalizeSiteProxySites(in.Sites)
	proxy := strings.TrimSpace(in.ProxyURL)
	switch {
	case proxy == "":
	case stored.ProxyURL != "" && proxy == httpclient.MaskProxyURL(stored.ProxyURL):
		// 界面原样交回了密码换成 *** 的地址：不改
		proxy = stored.ProxyURL
	default:
		if _, parseErr := httpclient.ParseProxyURL(proxy); parseErr != nil {
			return SiteProxySettings{}, fmt.Errorf("%w：%s", ErrSiteProxyInvalid, parseErr.Error())
		}
	}
	if in.Enabled && proxy == "" {
		return SiteProxySettings{}, fmt.Errorf("%w：开启时要填代理地址", ErrSiteProxyInvalid)
	}
	if mode == httpclient.SiteProxyInclude && len(sites) == 0 {
		return SiteProxySettings{}, fmt.Errorf("%w：选了「只有选中的站点」，却没有选站点", ErrSiteProxyInvalid)
	}
	enc := ""
	if proxy != "" {
		if enc, err = s.EncryptCookie(proxy); err != nil {
			return SiteProxySettings{}, fmt.Errorf("加密代理地址失败: %w", err)
		}
	}
	raw, err := json.Marshal(sites)
	if err != nil {
		return SiteProxySettings{}, err
	}
	row := models.SiteProxySetting{ID: 1, Enabled: in.Enabled, ProxyEncrypted: enc, Mode: mode, SitesJSON: string(raw), UpdatedAt: time.Now()}
	if err := s.db.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "proxy_encrypted", "mode", "sites_json", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return SiteProxySettings{}, fmt.Errorf("保存站点代理设置失败: %w", err)
	}
	saved := SiteProxySettings{Enabled: in.Enabled, ProxyURL: proxy, Mode: mode, Sites: sites, UpdatedAt: &row.UpdatedAt}
	httpclient.SetSiteProxyPolicy(siteProxyPolicy(saved))
	return saved, nil
}

// LoadSiteProxyPolicy 读站点代理设置并换上判定快照（启动时调）。读不出、或者开着却解不开代理地址时，
// 按关闭处理（站点请求照旧按环境变量）并返回错误，由调用方记日志；错误里没有代理地址。
func (s *ConfigStore) LoadSiteProxyPolicy() error {
	siteProxyMu.Lock()
	defer siteProxyMu.Unlock()
	cfg, err := s.SiteProxy()
	if err != nil {
		httpclient.SetSiteProxyPolicy(nil)
		return err
	}
	if cfg.ProxyUnreadable {
		httpclient.SetSiteProxyPolicy(nil)
		if cfg.Enabled {
			return errors.New("站点代理的代理地址解不开（加密密钥换过？），站点请求先按环境变量走；到全局设置里重新填一次代理地址")
		}
		return nil
	}
	httpclient.SetSiteProxyPolicy(siteProxyPolicy(cfg))
	return nil
}

func siteProxyPolicy(cfg SiteProxySettings) *httpclient.SiteProxyPolicy {
	if !cfg.Enabled || cfg.ProxyURL == "" {
		return nil
	}
	sites := make(map[string]bool, len(cfg.Sites))
	for _, id := range cfg.Sites {
		sites[id] = true
	}
	return &httpclient.SiteProxyPolicy{Enabled: true, Proxy: cfg.ProxyURL, Mode: cfg.Mode, Sites: sites}
}

func validSiteProxyMode(m string) bool {
	switch m {
	case httpclient.SiteProxyAll, httpclient.SiteProxyInclude, httpclient.SiteProxyExclude:
		return true
	}
	return false
}

// normalizeSiteProxySites 规范化、去重、排序站点 ID，去掉空的。
func normalizeSiteProxySites(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range in {
		n := httpclient.NormalizeSiteID(id)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func parseSiteProxySites(raw string) []string {
	var ids []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &ids) != nil {
		return []string{}
	}
	return normalizeSiteProxySites(ids)
}
