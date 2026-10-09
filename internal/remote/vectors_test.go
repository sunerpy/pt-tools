package remote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/vectors.json 是 v1 线格式的测试向量，App（Dart）用同一份文件核对自己的实现。
// 超长的帧不放进文件（太大），大小边界按 docs/design/remote-access.md 里冻结的数字各自测。
// 改了协议实现以后跑 `go test ./internal/remote -run TestVectors -update` 重新生成；平时这个测试核对实现与文件一字不差。
var updateVectors = flag.Bool("update", false, "重新生成 testdata/vectors.json")

const vectorsFile = "testdata/vectors.json"

type hexBytes []byte

func (h hexBytes) MarshalJSON() ([]byte, error) { return json.Marshal(hex.EncodeToString(h)) }

func (h *hexBytes) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := hex.DecodeString(s)
	*h = v
	return err
}

type vecHostID struct {
	Ed25519Seed   hexBytes `json:"ed25519_seed"`
	Ed25519Public hexBytes `json:"ed25519_public"`
	HostID        string   `json:"host_id"`
}

type vecLink struct {
	Link    string   `json:"link"`
	HostID  string   `json:"host_id"`
	HostKey hexBytes `json:"host_key"`
	Secret  hexBytes `json:"secret"`
	Relays  []string `json:"relays"`
	Direct  string   `json:"direct"`
}

type vecFrame struct {
	Type    int      `json:"type"`
	ID      uint32   `json:"id"`
	Payload hexBytes `json:"payload"`
	Encoded hexBytes `json:"encoded"`
}

type vecOuter struct {
	Type    int      `json:"type"`
	Stream  uint32   `json:"stream"`
	Payload hexBytes `json:"payload"`
	Encoded hexBytes `json:"encoded"`
}

type vecRelayAuth struct {
	Ed25519Seed hexBytes `json:"ed25519_seed"`
	HostID      string   `json:"host_id"`
	Nonce       hexBytes `json:"nonce"`
	Origin      string   `json:"origin"`
	Message     hexBytes `json:"message"`
	Auth        hexBytes `json:"auth"`
}

type vecMessage struct {
	From string `json:"from"`
	// Kind 是 handshake（Payload 是握手内容的 JSON）或 frame（Frame 是这条传输消息的明文）
	Kind       string    `json:"kind"`
	Payload    string    `json:"payload,omitempty"`
	Frame      *vecFrame `json:"frame,omitempty"`
	Ciphertext hexBytes  `json:"ciphertext"`
}

type vecSession struct {
	HostID             string       `json:"host_id"`
	Prologue           hexBytes     `json:"prologue"`
	HostEd25519Seed    hexBytes     `json:"host_ed25519_seed"`
	HostStatic         hexBytes     `json:"host_static"`
	HostStaticPublic   hexBytes     `json:"host_static_public"`
	DeviceStatic       hexBytes     `json:"device_static"`
	DeviceStaticPublic hexBytes     `json:"device_static_public"`
	DeviceEphemeral    hexBytes     `json:"device_ephemeral"`
	HostEphemeral      hexBytes     `json:"host_ephemeral"`
	Messages           []vecMessage `json:"messages"`
}

type vectors struct {
	Comment       string       `json:"_comment"`
	Protocol      string       `json:"protocol"`
	HostIDs       []vecHostID  `json:"host_ids"`
	Links         []vecLink    `json:"links"`
	InvalidLinks  []string     `json:"invalid_links"`
	Frames        []vecFrame   `json:"frames"`
	InvalidFrames []hexBytes   `json:"invalid_frames"`
	Outer         []vecOuter   `json:"outer_frames"`
	InvalidOuter  []hexBytes   `json:"invalid_outer_frames"`
	RelayAuth     vecRelayAuth `json:"relay_auth"`
	Session       vecSession   `json:"session"`
}

func rep(b byte, n int) []byte { return bytes.Repeat([]byte{b}, n) }

