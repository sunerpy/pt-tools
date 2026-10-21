package v2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestMTorrentDriver_ParseSearch_MallSingleFreeRequiresOngoingStatus(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{
		BaseURL: "https://api.m-team.cc",
		APIKey:  "test-api-key",
	})
	// 固定时间：基础半价到 2026-07-21 结束，过期的基础优惠不再有效
	fixedNow, err := ParseTimeInCST("2006-01-02 15:04:05", "2026-07-01 00:00:00")
	require.NoError(t, err)
	driver.now = func() time.Time { return fixedNow }

	res := MTorrentResponse{
		Code:    "0",
		Message: "success",
		Data: []byte(`{
			"data": [{
				"id": "1211993",
				"name": "Hell 2025",
				"size": "22967359242",
				"status": {
					"seeders": 53,
					"leechers": 3,
					"timesCompleted": 1,
					"discount": "PERCENT_50",
					"discountEndTime": "2026-07-21 05:26:41",
					"mallSingleFree": {
						"status": "ENDED",
						"startDate": "2000-01-01 00:00:00",
						"endDate": "2099-12-31 23:59:59"
					}
				},
				"category": "401"
			}],
			"total": 1
		}`),
	}

	items, err := driver.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, DiscountPercent50, items[0].DiscountLevel)
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

func TestParseMTorrentDiscountWithPromotion(t *testing.T) {
	tests := []struct {
		name             string
		baseDiscount     string
		baseEndTime      string
		promotion        *MTorrentPromotionRule
		wantLevel        DiscountLevel
		wantEndTimeAfter string
	}{
		{
			name:         "no promotion",
			baseDiscount: "FREE",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion:    nil,
			wantLevel:    DiscountFree,
		},
		{
			name:         "promotion better discount",
			baseDiscount: "PERCENT_50",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "FREE",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-05 23:59:59",
			},
			wantLevel:        DiscountFree,
			wantEndTimeAfter: "2026-02-05 23:00:00",
		},
		{
			name:         "same discount but promotion ends later",
			baseDiscount: "FREE",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "FREE",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-05 23:59:59",
			},
			wantLevel:        DiscountFree,
			wantEndTimeAfter: "2026-02-05 23:00:00",
		},
		{
			name:         "same discount but base ends later",
			baseDiscount: "FREE",
			baseEndTime:  "2026-02-06 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "FREE",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-05 23:59:59",
			},
			wantLevel:        DiscountFree,
			wantEndTimeAfter: "2026-02-06 19:00:00",
		},
		{
			name:         "promotion worse discount ignored",
			baseDiscount: "FREE",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "PERCENT_50",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-05 23:59:59",
			},
			wantLevel: DiscountFree,
		},
	}

	// Fixed time within all test promotion windows
	fixedNow, _ := ParseTimeInCST("2006-01-02 15:04:05", "2026-02-04 12:00:00")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, endTime := parseMTorrentDiscountWithPromotionAndMallSingleFreeAt(tt.baseDiscount, tt.baseEndTime, tt.promotion, nil, fixedNow)
			assert.Equal(t, tt.wantLevel, level)
			if tt.wantEndTimeAfter != "" {
				checkTime, _ := ParseTimeInCST("2006-01-02 15:04:05", tt.wantEndTimeAfter)
				assert.True(t, endTime.After(checkTime), "endTime %v should be after %v", endTime, checkTime)
			}
		})
	}
}

func TestParseMTorrentDiscountWithMallSingleFree(t *testing.T) {
	tests := []struct {
		name         string
		baseDiscount string
		baseEndTime  string
		promotion    *MTorrentPromotionRule
		mallSingle   *MallSingleFree
		wantLevel    DiscountLevel
		wantEndTime  string
	}{
		{
			name:         "mall single free active without promotion",
			baseDiscount: "PERCENT_50",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion:    nil,
			mallSingle: &MallSingleFree{
				Status:    "ONGOING",
				StartDate: "2026-02-03 00:00:00",
				EndDate:   "2026-02-05 23:59:59",
			},
			wantLevel:   DiscountFree,
			wantEndTime: "2026-02-05 23:59:59",
		},
		{
			name:         "mall single free inactive keeps base",
			baseDiscount: "PERCENT_50",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion:    nil,
			mallSingle: &MallSingleFree{
				Status:    "ONGOING",
				StartDate: "2026-02-06 00:00:00",
				EndDate:   "2026-02-07 00:00:00",
			},
			wantLevel:   DiscountPercent50,
			wantEndTime: "2026-02-05 19:57:34",
		},
		{
			// promotionRule 有优惠时也要看商城单种免费（以前会跳过）；两者都免费时免费期以结束更晚的为准
			name:         "promotion and mall single free both free: later end wins",
			baseDiscount: "PERCENT_50",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "FREE",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-05 22:22:22",
			},
			mallSingle: &MallSingleFree{
				Status:    "ONGOING",
				StartDate: "2026-02-03 00:00:00",
				EndDate:   "2026-02-05 23:59:59",
			},
			wantLevel:   DiscountFree,
			wantEndTime: "2026-02-05 23:59:59",
		},
		{
			name:         "promotion ends later than mall single free: promotion wins",
			baseDiscount: "PERCENT_50",
			baseEndTime:  "2026-02-05 19:57:34",
			promotion: &MTorrentPromotionRule{
				Discount:  "FREE",
				StartTime: "2026-02-03 00:00:00",
				EndTime:   "2026-02-06 22:22:22",
			},
			mallSingle: &MallSingleFree{
				Status:    "ONGOING",
				StartDate: "2026-02-03 00:00:00",
				EndDate:   "2026-02-05 23:59:59",
			},
			wantLevel:   DiscountFree,
			wantEndTime: "2026-02-06 22:22:22",
		},
	}

	fixedNow, _ := ParseTimeInCST("2006-01-02 15:04:05", "2026-02-04 12:00:00")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, endTime := parseMTorrentDiscountWithPromotionAndMallSingleFreeAt(tt.baseDiscount, tt.baseEndTime, tt.promotion, tt.mallSingle, fixedNow)
			assert.Equal(t, tt.wantLevel, level)

			wantEnd, err := ParseTimeInCST("2006-01-02 15:04:05", tt.wantEndTime)
			require.NoError(t, err)
			assert.Equal(t, wantEnd, endTime)
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

