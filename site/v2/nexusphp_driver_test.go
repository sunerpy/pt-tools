package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNexusPHPDriver(t *testing.T) {
	config := NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	}

	driver := NewNexusPHPDriver(config)

	assert.Equal(t, "https://example.com", driver.BaseURL)
	assert.Equal(t, "test-cookie", driver.Cookie)
	assert.NotNil(t, driver.httpClient)
	assert.NotEmpty(t, driver.userAgent)
}

func TestNewNexusPHPDriver_CustomSelectors(t *testing.T) {
	customSelectors := &SiteSelectors{
		TableRows: "table.custom > tr",
		Title:     "td.title a",
	}

	config := NexusPHPDriverConfig{
		BaseURL:   "https://example.com",
		Cookie:    "test-cookie",
		Selectors: customSelectors,
	}

	driver := NewNexusPHPDriver(config)

	assert.Equal(t, "table.custom > tr", driver.Selectors.TableRows)
	assert.Equal(t, "td.title a", driver.Selectors.Title)
}

func TestNexusPHPDriver_PrepareSearch(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	tests := []struct {
		name     string
		query    SearchQuery
		wantPath string
		wantKey  string
		wantVal  string
	}{
		{
			name:     "keyword search",
			query:    SearchQuery{Keyword: "test movie"},
			wantPath: "/torrents.php",
			wantKey:  "search",
			wantVal:  "test movie",
		},
		{
			name:     "free only",
			query:    SearchQuery{FreeOnly: true},
			wantPath: "/torrents.php",
			wantKey:  "spstate",
			wantVal:  "2",
		},
		{
			name:     "with category",
			query:    SearchQuery{Category: "401"},
			wantPath: "/torrents.php",
			wantKey:  "cat",
			wantVal:  "401",
		},
		{
			name:     "with page",
			query:    SearchQuery{Page: 2},
			wantPath: "/torrents.php",
			wantKey:  "page",
			wantVal:  "1", // 0-indexed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := driver.PrepareSearch(tt.query)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, req.Path)
			assert.Equal(t, "GET", req.Method)
			if tt.wantKey != "" {
				assert.Equal(t, tt.wantVal, req.Params.Get(tt.wantKey))
			}
		})
	}
}

func TestNexusPHPDriver_Execute(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Contains(t, r.Header.Get("Cookie"), "test-cookie")
		assert.NotEmpty(t, r.Header.Get("User-Agent"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><table class="torrents"><tbody><tr><td>Header</td></tr></tbody></table></body></html>`))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{
		Path:   "/torrents.php",
		Method: "GET",
	}

	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.NotNil(t, res.Document)
}

func TestNexusPHPDriver_Execute_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "invalid-cookie",
	})

	req := NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestNexusPHPDriver_ParseSearch(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	// Sample HTML response
	html := `
	<html>
	<body>
	<table class="torrents">
		<tbody>
			<tr><td>Header</td></tr>
			<tr>
				<td><img alt="Movie" /></td>
				<td><a href="details.php?id=12345">Test Movie 2024</a></td>
				<td></td>
				<td><span>2024-01-01</span></td>
				<td>1.5 GB</td>
				<td>100</td>
				<td>10</td>
				<td>500</td>
			</tr>
		</tbody>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver.BaseURL = server.URL

	req := NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "12345", items[0].ID)
	assert.Equal(t, "Test Movie 2024", items[0].Title)
}

func TestNexusPHPDriver_ParseSearch_DiscountEndTimeFromOnmouseover(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://hdsky.me",
		Cookie:  "test-cookie",
	})

	html := `
	<html>
	<body>
	<table class="torrents">
		<tbody>
			<tr><td>Header</td></tr>
			<tr>
				<td><img alt="Movie" /></td>
				<td>
					<a href="details.php?id=12345">Test Free Movie</a>
					<img class="pro_free" src="pic/trans.gif" alt="Free" onmouseover="domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-01-20 15:30:00&quot;&gt;2天3时&lt;/span&gt;&lt;/b&gt;', 'trail', false)" />
				</td>
				<td></td>
				<td><span>2024-01-01</span></td>
				<td>2.5 GB</td>
				<td>50</td>
				<td>5</td>
				<td>200</td>
			</tr>
		</tbody>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver.BaseURL = server.URL

	req := NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "12345", items[0].ID)
	assert.Equal(t, "Test Free Movie", items[0].Title)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.False(t, items[0].DiscountEndTime.IsZero(), "DiscountEndTime should be parsed from onmouseover")
	assert.Equal(t, 2026, items[0].DiscountEndTime.Year())
	assert.Equal(t, 1, int(items[0].DiscountEndTime.Month()))
	assert.Equal(t, 20, items[0].DiscountEndTime.Day())
	assert.Equal(t, 15, items[0].DiscountEndTime.Hour())
	assert.Equal(t, 30, items[0].DiscountEndTime.Minute())
}

func TestNexusPHPDriver_PrepareDetail(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	req, err := driver.PrepareDetail("12345")
	require.NoError(t, err)
	assert.Equal(t, "/details.php", req.Path)
	assert.Equal(t, "12345", req.Params.Get("id"))
	assert.Equal(t, "1", req.Params.Get("hit"))
}

func TestNexusPHPDriver_ParseDetail(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://hdsky.me",
		Cookie:  "test-cookie",
	})

	// Sample HDSky detail page HTML
	html := `
	<html>
	<body>
	<table width="98%" cellspacing="0" cellpadding="5">
		<tr>
			<td class="rowhead">下载</td>
			<td class="rowfollow" align="left">
				<form class="index" style="display:inline" action="https://hdsky.me/download.php?id=164895&t=1768127842&sign=25f38b6c3192f55212409d63728954f5" method="POST">
					<input type="submit" value="[HDSky].Test.torrent" class="a" style="font-weight:bold">
				</form>
			</td>
		</tr>
		<tr>
			<td class="rowhead" valign="top">下载链接</td>
			<td class="rowfollow" align="left" valign="top">
				<a href="https://hdsky.me/download.php?id=164895&passkey=6e09ad8772d3f2d04fab9476cebe2190&sign=b2f34df2e7ef00407d120d5ccbcd60bf">https://hdsky.me/download.php?id=164895&passkey=...</a>
			</td>
		</tr>
		<tr>
			<td class="rowhead nowrap" valign="top" align="right">副标题</td>
			<td class="rowfollow" valign="top" align="left">这是一个测试副标题</td>
		</tr>
		<tr>
			<td class="rowhead nowrap" valign="top" align="right">种子文件</td>
			<td class="rowfollow" valign="top" align="left">
				<table>
					<tr>
						<td class="no_border_wide">
							<b>Hash码:</b>
							&nbsp;303a850dedc19e60bd7cc814f60e0e28d7f2c202
						</td>
					</tr>
				</table>
			</td>
		</tr>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver.BaseURL = server.URL

	req := NexusPHPRequest{Path: "/details.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	detail, err := driver.ParseDetail(res)
	require.NoError(t, err)

	// Should find the download link from "下载链接" row
	assert.Contains(t, detail.DownloadURL, "download.php")
	assert.Contains(t, detail.DownloadURL, "passkey=")
	assert.Equal(t, "这是一个测试副标题", detail.Subtitle)
	assert.Equal(t, "303a850dedc19e60bd7cc814f60e0e28d7f2c202", detail.InfoHash)
}

