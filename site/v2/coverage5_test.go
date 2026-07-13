package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// batch_download.go — createTarGzArchive / createZipArchive real files
// ---------------------------------------------------------------------------

func TestCreateTarGzArchive(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.torrent")
	f2 := filepath.Join(dir, "b.torrent")
	require.NoError(t, os.WriteFile(f1, []byte("aaa"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("bbb"), 0o644))

	archive := filepath.Join(dir, "out.tar.gz")
	err := createTarGzArchive(archive, dir, []string{f1, f2})
	require.NoError(t, err)

	info, err := os.Stat(archive)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestCreateZipArchive(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.torrent")
	require.NoError(t, os.WriteFile(f1, []byte("aaa"), 0o644))

	archive := filepath.Join(dir, "out.zip")
	err := createZipArchive(archive, dir, []string{f1})
	require.NoError(t, err)

	info, err := os.Stat(archive)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestCreateTarGzArchive_MissingFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "out.tar.gz")
	err := createTarGzArchive(archive, dir, []string{filepath.Join(dir, "missing.torrent")})
	require.Error(t, err)
}

func TestCreateZipArchive_MissingFile(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "out.zip")
	err := createZipArchive(archive, dir, []string{filepath.Join(dir, "missing.torrent")})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// FetchSeedingStatus — table row accumulation (Method 2)
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_FetchSeedingStatus_TableRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table>
			<tr><th>name</th><th>x</th><th>size</th></tr>
			<tr><td>t1</td><td>-</td><td>1.00 GB</td></tr>
			<tr><td>t2</td><td>-</td><td>2.00 GB</td></tr>
		</table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "42")
	require.NoError(t, err)
	assert.Equal(t, 2, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_ParseSeedingStatus_PipeFormat(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><div><div>10 | 100 GB</div></div></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	seeding, size, err := d.ParseSeedingStatus(NexusPHPResponse{Document: doc, RawBody: []byte(html)})
	require.NoError(t, err)
	assert.Equal(t, 10, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_ParseSeedingStatus_NilDoc(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, _, err := d.ParseSeedingStatus(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

// ---------------------------------------------------------------------------
// GuessUserLevelID — VIP & manager group branches
// ---------------------------------------------------------------------------

func TestGuessUserLevelID_VIPGroup(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 100, Name: "VIP", GroupType: LevelGroupVIP},
	}
	info := &UserInfo{LevelName: "VIP"}
	assert.Equal(t, 100, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_VIPGroup_NoMatchingReq(t *testing.T) {
	reqs := []SiteLevelRequirement{{ID: 1, Name: "User"}}
	info := &UserInfo{LevelName: "贵宾"}
	assert.Equal(t, MinVipLevelID, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_ManagerGroup(t *testing.T) {
	reqs := []SiteLevelRequirement{{ID: 1, Name: "User"}}
	info := &UserInfo{LevelName: "Administrator"}
	assert.Equal(t, MinManagerLevelID, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_PreviousLevelOnUnmet(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "1PB"},
	}
	info := &UserInfo{Downloaded: 1024}
	got := GuessUserLevelID(info, reqs)
	assert.Equal(t, 1, got)
}

// ---------------------------------------------------------------------------
// executeProcess — critical error propagation via full pipeline
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_GetUserInfoWithDefinition_SessionExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><form action="takelogin.php"></form></body></html>`))
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID:     "npdef2",
		Schema: SchemaNexusPHP,
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{RequestConfig: RequestConfig{URL: "/index.php"}, Fields: []string{"id"}},
			},
			Selectors: map[string]FieldSelector{"id": {Selector: []string{"#id"}}},
		},
	}
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewDBUserInfoRepo — success path
// ---------------------------------------------------------------------------

func TestNewDBUserInfoRepo_Success(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	repo, err := NewDBUserInfoRepo(db)
	require.NoError(t, err)
	require.NotNil(t, repo)
}

// ---------------------------------------------------------------------------
// mtorrent_driver.go — executeDirectly form-urlencoded body + string body
// ---------------------------------------------------------------------------

func TestMTorrentDriver_ExecuteDirectly_FormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	res, err := d.Execute(context.Background(), MTorrentRequest{
		Endpoint:    "/api/x",
		Method:      "POST",
		Body:        map[string]any{"id": "5"},
		ContentType: "application/x-www-form-urlencoded",
	})
	require.NoError(t, err)
	assert.True(t, res.Code.IsSuccess())

	res2, err := d.Execute(context.Background(), MTorrentRequest{
		Endpoint:    "/api/x",
		Method:      "POST",
		Body:        "id=7",
		ContentType: "application/x-www-form-urlencoded",
	})
	require.NoError(t, err)
	assert.True(t, res2.Code.IsSuccess())
}

// ---------------------------------------------------------------------------
// SuggestBestDiscount — branches
// ---------------------------------------------------------------------------

func TestSuggestBestDiscount_Cov5(t *testing.T) {
	// already free
	assert.Equal(t, DiscountFree, SuggestBestDiscount(DiscountFree, time.Time{}, time.Hour))
	// permanent discount
	assert.Equal(t, DiscountPercent50, SuggestBestDiscount(DiscountPercent50, time.Time{}, time.Hour))
	// enough time remaining
	assert.Equal(t, DiscountPercent50, SuggestBestDiscount(DiscountPercent50, time.Now().Add(2*time.Hour), time.Hour))
	// not enough time
	assert.Equal(t, DiscountPercent50, SuggestBestDiscount(DiscountPercent50, time.Now().Add(time.Minute), time.Hour))
}

// ---------------------------------------------------------------------------
// hddolby Search error + getOrRefreshDetailCache array fallback
// ---------------------------------------------------------------------------

func TestHDDolbyDriver_Search_Error(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "http://127.0.0.1:1", APIURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.Search(context.Background(), SearchQuery{Keyword: "x"})
	require.Error(t, err)
}

func TestHDDolbyDriver_GetTorrentDetail_ArrayCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":200,"data":[{"id":7,"name":"ArrForm"}]}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	item, err := d.GetTorrentDetail(context.Background(), "7", "", "")
	require.NoError(t, err)
	assert.Equal(t, "7", item.ID)
	assert.Equal(t, "ArrForm", item.Title)
}
