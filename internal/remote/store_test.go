package remote

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// 配对窗口：10 分钟内有效，只能用一次，输错 5 次作废；新开的窗口替换旧的
func TestPairingStateMachine(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	p := newPairings(clk.now)
	created := 0
	create := func(scopes []string) (Device, error) {
		created++
		return Device{ID: uint(created), Name: "手机", Scopes: scopes}, nil
	}
	assert.False(t, p.open())
	assert.Equal(t, PairingNone, p.status().State)
	_, _, err := p.redeem(make([]byte, KeyLen), create)
	assert.ErrorIs(t, err, ErrPairingClosed)

	secret, expires, err := p.start(ScopesRead)
	require.NoError(t, err)
	assert.Equal(t, clk.now().Add(PairingTTL), expires)
	assert.True(t, p.open())
	st := p.status()
	assert.Equal(t, PairingWaiting, st.State)
	assert.Equal(t, ScopesRead, st.Scopes)

	// 只能用一次：配对成功以后窗口关掉，同一个密钥再来是失效
	dev, closedNow, err := p.redeem(secret, create)
	require.NoError(t, err)
	assert.False(t, closedNow)
	assert.Equal(t, ScopesRead, dev.Scopes)
	assert.False(t, p.open())
	st = p.status()
	assert.Equal(t, PairingPaired, st.State)
	require.NotNil(t, st.Device)
	assert.Equal(t, dev.ID, st.Device.ID)
	_, _, err = p.redeem(secret, create)
	assert.ErrorIs(t, err, ErrPairingClosed)
	assert.Equal(t, 1, created)

	// 写设备表失败：窗口继续等
	secret, _, err = p.start(ScopesFull)
	require.NoError(t, err)
	_, _, err = p.redeem(secret, func([]string) (Device, error) { return Device{}, ErrInvalid })
	assert.ErrorIs(t, err, ErrInvalid)
	assert.True(t, p.open())

	// 输错 5 次作废：第 5 次报 closedNow，之后正确的密钥也没用了
	for i := 0; i < MaxPairingFailures; i++ {
		_, closed, rerr := p.redeem(bytes.Repeat([]byte{byte(i)}, KeyLen), create)
		assert.ErrorIs(t, rerr, ErrPairingSecret)
		assert.Equal(t, i == MaxPairingFailures-1, closed, "第 %d 次", i+1)
	}
	assert.False(t, p.open())
	_, _, err = p.redeem(secret, create)
	assert.ErrorIs(t, err, ErrPairingClosed)
	assert.Equal(t, PairingClosed, p.status().State)
	assert.Equal(t, MaxPairingFailures, p.status().Failures)

	// 格式不对的密钥也算输错
	secret, _, err = p.start(ScopesFull)
	require.NoError(t, err)
	_, _, err = p.redeem(nil, create)
	assert.ErrorIs(t, err, ErrPairingSecret)
	assert.Equal(t, 1, p.status().Failures)

	// 过期
	clk.add(PairingTTL)
	assert.False(t, p.open())
	_, _, err = p.redeem(secret, create)
	assert.ErrorIs(t, err, ErrPairingClosed)
	assert.Equal(t, PairingExpired, p.status().State)

	// 新开的窗口替换旧的：旧的密钥不能用了
	old, _, err := p.start(ScopesFull)
	require.NoError(t, err)
	fresh, _, err := p.start(ScopesFull)
	require.NoError(t, err)
	_, _, err = p.redeem(old, create)
	assert.ErrorIs(t, err, ErrPairingSecret)

	// 取消以后正确的密钥也失效，设备表不写
	before := created
	p.cancel()
	assert.False(t, p.open())
	_, _, err = p.redeem(fresh, create)
	assert.ErrorIs(t, err, ErrPairingClosed)
	assert.Equal(t, before, created)
}

// 写设备表的时候（拿着窗口的锁）握手照样能问窗口开没开，不被拖住
func TestPairingOpenDoesNotWaitForRedeem(t *testing.T) {
	p := newPairings(time.Now)
	secret, _, err := p.start(ScopesFull)
	require.NoError(t, err)
	inCreate := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = p.redeem(secret, func(scopes []string) (Device, error) {
			close(inCreate)
			<-release
			return Device{ID: 1, Scopes: scopes}, nil
		})
	}()
	<-inCreate
	got := make(chan bool, 1)
	go func() { got <- p.open() }()
	select {
	case open := <-got:
		assert.True(t, open)
	case <-time.After(time.Second):
		t.Fatal("open 被写设备表拖住了")
	}
	close(release)
	<-done
	assert.False(t, p.open(), "配对成功以后窗口关了")
}