func TestNexusPHPDriver_ParseDetail_FormAction(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	// HTML with form action but no direct link
	html := `
	<html>
	<body>
	<table width="98%" cellspacing="0" cellpadding="5">
		<tr>
			<td class="rowhead">下载</td>
			<td class="rowfollow">
				<form action="https://example.com/download.php?id=12345&t=123&sign=abc" method="POST">
					<input type="submit" value="Download">
				</form>
			</td>
		</tr>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver.BaseURL = server.URL

	req := NexusPHPRequest{Path: "/details.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	detail, err := driver.ParseDetail(res)
	require.NoError(t, err)

	assert.Contains(t, detail.DownloadURL, "download.php")
	assert.Contains(t, detail.DownloadURL, "id=12345")
}

func TestNexusPHPDriver_PrepareUserInfo(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	req, err := driver.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "/index.php", req.Path)
	assert.Equal(t, "GET", req.Method)
}

func TestNexusPHPDriver_PrepareDownload(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	req, err := driver.PrepareDownload("12345")
	require.NoError(t, err)
	// PrepareDownload now requests the detail page first to get the passkey download URL
	assert.Equal(t, "/details.php", req.Path)
	assert.Equal(t, "12345", req.Params.Get("id"))
	assert.Equal(t, "1", req.Params.Get("hit"))
}

func TestExtractTorrentID(t *testing.T) {
	tests := []struct {
		href     string
		expected string
	}{
		{"details.php?id=12345", "12345"},
		{"details.php?id=12345&hit=1", "12345"},
		{"/details.php?id=99999", "99999"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractTorrentID(tt.href))
		})
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"1 KB", 1024},
		{"1KB", 1024},
		{"1.5 KB", 1536},
		{"1 MB", 1024 * 1024},
		{"1.5 GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"1 TB", 1024 * 1024 * 1024 * 1024},
		{"100 B", 100},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseSize(tt.input))
		})
	}
}

func TestParseRatio(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"1.5", 1.5},
		{"0.5", 0.5},
		{"inf", -1},
		{"Inf", -1},
		{"∞", -1},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseRatio(tt.input))
		})
	}
}

func TestDefaultNexusPHPSelectors(t *testing.T) {
	selectors := DefaultNexusPHPSelectors()

	assert.NotEmpty(t, selectors.TableRows)
	assert.NotEmpty(t, selectors.Title)
	assert.NotEmpty(t, selectors.TitleLink)
	assert.NotEmpty(t, selectors.Size)
	assert.NotEmpty(t, selectors.Seeders)
	assert.NotEmpty(t, selectors.Leechers)
}

func TestNexusPHPDriver_PrepareUserSeedingPage(t *testing.T) {
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://example.com",
		Cookie:  "test-cookie",
	})

	req, err := driver.PrepareUserSeedingPage("12345", "seeding")
	require.NoError(t, err)
	assert.Equal(t, "/getusertorrentlistajax.php", req.Path)
	assert.Equal(t, "12345", req.Params.Get("userid"))
	assert.Equal(t, "seeding", req.Params.Get("type"))
}

func TestNexusPHPDriver_ParseSeedingStatus_Method1_DirectSummary(t *testing.T) {
	// Test Method 1: Direct summary parsing (e.g., "10 | 100 GB")
	html := `
	<html>
	<body>
	<div>
		<div>10 | 100 GB</div>
	</div>
	<table>
		<tr><th>Name</th><th>Size</th></tr>
		<tr><td>Torrent 1</td><td>10 GB</td></tr>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{Path: "/getusertorrentlistajax.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	seeding, seedingSize, err := driver.ParseSeedingStatus(res)
	require.NoError(t, err)
	assert.Equal(t, 10, seeding)
	assert.Equal(t, int64(100*1024*1024*1024), seedingSize) // 100 GB in bytes
}

func TestNexusPHPDriver_ParseSeedingStatus_Method2_TableAccumulation(t *testing.T) {
	// Test Method 2: Table accumulation (fallback when no summary)
	html := `
	<html>
	<body>
	<table>
		<tr><th>Name</th><th>Category</th><th>Size</th><th>Seeders</th></tr>
		<tr><td>Torrent 1</td><td>Movie</td><td>10 GB</td><td>5</td></tr>
		<tr><td>Torrent 2</td><td>TV</td><td>20 GB</td><td>3</td></tr>
		<tr><td>Torrent 3</td><td>Music</td><td>5.5 GB</td><td>10</td></tr>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{Path: "/getusertorrentlistajax.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	seeding, seedingSize, err := driver.ParseSeedingStatus(res)
	require.NoError(t, err)
	assert.Equal(t, 3, seeding) // 3 torrent rows

	// Expected: 10 GB + 20 GB + 5.5 GB = 35.5 GB
	expectedSize := int64(10*1024*1024*1024) + int64(20*1024*1024*1024) + int64(5.5*1024*1024*1024)
	assert.Equal(t, expectedSize, seedingSize)
}

func TestNexusPHPDriver_ParseSeedingStatus_EmptyTable(t *testing.T) {
	// Test with no table data
	html := `
	<html>
	<body>
	<div>No torrents found</div>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{Path: "/getusertorrentlistajax.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	seeding, seedingSize, err := driver.ParseSeedingStatus(res)
	require.NoError(t, err)
	assert.Equal(t, 0, seeding)
	assert.Equal(t, int64(0), seedingSize)
}

func TestNexusPHPDriver_ParseSeedingStatus_AutoDetectSizeColumn(t *testing.T) {
	// Test auto-detection of size column with different column order
	html := `
	<html>
	<body>
	<table>
		<tr><th>Name</th><th>Seeders</th><th>Leechers</th><th>Size</th><th>Time</th></tr>
		<tr><td>Movie A</td><td>10</td><td>2</td><td>4.5 GB</td><td>2024-01-01</td></tr>
		<tr><td>Movie B</td><td>5</td><td>1</td><td>2.5 GB</td><td>2024-01-02</td></tr>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{Path: "/getusertorrentlistajax.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	seeding, seedingSize, err := driver.ParseSeedingStatus(res)
	require.NoError(t, err)
	assert.Equal(t, 2, seeding)

	// Expected: 4.5 GB + 2.5 GB = 7 GB
	expectedSize := int64(4.5*1024*1024*1024) + int64(2.5*1024*1024*1024)
	assert.Equal(t, expectedSize, seedingSize)
}

func TestNexusPHPDriver_ParseSeedingStatus_SpringSundayFormat(t *testing.T) {
	// Test SpringSunday format: "<b>94</b>条记录，共计<b>2.756 TB</b>"
	html := `<b>94</b>条记录，共计<b>2.756 TB</b>。其中官种<b>46</b>个，共计<b>1.726 TB</b>。当前做种积分为<b>66.90</b>/小时<br /><table border="1" cellspacing="0" cellpadding="5" width="800" class="common"><tr class="nowrap"><td class="colhead" align="center">类型</td><td class="colhead" align="center">标题</td><td class="colhead" align="center">大小</td></tr><tr><td>Movie</td><td>Test.Movie.2024</td><td>10 GB</td></tr></table>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "test-cookie",
	})

	req := NexusPHPRequest{Path: "/getusertorrentlistajax.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	seeding, seedingSize, err := driver.ParseSeedingStatus(res)
	require.NoError(t, err)
	assert.Equal(t, 94, seeding)

	// Expected: 2.756 TB = 2.756 * 1024^4 bytes
	tb := float64(1024 * 1024 * 1024 * 1024)
	expectedSize := int64(2.756 * tb)
	assert.Equal(t, expectedSize, seedingSize)
}

func TestNexusPHPDriver_Execute_SessionExpired(t *testing.T) {
	// Test login page detection - when server returns a login page instead of actual content
	loginPageHTML := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>SSD :: 登录</title>
	</head>
	<body>
	<div class="login-panel">
		<form id="login-form" method="POST" action="takelogin.php">
			<input type="text" name="username" placeholder="用户名">
			<input type="password" name="password" placeholder="密码">
			<button type="submit">登录</button>
		</form>
	</div>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(loginPageHTML))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "expired-cookie",
	})

	req := NexusPHPRequest{Path: "/index.php", Method: "GET"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestNexusPHPDriver_Execute_SessionExpired_TakeloginForm(t *testing.T) {
	// Test login page detection - only form action contains takelogin
	loginPageHTML := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Some Site</title>
	</head>
	<body>
	<form method="POST" action="takelogin.php">
		<input type="text" name="username">
		<input type="password" name="password">
	</form>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(loginPageHTML))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "expired-cookie",
	})

	req := NexusPHPRequest{Path: "/index.php", Method: "GET"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestNexusPHPDriver_Execute_NormalPage(t *testing.T) {
	normalPageHTML := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>My Profile</title>
	</head>
	<body>
	<div id="info_block">
		<a href="userdetails.php?id=12345">TestUser</a>
		<span>上传量: 1.5 TB</span>
	</div>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(normalPageHTML))
	}))
	defer server.Close()

	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: server.URL,
		Cookie:  "valid-cookie",
	})

	req := NexusPHPRequest{Path: "/index.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, res.Document)
}

