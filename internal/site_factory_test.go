package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func setupFactoryTestDB(t *testing.T) func() {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	return func() {}
}

func TestGetAllSupportedSiteGroups(t *testing.T) {
	groups := GetAllSupportedSiteGroups()

	if len(groups) < 4 {
		t.Errorf("GetAllSupportedSiteGroups() returned %d groups, want at least 4", len(groups))
	}

	expected := map[models.SiteGroup]bool{
		models.SiteGroup("mteam"):        false,
		models.SiteGroup("hdsky"):        false,
		models.SiteGroup("springsunday"): false,
		models.SiteGroup("hddolby"):      false,
	}

	for _, g := range groups {
		if _, ok := expected[g]; ok {
			expected[g] = true
		}
	}

	for g, found := range expected {
		if !found {
			t.Errorf("Expected site group %s not found", g)
		}
	}
}

func TestIsSiteGroupSupported(t *testing.T) {
	tests := []struct {
		siteGroup models.SiteGroup
		want      bool
	}{
		{models.SiteGroup("mteam"), true},
		{models.SiteGroup("hdsky"), true},
		{models.SiteGroup("springsunday"), true},
		{models.SiteGroup("hddolby"), true}, // HDDolby is now available via API
		{models.SiteGroup("unknown"), false},
		{models.SiteGroup(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.siteGroup), func(t *testing.T) {
			if got := IsSiteGroupSupported(tt.siteGroup); got != tt.want {
				t.Errorf("IsSiteGroupSupported(%s) = %v, want %v", tt.siteGroup, got, tt.want)
			}
		})
	}
}

func TestNewUnifiedSiteImpl_AllSupportedSites(t *testing.T) {
	cleanup := setupFactoryTestDB(t)
	defer cleanup()

	registry := v2.GetGlobalSiteRegistry()

	for _, siteGroup := range GetAllSupportedSiteGroups() {
		t.Run(string(siteGroup), func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), siteGroup)
			if err != nil {
				t.Fatalf("NewUnifiedSiteImpl(%s) error = %v", siteGroup, err)
			}
			if impl == nil {
				t.Fatal("NewUnifiedSiteImpl returned nil")
			}
			if impl.SiteGroup() != siteGroup {
				t.Errorf("SiteGroup() = %v, want %v", impl.SiteGroup(), siteGroup)
			}

			meta, ok := registry.Get(string(siteGroup))
			if !ok {
				t.Fatalf("Registry.Get(%s) failed", siteGroup)
			}
			if impl.siteKind != meta.Kind {
				t.Errorf("siteKind = %v, want %v", impl.siteKind, meta.Kind)
			}
			if impl.siteID != string(siteGroup) {
				t.Errorf("siteID = %v, want %v", impl.siteID, string(siteGroup))
			}
		})
	}
}

func TestNewUnifiedSiteImpl_UnsupportedSite(t *testing.T) {
	cleanup := setupFactoryTestDB(t)
	defer cleanup()

	unsupportedGroups := []models.SiteGroup{
		models.SiteGroup("unknown"),
		models.SiteGroup("invalid"),
		models.SiteGroup(""),
	}

	for _, siteGroup := range unsupportedGroups {
		t.Run(string(siteGroup), func(t *testing.T) {
			impl, err := NewUnifiedSiteImpl(context.Background(), siteGroup)
			if err == nil {
				t.Errorf("NewUnifiedSiteImpl(%s) expected error, got nil", siteGroup)
			}
			if impl != nil {
				t.Errorf("NewUnifiedSiteImpl(%s) expected nil impl, got %v", siteGroup, impl)
			}
		})
	}
}
