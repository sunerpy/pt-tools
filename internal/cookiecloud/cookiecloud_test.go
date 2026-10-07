package cookiecloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试向量用 CookieCloud 浏览器扩展所用的 crypto-js 4.2.0 生成：legacy 是 CryptoJS.AES.encrypt(明文, 口令)，
// aes-128-cbc-fixed 是 key = 口令的 UTF-8 字节、iv = 16 个 0 字节的 AES-CBC + PKCS7。口令 = md5(uuid + "-" + password) 的前 16 个字符。
const (
	vecUUID     = "qa-uuid-7f3e"
	vecPassword = "qa-pass-9b21"
	vecKey      = "00179b811246b8bd"
	vecLegacy   = "U2FsdGVkX199+lVxZNI1bgee1bYxF5NnsclXVwvknxBVBTcc8gK0lF6X/yBENwqN70TurnrUSn1Gew+RvIi0fSdfdLoTSeOb0X/9WIUzsbHa3aWv7jislV5iDIWIpbSlVZJ5PL8i0y6ZZE/8CiSNIjmgP/OrF2qJVrezl/0q9WK8KXjYspHIbOASRWjvRzLuEhtQ0M4Kq6DwDNxcWjPV05Aiut3rbRnoRSTMGCzxEG2/t45pSKD4fPBpJgIhcAlgVuFYq5npUWOxhxUDUiNSevs6LaMj8K+SAP6qP5LKAdBHMXIJTO+bmyfdBQSf2oHIsmDX7/PTZxJCLUq0xhWdoz2t8XqNrTURooYw4qA2WCedEjZNxPXWa5cnalrs/tiWliEuk2fhCr7gDSJXlkKUL5lZb0mhK+GxJT+rkVxV3KvUKyt0V8wDX54qnrES1wFIxT8QYPzaJtOCdjKsX9t88TQuE+NsyGzgoCGKku023UxGNDmTvY21LOzW2vKuMR1EhsX09fhrb/7PXaGzDwmbjc6MxyvSzuGPiWKSkClU5rk="
	vecFixed    = "9D05vyIUUqlI4l4WE26RPYZjhxRq2YiSLTHHivcQueIkL5jFP0Lb4ZFg1s+DxXvFzIzTM6ZgLJ/SE8RptVhCC37Vkt5fRnMGhYD9Wp89zGupz4heGpAC/64yKzVaggKDv1ObgdvMvpDLsoK81XhqReDzV/iDUGNFXeIJZvDeqXf+lSm6ruQeg3nnCngWj03Znric4oLK1R6eQW5t+DgwxdriTwwSelMcuaD9k3GIpXZ79rXcVuPpIeflvMFrlPgcWeeecaF2DFBIQV9jEAnj1JRR3qcO0c7HjZY5Kwcth5psTxRtihfwpdJB5B7fpSa4C2qnG+X0NAADCB8Roq8zKps3qp2tyb1jMBIPw5LtZGOv44NzV1mcIqvNtuUOPIuRZc7jK2Q/j2EBzT/l2o7A9DZUN0KsrlRPkE7IlG3OMDKZuTE6oS3nUXZFc/TsFxlkD2xZQrmqUgta7u1ihaglBXhVekt1YxcnKIvcD7jFXPIyi1n/gLcbvkDIDyJdmLJewB6M6OqXCjceNC/Zdy4agQ=="
)

func TestPassphrase(t *testing.T) {
	assert.Equal(t, vecKey, passphrase(vecUUID, vecPassword))
}

func TestDecryptVectors(t *testing.T) {
	for _, p := range []Payload{
		{Encrypted: vecLegacy},
		{Encrypted: vecLegacy, CryptoType: CryptoLegacy},
		{Encrypted: vecFixed, CryptoType: CryptoFixed},
		{Encrypted: vecFixed}, // 老服务端不带 crypto_type、密文却不是 OpenSSL 格式：按 fixed 再试
	} {
		d, err := Decrypt(p, vecUUID, vecPassword)
		require.NoError(t, err, p.CryptoType)
		require.Len(t, d.CookieData["hdsky.me"], 2)
		assert.Equal(t, "c_secure_uid", d.CookieData["hdsky.me"][0].Name)
		assert.Equal(t, "42", d.CookieData["hdsky.me"][0].Value)
	}
	_, err := Decrypt(Payload{Encrypted: vecLegacy}, vecUUID, "wrong")
	assert.ErrorIs(t, err, ErrDecrypt)
	_, err = Decrypt(Payload{Encrypted: vecFixed, CryptoType: CryptoFixed}, vecUUID, "wrong")
	assert.ErrorIs(t, err, ErrDecrypt)
	_, err = Decrypt(Payload{Encrypted: "not base64!", CryptoType: CryptoFixed}, vecUUID, vecPassword)
	assert.ErrorIs(t, err, ErrDecrypt)
	_, err = Decrypt(Payload{Encrypted: vecFixed, CryptoType: "rot13"}, vecUUID, vecPassword)
	assert.ErrorContains(t, err, "rot13")
}

