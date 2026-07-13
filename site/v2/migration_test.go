package v2

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoStartAddAtPausedConversion(t *testing.T) {
	assert.False(t, AutoStartToAddAtPaused(true))
	assert.True(t, AutoStartToAddAtPaused(false))
	assert.True(t, AddAtPausedToAutoStart(false))
	assert.False(t, AddAtPausedToAutoStart(true))
}

func TestNewConfigMigrator_NilLogger(t *testing.T) {
	m := NewConfigMigrator(nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.logger)
}

func TestMigrateDownloaderConfig(t *testing.T) {
	m := NewConfigMigrator(nil)
	newCfg := m.MigrateDownloaderConfig(OldDownloaderConfig{
		Type: "qbittorrent", Name: "qb", URL: "http://localhost", AutoStart: true,
	})
	assert.Equal(t, "qbittorrent", newCfg.Type)
	assert.Equal(t, "qb", newCfg.Name)
	assert.False(t, newCfg.AddAtPaused) // autoStart=true -> addAtPaused=false
}

func TestMigrateDownloaderConfigJSON(t *testing.T) {
	m := NewConfigMigrator(nil)
	out, err := m.MigrateDownloaderConfigJSON([]byte(`{"type":"qbittorrent","name":"qb","autoStart":false}`))
	require.NoError(t, err)
	var cfg NewDownloaderConfig
	require.NoError(t, json.Unmarshal(out, &cfg))
	assert.True(t, cfg.AddAtPaused)

	_, err = m.MigrateDownloaderConfigJSON([]byte(`not json`))
	assert.Error(t, err)
}

func TestMigrateSiteConfig(t *testing.T) {
	m := NewConfigMigrator(nil)

	// nexusphp with cookie
	np := m.MigrateSiteConfig(OldSiteConfig{Name: "hdsky", Type: "nexusphp", URL: "https://hdsky.me", Cookie: "c=1", RateLimit: 2})
	assert.Equal(t, "nexusphp", np.Type)
	assert.Equal(t, "hdsky", np.ID)
	assert.Equal(t, "https://hdsky.me", np.BaseURL)
	assert.InDelta(t, 2.0, np.RateLimit, 0.001)
	var npOpts NexusPHPOptions
	require.NoError(t, json.Unmarshal(np.Options, &npOpts))
	assert.Equal(t, "c=1", npOpts.Cookie)

	// mteam -> mtorrent normalization
	mt := m.MigrateSiteConfig(OldSiteConfig{Name: "mteam", Type: "mteam", APIKey: "key"})
	assert.Equal(t, "mtorrent", mt.Type)
	var mtOpts MTorrentOptions
	require.NoError(t, json.Unmarshal(mt.Options, &mtOpts))
	assert.Equal(t, "key", mtOpts.APIKey)

	// unit3d
	u3 := m.MigrateSiteConfig(OldSiteConfig{Name: "blu", Type: "unit3d", APIKey: "k"})
	assert.Equal(t, "unit3d", u3.Type)

	// gazelle
	gz := m.MigrateSiteConfig(OldSiteConfig{Name: "red", Type: "gazelle", APIKey: "k", Cookie: "c"})
	assert.Equal(t, "gazelle", gz.Type)

	// empty type with apiKey -> inferred mtorrent
	inf := m.MigrateSiteConfig(OldSiteConfig{Name: "x", APIKey: "k"})
	assert.Equal(t, "mtorrent", inf.Type)

	// empty type with cookie -> inferred nexusphp
	inf2 := m.MigrateSiteConfig(OldSiteConfig{Name: "y", Cookie: "c"})
	assert.Equal(t, "nexusphp", inf2.Type)
}

func TestMigrateSiteConfig_SelectorsPtr(t *testing.T) {
	m := NewConfigMigrator(nil)
	sel := &SiteSelectors{TableRows: "table tr"}
	cfg := m.MigrateSiteConfig(OldSiteConfig{Name: "s", Type: "nexusphp", Selectors: sel})
	var opts NexusPHPOptions
	require.NoError(t, json.Unmarshal(cfg.Options, &opts))
	require.NotNil(t, opts.Selectors)
	assert.Equal(t, "table tr", opts.Selectors.TableRows)
}

func TestMigrateSiteConfigJSON(t *testing.T) {
	m := NewConfigMigrator(nil)
	out, err := m.MigrateSiteConfigJSON([]byte(`{"name":"hdsky","type":"nexusphp","url":"https://x.me","cookie":"c"}`))
	require.NoError(t, err)
	var cfg SiteConfig
	require.NoError(t, json.Unmarshal(out, &cfg))
	assert.Equal(t, "hdsky", cfg.ID)

	_, err = m.MigrateSiteConfigJSON([]byte(`bad`))
	assert.Error(t, err)
}

func TestDetectDownloaderConfigVersion(t *testing.T) {
	m := NewConfigMigrator(nil)
	assert.Equal(t, "new", m.DetectDownloaderConfigVersion([]byte(`{"addAtPaused":true}`)))
	assert.Equal(t, "old", m.DetectDownloaderConfigVersion([]byte(`{"autoStart":true}`)))
	assert.Equal(t, "old", m.DetectDownloaderConfigVersion([]byte(`{"auto_start":true}`)))
	assert.Equal(t, "unknown", m.DetectDownloaderConfigVersion([]byte(`{"other":1}`)))
	assert.Equal(t, "unknown", m.DetectDownloaderConfigVersion([]byte(`bad json`)))
}

