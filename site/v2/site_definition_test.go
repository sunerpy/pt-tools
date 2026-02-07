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
		{"123site", true},
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
