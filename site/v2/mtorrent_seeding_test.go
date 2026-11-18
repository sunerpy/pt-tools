package v2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mteamSeedingServer 按 2026-11-02 在 M-Team 上看到的返回拼：myPeerStatistics 对 API Key 全是 "0"，
// myPeerStatus 给页头「當前活動」的做种数与下载数，getUserTorrentList（个人页「目前做種」）分页给出每个种子的体积。
type mteamSeedingServer struct {
	seeder, leecher string
	pages           [][]int64 // 每页各个种子的体积
	failStatus      bool
	failList        bool
	failBonus       bool
	failMessages    bool
	// minGap 不为 0 时，两次列表请求隔得比它近就回「請求過於頻繁」（2026-11-02 在 M-Team 上连着读两页时遇到的）
	minGap time.Duration
	// limitOnce 为真时第 2 页头一次请求回「請求過於頻繁」
	limitOnce bool

	mu        sync.Mutex
	listCalls []map[string]any
	lastList  time.Time
	limited   int
}

func (s *mteamSeedingServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.Contains(r.URL.Path, "member/profile"):
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
			"id":"1001","username":"tester","role":"2","createdDate":"2020-01-01 00:00:00",
			"memberCount":{"uploaded":"10737418240","downloaded":"1073741824","shareRate":"10.0","bonus":"5000"},
			"memberStatus":{"lastBrowse":"2024-06-01 12:00:00"}}}`))
	case strings.Contains(r.URL.Path, "mybonus"):
		if s.failBonus {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"formulaParams":{"finalBs":"8.8"}}}`))
	case strings.Contains(r.URL.Path, "notify/statistic"):
		if s.failMessages {
			_, _ = w.Write([]byte(`{"code":"1","message":"ERROR"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"unMake":"0","count":"5"}}`))
	case strings.Contains(r.URL.Path, "myPeerStatistics"):
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seederCount":"0","seederSize":"0","leecherCount":"0","leecherSize":"0","uploadCount":"0"}}`))
	case strings.Contains(r.URL.Path, "myPeerStatus"):
		if s.failStatus {
			_, _ = w.Write([]byte(`{"code":"1","message":"ERROR"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"seeder":"` + s.seeder + `","leecher":"` + s.leecher + `"}}`))
	case strings.Contains(r.URL.Path, "getUserTorrentList"):
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		s.mu.Lock()
		s.listCalls = append(s.listCalls, req)
		tooSoon := s.minGap > 0 && !s.lastList.IsZero() && time.Since(s.lastList) < s.minGap
		s.lastList = time.Now()
		limitNow := s.limitOnce && s.limited == 0 && int(req["pageNumber"].(float64)) == 2
		if tooSoon || limitNow {
			s.limited++
		}
		s.mu.Unlock()
		if tooSoon || limitNow {
			_, _ = w.Write([]byte(`{"code":"4","message":"請求過於頻繁"}`))
			return
		}
		if s.failList {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		page := int(req["pageNumber"].(float64))
		total := 0
		for _, p := range s.pages {
			total += len(p)
		}
		rows := make([]string, 0)
		if page >= 1 && page <= len(s.pages) {
			for i, size := range s.pages[page-1] {
				rows = append(rows, `{"torrent":{"id":"`+strconv.Itoa(page*1000+i)+`","name":"x","size":"`+strconv.FormatInt(size, 10)+`"},"peer":{"left":"0"}}`)
			}
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"pageNumber":"` + strconv.Itoa(page) +
			`","pageSize":"100","total":"` + strconv.Itoa(total) + `","totalPages":"` + strconv.Itoa(len(s.pages)) +
			`","data":[` + strings.Join(rows, ",") + `]}}`))
	default:
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{}}`))
	}
}

func (s *mteamSeedingServer) start(t *testing.T) *MTorrentDriver {
	t.Helper()
	old := mtorrentSeedingPageDelay
	mtorrentSeedingPageDelay = 0
	t.Cleanup(func() { mtorrentSeedingPageDelay = old })
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	t.Cleanup(srv.Close)
	return NewMTorrentDriver(MTorrentDriverConfig{BaseURL: srv.URL, APIKey: "k"})
}

// myPeerStatistics 现在对 API Key 全返回 0：做种数、下载数改从 myPeerStatus 取，做种体积把「目前做種」列表各页的体积加起来。
func TestMTorrentDriver_GetUserInfo_SeedingFromPeerStatusAndList(t *testing.T) {
	s := &mteamSeedingServer{seeder: "3", leecher: "1", pages: [][]int64{{1 << 30, 2 << 30}, {4 << 30}}}
	d := s.start(t)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, info.Seeding)
	assert.Equal(t, 3, info.SeederCount)
	assert.Equal(t, 1, info.Leeching)
	assert.Equal(t, 1, info.LeecherCount)
	assert.Equal(t, int64(7)<<30, info.SeederSize)

	require.Len(t, s.listCalls, 2, "两页都读")
	for i, call := range s.listCalls {
		assert.EqualValues(t, 1001, call["userid"], "用个人资料里的用户编号")
		assert.Equal(t, "SEEDING", call["type"])
		assert.EqualValues(t, i+1, call["pageNumber"])
		assert.EqualValues(t, mtorrentSeedingPageSize, call["pageSize"])
	}
}

// 没有在做种时不读列表，体积是 0。
func TestMTorrentDriver_GetUserInfo_NoSeedingSkipsList(t *testing.T) {
	s := &mteamSeedingServer{seeder: "0", leecher: "0"}
	d := s.start(t)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Zero(t, info.Seeding)
	assert.Zero(t, info.SeederSize)
	assert.Empty(t, s.listCalls)
}

// 做种列表没取到：做种数照样有，体积按没取到算（只取到一部分，保存时沿用上一次的），不是 0。
func TestMTorrentDriver_GetUserInfo_SeedingListFailureIsPartial(t *testing.T) {
	s := &mteamSeedingServer{seeder: "3", leecher: "1", failList: true}
	d := s.start(t)

	info, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.True(t, errors.As(err, &partial), "err=%v", err)
	assert.Equal(t, []string{"seederSize"}, partial.Missing)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, 3, info.Seeding)
}

// myPeerStatus 没取到：做种数用列表的总条数，下载数按没取到算。
func TestMTorrentDriver_GetUserInfo_PeerStatusFailureUsesListTotal(t *testing.T) {
	s := &mteamSeedingServer{failStatus: true, pages: [][]int64{{1 << 30, 1 << 30}}}
	d := s.start(t)

	info, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.True(t, errors.As(err, &partial), "err=%v", err)
	assert.Equal(t, []string{"leeching"}, partial.Missing)
	assert.Equal(t, 2, info.Seeding)
	assert.Equal(t, int64(2)<<30, info.SeederSize)
}

// 做种太多、页数超过上限时不读到一半就算：体积按没取到算。
func TestMTorrentDriver_GetUserInfo_TooManySeedingPages(t *testing.T) {
	pages := make([][]int64, mtorrentMaxSeedingPages+1)
	for i := range pages {
		pages[i] = []int64{1}
	}
	s := &mteamSeedingServer{seeder: strconv.Itoa(len(pages)), leecher: "0", pages: pages}
	d := s.start(t)

	_, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.True(t, errors.As(err, &partial), "err=%v", err)
	assert.Equal(t, []string{"seederSize"}, partial.Missing)
	assert.LessOrEqual(t, len(s.listCalls), 1, "看到总页数超过上限就不再往下读")
}

// 时魔、未读消息没取到时同样按没取到算，不当成 0（未读消息 0 会被保存成「没有未读」）。
func TestMTorrentDriver_GetUserInfo_BonusAndMessagesFailureArePartial(t *testing.T) {
	s := &mteamSeedingServer{seeder: "0", leecher: "0", failBonus: true, failMessages: true}
	d := s.start(t)

	_, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.True(t, errors.As(err, &partial), "err=%v", err)
	assert.ElementsMatch(t, []string{"bonusPerHour", "messageCount"}, partial.Missing)
}

// 连着读两页会被 M-Team 限流（「請求過於頻繁」）：每页之间隔 mtorrentSeedingPageDelay。
func TestMTorrentDriver_GetSeedingSize_PacesPages(t *testing.T) {
	s := &mteamSeedingServer{seeder: "3", leecher: "0", pages: [][]int64{{1 << 30}, {1 << 30}, {1 << 30}}, minGap: 30 * time.Millisecond}
	d := s.start(t)
	mtorrentSeedingPageDelay = 40 * time.Millisecond

	count, size, err := d.GetSeedingSize(context.Background(), "1001")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
	assert.Equal(t, int64(3)<<30, size)
	assert.Zero(t, s.limited, "每页之间隔开了，没被限流")
}

// 被限流的那一页隔一会儿再读一次。
func TestMTorrentDriver_GetSeedingSize_RetriesRateLimitedPage(t *testing.T) {
	s := &mteamSeedingServer{seeder: "2", leecher: "0", pages: [][]int64{{1 << 30}, {2 << 30}}, limitOnce: true}
	d := s.start(t)

	count, size, err := d.GetSeedingSize(context.Background(), "1001")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Equal(t, int64(3)<<30, size)
	assert.Equal(t, 1, s.limited)
	assert.Len(t, s.listCalls, 3, "第 2 页读了两次")
}

// mteamRawDriver 的每个请求由 respond 按路径与请求体回（HTTP 状态码与原样的返回），给读不出来的各种样子用。
func mteamRawDriver(t *testing.T, respond func(path string, body map[string]any) (int, string)) *MTorrentDriver {
	t.Helper()
	old := mtorrentSeedingPageDelay
	mtorrentSeedingPageDelay = 0
	t.Cleanup(func() { mtorrentSeedingPageDelay = old })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		status, payload := respond(r.URL.Path, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	}))
	t.Cleanup(srv.Close)
	return NewMTorrentDriver(MTorrentDriverConfig{BaseURL: srv.URL, APIKey: "k"})
}

// myPeerStatus 读不出来时报错，不当成 0。
func TestMTorrentDriver_GetPeerStatus_Failures(t *testing.T) {
	cases := map[string]struct {
		status  int
		payload string
	}{
		"http error":            {http.StatusBadGateway, ``},
		"data is not an object": {http.StatusOK, `{"code":"0","message":"SUCCESS","data":"oops"}`},
		"seeder is not whole":   {http.StatusOK, `{"code":"0","message":"SUCCESS","data":{"seeder":"1.5","leecher":"1"}}`},
		"leecher is not whole":  {http.StatusOK, `{"code":"0","message":"SUCCESS","data":{"seeder":"1","leecher":"1.5"}}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := mteamRawDriver(t, func(string, map[string]any) (int, string) { return c.status, c.payload })
			_, _, err := d.GetPeerStatus(context.Background())
			assert.Error(t, err)
		})
	}
}

