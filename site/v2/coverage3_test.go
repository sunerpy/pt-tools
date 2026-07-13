package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// site_definition.go — validateSchemaSpecific / validateUserInfo /
// validateLevelRequirements branches via Validate()
// ---------------------------------------------------------------------------

func TestValidate_NexusPHP_MissingSelectorsAndUserInfo(t *testing.T) {
	def := &SiteDefinition{
		ID:             "nptest",
		Name:           "NP",
		Schema:         SchemaNexusPHP,
		URLs:           []string{"https://np.example/"},
		TimezoneOffset: "+0800",
	}
	err := def.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Selectors")
	assert.Contains(t, msg, "UserInfo")
}

func TestValidate_NexusPHP_PartialSelectors(t *testing.T) {
	def := &SiteDefinition{
		ID:             "nptest2",
		Name:           "NP",
		Schema:         SchemaNexusPHP,
		URLs:           []string{"https://np.example/"},
		TimezoneOffset: "+0800",
		Selectors:      &SiteSelectors{TableRows: "tr"},
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{RequestConfig: RequestConfig{URL: "/index.php"}, Fields: []string{"id"}},
			},
			Selectors: map[string]FieldSelector{"id": {Selector: []string{"#id"}}},
		},
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Title")
}

func TestValidate_MTorrent_MissingUserInfo(t *testing.T) {
	def := &SiteDefinition{
		ID:             "mttest",
		Name:           "MT",
		Schema:         SchemaMTorrent,
		URLs:           []string{"https://mt.example/"},
		TimezoneOffset: "+0800",
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UserInfo")
}

func TestValidate_HDDolby_BadAuthMethod(t *testing.T) {
	def := &SiteDefinition{
		ID:             "hdtest",
		Name:           "HD",
		Schema:         SchemaHDDolby,
		URLs:           []string{"https://hd.example/"},
		TimezoneOffset: "+0800",
		AuthMethod:     AuthMethodCookie,
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AuthMethod")
}

func TestValidate_Rousi_MissingCreateDriver(t *testing.T) {
	def := &SiteDefinition{
		ID:             "rstest",
		Name:           "RS",
		Schema:         SchemaRousi,
		URLs:           []string{"https://rs.example/"},
		TimezoneOffset: "+0800",
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CreateDriver")
}

func TestValidate_UserInfo_BadAssertionAndSelectors(t *testing.T) {
	def := &SiteDefinition{
		ID:             "uitest",
		Name:           "UI",
		Schema:         SchemaMTorrent,
		URLs:           []string{"https://ui.example/"},
		TimezoneOffset: "+0800",
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/detail"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"name"},
				},
			},
			Selectors: map[string]FieldSelector{
				"name": {Selector: []string{"n"}},
				"bad":  {},
			},
		},
	}
	err := def.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "InvalidReference")
	assert.Contains(t, msg, "NoSelector")
}

func TestValidate_UserInfo_EmptyProcess(t *testing.T) {
	def := &SiteDefinition{
		ID:             "uitest2",
		Name:           "UI",
		Schema:         SchemaMTorrent,
		URLs:           []string{"https://ui.example/"},
		TimezoneOffset: "+0800",
		UserInfo:       &UserInfoConfig{},
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one process")
}

func TestValidate_LevelRequirements_Invalid(t *testing.T) {
	def := &SiteDefinition{
		ID:             "lvtest",
		Name:           "LV",
		Schema:         SchemaMTorrent,
		URLs:           []string{"https://lv.example/"},
		TimezoneOffset: "+0800",
		UserInfo: &UserInfoConfig{
			Process:   []UserInfoProcess{{RequestConfig: RequestConfig{URL: "/x"}, Fields: []string{"id"}}},
			Selectors: map[string]FieldSelector{"id": {Selector: []string{"#id"}}},
		},
		LevelRequirements: []SiteLevelRequirement{
			{ID: 1, Name: ""},
			{ID: 1, Name: "Dup", Downloaded: "notasize", Interval: "bad", Ratio: -1},
			{ID: 2, Name: "Dup"},
		},
	}
	err := def.Validate()
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "Duplicate")
	assert.Contains(t, msg, "Format")
}

