package definitions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// SiteTestConfig holds configuration for testing a site
type SiteTestConfig struct {
	SiteID     string   // Site ID (e.g., "hdsky", "springsunday", "hddolby")
	Aliases    []string // Aliases for environment variables (e.g., "cmct" for springsunday)
	BaseURL    string
	EnvVarName string // Environment variable name for cookie (e.g., "HDSKY_COOKIE")
}

// GetAllSiteTestConfigs returns test configurations for all supported sites
func GetAllSiteTestConfigs() []SiteTestConfig {
	return []SiteTestConfig{
		{
			SiteID:     "hdsky",
			Aliases:    []string{"hdsky"},
			BaseURL:    "https://hdsky.me",
			EnvVarName: "HDSKY_COOKIE",
		},
		{
			SiteID:     "springsunday",
			Aliases:    []string{"springsunday", "ssd"},
			BaseURL:    "https://springsunday.net",
			EnvVarName: "SPRINGSUNDAY_COOKIE",
		},
		{
			SiteID:     "hddolby",
			Aliases:    []string{"hddolby"},
			BaseURL:    "https://www.hddolby.com",
			EnvVarName: "HDDOLBY_COOKIE",
		},
		{
			SiteID:     "ourbits",
			Aliases:    []string{"ourbits", "ob"},
			BaseURL:    "https://ourbits.club",
			EnvVarName: "OURBITS_COOKIE",
		},
		{
			SiteID:     "ttg",
			Aliases:    []string{"ttg"},
			BaseURL:    "https://totheglory.im",
			EnvVarName: "TTG_COOKIE",
		},
	}
}

// findSiteConfig finds site config by ID or alias
func findSiteConfig(idOrAlias string) *SiteTestConfig {
	idOrAlias = strings.ToLower(idOrAlias)
	for _, cfg := range GetAllSiteTestConfigs() {
		if strings.ToLower(cfg.SiteID) == idOrAlias {
			return &cfg
		}
		for _, alias := range cfg.Aliases {
			if strings.ToLower(alias) == idOrAlias {
				return &cfg
			}
		}
	}
	return nil
}

// TestAllSitesUserInfoLive runs user info test for all sites with available cookies
// Run with: go test -v -run TestAllSitesUserInfoLive ./site/v2/definitions/
func TestAllSitesUserInfoLive(t *testing.T) {
	for _, cfg := range GetAllSiteTestConfigs() {
		t.Run(cfg.SiteID, func(t *testing.T) {
			cookie := os.Getenv(cfg.EnvVarName)
			if cookie == "" {
				t.Skipf("%s environment variable not set, skipping", cfg.EnvVarName)
			}
			testSiteUserInfo(t, cfg, cookie, false)
		})
	}
}

// TestAllSitesUserInfoDebug runs user info test with debug output for all sites
// Run with: go test -v -run TestAllSitesUserInfoDebug ./site/v2/definitions/
func TestAllSitesUserInfoDebug(t *testing.T) {
	for _, cfg := range GetAllSiteTestConfigs() {
		t.Run(cfg.SiteID, func(t *testing.T) {
			cookie := os.Getenv(cfg.EnvVarName)
			if cookie == "" {
				t.Skipf("%s environment variable not set, skipping", cfg.EnvVarName)
			}
			testSiteUserInfo(t, cfg, cookie, true)
		})
	}
}

// testSiteUserInfo is the common test function for all sites
func testSiteUserInfo(t *testing.T, cfg SiteTestConfig, cookie string, debug bool) {
	if debug {
		v2.DebugUserInfo = true
		defer func() { v2.DebugUserInfo = false }()
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	factory := v2.NewSiteFactory(logger)
	site, err := factory.CreateSite(v2.SiteConfig{
		Type:    "nexusphp",
		ID:      cfg.SiteID,
		Name:    cfg.SiteID,
		BaseURL: cfg.BaseURL,
		Options: []byte(fmt.Sprintf(`{"cookie":"%s"}`, cookie)),
	})
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := site.GetUserInfo(ctx)
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}

	// Print all fields
	printUserInfo(t, cfg.SiteID, info)

	// Basic assertions
	assert.NotEmpty(t, info.UserID, "UserID should not be empty")
	assert.NotEmpty(t, info.Username, "Username should not be empty")
}

