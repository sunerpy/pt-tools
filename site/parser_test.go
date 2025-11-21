package site

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	sm "github.com/sunerpy/pt-tools/site/mocks"
	"go.uber.org/zap"
)

func newSelection2(h string) *goquery.Selection {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(h))
	return doc.Selection
}

func TestCMCTParser_ParseAll(t *testing.T) {
	p := NewCMCTParser(WithTimeLayout("2006-01-02 15:04:05"))
	// title & id
	e := &colly.HTMLElement{DOM: newSelection2(`<input name='torrent_name' value='T1'><input name='detail_torrent_id' value='ID1'>`)}
	var info models.PHPTorrentInfo
	p.ParseTitleAndID(e, &info)
	assert.Equal(t, "T1", info.Title)
	assert.Equal(t, "ID1", info.TorrentID)
	// discount
	e2 := &colly.HTMLElement{DOM: newSelection2(`<h1><font class='free'></font><span title='2025-01-01 00:00:00'></span></h1>`)}
	p.ParseDiscount(e2, &info)
	assert.True(t, info.IsFree())
	assert.NotNil(t, info.GetFreeEndTime())
	// size & HR
	e3 := &colly.HTMLElement{DOM: newSelection2(`<span title='大小'>1.5 GB</span><img src='hit_run.gif'>`)}
	p.ParseTorrentSizeMB(e3, &info)
	assert.InDelta(t, 1536, info.SizeMB, 1)
	p.ParseHR(e3, &info)
	assert.True(t, info.HR)
}

func TestHDSkyParser_ParseAll(t *testing.T) {
	p := NewHDSkyParser(WithTimeLayout("2006-01-02 15:04:05"))
	e := &colly.HTMLElement{DOM: newSelection2(`<input name='torrent_name' value='X'><input name='detail_torrent_id' value='IDX'>`)}
	var info models.PHPTorrentInfo
	p.ParseTitleAndID(e, &info)
	assert.Equal(t, "X", info.Title)
	assert.Equal(t, "IDX", info.TorrentID)
	e2 := &colly.HTMLElement{DOM: newSelection2(`<h1><font class='free'></font><span title='2025-01-01 00:00:00'></span></h1>
             <table><tr><td class='rowhead'>基本信息</td><td>大小： 2.0 GB</td></tr></table>
             <img src='hit_run.gif'>`)}
	p.ParseDiscount(e2, &info)
	p.ParseTorrentSizeMB(e2, &info)
	p.ParseHR(e2, &info)
	assert.True(t, info.IsFree())
	assert.NotNil(t, info.GetFreeEndTime())
	assert.InDelta(t, 2048, info.SizeMB, 1)
	assert.True(t, info.HR)
}

func TestParser_WithTimeLayout(t *testing.T) {
	p := NewHDSkyParser(WithTimeLayout("2006-01-02 15:04:05"))
	assert.NotEmpty(t, p.Config.TimeLayout)
}

func TestParsers_NegativeCases(t *testing.T) {
	global.InitLogger(zap.NewNop())
	// CMCT: no free icon -> not free, no size -> default 0
	p := NewCMCTParser(WithTimeLayout("2006-01-02 15:04:05"))
	e := &colly.HTMLElement{DOM: newSelection2(`<h1><span title='2025-01-01 00:00:00'></span></h1>`)}
	var info models.PHPTorrentInfo
	p.ParseDiscount(e, &info)
	assert.False(t, info.IsFree())
	e2 := &colly.HTMLElement{DOM: newSelection2(`<div>no size</div>`)}
	p.ParseTorrentSizeMB(e2, &info)
	assert.True(t, info.SizeMB == 0)
	// HDSKY: missing HR image -> HR false
	hp := NewHDSkyParser(WithTimeLayout("2006-01-02 15:04:05"))
	e3 := &colly.HTMLElement{DOM: newSelection2(`<span title='大小'>1.0 GB</span>`)}
	hp.ParseHR(e3, &info)
	assert.False(t, info.HR)
}

// helper only if not defined elsewhere
func newSel(h string) *goquery.Selection {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(h))
	return doc.Selection
}

func TestHDSkyParser_ParseBasic(t *testing.T) {
	p := NewHDSkyParser(WithTimeLayout("2006-01-02 15:04:05"))
	html := `<input name='torrent_name' value='X'><input name='detail_torrent_id' value='IDX'>`
	e := &colly.HTMLElement{DOM: newSel(html)}
	var info models.PHPTorrentInfo
	p.ParseTitleAndID(e, &info)
	if info.Title != "X" || info.TorrentID != "IDX" {
		t.Fatalf("title/id")
	}
}

