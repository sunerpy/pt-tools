package v2

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 下面的 API 返回、网页都照 2026-11-02 站点（www.uhddolby.com）的结构写，数值与 downhash 是示例。

func TestNewHDDolbyDriver_Defaults(t *testing.T) {
	// API 和网页在同一个主机上（www.uhddolby.com/api/v1/…）
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com/"})
	assert.Equal(t, "https://www.uhddolby.com", d.BaseURL)
	assert.Equal(t, "https://www.uhddolby.com", d.APIURL)
	assert.Equal(t, "pt-tools/1.0", d.userAgent)
	assert.NotNil(t, d.httpClient)

	// explicit APIURL + userAgent
	d2 := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com", APIURL: "https://api.x.com/", UserAgent: "custom"})
	assert.Equal(t, "https://api.x.com", d2.APIURL)
	assert.Equal(t, "custom", d2.userAgent)
}

func TestHDDolbyDriver_PrepareSearch(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com"})
	req, err := d.PrepareSearch(SearchQuery{Keyword: "matrix"})
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/torrent/search", req.Endpoint)
	assert.Equal(t, http.MethodPost, req.Method)
	body := req.Body.(HDDolbySearchRequest)
	assert.Equal(t, "matrix", body.Keyword)
	assert.Equal(t, 0, body.PageNumber, "page_number 是跳过前面多少条：给 1 会少了第一条")
	assert.Equal(t, 100, body.PageSize)
	assert.Equal(t, 1, body.Visible)
}

// hddolbySearchJSON 是 torrent/search 的一份返回：data 直接是种子列表，status 0 是成功。
const hddolbySearchJSON = `{"status":0,"data":[
	{"id":900001,"sp_state":2,"promotion_time_type":2,"promotion_until":"2026-11-04 14:36:32","category":402,
	 "name":"Show.S01E01.2026.2160p.WEB-DL.H265-DBTV","small_descr":"示例剧集 第1集","size":9734472868,
	 "seeders":12,"leechers":3,"times_completed":40,"added":"2026-11-02 14:36:32","hr":0,
	 "tags":"gf;gy;zz;xz;lz;hq;yq","info_hash":"ABCDEF0123456789ABCDEF0123456789ABCDEF01","downhash":"dh-900001"},
	{"id":900002,"sp_state":2,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":401,
	 "name":"Movie.2026.1080p.BluRay.x264","small_descr":"","size":27609205714,"seeders":5,"leechers":9,
	 "times_completed":2,"added":"2026-11-02 13:29:13","hr":1,"tags":"gy;zz;wj","downhash":"dh-900002"},
	{"id":900003,"sp_state":4,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":401,
	 "name":"Movie.Two.2026.2160p.UHD","size":47652518027,"added":"2026-11-02 12:27:28","tags":"zz","downhash":"dh-900003"},
	{"id":900004,"sp_state":5,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":404,
	 "name":"Doc.2026.1080p","size":64496595623,"added":"2026-11-02 14:04:52","tags":"zz;diy;db;hdr10","downhash":"dh-900004"},
	{"id":900005,"sp_state":2,"promotion_time_type":1,"promotion_until":"0000-00-00 00:00:00","category":402,
	 "name":"Old.Pack.2020","size":1514839481793,"added":"2020-01-15 15:46:42","tags":"","downhash":"dh-900005"},
	{"id":900006,"sp_state":1,"promotion_time_type":0,"promotion_until":"0000-00-00 00:00:00","category":409,
	 "name":"Official.Normal.2026","size":1024,"added":"2026-11-01 08:00:00","tags":"gf;sf;hfr;gz","downhash":"dh-900006"}
]}`