func TestNexusPHPDriver_ParseSearch_DiscountEndTimeFromDOMElement(t *testing.T) {
	// SpringSunday uses a DOM element for discount end time, not onmouseover
	// Selector: "div.torrent-title span[style*='DimGray'] span[title]"
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{
		BaseURL: "https://springsunday.net",
		Cookie:  "test-cookie",
		Selectors: &SiteSelectors{
			TableRows:       "table.torrents > tbody > tr:has(table.torrentname), table.torrents > tr:has(table.torrentname)",
			Title:           "div.torrent-title > a[href*='details.php']",
			TitleLink:       "div.torrent-title > a[href*='details.php']",
			Subtitle:        "div.torrent-smalldescr > span[title]:last-of-type",
			Size:            "td.rowfollow:nth-child(5)",
			Seeders:         "td.rowfollow:nth-child(6)",
			Leechers:        "td.rowfollow:nth-child(7)",
			Snatched:        "td.rowfollow:nth-child(8)",
			DiscountIcon:    "span.torrent-pro-free, span.torrent-pro-2up, span.torrent-pro-50pctdown, span.torrent-pro-30pctdown, span.torrent-pro-2xfree",
			DiscountEndTime: "div.torrent-title span[style*='DimGray'] span[title]",
			DownloadLink:    "a[href*='download.php']",
			Category:        "td.rowfollow:nth-child(1) img[alt]",
			UploadTime:      "td.rowfollow.nowrap span[title]",
		},
	})

	// SpringSunday HTML structure with free torrent and discount end time in DOM element
	html := `
	<html>
	<body>
	<table class="torrents">
		<tbody>
			<tr>
				<td class="rowfollow">
					<img alt="电影" />
				</td>
				<td class="rowfollow">
					<table class="torrentname">
						<tr>
							<td>
								<div class="torrent-title">
									<a href="details.php?id=98765">Test.Movie.2024.BluRay</a>
									<span class="torrent-pro-free">Free</span>
									<span style="color: DimGray;"> (限时: <span title="2026-01-25 18:00:00">6天23时</span>)</span>
								</div>
								<div class="torrent-smalldescr">
									<span title="测试电影副标题">测试电影副标题</span>
								</div>
							</td>
						</tr>
					</table>
				</td>
				<td class="rowfollow"></td>
				<td class="rowfollow nowrap">
					<span title="2026-01-18 12:00:00">1时前</span>
				</td>
				<td class="rowfollow">4.2 GB</td>
				<td class="rowfollow">120</td>
				<td class="rowfollow">10</td>
				<td class="rowfollow">500</td>
			</tr>
		</tbody>
	</table>
	</body>
	</html>
	`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	driver.BaseURL = server.URL

	req := NexusPHPRequest{Path: "/torrents.php", Method: "GET"}
	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "98765", items[0].ID)
	assert.Equal(t, "Test.Movie.2024.BluRay", items[0].Title)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.False(t, items[0].DiscountEndTime.IsZero(), "DiscountEndTime should be parsed from DOM element")
	assert.Equal(t, 2026, items[0].DiscountEndTime.Year())
	assert.Equal(t, 1, int(items[0].DiscountEndTime.Month()))
	assert.Equal(t, 25, items[0].DiscountEndTime.Day())
	assert.Equal(t, 18, items[0].DiscountEndTime.Hour())
	assert.Equal(t, 0, items[0].DiscountEndTime.Minute())
}