// recordConn 记下写出去的每一条消息。
type recordConn struct {
	MsgConn
	mu   sync.Mutex
	sent [][]byte
}

func (r *recordConn) WriteMsg(ctx context.Context, b []byte) error {
	r.mu.Lock()
	r.sent = append(r.sent, append([]byte(nil), b...))
	r.mu.Unlock()
	return r.MsgConn.WriteMsg(ctx, b)
}

func (r *recordConn) take() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.sent[0]
	r.sent = r.sent[1:]
	return b
}

func buildVectors(t *testing.T) vectors {
	t.Helper()
	v := vectors{
		Comment:  "pt-tools 远程访问 v1 的测试向量（docs/design/remote-access.md）。二进制值都是十六进制；由 internal/remote 的 TestVectors 生成并核对，App 端用同一份文件。",
		Protocol: ProtocolName,
	}
	for _, b := range []byte{0, 1, 0x42} {
		k := ed25519.NewKeyFromSeed(rep(b, ed25519.SeedSize))
		pub := k.Public().(ed25519.PublicKey)
		v.HostIDs = append(v.HostIDs, vecHostID{Ed25519Seed: rep(b, ed25519.SeedSize), Ed25519Public: hexBytes(pub), HostID: HostIDOf(pub)})
	}

	host := seedKeys(t, 0x10)
	links := []PairingLink{
		{HostID: host.HostID(), HostKey: host.Noise.Public, Secret: rep(0xaa, KeyLen), Relays: []string{"wss://relay.example.com"}, Direct: "http://192.168.1.10:8080"},
		{HostID: host.HostID(), HostKey: host.Noise.Public, Secret: rep(0xbb, KeyLen), Direct: "https://pt.example.com/pt-tools"},
		{HostID: host.HostID(), HostKey: host.Noise.Public, Secret: rep(0xcc, KeyLen), Relays: []string{"wss://a.example.com", "ws://10.0.0.2:9000/relay"}},
	}
	for _, l := range links {
		v.Links = append(v.Links, vecLink{Link: l.String(), HostID: l.HostID, HostKey: l.HostKey, Secret: l.Secret, Relays: append([]string{}, l.Relays...), Direct: l.Direct})
	}
	base := "pttools://pair?v=1&h=" + host.HostID() + "&k=" + EncodeKey(host.Noise.Public) + "&s=" + EncodeKey(rep(0xaa, KeyLen))
	v.InvalidLinks = []string{
		base,
		"pttools://pair?v=2&h=" + host.HostID() + "&k=" + EncodeKey(host.Noise.Public) + "&s=" + EncodeKey(rep(0xaa, KeyLen)) + "&d=http%3A%2F%2Fa",
		base + "&d=ftp%3A%2F%2Fa",
		base + "&r=https%3A%2F%2Frelay",
		base + "&h=" + host.HostID() + "&d=http%3A%2F%2Fa",
		"pttools://pair?v=1&h=short&k=" + EncodeKey(host.Noise.Public) + "&s=" + EncodeKey(rep(0xaa, KeyLen)) + "&d=http%3A%2F%2Fa",
		"https://pair?v=1",
	}

	head, _ := json.Marshal(RequestHead{Method: "GET", Path: "/api/app/v1/meta", Headers: map[string]string{"Accept": "application/json"}})
	for _, f := range []Frame{
		{Type: FrameReqHead, ID: 1, Payload: head},
		{Type: FrameReqBody, ID: 1, Payload: []byte("hello")},
		{Type: FrameReqEnd, ID: 1},
		{Type: FrameCancel, ID: 0x01020304},
		{Type: FrameRespHead, ID: 1, Payload: []byte(`{"status":200}`)},
		{Type: FrameRespBody, ID: 1, Payload: []byte("{}")},
		{Type: FrameRespEnd, ID: 1},
		{Type: FramePing, Payload: []byte("p")},
		{Type: FramePong, Payload: []byte("p")},
		{Type: FrameGoAway, Payload: []byte(`{"reason":"revoked"}`)},
	} {
		enc, err := AppendFrame(nil, f)
		require.NoError(t, err)
		v.Frames = append(v.Frames, vecFrame{Type: int(f.Type), ID: f.ID, Payload: append(hexBytes{}, f.Payload...), Encoded: enc})
	}
	v.InvalidFrames = []hexBytes{
		{0x01, 0, 0, 0},
		{0x01, 0, 0, 0, 0, '{', '}'},
		{0x02, 0, 0, 0, 1},
		{0x03, 0, 0, 0, 1, 'x'},
		{0x20, 0, 0, 0, 1},
		{0x30, 0, 0, 0, 0},
		{0x7f, 0, 0, 0, 1},
	}

	for _, f := range []OuterFrame{
		{Type: OuterChallenge, Payload: rep(7, NonceLen)},
		{Type: OuterReady},
		{Type: OuterOpen, Stream: 1},
		{Type: OuterData, Stream: 1, Payload: []byte{1, 2, 3}},
		{Type: OuterClose, Stream: 1},
		{Type: OuterClose, Stream: 0xfffffffe, Payload: ClosePayload(CloseLimited, "too many streams")},
	} {
		enc, err := AppendOuter(nil, f)
		require.NoError(t, err)
		v.Outer = append(v.Outer, vecOuter{Type: int(f.Type), Stream: f.Stream, Payload: append(hexBytes{}, f.Payload...), Encoded: enc})
	}
	v.InvalidOuter = []hexBytes{
		{0x10, 0, 0, 0, 0},
		{0x11, 0, 0, 0, 1},
		{0x12, 0, 0, 0, 1, 9},
		{0x03, 0, 0, 0, 1},
	}

	nonce := rep(0x5a, NonceLen)
	origin := "wss://relay.example.com"
	v.RelayAuth = vecRelayAuth{
		Ed25519Seed: rep(0x10, ed25519.SeedSize), HostID: host.HostID(), Nonce: nonce, Origin: origin,
		Message: RelayAuthMessage(host.HostID(), nonce, origin), Auth: host.RelayAuth(nonce, origin),
	}

	v.Session = buildSessionVector(t, host)
	return v
}