func TestHDDolbyDriver_ParseSearch(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com"})
	var res HDDolbyResponse
	require.NoError(t, json.Unmarshal([]byte(hddolbySearchJSON), &res))
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 6)
	now := time.Date(2026, 11, 2, 15, 0, 0, 0, CSTLocation)

	// 到 promotion_until 为止的免费
	a := items[0]
	assert.Equal(t, "900001", a.ID)
	assert.Equal(t, "Show.S01E01.2026.2160p.WEB-DL.H265-DBTV", a.Title)
	assert.Equal(t, "示例剧集 第1集", a.Subtitle)
	assert.Equal(t, int64(9734472868), a.SizeBytes)
	assert.Equal(t, 12, a.Seeders)
	assert.Equal(t, 3, a.Leechers)
	assert.Equal(t, 40, a.Snatched)
	assert.Equal(t, "电视剧", a.Category)
	assert.Equal(t, "https://www.uhddolby.com/details.php?id=900001", a.URL)
	assert.Equal(t, "/api/site/hddolby/torrent/900001/download?downhash=dh-900001", a.DownloadURL)
	assert.Equal(t, "abcdef0123456789abcdef0123456789abcdef01", a.InfoHash)
	assert.Equal(t, []string{"官方", "国语", "中字", "限转", "连载", "高码率", "应求"}, a.Tags)
	assert.Equal(t, DiscountFree, a.DiscountLevel)
	assert.True(t, a.DiscountEndTime.Equal(time.Date(2026, 11, 4, 14, 36, 32, 0, CSTLocation)), a.DiscountEndTime)
	assert.Equal(t, FreeUntil, a.Promotion(now).Validity)
	assert.Equal(t, time.Date(2026, 11, 2, 14, 36, 32, 0, CSTLocation).Unix(), a.UploadedAt)
	assert.False(t, a.HasHR)

	// 跟站点默认的免费：发布以后 24 小时结束
	b := items[1]
	assert.Equal(t, DiscountFree, b.DiscountLevel)
	assert.True(t, b.DiscountEndTime.Equal(time.Date(2026, 11, 3, 13, 29, 13, 0, CSTLocation)), b.DiscountEndTime)
	assert.Equal(t, FreeUntil, b.Promotion(now).Validity)
	assert.True(t, b.HasHR)
	assert.Equal(t, "电影", b.Category)

	// 跟站点默认的 2X 免费：也是 24 小时
	c := items[2]
	assert.Equal(t, Discount2xFree, c.DiscountLevel)
	assert.True(t, c.DiscountEndTime.Equal(time.Date(2026, 11, 3, 12, 27, 28, 0, CSTLocation)), c.DiscountEndTime)

	// 跟站点默认的 50%：没有结束时间
	e := items[3]
	assert.Equal(t, DiscountPercent50, e.DiscountLevel)
	assert.True(t, e.DiscountEndTime.IsZero())
	assert.Equal(t, FreeNotFree, e.Promotion(now).Validity)
	assert.Equal(t, "纪录片", e.Category)

	// 永久免费
	f := items[4]
	assert.Equal(t, DiscountFree, f.DiscountLevel)
	assert.True(t, f.DiscountEndTime.IsZero())
	assert.Equal(t, FreePermanent, f.Promotion(now).Validity)
	assert.Nil(t, f.Tags)

	// 标签不是优惠：官方（gf）、首发（sf）、高帧率（hfr）的普通种子还是没有优惠
	g := items[5]
	assert.Equal(t, DiscountNone, g.DiscountLevel)
	assert.Equal(t, FreeNotFree, g.Promotion(now).Validity)
	assert.Equal(t, []string{"官方", "首发", "高帧率", "官字"}, g.Tags)
	assert.Equal(t, "其他", g.Category)
}

func TestHDDolbyDiscount_SpState(t *testing.T) {
	for sp, want := range map[int]DiscountLevel{
		0: DiscountNone, 1: DiscountNone, 2: DiscountFree, 3: Discount2xUp, 4: Discount2xFree,
		5: DiscountPercent50, 6: Discount2x50, 7: DiscountPercent30, 8: DiscountNone,
	} {
		assert.Equal(t, want, hddolbyDiscount(sp), "sp_state %d", sp)
	}
}

// 结束时间解析不了、时限认不出：免费是 unknown 并记下原因；没有结束时间不等于长期免费
func TestHDDolbyDriver_ParseSearch_DiscountEndValidity(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com"})
	searchJSON := `[
		{"id":1,"name":"A","sp_state":2,"promotion_time_type":2,"promotion_until":"2099-10-20 12:00:00"},
		{"id":2,"name":"B","sp_state":2,"promotion_time_type":2,"promotion_until":"2099/10/20"},
		{"id":3,"name":"C","sp_state":2,"promotion_time_type":2,"promotion_until":"0000-00-00 00:00:00"},
		{"id":4,"name":"D","sp_state":5,"promotion_time_type":2,"promotion_until":"bad"},
		{"id":5,"name":"E","sp_state":1,"promotion_time_type":2,"promotion_until":"bad"},
		{"id":6,"name":"F","sp_state":2,"promotion_time_type":9},
		{"id":7,"name":"G","sp_state":2,"promotion_time_type":0,"added":"bad"}
	]`
	items, err := d.ParseSearch(HDDolbyResponse{Data: json.RawMessage(searchJSON)})
	require.NoError(t, err)
	require.Len(t, items, 7)
	now := time.Now()

	assert.Empty(t, items[0].DiscountErr)
	assert.Equal(t, FreeUntil, items[0].Promotion(now).Validity)

	assert.Equal(t, DiscountFree, items[1].DiscountLevel)
	assert.Equal(t, FreeUnknown, items[1].DiscountValidity)
	assert.Equal(t, "promotion_until: 时间格式无法解析", items[1].DiscountErr)
	assert.Equal(t, FreeUnknown, items[1].Promotion(now).Validity)

	assert.Empty(t, items[2].DiscountValidity)
	assert.Empty(t, items[2].DiscountErr)
	assert.Equal(t, FreeUnknown, items[2].Promotion(now).Validity, "没有结束时间不等于长期免费")

	assert.Equal(t, DiscountPercent50, items[3].DiscountLevel)
	assert.Equal(t, "promotion_until: 时间格式无法解析", items[3].DiscountErr)
	assert.Equal(t, FreeNotFree, items[3].Promotion(now).Validity)

	assert.Equal(t, DiscountNone, items[4].DiscountLevel)
	assert.Empty(t, items[4].DiscountErr, "没有优惠时结束时间无关")

	assert.Equal(t, "promotion_time_type: 格式无法解析", items[5].DiscountErr)
	assert.Equal(t, FreeUnknown, items[5].Promotion(now).Validity)

	assert.Equal(t, "added: 时间格式无法解析", items[6].DiscountErr)
	assert.Equal(t, FreeUnknown, items[6].Promotion(now).Validity)
}