func TestHDSKY_ParseDiscountAndEndTime(t *testing.T) {
	html := `<h1><font class='free'>FREE</font><span title='2006-01-02 15:04:05'></span></h1>`
	e := newSelection2(html)
	var info models.PHPTorrentInfo
	p := NewHDSkyParser()
	p.ParseDiscount(&colly.HTMLElement{DOM: e}, &info)
	if !info.IsFree() {
		t.Fatalf("expected free")
	}
}

func makeHTMLElement(t *testing.T, html string, selector string) *colly.HTMLElement {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("doc: %v", err)
	}
	sel := doc.Find(selector).First()
	return &colly.HTMLElement{DOM: sel}
}

func TestCMCT_ParseTitleAndID(t *testing.T) {
	html := `<div>
        <input name='torrent_name' value='Test Title'>
        <input name='detail_torrent_id' value='T123'>
      </div>`
	e := makeHTMLElement(t, html, "div")
	var info models.PHPTorrentInfo
	NewCMCTParser().ParseTitleAndID(e, &info)
	if info.Title != "Test Title" || info.TorrentID != "T123" {
		t.Fatalf("parse failed: %+v", info)
	}
}

func TestHDSkyParser_DiscountAndSizeHR(t *testing.T) {
	p := NewHDSkyParser(WithTimeLayout("2006-01-02 15:04:05"))
	html := `<h1><font class='free'></font><span title='2025-01-01 00:00:00'></span></h1>
             <table><tr><td class='rowhead'>基本信息</td><td>大小： 2.0 GB</td></tr></table>
             <img src='hit_run.gif'>`
	e := &colly.HTMLElement{DOM: newSel(html)}
	var info models.PHPTorrentInfo
	p.ParseDiscount(e, &info)
	p.ParseTorrentSizeMB(e, &info)
	p.ParseHR(e, &info)
	if !info.IsFree() || info.GetFreeEndTime() == nil {
		t.Fatalf("discount")
	}
	if info.SizeMB < 2048-1 || info.SizeMB > 2048+1 {
		t.Fatalf("sizeMB: %f", info.SizeMB)
	}
	if !info.HR {
		t.Fatalf("hr")
	}
}

func TestCMCTParser_ParseTitleAndID_Table(t *testing.T) {
	p := NewCMCTParser()
	html := `<input name='torrent_name' value='T1'><input name='detail_torrent_id' value='ID1'>`
	e := &colly.HTMLElement{DOM: newSelection2(html)}
	var info models.PHPTorrentInfo
	p.ParseTitleAndID(e, &info)
	if info.Title != "T1" || info.TorrentID != "ID1" {
		t.Fatalf("title/id")
	}
}

func TestCMCTParser_ParseDiscount_Table(t *testing.T) {
	p := NewCMCTParser(WithTimeLayout("2006-01-02 15:04:05"))
	html := `<h1><font class='free'></font><span title='2025-01-01 00:00:00'></span></h1>`
	e := &colly.HTMLElement{DOM: newSelection2(html)}
	var info models.PHPTorrentInfo
	p.ParseDiscount(e, &info)
	if !info.IsFree() || info.GetFreeEndTime() == nil {
		t.Fatalf("discount/end")
	}
}

func TestCMCTParser_ParseHRAndSize_Table(t *testing.T) {
	p := NewCMCTParser()
	html := `<span title='大小'>1.5 GB</span><img src='hit_run.gif'>`
	e := &colly.HTMLElement{DOM: newSelection2(html)}
	var info models.PHPTorrentInfo
	p.ParseTorrentSizeMB(e, &info)
	if info.SizeMB < 1536-1 || info.SizeMB > 1536+1 {
		t.Fatalf("size: %f", info.SizeMB)
	}
	p.ParseHR(e, &info)
	if !info.HR {
		t.Fatalf("hr")
	}
}

func TestNewCollectorWithTransport(t *testing.T) {
	c := NewCollectorWithTransport()
	require.NotNil(t, c)
}

func TestCommonFetchTorrentInfo_ParserCalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mp := sm.NewMockSiteParser(ctrl)
	mp.EXPECT().ParseTitleAndID(gomock.Any(), gomock.Any()).Times(1)
	mp.EXPECT().ParseDiscount(gomock.Any(), gomock.Any()).Times(1)
	mp.EXPECT().ParseHR(gomock.Any(), gomock.Any()).Times(1)
	mp.EXPECT().ParseTorrentSizeMB(gomock.Any(), gomock.Any()).Times(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html><body><div>ok</div></body></html>"))
	}))
	defer srv.Close()
	c := colly.NewCollector()
	conf := models.SiteConfig{Cookie: "c"}
	smc := NewSiteMapConfig(models.CMCT, conf.Cookie, conf, mp)
	out, err := CommonFetchTorrentInfo(context.Background(), c, smc, srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if out == nil {
		t.Fatalf("nil out")
	}
}