// buildSessionVector 用固定的密钥与临时密钥跑一遍真实的握手与几条传输消息，记下线上的字节。
func buildSessionVector(t *testing.T, host *HostKeys) vecSession {
	t.Helper()
	device, err := KeypairFromPrivate(rep(0x21, KeyLen))
	require.NoError(t, err)
	devEph, hostEph := rep(0x31, KeyLen), rep(0x41, KeyLen)
	a, b := memPipe()
	da, hb := &recordConn{MsgConn: a}, &recordConn{MsgConn: b}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type res struct {
		c   *secureConn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, _, _, herr := acceptHandshake(ctx, hb, host, host.HostID(), bytes.NewReader(hostEph), func([]byte, ClientHello) HostHello {
			return HostHello{Mode: ModeDevice, Host: "v1.0.0"}
		})
		ch <- res{c, herr}
	}()
	dc, _, err := dialHandshake(ctx, da, host.HostID(), host.Noise.Public, device, bytes.NewReader(devEph), ClientHello{Client: "pt-tools-app/1.0.0"})
	require.NoError(t, err)
	r := <-ch
	require.NoError(t, r.err)
	hc := r.c

	s := vecSession{
		HostID: host.HostID(), Prologue: Prologue(host.HostID()), HostEd25519Seed: host.Sign.Seed(),
		HostStatic: host.Noise.Private, HostStaticPublic: host.Noise.Public,
		DeviceStatic: device.Private, DeviceStaticPublic: device.Public,
		DeviceEphemeral: devEph, HostEphemeral: hostEph,
	}
	s.Messages = append(s.Messages,
		vecMessage{From: "device", Kind: "handshake", Payload: `{"v":1,"client":"pt-tools-app/1.0.0"}`, Ciphertext: da.take()},
		vecMessage{From: "host", Kind: "handshake", Payload: `{"v":1,"mode":"device","host":"v1.0.0"}`, Ciphertext: hb.take()},
	)
	head, _ := json.Marshal(RequestHead{Method: "GET", Path: "/api/app/v1/meta"})
	respHead, _ := json.Marshal(ResponseHead{Status: 200, Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}})
	steps := []struct {
		from string
		f    Frame
	}{
		{"device", Frame{Type: FrameReqHead, ID: 1, Payload: head}},
		{"device", Frame{Type: FrameReqEnd, ID: 1}},
		{"host", Frame{Type: FrameRespHead, ID: 1, Payload: respHead}},
		{"host", Frame{Type: FrameRespBody, ID: 1, Payload: []byte(`{"name":"pt-tools"}`)}},
		{"host", Frame{Type: FrameRespEnd, ID: 1}},
		{"device", Frame{Type: FramePing}},
		{"host", Frame{Type: FramePong}},
		{"host", Frame{Type: FrameGoAway, Payload: []byte(`{"reason":"revoked"}`)}},
	}
	for _, st := range steps {
		sender, rec, receiver := dc, da, hc
		if st.from == "host" {
			sender, rec, receiver = hc, hb, dc
		}
		require.NoError(t, sender.WriteFrame(ctx, st.f))
		got, err := receiver.ReadFrame(ctx)
		require.NoError(t, err)
		require.Equal(t, st.f.Type, got.Type)
		enc, _ := AppendFrame(nil, st.f)
		s.Messages = append(s.Messages, vecMessage{
			From: st.from, Kind: "frame", Ciphertext: rec.take(),
			Frame: &vecFrame{Type: int(st.f.Type), ID: st.f.ID, Payload: append(hexBytes{}, st.f.Payload...), Encoded: enc},
		})
	}
	return s
}

