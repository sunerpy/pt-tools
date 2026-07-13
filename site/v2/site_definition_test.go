package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidate_MinimalValid(t *testing.T) {
	tests := []struct {
		name string
		def  SiteDefinition
	}{
		{
			name: "NexusPHP minimal",
			def: SiteDefinition{
				ID:     "testsite",
				Name:   "Test Site",
				Schema: SchemaNexusPHP,
				URLs:   []string{"https://example.com/"},
				Selectors: &SiteSelectors{
					TableRows: "tr",
					Title:     "a.title",
					TitleLink: "a.title",
				},
				UserInfo: &UserInfoConfig{
					Process: []UserInfoProcess{
						{
							RequestConfig: RequestConfig{URL: "/index.php"},
							Fields:        []string{"id"},
						},
					},
					Selectors: map[string]FieldSelector{
						"id": {Selector: []string{"a[href]"}},
					},
				},
			},
		},
		{
			name: "mTorrent minimal",
			def: SiteDefinition{
				ID:     "mtorrent-test",
				Name:   "MTorrent Test",
				Schema: SchemaMTorrent,
				URLs:   []string{"https://api.example.com"},
				UserInfo: &UserInfoConfig{
					Process: []UserInfoProcess{
						{
							RequestConfig: RequestConfig{URL: "/api/profile", Method: "POST"},
							Fields:        []string{"id"},
						},
					},
					Selectors: map[string]FieldSelector{
						"id": {Selector: []string{"data.id"}},
					},
				},
			},
		},
		{
			name: "Rousi with CreateDriver",
			def: SiteDefinition{
				ID:           "rousi-test",
				Name:         "Rousi Test",
				Schema:       SchemaRousi,
				URLs:         []string{"https://rousi.example.com"},
				CreateDriver: func(config SiteConfig, logger *zap.Logger) (Site, error) { return nil, nil },
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestValidate_RequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		def         SiteDefinition
		expectRules []string
	}{
		{
			name:        "empty definition",
			def:         SiteDefinition{},
			expectRules: []string{"Required", "Required", "InvalidValue", "Required"},
		},
		{
			name: "missing Name",
			def: SiteDefinition{
				ID:     "test",
				Schema: SchemaNexusPHP,
				URLs:   []string{"https://example.com/"},
				Selectors: &SiteSelectors{
					TableRows: "tr", Title: "a", TitleLink: "a",
				},
				UserInfo: &UserInfoConfig{
					Process:   []UserInfoProcess{{RequestConfig: RequestConfig{URL: "/index.php"}, Fields: []string{"id"}}},
					Selectors: map[string]FieldSelector{"id": {Selector: []string{"a"}}},
				},
			},
			expectRules: []string{"Required"},
		},
		{
			name: "invalid Schema",
			def: SiteDefinition{
				ID:     "test",
				Name:   "Test",
				Schema: "InvalidSchema",
				URLs:   []string{"https://example.com/"},
			},
			expectRules: []string{"InvalidValue"},
		},
		{
			name: "empty URLs",
			def: SiteDefinition{
				ID:     "test",
				Name:   "Test",
				Schema: SchemaMTorrent,
				URLs:   []string{},
				UserInfo: &UserInfoConfig{
					Process:   []UserInfoProcess{{RequestConfig: RequestConfig{URL: "/api"}, Fields: []string{"id"}}},
					Selectors: map[string]FieldSelector{"id": {Selector: []string{"data.id"}}},
				},
			},
			expectRules: []string{"Required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			require.Error(t, err)
			valErrs, ok := err.(ValidationErrors)
			require.True(t, ok, "expected ValidationErrors type")
			for _, expectedRule := range tt.expectRules {
				found := false
				for _, ve := range valErrs {
					if ve.Rule == expectedRule {
						found = true
						break
					}
				}
				assert.True(t, found, "expected rule %q in errors: %s", expectedRule, err)
			}
		})
	}
}

func TestValidate_IDFormat(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"hdsky", false},
		{"my-site", false},
		{"site_v2", false},
		{"a", false},
		{"HDSKY", true},
		{"123site", false},
		{"-invalid", true},
		{"has space", true},
		{"has.dot", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			def := makeMinimalNexusPHP(tt.id)
			err := def.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "ID")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_URLFormat(t *testing.T) {
	tests := []struct {
		url     string
		wantErr bool
	}{
		{"https://example.com/", false},
		{"https://example.com", false},
		{"http://example.com/", false},
		{"ftp://example.com/", true},
		{"example.com", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			def := makeMinimalNexusPHP("test")
			if tt.url == "" {
				def.URLs = []string{}
			} else {
				def.URLs = []string{tt.url}
			}
			err := def.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_TimezoneFormat(t *testing.T) {
	tests := []struct {
		tz      string
		wantErr bool
	}{
		{"", false},
		{"+0800", false},
		{"-0500", false},
		{"+08:00", true},
		{"UTC", true},
		{"0800", true},
	}

	for _, tt := range tests {
		t.Run(tt.tz, func(t *testing.T) {
			def := makeMinimalNexusPHP("test")
			def.TimezoneOffset = tt.tz
			err := def.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "TimezoneOffset")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_NexusPHPRequiresSelectors(t *testing.T) {
	def := &SiteDefinition{
		ID:     "test",
		Name:   "Test",
		Schema: SchemaNexusPHP,
		URLs:   []string{"https://example.com/"},
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Selectors")
	assert.Contains(t, err.Error(), "UserInfo")
}

func TestValidate_RousiRequiresCreateDriver(t *testing.T) {
	def := &SiteDefinition{
		ID:     "test",
		Name:   "Test",
		Schema: SchemaRousi,
		URLs:   []string{"https://example.com/"},
	}
	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CreateDriver")
}

func TestValidate_UserInfoFieldSelectorConsistency(t *testing.T) {
	def := makeMinimalNexusPHP("test")
	def.UserInfo.Process[0].Fields = []string{"id", "name", "missing_field"}

	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing_field")
}

func TestValidate_UserInfoAssertionReference(t *testing.T) {
	def := makeMinimalNexusPHP("test")
	def.UserInfo.Process = []UserInfoProcess{
		{
			RequestConfig: RequestConfig{URL: "/page1"},
			Fields:        []string{"name"},
		},
		{
			RequestConfig: RequestConfig{URL: "/page2"},
			Assertion:     map[string]string{"id": "params.id"},
			Fields:        []string{"uploaded"},
		},
	}
	def.UserInfo.Selectors = map[string]FieldSelector{
		"name":     {Selector: []string{"a"}},
		"uploaded": {Selector: []string{"td"}},
	}

	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "params.id")
}

func TestValidate_LevelRequirements(t *testing.T) {
	def := makeMinimalNexusPHP("test")
	def.LevelRequirements = []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 1, Name: "Power User", Downloaded: "200GB", Interval: "P5W"},
	}

	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Duplicate")
}

func TestValidate_LevelRequirementsSizeFormat(t *testing.T) {
	def := makeMinimalNexusPHP("test")
	def.LevelRequirements = []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "invalid-size", Interval: "P5W"},
	}

	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-size")
}

func TestValidate_UnavailableRequiresReason(t *testing.T) {
	def := makeMinimalNexusPHP("test")
	def.Unavailable = true

	err := def.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UnavailableReason")
}

func makeMinimalNexusPHP(id string) *SiteDefinition {
	return &SiteDefinition{
		ID:     id,
		Name:   "Test Site",
		Schema: SchemaNexusPHP,
		URLs:   []string{"https://example.com/"},
		Selectors: &SiteSelectors{
			TableRows: "tr",
			Title:     "a.title",
			TitleLink: "a.title",
		},
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/index.php"},
					Fields:        []string{"id"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id": {Selector: []string{"a[href]"}},
			},
		},
	}
}

func TestCalcHRSeedTimeH(t *testing.T) {
	def := &SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 504, // fallback
		HRSeedTimeRules: []HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 204},   // 0-10 GiB
			{MinSizeGB: 10, MaxSizeGB: 20, SeedTimeH: 240},  // 10-20 GiB
			{MinSizeGB: 20, MaxSizeGB: 50, SeedTimeH: 288},  // 20-50 GiB
			{MinSizeGB: 50, MaxSizeGB: 200, SeedTimeH: 336}, // 50-200 GiB
			{MinSizeGB: 200, MaxSizeGB: 0, SeedTimeH: 504},  // 200+ GiB
		},
	}

	tests := []struct {
		name      string
		sizeBytes int64
		expected  int
	}{
		{"5 GiB torrent", 5 * 1024 * 1024 * 1024, 204},
		{"10 GiB boundary (falls into 10-20)", 10 * 1024 * 1024 * 1024, 240},
		{"15 GiB torrent", 15 * 1024 * 1024 * 1024, 240},
		{"25 GiB torrent", 25 * 1024 * 1024 * 1024, 288},
		{"100 GiB torrent", 100 * 1024 * 1024 * 1024, 336},
		{"200 GiB boundary (falls into 200+)", 200 * 1024 * 1024 * 1024, 504},
		{"500 GiB torrent", 500 * 1024 * 1024 * 1024, 504},
		{"0 bytes falls back", 0, 504},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := def.CalcHRSeedTimeH(tt.sizeBytes)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCalcHRSeedTimeH_NoRules(t *testing.T) {
	def := &SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 72,
	}
	assert.Equal(t, 72, def.CalcHRSeedTimeH(5*1024*1024*1024))
	assert.Equal(t, 72, def.CalcHRSeedTimeH(0))
}

func TestCalcHRSeedTimeH_EmptyRulesFallback(t *testing.T) {
	def := &SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 100,
		HRSeedTimeRules: []HRSeedTimeRule{},
	}
	assert.Equal(t, 100, def.CalcHRSeedTimeH(50*1024*1024*1024))
}

