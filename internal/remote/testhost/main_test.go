package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/remote"
)

// 测试主机起来以后写地址文件；控制接口能开配对窗口；Go 的设备端经它的 relay 配对、调 /meta；ctx 结束时退出并删掉地址文件
func TestRun(t *testing.T) {
	info := filepath.Join(t.TempDir(), "testhost.json")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, info) }()
	require.Eventually(t, func() bool { _, err := os.Stat(info); return err == nil }, 30*time.Second, 50*time.Millisecond)
	var addr struct{ Direct, Relay, Control string }
	b, err := os.ReadFile(info)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &addr))

	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
	post := func(path string, out any) int {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, addr.Control+path, nil)
		resp, perr := client.Do(req)
		require.NoError(t, perr)
		defer resp.Body.Close()
		if out != nil {
			_ = json.NewDecoder(resp.Body).Decode(out)
		}
		return resp.StatusCode
	}
	var ticket remote.PairingTicket
	require.Equal(t, http.StatusOK, post("/pair?scopes=read", &ticket))
	link, err := remote.ParseLink(ticket.Link)
	require.NoError(t, err)
	assert.Equal(t, []string{addr.Relay}, link.Relays)
	assert.Equal(t, addr.Direct, link.Direct)

	dev, err := remote.GenerateKeypair(nil)
	require.NoError(t, err)
	cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
	defer ccancel()
	connect := func() (*remote.Client, error) {
		raw, derr := remote.DialRelay(cctx, link.Relays[0], link.HostID, client)
		if derr != nil {
			return nil, derr
		}
		return remote.Connect(cctx, raw, remote.ClientConfig{HostID: link.HostID, HostKey: link.HostKey, Device: dev, Client: "testhost-test"})
	}
	pc, err := connect()
	require.NoError(t, err)
	d, err := pc.Pair(cctx, link.Secret, "Go 测试")
	require.NoError(t, err)
	assert.Equal(t, remote.ScopesRead, d.Scopes)
	<-pc.Done()

	c, err := connect()
	require.NoError(t, err)
	req, _ := http.NewRequestWithContext(cctx, http.MethodGet, "/api/app/v1/meta", nil)
	resp, err := c.RoundTrip(req)
	require.NoError(t, err)
	var meta struct {
		Principal struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		} `json:"principal"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&meta))
	resp.Body.Close()
	assert.Equal(t, "Go 测试", meta.Principal.Name)
	assert.Equal(t, remote.ScopesRead, meta.Principal.Scopes)

	// 改权限、撤销：会话收到 GOAWAY
	id := "1"
	assert.Equal(t, http.StatusOK, post("/devices/"+id+"/scopes?scopes=full", nil))
	<-c.Done()
	assert.Equal(t, remote.GoAwayScopeChanged, c.GoAway())
	assert.Equal(t, http.StatusOK, post("/devices/"+id+"/revoke", nil))
	assert.Equal(t, http.StatusBadRequest, post("/devices/x/revoke", nil))
	_, err = connect()
	assert.ErrorIs(t, err, remote.ErrNotPaired)

	cancel()
	select {
	case rerr := <-done:
		require.NoError(t, rerr)
	case <-time.After(15 * time.Second):
		t.Fatal("ctx 结束以后测试主机没有退出")
	}
	_, err = os.Stat(info)
	assert.True(t, os.IsNotExist(err), "退出时删掉地址文件")
}
