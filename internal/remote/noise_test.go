package remote

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/flynn/noise"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ikVector struct {
	ProtocolName     string `json:"protocol_name"`
	InitPrologue     string `json:"init_prologue"`
	InitStatic       string `json:"init_static"`
	InitEphemeral    string `json:"init_ephemeral"`
	InitRemoteStatic string `json:"init_remote_static"`
	RespPrologue     string `json:"resp_prologue"`
	RespStatic       string `json:"resp_static"`
	RespEphemeral    string `json:"resp_ephemeral"`
	HandshakeHash    string `json:"handshake_hash"`
	Messages         []struct {
		Payload    string `json:"payload"`
		Ciphertext string `json:"ciphertext"`
	} `json:"messages"`
	Source string `json:"source"`
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// v1 用的密码套件对得上公开的测试向量（cacophony 与 snow 各一组 Noise_IK_25519_ChaChaPoly_BLAKE2s）：
// 握手两条消息、之后的传输消息逐字节一致，握手散列一致
func TestNoiseIKOfficialVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/noise_ik_vectors.json")
	require.NoError(t, err)
	var file struct {
		Vectors []ikVector `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &file))
	require.Len(t, file.Vectors, 2)
	for _, v := range file.Vectors {
		t.Run(v.Source, func(t *testing.T) {
			require.Equal(t, ProtocolName, v.ProtocolName)
			is, err := KeypairFromPrivate(unhex(t, v.InitStatic))
			require.NoError(t, err)
			rs, err := KeypairFromPrivate(unhex(t, v.RespStatic))
			require.NoError(t, err)
			require.Equal(t, rs.Public, unhex(t, v.InitRemoteStatic))
			init, err := noise.NewHandshakeState(noise.Config{
				CipherSuite: cipherSuite, Pattern: noise.HandshakeIK, Initiator: true, Prologue: unhex(t, v.InitPrologue),
				StaticKeypair: is, PeerStatic: rs.Public, Random: bytes.NewReader(unhex(t, v.InitEphemeral)),
			})
			require.NoError(t, err)
			resp, err := noise.NewHandshakeState(noise.Config{
				CipherSuite: cipherSuite, Pattern: noise.HandshakeIK, Prologue: unhex(t, v.RespPrologue),
				StaticKeypair: rs, Random: bytes.NewReader(unhex(t, v.RespEphemeral)),
			})
			require.NoError(t, err)

			m0 := v.Messages[0]
			ct, _, _, err := init.WriteMessage(nil, unhex(t, m0.Payload))
			require.NoError(t, err)
			assert.Equal(t, m0.Ciphertext, hex.EncodeToString(ct))
			pt, _, _, err := resp.ReadMessage(nil, ct)
			require.NoError(t, err)
			assert.Equal(t, m0.Payload, hex.EncodeToString(pt))
			assert.Equal(t, is.Public, resp.PeerStatic())

			m1 := v.Messages[1]
			ct, respRecv, respSend, err := resp.WriteMessage(nil, unhex(t, m1.Payload))
			require.NoError(t, err)
			assert.Equal(t, m1.Ciphertext, hex.EncodeToString(ct))
			pt, initSend, initRecv, err := init.ReadMessage(nil, ct)
			require.NoError(t, err)
			assert.Equal(t, m1.Payload, hex.EncodeToString(pt))
			if v.HandshakeHash != "" {
				assert.Equal(t, v.HandshakeHash, hex.EncodeToString(init.ChannelBinding()))
				assert.Equal(t, v.HandshakeHash, hex.EncodeToString(resp.ChannelBinding()))
			}

			// 之后的传输消息：发起方与响应方轮流发
			for i, m := range v.Messages[2:] {
				send, recv := initSend, respRecv
				if i%2 == 1 {
					send, recv = respSend, initRecv
				}
				ct, err := send.Encrypt(nil, nil, unhex(t, m.Payload))
				require.NoError(t, err)
				assert.Equal(t, m.Ciphertext, hex.EncodeToString(ct), "第 %d 条传输消息", i)
				pt, err := recv.Decrypt(nil, nil, ct)
				require.NoError(t, err)
				assert.Equal(t, m.Payload, hex.EncodeToString(pt))
			}
		})
	}
}

// handshakeResult 是一次握手两端的结果。
type handshakeResult struct {
	dev     *secureConn
	hello   HostHello
	devErr  error
	host    *secureConn
	hostErr error
}

// handshakePair 在内存连接上跑一次握手。
func handshakePair(t *testing.T, keys *HostKeys, hostIDHost, hostIDDevice string, hostKey []byte, decide func([]byte, ClientHello) HostHello) handshakeResult {
	t.Helper()
	dev, err := GenerateKeypair(nil)
	require.NoError(t, err)
	a, b := memPipe()
	type hostResult struct {
		conn *secureConn
		err  error
	}
	ch := make(chan hostResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		c, _, _, err := acceptHandshake(ctx, b, keys, hostIDHost, nil, decide)
		if err != nil {
			// 和 serveConn 一样：握手失败就关掉连接，设备那边马上知道
			b.Close(1000, "")
		}
		ch <- hostResult{c, err}
	}()
	var r handshakeResult
	r.dev, r.hello, r.devErr = dialHandshake(ctx, a, hostIDDevice, hostKey, dev, nil, ClientHello{Client: "t"})
	if r.devErr != nil {
		a.Close(1000, "")
	}
	hr := <-ch
	r.host, r.hostErr = hr.conn, hr.err
	return r
}

func acceptAll(_ []byte, _ ClientHello) HostHello { return HostHello{Mode: ModeDevice, Host: "v"} }

// 握手：公钥对、hostId 对时两边都能收发；设备记的主机公钥不对、hostId（prologue）不对时主机解不开第一条消息
func TestHandshake(t *testing.T) {
	keys := seedKeys(t, 7)
	id := keys.HostID()
	r := handshakePair(t, keys, id, id, keys.Noise.Public, acceptAll)
	require.NoError(t, r.devErr)
	require.NoError(t, r.hostErr)
	assert.Equal(t, ModeDevice, r.hello.Mode)
	dc, hc := r.dev, r.host
	ctx := context.Background()
	require.NoError(t, dc.WriteFrame(ctx, Frame{Type: FramePing, Payload: []byte("hi")}))
	f, err := hc.ReadFrame(ctx)
	require.NoError(t, err)
	assert.Equal(t, FramePing, f.Type)
	assert.Equal(t, "hi", string(f.Payload))
	require.NoError(t, hc.WriteFrame(ctx, Frame{Type: FramePong, Payload: []byte("hi")}))
	f, err = dc.ReadFrame(ctx)
	require.NoError(t, err)
	assert.Equal(t, FramePong, f.Type)

	other := seedKeys(t, 8)
	r = handshakePair(t, keys, id, id, other.Noise.Public, acceptAll)
	assert.Error(t, r.devErr)
	assert.ErrorIs(t, r.hostErr, ErrHandshake)

	r = handshakePair(t, keys, id, other.HostID(), keys.Noise.Public, acceptAll)
	assert.Error(t, r.devErr)
	assert.ErrorIs(t, r.hostErr, ErrHandshake)
}

// 主机不认识设备时照样回第二条消息，设备拿到明确的 ErrNotPaired（App 据此提示重新配对）
func TestHandshakeRejected(t *testing.T) {
	keys := seedKeys(t, 7)
	id := keys.HostID()
	r := handshakePair(t, keys, id, id, keys.Noise.Public, func([]byte, ClientHello) HostHello {
		return HostHello{Error: HelloNotPaired}
	})
	assert.ErrorIs(t, r.devErr, ErrNotPaired)
	assert.Equal(t, HelloNotPaired, r.hello.Error)
	var rej handshakeRejected
	assert.ErrorAs(t, r.hostErr, &rej)

	r = handshakePair(t, keys, id, id, keys.Noise.Public, func([]byte, ClientHello) HostHello {
		return HostHello{Error: HelloBusy}
	})
	assert.ErrorIs(t, r.devErr, ErrHandshake)
	assert.NotErrorIs(t, r.devErr, ErrNotPaired)
}

// 加密信道：明文 65519 字节（帧头加 65514 字节的 payload）正好放得下；改过的密文、太短太长的消息都拒绝
func TestSecureConnSizes(t *testing.T) {
	keys := seedKeys(t, 7)
	id := keys.HostID()
	r := handshakePair(t, keys, id, id, keys.Noise.Public, acceptAll)
	require.NoError(t, r.devErr)
	require.NoError(t, r.hostErr)
	dc, hc := r.dev, r.host
	ctx := context.Background()
	big := bytes.Repeat([]byte{'x'}, MaxFramePayload)
	require.NoError(t, dc.WriteFrame(ctx, Frame{Type: FrameReqBody, ID: 1, Payload: big}))
	f, err := hc.ReadFrame(ctx)
	require.NoError(t, err)
	assert.Equal(t, big, f.Payload)
	assert.ErrorIs(t, dc.WriteFrame(ctx, Frame{Type: FrameReqBody, ID: 1, Payload: append(big, 'y')}), ErrFrame)

	// 直接往连接里塞不合规的消息
	peer := dc.conn.(*memConn)
	require.NoError(t, peer.WriteMsg(ctx, make([]byte, MaxNoiseMessage+1)))
	_, err = hc.ReadFrame(ctx)
	assert.ErrorIs(t, err, ErrFrame)
	require.NoError(t, peer.WriteMsg(ctx, make([]byte, 10)))
	_, err = hc.ReadFrame(ctx)
	assert.ErrorIs(t, err, ErrFrame)
}

func TestSecureConnTampered(t *testing.T) {
	keys := seedKeys(t, 7)
	id := keys.HostID()
	r := handshakePair(t, keys, id, id, keys.Noise.Public, acceptAll)
	require.NoError(t, r.devErr)
	require.NoError(t, r.hostErr)
	dc, hc := r.dev, r.host
	ctx := context.Background()
	// 截下设备发的一条消息，改一个字节再交给主机
	tap, b := memPipe()
	dc.conn = tap
	go func() {
		msg, err := b.ReadMsg(ctx)
		if err != nil {
			return
		}
		msg[len(msg)-1] ^= 1
		_ = hc.conn.(*memConn).peerWrite(ctx, msg)
	}()
	require.NoError(t, dc.WriteFrame(ctx, Frame{Type: FramePing}))
	_, err := hc.ReadFrame(ctx)
	assert.ErrorContains(t, err, "解密失败")
}

// peerWrite 以对端的身份往这条连接里写一条消息（测试改包用）。
func (c *memConn) peerWrite(ctx context.Context, b []byte) error {
	select {
	case c.in <- b:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