func TestHDDolbyDriver_ParseSearch_Invalid(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com"})
	_, err := d.ParseSearch(HDDolbyResponse{Data: json.RawMessage(`[{"id":1,"downhash":"dh-secret"},`)})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "dh-secret", "错误里不写返回的内容：里面有 downhash")
}

func TestHDDolbyDriver_ParseDiscountEndTime(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	end, err := d.parseDiscountEndTime("")
	require.NoError(t, err)
	assert.True(t, end.IsZero())
	end, err = d.parseDiscountEndTime("0000-00-00 00:00:00")
	require.NoError(t, err, "零值日期表示没有结束时间，不是解析失败")
	assert.True(t, end.IsZero())
	end, err = d.parseDiscountEndTime("invalid")
	require.Error(t, err)
	assert.True(t, end.IsZero())
	end, err = d.parseDiscountEndTime("2024-02-15 10:30:00")
	require.NoError(t, err)
	assert.True(t, end.Equal(time.Date(2024, 2, 15, 10, 30, 0, 0, CSTLocation)))
}

func TestHDDolbyDriver_GetCategoryName(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.Equal(t, "电影", d.getCategoryName(401))
	assert.Equal(t, "音乐", d.getCategoryName(408))
	assert.Equal(t, "学习", d.getCategoryName(411))
	assert.Equal(t, "12345", d.getCategoryName(12345))
}

func TestHDDolbyTags(t *testing.T) {
	assert.Equal(t, []string{"官方", "Dolby Vision", "HDR10+"}, hddolbyTags("gf; db ;HDRM;unknown;"))
	assert.Nil(t, hddolbyTags(""))
}

func TestHDDolbyDriver_PrepareUserInfo(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareUserInfo()
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/user/data", req.Endpoint)
	assert.Equal(t, http.MethodGet, req.Method)
}

func TestHDDolbyDriver_ParseUserInfo(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com"})
	userJSON := `[{
		"id":"10001","username":"tester","added":"2020-01-01 00:00:00",
		"last_access":"2024-06-01 12:00:00","class":"5",
		"uploaded":"10737418240","downloaded":"1073741824",
		"seedbonus":"50000.0","sebonus":"1234.5","unread_messages":"3"
	}]`
	info, err := d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(userJSON)})
	require.NoError(t, err)
	assert.Equal(t, "hddolby", info.Site)
	assert.Equal(t, "10001", info.UserID)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(10737418240), info.Uploaded)
	assert.Equal(t, int64(1073741824), info.Downloaded)
	assert.InDelta(t, 10.0, info.Ratio, 0.001)
	assert.InDelta(t, 50000, info.Bonus, 0.001)
	assert.InDelta(t, 1234.5, info.SeedingBonus, 0.001)
	assert.Equal(t, 3, info.UnreadMessageCount)
	assert.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, CSTLocation).Unix(), info.JoinDate)
	assert.Equal(t, time.Date(2024, 6, 1, 12, 0, 0, 0, CSTLocation).Unix(), info.LastAccess)
	assert.Equal(t, "5", info.LevelName) // no siteDef -> classID unchanged
}

func TestHDDolbyDriver_ParseUserInfo_Errors(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	_, err := d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(`bad`)})
	assert.Error(t, err)
	_, err = d.ParseUserInfo(HDDolbyResponse{Data: json.RawMessage(`[]`)})
	assert.Error(t, err)
}

func TestHDDolbyDriver_GetLevelName(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	// no siteDef
	assert.Equal(t, "3", d.getLevelName("3"))

	d.SetSiteDefinition(&SiteDefinition{
		ID: "hddolby",
		LevelRequirements: []SiteLevelRequirement{
			{ID: 5, Name: "Elite User"},
		},
	})
	assert.Equal(t, "Elite User", d.getLevelName("5"))
	assert.Equal(t, "99", d.getLevelName("99"))   // not found
	assert.Equal(t, "abc", d.getLevelName("abc")) // non-numeric
}

func TestHDDolbyDriver_GetSiteID(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	assert.Equal(t, "hddolby", d.getSiteID())
	d.SetSiteDefinition(&SiteDefinition{ID: "custom"})
	assert.Equal(t, "custom", d.getSiteID())
}

