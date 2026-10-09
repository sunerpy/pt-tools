package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/flynn/noise"
)

// ProtocolName 是 v1 用的 Noise 协议：设备是发起方（扫码拿到了主机的 X25519 公钥），主机是响应方。
const ProtocolName = "Noise_IK_25519_ChaChaPoly_BLAKE2s"

const (
	prologuePrefix = "pt-tools-remote-v1"
	// HandshakeTimeout 是等每一条握手消息的上限
	HandshakeTimeout = 10 * time.Second
	// maxHelloLen 是握手消息里 JSON 的上限
	maxHelloLen = 1024
	// protocolVersion 是握手内容里的协议版本
	protocolVersion = 1
)

// 会话的种类（HostHello.Mode）。
const (
	// ModeDevice 是配对过的设备：可以调用 App API
	ModeDevice = "device"
	// ModePairing 是配对窗口里不认识的设备：只能 POST /remote/v1/pair
	ModePairing = "pairing"
)

// 主机拒绝握手时 HostHello.Error 的取值。
const (
	// HelloNotPaired 是主机不认识这台设备（没有配对或已经撤销），而且现在没有配对窗口
	HelloNotPaired = "not_paired"
	// HelloUnsupported 是设备用的协议版本主机不认识
	HelloUnsupported = "unsupported_version"
	// HelloBusy 是主机的会话太多了，稍后再连
	HelloBusy = "busy"
)

var (
	// ErrHandshake 是握手没有完成
	ErrHandshake = errors.New("握手失败")
	// ErrNotPaired 是主机不认识这台设备
	ErrNotPaired = errors.New("这台设备没有配对，或者已经被撤销")
)

var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2s)

// Prologue 是握手的 prologue："pt-tools-remote-v1" || hostId。两边不一致时握手失败。
func Prologue(hostID string) []byte { return append([]byte(prologuePrefix), hostID...) }

// ClientHello 是设备放在第一条握手消息里的内容（加密，只有主机读得到）。
type ClientHello struct {
	V int `json:"v"`
	// Client 是 App 的名字与版本，只用来记日志
	Client string `json:"client,omitempty"`
}

// HostHello 是主机放在第二条握手消息里的内容（加密，只有设备读得到）。
type HostHello struct {
	V int `json:"v"`
	// Mode 是会话的种类（ModeDevice 或 ModePairing）
	Mode string `json:"mode,omitempty"`
	// Host 是 pt-tools 的版本
	Host string `json:"host,omitempty"`
	// Error 不为空时主机随后关闭连接
	Error string `json:"error,omitempty"`
}

// handshakeRejected 是主机发出了带 Error 的第二条消息。
type handshakeRejected struct{ reason string }

func (e handshakeRejected) Error() string { return "握手被拒绝: " + e.reason }

// acceptHandshake 作为响应方完成握手：读第一条消息，按设备公钥由 decide 决定会话种类，回第二条消息。
// decide 返回的 Error 不为空时第二条消息照样发出（设备据此提示重新配对），然后返回 handshakeRejected。
// rng 是临时密钥的随机源，nil 时用 crypto/rand（测试向量用固定的）。
func acceptHandshake(ctx context.Context, conn MsgConn, keys *HostKeys, hostID string, rng io.Reader, decide func(peer []byte, hello ClientHello) HostHello) (*secureConn, []byte, HostHello, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite: cipherSuite, Pattern: noise.HandshakeIK, Prologue: Prologue(hostID), StaticKeypair: keys.Noise, Random: rng,
	})
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	rctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	msg, err := conn.ReadMsg(rctx)
	cancel()
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: 读第一条消息: %v", ErrHandshake, err)
	}
	if len(msg) > MaxNoiseMessage {
		return nil, nil, HostHello{}, fmt.Errorf("%w: 第一条消息太长", ErrHandshake)
	}
	// 解不开说明设备用的不是这台主机的公钥，或者 hostId 不对（prologue 不同）
	payload, _, _, err := hs.ReadMessage(nil, msg)
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: 第一条消息解不开: %v", ErrHandshake, err)
	}
	peer := append([]byte(nil), hs.PeerStatic()...)
	var hello ClientHello
	var reply HostHello
	if len(payload) > maxHelloLen || json.Unmarshal(payload, &hello) != nil || hello.V != protocolVersion {
		reply = HostHello{Error: HelloUnsupported}
	} else {
		reply = decide(peer, hello)
	}
	reply.V = protocolVersion
	out, err := json.Marshal(reply)
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	msg2, recv, send, err := hs.WriteMessage(nil, out)
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	wctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	err = conn.WriteMsg(wctx, msg2)
	cancel()
	if err != nil {
		return nil, nil, HostHello{}, fmt.Errorf("%w: 写第二条消息: %v", ErrHandshake, err)
	}
	if reply.Error != "" {
		return nil, peer, reply, handshakeRejected{reason: reply.Error}
	}
	// 响应方：Split 的第一个 CipherState 解设备发来的消息，第二个加密发给设备的消息
	return newSecureConn(conn, send, recv), peer, reply, nil
}

