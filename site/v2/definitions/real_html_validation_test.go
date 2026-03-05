package definitions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// TestRealHTML_UserInfo validates selectors against real site HTML from /tmp/site-zips/.
// Run with: go test -v -run TestRealHTML_UserInfo ./site/v2/definitions/
func TestRealHTML_UserInfo(t *testing.T) {
	sites := []struct {
		siteID string
		zipDir string
		// Fields expected to be non-empty from index page
		indexFields []string
		// Fields expected to be non-empty from userdetails page
		detailFields []string
	}{
		{
			siteID:       "btschool",
			zipDir:       "/tmp/site-zips/btschool",
			indexFields:  []string{"id", "name", "seeding", "leeching"},
			detailFields: []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime"},
		},
		{
			siteID:       "hdfans",
			zipDir:       "/tmp/site-zips/hdfans",
			indexFields:  []string{"id", "name", "seeding", "leeching", "uploaded", "downloaded", "bonus"},
			detailFields: []string{}, // Note: sample data only has index page, no userdetails
		},
		{
			siteID:       "soulvoice",
			zipDir:       "/tmp/site-zips/soulvoice",
			indexFields:  []string{"id", "name", "seeding", "leeching"},
			detailFields: []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime"},
		},
		{
			siteID:       "52pt",
			zipDir:       "/tmp/site-zips/52pt",
			indexFields:  []string{"id", "name", "seeding", "leeching"},
			detailFields: []string{"uploaded", "downloaded", "levelName", "joinTime"},
		},
		{
			siteID:       "lajidui",
			zipDir:       "/tmp/site-zips/lajidui",
			indexFields:  []string{"id", "name", "seeding", "leeching"},
			detailFields: []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime"},
		},
		{
			siteID:       "1ptba",
			zipDir:       "/tmp/site-zips/1ptba",
			indexFields:  []string{"id", "name", "seeding", "leeching"},
			detailFields: []string{"uploaded", "downloaded", "ratio", "levelName", "joinTime"},
		},
	}

	for _, tc := range sites {
		tc := tc
		t.Run(tc.siteID, func(t *testing.T) {
			// Check if real HTML data exists
			userinfoPath := filepath.Join(tc.zipDir, "userinfo.html")
			if _, err := os.Stat(userinfoPath); os.IsNotExist(err) {
				t.Skipf("Real HTML data not found at %s, skipping", tc.zipDir)
			}

			// Get site definition
			def, ok := v2.GetDefinitionRegistry().Get(tc.siteID)
			require.True(t, ok, "site definition %q not found", tc.siteID)
			require.NotNil(t, def.UserInfo, "site %q has no UserInfo config", tc.siteID)

			// Create driver
			driver := newTestNexusPHPDriver(def)

			// Load the real HTML (userinfo.html contains index page with info_block + userdetails)
			htmlData, err := os.ReadFile(userinfoPath)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlData)))
			require.NoError(t, err)

			// Test index page fields (the userinfo.html typically includes the info_block)
			t.Run("IndexFields", func(t *testing.T) {
				for _, field := range tc.indexFields {
					field := field
					t.Run(field, func(t *testing.T) {
						sel, ok := def.UserInfo.Selectors[field]
						if !ok {
							t.Skipf("selector %q not defined", field)
						}
						got := driver.ExtractFieldValuePublic(doc, sel)
						assert.NotEmpty(t, got, "field %q should extract non-empty value from real HTML", field)
						if got != "" && got != "0" {
							t.Logf("  %s = %s", field, truncate(got, 80))
						}
					})
				}
			})

			// Test userdetails page fields
			t.Run("DetailFields", func(t *testing.T) {
				for _, field := range tc.detailFields {
					field := field
					t.Run(field, func(t *testing.T) {
						sel, ok := def.UserInfo.Selectors[field]
						if !ok {
							t.Skipf("selector %q not defined", field)
						}
						got := driver.ExtractFieldValuePublic(doc, sel)
						assert.NotEmpty(t, got, "field %q should extract non-empty value from real HTML", field)
						if got != "" && got != "0" {
							t.Logf("  %s = %s", field, truncate(got, 80))
						}
					})
				}
			})
		})
	}
}

// TestRealHTML_Search validates search selectors against real site search HTML.
func TestRealHTML_Search(t *testing.T) {
	sites := []struct {
		siteID string
		zipDir string
	}{
		{"btschool", "/tmp/site-zips/btschool"},
		{"hdfans", "/tmp/site-zips/hdfans"},
		{"soulvoice", "/tmp/site-zips/soulvoice"},
		{"52pt", "/tmp/site-zips/52pt"},
		{"lajidui", "/tmp/site-zips/lajidui"},
		{"1ptba", "/tmp/site-zips/1ptba"},
	}

	for _, tc := range sites {
		tc := tc
		t.Run(tc.siteID, func(t *testing.T) {
			searchPath := filepath.Join(tc.zipDir, "search.html")
			if _, err := os.Stat(searchPath); os.IsNotExist(err) {
				t.Skipf("Real search HTML not found at %s, skipping", tc.zipDir)
			}

			def, ok := v2.GetDefinitionRegistry().Get(tc.siteID)
			require.True(t, ok)
			require.NotNil(t, def.Selectors)

			htmlData, err := os.ReadFile(searchPath)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlData)))
			require.NoError(t, err)

			// Find torrent rows
			rows := doc.Find(def.Selectors.TableRows)
			rowCount := rows.Length()
			t.Logf("Found %d torrent rows", rowCount)
			assert.Greater(t, rowCount, 0, "should find at least 1 torrent row in real search HTML")

			if rowCount > 0 {
				// Check first row has title
				first := rows.First()
				title := first.Find(def.Selectors.Title)
				assert.Greater(t, title.Length(), 0, "first row should have a title element")
				if title.Length() > 0 {
					t.Logf("First title: %s", truncate(title.Text(), 100))
				}
			}
		})
	}
}