func TestHDDolbyDriver_ParseDownload(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	data, err := d.ParseDownload(HDDolbyResponse{RawBody: []byte("torrentdata")})
	require.NoError(t, err)
	assert.Equal(t, []byte("torrentdata"), data)

	_, err = d.ParseDownload(HDDolbyResponse{})
	assert.Error(t, err)
}

func TestHDDolbyDriver_PrepareDownload(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	req, err := d.PrepareDownload("42")
	require.NoError(t, err)
	assert.Contains(t, req.Endpoint, "id=42")
}

func TestHDDolbyDriver_Execute_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		// API 不认浏览器的 User-Agent（「Browser access is blocked!」）
		assert.Equal(t, "pt-tools/1.0", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":[{"id":1,"name":"X"}]}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "test-key"})
	res, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/api/v1/torrent/search", Method: http.MethodPost, Body: map[string]any{"keyword": ""}})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Status)
	assert.JSONEq(t, `[{"id":1,"name":"X"}]`, string(res.Data))
}

// 出错时 HTTP 照样是 200：key 不对（包括过期）是 30001，没带 key 是 10405，都算凭据无效；超了每小时次数算限流；浏览器访问被拦是别的错误
func TestHDDolbyDriver_Execute_APIErrorStatus(t *testing.T) {
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})

	for _, tc := range []struct {
		body    string
		invalid bool
		limited bool
		text    string
	}{
		{`{"status":30001,"error":{"message":"Illegal api key","details":""}}`, true, false, "Illegal api key"},
		{`{"status":10405,"error":{"message":"Missing x-api-key","details":""}}`, true, false, "Missing x-api-key"},
		{`{"status":10405,"error":{"message":"Browser access is blocked!","details":""}}`, false, false, "Browser access is blocked!"},
		// wiki「API & 自动化」页的失败示例
		{`{"status":10405,"error":{"message":"Limit of api call per hour reached! please wait for four hour!","details":""}}`, false, true, "4 小时"},
		{`{"status":12345,"data":[]}`, false, false, "status 12345"},
	} {
		body = tc.body
		_, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/api/v1/user/data", Method: http.MethodGet})
		require.Error(t, err, tc.body)
		assert.Equal(t, tc.invalid, errors.Is(err, ErrInvalidCredentials), "%s: %v", tc.body, err)
		assert.Equal(t, tc.limited, errors.Is(err, ErrRateLimited), "%s: %v", tc.body, err)
		assert.Contains(t, err.Error(), tc.text)
		if tc.invalid {
			assert.Contains(t, err.Error(), "RSS key", "写明去哪里换 key")
		}
	}
}

func TestHDDolbyDriver_Execute_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"status":403,"error":{"code":"FORBIDDEN","message":"denied"}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/x", Method: http.MethodGet})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.Contains(t, err.Error(), "denied")
}

