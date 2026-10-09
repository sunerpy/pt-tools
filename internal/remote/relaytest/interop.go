package relaytest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/remote"
	"github.com/sunerpy/pt-tools/models"
)

// plainCipher 是互通测试用的主机密钥加解密（加个前缀）。
type plainCipher struct{}

func (plainCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (plainCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("不是密文")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

// echoDispatcher 回调用的路径与设备名，代替 App API。
type echoDispatcher struct{}

func (echoDispatcher) ServeRemote(w http.ResponseWriter, r *http.Request, p remote.Peer) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`","device":"`+p.Device.Name+`","via":"`+p.Via+`"}`)
}

// RunHostInterop 用真正的主机端（internal/remote 的 Host 与它的 relay 客户端）经 relayURL 这台 relay 走完一遍：
// 主机认证上线、扫码配对（配对会话）、设备会话里调用 App API、撤销以后会话收到 GOAWAY、之后连不上。
// NewHost 起一台真正的主机（internal/remote 的 Host，库在临时目录，App API 换成回显），打开远程访问并等它经 relayURL 上线。
func NewHost(t *testing.T, relayURL string) *remote.Host {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "interop.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.RemoteSetting{}, &models.RemoteDevice{}))
	h := remote.New(remote.Config{Store: remote.NewStore(db, plainCipher{}), Dispatcher: echoDispatcher{}, Version: "interop"})
	t.Cleanup(h.Close)
	ctx := context.Background()
	_, err = h.UpdateSettings(ctx, remote.Settings{Enabled: true, Relays: []string{relayURL}})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		ov, oerr := h.Overview(ctx)
		return oerr == nil && len(ov.RelayStatus) == 1 && ov.RelayStatus[0].State == remote.RelayOnline
	}, 20*time.Second, 50*time.Millisecond, "主机没有经 relay 上线")
	return h
}

func RunHostInterop(t *testing.T, relayURL string) {
	t.Helper()
	h := NewHost(t, relayURL)
	ctx := context.Background()

	ticket, err := h.StartPairing(ctx, remote.ScopesRead, "")
	require.NoError(t, err)
	link, err := remote.ParseLink(ticket.Link)
	require.NoError(t, err)
	require.Equal(t, []string{strings.TrimRight(relayURL, "/")}, link.Relays)

	dev, err := remote.GenerateKeypair(nil)
	require.NoError(t, err)
	connect := func() (*remote.Client, error) {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		raw, derr := remote.DialRelay(cctx, link.Relays[0], link.HostID, nil)
		if derr != nil {
			return nil, derr
		}
		return remote.Connect(cctx, raw, remote.ClientConfig{HostID: link.HostID, HostKey: link.HostKey, Device: dev, Client: "interop"})
	}

	pc, err := connect()
	require.NoError(t, err)
	require.Equal(t, remote.ModePairing, pc.Mode())
	paired, err := pc.Pair(ctx, link.Secret, "互通测试")
	require.NoError(t, err)
	select {
	case <-pc.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("配对以后会话没有关")
	}
	assert.Equal(t, remote.GoAwayPaired, pc.GoAway())

	c, err := connect()
	require.NoError(t, err)
	require.Equal(t, remote.ModeDevice, c.Mode())
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(rctx, http.MethodGet, "/api/app/v1/meta", nil)
	resp, err := c.RoundTrip(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"path":"/api/app/v1/meta","device":"互通测试","via":"relay"}`, string(body))

	_, err = h.RevokeDevice(ctx, paired.ID)
	require.NoError(t, err)
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("撤销以后会话没有关")
	}
	assert.Equal(t, remote.GoAwayRevoked, c.GoAway())
	_, err = connect()
	assert.ErrorIs(t, err, remote.ErrNotPaired)
}
