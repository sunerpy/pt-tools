package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMTorrentDriver(t *testing.T) {
	config := MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	}

	driver := NewMTorrentDriver(config)

	assert.Equal(t, "https://api.m-team.cc", driver.BaseURL)
	assert.Equal(t, "test-api-key", driver.APIKey)
	assert.NotNil(t, driver.httpClient)
}

func TestMTorrentDriver_PrepareSearch(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	tests := []struct {
		name     string
		query    SearchQuery
		wantMode string
		wantKey  string
		wantPage int
	}{
		{
			name:     "basic search",
			query:    SearchQuery{Keyword: "test"},
			wantMode: "normal",
			wantKey:  "test",
			wantPage: 1,
		},
		{
			name:     "with page",
			query:    SearchQuery{Keyword: "test", Page: 3},
			wantMode: "normal",
			wantKey:  "test",
			wantPage: 3,
		},
		{
			name:     "empty keyword",
			query:    SearchQuery{},
			wantMode: "normal",
			wantKey:  "",
			wantPage: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := driver.PrepareSearch(tt.query)
			require.NoError(t, err)
			assert.Equal(t, "/api/torrent/search", req.Endpoint)
			assert.Equal(t, "POST", req.Method)

			body := req.Body.(MTorrentSearchRequest)
			assert.Equal(t, tt.wantMode, body.Mode)
			assert.Equal(t, tt.wantKey, body.Keyword)
			assert.Equal(t, tt.wantPage, body.PageNumber)
		})
	}
}