func TestMigrateDownloaderConfigIfNeeded(t *testing.T) {
	m := NewConfigMigrator(nil)

	// new -> no migration
	out, migrated, err := m.MigrateDownloaderConfigIfNeeded([]byte(`{"addAtPaused":true}`))
	require.NoError(t, err)
	assert.False(t, migrated)
	assert.JSONEq(t, `{"addAtPaused":true}`, string(out))

	// old -> migrated
	out, migrated, err = m.MigrateDownloaderConfigIfNeeded([]byte(`{"type":"qb","name":"n","autoStart":true}`))
	require.NoError(t, err)
	assert.True(t, migrated)
	var cfg NewDownloaderConfig
	require.NoError(t, json.Unmarshal(out, &cfg))
	assert.False(t, cfg.AddAtPaused)

	// unknown -> as-is
	_, migrated, err = m.MigrateDownloaderConfigIfNeeded([]byte(`{"x":1}`))
	require.NoError(t, err)
	assert.False(t, migrated)
}

func TestGetDeprecationWarnings(t *testing.T) {
	warnings := GetDeprecationWarnings()
	assert.Len(t, warnings, 3)
	fields := map[string]bool{}
	for _, w := range warnings {
		fields[w.Field] = true
	}
	assert.True(t, fields["autoStart"])
	assert.True(t, fields["authMethod"])
}

func TestCheckForDeprecatedFields(t *testing.T) {
	m := NewConfigMigrator(nil)
	warnings := m.CheckForDeprecatedFields([]byte(`{"autoStart":true,"authMethod":"cookie"}`))
	assert.Len(t, warnings, 2)

	none := m.CheckForDeprecatedFields([]byte(`{"clean":true}`))
	assert.Empty(t, none)

	bad := m.CheckForDeprecatedFields([]byte(`not json`))
	assert.Nil(t, bad)
}

func TestGetSiteMigrationGuides(t *testing.T) {
	guides := GetSiteMigrationGuides()
	assert.Len(t, guides, 4)
	types := map[string]bool{}
	for _, g := range guides {
		types[g.SiteType] = true
		assert.NotEmpty(t, g.Steps)
	}
	assert.True(t, types["nexusphp"])
	assert.True(t, types["mtorrent"])
	assert.True(t, types["unit3d"])
	assert.True(t, types["gazelle"])
}

func TestSiteAdapter(t *testing.T) {
	newSite := &fakeBatchSite{id: "new"}
	adapter := NewSiteAdapter("old-impl", newSite, true)
	assert.True(t, adapter.IsUsingNewImplementation())
	assert.Equal(t, newSite, adapter.GetSite())

	adapter2 := NewSiteAdapter("old-impl", newSite, false)
	assert.False(t, adapter2.IsUsingNewImplementation())
	assert.Equal(t, "old-impl", adapter2.GetSite())

	adapter3 := NewSiteAdapter("old-impl", nil, true)
	assert.False(t, adapter3.IsUsingNewImplementation())
	assert.Equal(t, "old-impl", adapter3.GetSite())
}

func TestSiteMigrationManager(t *testing.T) {
	m := NewSiteMigrationManager(nil)
	require.NotNil(t, m)

	newSite := &fakeBatchSite{id: "hdsky"}
	m.RegisterSite("hdsky", "old", newSite, false)

	assert.Equal(t, "old", m.GetSite("hdsky"))
	assert.Equal(t, newSite, m.GetNewSite("hdsky"))
	assert.Nil(t, m.GetSite("missing"))
	assert.Nil(t, m.GetNewSite("missing"))

	// migrate to new
	require.NoError(t, m.MigrateToNew("hdsky"))
	assert.Equal(t, newSite, m.GetSite("hdsky"))

	// migrate missing site
	assert.Error(t, m.MigrateToNew("missing"))

	// rollback
	require.NoError(t, m.RollbackToOld("hdsky"))
	assert.Equal(t, "old", m.GetSite("hdsky"))
	assert.Error(t, m.RollbackToOld("missing"))

	// status
	statuses := m.GetMigrationStatus()
	require.Len(t, statuses, 1)
	assert.Equal(t, "hdsky", statuses[0].SiteID)
	assert.True(t, statuses[0].NewImplementation)
	assert.True(t, statuses[0].OldImplementation)
	assert.Contains(t, statuses[0].Notes, "old")
}

func TestSiteMigrationManager_ErrorCases(t *testing.T) {
	m := NewSiteMigrationManager(nil)
	// site with no new impl
	m.RegisterSite("x", "old", nil, false)
	assert.Error(t, m.MigrateToNew("x"))

	// site with no old impl
	newSite := &fakeBatchSite{id: "y"}
	m.RegisterSite("y", nil, newSite, true)
	assert.Error(t, m.RollbackToOld("y"))

	statuses := m.GetMigrationStatus()
	assert.Len(t, statuses, 2)
}
