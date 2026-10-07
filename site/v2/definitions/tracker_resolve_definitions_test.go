package definitions

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/publicsuffix"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 每个内置站点：站点域名下的 tracker 子域都能识别回这个站点（容量闸门、下载器助手按 tracker 归属种子）。
func TestTrackerResolverCoversAllDefinitions(t *testing.T) {
	r := v2.NewTrackerResolver()
	owners := map[string][]string{}
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		require.NotEmpty(t, def.URLs, def.ID)
		for _, raw := range def.URLs {
			u, err := url.Parse(raw)
			require.NoError(t, err, def.ID)
			require.NotEmpty(t, u.Hostname(), "%s: %s", def.ID, raw)
			domain, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(u.Hostname()))
			require.NoError(t, err, def.ID)
			owners[domain] = appendIfMissing(owners[domain], def.ID)
		}
	}
	for domain, ids := range owners {
		assert.Len(t, ids, 1, "可注册域 %s 被多个站点共用：%v，在定义里写 TrackerHosts 区分", domain, ids)
		if len(ids) != 1 {
			continue
		}
		got, ok := r.Resolve("https://tracker." + domain + "/announce.php?passkey=x")
		assert.True(t, ok, domain)
		assert.Equal(t, ids[0], got, domain)
	}
}

// 每个内置站点的下载地址都按架构解析出种子 ID；M-Team 与 Rousi 的下载地址不含可复用的种子 ID，解析不了。
func TestParseDownloadURLAllDefinitions(t *testing.T) {
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		base := strings.TrimRight(def.URLs[0], "/")
		var raw string
		want := "123"
		switch def.Schema {
		case v2.SchemaNexusPHP, v2.SchemaHDDolby:
			raw = base + "/download.php?id=123&passkey=x"
		case v2.SchemaUnit3D:
			raw = base + "/torrents/download/123"
		case v2.SchemaGazelle:
			raw = base + "/torrents.php?action=download&id=123&authkey=a&torrent_pass=b"
		default:
			raw, want = base+"/download.php?id=123", ""
		}
		got, ok := v2.ParseDownloadURL(def, raw)
		assert.Equal(t, want, got, "%s (%s): %s", def.ID, def.Schema, raw)
		assert.Equal(t, want != "", ok, def.ID)

		_, ok = v2.ParseDownloadURL(def, "https://not-this-site.invalid/download.php?id=123")
		assert.False(t, ok, "%s: 别的主机不算这个站点", def.ID)
	}
}

func appendIfMissing(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