func TestHDDolbyDriver_Execute_Cloudflare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<html>Just a moment...</html>`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.Execute(context.Background(), HDDolbyRequest{Endpoint: "/x", Method: http.MethodGet})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cloudflare")
}

func TestHDDolbyDriver_Search_HTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/torrent/search", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		assert.Equal(t, "movie", body["keyword"])
		assert.EqualValues(t, 0, body["page_number"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(hddolbySearchJSON))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	items, err := d.Search(context.Background(), SearchQuery{Keyword: "movie"})
	require.NoError(t, err)
	require.Len(t, items, 6)
	assert.Equal(t, "900001", items[0].ID)
	assert.Equal(t, server.URL+"/details.php?id=900001", items[0].URL)
}

func TestHDDolbyDriver_Search_Error(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "http://127.0.0.1:1", APIKey: "k"})
	_, err := d.Search(context.Background(), SearchQuery{Keyword: "x"})
	require.Error(t, err)
}

// hddolbyBonusPage 是时魔页「您目前每小时可以获取到的鲸币/做种积分如下」那张表的样子（数值是示例）
const hddolbyBonusPage = `<html><head><title>UHDDolby :: tester的鲸币 - Powered by NexusPHP</title></head><body>
<table width="800"><tr align="Center"><td colspan="4"><b>您目前每小时可以获取到的鲸币/做种积分如下</b></td></tr>
<tr align="center"><td>官种</td><td colspan="5">-</td><td>3</td><td>1.5 / 1.5</td></tr>
<tr align="center"><td>合计</td><td colspan="5">-</td><td>460</td><td>25.98 / 25.98</td></tr></table></body></html>`

// hddolbyTwoFactorPage 是 Cookie 要先过两步验证时站点回的页面：脚本跳到 take2fa.php
const hddolbyTwoFactorPage = `<html><head><title>UHDDolby :: 两步验证 - Powered by NexusPHP</title></head><body>
<script type="text/javascript">window.location.href = "take2fa.php?returnto=%2Fmybonus.php";</script></body></html>`

const hddolbyLoginPage = `<html><head><title>UHDDolby :: 登录 - Powered by NexusPHP</title></head><body>
<form method="post" action="takelogin.php"><input type="text" name="username"><input type="password" name="password">
<input type="text" name="2fa"></form></body></html>`

func TestHDDolbyDriver_GetBonusPerHour(t *testing.T) {
	// 没配 Cookie：0，不是错误（API 不给时魔）
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	bph, err := d.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.Equal(t, float64(0), bph)

	page := hddolbyBonusPage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/mybonus.php", r.URL.Path)
		assert.Equal(t, "c_secure_uid=1", r.Header.Get("Cookie"))
		assert.Contains(t, r.Header.Get("User-Agent"), "Mozilla", "网页用浏览器的 User-Agent")
		_, _ = w.Write([]byte(page))
	}))
	defer server.Close()

	d2 := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, Cookie: "c_secure_uid=1"})
	bph, err = d2.GetBonusPerHour(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 25.98, bph, 0.001)

	page = hddolbyTwoFactorPage
	_, err = d2.GetBonusPerHour(context.Background())
	require.ErrorIs(t, err, Err2FARequired)
	assert.Contains(t, err.Error(), "两步验证")

	page = hddolbyLoginPage
	_, err = d2.GetBonusPerHour(context.Background())
	require.ErrorIs(t, err, ErrSessionExpired)

	page = `<html><body><table><tr><td>别的</td></tr></table></body></html>`
	_, err = d2.GetBonusPerHour(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "合计")
}

// hddolbyUserServer 模拟用户数据要用的几个接口与时魔页；peersFail 时做种列表回 500，bonusPage 是时魔页
func hddolbyUserServer(t *testing.T, peersFail *atomic.Bool, bonusPage *atomic.Value) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user/data":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":0,"data":[{"id":"10001","username":"u","added":"2020-01-01 00:00:00",` +
				`"last_access":"2026-11-02 10:00:00","class":"1","uploaded":"1000","downloaded":"500","seedbonus":"10.0","sebonus":"2.0","unread_messages":"0"}]}`))
		case "/api/v1/user/peers":
			if peersFail.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":0,"data":[{"id":1,"size":2048,"seeders":1,"leechers":0},{"id":2,"size":1024,"seeders":3,"leechers":1}]}`))
		case "/mybonus.php":
			_, _ = w.Write([]byte(bonusPage.Load().(string)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestHDDolbyDriver_GetUserInfo_HTTP(t *testing.T) {
	var peersFail atomic.Bool
	var bonus atomic.Value
	bonus.Store(hddolbyBonusPage)
	server := hddolbyUserServer(t, &peersFail, &bonus)

	// 只有 API Key：没有时魔，不算出错
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "u", info.Username)
	assert.Equal(t, int64(1000), info.Uploaded)
	assert.Equal(t, 2, info.SeederCount)
	assert.Equal(t, int64(3072), info.SeederSize)
	assert.Zero(t, info.BonusPerHour)

	// 配了 Cookie：时魔从时魔页读
	d = NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k", Cookie: "c_secure_uid=1"})
	info, err = d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.InDelta(t, 25.98, info.BonusPerHour, 0.001)
}

// 做种列表、时魔没取到：交回取到的部分，*PartialUserInfoError 写明缺哪些字段（保存时沿用上一次的）
func TestHDDolbyDriver_GetUserInfo_Partial(t *testing.T) {
	var peersFail atomic.Bool
	var bonus atomic.Value
	bonus.Store(hddolbyTwoFactorPage)
	server := hddolbyUserServer(t, &peersFail, &bonus)
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k", Cookie: "c_secure_uid=1"})

	info, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.Equal(t, []string{"bonusPerHour"}, partial.Missing)
	require.ErrorIs(t, err, Err2FARequired)
	assert.Equal(t, "u", info.Username)
	assert.Equal(t, 2, info.SeederCount)

	peersFail.Store(true)
	bonus.Store(hddolbyBonusPage)
	info, err = d.GetUserInfo(context.Background())
	require.ErrorAs(t, err, &partial)
	assert.Equal(t, []string{"seeding", "seederSize", "leeching", "leecherSize"}, partial.Missing)
	assert.Contains(t, err.Error(), "做种列表")
	assert.Equal(t, "u", info.Username)
	assert.Zero(t, info.SeederCount)
	assert.InDelta(t, 25.98, info.BonusPerHour, 0.001)
}

const (
	hddolbyAttendSignedPage = `<html><head><title>UHDDolby :: 签到 - Powered by NexusPHP</title></head><body><table><tr><td>
<h2>签到成功</h2><p>这是您的第 <b>137</b> 次签到，已连续签到 <b>1</b> 天，本次签到获得 <b>292</b> 个鲸币。</p></td></tr></table></body></html>`
	hddolbyAttendAlreadyPage = `<html><head><title>UHDDolby :: 签到 - Powered by NexusPHP</title></head><body><table><tr><td>
<h2>抱歉</h2><p>您今天已经签到过了，请勿重复刷新。</p></td></tr></table></body></html>`
)

func hddolbyAttendDriver(baseURL, cookie string) *HDDolbyDriver {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: baseURL, APIKey: "k", Cookie: cookie})
	d.SetSiteDefinition(&SiteDefinition{ID: "hddolby", Schema: SchemaHDDolby, Attendance: &AttendanceConfig{}})
	return d
}