func TestParseDiscountEndTimeFromOnmouseover(t *testing.T) {
	tests := []struct {
		name        string
		onmouseover string
		wantYear    int
		wantMonth   int
		wantDay     int
		wantHour    int
		wantMinute  int
		wantSecond  int
		wantZero    bool
	}{
		{
			name:        "HDSky format with HTML entities",
			onmouseover: `domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;free&quot;&gt;免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-01-18 22:37:47&quot;&gt;1时19分&lt;/span&gt;&lt;/b&gt;', 'trail', false)`,
			wantYear:    2026,
			wantMonth:   1,
			wantDay:     18,
			wantHour:    22,
			wantMinute:  37,
			wantSecond:  47,
		},
		{
			name:        "HDSky format with regular quotes",
			onmouseover: `domTT_activate(this, event, 'content', '<b><font class="free">免费</font></b>优惠剩余时间：<b><span title="2026-01-21 08:42:24">2天11时</span></b>', 'trail', false)`,
			wantYear:    2026,
			wantMonth:   1,
			wantDay:     21,
			wantHour:    8,
			wantMinute:  42,
			wantSecond:  24,
		},
		{
			name:        "2xFree format",
			onmouseover: `domTT_activate(this, event, 'content', '&lt;b&gt;&lt;font class=&quot;twoupfree&quot;&gt;2X免费&lt;/font&gt;&lt;/b&gt;优惠剩余时间：&lt;b&gt;&lt;span title=&quot;2026-02-15 00:00:00&quot;&gt;27天&lt;/span&gt;&lt;/b&gt;', 'trail', false)`,
			wantYear:    2026,
			wantMonth:   2,
			wantDay:     15,
			wantHour:    0,
			wantMinute:  0,
			wantSecond:  0,
		},
		{
			name:        "empty string",
			onmouseover: "",
			wantZero:    true,
		},
		{
			name:        "no title attribute",
			onmouseover: `domTT_activate(this, event, 'content', 'some content without title')`,
			wantZero:    true,
		},
		{
			name:        "invalid date format",
			onmouseover: `domTT_activate(this, event, 'content', '&lt;span title=&quot;invalid-date&quot;&gt;')`,
			wantZero:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDiscountEndTimeFromOnmouseover(tt.onmouseover)
			if tt.wantZero {
				assert.True(t, result.IsZero(), "expected zero time")
			} else {
				assert.False(t, result.IsZero(), "expected non-zero time")
				assert.Equal(t, tt.wantYear, result.Year())
				assert.Equal(t, tt.wantMonth, int(result.Month()))
				assert.Equal(t, tt.wantDay, result.Day())
				assert.Equal(t, tt.wantHour, result.Hour())
				assert.Equal(t, tt.wantMinute, result.Minute())
				assert.Equal(t, tt.wantSecond, result.Second())
			}
		})
	}
}

func TestNexusPHPDriver_Execute_LoginPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><form action="takelogin.php"></form></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestNexusPHPDriver_Execute_2FA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>二次验证</title></head><body><form action="take2fa.php"></form></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, Err2FARequired)
}

func TestNexusPHPDriver_Execute_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestNexusPHPDriver_Execute_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestNexusPHPDriver_Execute_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "a=1", r.URL.RawQuery)
		_, _ = w.Write([]byte(`<html><body><table class="torrents"></table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	params := map[string][]string{"a": {"1"}}
	res, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php", Params: params})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	require.NotNil(t, res.Document)
}

func TestNexusPHPDriver_ParseDownload_LiveFetch(t *testing.T) {
	var torrentHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "download.php") {
			torrentHits++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("d8:announce"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})

	// Build a detail document with a relative download link.
	html := `<html><body><a href="download.php?id=5&passkey=abc">dl</a></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)

	data, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)
	assert.Equal(t, 1, torrentHits)
}

func TestNexusPHPDriver_ParseDownload_NoURL(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><body>nothing</body></html>`))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no download URL")
}

func TestNexusPHPDriver_ParseDownload_AbsoluteURL(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("d4:info"))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com", Cookie: "c=1"})
	html := `<html><body><a href="` + server.URL + `/download.php?id=1&hash=xx">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	data, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, []byte("d4:info"), data)
	assert.Equal(t, 1, hits)
}

func TestNexusPHPDriver_ParseDownload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	html := `<html><body><a href="/download.php?id=1&passkey=x">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestNexusPHPDriver_ParseDownload_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	html := `<html><body><a href="/download.php?id=1&passkey=x">dl</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	_, err := d.ParseDownload(NexusPHPResponse{Document: doc})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty torrent file")
}

func TestNexusPHPDriver_FetchSeedingStatus_ExecuteError(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "http://127.0.0.1:1", Cookie: "c=1"})
	_, _, err := d.FetchSeedingStatus(context.Background(), "42")
	require.Error(t, err)
}

func TestFilterNames(t *testing.T) {
	filters := []Filter{{Name: "parseSize"}, {Name: "regex", Args: []any{"x"}}}
	assert.Equal(t, []string{"parseSize", "regex"}, filterNames(filters))
	assert.Empty(t, filterNames(nil))
}