// ---------------------------------------------------------------------------
// nexusphp_parser.go — NewNexusPHPParserFromDefinition custom config + ParseAll
// ---------------------------------------------------------------------------

func TestNewNexusPHPParserFromDefinition_Custom(t *testing.T) {
	def := &SiteDefinition{
		DetailParser: &DetailParserConfig{
			TimeLayout:       "2006-01-02 15:04:05",
			DiscountMapping:  map[string]DiscountLevel{"myfree": DiscountFree},
			HRKeywords:       []string{"MYHR"},
			TitleSelector:    "input[name='torrent_name']",
			IDSelector:       "input[name='detail_torrent_id']",
			DiscountSelector: "h1 font",
			EndTimeSelector:  "h1 span[title]",
			SizeSelector:     "td.rowhead:contains('基本信息')",
			SizeRegex:        `大小：[^\d]*([\d.]+)\s*(GB|MB|KB|TB)`,
		},
	}
	parser := NewNexusPHPParserFromDefinition(def)
	require.NotNil(t, parser)

	html := `<html><body>
		<input name="torrent_name" value="Cool.Movie.2024">
		<input name="detail_torrent_id" value="777">
		<h1><font class="myfree">FREE</font><span title="2026-01-20 15:30:00">x</span></h1>
		<table><tr><td class="rowhead">基本信息</td><td>大小：4.00 GB</td></tr></table>
		<div>MYHR flag</div>
	</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)

	info := parser.ParseAll(doc.Selection)
	assert.Equal(t, "777", info.TorrentID)
	assert.Equal(t, "Cool.Movie.2024", info.Title)
	assert.Equal(t, DiscountFree, info.DiscountLevel)
	assert.InDelta(t, 4.0*1024, info.SizeMB, 0.1)
	assert.True(t, info.HasHR)
	assert.False(t, info.DiscountEnd.IsZero())
}

func TestNewNexusPHPParserFromDefinition_Nil(t *testing.T) {
	parser := NewNexusPHPParserFromDefinition(nil)
	require.NotNil(t, parser)
	parser2 := NewNexusPHPParserFromDefinition(&SiteDefinition{})
	require.NotNil(t, parser2)
}

// ---------------------------------------------------------------------------
// level.go — GuessUserLevelID, GetSiteNextLevelUnmet, CalculateSiteLevelProgress
// ---------------------------------------------------------------------------

func levelReqs() []SiteLevelRequirement {
	return []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", NameAka: []string{"高级用户"}, Downloaded: "100GB", Ratio: 2.0, Bonus: 1000, Interval: "P5W", SeedingBonus: 500},
		{ID: 3, Name: "Elite User", Downloaded: "500GB", Ratio: 3.0},
		{ID: 10, Name: "VIP", GroupType: LevelGroupVIP},
	}
}

func TestGuessUserLevelID_NameMatch(t *testing.T) {
	info := &UserInfo{LevelName: "Power User"}
	assert.Equal(t, 2, GuessUserLevelID(info, levelReqs()))
}

func TestGuessUserLevelID_AkaMatch(t *testing.T) {
	info := &UserInfo{LevelName: "高级用户"}
	assert.Equal(t, 2, GuessUserLevelID(info, levelReqs()))
}

func TestGuessUserLevelID_EmptyNoReqs(t *testing.T) {
	assert.Equal(t, -1, GuessUserLevelID(&UserInfo{}, nil))
}

func TestGuessUserLevelID_ByRequirements(t *testing.T) {
	info := &UserInfo{
		Downloaded:   150 * 1024 * 1024 * 1024,
		Ratio:        2.5,
		Bonus:        2000,
		SeedingBonus: 600,
		JoinDate:     time.Now().Add(-100 * 24 * time.Hour).Unix(),
	}
	id := GuessUserLevelID(info, levelReqs())
	assert.GreaterOrEqual(t, id, 1)
}

func TestGetSiteNextLevelUnmet_Cov3(t *testing.T) {
	info := &UserInfo{
		LevelID:      2,
		Downloaded:   10 * 1024 * 1024 * 1024,
		Ratio:        1.0,
		Bonus:        100,
		BonusPerHour: 10,
	}
	unmet := GetSiteNextLevelUnmet(info, levelReqs())
	require.NotNil(t, unmet)
	assert.Contains(t, unmet, "downloaded")
	assert.Contains(t, unmet, "ratio")
}

func TestGetSiteNextLevelUnmet_NoNext(t *testing.T) {
	info := &UserInfo{LevelID: 3}
	unmet := GetSiteNextLevelUnmet(info, levelReqs())
	assert.Empty(t, unmet)
}

func TestCalculateSiteLevelProgress_Cov3(t *testing.T) {
	info := &UserInfo{
		LevelID:    1,
		Downloaded: 50 * 1024 * 1024 * 1024,
		Ratio:      1.0,
		Bonus:      100,
	}
	progress := CalculateSiteLevelProgress(info, levelReqs())
	require.NotNil(t, progress)
	assert.NotNil(t, progress.CurrentLevel)
	assert.LessOrEqual(t, progress.ProgressPercent, float64(100))
}

func TestCalculateSiteLevelProgress_NoReqs(t *testing.T) {
	assert.Nil(t, CalculateSiteLevelProgress(&UserInfo{}, nil))
}

func TestCalculateSiteLevelProgress_MaxLevel(t *testing.T) {
	info := &UserInfo{LevelID: 3}
	progress := CalculateSiteLevelProgress(info, levelReqs())
	require.NotNil(t, progress)
	assert.Equal(t, float64(100), progress.ProgressPercent)
}

func TestIsSiteRequirementMet_ExtendedFields(t *testing.T) {
	req := SiteLevelRequirement{
		Uploads:     5,
		Seeding:     10,
		SeedingSize: "1TB",
	}
	// not met — nothing set
	assert.False(t, isSiteRequirementMet(&UserInfo{}, req))
	// met
	info := &UserInfo{
		Uploads:    6,
		Seeding:    11,
		SeederSize: 2 * 1024 * 1024 * 1024 * 1024,
	}
	assert.True(t, isSiteRequirementMet(info, req))
}

// ---------------------------------------------------------------------------
// nexusphp_driver.go — getUserInfoWithDefinition full pipeline
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_GetUserInfoWithDefinition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "userdetails.php"):
			_, _ = w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">上传量</td><td>1.50 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td>500.00 GB</td></tr>
			</table></body></html>`))
		default:
			_, _ = w.Write([]byte(`<html><body>
				<a href="userdetails.php?id=123">MyName</a>
			</body></html>`))
		}
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID:     "npdef",
		Name:   "NPDef",
		Schema: SchemaNexusPHP,
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"},
					Fields:        []string{"id", "name"},
				},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded", "downloaded"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id":   {Selector: []string{"a[href*='userdetails.php']"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name": {Selector: []string{"a[href*='userdetails.php']"}},
				"uploaded": {
					Selector: []string{"td.rowhead:contains('上传量') + td"},
					Filters:  []Filter{{Name: "parseSize"}},
				},
				"downloaded": {
					Selector: []string{"td.rowhead:contains('下载量') + td"},
					Filters:  []Filter{{Name: "parseSize"}},
				},
			},
		},
	}

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "123", info.UserID)
	assert.Equal(t, "MyName", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
}

