package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrackerResolver(t *testing.T) {
	r := newTrackerResolver([]*SiteDefinition{
		{ID: "hdsky", Schema: SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}},
		{ID: "mteam", Schema: SchemaMTorrent, URLs: []string{"https://kp.m-team.cc/"}, LegacyURLs: []string{"https://pt.m-team.cc/"}},
		{ID: "other", Schema: SchemaNexusPHP, URLs: []string{"https://www.example.org/"}, TrackerHosts: []string{"t.example-tracker.net"}},
		// 两个站点共用一个可注册域：只认主机名完全相同的
		{ID: "a", Schema: SchemaNexusPHP, URLs: []string{"https://a.shared.com/"}},
		{ID: "b", Schema: SchemaNexusPHP, URLs: []string{"https://b.shared.com/"}},
	})
	cases := map[string]string{
		"https://tracker.hdsky.me/announce.php?passkey=x": "hdsky",
		"HTTPS://HDSKY.ME:443/announce":                   "hdsky",
		"https://tracker.m-team.cc/announce?credential=x": "mteam",
		"https://t.example-tracker.net/announce":          "other",
		"https://a.shared.com/announce":                   "a",
		"https://shared.com/announce":                     "",
		"https://unknown.net/announce":                    "",
		"udp://tracker.hdsky.me:6969/announce":            "hdsky",
		"":                                                "",
		"not a url":                                       "",
	}
	for in, want := range cases {
		got, ok := r.Resolve(in)
		assert.Equal(t, want, got, in)
		assert.Equal(t, want != "", ok, in)
	}
	var nilResolver *TrackerResolver
	_, ok := nilResolver.Resolve("https://hdsky.me/")
	assert.False(t, ok)
	_, ok = nilResolver.Definition("hdsky")
	assert.False(t, ok)
	def, ok := r.Definition("hdsky")
	assert.True(t, ok)
	assert.Equal(t, "hdsky", def.ID)
}

func TestParseDownloadURL(t *testing.T) {
	nexus := &SiteDefinition{ID: "hdsky", Schema: SchemaNexusPHP, URLs: []string{"https://hdsky.me/"}}
	dolby := &SiteDefinition{ID: "hddolby", Schema: SchemaHDDolby, URLs: []string{"https://www.hddolby.com/"}}
	unit3d := &SiteDefinition{ID: "u3d", Schema: SchemaUnit3D, URLs: []string{"https://u3d.example.com/"}}
	gazelle := &SiteDefinition{ID: "gz", Schema: SchemaGazelle, URLs: []string{"https://gz.example.com/"}}
	mteam := &SiteDefinition{ID: "mteam", Schema: SchemaMTorrent, URLs: []string{"https://kp.m-team.cc/"}}
	cases := []struct {
		def  *SiteDefinition
		raw  string
		want string
	}{
		{nexus, "https://hdsky.me/download.php?id=123&passkey=abc", "123"},
		{nexus, "https://hdsky.me/download.php?hash=abc", ""},
		{nexus, "https://hdsky.me/details.php?id=123", ""},
		{nexus, "https://evil.example.com/download.php?id=123", ""},
		{dolby, "https://www.hddolby.com/download.php?id=9&downhash=x", "9"},
		{unit3d, "https://u3d.example.com/torrents/download/42", "42"},
		{unit3d, "https://u3d.example.com/torrent/download/42.rsskey123", "42"},
		{unit3d, "https://u3d.example.com/torrents/42", ""},
		{gazelle, "https://gz.example.com/torrents.php?action=download&id=7&authkey=a&torrent_pass=b", "7"},
		{gazelle, "https://gz.example.com/torrents.php?id=7", ""},
		{mteam, "https://api.m-team.cc/api/rss/dl?credential=x", ""},
		{nil, "https://hdsky.me/download.php?id=1", ""},
		{nexus, "::bad", ""},
	}
	for _, c := range cases {
		got, ok := ParseDownloadURL(c.def, c.raw)
		assert.Equal(t, c.want, got, c.raw)
		assert.Equal(t, c.want != "", ok, c.raw)
	}

	r := newTrackerResolver([]*SiteDefinition{nexus, unit3d})
	site, id, ok := r.ResolveDownloadURL("https://hdsky.me/download.php?id=5")
	assert.True(t, ok)
	assert.Equal(t, "hdsky", site)
	assert.Equal(t, "5", id)
	_, _, ok = r.ResolveDownloadURL("https://unknown.example.net/download.php?id=5")
	assert.False(t, ok)
	_, _, ok = r.ResolveDownloadURL("https://hdsky.me/details.php?id=5")
	assert.False(t, ok)
}