func TestFlexibleCode_UnmarshalJSON(t *testing.T) {
	var fc FlexibleCode
	require.NoError(t, fc.UnmarshalJSON([]byte(`"SUCCESS"`)))
	assert.Equal(t, "SUCCESS", fc.String())
	assert.True(t, fc.IsSuccess())

	require.NoError(t, fc.UnmarshalJSON([]byte(`0`)))
	assert.Equal(t, "0", fc.String())
	assert.True(t, fc.IsSuccess())

	require.NoError(t, fc.UnmarshalJSON([]byte(`42`)))
	assert.Equal(t, "42", fc.String())

	require.Error(t, fc.UnmarshalJSON([]byte(`{bad`)))
}

func TestFlexInt_UnmarshalJSON(t *testing.T) {
	var fi FlexInt
	require.NoError(t, fi.UnmarshalJSON([]byte(`123`)))
	assert.Equal(t, 123, fi.Int())

	require.NoError(t, fi.UnmarshalJSON([]byte(`"456"`)))
	assert.Equal(t, 456, fi.Int())

	require.NoError(t, fi.UnmarshalJSON([]byte(`""`)))
	assert.Equal(t, 0, fi.Int())

	require.Error(t, fi.UnmarshalJSON([]byte(`"notanint"`)))
	require.Error(t, fi.UnmarshalJSON([]byte(`{bad`)))
}

func TestMTorrentDriver_ParseDownload_Full(t *testing.T) {
	torrentServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		// minimal valid torrent bencode
		_, _ = w.Write([]byte("d8:announce4:test4:infod4:name4:teste"))
	}))
	defer torrentServer.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	res := MTorrentResponse{Code: "0", Data: []byte(`"` + torrentServer.URL + `/dl"`)}
	data, err := d.ParseDownload(res)
	// ValidateTorrentFile may reject; assert error surface is meaningful either way
	if err != nil {
		assert.Contains(t, err.Error(), "invalid torrent")
	} else {
		assert.NotEmpty(t, data)
	}
}

// genDlToken 的响应就是带 sign 的下载地址，不能无条件打印到 stdout。
func TestMTorrentDriver_Execute_DoesNotPrintDownloadToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"https://api.m-team.cc/api/rss/dlv2?sign=TOPSECRET"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	req, err := d.PrepareDownload("123")
	require.NoError(t, err)

	out := captureStdout(t, func() {
		_, err = d.Execute(context.Background(), req)
	})
	require.NoError(t, err)
	assert.NotContains(t, out, "TOPSECRET")
	assert.NotContains(t, out, "genDlToken")
}

// quickDownloadRetry 把下载种子文件的重试间隔调到 1 毫秒（测试不等真的退避）。
func quickDownloadRetry(t *testing.T) {
	t.Helper()
	old := downloadBackoff
	downloadBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { downloadBackoff = old })
}

// 拉种子文件的第二段请求要有超时，签名地址卡住时不能一直挂着。
func TestMTorrentDriver_ParseDownload_FetchTimesOut(t *testing.T) {
	quickDownloadRetry(t)
	old := mteamTorrentFetchTimeout
	mteamTorrentFetchTimeout = 100 * time.Millisecond
	t.Cleanup(func() { mteamTorrentFetchTimeout = old })

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	start := time.Now()
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/dl?sign=TOPSECRET"`)})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.NotContains(t, err.Error(), "TOPSECRET")
}

// 错误信息里的签名地址要脱敏，避免随日志泄露可复用的下载链接。
func TestMTorrentDriver_ParseDownload_RedactsSignedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/dl?sign=TOPSECRET"`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.NotContains(t, err.Error(), "TOPSECRET")
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	fn()

	require.NoError(t, w.Close())
	os.Stdout = orig
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestMTorrentDriver_ParseDownload_APIError(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "1", Message: "fail", RawBody: []byte("errbody")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestMTorrentDriver_ParseDownload_EmptyURL(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`""`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty download URL")
}

func TestMTorrentDriver_ParseDownload_BadData(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`{notstring}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse download URL")
}

func TestMTorrentDriver_ParseDownload_FetchError(t *testing.T) {
	quickDownloadRetry(t)
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"http://127.0.0.1:1/dl"`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch torrent file")
	assert.Contains(t, err.Error(), "(3 attempts)", "连不上是网络错误：重试两次")
	assert.ErrorIs(t, err, ErrNetworkError)
}

// 下载节点断开连接、回 5xx 是一时的：隔一会儿重拉同一个签名地址（不再取下载令牌），拉到就成。
func TestMTorrentDriver_ParseDownload_RetriesTransientFailures(t *testing.T) {
	quickDownloadRetry(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch hits.Add(1) {
		case 1:
			// 连接被重置：不回任何东西就断开
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close()
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write(testTorrentBytes)
		}
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	data, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/dl?sign=TOPSECRET"`)})
	require.NoError(t, err)
	assert.Equal(t, testTorrentBytes, data)
	assert.Equal(t, int32(3), hits.Load())
}

// 403 这类回答、不是种子文件的内容不是一时的：不重试。
func TestMTorrentDriver_ParseDownload_NoRetryOnRefusal(t *testing.T) {
	quickDownloadRetry(t)
	for _, c := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"403", http.StatusForbidden, "", "HTTP 403"},
		{"不是种子", http.StatusOK, "<html>下载次数超限</html>", "invalid torrent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()
			d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
			_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/dl"`)})
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
			assert.NotContains(t, err.Error(), "attempts")
			assert.Equal(t, int32(1), hits.Load())
		})
	}
}

