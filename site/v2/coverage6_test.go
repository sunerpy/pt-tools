package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newLegacyUserInfoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "userdetails.php") {
			_, _ = w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">用户名</td><td class="rowfollow">LegacyUser</td></tr>
				<tr><td class="rowhead">上传量</td><td class="rowfollow">1.50 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td class="rowfollow">500.00 GB</td></tr>
			</table></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>
			<div id="info_block"><a class="User_Name" href="userdetails.php?id=555">LegacyUser</a></div>
		</body></html>`))
	}))
}

// hashDriver implements Driver[string,string] AND HashDownloader.
type hashDriver struct {
	hashCalled bool
}

func (h *hashDriver) PrepareSearch(SearchQuery) (string, error)       { return "", nil }
func (h *hashDriver) Execute(context.Context, string) (string, error) { return "", nil }
func (h *hashDriver) ParseSearch(string) ([]TorrentItem, error)       { return nil, nil }
func (h *hashDriver) GetUserInfo(context.Context) (UserInfo, error)   { return UserInfo{}, nil }
func (h *hashDriver) PrepareDownload(string) (string, error)          { return "", nil }
func (h *hashDriver) ParseDownload(string) ([]byte, error)            { return nil, nil }
func (h *hashDriver) DownloadWithHash(_ context.Context, _, hash string) ([]byte, error) {
	h.hashCalled = true
	return []byte("hash:" + hash), nil
}

func TestBaseSite_DownloadWithHash_UsesHashDownloader(t *testing.T) {
	hd := &hashDriver{}
	site := NewBaseSite[string, string](hd, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteHDDolby, RateLimit: 100, RateBurst: 100, Logger: zap.NewNop()})
	data, err := site.DownloadWithHash(context.Background(), "5", "abc")
	require.NoError(t, err)
	assert.Equal(t, []byte("hash:abc"), data)
	assert.True(t, hd.hashCalled)
}

func TestBaseSite_DownloadWithHash_RateLimitCanceled(t *testing.T) {
	hd := &hashDriver{}
	site := NewBaseSite[string, string](hd, BaseSiteConfig{ID: "t", Name: "T", Kind: SiteHDDolby, RateLimit: 0.0001, RateBurst: 1, Logger: zap.NewNop()})
	// exhaust the single burst token, then cancel
	_, _ = site.DownloadWithHash(context.Background(), "1", "h")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := site.DownloadWithHash(ctx, "2", "h")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ParseSizeMB — TB and KB unit branches
// ---------------------------------------------------------------------------

func TestNexusPHPParser_ParseSizeMB_Units(t *testing.T) {
	p := NewNexusPHPParser()

	tb := `<html><body><table><tr><td class="rowhead">基本信息</td><td>大小：2.00 TB</td></tr></table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(tb))
	assert.InDelta(t, 2.0*1024*1024, p.ParseSizeMB(doc.Selection), 1)

	kb := `<html><body><table><tr><td class="rowhead">基本信息</td><td>大小：512.00 KB</td></tr></table></body></html>`
	doc2, _ := goquery.NewDocumentFromReader(strings.NewReader(kb))
	assert.InDelta(t, 512.0/1024, p.ParseSizeMB(doc2.Selection), 0.01)

	none := `<html><body><table><tr><td class="rowhead">基本信息</td><td>无</td></tr></table></body></html>`
	doc3, _ := goquery.NewDocumentFromReader(strings.NewReader(none))
	assert.Equal(t, float64(0), p.ParseSizeMB(doc3.Selection))
}

// ---------------------------------------------------------------------------
// mtorrent GetUnreadMessageCount error
// ---------------------------------------------------------------------------

