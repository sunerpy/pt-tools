package definitions

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestLastAccessParse_最近动向(t *testing.T) {
	cst, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	cases := []struct {
		name    string
		def     *v2.SiteDefinition
		value   string
		wantRaw string
	}{
		{
			name:    "hdsky",
			def:     HDSkyDefinition,
			value:   "2026-06-03 22:57:56 (&lt; 1分前)",
			wantRaw: "2026-06-03 22:57:56",
		},
		{
			name:    "springsunday",
			def:     SpringSundayDefinition,
			value:   "2026-06-03 22:58:23 (&lt; 1分前)",
			wantRaw: "2026-06-03 22:58:23",
		},
		{
			name:    "duckboobee",
			def:     DuckBoobeeDefinition,
			value:   `2026-05-13 15:37:03 (<span title="2026-05-13 15:37:03">&lt; 1分钟前</span>)`,
			wantRaw: "2026-05-13 15:37:03",
		},
		{
			name:    "gamegamept",
			def:     GameGamePTDefinition,
			value:   `2026-05-09 12:52:58 (<span title="2026-05-09 12:52:58">23分钟前</span>)`,
			wantRaw: "2026-05-09 12:52:58",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			wantTime, err := time.ParseInLocation("2006-01-02 15:04:05", tt.wantRaw, cst)
			require.NoError(t, err)

			html := fmt.Sprintf(`<html><body><table>
<tr><td><a href="userdetails.php?id=10001">fixture_user</a></td></tr>
<tr><td class="rowhead">加入日期</td><td class="rowfollow">2015-05-23 10:43:42 (11年前)</td></tr>
<tr><td class="rowhead">最近动向</td><td class="rowfollow">%s</td></tr>
</table></body></html>`, tt.value)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
			require.NoError(t, err)

			selector, ok := tt.def.UserInfo.Selectors["lastAccessAt"]
			require.True(t, ok, "lastAccessAt selector must exist")

			driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: tt.def.URLs[0], Cookie: "test_cookie=1"})
			driver.SetSiteDefinition(tt.def)

			gotRaw := driver.ExtractFieldValuePublic(doc, selector)
			gotUnix, err := strconv.ParseInt(gotRaw, 10, 64)
			require.NoError(t, err)
			assert.Positive(t, gotUnix)
			assert.Equal(t, wantTime.Unix(), gotUnix)
		})
	}
}

func TestLastAccessParse_BatchBGetUserInfo(t *testing.T) {
	cst, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	cases := []struct {
		name    string
		def     *v2.SiteDefinition
		rowHTML string
		wantRaw string
	}{
		{
			name:    "duckboobee",
			def:     DuckBoobeeDefinition,
			rowHTML: `<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-13 15:37:03 (<span title="2026-05-13 15:37:03">&lt; 1分钟前</span>)</td></tr>`,
			wantRaw: "2026-05-13 15:37:03",
		},
		{
			name:    "gamegamept",
			def:     GameGamePTDefinition,
			rowHTML: `<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-09 12:52:58 (<span title="2026-05-09 12:52:58">23分钟前</span>)</td></tr>`,
			wantRaw: "2026-05-09 12:52:58",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			wantTime, err := time.ParseInLocation("2006-01-02 15:04:05", tt.wantRaw, cst)
			require.NoError(t, err)

			indexHTML := `<html><body><div id="info_block"><a href="userdetails.php?id=10001" class="User_Name"><b>fixture_user</b></a><font class="color_bonus">魔力值</font>: 1.0 <img class="arrowup" /> 1 <img class="arrowdown" /> 0</div></body></html>`
			userHTML := `<html><body><table>` +
				`<tr><td><a href="userdetails.php?id=10001" class="User_Name"><b>fixture_user</b></a></td></tr>` +
				`<tr><td class="rowhead">加入日期</td><td class="rowfollow">2015-05-23 10:43:42 (11年前)</td></tr>` +
				tt.rowHTML +
				`</table></body></html>`

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				switch r.URL.Path {
				case "/index.php":
					_, _ = w.Write([]byte(indexHTML))
				case "/userdetails.php":
					_, _ = w.Write([]byte(userHTML))
				case "/mybonus.php":
					_, _ = w.Write([]byte(`<html><body>你当前每小时能获取 1.0 个魔力值</body></html>`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "test_cookie=1"})
			driver.SetSiteDefinition(tt.def)

			info, err := driver.GetUserInfo(context.Background())
			require.NoError(t, err)
			assert.Positive(t, info.LastAccess)
			assert.Equal(t, wantTime.Unix(), info.LastAccess)
		})
	}
}