// 签名地址跳到下载节点以后超时：错误里跳转后的地址也带签名（sign、playload），一样要脱敏。
func TestMTorrentDriver_ParseDownload_RedactsRedirectTarget(t *testing.T) {
	quickDownloadRetry(t)
	old := mteamTorrentFetchTimeout
	mteamTorrentFetchTimeout = 100 * time.Millisecond
	t.Cleanup(func() { mteamTorrentFetchTimeout = old })

	release := make(chan struct{})
	node := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer node.Close()
	defer close(release)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, node.URL+"/?app_id=1&playload=NODEPAYLOAD&sign=NODESIGN&t=1", http.StatusFound)
	}))
	defer api.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	_, err := d.ParseDownload(MTorrentResponse{Code: "0", Data: []byte(`"` + api.URL + `/api/rss/dlv2?sign=APISIGN&tid=1"`)})
	require.Error(t, err)
	for _, secret := range []string{"APISIGN", "NODEPAYLOAD", "NODESIGN"} {
		assert.NotContains(t, err.Error(), secret)
	}
	assert.Contains(t, err.Error(), "(3 attempts)")
	assert.ErrorIs(t, err, ErrNetworkError)
}

// 调用方取消了就不再重试：不等退避，马上返回。
func TestMTorrentDriver_ParseDownloadContext_StopsWhenCanceled(t *testing.T) {
	old, oldBackoff := mteamTorrentFetchTimeout, downloadBackoff
	mteamTorrentFetchTimeout, downloadBackoff = 100*time.Millisecond, []time.Duration{10 * time.Second, 10 * time.Second}
	t.Cleanup(func() { mteamTorrentFetchTimeout, downloadBackoff = old, oldBackoff })

	release := make(chan struct{})
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := d.ParseDownloadContext(ctx, MTorrentResponse{Code: "0", Data: []byte(`"` + server.URL + `/dl"`)})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "不等 10 秒的退避")
	assert.Equal(t, int32(1), hits.Load())
}

func TestMapMTorrentRole(t *testing.T) {
	assert.Equal(t, "Power User", mapMTorrentRole("2"))
	assert.Equal(t, "Nexus Master", mapMTorrentRole("9"))
	assert.Equal(t, "User", mapMTorrentRole("999"))
}

func TestGetMTeamCategoryName(t *testing.T) {
	assert.Equal(t, "unknowncat", getMTeamCategoryName("unknowncat"))
}

func TestNewMTorrentDriverWithFailover_Extra(t *testing.T) {
	GetGlobalRegistry().RegisterURLs(SiteName("mteam"), []string{"https://api.m-team.cc"})
	d := NewMTorrentDriverWithFailover("apikey")
	require.NotNil(t, d)
}

func TestMTorrentDriver_ExecuteWithFailover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
	}))
	defer server.Close()

	GetGlobalRegistry().RegisterURLs(SiteNameMTeam, []string{server.URL})
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k", UseFailover: true})
	if d.failoverClient == nil {
		t.Skip("failover client not initialized")
	}
	res, err := d.Execute(context.Background(), MTorrentRequest{Endpoint: "/api/x", Method: "POST"})
	require.NoError(t, err)
	assert.True(t, res.Code.IsSuccess())
}

func TestMTorrentDriver_GetBonusPerHour_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetBonusPerHour(context.Background())
	require.Error(t, err)
}

func TestMTorrentDriver_GetPeerStatistics_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.GetPeerStatistics(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// gazelle_driver.go — selectGazelleDetailTorrent branches, GetUserInfo,
// GetTorrentDetail with Torrents array
// ---------------------------------------------------------------------------

func TestMTorrentDriver_ExecuteDirectly_FormBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	res, err := d.Execute(context.Background(), MTorrentRequest{
		Endpoint:    "/api/x",
		Method:      "POST",
		Body:        map[string]any{"id": "5"},
		ContentType: "application/x-www-form-urlencoded",
	})
	require.NoError(t, err)
	assert.True(t, res.Code.IsSuccess())

	res2, err := d.Execute(context.Background(), MTorrentRequest{
		Endpoint:    "/api/x",
		Method:      "POST",
		Body:        "id=7",
		ContentType: "application/x-www-form-urlencoded",
	})
	require.NoError(t, err)
	assert.True(t, res2.Code.IsSuccess())
}

// ---------------------------------------------------------------------------
// SuggestBestDiscount — branches
// ---------------------------------------------------------------------------