func TestHDDolbyDriver_Attend(t *testing.T) {
	var page atomic.Value
	page.Store(hddolbyAttendSignedPage)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/attendance.php", r.URL.Path)
		assert.Equal(t, "c_secure_uid=1", r.Header.Get("Cookie"))
		_, _ = w.Write([]byte(page.Load().(string)))
	}))
	defer server.Close()
	d := hddolbyAttendDriver(server.URL, "c_secure_uid=1")
	assert.Empty(t, d.AttendanceUnsupportedReason())

	res, err := d.Attend(context.Background())
	require.NoError(t, err)
	assert.Equal(t, AttendSigned, res.Status)
	assert.Contains(t, res.Message, "第 137 次签到")

	page.Store(hddolbyAttendAlreadyPage)
	res, err = d.Attend(context.Background())
	require.NoError(t, err)
	assert.Equal(t, AttendAlready, res.Status)

	page.Store(hddolbyTwoFactorPage)
	_, err = d.Attend(context.Background())
	require.ErrorIs(t, err, Err2FARequired)

	page.Store(`<html><body>别的页面</body></html>`)
	_, err = d.Attend(context.Background())
	require.ErrorIs(t, err, ErrAttendanceUnrecognized)
}

// 没配 Cookie：不能签到，原因写明要 Cookie；站点定义没写签到时照旧不支持
func TestHDDolbyDriver_AttendWithoutCookie(t *testing.T) {
	d := hddolbyAttendDriver("https://www.uhddolby.com", "")
	assert.Contains(t, d.AttendanceUnsupportedReason(), "Cookie")
	_, err := d.Attend(context.Background())
	require.ErrorIs(t, err, ErrAttendanceUnsupported)

	site := NewBaseSite[HDDolbyRequest, HDDolbyResponse](d, BaseSiteConfig{ID: "hddolby", Name: "UHDDolby", Kind: SiteHDDolby, Logger: zap.NewNop()})
	assert.False(t, site.SupportsAttendance())
	assert.Contains(t, site.AttendanceUnsupportedReason(), "Cookie", "签到监控把这句原因记进签到记录")
	_, err = site.Attend(context.Background())
	require.ErrorIs(t, err, ErrAttendanceUnsupported)
	assert.Contains(t, err.Error(), "Cookie")
	withCookie := hddolbyAttendDriver("https://www.uhddolby.com", "c_secure_uid=1")
	site = NewBaseSite[HDDolbyRequest, HDDolbyResponse](withCookie, BaseSiteConfig{ID: "hddolby", Name: "UHDDolby", Kind: SiteHDDolby, Logger: zap.NewNop()})
	assert.True(t, site.SupportsAttendance())
	assert.Empty(t, site.AttendanceUnsupportedReason())

	noDef := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://www.uhddolby.com", Cookie: "c_secure_uid=1"})
	assert.NotEmpty(t, noDef.AttendanceUnsupportedReason())
	_, err = noDef.Attend(context.Background())
	require.ErrorIs(t, err, ErrAttendanceUnsupported)
}

func TestHDDolbyDriver_GetTorrentDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":[{"id":42,"name":"Found","sp_state":2,"promotion_time_type":1,"size":500}]}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})

	item, err := d.GetTorrentDetail(context.Background(), "42", "", "Found")
	require.NoError(t, err)
	assert.Equal(t, "42", item.ID)
	assert.Equal(t, "Found", item.Title)
	assert.Equal(t, DiscountFree, item.DiscountLevel)
	assert.Equal(t, FreePermanent, item.DiscountValidity)

	// 缓存里没有：读不到详情就是取详情失败，不能返回一个「没有优惠」的占位结果（那等于声称不是免费）
	item2, err := d.GetTorrentDetail(context.Background(), "999", "", "")
	require.ErrorIs(t, err, ErrTorrentDetailUnavailable)
	assert.Nil(t, item2)
}

func TestHDDolbyDriver_GetTorrentDetail_InvalidID(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	_, err := d.GetTorrentDetail(context.Background(), "", "", "")
	assert.Error(t, err)
	_, err = d.GetTorrentDetail(context.Background(), "notanumber", "", "")
	assert.Error(t, err)
}

func TestHDDolbyDriver_InvalidateDetailCache(t *testing.T) {
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: "https://x.com"})
	d.detailCache = []HDDolbyTorrent{{ID: 1}}
	d.invalidateDetailCache()
	assert.Nil(t, d.detailCache)
}