func TestMTorrentDriver_GetUnreadMessageCount_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, _, err := d.GetUnreadMessageCount(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// GetSiteNextLevelUnmet — seedingBonus + interval + bonusPerHour branches
// ---------------------------------------------------------------------------

func TestGetSiteNextLevelUnmet_ExtendedBranches(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{
			ID:           2,
			Name:         "Power User",
			Downloaded:   "100GB",
			Uploaded:     "500GB",
			Ratio:        3.0,
			Bonus:        10000,
			SeedingBonus: 5000,
			Interval:     "P52W",
		},
	}
	info := &UserInfo{
		LevelID:             1,
		Downloaded:          10 * 1024 * 1024 * 1024,
		Uploaded:            1024,
		Ratio:               1.0,
		Bonus:               100,
		BonusPerHour:        10,
		SeedingBonus:        100,
		SeedingBonusPerHour: 5,
		JoinDate:            time.Now().Add(-24 * time.Hour).Unix(),
	}
	unmet := GetSiteNextLevelUnmet(info, reqs)
	assert.Contains(t, unmet, "downloaded")
	assert.Contains(t, unmet, "uploaded")
	assert.Contains(t, unmet, "ratio")
	assert.Contains(t, unmet, "bonus")
	assert.Contains(t, unmet, "bonusNeededHours")
	assert.Contains(t, unmet, "seedingBonus")
	assert.Contains(t, unmet, "seedingBonusNeededHours")
	assert.Contains(t, unmet, "interval")
}

// ---------------------------------------------------------------------------
// isAlternativeMet — each field branch
// ---------------------------------------------------------------------------

func TestIsAlternativeMet_Branches(t *testing.T) {
	alt := AlternativeRequirement{SeedingBonus: 100, Uploads: 5, Bonus: 1000, Downloaded: "10GB", Ratio: 2.0}
	// all met
	ok := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 20 * 1024 * 1024 * 1024, Ratio: 3.0}
	assert.True(t, isAlternativeMet(ok, alt))
	// downloaded not met
	bad := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 1, Ratio: 3.0}
	assert.False(t, isAlternativeMet(bad, alt))
	// ratio not met
	bad2 := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 20 * 1024 * 1024 * 1024, Ratio: 1.0}
	assert.False(t, isAlternativeMet(bad2, alt))
}

// ---------------------------------------------------------------------------
// getUserInfoLegacy — full 2-step legacy pipeline
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_GetUserInfoLegacy(t *testing.T) {
	server := newLegacyUserInfoServer(t)
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	// no site definition -> legacy path
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "LegacyUser", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
}

// ---------------------------------------------------------------------------
// ParseDetail — DetailDownloadLink custom selector (form + link)
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseDetail_CustomSelector(t *testing.T) {
	sel := DefaultNexusPHPSelectors()
	sel.DetailDownloadLink = "a.customdl"
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com", Selectors: &sel})
	html := `<html><body><a class="customdl" href="/dl?id=1">go</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "/dl?id=1", detail.DownloadURL)
}

func TestNexusPHPDriver_ParseDetail_Strategy4IDOnly(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><a href="download.php?id=42">go</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "id=42")
}

// ---------------------------------------------------------------------------
// registry.CreateSite — success creating each supported kind
// ---------------------------------------------------------------------------

func TestSiteRegistry_CreateSite_Success(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())
	registry.Register(SiteMeta{ID: "hd", Name: "HD", Kind: SiteHDDolby, DefaultBaseURL: "https://hd.example"})

	site, err := registry.CreateSite("hd", SiteCredentials{APIKey: "k", Cookie: "c=1"}, "")
	require.NoError(t, err)
	require.NotNil(t, site)

	registry.Register(SiteMeta{ID: "gz", Name: "GZ", Kind: SiteGazelle, DefaultBaseURL: "https://gz.example"})
	site2, err := registry.CreateSite("gz", SiteCredentials{APIKey: "k"}, "")
	require.NoError(t, err)
	require.NotNil(t, site2)
}

func TestSiteRegistry_CreateSite_NoBaseURL(t *testing.T) {
	registry := NewSiteRegistry(zap.NewNop())
	registry.Register(SiteMeta{ID: "nb", Name: "NB", Kind: SiteNexusPHP})
	_, err := registry.CreateSite("nb", SiteCredentials{Cookie: "c=1"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no base URL")
}