func TestMTorrentDriver_Execute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "POST", r.Method)

		resp := MTorrentResponse{
			Code:    "0",
			Message: "success",
			Data:    json.RawMessage(`{"data":[],"total":0}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
	})

	req := MTorrentRequest{
		Endpoint: "/torrent/search",
		Method:   "POST",
		Body:     MTorrentSearchRequest{Mode: "normal"},
	}

	res, err := driver.Execute(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, FlexibleCode("0"), res.Code)
	assert.Equal(t, "success", res.Message)
}

func TestMTorrentDriver_Execute_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: server.URL,
		APIKey:  "invalid-key",
	})

	req := MTorrentRequest{Endpoint: "/api/test", Method: "POST"}
	_, err := driver.Execute(context.Background(), req)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestMTorrentDriver_ParseSearch(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	// Use JSON to create test data to avoid struct type issues with FlexInt
	searchDataJSON := `{
		"data": [{
			"id": "12345",
			"name": "Test Movie 2024",
			"size": "1073741824",
			"createdDate": "2024-01-15 10:30:00",
			"status": {
				"seeders": 100,
				"leechers": 10,
				"timesCompleted": 500,
				"discount": "FREE"
			},
			"category": "401"
		}],
		"total": 1
	}`

	dataBytes := []byte(searchDataJSON)
	res := MTorrentResponse{
		Code:    "0",
		Message: "success",
		Data:    dataBytes,
	}

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, "12345", items[0].ID)
	assert.Equal(t, "Test Movie 2024", items[0].Title)
	assert.Equal(t, int64(1073741824), items[0].SizeBytes)
	assert.Equal(t, 100, items[0].Seeders)
	assert.Equal(t, 10, items[0].Leechers)
	assert.Equal(t, 500, items[0].Snatched)
	assert.Equal(t, DiscountFree, items[0].DiscountLevel)
	assert.Equal(t, "电影/SD", items[0].Category) // Should be mapped from 401
}

func TestMTorrentDriver_ParseSearch_APIError(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	res := MTorrentResponse{
		Code:    "1",
		Message: "Invalid API key",
	}

	_, err := driver.ParseSearch(res)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestMTorrentDriver_PrepareUserInfo(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "/api/member/profile", req.Endpoint)
	assert.Equal(t, "POST", req.Method)
}

func TestMTorrentDriver_ParseUserInfo(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	userData := MTorrentUserInfo{
		ID:          "12345",
		Username:    "testuser",
		CreatedDate: "2020-01-01 00:00:00",
		Role:        "2",
		MemberCount: MTorrentMemberCount{
			Uploaded:   "1099511627776", // 1 TB
			Downloaded: "549755813888",  // 512 GB
			ShareRate:  "2.0",
			Bonus:      "10000.5",
		},
		MemberStatus: MTorrentMemberStatus{
			LastBrowse: "2024-01-15 10:30:00",
		},
	}

	dataBytes, _ := json.Marshal(userData)
	res := MTorrentResponse{
		Code:    "0",
		Message: "success",
		Data:    dataBytes,
	}

	info, err := driver.ParseUserInfo(res)
	require.NoError(t, err)

	assert.Equal(t, "12345", info.UserID)
	assert.Equal(t, "testuser", info.Username)
	assert.Equal(t, int64(1099511627776), info.Uploaded)
	assert.Equal(t, int64(549755813888), info.Downloaded)
	assert.Equal(t, 2.0, info.Ratio)
	assert.Equal(t, 10000.5, info.Bonus)
	assert.Equal(t, "Power User", info.Rank)
}

func TestMTorrentDriver_PrepareDownload(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	req, err := driver.PrepareDownload("12345")
	require.NoError(t, err)
	assert.Equal(t, "/api/torrent/genDlToken", req.Endpoint)
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "application/x-www-form-urlencoded", req.ContentType)

	// M-Team genDlToken API uses form-urlencoded format
	body := req.Body.(string)
	assert.Equal(t, "id=12345", body)
}

func TestParseMTorrentDiscount(t *testing.T) {
	tests := []struct {
		input    string
		expected DiscountLevel
	}{
		{"FREE", DiscountFree},
		{"free", DiscountFree},
		{"2XFREE", Discount2xFree},
		{"_2X_FREE", Discount2xFree},
		{"PERCENT_50", DiscountPercent50},
		{"50%", DiscountPercent50},
		{"PERCENT_30", DiscountPercent30},
		{"30%", DiscountPercent30},
		{"PERCENT_70", DiscountPercent70},
		{"70%", DiscountPercent70},
		{"2XUP", Discount2xUp},
		{"_2X_UP", Discount2xUp},
		{"_2X_PERCENT_50", Discount2x50},
		{"2X50%", Discount2x50},
		{"NORMAL", DiscountNone},
		{"", DiscountNone},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseMTorrentDiscount(tt.input))
		})
	}
}

// TestMTorrentDriver_PrepareExtendedEndpoints tests all extended API endpoint paths
func TestMTorrentDriver_PrepareExtendedEndpoints(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	t.Run("PrepareGetBonusPerHour", func(t *testing.T) {
		req, err := driver.PrepareGetBonusPerHour()
		require.NoError(t, err)
		assert.Equal(t, "/api/tracker/mybonus", req.Endpoint)
		assert.Equal(t, "POST", req.Method)
	})

	t.Run("PrepareGetUnreadMessageCount", func(t *testing.T) {
		req, err := driver.PrepareGetUnreadMessageCount()
		require.NoError(t, err)
		assert.Equal(t, "/api/msg/notify/statistic", req.Endpoint)
		assert.Equal(t, "POST", req.Method)
	})

	t.Run("PrepareGetPeerStatistics", func(t *testing.T) {
		req, err := driver.PrepareGetPeerStatistics()
		require.NoError(t, err)
		assert.Equal(t, "/api/tracker/myPeerStatistics", req.Endpoint)
		assert.Equal(t, "POST", req.Method)
	})
}

// TestMTorrentDriver_AllEndpointsHaveAPIPrefix verifies all endpoints have /api prefix
func TestMTorrentDriver_AllEndpointsHaveAPIPrefix(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	endpoints := []struct {
		name     string
		getReq   func() (MTorrentRequest, error)
		expected string
	}{
		{"Search", func() (MTorrentRequest, error) { return driver.PrepareSearch(SearchQuery{}) }, "/api/torrent/search"},
		{"UserInfo", driver.PrepareUserInfo, "/api/member/profile"},
		{"Download", func() (MTorrentRequest, error) { return driver.PrepareDownload("123") }, "/api/torrent/genDlToken"},
		{"BonusPerHour", driver.PrepareGetBonusPerHour, "/api/tracker/mybonus"},
		{"UnreadMessageCount", driver.PrepareGetUnreadMessageCount, "/api/msg/notify/statistic"},
		{"PeerStatistics", driver.PrepareGetPeerStatistics, "/api/tracker/myPeerStatistics"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req, err := ep.getReq()
			require.NoError(t, err)
			assert.Equal(t, ep.expected, req.Endpoint, "Endpoint should have /api prefix")
			assert.True(t, len(req.Endpoint) > 4 && req.Endpoint[:4] == "/api", "Endpoint must start with /api")
		})
	}
}

// ============================================================================
// Property-Based Tests for Response Parsing
// ============================================================================

// TestMTorrentDriver_ParsePeerStatistics_Property tests that peer statistics
// response parsing correctly handles string values for all fields.
// Property 1: PeerStatistics Response Parsing
// Validates: Requirements 3.1
func TestMTorrentDriver_ParsePeerStatistics_Property(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	// Test cases with various string number formats
	testCases := []struct {
		name         string
		seederCount  string
		seederSize   string
		leecherCount string
		leecherSize  string
	}{
		{"zero values", "0", "0", "0", "0"},
		{"small values", "10", "1024", "5", "512"},
		{"large values", "283", "11715703791635", "0", "0"},
		{"mixed values", "100", "1099511627776", "50", "549755813888"},
		{"max int values", "2147483647", "9223372036854775807", "2147483647", "9223372036854775807"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build raw response body
			rawBody := []byte(`{
				"code": "0",
				"message": "SUCCESS",
				"data": {
					"uid": "12345",
					"seederCount": "` + tc.seederCount + `",
					"seederSize": "` + tc.seederSize + `",
					"leecherCount": "` + tc.leecherCount + `",
					"leecherSize": "` + tc.leecherSize + `",
					"uploadCount": "0"
				}
			}`)

			res := MTorrentResponse{
				Code:    "0",
				Message: "SUCCESS",
				RawBody: rawBody,
			}

			stats, err := driver.ParsePeerStatistics(res)
			require.NoError(t, err)
			require.NotNil(t, stats)

			// Verify parsed values match expected
			expectedSeederCount, _ := parseInt(tc.seederCount)
			expectedSeederSize, _ := parseInt64(tc.seederSize)
			expectedLeecherCount, _ := parseInt(tc.leecherCount)
			expectedLeecherSize, _ := parseInt64(tc.leecherSize)

			assert.Equal(t, expectedSeederCount, stats.SeederCount, "SeederCount should match")
			assert.Equal(t, expectedSeederSize, stats.SeederSize, "SeederSize should match")
			assert.Equal(t, expectedLeecherCount, stats.LeecherCount, "LeecherCount should match")
			assert.Equal(t, expectedLeecherSize, stats.LeecherSize, "LeecherSize should match")
		})
	}
}

// Helper functions for parsing
func parseInt(s string) (int, error) {
	i, err := strconv.Atoi(s)
	return i, err
}

func parseInt64(s string) (int64, error) {
	i, err := strconv.ParseInt(s, 10, 64)
	return i, err
}

// TestMTorrentDriver_ParseUnreadMessageCount_Property tests that message statistics
// response parsing correctly handles string values for count and unMake fields.
// Property 2: Message Statistics Response Parsing
// Validates: Requirements 3.2
func TestMTorrentDriver_ParseUnreadMessageCount_Property(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	// Test cases with various string number formats
	testCases := []struct {
		name           string
		count          string
		unMake         string
		expectedTotal  int
		expectedUnread int
	}{
		{"zero values", "0", "0", 0, 0},
		{"no unread", "8", "0", 8, 0},
		{"some unread", "100", "5", 100, 5},
		{"all unread", "10", "10", 10, 10},
		{"large values", "9999", "123", 9999, 123},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build raw response body
			rawBody := []byte(`{
				"code": "0",
				"message": "SUCCESS",
				"data": {
					"count": "` + tc.count + `",
					"unMake": "` + tc.unMake + `"
				}
			}`)

			res := MTorrentResponse{
				Code:    "0",
				Message: "SUCCESS",
				RawBody: rawBody,
			}

			unread, total, err := driver.ParseUnreadMessageCount(res)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedUnread, unread, "Unread count should match")
			assert.Equal(t, tc.expectedTotal, total, "Total count should match")
		})
	}
}

// TestMTorrentDriver_ParseBonusPerHour_Property tests that bonus response parsing
// correctly handles finalBs field which may be string or number.
// Property 3: Bonus Response Parsing (Flexible Number)
// Validates: Requirements 3.3
func TestMTorrentDriver_ParseBonusPerHour_Property(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})

	// Test cases with both string and number formats for finalBs
	testCases := []struct {
		name          string
		finalBsValue  string // The raw JSON value (could be "123.45" or 123.45)
		expectedBonus float64
	}{
		{"number zero", "0", 0},
		{"number integer", "100", 100},
		{"number float", "123.45", 123.45},
		{"number large", "9999.99", 9999.99},
		{"string zero", `"0"`, 0},
		{"string integer", `"100"`, 100},
		{"string float", `"123.45"`, 123.45},
		{"string large", `"9999.99"`, 9999.99},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build raw response body with finalBs as either string or number
			rawBody := []byte(`{
				"code": "0",
				"message": "SUCCESS",
				"data": {
					"formulaParams": {
						"finalBs": ` + tc.finalBsValue + `
					}
				}
			}`)

			res := MTorrentResponse{
				Code:    "0",
				Message: "SUCCESS",
				RawBody: rawBody,
			}

			bonus, err := driver.ParseBonusPerHour(res)
			require.NoError(t, err)

			assert.InDelta(t, tc.expectedBonus, bonus, 0.001, "Bonus should match expected value")
		})
	}
}