func TestNexusPHPDriver_ExtractFieldValue_AttrAndDefault(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><a href="/details.php?id=42" title="mytitle">link</a><span class="lvl"></span></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	// href attribute
	v := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "href"})
	assert.Equal(t, "/details.php?id=42", v)

	// html attribute
	vh := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "html"})
	assert.Equal(t, "link", vh)

	// default text when no match
	vd := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"#missing"}, Text: "fallback"})
	assert.Equal(t, "fallback", vd)

	// text with filter
	vf := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}})
	assert.Equal(t, "42", vf)
}

// ---------------------------------------------------------------------------
// hddolby_driver.go — GetTorrentDetail cache-miss refresh path + DownloadWithHash
// ---------------------------------------------------------------------------

func TestHDDolbyDriver_GetTorrentDetail_CacheMissRefresh(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":200,"data":{"data":[{"id":1,"name":"A"}],"total":1}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	// Requesting a missing ID repeatedly should trigger cache invalidation + refresh at miss>=3.
	for i := 0; i < 3; i++ {
		item, err := d.GetTorrentDetail(context.Background(), "999", "", "")
		require.NoError(t, err)
		assert.Equal(t, "999", item.ID)
	}
	assert.GreaterOrEqual(t, calls, 2)
}

func TestHDDolbyDriver_DownloadWithHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("d8:announce"))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	data, err := d.DownloadWithHash(context.Background(), "5", "hh")
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)

	data2, err := d.Download(context.Background(), "5")
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data2)
}

