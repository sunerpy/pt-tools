package v2

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a goquery.Selection from HTML
func parseHTML(t *testing.T, html string) *goquery.Selection {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc.Selection
}

// TestHDSkyParser tests the HDSky parser
func TestHDSkyParser(t *testing.T) {
	parser := NewHDSkyParser()

	t.Run("parse title and ID", func(t *testing.T) {
		html := `<html>
			<input name="torrent_name" value="Test Movie 2024">
			<input name="detail_torrent_id" value="12345">
		</html>`

		doc := parseHTML(t, html)
		title, id := parser.ParseTitleAndID(doc)

		assert.Equal(t, "Test Movie 2024", title)
		assert.Equal(t, "12345", id)
	})

	t.Run("parse free discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="free">Free</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})

	t.Run("parse 2x free discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="twoupfree">2x Free</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, Discount2xFree, discount)
	})

	t.Run("parse 2x up discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="twoup">2x Up</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, Discount2xUp, discount)
	})

	t.Run("parse 50% discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="halfdown">50%</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountPercent50, discount)
	})

	t.Run("parse 30% discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="thirtypercent">30%</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountPercent30, discount)
	})

	t.Run("parse no discount", func(t *testing.T) {
		html := `<html>
			<h1>Normal Torrent</h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountNone, discount)
	})

	t.Run("parse discount end time", func(t *testing.T) {
		html := `<html>
			<h1>
				<font class="free">Free</font>
				<span title="2024-12-31 23:59:59">Until</span>
			</h1>
		</html>`

		doc := parseHTML(t, html)
		_, endTime := parser.ParseDiscount(doc)

		expected := time.Date(2024, 12, 31, 23, 59, 59, 0, CSTLocation)
		assert.Equal(t, expected, endTime)
	})

	t.Run("parse HR status - hitandrun keyword", func(t *testing.T) {
		html := `<html>
			<div class="hitandrun">HR</div>
		</html>`

		doc := parseHTML(t, html)
		hasHR := parser.ParseHR(doc)

		assert.True(t, hasHR)
	})

	t.Run("parse HR status - hit_run.gif", func(t *testing.T) {
		html := `<html>
			<img src="hit_run.gif">
		</html>`

		doc := parseHTML(t, html)
		hasHR := parser.ParseHR(doc)

		assert.True(t, hasHR)
	})

	t.Run("parse no HR", func(t *testing.T) {
		html := `<html>
			<div>Normal torrent</div>
		</html>`

		doc := parseHTML(t, html)
		hasHR := parser.ParseHR(doc)

		assert.False(t, hasHR)
	})

	t.Run("parse size in GB", func(t *testing.T) {
		html := `<html>
			<table>
				<tr>
					<td class="rowhead">基本信息</td>
					<td>大小：10.5 GB</td>
				</tr>
			</table>
		</html>`

		doc := parseHTML(t, html)
		sizeMB := parser.ParseSizeMB(doc)

		assert.InDelta(t, 10752.0, sizeMB, 0.1) // 10.5 * 1024
	})

	t.Run("parse size in MB", func(t *testing.T) {
		html := `<html>
			<table>
				<tr>
					<td class="rowhead">基本信息</td>
					<td>大小：500 MB</td>
				</tr>
			</table>
		</html>`

		doc := parseHTML(t, html)
		sizeMB := parser.ParseSizeMB(doc)

		assert.InDelta(t, 500.0, sizeMB, 0.1)
	})

	t.Run("parse size in KB", func(t *testing.T) {
		html := `<html>
			<table>
				<tr>
					<td class="rowhead">基本信息</td>
					<td>大小：1024 KB</td>
				</tr>
			</table>
		</html>`

		doc := parseHTML(t, html)
		sizeMB := parser.ParseSizeMB(doc)

		assert.InDelta(t, 1.0, sizeMB, 0.1) // 1024 / 1024
	})

	t.Run("parse all", func(t *testing.T) {
		html := `<html>
			<input name="torrent_name" value="Complete Movie">
			<input name="detail_torrent_id" value="99999">
			<h1>
				<font class="free">Free</font>
				<span title="2025-01-15 12:00:00">Until</span>
			</h1>
			<div class="hitandrun">HR</div>
			<table>
				<tr>
					<td class="rowhead">基本信息</td>
					<td>大小：5.5 GB</td>
				</tr>
			</table>
		</html>`

		doc := parseHTML(t, html)
		info := parser.ParseAll(doc)

		assert.Equal(t, "Complete Movie", info.Title)
		assert.Equal(t, "99999", info.TorrentID)
		assert.Equal(t, DiscountFree, info.DiscountLevel)
		assert.True(t, info.HasHR)
		assert.InDelta(t, 5632.0, info.SizeMB, 0.1) // 5.5 * 1024
	})
}

// TestSpringSundayParser tests the SpringSunday parser
func TestSpringSundayParser(t *testing.T) {
	parser := NewSpringSundayParser()

	t.Run("parse title and ID", func(t *testing.T) {
		html := `<html>
			<input name="torrent_name" value="SpringSunday Movie 2024">
			<input name="detail_torrent_id" value="54321">
		</html>`

		doc := parseHTML(t, html)
		title, id := parser.ParseTitleAndID(doc)

		assert.Equal(t, "SpringSunday Movie 2024", title)
		assert.Equal(t, "54321", id)
	})

	t.Run("parse free discount", func(t *testing.T) {
		html := `<html>
			<h1><font class="free">Free</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})

	t.Run("parse all", func(t *testing.T) {
		html := `<html>
			<input name="torrent_name" value="SpringSunday Complete">
			<input name="detail_torrent_id" value="11111">
			<h1><font class="twoupfree">2x Free</font></h1>
			<table>
				<tr>
					<td class="rowhead">基本信息</td>
					<td>大小：2.5 GB</td>
				</tr>
			</table>
		</html>`

		doc := parseHTML(t, html)
		info := parser.ParseAll(doc)

		assert.Equal(t, "SpringSunday Complete", info.Title)
		assert.Equal(t, "11111", info.TorrentID)
		assert.Equal(t, Discount2xFree, info.DiscountLevel)
		assert.InDelta(t, 2560.0, info.SizeMB, 0.1) // 2.5 * 1024
	})
}