// 核对与写设备表在同一把锁里：同一个密钥并发提交，只有一次写设备表；写的时候取消要等它写完，之后窗口是已配对
func TestPairingRedeemLinearized(t *testing.T) {
	p := newPairings(time.Now)
	secret, _, err := p.start(ScopesFull)
	require.NoError(t, err)
	var mu sync.Mutex
	calls := 0
	inCreate := make(chan struct{})
	releaseCreate := make(chan struct{})
	create := func(scopes []string) (Device, error) {
		mu.Lock()
		calls++
		first := calls == 1
		mu.Unlock()
		if first {
			close(inCreate)
			<-releaseCreate
		}
		return Device{ID: 1, Scopes: scopes}, nil
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, _, rerr := p.redeem(secret, create)
			results <- rerr
		}()
	}
	<-inCreate
	cancelled := make(chan struct{})
	go func() {
		p.cancel()
		close(cancelled)
	}()
	select {
	case <-cancelled:
		t.Fatal("取消插到了核对与写库之间")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCreate)
	<-cancelled
	var okN, closedN int
	for i := 0; i < 2; i++ {
		switch rerr := <-results; {
		case rerr == nil:
			okN++
		case errors.Is(rerr, ErrPairingClosed):
			closedN++
		default:
			t.Fatalf("意外的错误: %v", rerr)
		}
	}
	assert.Equal(t, 1, okN)
	assert.Equal(t, 1, closedN)
	assert.Equal(t, 1, calls)
	assert.Equal(t, PairingPaired, p.status().State, "已经配对成功的窗口取消不了")
}

func TestStoreSettings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	got, err := s.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, Settings{Relays: []string{}}, got)

	got, err = s.SaveSettings(ctx, Settings{Enabled: true, Relays: []string{"wss://R.example.com/", "", "wss://r.example.com"}, DirectURL: "http://10.0.0.2:8080/"})
	require.NoError(t, err)
	assert.Equal(t, Settings{Enabled: true, Relays: []string{"wss://r.example.com"}, DirectURL: "http://10.0.0.2:8080"}, got)

	for _, bad := range []Settings{
		{Relays: []string{"https://r"}},
		{Relays: []string{"wss://a1", "wss://a2", "wss://a3", "wss://a4", "wss://a5"}},
		{DirectURL: "ftp://x"},
	} {
		_, serr := s.SaveSettings(ctx, bad)
		assert.ErrorIs(t, serr, ErrInvalid)
	}
	again, err := s.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, got, again)
}

// 主机密钥：第一次要的时候生成，落库的是密文；轮换时换一套并撤销所有设备；没有加密密钥时生成不了
func TestStoreHostKeys(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	k, err := s.HostKeys(ctx, false)
	require.NoError(t, err)
	assert.Nil(t, k)
	k, err = s.HostKeys(ctx, true)
	require.NoError(t, err)
	require.NotNil(t, k)
	var row models.RemoteSetting
	require.NoError(t, s.db.First(&row, 1).Error)
	assert.True(t, strings.HasPrefix(row.HostKeysEncrypted, "enc:"))
	again, err := s.HostKeys(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, k.HostID(), again.HostID())

	d, _ := addStoreDevice(t, s, "手机", ScopesFull)
	rotated, err := s.RotateHostKeys(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, k.HostID(), rotated.HostID())
	back, err := s.HostKeys(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, rotated.HostID(), back.HostID())
	dev, err := s.Device(ctx, d.ID)
	require.NoError(t, err)
	assert.NotNil(t, dev.RevokedAt)

	noCipher := NewStore(newTestDB(t), nil)
	_, err = noCipher.HostKeys(ctx, true)
	assert.ErrorIs(t, err, ErrNoCipher)
}

// 两个调用同时生成主机密钥时用同一套（不互相覆盖）
func TestStoreHostKeysConcurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	ids := make([]string, 8)
	for i := range ids {
		wg.Go(func() {
			k, err := s.HostKeys(ctx, true)
			if assert.NoError(t, err) {
				ids[i] = k.HostID()
			}
		})
	}
	wg.Wait()
	final, err := s.HostKeys(ctx, false)
	require.NoError(t, err)
	for _, id := range ids {
		assert.Equal(t, final.HostID(), id)
	}
}