func TestMTorrentDriver_GetUnreadMessageCount_Error(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, _, err := d.GetUnreadMessageCount(context.Background())
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// GetSiteNextLevelUnmet — seedingBonus + interval + bonusPerHour branches
// ---------------------------------------------------------------------------

func TestMTorrentDriver_GetSiteDefinition(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	assert.Nil(t, d.GetSiteDefinition())
	def := &SiteDefinition{ID: "mteam"}
	d.SetSiteDefinition(def)
	assert.Equal(t, def, d.GetSiteDefinition())
	assert.Equal(t, "mteam", d.getSiteID())
}

func TestMTorrentDriver_getSiteID_Default(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	assert.NotEmpty(t, d.getSiteID())
}

func TestNewMTorrentDriverWithFailover(t *testing.T) {
	d := NewMTorrentDriverWithFailover("apikey")
	require.NotNil(t, d)
	assert.Equal(t, "apikey", d.APIKey)
}

func TestTorrentResponsePreview(t *testing.T) {
	assert.Equal(t, "hello world", torrentResponsePreview([]byte("hello   world")))
	assert.Contains(t, torrentResponsePreview([]byte("d4:info")), "d4:info")
	long := strings.Repeat("a", 300)
	assert.LessOrEqual(t, len([]rune(torrentResponsePreview([]byte(long)))), 160)
}

func TestMTorrentDriver_ParseDownload_Errors(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})

	// API error code
	_, err := d.ParseDownload(MTorrentResponse{Code: FlexibleCode("1"), Message: "bad"})
	assert.Error(t, err)

	// bad JSON download URL
	_, err = d.ParseDownload(MTorrentResponse{Code: FlexibleCode("0"), Data: json.RawMessage(`{invalid`)})
	assert.Error(t, err)

	// empty URL
	_, err = d.ParseDownload(MTorrentResponse{Code: FlexibleCode("0"), Data: json.RawMessage(`""`)})
	assert.Error(t, err)
}

func TestMTorrentDriver_GetBonusPerHour(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "mybonus")
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"formulaParams":{"finalBs":"12.5"}}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	bph, err := d.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 12.5, bph, 0.01)
}

func TestMTorrentDriver_GetUnreadMessageCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"unMake":"3","count":"10"}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	unread, total, err := d.GetUnreadMessageCount(context.Background())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, unread, 0)
	assert.GreaterOrEqual(t, total, 0)
}

func TestMTorrentDriver_GetPeerStatistics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seederCount":"10","seederSize":"1073741824","leecherCount":"2","leecherSize":"536870912"}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	stats, err := d.GetPeerStatistics(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 10, stats.SeederCount)
	assert.Equal(t, int64(1073741824), stats.SeederSize)
}

func TestMTorrentDriver_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "member/profile"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
				"id":"1001","username":"tester","role":"2","createdDate":"2020-01-01 00:00:00",
				"memberCount":{"uploaded":"10737418240","downloaded":"1073741824","shareRate":"10.0","bonus":"5000"},
				"memberStatus":{"lastBrowse":"2024-06-01 12:00:00"}
			}}`))
		case strings.Contains(r.URL.Path, "mybonus"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"formulaParams":{"finalBs":"8.8"}}}`))
		case strings.Contains(r.URL.Path, "notify/statistic"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"unMake":"2","count":"5"}}`))
		case strings.Contains(r.URL.Path, "myPeerStatistics"):
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seederCount":"20","seederSize":"2048","leecherCount":"1","leecherSize":"512"}}`))
		default:
			w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
		}
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "1001", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, "Power User", info.Rank)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, 20, info.SeederCount)
	assert.Greater(t, info.LastAccess, int64(0))
}

// M1b：memberStatus.lastLogin 映射到 UserInfo.LastLogin（按站点时区 CST 解析）；访问时间的取法不变。
func TestMTorrentDriver_ParseUserInfo_LastLogin(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.example", APIKey: "k"})
	info, err := d.ParseUserInfo(MTorrentResponse{
		Code: "0",
		Data: []byte(`{"id":"1001","username":"tester","role":"2",
			"memberStatus":{"lastLogin":"2024-06-01 10:00:00","lastBrowse":"2024-06-01 12:00:00"}}`),
	})
	require.NoError(t, err)
	login, err := ParseTimeInCST("2006-01-02 15:04:05", "2024-06-01 10:00:00")
	require.NoError(t, err)
	browse, err := ParseTimeInCST("2006-01-02 15:04:05", "2024-06-01 12:00:00")
	require.NoError(t, err)
	assert.Equal(t, login.Unix(), info.LastLogin)
	assert.Equal(t, browse.Unix(), info.LastAccess)

	info, err = d.ParseUserInfo(MTorrentResponse{Code: "0", Data: []byte(`{"id":"1","username":"t","memberStatus":{"lastLogin":"not-a-time"}}`)})
	require.NoError(t, err)
	assert.Zero(t, info.LastLogin, "an unparsable lastLogin is left empty")
}

func TestMTorrentDriver_GetUserInfo_ProfileError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"1","message":"denied"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.GetUserInfo(context.Background())
	assert.Error(t, err)
}

func TestMTorrentDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "torrent/detail")
		w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
			"id":"555","name":"Detail.Movie","size":"2147483648","smallDescr":"desc",
			"status":{"seeders":30,"leechers":3,"timesCompleted":99,"discount":"FREE"}
		}}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	item, err := d.GetTorrentDetail(context.Background(), "555", "", "")
	require.NoError(t, err)
	assert.Equal(t, "555", item.ID)
	assert.Equal(t, "Detail.Movie", item.Title)
	assert.Equal(t, int64(2147483648), item.SizeBytes)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Equal(t, []string{"desc"}, item.Tags)
	assert.Equal(t, 30, item.Seeders)
}