// dialHandshake 作为发起方完成握手（设备端）。主机回了 Error 时返回 ErrNotPaired 或 ErrHandshake。
func dialHandshake(ctx context.Context, conn MsgConn, hostID string, hostKey []byte, device noise.DHKey, rng io.Reader, hello ClientHello) (*secureConn, HostHello, error) {
	if len(hostKey) != KeyLen {
		return nil, HostHello{}, fmt.Errorf("%w: 主机公钥长度不对", ErrHandshake)
	}
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite: cipherSuite, Pattern: noise.HandshakeIK, Initiator: true, Prologue: Prologue(hostID),
		StaticKeypair: device, PeerStatic: hostKey, Random: rng,
	})
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	hello.V = protocolVersion
	payload, err := json.Marshal(hello)
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	msg1, _, _, err := hs.WriteMessage(nil, payload)
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: %v", ErrHandshake, err)
	}
	wctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	err = conn.WriteMsg(wctx, msg1)
	cancel()
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: 写第一条消息: %v", ErrHandshake, err)
	}
	rctx, cancel := context.WithTimeout(ctx, HandshakeTimeout)
	msg2, err := conn.ReadMsg(rctx)
	cancel()
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: 读第二条消息: %v", ErrHandshake, err)
	}
	if len(msg2) > MaxNoiseMessage {
		return nil, HostHello{}, fmt.Errorf("%w: 第二条消息太长", ErrHandshake)
	}
	p2, send, recv, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, HostHello{}, fmt.Errorf("%w: 第二条消息解不开: %v", ErrHandshake, err)
	}
	var hh HostHello
	if len(p2) > maxHelloLen || json.Unmarshal(p2, &hh) != nil || hh.V != protocolVersion {
		return nil, HostHello{}, fmt.Errorf("%w: 不认识主机的握手内容", ErrHandshake)
	}
	switch {
	case hh.Error == HelloNotPaired:
		return nil, hh, ErrNotPaired
	case hh.Error != "":
		return nil, hh, fmt.Errorf("%w: %s", ErrHandshake, hh.Error)
	case hh.Mode != ModeDevice && hh.Mode != ModePairing:
		return nil, hh, fmt.Errorf("%w: 不认识的会话种类 %q", ErrHandshake, hh.Mode)
	}
	// 发起方：Split 的第一个 CipherState 加密发给主机的消息，第二个解主机发来的消息
	return newSecureConn(conn, send, recv), hh, nil
}

// secureConn 是握手之后的加密信道：每条 Noise 传输消息装一个隧道帧。
// 发送串行（nonce 按加密顺序递增，加密与发送必须同序），用一格的信号量而不是互斥锁，等发送权时也认 ctx；接收只有一个读者。
type secureConn struct {
	conn     MsgConn
	sendLock chan struct{}
	send     *noise.CipherState
	recv     *noise.CipherState
}

func newSecureConn(conn MsgConn, send, recv *noise.CipherState) *secureConn {
	return &secureConn{conn: conn, sendLock: make(chan struct{}, 1), send: send, recv: recv}
}

// WriteFrame 加密并发出一个帧。
func (c *secureConn) WriteFrame(ctx context.Context, f Frame) error {
	pt, err := AppendFrame(make([]byte, 0, FrameHeaderLen+len(f.Payload)), f)
	if err != nil {
		return err
	}
	select {
	case c.sendLock <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.sendLock }()
	ct, err := c.send.Encrypt(make([]byte, 0, len(pt)+TagLen), nil, pt)
	if err != nil {
		return err
	}
	return c.conn.WriteMsg(ctx, ct)
}

// ReadFrame 读一条消息、解密并解出帧。只能有一个调用者。
func (c *secureConn) ReadFrame(ctx context.Context) (Frame, error) {
	msg, err := c.conn.ReadMsg(ctx)
	if err != nil {
		return Frame{}, err
	}
	if len(msg) < TagLen+FrameHeaderLen || len(msg) > MaxNoiseMessage {
		return Frame{}, fmt.Errorf("%w: 消息长度 %d 字节", ErrFrame, len(msg))
	}
	pt, err := c.recv.Decrypt(nil, nil, msg)
	if err != nil {
		return Frame{}, fmt.Errorf("解密失败: %w", err)
	}
	return ParseFrame(pt)
}