func TestLastAccessParse_BatchCRealHTML(t *testing.T) {
	cst, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	cases := []struct {
		name    string
		def     *v2.SiteDefinition
		html    string
		wantRaw string
	}{
		{
			name:    "hxpt",
			def:     HXPTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-27 10:56:06 (<span title="2026-03-27 10:56:06">1时16分钟前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-27 10:56:06",
		},
		{
			name:    "longpt",
			def:     LongPTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-13 13:23:09 (<span title="2026-05-13 13:23:09">2时10分钟前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-13 13:23:09",
		},
		{
			name:    "nicept",
			def:     NicePTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-14 10:25:19 (<span title="2026-05-14 10:25:19">50分前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-14 10:25:19",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			wantTime, err := time.ParseInLocation("2006-01-02 15:04:05", tt.wantRaw, cst)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)

			selector, ok := tt.def.UserInfo.Selectors["lastAccessAt"]
			require.True(t, ok, "lastAccessAt selector must exist")

			driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: tt.def.URLs[0], Cookie: "test_cookie=1"})
			driver.SetSiteDefinition(tt.def)

			gotRaw := driver.ExtractFieldValuePublic(doc, selector)
			gotUnix, err := strconv.ParseInt(gotRaw, 10, 64)
			require.NoError(t, err)
			assert.Positive(t, gotUnix)
			assert.Equal(t, wantTime.Unix(), gotUnix)
		})
	}
}

func TestLastAccessParse_BackfillBatch(t *testing.T) {
	cst, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	cases := []struct {
		name    string
		def     *v2.SiteDefinition
		html    string
		wantRaw string
	}{
		{
			name:    "carpt",
			def:     CarPTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-31 15:02:20 (<span title="2026-03-31 15:02:20">2月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-31 15:02:20",
		},
		{
			name:    "crabpt",
			def:     CrabPTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-09 21:09:46 (<span title="2026-05-09 21:09:46">25天前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-09 21:09:46",
		},
		{
			name:    "dubhe",
			def:     DubheDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-07 09:32:35 (<span title="2026-05-07 09:32:35">27天前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-07 09:32:35",
		},
		{
			name:    "ourbits",
			def:     OurBitsDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">网站访问: 2026-05-15 02:49:47 (<span title="2026-05-15 02:49:47">19天前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-15 02:49:47",
		},
		{
			name:    "opencd",
			def:     OpenCDDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近動向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-04-10 11:41:13 (<span title="2026-04-10 11:41:13">54天前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-04-10 11:41:13",
		},
		{
			name:    "ptlover",
			def:     PTLoverDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-07 22:35:57 (<span title="2026-03-07 22:35:57">3月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-07 22:35:57",
		},
		{
			name:    "ptskit",
			def:     PTSKITDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-27 12:17:13 (<span title="2026-03-27 12:17:13">2月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-27 12:17:13",
		},
		{
			name:    "pt0ffcc",
			def:     PT0FFCCDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-05-02 09:45:54 (<span title="2026-05-02 09:45:54">32天前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-05-02 09:45:54",
		},
		{
			name:    "raingfh",
			def:     RaingfhDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-02-25 20:18:53 (<span title="2026-02-25 20:18:53">3月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-02-25 20:18:53",
		},
		{
			name:    "soulvoice",
			def:     SoulVoiceDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-04 09:50:02 (<span title="2026-03-04 09:50:02">3月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-04 09:50:02",
		},
		{
			name:    "tmpt",
			def:     TMPTDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-16 11:31:35 (<span title="2026-03-16 11:31:35">3月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-16 11:31:35",
		},
		{
			name:    "ubits",
			def:     UBitsDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-03-20 18:14:39 (<span title="2026-03-20 18:14:39">2月前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-03-20 18:14:39",
		},
		{
			name:    "hdfans",
			def:     HDFansDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-06-07 19:06:04 (<span title="2026-06-07 19:06:04">&lt; 1分钟前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-06-07 19:06:04",
		},
		{
			name:    "pttime",
			def:     PTTimeDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="95%" class="rowfollow" valign="top" align="left">2026-06-07 00:46:25 (<span title="2026-06-07 00:46:25">18时19分前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-06-07 00:46:25",
		},
		{
			name:    "btschool",
			def:     BTSchoolDefinition,
			html:    `<html><body><table><tr><td width="1%" class="rowhead nowrap" valign="top" align="right">最近动向</td><td width="99%" class="rowfollow" valign="top" align="left">2026-06-07 19:06:07 (<span title="2026-06-07 19:06:07">&lt; 1分前</span>)</td></tr></table></body></html>`,
			wantRaw: "2026-06-07 19:06:07",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			wantTime, err := time.ParseInLocation("2006-01-02 15:04:05", tt.wantRaw, cst)
			require.NoError(t, err)

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)

			selector, ok := tt.def.UserInfo.Selectors["lastAccessAt"]
			require.True(t, ok, "lastAccessAt selector must exist")

			driver := v2.NewNexusPHPDriver(v2.NexusPHPDriverConfig{BaseURL: tt.def.URLs[0], Cookie: "test_cookie=1"})
			driver.SetSiteDefinition(tt.def)

			gotRaw := driver.ExtractFieldValuePublic(doc, selector)
			gotUnix, err := strconv.ParseInt(gotRaw, 10, 64)
			require.NoError(t, err)
			assert.Positive(t, gotUnix)
			assert.Equal(t, wantTime.Unix(), gotUnix)
		})
	}
}