func TestHDDolbyDriver_GetTorrentDetail_CacheMissRefresh(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":[{"id":1,"name":"A"}]}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	// Requesting a missing ID repeatedly should trigger cache invalidation + refresh at miss>=3.
	for range 3 {
		item, err := d.GetTorrentDetail(context.Background(), "999", "", "")
		require.ErrorIs(t, err, ErrTorrentDetailUnavailable)
		assert.Nil(t, item)
	}
	assert.GreaterOrEqual(t, calls, 2)
}

// 详情来自最近一次列表的缓存：FetchedAt 是缓存取得的时间；要求 fresh-after 时缓存更早就先刷新，刷新失败返回错误
func TestHDDolbyDriver_GetTorrentDetail_FreshAfter(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":[` +
			`{"id":42,"name":"Found","sp_state":2,"promotion_time_type":2,"promotion_until":"2099-10-20 12:00:00"},` +
			`{"id":43,"name":"BadEnd","sp_state":2,"promotion_time_type":2,"promotion_until":"20991020"}]}`))
	}))
	defer server.Close()
	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})

	before := time.Now()
	item, err := d.GetTorrentDetail(context.Background(), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.False(t, item.FetchedAt.Before(before), "FetchedAt 是缓存取得的时间：%s", item.FetchedAt)
	assert.False(t, item.FetchedAt.After(time.Now()))
	assert.Equal(t, FreeUntil, item.Promotion(time.Now()).Validity)
	first := item.FetchedAt

	// 详情走同一套优惠解析：结束时间解析不了是 unknown
	item, err = d.GetTorrentDetail(context.Background(), "43", "", "")
	require.NoError(t, err)
	assert.Equal(t, FreeUnknown, item.DiscountValidity)
	assert.Equal(t, "promotion_until: 时间格式无法解析", item.DiscountErr)

	// 5 分钟内、没要求新鲜：直接用缓存，FetchedAt 仍是缓存的时间
	item, err = d.GetTorrentDetail(context.Background(), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.True(t, first.Equal(item.FetchedAt))

	// 缓存早于 fresh-after：先刷新再回答
	d.detailCacheMu.Lock()
	d.detailCacheTime = d.detailCacheTime.Add(-time.Minute)
	d.detailCacheMu.Unlock()
	freshAfter := time.Now()
	item, err = d.GetTorrentDetail(WithFreshAfter(context.Background(), freshAfter), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load(), "缓存早于 fresh-after 时必须刷新")
	assert.False(t, item.FetchedAt.Before(freshAfter), "FetchedAt %s 早于 fresh-after %s", item.FetchedAt, freshAfter)

	// 缓存已经不早于 fresh-after：不重复请求站点
	_, err = d.GetTorrentDetail(WithFreshAfter(context.Background(), freshAfter), "42", "", "")
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())

	// 刷新失败：返回错误，不拿旧缓存充数
	fail.Store(true)
	item, err = d.GetTorrentDetail(WithFreshAfter(context.Background(), time.Now().Add(time.Second)), "42", "", "")
	require.Error(t, err)
	assert.Nil(t, item)
	assert.Equal(t, int32(3), calls.Load())
}

func TestHDDolbyDriver_GetTorrentDetail_ArrayCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"data":{"data":[{"id":7,"name":"ObjForm"}],"total":1}}`))
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	item, err := d.GetTorrentDetail(context.Background(), "7", "", "")
	require.NoError(t, err)
	assert.Equal(t, "7", item.ID)
	assert.Equal(t, "ObjForm", item.Title)
}

// 下载要带 downhash：调用方给了就用；没给就到详情缓存（最近 100 条搜索结果）里找；回的不是种子文件时报错
func TestHDDolbyDriver_DownloadWithHash(t *testing.T) {
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/torrent/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":0,"data":[{"id":5,"name":"Five","downhash":"dh-5"},{"id":6,"name":"Six","downhash":""}]}`))
		case "/download.php":
			downloads.Add(1)
			switch r.URL.Query().Get("downhash") {
			case "dh-5", "hh":
				_, _ = w.Write([]byte("d8:announce"))
			default:
				_, _ = w.Write([]byte("error B!"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	data, err := d.DownloadWithHash(context.Background(), "5", "hh")
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)

	data, err = d.Download(context.Background(), "5")
	require.NoError(t, err)
	assert.Equal(t, []byte("d8:announce"), data)

	_, err = d.Download(context.Background(), "6")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "没有这个种子")
	assert.Contains(t, err.Error(), "Cookie", "告诉用户填上 Cookie 就能按编号下载")
	_, err = d.Download(context.Background(), "99")
	require.Error(t, err)

	_, err = d.DownloadWithHash(context.Background(), "5", "wrong")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不是种子文件")
	assert.Equal(t, int32(3), downloads.Load(), "缓存里没有 downhash 时不去下载")
}

func TestHDDolbyDriver_DownloadWithHash_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: server.URL, APIKey: "k"})
	_, err := d.DownloadWithHash(context.Background(), "5", "hh")
	require.Error(t, err)
}