// 做种列表读不出来的各种样子都报错（体积按没取到算），不把读到一半的当总数。
func TestMTorrentDriver_GetSeedingSize_Failures(t *testing.T) {
	page := func(total, pages, size string) string {
		return `{"code":"0","message":"SUCCESS","data":{"total":"` + total + `","totalPages":"` + pages +
			`","data":[{"torrent":{"size":"` + size + `"}}]}}`
	}
	cases := map[string]string{
		"error code":             `{"code":"1","message":"ERROR"}`,
		"rate limited twice":     `{"code":"4","message":"請求過於頻繁"}`,
		"data is not an object":  `{"code":"0","message":"SUCCESS","data":"oops"}`,
		"total pages not whole":  page("1", "1.5", "1024"),
		"torrent size not whole": page("1", "1", "1.5"),
		"total count not whole":  page("1.5", "1", "1024"),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			d := mteamRawDriver(t, func(string, map[string]any) (int, string) { return http.StatusOK, payload })
			_, _, err := d.GetSeedingSize(context.Background(), "1001")
			assert.Error(t, err)
		})
	}

	t.Run("user id is not a number", func(t *testing.T) {
		called := false
		d := mteamRawDriver(t, func(string, map[string]any) (int, string) { called = true; return http.StatusOK, page("1", "1", "1") })
		_, _, err := d.GetSeedingSize(context.Background(), "tester")
		assert.Error(t, err)
		assert.False(t, called, "没有用户编号就不请求")
	})
}

// 读做种列表时到了时限（同步超时、退出）：在两页之间、或等限流过去时停下，不接着读。
// 第一页很快回来，时限落在等待里。
func TestMTorrentDriver_GetSeedingSize_StopsWhenCanceled(t *testing.T) {
	for name, c := range map[string]struct{ firstPage, stopsAt string }{
		"between pages": {
			`{"code":"0","message":"SUCCESS","data":{"total":"2","totalPages":"2","data":[{"torrent":{"size":"1"}}]}}`,
			"seeding list page 2",
		},
		"waiting out the limit": {`{"code":"4","message":"請求過於頻繁"}`, "seeding list page 1"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			d := mteamRawDriver(t, func(string, map[string]any) (int, string) {
				calls.Add(1)
				return http.StatusOK, c.firstPage
			})
			mtorrentSeedingPageDelay = time.Hour
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			_, _, err := d.GetSeedingSize(ctx, "1001")
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Contains(t, err.Error(), c.stopsAt)
			assert.NotContains(t, err.Error(), "execute request", "停在等待里，不是请求被打断")
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}
