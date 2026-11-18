package cookiecloud

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

// Site 是 pt-tools 的一个站点：名称和 pt-tools 访问它用的地址。
type Site struct {
	Name    string
	BaseURL string
}

// Match 是一个站点在 CookieCloud 里找到的、对它的地址有效的 Cookie。
type Match struct {
	Site string
	// Host 是站点地址的主机名。
	Host string
	// Header 是拼好的 Cookie 请求头（name=value; ...）。只在本机使用，不出现在接口返回和日志里。
	Header string
	// Names 是 Cookie 的名字（按名字排序），预览时给用户看。
	Names []string
}

// MatchSites 按站点地址的主机名挑出对它有效的 Cookie：带前导点（或 hostOnly 为 false）的域名对它本身和子域名都有效，
// 其余的只对同一个主机名有效；路径不是 / 的、已经过期的不要。同名的取域名最具体的那个。
// 没有任何 Cookie 的站点不出现在结果里；结果按站点名排序。
func MatchSites(d Data, sites []Site, now time.Time) []Match {
	out := make([]Match, 0)
	for _, s := range sites {
		host := siteHost(s.BaseURL)
		if host == "" {
			continue
		}
		type pick struct {
			c     Cookie
			depth int
		}
		best := map[string]pick{}
		for _, list := range d.CookieData {
			for _, c := range list {
				if c.Name == "" || !pathOK(c.Path) || expired(c, now) {
					continue
				}
				depth, ok := applies(c, host)
				if !ok {
					continue
				}
				if cur, seen := best[c.Name]; !seen || depth > cur.depth {
					best[c.Name] = pick{c: c, depth: depth}
				}
			}
		}
		if len(best) == 0 {
			continue
		}
		names := make([]string, 0, len(best))
		for n := range best {
			names = append(names, n)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, n := range names {
			parts = append(parts, n+"="+best[n].c.Value)
		}
		out = append(out, Match{Site: s.Name, Host: host, Header: strings.Join(parts, "; "), Names: names})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Site < out[j].Site })
	return out
}

// siteHost 是地址的主机名（小写，不带端口）。
func siteHost(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

// applies 报告 Cookie 对 host 有没有效，以及匹配的具体程度（域名越长越具体，同一域名时只对本主机有效的更具体）。
func applies(c Cookie, host string) (int, bool) {
	d := strings.ToLower(strings.TrimSpace(c.Domain))
	hostOnly := !strings.HasPrefix(d, ".")
	if c.HostOnly != nil {
		hostOnly = *c.HostOnly
	}
	d = strings.TrimSuffix(strings.TrimPrefix(d, "."), ".")
	if d == "" {
		return 0, false
	}
	depth := len(d) * 2
	if hostOnly {
		depth++
	}
	switch {
	case host == d:
		return depth, true
	case !hostOnly && strings.HasSuffix(host, "."+d):
		return depth, true
	}
	return 0, false
}

func pathOK(p string) bool {
	return p == "" || p == "/"
}

func expired(c Cookie, now time.Time) bool {
	if c.ExpirationDate == nil || *c.ExpirationDate <= 0 {
		return false
	}
	return time.Unix(int64(*c.ExpirationDate), 0).Before(now)
}