func TestValidate_HRSeedTimeRules(t *testing.T) {
	t.Run("valid rules", func(t *testing.T) {
		def := SiteDefinition{
			ID: "test", Name: "Test", Schema: SchemaGazelle,
			URLs:      []string{"https://example.com/"},
			HREnabled: true,
			HRSeedTimeRules: []HRSeedTimeRule{
				{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 36},
				{MinSizeGB: 10, MaxSizeGB: 0, SeedTimeH: 72},
			},
		}
		err := def.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid range", func(t *testing.T) {
		def := SiteDefinition{
			ID: "test", Name: "Test", Schema: SchemaGazelle,
			URLs:      []string{"https://example.com/"},
			HREnabled: true,
			HRSeedTimeRules: []HRSeedTimeRule{
				{MinSizeGB: 50, MaxSizeGB: 10, SeedTimeH: 36},
			},
		}
		err := def.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "InvalidRange")
	})

	t.Run("zero seed time", func(t *testing.T) {
		def := SiteDefinition{
			ID: "test", Name: "Test", Schema: SchemaGazelle,
			URLs:      []string{"https://example.com/"},
			HREnabled: true,
			HRSeedTimeRules: []HRSeedTimeRule{
				{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 0},
			},
		}
		err := def.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SeedTimeH")
	})

	t.Run("rules without HREnabled", func(t *testing.T) {
		def := SiteDefinition{
			ID: "test", Name: "Test", Schema: SchemaGazelle,
			URLs:      []string{"https://example.com/"},
			HREnabled: false,
			HRSeedTimeRules: []HRSeedTimeRule{
				{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 36},
			},
		}
		err := def.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Consistency")
	})
}