// printUserInfo prints user info in a formatted way
func printUserInfo(t *testing.T, siteID string, info v2.UserInfo) {
	t.Logf("\n=== %s User Info ===", strings.ToUpper(siteID))
	t.Logf("Site:            %s", info.Site)
	t.Logf("UserID:          %s", info.UserID)
	t.Logf("Username:        %s", info.Username)
	t.Logf("Uploaded:        %d bytes (%.2f TB)", info.Uploaded, float64(info.Uploaded)/(1024*1024*1024*1024))
	t.Logf("Downloaded:      %d bytes (%.2f TB)", info.Downloaded, float64(info.Downloaded)/(1024*1024*1024*1024))
	t.Logf("Ratio:           %.3f", info.Ratio)
	t.Logf("Bonus:           %.2f", info.Bonus)
	t.Logf("BonusPerHour:    %.4f", info.BonusPerHour)
	t.Logf("SeedingBonus:    %.2f", info.SeedingBonus)
	t.Logf("Rank/LevelName:  %s", info.Rank)
	t.Logf("Seeding:         %d", info.Seeding)
	t.Logf("Leeching:        %d", info.Leeching)
	t.Logf("SeederCount:     %d", info.SeederCount)
	t.Logf("SeederSize:      %d bytes (%.2f TB)", info.SeederSize, float64(info.SeederSize)/(1024*1024*1024*1024))
	t.Logf("LeecherCount:    %d", info.LeecherCount)
	t.Logf("LeecherSize:     %d bytes (%.2f GB)", info.LeecherSize, float64(info.LeecherSize)/(1024*1024*1024))
	if info.JoinDate > 0 {
		t.Logf("JoinDate:        %d (%s)", info.JoinDate, time.Unix(info.JoinDate, 0).Format("2006-01-02"))
	} else {
		t.Logf("JoinDate:        (not set)")
	}
	t.Logf("LastUpdate:      %d", info.LastUpdate)
	t.Logf("UnreadMessages:  %d", info.UnreadMessageCount)
	t.Logf("HnRUnsatisfied:  %d", info.HnRUnsatisfied)
	t.Logf("HnRPreWarning:   %d", info.HnRPreWarning)
}

// Individual site tests with debug - for running specific site tests