func TestParseDiscountEdgeCases(t *testing.T) {
	parser := NewHDSkyParser()

	t.Run("class with trailing whitespace", func(t *testing.T) {
		html := `<html>
			<h1><font class="free ">免费</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})

	t.Run("class with leading whitespace", func(t *testing.T) {
		html := `<html>
			<h1><font class=" free">免费</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})

	t.Run("multiple classes with free", func(t *testing.T) {
		html := `<html>
			<h1><font class="highlight free bold">免费</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})

	t.Run("multiple classes with twoupfree", func(t *testing.T) {
		html := `<html>
			<h1><font class="discount twoupfree special">2x Free</font></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, Discount2xFree, discount)
	})

	t.Run("NovaHD real HTML pattern", func(t *testing.T) {
		html := `<html>
			<h1 align="center" id="top">窈窕淑女（92集）刘擎＆姚慧&nbsp;&nbsp;&nbsp; <b>[<font class='free' >免费</font>]</b></h1>
		</html>`

		doc := parseHTML(t, html)
		discount, _ := parser.ParseDiscount(doc)

		assert.Equal(t, DiscountFree, discount)
	})
}

func TestParserConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DefaultNexusPHPParserConfig()
		assert.Equal(t, "2006-01-02 15:04:05", config.TimeLayout)
	})

	t.Run("custom time layout", func(t *testing.T) {
		parser := NewNexusPHPParser(WithTimeLayout("2006/01/02 15:04:05"))
		assert.Equal(t, "2006/01/02 15:04:05", parser.config.TimeLayout)
	})

	t.Run("type aliases work", func(t *testing.T) {
		p1 := NewHDSkyParser()
		p2 := NewSpringSundayParser()
		assert.NotNil(t, p1)
		assert.NotNil(t, p2)
	})
}

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

func TestNexusPHPParser_Options(t *testing.T) {
	p := NewNexusPHPParser(
		WithDiscountMapping(map[string]DiscountLevel{"foo": DiscountFree}),
		WithHRKeywords([]string{"kw"}),
		WithParserTimeLayout("2006-01-02"),
	)
	require.NotNil(t, p)
	assert.Equal(t, DiscountFree, p.config.DiscountMapping["foo"])
	assert.Equal(t, []string{"kw"}, p.config.HRKeywords)
	assert.Equal(t, "2006-01-02", p.config.TimeLayout)
}

func TestNexusPHPParserFromDefinition_Default(t *testing.T) {
	p := NewNexusPHPParserFromDefinition(nil)
	require.NotNil(t, p)

	def := &SiteDefinition{DetailParser: &DetailParserConfig{TimeLayout: "2006-01-02"}}
	p2 := NewNexusPHPParserFromDefinition(def)
	assert.Equal(t, "2006-01-02", p2.config.TimeLayout)
}