func TestBuildCurlCommand(t *testing.T) {
	cmd := buildCurlCommand("GET", "https://x.com/a", map[string]string{"Cookie": "c='v'"})
	assert.Contains(t, cmd, "curl -X GET")
	assert.Contains(t, cmd, "'https://x.com/a'")
	assert.Contains(t, cmd, "Cookie:")
}

func TestIsHexString(t *testing.T) {
	assert.True(t, isHexString("abcDEF0123456789"))
	assert.False(t, isHexString("xyz"))
	assert.True(t, isHexString(""))
}

func TestNewNexusPHPDriverWithFailover(t *testing.T) {
	GetGlobalRegistry().RegisterURLs(SiteName("failtest"), []string{"https://a.example", "https://b.example"})
	d := NewNexusPHPDriverWithFailover(SiteName("failtest"), "cookie=1")
	require.NotNil(t, d)
	assert.Equal(t, "https://a.example", d.BaseURL)
	assert.NotNil(t, d.failoverClient)
}

func TestNexusPHPDriver_ExecuteWithFailover(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table class="torrents"></table></body></html>`))
	}))
	defer good.Close()

	GetGlobalRegistry().RegisterURLs(SiteName("failtest2"), []string{good.URL})
	d := NewNexusPHPDriverWithFailover(SiteName("failtest2"), "cookie=1")
	require.NotNil(t, d.failoverClient)
	res, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	require.NoError(t, err)
	require.NotNil(t, res.Document)
}

// ---------------------------------------------------------------------------
// mtorrent_driver.go — UnmarshalJSON, ParseDownload, Execute failover,
// GetBonusPerHour/GetPeerStatistics error paths, mapMTorrentRole,
// getMTeamCategoryName, WithFailover
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_GetUserInfoWithDefinition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "userdetails.php"):
			_, _ = w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">上传量</td><td>1.50 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td>500.00 GB</td></tr>
			</table></body></html>`))
		default:
			_, _ = w.Write([]byte(`<html><body>
				<a href="userdetails.php?id=123">MyName</a>
			</body></html>`))
		}
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID:     "npdef",
		Name:   "NPDef",
		Schema: SchemaNexusPHP,
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"},
					Fields:        []string{"id", "name"},
				},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded", "downloaded"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id":   {Selector: []string{"a[href*='userdetails.php']"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name": {Selector: []string{"a[href*='userdetails.php']"}},
				"uploaded": {
					Selector: []string{"td.rowhead:contains('上传量') + td"},
					Filters:  []Filter{{Name: "parseSize"}},
				},
				"downloaded": {
					Selector: []string{"td.rowhead:contains('下载量') + td"},
					Filters:  []Filter{{Name: "parseSize"}},
				},
			},
		},
	}

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "123", info.UserID)
	assert.Equal(t, "MyName", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
}

func TestNexusPHPDriver_ExtractFieldValue_AttrAndDefault(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><a href="/details.php?id=42" title="mytitle">link</a><span class="lvl"></span></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	// href attribute
	v := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "href"})
	assert.Equal(t, "/details.php?id=42", v)

	// html attribute
	vh := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "html"})
	assert.Equal(t, "link", vh)

	// default text when no match
	vd := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"#missing"}, Text: "fallback"})
	assert.Equal(t, "fallback", vd)

	// text with filter
	vf := d.ExtractFieldValuePublic(doc, FieldSelector{Selector: []string{"a"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}})
	assert.Equal(t, "42", vf)
}

// ---------------------------------------------------------------------------
// hddolby_driver.go — GetTorrentDetail cache-miss refresh path + DownloadWithHash
// ---------------------------------------------------------------------------

func discountElem(t *testing.T, class, src, alt string) *goquery.Selection {
	t.Helper()
	html := `<html><body><img class="` + class + `" src="` + src + `" alt="` + alt + `"></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc.Find("img")
}

func TestParseDiscountFromElement_Branches(t *testing.T) {
	assert.Equal(t, Discount2xFree, parseDiscountFromElement(discountElem(t, "pro_free2up", "", ""), nil))
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "pro_free", "", ""), nil))
	assert.Equal(t, DiscountPercent50, parseDiscountFromElement(discountElem(t, "pro_50pctdown", "", ""), nil))
	assert.Equal(t, DiscountPercent30, parseDiscountFromElement(discountElem(t, "pro_30pctdown", "", ""), nil))
	assert.Equal(t, DiscountPercent70, parseDiscountFromElement(discountElem(t, "pro_70pctdown", "", ""), nil))
	assert.Equal(t, Discount2xUp, parseDiscountFromElement(discountElem(t, "pro_2up", "", ""), nil))
	assert.Equal(t, DiscountNone, parseDiscountFromElement(discountElem(t, "normal", "", ""), nil))

	// custom mapping wins
	custom := map[string]DiscountLevel{"specialtag": Discount2x50}
	assert.Equal(t, Discount2x50, parseDiscountFromElement(discountElem(t, "specialtag", "", ""), custom))

	// match via src/alt
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "", "free.gif", ""), nil))
	assert.Equal(t, DiscountFree, parseDiscountFromElement(discountElem(t, "", "", "FREE"), nil))
}

// ---------------------------------------------------------------------------
// ParseDetail — subtitle, info hash, custom selector, form-action strategies
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseDetail_SubtitleAndHash(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><table>
		<tr><td class="rowhead">下载链接</td><td><a href="download.php?id=9&passkey=k">dl</a></td></tr>
		<tr><td class="rowhead">副标题</td><td>My Subtitle</td></tr>
		<tr><td class="no_border_wide">Hash码: 303a850dedc19e60bd7cc814f60e0e28d7f2c202</td></tr>
	</table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "download.php")
	assert.Equal(t, "My Subtitle", detail.Subtitle)
	assert.Equal(t, "303a850dedc19e60bd7cc814f60e0e28d7f2c202", detail.InfoHash)
}

func TestNexusPHPDriver_ParseDetail_FormAction_Cov4(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
		<form action="download.php?id=55"></form>
	</body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "download.php?id=55")
}

func TestNexusPHPDriver_ParseDetail_NilDoc(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, err := d.ParseDetail(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

// ---------------------------------------------------------------------------
// base_site.go — Download error paths, GetUserInfo error
// ---------------------------------------------------------------------------

func TestIsLoginPage_Branches(t *testing.T) {
	panel, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div class="login-panel"></div>`))
	assert.True(t, isLoginPage(panel))

	title, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>登录</title><input name="username"><input name="password">`))
	assert.True(t, isLoginPage(title))

	meta, _ := goquery.NewDocumentFromReader(strings.NewReader(`<meta http-equiv="refresh" content="0;url=login.php">`))
	assert.True(t, isLoginPage(meta))

	normal, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div>hello</div>`))
	assert.False(t, isLoginPage(normal))
}