// 实现与 testdata/vectors.json 一字不差；文件里的每一项也都能被实现读回去（无效的被拒绝）
func TestVectors(t *testing.T) {
	built := buildVectors(t)
	out, err := json.MarshalIndent(built, "", "  ")
	require.NoError(t, err)
	out = append(out, '\n')
	if *updateVectors {
		require.NoError(t, os.WriteFile(vectorsFile, out, 0o644))
	}
	want, err := os.ReadFile(vectorsFile)
	require.NoError(t, err, "先跑 go test ./internal/remote -run TestVectors -update 生成向量文件")
	require.Equal(t, string(want), string(out), "实现与向量文件不一致：确认是有意改协议以后再 -update")

	var v vectors
	require.NoError(t, json.Unmarshal(want, &v))
	for _, h := range v.HostIDs {
		assert.Equal(t, h.HostID, HostIDOf(ed25519.NewKeyFromSeed(h.Ed25519Seed).Public().(ed25519.PublicKey)))
	}
	for _, l := range v.Links {
		got, err := ParseLink(l.Link)
		require.NoError(t, err, l.Link)
		assert.Equal(t, l.HostID, got.HostID)
		assert.Equal(t, []byte(l.Secret), got.Secret)
	}
	for _, l := range v.InvalidLinks {
		_, err := ParseLink(l)
		assert.Error(t, err, l)
	}
	for _, f := range v.Frames {
		got, err := ParseFrame(f.Encoded)
		require.NoError(t, err)
		assert.Equal(t, FrameType(f.Type), got.Type)
	}
	for _, b := range v.InvalidFrames {
		_, err := ParseFrame(b)
		assert.Error(t, err)
	}
	for _, f := range v.Outer {
		_, err := ParseOuter(f.Encoded)
		require.NoError(t, err)
	}
	for _, b := range v.InvalidOuter {
		_, err := ParseOuter(b)
		assert.Error(t, err)
	}
	require.NoError(t, VerifyRelayAuth(v.RelayAuth.HostID, v.RelayAuth.Nonce, v.RelayAuth.Origin, v.RelayAuth.Auth))
}