func addStoreDevice(t *testing.T, s *Store, name string, scopes []string) (Device, []byte) {
	t.Helper()
	k, err := GenerateKeypair(nil)
	require.NoError(t, err)
	d, err := s.CreateDevice(context.Background(), name, k.Public, scopes)
	require.NoError(t, err)
	return d, k.Public
}

func TestStoreDevices(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	d, pub := addStoreDevice(t, s, "  我的手机 ", []string{"app:write", "app:read"})
	assert.Equal(t, "我的手机", d.Name)
	assert.Equal(t, ScopesFull, d.Scopes)
	found, err := s.ActiveDeviceByKey(ctx, pub)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, d.ID, found.ID)

	// 同一把公钥不能有两台没撤销的设备
	_, err = s.CreateDevice(ctx, "又一台", pub, ScopesRead)
	assert.ErrorIs(t, err, ErrInvalid)

	unnamed, _ := addStoreDevice(t, s, "", ScopesRead)
	assert.Equal(t, defaultDeviceName, unnamed.Name)
	assert.Equal(t, ScopesRead, unnamed.Scopes)

	for _, bad := range [][]string{{}, {"app:write"}, {"mcp:read"}, {"app:read", "qbit:compat"}} {
		k, _ := GenerateKeypair(nil)
		_, cerr := s.CreateDevice(ctx, "x", k.Public, bad)
		assert.ErrorIs(t, cerr, ErrInvalid, "%v", bad)
	}
	_, err = s.CreateDevice(ctx, strings.Repeat("长", 65), bytes.Repeat([]byte{1}, KeyLen), ScopesRead)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = s.CreateDevice(ctx, "a\nb", bytes.Repeat([]byte{1}, KeyLen), ScopesRead)
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = s.CreateDevice(ctx, "x", []byte{1, 2}, ScopesRead)
	assert.ErrorIs(t, err, ErrInvalid)

	// 改名不算改权限；降成只读算
	name := "平板"
	got, changed, err := s.UpdateDevice(ctx, d.ID, &name, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, "平板", got.Name)
	got, changed, err = s.UpdateDevice(ctx, d.ID, nil, ScopesFull)
	require.NoError(t, err)
	assert.False(t, changed)
	got, changed, err = s.UpdateDevice(ctx, d.ID, nil, ScopesRead)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, ScopesRead, got.Scopes)
	_, _, err = s.UpdateDevice(ctx, 999, &name, nil)
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	// 没撤销的不能删；撤销以后公钥找不到这台设备，也不能再改，可以删记录；同一把公钥可以重新配对成新设备
	assert.ErrorIs(t, s.DeleteDevice(ctx, d.ID), ErrDeviceActive)
	rev, err := s.RevokeDevice(ctx, d.ID)
	require.NoError(t, err)
	assert.NotNil(t, rev.RevokedAt)
	found, err = s.ActiveDeviceByKey(ctx, pub)
	require.NoError(t, err)
	assert.Nil(t, found)
	_, _, err = s.UpdateDevice(ctx, d.ID, &name, nil)
	assert.ErrorIs(t, err, ErrDeviceRevoked)
	again, err := s.RevokeDevice(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, rev.RevokedAt.Unix(), again.RevokedAt.Unix())
	repaired, err := s.CreateDevice(ctx, "重新配对", pub, ScopesRead)
	require.NoError(t, err)
	assert.NotEqual(t, d.ID, repaired.ID)

	list, err := s.Devices(ctx)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Nil(t, list[0].RevokedAt)
	assert.Nil(t, list[1].RevokedAt)
	assert.NotNil(t, list[2].RevokedAt, "撤销了的排在后面")
	require.NoError(t, s.DeleteDevice(ctx, d.ID))
	_, err = s.Device(ctx, d.ID)
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	at := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	require.NoError(t, s.TouchDevice(ctx, repaired.ID, ViaRelay, at))
	got, err = s.Device(ctx, repaired.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastSeenAt)
	assert.True(t, got.LastSeenAt.Equal(at))
	assert.Equal(t, ViaRelay, got.LastSeenVia)
}

func TestStoreDeviceLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < maxDevices; i++ {
		addStoreDevice(t, s, "d", ScopesRead)
	}
	k, _ := GenerateKeypair(nil)
	_, err := s.CreateDevice(context.Background(), "多的", k.Public, ScopesRead)
	assert.ErrorIs(t, err, ErrInvalid)
}