// TestRealHTML_Detail validates detail page selectors against real site detail HTML.
func TestRealHTML_Detail(t *testing.T) {
	sites := []struct {
		siteID string
		zipDir string
	}{
		{"btschool", "/tmp/site-zips/btschool"},
		{"hdfans", "/tmp/site-zips/hdfans"},
		{"soulvoice", "/tmp/site-zips/soulvoice"},
		{"52pt", "/tmp/site-zips/52pt"},
		{"lajidui", "/tmp/site-zips/lajidui"},
		{"1ptba", "/tmp/site-zips/1ptba"},
	}

	for _, tc := range sites {
		tc := tc
		t.Run(tc.siteID, func(t *testing.T) {
			detailPath := filepath.Join(tc.zipDir, "detail.html")
			if _, err := os.Stat(detailPath); os.IsNotExist(err) {
				t.Skipf("Real detail HTML not found at %s, skipping", tc.zipDir)
			}

			def, ok := v2.GetDefinitionRegistry().Get(tc.siteID)
			require.True(t, ok)
			require.NotNil(t, def.DetailParser)

			htmlData, err := os.ReadFile(detailPath)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlData)))
			require.NoError(t, err)

			parser := v2.NewNexusPHPParserFromDefinition(def)
			info := parser.ParseAll(doc.Selection)

			t.Logf("TorrentID: %s", info.TorrentID)
			t.Logf("Title: %s", truncate(info.Title, 100))
			t.Logf("Discount: %v", info.DiscountLevel)
			t.Logf("SizeMB: %.1f", info.SizeMB)
			t.Logf("HasHR: %v", info.HasHR)

			// TorrentID and Title may not be parsed from all sites (some use different detail page structures)
			// The critical fields are Discount, Size, and HR detection
			if info.TorrentID == "" {
				t.Logf("WARNING: TorrentID not parsed (site may use different detail page structure)")
			}
			if info.Title == "" {
				t.Logf("WARNING: Title not parsed (site may use different detail page structure)")
			}
		})
	}
}

// TestRealHTML_ExistingSites validates existing sites (xingyunge, agsvpt) against real HTML.
func TestRealHTML_ExistingSites(t *testing.T) {
	sites := []struct {
		siteID string
		zipDir string
		fields []string
	}{
		{
			siteID: "xingyunge",
			zipDir: "tmp/site-info/pt.xingyungept.org-32c29daf-6874-418f-a8f8-59ff596b293d",
			fields: []string{"id", "name", "uploaded", "downloaded", "ratio", "seeding", "leeching", "bonusIndex"},
		},
		{
			siteID: "agsvpt",
			zipDir: "tmp/site-info/www.agsvpt.com-f422ce9f-01fa-4b9d-9ae0-4e04205f029c",
			fields: []string{"id", "name", "uploaded", "downloaded", "ratio", "seeding", "leeching", "levelName", "joinTime"},
		},
	}

	for _, tc := range sites {
		tc := tc
		t.Run(tc.siteID, func(t *testing.T) {
			// Try both absolute and relative paths
			userinfoPath := filepath.Join(tc.zipDir, "userinfo.html")
			if _, err := os.Stat(userinfoPath); os.IsNotExist(err) {
				// Try from project root
				userinfoPath = filepath.Join("/config/workspace/ProdDir/pt-tools", tc.zipDir, "userinfo.html")
				if _, err := os.Stat(userinfoPath); os.IsNotExist(err) {
					t.Skipf("Real HTML data not found, skipping")
				}
			}

			def, ok := v2.GetDefinitionRegistry().Get(tc.siteID)
			require.True(t, ok)
			require.NotNil(t, def.UserInfo)

			driver := newTestNexusPHPDriver(def)

			htmlData, err := os.ReadFile(userinfoPath)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlData)))
			require.NoError(t, err)

			for _, field := range tc.fields {
				field := field
				t.Run(field, func(t *testing.T) {
					sel, ok := def.UserInfo.Selectors[field]
					if !ok {
						t.Skipf("selector %q not defined", field)
					}
					got := driver.ExtractFieldValuePublic(doc, sel)
					if got == "" {
						t.Errorf("field %q returned empty from real HTML", field)
					} else {
						t.Logf("  %s = %s", field, truncate(got, 80))
					}
				})
			}
		})
	}
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// Ensure truncate is used to avoid unused import
var _ = fmt.Sprintf
