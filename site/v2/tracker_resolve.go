package v2

import (
	"net/url"
	"regexp"
	"strings"
)

// TrackerResolver 按 tracker 或下载地址的主机名找出所属站点：先比主机名（TrackerHosts 与站点地址），
// 再比可注册域（如 tracker.hdsky.me 与 hdsky.me 同属 hdsky.me）。多个站点共用一个可注册域时只认主机名完全相同的。
//
// 它是注册表的一份快照；循环里判断很多种子时建一个复用，不要每次都调 ResolveSiteByTracker。
type TrackerResolver struct {
	byHost   map[string][]string
	byDomain map[string][]string
	defs     map[string]*SiteDefinition
}

// NewTrackerResolver 用当前注册的全部站点定义建一个解析器（含已停用的站点：旧种子仍归它们）。
func NewTrackerResolver() *TrackerResolver {
	return newTrackerResolver(GetDefinitionRegistry().GetAll())
}

// NewTrackerResolverFrom 用给定的站点定义建一个解析器（测试与只关心部分站点的调用方用）。
func NewTrackerResolverFrom(defs ...*SiteDefinition) *TrackerResolver {
	return newTrackerResolver(defs)
}

func newTrackerResolver(defs []*SiteDefinition) *TrackerResolver {
	r := &TrackerResolver{byHost: map[string][]string{}, byDomain: map[string][]string{}, defs: map[string]*SiteDefinition{}}
	for _, def := range defs {
		if def == nil || def.ID == "" {
			continue
		}
		r.defs[def.ID] = def
		for _, raw := range siteHostSources(def) {
			host := hostOf(raw)
			if host == "" {
				continue
			}
			r.byHost[host] = appendUnique(r.byHost[host], def.ID)
			r.byDomain[domainKey(host)] = appendUnique(r.byDomain[domainKey(host)], def.ID)
		}
	}
	return r
}

// siteHostSources 是能代表站点主机的地址：站点地址、网页地址、旧地址与 TrackerHosts。
func siteHostSources(def *SiteDefinition) []string {
	out := make([]string, 0, len(def.URLs)+len(def.LegacyURLs)+len(def.TrackerHosts)+1)
	out = append(out, def.URLs...)
	if def.WebURL != "" {
		out = append(out, def.WebURL)
	}
	out = append(out, def.LegacyURLs...)
	out = append(out, def.TrackerHosts...)
	return out
}

// Resolve 返回 tracker 地址（announce URL）所属站点的 ID。
func (r *TrackerResolver) Resolve(rawURL string) (string, bool) {
	if r == nil {
		return "", false
	}
	host := hostOf(rawURL)
	if host == "" {
		return "", false
	}
	if ids := r.byHost[host]; len(ids) == 1 {
		return ids[0], true
	}
	if ids := r.byDomain[domainKey(host)]; len(ids) == 1 {
		return ids[0], true
	}
	return "", false
}

// Definition 返回站点 ID 对应的定义。
func (r *TrackerResolver) Definition(siteID string) (*SiteDefinition, bool) {
	if r == nil {
		return nil, false
	}
	def, ok := r.defs[siteID]
	return def, ok
}

// ResolveDownloadURL 找出下载地址所属的站点与种子 ID；地址不属于任何内置站点或格式认不出时 ok 为 false。
func (r *TrackerResolver) ResolveDownloadURL(rawURL string) (siteID, torrentID string, ok bool) {
	siteID, ok = r.Resolve(rawURL)
	if !ok {
		return "", "", false
	}
	torrentID, ok = ParseDownloadURL(r.defs[siteID], rawURL)
	if !ok {
		return "", "", false
	}
	return siteID, torrentID, true
}

// ResolveSiteByTracker 是 NewTrackerResolver().Resolve 的简写，适合偶尔判断一次。
func ResolveSiteByTracker(announceURL string) (string, bool) {
	return NewTrackerResolver().Resolve(announceURL)
}

var (
	numericID        = regexp.MustCompile(`^\d+$`)
	unit3dDownloadRe = regexp.MustCompile(`^/torrents?/download/(\d+)(?:\.[^/]*)?/?$`)
)

// ParseDownloadURL 按站点架构从种子下载地址里取出种子 ID：
// NexusPHP 与 HDDolby 是 download.php?id=，Unit3D 是 /torrents/download/{id}（RSS 链接带 .{key} 后缀），
// Gazelle 是 torrents.php?action=download&id=。M-Team 的下载地址是临时签名链接、不含种子 ID，返回 false；
// 地址的主机不属于这个站点时也返回 false。
func ParseDownloadURL(def *SiteDefinition, rawURL string) (string, bool) {
	if def == nil {
		return "", false
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	if !belongsToSite(def, strings.ToLower(u.Hostname())) {
		return "", false
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	q := u.Query()
	switch def.Schema {
	case SchemaNexusPHP, SchemaHDDolby:
		if strings.HasSuffix(path, "/download.php") && numericID.MatchString(q.Get("id")) {
			return q.Get("id"), true
		}
	case SchemaUnit3D:
		if m := unit3dDownloadRe.FindStringSubmatch(u.EscapedPath()); m != nil {
			return m[1], true
		}
	case SchemaGazelle:
		if strings.HasSuffix(path, "/torrents.php") && q.Get("action") == "download" && numericID.MatchString(q.Get("id")) {
			return q.Get("id"), true
		}
	}
	return "", false
}

func belongsToSite(def *SiteDefinition, host string) bool {
	domain := domainKey(host)
	for _, raw := range siteHostSources(def) {
		h := hostOf(raw)
		if h != "" && (h == host || domainKey(h) == domain) {
			return true
		}
	}
	return false
}

// hostOf 取出地址里的小写主机名；只写了主机名（TrackerHosts 常见）也认。
func hostOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "//" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
}

// domainKey 是主机名的可注册域（eTLD+1）；IP 与算不出可注册域的主机名原样使用。
func domainKey(host string) string {
	if d := registrableDomain(host); d != "" {
		return d
	}
	return host
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