func TestCreateHDDolbySite(t *testing.T) {
	opts := HDDolbyOptions{APIKey: "rsskey", Cookie: "c=1"}
	optsBytes, _ := json.Marshal(opts)
	site, err := createHDDolbySite(SiteConfig{
		ID:      "hddolby",
		Name:    "UHDDolby",
		BaseURL: "https://www.uhddolby.com",
		Options: optsBytes,
	}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
	assert.Equal(t, "hddolby", site.ID())

	// 只有 API Key 也能建：Cookie 是可选的
	optsBytes, _ = json.Marshal(HDDolbyOptions{APIKey: "rsskey"})
	site, err = createHDDolbySite(SiteConfig{ID: "hddolby", Name: "UHDDolby", BaseURL: "https://www.uhddolby.com", Options: optsBytes}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
}

func TestCreateHDDolbySite_MissingCreds(t *testing.T) {
	_, err := createHDDolbySite(SiteConfig{ID: "hddolby", Name: "UHDDolby", BaseURL: "https://x.com"}, zap.NewNop())
	require.Error(t, err)

	optsBytes, _ := json.Marshal(HDDolbyOptions{Cookie: "c=1"})
	_, err = createHDDolbySite(SiteConfig{ID: "hddolby", Name: "UHDDolby", BaseURL: "https://x.com", Options: optsBytes}, zap.NewNop())
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "API Key"), err.Error())
}

func TestCreateHDDolbySite_BadOptions(t *testing.T) {
	_, err := createHDDolbySite(SiteConfig{ID: "hddolby", Options: []byte("notjson")}, zap.NewNop())
	require.Error(t, err)
}

// 只知道种子编号时（辅种、转种、批量下载）：配了 Cookie 就像在网页上点「下载」一样用 Cookie 下 download.php?id=，不要 downhash，
// 也不为找 downhash 去调 API（每小时只有 50 次）。2026-11-02 生产辅种的 4 个 UHDDolby 种子以前拿到的都是登录页。
func TestHDDolbyDriver_DownloadByIDWithCookie(t *testing.T) {
	torrent := []byte("d8:announce4:infod4:name1:aee")
	var searches, letdowns atomic.Int32
	var gotCookie, gotUA atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/api/v1/torrent/search":
			searches.Add(1)
			_, _ = w.Write([]byte(`{"status":0,"data":[]}`))
		case "/download.php":
			assert.Empty(t, q.Get("downhash"), "Cookie 下载不带 downhash")
			gotCookie.Store(r.Header.Get("Cookie"))
			gotUA.Store(r.Header.Get("User-Agent"))
			switch {
			case q.Get("letdown") == "1":
				letdowns.Add(1)
				_, _ = w.Write(torrent)
			case q.Get("id") == "8":
				http.Redirect(w, r, "/downloadnotice.php?torrentid=8&type=firsttime", http.StatusFound)
			case q.Get("id") == "9":
				_, _ = w.Write([]byte(hddolbyLoginPage))
			case q.Get("id") == "10":
				_, _ = w.Write([]byte(hddolbyTwoFactorPage))
			default:
				_, _ = w.Write(torrent)
			}
		case "/downloadnotice.php":
			_, _ = w.Write([]byte(noticePage(q.Get("torrentid"), q.Get("type"))))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	optsBytes, _ := json.Marshal(HDDolbyOptions{APIKey: "rsskey", Cookie: "c_secure_uid=1"})
	site, err := createHDDolbySite(SiteConfig{ID: "hddolby", Name: "UHDDolby", BaseURL: server.URL, Options: optsBytes, RateLimit: 100, RateBurst: 100}, zap.NewNop())
	require.NoError(t, err)
	ctx := context.Background()

	// BaseSite.Download（辅种走这里）交给驱动按编号下载
	data, err := site.Download(ctx, "7")
	require.NoError(t, err)
	assert.Equal(t, torrent, data)
	assert.Equal(t, "c_secure_uid=1", gotCookie.Load())
	assert.Contains(t, gotUA.Load(), "Mozilla", "网页用浏览器的 User-Agent")

	// 账号开着「第一次下载提示」：带上 letdown=1 再下
	data, err = site.Download(ctx, "8")
	require.NoError(t, err)
	assert.Equal(t, torrent, data)
	assert.Equal(t, int32(1), letdowns.Load())

	// Cookie 失效、要两步验证：报清楚，不把网页当成种子
	_, err = site.Download(ctx, "9")
	require.ErrorIs(t, err, ErrSessionExpired)
	assert.ErrorIs(t, err, ErrNotTorrentFile)
	_, err = site.Download(ctx, "10")
	require.ErrorIs(t, err, Err2FARequired)
	assert.Contains(t, err.Error(), "两步验证")

	// 调用方没给 downhash 的 DownloadWithHash 也一样
	hd, ok := site.(HashDownloader)
	require.True(t, ok)
	data, err = hd.DownloadWithHash(ctx, "7", "")
	require.NoError(t, err)
	assert.Equal(t, torrent, data)
	assert.Zero(t, searches.Load(), "有 Cookie 时不为找 downhash 调 API")
}
