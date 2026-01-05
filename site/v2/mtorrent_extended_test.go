package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseBonusPerHour tests bonus per hour parsing
func TestParseBonusPerHour(t *testing.T) {
	driver := &MTorrentDriver{}

	t.Run("valid bonus response with float", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"formulaParams": {
					"finalBs": 123.456
				}
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		bonus, err := driver.ParseBonusPerHour(res)
		assert.NoError(t, err)
		assert.InDelta(t, 123.456, bonus, 0.001)
	})

	t.Run("valid bonus response with string", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"formulaParams": {
					"finalBs": "456.789"
				}
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		bonus, err := driver.ParseBonusPerHour(res)
		assert.NoError(t, err)
		assert.InDelta(t, 456.789, bonus, 0.001)
	})

	t.Run("integer bonus value", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"formulaParams": {
					"finalBs": 500
				}
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		bonus, err := driver.ParseBonusPerHour(res)
		assert.NoError(t, err)
		assert.InDelta(t, 500.0, bonus, 0.001)
	})

	t.Run("zero bonus", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"formulaParams": {
					"finalBs": 0
				}
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		bonus, err := driver.ParseBonusPerHour(res)
		assert.NoError(t, err)
		assert.InDelta(t, 0.0, bonus, 0.001)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		responseBody := []byte(`invalid json`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		_, err := driver.ParseBonusPerHour(res)
		assert.Error(t, err)
	})

	t.Run("API error code", func(t *testing.T) {
		res := MTorrentResponse{
			Code:    "1",
			Message: "Error",
		}

		_, err := driver.ParseBonusPerHour(res)
		assert.Error(t, err)
	})
}

// TestParseUnreadMessageCount tests unread message count parsing
func TestParseUnreadMessageCount(t *testing.T) {
	driver := &MTorrentDriver{}

	t.Run("valid message count", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"count": "10",
				"unMake": "5"
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		unread, total, err := driver.ParseUnreadMessageCount(res)
		assert.NoError(t, err)
		assert.Equal(t, 5, unread)
		assert.Equal(t, 10, total)
	})

	t.Run("zero messages", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"count": "8",
				"unMake": "0"
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		unread, total, err := driver.ParseUnreadMessageCount(res)
		assert.NoError(t, err)
		assert.Equal(t, 0, unread)
		assert.Equal(t, 8, total)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		responseBody := []byte(`invalid json`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		_, _, err := driver.ParseUnreadMessageCount(res)
		assert.Error(t, err)
	})

	t.Run("API error code", func(t *testing.T) {
		res := MTorrentResponse{
			Code:    "1",
			Message: "Error",
		}

		_, _, err := driver.ParseUnreadMessageCount(res)
		assert.Error(t, err)
	})
}

// TestParsePeerStatistics tests peer statistics parsing
func TestParsePeerStatistics(t *testing.T) {
	driver := &MTorrentDriver{}

	t.Run("valid peer statistics", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"seederCount": "100",
				"seederSize": "1099511627776",
				"leecherCount": "50",
				"leecherSize": "549755813888"
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		stats, err := driver.ParsePeerStatistics(res)
		assert.NoError(t, err)
		assert.Equal(t, 100, stats.SeederCount)
		assert.Equal(t, int64(1099511627776), stats.SeederSize)
		assert.Equal(t, 50, stats.LeecherCount)
		assert.Equal(t, int64(549755813888), stats.LeecherSize)
	})

	t.Run("zero statistics", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"seederCount": "0",
				"seederSize": "0",
				"leecherCount": "0",
				"leecherSize": "0"
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		stats, err := driver.ParsePeerStatistics(res)
		assert.NoError(t, err)
		assert.Equal(t, 0, stats.SeederCount)
		assert.Equal(t, int64(0), stats.SeederSize)
		assert.Equal(t, 0, stats.LeecherCount)
		assert.Equal(t, int64(0), stats.LeecherSize)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		responseBody := []byte(`invalid json`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		_, err := driver.ParsePeerStatistics(res)
		assert.Error(t, err)
	})

	t.Run("API error code", func(t *testing.T) {
		res := MTorrentResponse{
			Code:    "1",
			Message: "Error",
		}

		_, err := driver.ParsePeerStatistics(res)
		assert.Error(t, err)
	})

	t.Run("large values", func(t *testing.T) {
		responseBody := []byte(`{
			"code": "0",
			"message": "SUCCESS",
			"data": {
				"seederCount": "9999",
				"seederSize": "10995116277760",
				"leecherCount": "5000",
				"leecherSize": "5497558138880"
			}
		}`)

		res := MTorrentResponse{
			Code:    "0",
			RawBody: responseBody,
		}

		stats, err := driver.ParsePeerStatistics(res)
		assert.NoError(t, err)
		assert.Equal(t, 9999, stats.SeederCount)
		assert.Equal(t, int64(10995116277760), stats.SeederSize)
		assert.Equal(t, 5000, stats.LeecherCount)
		assert.Equal(t, int64(5497558138880), stats.LeecherSize)
	})
}

// TestUserInfoWithExtendedFields tests the UserInfo structure with extended fields
func TestUserInfoWithExtendedFields(t *testing.T) {
	t.Run("create user info with extended fields", func(t *testing.T) {
		info := UserInfo{
			Username:           "testuser",
			Uploaded:           1024 * 1024 * 1024,
			Downloaded:         512 * 1024 * 1024,
			Bonus:              1000.5,
			BonusPerHour:       50.25,
			UnreadMessageCount: 3,
			SeederCount:        100,
			SeederSize:         1099511627776,
			LeecherCount:       50,
			LeecherSize:        549755813888,
		}

		assert.Equal(t, "testuser", info.Username)
		assert.InDelta(t, 50.25, info.BonusPerHour, 0.001)
		assert.Equal(t, 3, info.UnreadMessageCount)
		assert.Equal(t, 100, info.SeederCount)
	})
}