func TestCalcHRSeedTimeH_CustomFunc(t *testing.T) {
	// Custom function takes highest priority
	def := &SiteDefinition{
		HREnabled:       true,
		HRSeedTimeHours: 999,
		HRSeedTimeRules: []HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 888},
		},
		HRCalcSeedTime: func(sizeBytes int64) int {
			if sizeBytes < 1024*1024*1024 {
				return 24
			}
			return 48
		},
	}
	assert.Equal(t, 24, def.CalcHRSeedTimeH(500*1024*1024), "custom func should override rules")
	assert.Equal(t, 48, def.CalcHRSeedTimeH(5*1024*1024*1024), "custom func should override rules")
	assert.Equal(t, 24, def.CalcHRSeedTimeH(0), "custom func handles 0 bytes too")
}

func TestNewSizeTieredHRCalc(t *testing.T) {
	calc := NewSizeTieredHRCalc(
		[]HRSeedTimeRule{
			{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 36},
			{MinSizeGB: 10, MaxSizeGB: 20, SeedTimeH: 72},
			{MinSizeGB: 20, MaxSizeGB: 50, SeedTimeH: 120},
			{MinSizeGB: 50, MaxSizeGB: 200, SeedTimeH: 168},
			{MinSizeGB: 200, MaxSizeGB: 0, SeedTimeH: 336},
		},
		168, // window hours
	)

	tests := []struct {
		name      string
		sizeBytes int64
		expected  int
	}{
		{"5 GiB", 5 * 1024 * 1024 * 1024, 36 + 168},
		{"10 GiB boundary", 10 * 1024 * 1024 * 1024, 72 + 168},
		{"15 GiB", 15 * 1024 * 1024 * 1024, 72 + 168},
		{"30 GiB", 30 * 1024 * 1024 * 1024, 120 + 168},
		{"100 GiB", 100 * 1024 * 1024 * 1024, 168 + 168},
		{"300 GiB", 300 * 1024 * 1024 * 1024, 336 + 168},
		{"0 bytes uses max", 0, 336 + 168},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, calc(tt.sizeBytes))
		})
	}
}