func TestMTorrentDriver_GetTorrentDetail_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":"500","message":"error"}`))
	}))
	defer server.Close()

	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.GetTorrentDetail(context.Background(), "1", "", "")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// M-Team 优惠严格解析：基础优惠、promotionRule（起止时间与适用范围）、mallSingleFree
// ---------------------------------------------------------------------------

// mteamAt 把北京时间字符串解析成时间
func mteamAt(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := ParseTimeInCST("2006-01-02 15:04:05", s)
	require.NoError(t, err)
	return v
}

func TestEvaluateMTorrentDiscount(t *testing.T) {
	now := mteamAt(t, "2026-10-16 12:00:00")
	const (
		past        = "2026-10-15 12:00:00"
		startPast   = "2026-10-01 00:00:00"
		startFuture = "2026-10-17 00:00:00"
		future1     = "2026-10-18 12:00:00"
		future2     = "2026-10-20 12:00:00"
	)
	promo := func(discount, start, end string) *MTorrentPromotionRule {
		return &MTorrentPromotionRule{Discount: discount, StartTime: start, EndTime: end}
	}
	scoped := func(categories, teams []string) *MTorrentPromotionRule {
		return &MTorrentPromotionRule{
			Discount: "FREE", StartTime: startPast, EndTime: future2,
			Categories: MTorrentIDList{Values: categories}, Teams: MTorrentIDList{Values: teams},
		}
	}
	ongoing := func(start, end string) *MallSingleFree {
		return &MallSingleFree{Status: "ONGOING", StartDate: start, EndDate: end}
	}

	cases := []struct {
		name        string
		discount    string
		discountEnd string
		promotion   *MTorrentPromotionRule
		mall        *MallSingleFree
		category    string
		team        string

		wantLevel    DiscountLevel
		wantEnd      string       // 空表示没有结束时间
		wantValidity FreeValidity // item.Promotion(now) 的规范状态
		wantSource   string
		wantErr      []string // DiscountErr 应包含的说明；为空表示不应有说明
	}{
		// 基础优惠
		{name: "基础免费有将来的结束时间", discount: "FREE", discountEnd: future1, wantLevel: DiscountFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "base"},
		{name: "基础2X免费", discount: "_2X_FREE", discountEnd: future1, wantLevel: Discount2xFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "base"},
		{name: "基础免费已过期不再有效", discount: "FREE", discountEnd: past, wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "基础半价已过期同样失效", discount: "PERCENT_50", discountEnd: past, wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "基础免费恰好在现在结束", discount: "FREE", discountEnd: "2026-10-16 12:00:00", wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "基础免费没有结束时间是未知", discount: "FREE", wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "base"},
		{name: "基础免费结束时间非法是未知并记下原因", discount: "FREE", discountEnd: "2026/10/18 12:00", wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "base", wantErr: []string{"discountEndTime: 时间格式无法解析"}},
		{name: "基础半价有将来的结束时间", discount: "PERCENT_50", discountEnd: future1, wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "没有任何优惠", discount: "NORMAL", wantLevel: DiscountNone, wantValidity: FreeNotFree},

		// promotionRule：接口没有状态字段，只按起止时间判断
		{name: "促销的更好优惠覆盖基础优惠", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startPast, future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "促销还没开始不生效", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startFuture, future2), wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "促销已结束不生效", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startPast, past), wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "促销结束时间非法是未知而不是长期免费", discount: "NORMAL", promotion: promo("FREE", startPast, "not-a-time"), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "promotion_rule", wantErr: []string{"promotionRule.endTime: 时间格式无法解析"}},
		{name: "促销没有结束时间是未知", discount: "NORMAL", promotion: promo("FREE", startPast, ""), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "promotion_rule"},
		{name: "促销开始时间非法是未知", discount: "NORMAL", promotion: promo("FREE", "bad-start", future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUnknown, wantSource: "promotion_rule", wantErr: []string{"promotionRule.startTime: 时间格式无法解析"}},
		{name: "促销开始时间非法但已经结束仍不生效", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", "bad-start", past), wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base", wantErr: []string{"promotionRule.startTime: 时间格式无法解析"}},
		{name: "促销没有开始时间不限制开始", discount: "NORMAL", promotion: promo("FREE", "", future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "促销与基础同级时取结束更晚的促销", discount: "FREE", discountEnd: future1, promotion: promo("FREE", startPast, future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "促销与基础同级时取结束更晚的基础", discount: "FREE", discountEnd: future2, promotion: promo("FREE", startPast, future1), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "base"},
		{name: "同为免费时可信的结束时间优先于未知", discount: "FREE", promotion: promo("FREE", startPast, future1), wantLevel: DiscountFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "下载比例更低的未知候选胜出但仍是未知", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startPast, "bad-end"), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "promotion_rule", wantErr: []string{"promotionRule.endTime: 时间格式无法解析"}},
		{name: "促销更差的优惠不覆盖基础免费", discount: "FREE", discountEnd: future1, promotion: promo("PERCENT_50", startPast, future2), wantLevel: DiscountFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "base"},
		{name: "多处时间非法都记下", discount: "FREE", discountEnd: "bad-base", promotion: promo("FREE", startPast, "bad-end"), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "base", wantErr: []string{"discountEndTime: 时间格式无法解析; promotionRule.endTime: 时间格式无法解析"}},

		// promotionRule 的适用范围：categories、teams 为空表示全站
		{name: "全站促销生效", discount: "PERCENT_50", discountEnd: future1, promotion: scoped(nil, nil), category: "402", team: "44", wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "限定其他分类的促销不生效", discount: "PERCENT_50", discountEnd: future1, promotion: scoped([]string{"401", "419"}, nil), category: "402", wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "限定本分类的促销生效", discount: "PERCENT_50", discountEnd: future1, promotion: scoped([]string{"401", "402"}, nil), category: "402", wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "限定制作组且不含本种子的促销不生效", discount: "PERCENT_50", discountEnd: future1, promotion: scoped(nil, []string{"9"}), category: "402", team: "44", wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "限定本制作组的促销生效", discount: "NORMAL", promotion: scoped([]string{"402"}, []string{"44"}), category: "402", team: "44", wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "promotion_rule"},
		{name: "有分类范围但种子缺分类时不生效", discount: "NORMAL", promotion: scoped([]string{"402"}, nil), wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "有制作组范围但种子缺制作组时不生效", discount: "NORMAL", promotion: scoped(nil, []string{"44"}), category: "402", wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "范围解不开时不生效并记下原因", discount: "NORMAL", promotion: &MTorrentPromotionRule{Discount: "FREE", StartTime: startPast, EndTime: future2, Categories: MTorrentIDList{Invalid: true}}, category: "402", wantLevel: DiscountNone, wantValidity: FreeNotFree, wantErr: []string{"promotionRule.categories: 格式无法解析"}},

		// mallSingleFree：只认 ONGOING，起止时间与 promotionRule 同样处理
		{name: "商城单种免费进行中且在区间内", discount: "NORMAL", mall: ongoing(startPast, future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "商城单种免费已结束不生效", discount: "PERCENT_50", discountEnd: future1, mall: ongoing(startPast, past), wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "商城单种免费状态不是进行中不生效", discount: "PERCENT_50", discountEnd: future1, mall: &MallSingleFree{Status: "ENDED", StartDate: startPast, EndDate: future2}, wantLevel: DiscountPercent50, wantEnd: future1, wantValidity: FreeNotFree, wantSource: "base"},
		{name: "商城单种免费状态不区分大小写", discount: "NORMAL", mall: &MallSingleFree{Status: " ongoing ", StartDate: startPast, EndDate: future2}, wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "商城单种免费还没开始不生效", discount: "NORMAL", mall: ongoing(startFuture, future2), wantLevel: DiscountNone, wantValidity: FreeNotFree},
		{name: "商城单种免费没有开始时间不限制开始", discount: "NORMAL", mall: ongoing("", future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "商城单种免费没有结束时间是未知而不是长期免费", discount: "NORMAL", mall: ongoing(startPast, ""), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "mall_single_free"},
		{name: "商城单种免费结束时间非法是未知", discount: "NORMAL", mall: ongoing(startPast, "bad-end"), wantLevel: DiscountFree, wantValidity: FreeUnknown, wantSource: "mall_single_free", wantErr: []string{"mallSingleFree.endDate: 时间格式无法解析"}},
		{name: "促销还没开始时仍看商城单种免费", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startFuture, future2), mall: ongoing(startPast, future1), wantLevel: DiscountFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "促销优惠更差时仍看商城单种免费", discount: "NORMAL", promotion: promo("PERCENT_50", startPast, future2), mall: ongoing(startPast, future1), wantLevel: DiscountFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "促销与商城都免费时取结束更晚的", discount: "PERCENT_50", discountEnd: future1, promotion: promo("FREE", startPast, future1), mall: ongoing(startPast, future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "mall_single_free"},
		{name: "基础2X免费不被商城单种免费降级", discount: "_2X_FREE", discountEnd: future2, mall: ongoing(startPast, future1), wantLevel: Discount2xFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "base"},
		{name: "同样的结束时间时2X免费优先于免费", discount: "_2X_FREE", discountEnd: future1, mall: ongoing(startPast, future1), wantLevel: Discount2xFree, wantEnd: future1, wantValidity: FreeUntil, wantSource: "base"},
		{name: "商城单种免费结束更晚时免费期以它为准", discount: "_2X_FREE", discountEnd: future1, mall: ongoing(startPast, future2), wantLevel: DiscountFree, wantEnd: future2, wantValidity: FreeUntil, wantSource: "mall_single_free"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tor := &MTorrentTorrent{Category: c.category, Team: MTorrentID(c.team)}
			tor.Status.Discount = c.discount
			tor.Status.DiscountEndTime = c.discountEnd
			tor.Status.PromotionRule = c.promotion
			tor.Status.MallSingleFree = c.mall

			var item TorrentItem
			applyMTorrentDiscount(&item, evaluateMTorrentDiscount(tor, now))

			assert.Equal(t, c.wantLevel, item.DiscountLevel)
			if c.wantEnd == "" {
				assert.True(t, item.DiscountEndTime.IsZero(), "end = %s", item.DiscountEndTime)
			} else {
				assert.True(t, mteamAt(t, c.wantEnd).Equal(item.DiscountEndTime), "end = %s", item.DiscountEndTime)
			}
			assert.Equal(t, c.wantSource, item.DiscountSource)
			if len(c.wantErr) == 0 {
				assert.Empty(t, item.DiscountErr)
			}
			for _, e := range c.wantErr {
				assert.Contains(t, item.DiscountErr, e)
			}
			// 诊断说明只写字段名与错误类型，不带原始值
			raw := []string{c.discountEnd}
			if c.promotion != nil {
				raw = append(raw, c.promotion.StartTime, c.promotion.EndTime)
			}
			if c.mall != nil {
				raw = append(raw, c.mall.StartDate, c.mall.EndDate)
			}
			for _, v := range raw {
				if v != "" {
					assert.NotContains(t, item.DiscountErr, v)
				}
			}

			snap := item.Promotion(now)
			assert.Equal(t, c.wantValidity, snap.Validity)
			assert.NotEqual(t, FreePermanent, snap.Validity, "M-Team 没有长期有效的明确标记，不能得出长期免费")
			assert.Equal(t, c.wantSource, snap.Source)
		})
	}
}

func TestMTorrentIDListAndID_Decode(t *testing.T) {
	var rule MTorrentPromotionRule
	require.NoError(t, json.Unmarshal([]byte(`{"discount":"FREE","categories":[401," 402 "],"teams":null}`), &rule))
	assert.Equal(t, []string{"401", "402"}, rule.Categories.Values)
	assert.False(t, rule.Categories.Invalid)
	assert.True(t, rule.Categories.covers("402"))
	assert.False(t, rule.Categories.covers("419"))
	assert.False(t, rule.Categories.covers(""), "种子缺分类时无法证明覆盖")
	assert.Empty(t, rule.Teams.Values)
	assert.True(t, rule.Teams.covers("44"), "空范围表示全站")

	// 解不开的范围不让整条结果解析失败，但促销不适用
	require.NoError(t, json.Unmarshal([]byte(`{"discount":"FREE","categories":"402","teams":[{"id":1}]}`), &rule))
	assert.True(t, rule.Categories.Invalid)
	assert.False(t, rule.Categories.covers("402"))
	assert.True(t, rule.Teams.Invalid)
	assert.False(t, rule.Teams.covers("1"))

	var tor MTorrentTorrent
	require.NoError(t, json.Unmarshal([]byte(`{"id":"1","category":"402","team":44}`), &tor))
	assert.Equal(t, MTorrentID("44"), tor.Team)
	require.NoError(t, json.Unmarshal([]byte(`{"id":"1","category":"402","team":" 9 "}`), &tor))
	assert.Equal(t, MTorrentID("9"), tor.Team)
	require.NoError(t, json.Unmarshal([]byte(`{"id":"1","category":"402","team":{"id":9}}`), &tor))
	assert.Equal(t, MTorrentID(""), tor.Team, "解不开的制作组当作没有")
}

// 走真实的解码路径：搜索结果的字段形状来自接口实测（脱敏，不含 API Key、下载地址或用户数据）
func TestMTorrentDriver_ParseSearch_StrictDiscount(t *testing.T) {
	driver := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "test-api-key"})
	now := mteamAt(t, "2026-10-16 12:00:00")
	driver.now = func() time.Time { return now }

	raw := `{"data":[
		{"id":"101","name":"A","size":"1024","category":"402","team":"44",
		 "status":{"discount":"FREE","discountEndTime":null,"promotionRule":null,"mallSingleFree":null}},
		{"id":"102","name":"B","size":"1024","category":"402",
		 "status":{"discount":"FREE","discountEndTime":"2026/10/20 12:00:00"}},
		{"id":"103","name":"C","size":"1024","category":"402",
		 "status":{"discount":"FREE","discountEndTime":"2026-10-15 12:00:00"}},
		{"id":"104","name":"D","size":"1024","category":"402","team":44,
		 "status":{"discount":"PERCENT_50","discountEndTime":"2026-10-18 12:00:00",
		  "promotionRule":{"id":"7","categories":[],"teams":[],"discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":null,"operator":"x"}}},
		{"id":"105","name":"E","size":"1024","category":"402",
		 "status":{"discount":"PERCENT_50","discountEndTime":"2026-10-18 12:00:00",
		  "promotionRule":{"id":"8","categories":[401,"419"],"teams":[],"discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":"2026-10-20 12:00:00"}}},
		{"id":"106","name":"F","size":"1024","category":"402",
		 "status":{"discount":"_2X_FREE","discountEndTime":"2026-10-18 12:00:00"}},
		{"id":"107","name":"G","size":"1024","category":"402",
		 "status":{"discount":"NORMAL","mallSingleFree":{"status":"ONGOING","endDate":"2026-10-20 12:00:00","isAdult":false}}},
		{"id":"108","name":"H","size":"1024","category":"402","team":"44",
		 "status":{"discount":"NORMAL",
		  "promotionRule":{"categories":[402],"teams":["9"],"discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":"2026-10-20 12:00:00"}}},
		{"id":"109","name":"I","size":"1024","category":"402",
		 "status":{"discount":"NORMAL",
		  "promotionRule":{"categories":"402","discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":"2026-10-20 12:00:00"}}},
		{"id":"110","name":"J","size":"1024","category":"402",
		 "status":{"discount":"PERCENT_50","discountEndTime":"2026-10-18 12:00:00",
		  "promotionRule":{"categories":[402],"teams":[],"discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":"2026-10-20 12:00:00"},
		  "mallSingleFree":{"status":"ONGOING","startDate":"2026-10-01 00:00:00","endDate":"","isAdult":true}}}
	],"total":10}`
	items, err := driver.ParseSearch(MTorrentResponse{Code: "0", Message: "success", Data: json.RawMessage(raw)})
	require.NoError(t, err)
	require.Len(t, items, 10)
	byID := make(map[string]TorrentItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}

	cases := []struct {
		id           string
		wantLevel    DiscountLevel
		wantEnd      string
		wantValidity FreeValidity
		wantSource   string
		wantErr      string
	}{
		{"101", DiscountFree, "", FreeUnknown, "base", ""},                              // 结束时间是 null：未知，不是长期免费
		{"102", DiscountFree, "", FreeUnknown, "base", "discountEndTime: 时间格式无法解析"},     // 结束时间格式不对
		{"103", DiscountNone, "", FreeNotFree, "", ""},                                  // 基础免费已过期
		{"104", DiscountFree, "", FreeUnknown, "promotion_rule", ""},                    // 促销有开始没结束（null）：未知
		{"105", DiscountPercent50, "2026-10-18 12:00:00", FreeNotFree, "base", ""},      // 促销只覆盖别的分类
		{"106", Discount2xFree, "2026-10-18 12:00:00", FreeUntil, "base", ""},           // _2X_FREE
		{"107", DiscountFree, "2026-10-20 12:00:00", FreeUntil, "mall_single_free", ""}, // 商城单种免费没有 startDate
		{"108", DiscountNone, "", FreeNotFree, "", ""},                                  // 分类覆盖但制作组不覆盖
		{"109", DiscountNone, "", FreeNotFree, "", "promotionRule.categories: 格式无法解析"},  // 范围不是数组
		{"110", DiscountFree, "2026-10-20 12:00:00", FreeUntil, "promotion_rule", ""},   // 可信的促销优先于没有结束时间的商城单种免费
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			item, ok := byID[c.id]
			require.True(t, ok)
			assert.Equal(t, c.wantLevel, item.DiscountLevel)
			if c.wantEnd == "" {
				assert.True(t, item.DiscountEndTime.IsZero(), "end = %s", item.DiscountEndTime)
			} else {
				assert.True(t, mteamAt(t, c.wantEnd).Equal(item.DiscountEndTime), "end = %s", item.DiscountEndTime)
			}
			assert.Equal(t, c.wantSource, item.DiscountSource)
			assert.Equal(t, c.wantErr, item.DiscountErr)
			assert.Equal(t, c.wantValidity, item.Promotion(now).Validity)
			assert.True(t, item.FetchedAt.IsZero(), "搜索结果不写 FetchedAt")
		})
	}
}