func TestFetch(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		switch r.URL.Path {
		case "/cc/get/" + vecUUID:
			_, _ = w.Write([]byte(`{"encrypted":"` + vecFixed + `","crypto_type":"aes-128-cbc-fixed"}`))
		case "/cc/get/missing":
			http.NotFound(w, r)
		case "/cc/get/empty":
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	p, err := Fetch(ctx, srv.Client(), srv.URL+"/cc/", " "+vecUUID+" ")
	require.NoError(t, err)
	assert.Equal(t, CryptoFixed, p.CryptoType)
	assert.Equal(t, "/cc/get/"+vecUUID, gotPath, "去掉末尾的 / 和 UUID 两边的空格")
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "missing")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "empty")
	assert.ErrorContains(t, err, "没有密文")
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "boom")
	assert.ErrorContains(t, err, "HTTP 502")
	_, err = Fetch(ctx, srv.Client(), srv.URL+"/cc", "a/b")
	require.Error(t, err)
	assert.Equal(t, "/cc/get/a%2Fb", gotPath, "UUID 按路径的一段转义")
	_, err = Fetch(ctx, nil, "ftp://example.org", "x")
	assert.ErrorContains(t, err, "http:// 或 https://")
	_, err = Fetch(ctx, nil, srv.URL, " ")
	assert.ErrorContains(t, err, "UUID")
}

func ptr[T any](v T) *T { return &v }

func TestMatchSites(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	future := float64(now.Add(time.Hour).Unix())
	past := float64(now.Add(-time.Hour).Unix())
	d := Data{CookieData: map[string][]Cookie{
		"hdsky.me": {
			{Name: "c_secure_uid", Value: "42", Domain: ".hdsky.me", Path: "/"},
			{Name: "c_secure_pass", Value: "s3cret", Domain: ".hdsky.me", Path: "/", ExpirationDate: &future},
			{Name: "old", Value: "x", Domain: ".hdsky.me", Path: "/", ExpirationDate: &past},
			{Name: "forum", Value: "x", Domain: ".hdsky.me", Path: "/forum"},
		},
		"www.hdsky.me": {
			{Name: "c_secure_uid", Value: "host-only", Domain: "www.hdsky.me", Path: "/"},
		},
		"tracker.hdsky.me": {
			{Name: "t", Value: "1", Domain: "tracker.hdsky.me", Path: "/"},
		},
		"ourbits.club": {
			{Name: "o", Value: "1", Domain: "ourbits.club", Path: "/", HostOnly: ptr(false)},
			{Name: "h", Value: "2", Domain: ".ourbits.club", Path: "/", HostOnly: ptr(true)},
		},
	}}
	got := MatchSites(d, []Site{
		{Name: "hdsky", BaseURL: "https://hdsky.me/"},
		{Name: "hdsky-www", BaseURL: "https://WWW.hdsky.me:443/"},
		{Name: "ourbits", BaseURL: "https://www.ourbits.club"},
		{Name: "nothing", BaseURL: "https://nothing.example/"},
		{Name: "bad", BaseURL: "::"},
	}, now)
	require.Len(t, got, 3)
	assert.Equal(t, Match{Site: "hdsky", Host: "hdsky.me", Header: "c_secure_pass=s3cret; c_secure_uid=42", Names: []string{"c_secure_pass", "c_secure_uid"}}, got[0],
		"子域名的、过期的、路径不是 / 的都不要")
	assert.Equal(t, "c_secure_pass=s3cret; c_secure_uid=host-only", got[1].Header, "同名取域名最具体的")
	assert.Equal(t, "www.hdsky.me", got[1].Host)
	assert.Equal(t, "o=1", got[2].Header, "hostOnly 字段优先于前导点")
}