func TestCalcHRSeedTimeH_PriorityChain(t *testing.T) {
	t.Run("custom func > rules > flat", func(t *testing.T) {
		def := &SiteDefinition{
			HREnabled:       true,
			HRSeedTimeHours: 100,
			HRSeedTimeRules: []HRSeedTimeRule{{MinSizeGB: 0, MaxSizeGB: 0, SeedTimeH: 200}},
			HRCalcSeedTime:  func(int64) int { return 300 },
		}
		assert.Equal(t, 300, def.CalcHRSeedTimeH(1024), "custom func wins")
	})

	t.Run("rules > flat when no custom func", func(t *testing.T) {
		def := &SiteDefinition{
			HREnabled:       true,
			HRSeedTimeHours: 100,
			HRSeedTimeRules: []HRSeedTimeRule{{MinSizeGB: 0, MaxSizeGB: 0, SeedTimeH: 200}},
		}
		assert.Equal(t, 200, def.CalcHRSeedTimeH(1024), "rules win over flat")
	})

	t.Run("flat when nothing else", func(t *testing.T) {
		def := &SiteDefinition{
			HREnabled:       true,
			HRSeedTimeHours: 100,
		}
		assert.Equal(t, 100, def.CalcHRSeedTimeH(1024), "flat fallback")
	})
}

func TestNewSizeTieredHRCalc_NoMatchFallback(t *testing.T) {
	rules := []HRSeedTimeRule{
		{MinSizeGB: 0, MaxSizeGB: 10, SeedTimeH: 24},
		{MinSizeGB: 10, MaxSizeGB: 50, SeedTimeH: 48},
		{MinSizeGB: 50, MaxSizeGB: 0, SeedTimeH: 96},
	}
	calc := NewSizeTieredHRCalc(rules, 12)

	// 5 GiB -> tier 1
	assert.Equal(t, 36, calc(5*1024*1024*1024))
	// 20 GiB -> tier 2
	assert.Equal(t, 60, calc(20*1024*1024*1024))
	// 100 GiB -> tier 3
	assert.Equal(t, 108, calc(100*1024*1024*1024))
	// unknown size (<=0) -> max
	assert.Equal(t, 108, calc(0))
}

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

func TestNewSizeTieredHRCalc_NoMatchReturnsMax(t *testing.T) {
	rules := []HRSeedTimeRule{
		{MinSizeGB: 100, MaxSizeGB: 200, SeedTimeH: 48},
	}
	calc := NewSizeTieredHRCalc(rules, 10)
	// 5 GiB matches no rule -> max (48+10)
	assert.Equal(t, 58, calc(5*1024*1024*1024))
}

func TestDefaultDetailParserConfig(t *testing.T) {
	cfg := DefaultDetailParserConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "2006-01-02 15:04:05", cfg.TimeLayout)
	assert.Equal(t, DiscountFree, cfg.DiscountMapping["free"])
	assert.NotEmpty(t, cfg.HRKeywords)
}
