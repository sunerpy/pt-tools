package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/core"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// 站点代理（全局设置）：GET/PUT /api/site-proxy。只认网页会话（s.auth）；App API 与 MCP 的配置网关读改不到
// （见 configExcluded：改站点流量从哪儿出去属于安全边界）。保存成功以后马上生效，不用重启。

// SiteProxyOption 是站点多选里的一项：注册表里能发请求的站点。
type SiteProxyOption struct {
	// ID 是规范化的站点 ID（注册表 ID 的小写）
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// SiteProxyResponse 是读到的站点代理设置。ProxyURL 里的密码换成了 ***。
type SiteProxyResponse struct {
	Enabled  bool     `json:"enabled"`
	ProxyURL string   `json:"proxy_url"`
	Mode     string   `json:"mode"`
	Sites    []string `json:"sites"`
	// UpdatedAt 是上次保存的时间，没保存过时为 null
	UpdatedAt *time.Time `json:"updated_at"`
	// SiteOptions 是站点多选的唯一数据来源：启用的在前，同组按名字排
	SiteOptions []SiteProxyOption `json:"site_options"`
	// ProxyUnreadable 为真时存着的代理地址解不开（加密密钥换过），要重新填
	ProxyUnreadable bool `json:"proxy_unreadable,omitempty"`
}

// SiteProxyRequest 是保存站点代理的请求。ProxyURL 与读到的（打了码的）相同表示不改，空串表示清除。
type SiteProxyRequest struct {
	Enabled  bool     `json:"enabled"`
	ProxyURL string   `json:"proxy_url"`
	Mode     string   `json:"mode"`
	Sites    []string `json:"sites"`
}

func (s *Server) apiSiteProxy(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.writeSiteProxy(w)
	case http.MethodPut, http.MethodPost:
		var in SiteProxyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
			http.Error(w, "请求格式错误", http.StatusBadRequest)
			return
		}
		options, err := s.siteProxyOptions()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		known := make(map[string]bool, len(options))
		for _, o := range options {
			known[o.ID] = true
		}
		for _, id := range in.Sites {
			if n := httpclient.NormalizeSiteID(id); n != "" && !known[n] {
				http.Error(w, fmt.Sprintf("不认识的站点：%s", n), http.StatusBadRequest)
				return
			}
		}
		if _, err := s.store.SaveSiteProxy(core.SiteProxyInput{Enabled: in.Enabled, ProxyURL: in.ProxyURL, Mode: in.Mode, Sites: in.Sites}); err != nil {
			if errors.Is(err, core.ErrSiteProxyInvalid) {
				// 只回原因：界面会在前面写「站点代理没有保存：」
				http.Error(w, strings.TrimPrefix(err.Error(), core.ErrSiteProxyInvalid.Error()+"："), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.writeSiteProxy(w)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) writeSiteProxy(w http.ResponseWriter) {
	cfg, err := s.store.SiteProxy()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	options, err := s.siteProxyOptions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 只回选项里有的站点：站点定义在新版本里去掉以后，存着的 ID 不让界面卡在「不认识的站点」上（下次保存就不带了）
	known := make(map[string]bool, len(options))
	for _, o := range options {
		known[o.ID] = true
	}
	sites := []string{}
	for _, id := range cfg.Sites {
		if known[id] {
			sites = append(sites, id)
		}
	}
	writeJSON(w, SiteProxyResponse{
		Enabled: cfg.Enabled, ProxyURL: httpclient.MaskProxyURL(cfg.ProxyURL), Mode: cfg.Mode, Sites: sites,
		UpdatedAt: cfg.UpdatedAt, SiteOptions: options, ProxyUnreadable: cfg.ProxyUnreadable,
	})
}

// siteProxyOptions 是站点多选的选项：全局站点注册表里的站点（能建出站点实例、会发请求的只有这些），
// 启用状态取同名站点设置。
func (s *Server) siteProxyOptions() ([]SiteProxyOption, error) {
	enabled, err := s.store.EnabledSiteNames()
	if err != nil {
		return nil, err
	}
	reg := v2.GetGlobalSiteRegistry()
	seen := map[string]bool{}
	out := []SiteProxyOption{}
	for _, id := range reg.List() {
		meta, ok := reg.Get(id)
		n := httpclient.NormalizeSiteID(id)
		if !ok || n == "" || seen[n] {
			continue
		}
		seen[n] = true
		name := strings.TrimSpace(meta.Name)
		if name == "" {
			name = n
		}
		out = append(out, SiteProxyOption{ID: n, Name: name, Enabled: enabled[n]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Enabled != out[j].Enabled {
			return out[i].Enabled
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