// TestHDSkyUserInfoLive tests HDSky user info fetching
// Run with: HDSKY_COOKIE="your_cookie" go test -v -run TestHDSkyUserInfoLive ./site/v2/definitions/
func TestHDSkyUserInfoLive(t *testing.T) {
	cookie := os.Getenv("HDSKY_COOKIE")
	if cookie == "" {
		t.Skip("HDSKY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("hdsky")
	testSiteUserInfo(t, *cfg, cookie, false)
}

// TestHDSkyUserInfoDebug tests HDSky with debug output
// Run with: HDSKY_COOKIE="your_cookie" go test -v -run TestHDSkyUserInfoDebug ./site/v2/definitions/
func TestHDSkyUserInfoDebug(t *testing.T) {
	cookie := os.Getenv("HDSKY_COOKIE")
	if cookie == "" {
		t.Skip("HDSKY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("hdsky")
	testSiteUserInfo(t, *cfg, cookie, true)
}

// TestSpringSundayUserInfoLive tests SpringSunday user info fetching
// Run with: SPRINGSUNDAY_COOKIE="your_cookie" go test -v -run TestSpringSundayUserInfoLive ./site/v2/definitions/
func TestSpringSundayUserInfoLive(t *testing.T) {
	cookie := os.Getenv("SPRINGSUNDAY_COOKIE")
	if cookie == "" {
		t.Skip("SPRINGSUNDAY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("springsunday")
	testSiteUserInfo(t, *cfg, cookie, false)
}

// TestSpringSundayUserInfoDebug tests SpringSunday with debug output
// Run with: SPRINGSUNDAY_COOKIE="your_cookie" go test -v -run TestSpringSundayUserInfoDebug ./site/v2/definitions/
func TestSpringSundayUserInfoDebug(t *testing.T) {
	cookie := os.Getenv("SPRINGSUNDAY_COOKIE")
	if cookie == "" {
		t.Skip("SPRINGSUNDAY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("springsunday")
	testSiteUserInfo(t, *cfg, cookie, true)
}

// TestHDDolbyUserInfoLive tests HDDolby user info fetching
// Run with: HDDOLBY_COOKIE="your_cookie" go test -v -run TestHDDolbyUserInfoLive ./site/v2/definitions/
func TestHDDolbyUserInfoLive(t *testing.T) {
	cookie := os.Getenv("HDDOLBY_COOKIE")
	if cookie == "" {
		t.Skip("HDDOLBY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("hddolby")
	testSiteUserInfo(t, *cfg, cookie, false)
}

// TestHDDolbyUserInfoDebug tests HDDolby with debug output
// Run with: HDDOLBY_COOKIE="your_cookie" go test -v -run TestHDDolbyUserInfoDebug ./site/v2/definitions/
func TestHDDolbyUserInfoDebug(t *testing.T) {
	cookie := os.Getenv("HDDOLBY_COOKIE")
	if cookie == "" {
		t.Skip("HDDOLBY_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("hddolby")
	testSiteUserInfo(t, *cfg, cookie, true)
}

// TestOurBitsUserInfoLive tests OurBits user info fetching
// Run with: OURBITS_COOKIE="your_cookie" go test -v -run TestOurBitsUserInfoLive ./site/v2/definitions/
func TestOurBitsUserInfoLive(t *testing.T) {
	cookie := os.Getenv("OURBITS_COOKIE")
	if cookie == "" {
		t.Skip("OURBITS_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("ourbits")
	testSiteUserInfo(t, *cfg, cookie, false)
}

// TestOurBitsUserInfoDebug tests OurBits with debug output
// Run with: OURBITS_COOKIE="your_cookie" go test -v -run TestOurBitsUserInfoDebug ./site/v2/definitions/
func TestOurBitsUserInfoDebug(t *testing.T) {
	cookie := os.Getenv("OURBITS_COOKIE")
	if cookie == "" {
		t.Skip("OURBITS_COOKIE environment variable not set, skipping live test")
	}
	cfg := findSiteConfig("ourbits")
	testSiteUserInfo(t, *cfg, cookie, true)
}

// TestSiteWithHTMLDump tests a site and dumps HTML responses for debugging
// First run TestSiteWithHTMLDump, then use TestSiteSelectorDebug to debug selectors
// Run with: SITE_COOKIE="your_cookie" SITE_ID="cmct" go test -v -run TestSiteWithHTMLDump ./site/v2/definitions/
func TestSiteWithHTMLDump(t *testing.T) {
	siteID := os.Getenv("SITE_ID")
	if siteID == "" {
		t.Skip("SITE_ID environment variable not set")
	}

	cookie := os.Getenv("SITE_COOKIE")
	if cookie == "" {
		t.Skip("SITE_COOKIE environment variable not set")
	}

	// Find site config
	cfg := findSiteConfig(siteID)
	if cfg == nil {
		t.Fatalf("Unknown site ID: %s. Available: hdsky, cmct/springsunday, hddolby, ourbits", siteID)
		return
	}

	// Enable debug
	v2.DebugUserInfo = true
	defer func() { v2.DebugUserInfo = false }()

	// Get site definition
	def := v2.GetDefinitionRegistry().GetOrDefault(cfg.SiteID)
	if def == nil {
		t.Fatalf("Site definition %s not found", cfg.SiteID)
		return
	}

	// Create driver
	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL: cfg.BaseURL,
		Cookie:  cookie,
	})
	driver.SetSiteDefinition(def)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Dump each page
	if def.UserInfo != nil {
		for i, process := range def.UserInfo.Process {
			t.Logf("\n=== Process %d: %s ===", i, process.RequestConfig.URL)

			req := v2.NexusPHPRequest{
				Path:   process.RequestConfig.URL,
				Method: "GET",
			}

			res, err := driver.Execute(ctx, req)
			if err != nil {
				t.Logf("Error fetching %s: %v", process.RequestConfig.URL, err)
				continue
			}

			// Save HTML
			filename := fmt.Sprintf("/tmp/%s_page%d.html", cfg.SiteID, i)
			os.WriteFile(filename, res.RawBody, 0o644)
			t.Logf("Saved response to %s (%d bytes)", filename, len(res.RawBody))
		}
	}

	// Now get full user info
	t.Log("\n=== Full User Info ===")
	info, err := driver.GetUserInfo(ctx)
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}
	printUserInfo(t, cfg.SiteID, info)
}

// TestSiteSelectorDebug tests selectors against saved HTML files
// First run TestSiteWithHTMLDump, then use this to debug selectors
// Run with: SITE_ID="cmct" go test -v -run TestSiteSelectorDebug ./site/v2/definitions/
func TestSiteSelectorDebug(t *testing.T) {
	siteID := os.Getenv("SITE_ID")
	if siteID == "" {
		t.Skip("SITE_ID environment variable not set")
	}

	// Find site config
	cfg := findSiteConfig(siteID)
	if cfg == nil {
		t.Fatalf("Unknown site ID: %s", siteID)
		return
	}

	// Get site definition
	def := v2.GetDefinitionRegistry().GetOrDefault(cfg.SiteID)
	if def == nil || def.UserInfo == nil {
		t.Fatalf("Site definition %s not found or has no UserInfo config", cfg.SiteID)
	}

	// Create driver for selector testing
	driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{
		BaseURL: cfg.BaseURL,
		Cookie:  "dummy",
	})
	driver.SetSiteDefinition(def)

	// Test selectors on each saved page
	for i, process := range def.UserInfo.Process {
		filename := fmt.Sprintf("/tmp/%s_page%d.html", cfg.SiteID, i)
		htmlData, err := os.ReadFile(filename)
		if err != nil {
			t.Logf("Skipping page %d: %v", i, err)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlData)))
		if err != nil {
			t.Logf("Failed to parse %s: %v", filename, err)
			continue
		}

		t.Logf("\n=== Testing selectors on page %d (%s) ===", i, process.RequestConfig.URL)
		t.Logf("Fields in this process: %v", process.Fields)

		for _, fieldName := range process.Fields {
			selector, ok := def.UserInfo.Selectors[fieldName]
			if !ok {
				t.Logf("⚠ %s: no selector defined", fieldName)
				continue
			}

			value := driver.ExtractFieldValuePublic(doc, selector)
			if value != "" {
				displayVal := value
				if len(displayVal) > 80 {
					displayVal = displayVal[:80] + "..."
				}
				t.Logf("✓ %s: %s", fieldName, displayVal)
			} else {
				t.Logf("✗ %s: NOT FOUND (selectors: %v)", fieldName, selector.Selector)
			}
		}
	}
}

// TestPrintSiteDefinition prints the site definition for inspection
// Run with: SITE_ID="cmct" go test -v -run TestPrintSiteDefinition ./site/v2/definitions/
func TestPrintSiteDefinition(t *testing.T) {
	siteID := os.Getenv("SITE_ID")
	if siteID == "" {
		// Print all available definitions
		t.Log("Available site definitions:")
		for _, id := range v2.GetDefinitionRegistry().List() {
			t.Logf("  - %s", id)
		}
		t.Log("\nAvailable site aliases for testing:")
		for _, cfg := range GetAllSiteTestConfigs() {
			t.Logf("  - %s (aliases: %v, env: %s)", cfg.SiteID, cfg.Aliases, cfg.EnvVarName)
		}
		t.Skip("Set SITE_ID to print a specific definition (e.g., SITE_ID=cmct)")
	}

	// Find site config
	cfg := findSiteConfig(siteID)
	if cfg == nil {
		t.Fatalf("Unknown site ID: %s", siteID)
		return
	}

	def := v2.GetDefinitionRegistry().GetOrDefault(cfg.SiteID)
	if def == nil {
		t.Fatalf("Site definition %s not found", cfg.SiteID)
		return
	}

	t.Logf("\n=== Site Definition: %s ===", def.ID)
	t.Logf("Name: %s", def.Name)
	t.Logf("Aka: %v", def.Aka)
	t.Logf("Schema: %s", def.Schema)
	t.Logf("URLs: %v", def.URLs)

	if def.UserInfo != nil {
		t.Logf("\nUserInfo Config:")
		t.Logf("  PickLast: %v", def.UserInfo.PickLast)
		t.Logf("  RequestDelay: %d", def.UserInfo.RequestDelay)

		t.Logf("\nProcess Steps:")
		for i, p := range def.UserInfo.Process {
			t.Logf("  [%d] URL: %s", i, p.RequestConfig.URL)
			t.Logf("      Fields: %v", p.Fields)
			if len(p.Assertion) > 0 {
				t.Logf("      Assertion: %v", p.Assertion)
			}
		}

		t.Logf("\nSelectors:")
		for name, sel := range def.UserInfo.Selectors {
			t.Logf("  %s:", name)
			t.Logf("    Selectors: %v", sel.Selector)
			if sel.Attr != "" {
				t.Logf("    Attr: %s", sel.Attr)
			}
			if sel.Text != "" {
				t.Logf("    DefaultText: %s", sel.Text)
			}
			if len(sel.Filters) > 0 {
				filterNames := make([]string, len(sel.Filters))
				for i, f := range sel.Filters {
					filterNames[i] = f.Name
				}
				t.Logf("    Filters: %v", filterNames)
			}
		}
	}
}