func TestMTorrentDriver_GetTorrentDetail_StrictDiscount(t *testing.T) {
	responses := map[string]string{
		// 基础半价已过期；促销限定本分类、有可信结束时间；商城单种免费进行中但没有结束时间
		"555": `{"code":"0","message":"SUCCESS","data":{
			"id":"555","name":"Detail.Movie","size":"2147483648","category":"402","team":"44",
			"status":{"seeders":30,"leechers":3,"timesCompleted":99,"discount":"PERCENT_50","discountEndTime":"2026-10-15 12:00:00",
				"promotionRule":{"categories":["402"],"teams":[],"discount":"FREE","startTime":"2026-10-01 00:00:00","endTime":"2026-10-20 12:00:00"},
				"mallSingleFree":{"status":"ONGOING","endDate":"","isAdult":false}}}}`,
		// 基础免费的结束时间无法解析
		"556": `{"code":"0","message":"SUCCESS","data":{
			"id":"556","name":"Bad.Time","size":"1024","category":"402",
			"status":{"discount":"FREE","discountEndTime":"16/10/2026"}}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		id := strings.TrimPrefix(string(body), "id=")
		w.Write([]byte(responses[id]))
	}))
	defer server.Close()

	now := mteamAt(t, "2026-10-16 12:00:00")
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: server.URL, APIKey: "k"})
	d.now = func() time.Time { return now }

	item, err := d.GetTorrentDetail(context.Background(), "555", "", "")
	require.NoError(t, err)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.True(t, mteamAt(t, "2026-10-20 12:00:00").Equal(item.DiscountEndTime))
	assert.Equal(t, "promotion_rule", item.DiscountSource)
	assert.Empty(t, item.DiscountErr)
	assert.Equal(t, FreeUntil, item.Promotion(now).Validity)
	assert.True(t, now.Equal(item.FetchedAt), "详情是实时请求，FetchedAt 是取得的时间：%s", item.FetchedAt)

	item, err = d.GetTorrentDetail(context.Background(), "556", "", "")
	require.NoError(t, err)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.True(t, item.DiscountEndTime.IsZero())
	assert.Equal(t, FreeUnknown, item.DiscountValidity)
	assert.Equal(t, "discountEndTime: 时间格式无法解析", item.DiscountErr)
	assert.Equal(t, FreeUnknown, item.Promotion(now).Validity)
}

// 下载令牌接口返回异常时，错误里的原始响应要把签名下载地址的凭证参数打码（错误会进日志）。
func TestMTorrentParseDownload_ErrorsHideSignedURL(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	const secret = "s1GnSECRET0123456789"
	cases := []MTorrentResponse{
		{Code: "0", Data: json.RawMessage(`{"url":"https://api.m-team.cc/api/rss/dlv2?sign=` + secret + `&t=1"}`)},
		{Code: "1", Message: "busy", RawBody: []byte(`{"code":"1","data":"https://api.m-team.cc/api/rss/dlv2?sign=` + secret + `"}`)},
		{Code: "1", Message: "请改用 https://api.m-team.cc/api/rss/dlv2?sign=" + secret, RawBody: []byte(`{"code":"1"}`)},
	}
	for _, res := range cases {
		_, err := d.ParseDownload(res)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), secret)
		assert.Contains(t, err.Error(), "api.m-team.cc", "地址的其余部分照样留着便于排查")
	}
}