func TestHDDolbyDriver_DownloadWithHash_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIURL: server.URL, APIKey: "k"})
	_, err := d.DownloadWithHash(context.Background(), "5", "")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// userinfo_service.go — FetchAndSaveAllWithConcurrency
// ---------------------------------------------------------------------------

func TestUserInfoService_FetchAndSaveAllWithConcurrency(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{Logger: zap.NewNop()})
	ctx := context.Background()

	s1 := &MockSite{}
	s1.On("ID").Return("hdsky")
	s1.On("GetUserInfo", mock.Anything).Return(UserInfo{Site: "hdsky", Username: "u1"}, nil)

	s2 := &MockSite{}
	s2.On("ID").Return("mteam")
	s2.On("GetUserInfo", mock.Anything).Return(UserInfo{Site: "mteam", Username: "u2"}, nil)

	service.RegisterSite(s1)
	service.RegisterSite(s2)

	results, errs := service.FetchAndSaveAllWithConcurrency(ctx, 2, 5*time.Second)
	assert.Len(t, results, 2)
	assert.Empty(t, errs)
}

func TestUserInfoService_FetchAndSaveAllWithConcurrency_Empty(t *testing.T) {
	service := NewUserInfoService(UserInfoServiceConfig{Logger: zap.NewNop()})
	results, errs := service.FetchAndSaveAllWithConcurrency(context.Background(), 2, time.Second)
	assert.Nil(t, results)
	assert.Nil(t, errs)
}

// ---------------------------------------------------------------------------
// createHDDolbySite — factory path
// ---------------------------------------------------------------------------

func TestCreateHDDolbySite(t *testing.T) {
	opts := HDDolbyOptions{APIKey: "rsskey", Cookie: "c=1"}
	optsBytes, _ := json.Marshal(opts)
	site, err := createHDDolbySite(SiteConfig{
		ID:      "hddolby",
		Name:    "HDDolby",
		BaseURL: "https://www.hddolby.com",
		Options: optsBytes,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "hddolby", site.ID())
}

func TestCreateHDDolbySite_MissingCreds(t *testing.T) {
	_, err := createHDDolbySite(SiteConfig{ID: "hddolby", Name: "HDDolby", BaseURL: "https://x.com"}, zap.NewNop())
	require.Error(t, err)

	opts := HDDolbyOptions{APIKey: "k"}
	optsBytes, _ := json.Marshal(opts)
	_, err = createHDDolbySite(SiteConfig{ID: "hddolby", Name: "HDDolby", BaseURL: "https://x.com", Options: optsBytes}, zap.NewNop())
	require.Error(t, err)
}

func TestCreateHDDolbySite_BadOptions(t *testing.T) {
	_, err := createHDDolbySite(SiteConfig{ID: "hddolby", Options: []byte("notjson")}, zap.NewNop())
	require.Error(t, err)
}
