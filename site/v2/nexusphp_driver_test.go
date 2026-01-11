package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	// Test that normal pages are not detected as login pages
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
