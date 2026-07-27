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
	p := NewNexusPHPParserFromDefinition(&SiteDefinition{DetailParser: &DetailParserConfig{
		SizeRegex: `大小：[^\d]*([\d.]+)\s*([KMGT]i?B)`,
	}})
	tests := []struct {
		name   string
		value  string
		wantMB float64
		delta  float64
	}{
		{name: "TB", value: "2.00 TB", wantMB: 2097152, delta: 1},
		{name: "GB", value: "2.00 GB", wantMB: 2048, delta: 0.1},
		{name: "MB", value: "2.00 MB", wantMB: 2, delta: 0.1},
		{name: "KB", value: "512.00 KB", wantMB: 0.5, delta: 0.01},
		{name: "TiB", value: "2.00 TiB", wantMB: 2097152, delta: 1},
		{name: "GiB", value: "2.00 GiB", wantMB: 2048, delta: 0.1},
		{name: "MiB", value: "2.00 MiB", wantMB: 2, delta: 0.1},
		{name: "KiB", value: "512.00 KiB", wantMB: 0.5, delta: 0.01},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := `<html><body><table><tr><td class="rowhead">基本信息</td><td>大小：` + tt.value + `</td></tr></table></body></html>`
			doc := parseHTML(t, html)
			assert.InDelta(t, tt.wantMB, p.ParseSizeMB(doc), tt.delta)
		})
	}

	t.Run("no match", func(t *testing.T) {
		doc := parseHTML(t, `<html><body><table><tr><td class="rowhead">基本信息</td><td>无</td></tr></table></body></html>`)
		assert.Zero(t, p.ParseSizeMB(doc))
	})

	t.Run("default regex does not opt into binary units", func(t *testing.T) {
		doc := parseHTML(t, `<html><body><table><tr><td class="rowhead">基本信息</td><td>大小：2.00 GiB</td></tr></table></body></html>`)
		assert.Zero(t, NewNexusPHPParser().ParseSizeMB(doc))
	})
}

func TestNexusPHPParser_ParseTitleAndID_ValueAndText(t *testing.T) {
	tests := []struct {
		name   string
		html   string
		want   string
		wantID string
	}{
		{
			name:   "falls back to element text and strips trailing promotion",
			html:   `<h1 id="top">【十日拍拖手册/绝配冤家】10bit HEVC版本 国英双语 评论音轨&nbsp;&nbsp;&nbsp; <b>[<font class="free">免费</font>]</b></h1><span id="torrent-id">2782256</span>`,
			want:   "【十日拍拖手册/绝配冤家】10bit HEVC版本 国英双语 评论音轨",
			wantID: "2782256",
		},
		// These cases guard against stripping legitimate resolution and release-group brackets.
		{
			name:   "preserves resolution bracket after repeated spaces",
			html:   `<h1 id="top">某剧集 第一季  [1080p BluRay]</h1>`,
			want:   "某剧集 第一季  [1080p BluRay]",
			wantID: "",
		},
		{
			name:   "preserves release group bracket after repeated spaces",
			html:   `<h1 id="top">Some Movie 2025  [FRDS]</h1>`,
			want:   "Some Movie 2025  [FRDS]",
			wantID: "",
		},
		{
			name:   "preserves resolution bracket after one space",
			html:   `<h1 id="top">某剧集 第一季 [1080p BluRay]</h1>`,
			want:   "某剧集 第一季 [1080p BluRay]",
			wantID: "",
		},
		{
			name:   "preserves title without brackets",
			html:   `<h1 id="top">普通标题 没有方括号</h1>`,
			want:   "普通标题 没有方括号",
			wantID: "",
		},
		{
			name:   "value attribute wins over element text",
			html:   `<div id="top" value="  Attribute Title  ">Text Title</div><div id="torrent-id" value="00777">888</div>`,
			want:   "  Attribute Title  ",
			wantID: "00777",
		},
		{
			name:   "present empty value does not fall back",
			html:   `<div id="top" value="">Text Title</div><div id="torrent-id" value="">888</div>`,
			want:   "",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewNexusPHPParserFromDefinition(&SiteDefinition{DetailParser: &DetailParserConfig{
				TitleSelector: "#top",
				IDSelector:    "#torrent-id",
			}})
			title, torrentID := p.ParseTitleAndID(parseHTML(t, tt.html))
			assert.Equal(t, tt.want, title)
			assert.Equal(t, tt.wantID, torrentID)
		})
	}
}

func TestStripTrailingPromotion_KnownVocabulary(t *testing.T) {
	discountMapping := DefaultDetailParserConfig().DiscountMapping
	tests := []struct {
		label string
	}{
		{label: "免费"},
		{label: "FREE"},
		{label: "50%"},
		{label: "30%"},
		{label: "2X"},
		{label: "2x Free"},
		{label: "2X 免费"},
		{label: "2x 50%"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, "Title", stripTrailingPromotion("Title  ["+tt.label+"]", discountMapping))
		})
	}
}

func TestStripTrailingPromotion_RequestedCases(t *testing.T) {
	discountMapping := DefaultDetailParserConfig().DiscountMapping
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "keepfrds promo", input: "...带章节名\u00a0\u00a0\u00a0 [免费]", want: "...带章节名"},
		{name: "resolution with repeated spaces", input: "某剧集 第一季  [1080p BluRay]", want: "某剧集 第一季  [1080p BluRay]"},
		{name: "release group", input: "Some Movie 2025  [FRDS]", want: "Some Movie 2025  [FRDS]"},
		{name: "resolution with one space", input: "某剧集 第一季 [1080p BluRay]", want: "某剧集 第一季 [1080p BluRay]"},
		{name: "no brackets", input: "普通标题 没有方括号", want: "普通标题 没有方括号"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := stripTrailingPromotion(tt.input, discountMapping)
			t.Logf("in=%q out=%q", tt.input, actual)
			assert.Equal(t, tt.want, actual)
		})
	}

	t.Run("value attribute remains byte identical", func(t *testing.T) {
		p := NewNexusPHPParserFromDefinition(&SiteDefinition{DetailParser: &DetailParserConfig{
			TitleSelector: "#top",
		}})
		const want = "  Attribute Title  "
		title, _ := p.ParseTitleAndID(parseHTML(t, `<div id="top" value="  Attribute Title  ">Text Title&nbsp;&nbsp; [免费]</div>`))
		t.Logf("value=%q out=%q", want, title)
		assert.Equal(t, want, title)
	})
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
