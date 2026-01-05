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
	return func() {
		// cleanup if needed
	}
}

func TestSiteGroupToID(t *testing.T) {
	tests := []struct {
		siteGroup models.SiteGroup
		wantID    string
		wantOK    bool
	}{
		{models.MTEAM, "mteam", true},
		{models.HDSKY, "hdsky", true},
		{models.SpringSunday, "springsunday", true},
		{models.SiteGroup("unknown"), "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.siteGroup), func(t *testing.T) {
			id, ok := SiteGroupToID[tt.siteGroup]
			if ok != tt.wantOK {
				t.Errorf("SiteGroupToID[%s] ok = %v, want %v", tt.siteGroup, ok, tt.wantOK)
			}
			if ok && id != tt.wantID {
				t.Errorf("SiteGroupToID[%s] = %v, want %v", tt.siteGroup, id, tt.wantID)
			}
		})
	}
}

func TestIDToSiteGroup(t *testing.T) {
	tests := []struct {
		id        string
		wantGroup models.SiteGroup
		wantOK    bool
	}{
		{"mteam", models.MTEAM, true},
		{"hdsky", models.HDSKY, true},
		{"springsunday", models.SpringSunday, true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			group, ok := IDToSiteGroup[tt.id]
			if ok != tt.wantOK {
				t.Errorf("IDToSiteGroup[%s] ok = %v, want %v", tt.id, ok, tt.wantOK)
			}
			if ok && group != tt.wantGroup {
				t.Errorf("IDToSiteGroup[%s] = %v, want %v", tt.id, group, tt.wantGroup)
			}
		})
	}
}

func TestSiteGroupToKind(t *testing.T) {
	tests := []struct {
		siteGroup models.SiteGroup
		wantKind  v2.SiteKind
		wantOK    bool
	}{
		{models.MTEAM, v2.SiteMTorrent, true},
		{models.HDSKY, v2.SiteNexusPHP, true},
		{models.SpringSunday, v2.SiteNexusPHP, true},
		{models.SiteGroup("unknown"), "", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.siteGroup), func(t *testing.T) {
			kind, ok := SiteGroupToKind[tt.siteGroup]
			if ok != tt.wantOK {
				t.Errorf("SiteGroupToKind[%s] ok = %v, want %v", tt.siteGroup, ok, tt.wantOK)
			}
			if ok && kind != tt.wantKind {
				t.Errorf("SiteGroupToKind[%s] = %v, want %v", tt.siteGroup, kind, tt.wantKind)
			}
		})
	}
}

func TestGetAllSupportedSiteGroups(t *testing.T) {
	groups := GetAllSupportedSiteGroups()

	if len(groups) != 3 {
		t.Errorf("GetAllSupportedSiteGroups() returned %d groups, want 3", len(groups))
	}

	// Check that all expected groups are present
	expected := map[models.SiteGroup]bool{
		models.MTEAM:        false,
		models.HDSKY:        false,
		models.SpringSunday: false,
	}

	for _, g := range groups {
		if _, ok := expected[g]; ok {
			expected[g] = true
		} else {
			t.Errorf("Unexpected site group: %s", g)
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
		{models.MTEAM, true},
		{models.HDSKY, true},
		{models.SpringSunday, true},
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

			// Verify site kind is set correctly
			expectedKind := SiteGroupToKind[siteGroup]
			if impl.siteKind != expectedKind {
				t.Errorf("siteKind = %v, want %v", impl.siteKind, expectedKind)
			}

			// Verify site ID is set correctly
			expectedID := SiteGroupToID[siteGroup]
			if impl.siteID != expectedID {
				t.Errorf("siteID = %v, want %v", impl.siteID, expectedID)
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
