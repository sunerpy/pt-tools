package nas

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPending(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	p := &Pending{Now: func() time.Time { return now }}
	key := PendingKey{Kind: "synology", URL: "https://nas:5001", Username: "admin", ID: 0}
	extra := map[string]string{"did": "device-token"}
	token, err := p.Put(key, extra)
	require.NoError(t, err)
	assert.Len(t, token, 43, "32 字节的 base64url")
	extra["did"] = "改了"

	// 绑定不符
	for _, other := range []PendingKey{
		{Kind: "fnos", URL: key.URL, Username: key.Username},
		{Kind: key.Kind, URL: "https://other:5001", Username: key.Username},
		{Kind: key.Kind, URL: key.URL, Username: "root"},
		{Kind: key.Kind, URL: key.URL, Username: key.Username, ID: 3},
	} {
		_, _, err = p.Claim(token, other)
		assert.ErrorIs(t, err, ErrPendingInvalid, "%+v", other)
	}
	_, _, err = p.Claim("nope", key)
	assert.ErrorIs(t, err, ErrPendingInvalid)

	got, done, err := p.Claim(token, key)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"did": "device-token"}, got, "存的是拷贝")
	_, _, err = p.Claim(token, key)
	assert.ErrorIs(t, err, ErrPendingInvalid, "正被一次保存用着")
	done(false)
	done(true) // 只认第一次
	_, done, err = p.Claim(token, key)
	require.NoError(t, err, "保存失败时放回去，还能再用")
	done(true)
	_, _, err = p.Claim(token, key)
	assert.ErrorIs(t, err, ErrPendingInvalid, "保存提交以后删掉，只能用一次")
	assert.Equal(t, 0, p.Len())
}

func TestPending_Expiry(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	p := &Pending{Now: func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}}
	key := PendingKey{Kind: "fnos", URL: "https://fn:5667", Username: "u", ID: 7}
	old, err := p.Put(key, nil)
	require.NoError(t, err)
	mu.Lock()
	now = now.Add(PendingTTL)
	mu.Unlock()
	_, _, err = p.Claim(old, key)
	assert.ErrorIs(t, err, ErrPendingInvalid, "10 分钟过期")

	fresh, err := p.Put(key, map[string]string{"token": "t"})
	require.NoError(t, err)
	assert.Equal(t, 1, p.Len(), "放新的时清掉过期的")
	_, err = p.Put(PendingKey{Kind: "fnos", URL: "https://fn:5667", Username: "u", ID: 8}, nil)
	require.NoError(t, err)
	p.DropID(7)
	_, _, err = p.Claim(fresh, key)
	assert.ErrorIs(t, err, ErrPendingInvalid, "删掉来源时一起丢掉")
	assert.Equal(t, 1, p.Len())

	assert.False(t, (&Pending{}).now().IsZero())
}