func TestIs2FAPage_Branches(t *testing.T) {
	script, _ := goquery.NewDocumentFromReader(strings.NewReader(`<script>window.location='take2fa.php'</script>`))
	assert.True(t, is2FAPage(script))

	form, _ := goquery.NewDocumentFromReader(strings.NewReader(`<form action="/take2fa"></form>`))
	assert.True(t, is2FAPage(form))

	title, _ := goquery.NewDocumentFromReader(strings.NewReader(`<title>两步验证</title>`))
	assert.True(t, is2FAPage(title))

	normal, _ := goquery.NewDocumentFromReader(strings.NewReader(`<div>hi</div>`))
	assert.False(t, is2FAPage(normal))
}

// ---------------------------------------------------------------------------
// ParseUserInfo / ParseUserDetails — NexusPHP transfer row parsing
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseUserInfo_InfoBlock(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
		<div id="info_block"><a class="User_Name" href="userdetails.php?id=88">Alice</a></div>
		<table><tr><td class="rowhead">上传量</td><td>2.00 TB</td></tr>
		<tr><td class="rowhead">下载量</td><td>1.00 TB</td></tr>
		<tr><td class="rowhead">分享率</td><td>2.00</td></tr>
		<tr><td class="rowhead">魔力值</td><td>5000</td></tr>
		<tr><td class="rowhead">等级</td><td>Power User</td></tr></table>
	</body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	info, err := d.ParseUserInfo(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "Alice", info.Username)
	assert.Equal(t, "88", info.UserID)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails_TransferRow_Cov4(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><table>
		<tr><td class="rowhead">用户名</td><td class="rowfollow">Bob</td></tr>
		<tr><td class="rowhead">传输</td><td class="rowfollow">上传量: 1.5 TB 下载量: 500 GB 分享率: 3.0</td></tr>
		<tr><td class="rowhead">魔力值</td><td class="rowfollow">123,456 (详情)</td></tr>
		<tr><td class="rowhead">等级</td><td class="rowfollow">Elite User</td></tr>
		<tr><td class="rowhead">上次访问</td><td class="rowfollow">2024-06-01 12:00:00</td></tr>
		<tr><td class="rowhead">上次登录</td><td class="rowfollow">2024-05-30 09:00:00</td></tr>
	</table></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	assert.Equal(t, "Elite User", info.Rank)
	assert.Greater(t, info.LastAccess, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
}

// ---------------------------------------------------------------------------
// unit3d_driver.go — ParseSearch full item mapping
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_FetchSeedingStatus_TableRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><table>
			<tr><th>name</th><th>x</th><th>size</th></tr>
			<tr><td>t1</td><td>-</td><td>1.00 GB</td></tr>
			<tr><td>t2</td><td>-</td><td>2.00 GB</td></tr>
		</table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "42")
	require.NoError(t, err)
	assert.Equal(t, 2, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_ParseSeedingStatus_PipeFormat(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><div><div>10 | 100 GB</div></div></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	seeding, size, err := d.ParseSeedingStatus(NexusPHPResponse{Document: doc, RawBody: []byte(html)})
	require.NoError(t, err)
	assert.Equal(t, 10, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_ParseSeedingStatus_NilDoc(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, _, err := d.ParseSeedingStatus(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}

// ---------------------------------------------------------------------------
// GuessUserLevelID — VIP & manager group branches
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_GetUserInfoWithDefinition_SessionExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><form action="takelogin.php"></form></body></html>`))
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID:     "npdef2",
		Schema: SchemaNexusPHP,
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{RequestConfig: RequestConfig{URL: "/index.php"}, Fields: []string{"id"}},
			},
			Selectors: map[string]FieldSelector{"id": {Selector: []string{"#id"}}},
		},
	}
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)
	_, err := d.GetUserInfo(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// NewDBUserInfoRepo — success path
// ---------------------------------------------------------------------------

func newLegacyUserInfoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "userdetails.php") {
			_, _ = w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">用户名</td><td class="rowfollow">LegacyUser</td></tr>
				<tr><td class="rowhead">上传量</td><td class="rowfollow">1.50 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td class="rowfollow">500.00 GB</td></tr>
			</table></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body>
			<div id="info_block"><a class="User_Name" href="userdetails.php?id=555">LegacyUser</a></div>
		</body></html>`))
	}))
}

func TestNexusPHPDriver_GetUserInfoLegacy(t *testing.T) {
	server := newLegacyUserInfoServer(t)
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	// no site definition -> legacy path
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "LegacyUser", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
}

// ---------------------------------------------------------------------------
// ParseDetail — DetailDownloadLink custom selector (form + link)
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ParseDetail_CustomSelector(t *testing.T) {
	sel := DefaultNexusPHPSelectors()
	sel.DetailDownloadLink = "a.customdl"
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com", Selectors: &sel})
	html := `<html><body><a class="customdl" href="/dl?id=1">go</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "/dl?id=1", detail.DownloadURL)
}

func TestNexusPHPDriver_ParseDetail_Strategy4IDOnly(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body><a href="download.php?id=42">go</a></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	detail, err := d.ParseDetail(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Contains(t, detail.DownloadURL, "id=42")
}

// ---------------------------------------------------------------------------
// registry.CreateSite — success creating each supported kind
// ---------------------------------------------------------------------------

func TestNexusPHPDriver_ExtractFieldValuePublic(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	doc := mustDoc(t, `<div id="v">hello</div>`)
	sel := FieldSelector{Selector: []string{"#v"}}
	assert.Equal(t, "hello", d.ExtractFieldValuePublic(doc, sel))
}

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc
}

func TestTruncateStr(t *testing.T) {
	assert.Equal(t, "abc", truncateStr("abc", 10))
	assert.Equal(t, "ab...", truncateStr("abcdef", 2))
}

func TestNexusPHPDriver_GetSiteDefinition(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	assert.Nil(t, d.GetSiteDefinition())
	def := &SiteDefinition{ID: "x"}
	d.SetSiteDefinition(def)
	assert.Equal(t, def, d.GetSiteDefinition())
}

func TestNexusPHPDriver_getSiteID(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://hdsky.me"})
	assert.Equal(t, "hdsky", d.getSiteID())
	d.SetSiteDefinition(&SiteDefinition{ID: "custom"})
	assert.Equal(t, "custom", d.getSiteID())
}

func TestExtractUserID(t *testing.T) {
	assert.Equal(t, "12345", extractUserID("userdetails.php?id=12345"))
	assert.Equal(t, "", extractUserID("no id here"))
}

func TestExtractTorrentIDFromURL(t *testing.T) {
	assert.Equal(t, "42", extractTorrentIDFromURL("https://x.com/details.php?id=42"))
	assert.Equal(t, "99", extractTorrentIDFromURL("https://x.com/torrents/99/name"))
	assert.Equal(t, "", extractTorrentIDFromURL("https://x.com/nothing"))
	assert.Equal(t, "", extractTorrentIDFromURL("://bad"))
}

func TestExtractSiteIDFromURL(t *testing.T) {
	assert.Equal(t, "hdsky", extractSiteIDFromURL("https://hdsky.me"))
	assert.Equal(t, "mteam", extractSiteIDFromURL("https://api.m-team.cc"))
	assert.Equal(t, "", extractSiteIDFromURL("://bad"))
	assert.Equal(t, "localhost", extractSiteIDFromURL("http://localhost:8080"))
}

func TestExtractNumber(t *testing.T) {
	assert.Equal(t, "123456", extractNumber("123,456 (详情)"))
	assert.Equal(t, "3.14", extractNumber("pi is 3.14"))
	assert.Equal(t, "", extractNumber("no digits"))
}

func TestExtractValueFromTransfer(t *testing.T) {
	text := "上传量: 1.5 TB 下载量: 500 GB 分享率: 3.5"
	assert.Equal(t, "3.5", extractValueFromTransfer(text, "分享率", "Ratio"))
	assert.Equal(t, "", extractValueFromTransfer(text, "NotThere"))
}

func TestExtractSizeFromTransfer(t *testing.T) {
	text := "上传量: 1.5 TB 下载量: 500 GB"
	up := extractSizeFromTransfer(text, "上传量", "Uploaded")
	assert.Greater(t, up, int64(0))
	assert.Equal(t, int64(0), extractSizeFromTransfer(text, "NotThere"))
}

func TestFindTextByLabel(t *testing.T) {
	doc := mustDoc(t, `<table><tr><td>上传量</td><td>1.5 TB</td></tr></table>`)
	assert.Equal(t, "1.5 TB", findTextByLabel(doc, "上传量"))
	assert.Equal(t, "", findTextByLabel(doc, "不存在"))
}

func TestFindInfoBlockValue(t *testing.T) {
	doc := mustDoc(t, `<div id="info_block">魔力值: 12345 | 上传量: 1.5 TB</div>`)
	assert.Equal(t, "12345", findInfoBlockValue(doc, "魔力值"))
	assert.Equal(t, "", findInfoBlockValue(doc, "不存在"))

	empty := mustDoc(t, `<div>nothing</div>`)
	assert.Equal(t, "", findInfoBlockValue(empty, "魔力值"))
}

func TestNexusPHPDriver_PrepareUserDetails(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareUserDetails("777")
	require.NoError(t, err)
	assert.Equal(t, "/userdetails.php", req.Path)
	assert.Equal(t, "777", req.Params.Get("id"))
}

func TestNexusPHPDriver_ParseUserInfo(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	// nil doc -> parse error
	_, err := d.ParseUserInfo(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)

	html := `<html><body>
	<div id="info_block">
		<a class="User_Name" href="userdetails.php?id=888">tester</a>
		上传量: 1.5 TB 下载量: 500 GB 分享率: 3.0 魔力值: 12345 等级: Power User
	</div>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserInfo(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, "888", info.UserID)
	assert.Greater(t, info.Uploaded, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	_, err := d.ParseUserDetails(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)

	html := `<html><body>
	<table>
		<tr><td class="rowhead">用户名</td><td class="rowfollow">tester</td></tr>
		<tr><td class="rowhead">上传量</td><td class="rowfollow">1.5 TB</td></tr>
		<tr><td class="rowhead">下载量</td><td class="rowfollow">500 GB</td></tr>
		<tr><td class="rowhead">分享率</td><td class="rowfollow">3.0</td></tr>
		<tr><td class="rowhead">魔力值</td><td class="rowfollow">12,345 (详情)</td></tr>
		<tr><td class="rowhead">等级</td><td class="rowfollow">Power User</td></tr>
		<tr><td class="rowhead">上次访问</td><td class="rowfollow">2024-06-01 12:00:00</td></tr>
		<tr><td class="rowhead">上次登录</td><td class="rowfollow">2024-06-02 09:00:00</td></tr>
	</table>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Equal(t, "tester", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	assert.InDelta(t, 12345, info.Bonus, 0.5)
	assert.Equal(t, "Power User", info.Rank)
	assert.Greater(t, info.LastAccess, int64(0))
	assert.Greater(t, info.LastLogin, int64(0))
}

func TestNexusPHPDriver_ParseUserDetails_TransferRow(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	html := `<html><body>
	<table>
		<tr><td class="rowhead">传输</td><td class="rowfollow">上传量: 2.0 TB 下载量: 1.0 TB 分享率: 2.0</td></tr>
		<tr><td class="rowhead">做种积分</td><td class="rowfollow">42</td></tr>
	</table>
	</body></html>`
	doc := mustDoc(t, html)
	info, err := d.ParseUserDetails(NexusPHPResponse{Document: doc})
	require.NoError(t, err)
	assert.Greater(t, info.Uploaded, int64(0))
	assert.Greater(t, info.Downloaded, int64(0))
	assert.InDelta(t, 2.0, info.Ratio, 0.01)
	assert.Equal(t, 42, info.Seeding)
}

func TestNexusPHPDriver_getUserInfoLegacy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if strings.Contains(r.URL.Path, "userdetails") {
			w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">用户名</td><td class="rowfollow">detailname</td></tr>
				<tr><td class="rowhead">上传量</td><td class="rowfollow">2.0 TB</td></tr>
				<tr><td class="rowhead">下载量</td><td class="rowfollow">1.0 TB</td></tr>
				<tr><td class="rowhead">分享率</td><td class="rowfollow">2.0</td></tr>
			</table></body></html>`))
			return
		}
		w.Write([]byte(`<html><body>
			<div id="info_block"><a class="User_Name" href="userdetails.php?id=888">tester</a></div>
		</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "detailname", info.Username)
	assert.Greater(t, info.Uploaded, int64(0))
}

func TestNexusPHPDriver_getUserInfoWithDefinition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.Contains(r.URL.Path, "userdetails"):
			w.Write([]byte(`<html><body><table>
				<tr><td class="rowhead">上传量</td><td class="rowfollow" id="up">91970600</td></tr>
			</table></body></html>`))
		case strings.Contains(r.URL.Path, "getusertorrentlistajax"):
			w.Write([]byte(`<html><body><b>10</b>条记录，共计<b>2.5 TB</b><table></table></body></html>`))
		default:
			w.Write([]byte(`<html><body><a id="uid" href="userdetails.php?id=888">tester</a></body></html>`))
		}
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID: "testsite",
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{
					RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"},
					Fields:        []string{"id", "name"},
				},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id":       {Selector: []string{"#uid"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name":     {Selector: []string{"#uid"}},
				"uploaded": {Selector: []string{"#up"}},
			},
		},
	}

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "888", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(91970600), info.Uploaded)
	// seeding status fetched via fallback since no seedingSize selector
	assert.Greater(t, info.SeederSize, int64(0))
}

func TestNexusPHPDriver_setUserInfoField(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	info := &UserInfo{}

	d.setUserInfoField(info, "id", "5")
	assert.Equal(t, "5", info.UserID)
	d.setUserInfoField(info, "name", "bob")
	assert.Equal(t, "bob", info.Username)
	d.setUserInfoField(info, "uploaded", "1.5 GB")
	assert.Greater(t, info.Uploaded, int64(0))
	d.setUserInfoField(info, "downloaded", "500 MB")
	assert.Greater(t, info.Downloaded, int64(0))
	d.setUserInfoField(info, "ratio", "3.0")
	assert.InDelta(t, 3.0, info.Ratio, 0.01)
	d.setUserInfoField(info, "bonus", "12345")
	assert.InDelta(t, 12345, info.Bonus, 0.5)
	d.setUserInfoField(info, "levelName", "PU")
	assert.Equal(t, "PU", info.LevelName)
	d.setUserInfoField(info, "seedingBonus", "100")
	assert.InDelta(t, 100, info.SeedingBonus, 0.5)
	d.setUserInfoField(info, "bonusPerHour", "5.5")
	assert.InDelta(t, 5.5, info.BonusPerHour, 0.01)
	d.setUserInfoField(info, "joinTime", "1600000000")
	assert.Equal(t, int64(1600000000), info.JoinDate)
	d.setUserInfoField(info, "lastAccessAt", "1700000000")
	assert.Equal(t, int64(1700000000), info.LastAccess)
	d.setUserInfoField(info, "lastLoginAt", "1710000000")
	assert.Equal(t, int64(1710000000), info.LastLogin)
	d.setUserInfoField(info, "messageCount", "3")
	assert.Equal(t, 3, info.UnreadMessageCount)
	d.setUserInfoField(info, "hnrUnsatisfied", "2")
	assert.Equal(t, 2, info.HnRUnsatisfied)
	d.setUserInfoField(info, "hnrPreWarning", "1")
	assert.Equal(t, 1, info.HnRPreWarning)
	d.setUserInfoField(info, "seeding", "10")
	assert.Equal(t, 10, info.Seeding)
	d.setUserInfoField(info, "leeching", "1")
	assert.Equal(t, 1, info.Leeching)
	d.setUserInfoField(info, "uploads", "7")
	assert.Equal(t, 7, info.Uploads)
	d.setUserInfoField(info, "trueUploaded", "1 GB")
	assert.Greater(t, info.TrueUploaded, int64(0))
	d.setUserInfoField(info, "trueDownloaded", "1 GB")
	assert.Greater(t, info.TrueDownloaded, int64(0))
	d.setUserInfoField(info, "seederSize", "1 GB")
	assert.Greater(t, info.SeederSize, int64(0))
	d.setUserInfoField(info, "leecherSize", "1 GB")
	assert.Greater(t, info.LeecherSize, int64(0))
}

func TestNexusPHPDriver_FetchSeedingStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><b>94</b>条记录，共计<b>2.756 TB</b><table></table></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "888")
	require.NoError(t, err)
	assert.Equal(t, 94, seeding)
	assert.Greater(t, size, int64(0))
}

func TestNexusPHPDriver_FetchSeedingStatus_NoTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>no table here</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	seeding, size, err := d.FetchSeedingStatus(context.Background(), "888")
	require.NoError(t, err)
	assert.Equal(t, 0, seeding)
	assert.Equal(t, int64(0), size)
}

func TestNexusPHPDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>
			<input name="torrent_name" value="My.Movie.2024">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2026-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：16.87 GB</td>
		</body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	item, err := d.GetTorrentDetail(context.Background(), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, "42", item.ID)
	assert.Equal(t, "My.Movie.2024", item.Title)
}

func TestNexusPHPDriver_GetTorrentDetail_IDFromLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "99", r.URL.Query().Get("id"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><input name="torrent_name" value="X"></body></html>`))
	}))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	_, err := d.GetTorrentDetail(context.Background(), "", server.URL+"/details.php?id=99", "")
	require.NoError(t, err)
}

func TestNexusPHPDriver_GetTorrentDetail_NoID(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})
	_, err := d.GetTorrentDetail(context.Background(), "", "", "")
	assert.Error(t, err)
}

func TestNexusPHPDriver_ParseDownload(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://x.com"})

	// raw body path (no document)
	data, err := d.ParseDownload(NexusPHPResponse{RawBody: []byte("torrentbytes")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrentbytes"), data)

	// nil doc + empty body -> error
	_, err = d.ParseDownload(NexusPHPResponse{})
	assert.ErrorIs(t, err, ErrParseError)
}
