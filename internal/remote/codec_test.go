package remote

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedKeys(t testing.TB, b byte) *HostKeys {
	t.Helper()
	seed := bytes.Repeat([]byte{b}, ed25519.SeedSize)
	dh, err := KeypairFromPrivate(bytes.Repeat([]byte{b + 1}, KeyLen))
	require.NoError(t, err)
	return &HostKeys{Sign: ed25519.NewKeyFromSeed(seed), Noise: dh}
}

// hostId 是 lower(base32(sha256(公钥)))[:26]，只有 a–z、2–7
func TestHostID(t *testing.T) {
	k := seedKeys(t, 1)
	id := k.HostID()
	assert.Len(t, id, HostIDLen)
	assert.True(t, ValidHostID(id))
	assert.Equal(t, id, HostIDOf(k.SignPublic()))
	assert.NotEqual(t, id, seedKeys(t, 9).HostID())
	for _, bad := range []string{"", strings.Repeat("a", 25), strings.Repeat("a", 27), strings.Repeat("A", 26), strings.Repeat("1", 26), strings.Repeat("8", 26)} {
		assert.False(t, ValidHostID(bad), bad)
	}
}

// 主机密钥落库的 JSON 只有私钥，读回来公钥与 hostId 都一样；格式不对的拒绝
func TestHostKeysMarshal(t *testing.T) {
	k, err := GenerateHostKeys(nil)
	require.NoError(t, err)
	b, err := k.Marshal()
	require.NoError(t, err)
	assert.NotContains(t, string(b), EncodeKey(k.Noise.Public))
	back, err := ParseHostKeys(b)
	require.NoError(t, err)
	assert.Equal(t, k.HostID(), back.HostID())
	assert.Equal(t, k.Noise.Public, back.Noise.Public)
	assert.Equal(t, k.Sign, back.Sign)
	for _, bad := range []string{`{`, `{"v":2}`, `{"v":1,"ed25519_seed":"AA","x25519":"AA"}`, `{"v":1,"ed25519_seed":"` + EncodeKey(make([]byte, 32)) + `","x25519":"!!"}`} {
		_, perr := ParseHostKeys([]byte(bad))
		assert.Error(t, perr, bad)
	}
	_, err = (&HostKeys{}).Marshal()
	assert.Error(t, err)
}

func TestDecodeKey(t *testing.T) {
	k := bytes.Repeat([]byte{7}, KeyLen)
	back, err := DecodeKey(EncodeKey(k))
	require.NoError(t, err)
	assert.Equal(t, k, back)
	for _, bad := range []string{"", "AA", EncodeKey(make([]byte, 31)), EncodeKey(k) + "=", "+" + EncodeKey(k)[1:]} {
		_, derr := DecodeKey(bad)
		assert.Error(t, derr, bad)
	}
	// 不规范的 base64（最后一个字符的填充位不是 0）拒绝，一个值只有一种写法
	enc := EncodeKey(k)
	last := enc[len(enc)-1]
	alt := enc[:len(enc)-1] + string(last+1)
	_, err = DecodeKey(alt)
	assert.Error(t, err)
}

func TestPairingLink(t *testing.T) {
	k := seedKeys(t, 3)
	l := PairingLink{
		HostID: k.HostID(), HostKey: k.Noise.Public, Secret: bytes.Repeat([]byte{9}, KeyLen),
		Relays: []string{"wss://relay.example.com", "ws://192.168.1.2:9000/r"}, Direct: "http://192.168.1.10:8080/pt",
	}
	s := l.String()
	assert.True(t, strings.HasPrefix(s, "pttools://pair?v=1&h="+k.HostID()+"&k="))
	assert.Contains(t, s, "&r=wss%3A%2F%2Frelay.example.com%2Cws%3A%2F%2F192.168.1.2%3A9000%2Fr")
	assert.Contains(t, s, "&d=http%3A%2F%2F192.168.1.10%3A8080%2Fpt")
	back, err := ParseLink(s)
	require.NoError(t, err)
	assert.Equal(t, l, back)

	// 只有直连地址、只有 relay 都行；不认识的参数忽略
	only, err := ParseLink(PairingLink{HostID: l.HostID, HostKey: l.HostKey, Secret: l.Secret, Direct: "https://pt.example.com"}.String() + "&x=1")
	require.NoError(t, err)
	assert.Empty(t, only.Relays)
	assert.Equal(t, "https://pt.example.com", only.Direct)

	base := "pttools://pair?v=1&h=" + l.HostID + "&k=" + EncodeKey(l.HostKey) + "&s=" + EncodeKey(l.Secret)
	cases := map[string]string{
		"别的协议":       "https://pair?v=1",
		"别的主机":       strings.Replace(base, "//pair", "//other", 1) + "&d=http%3A%2F%2Fa",
		"没有地址":       base,
		"版本不认识":      strings.Replace(base, "v=1", "v=2", 1) + "&d=http%3A%2F%2Fa",
		"没有版本":       strings.Replace(base, "v=1&", "", 1) + "&d=http%3A%2F%2Fa",
		"hostId 不对":  strings.Replace(base, "h="+l.HostID, "h=ABC", 1) + "&d=http%3A%2F%2Fa",
		"公钥短了":       strings.Replace(base, "k="+EncodeKey(l.HostKey), "k=AAAA", 1) + "&d=http%3A%2F%2Fa",
		"密钥不对":       strings.Replace(base, "s="+EncodeKey(l.Secret), "s=%%", 1) + "&d=http%3A%2F%2Fa",
		"参数重复":       base + "&h=" + l.HostID + "&d=http%3A%2F%2Fa",
		"relay 协议不对": base + "&r=https%3A%2F%2Frelay",
		"直连协议不对":     base + "&d=ws%3A%2F%2Fa",
		"relay 太多":   base + "&r=wss%3A%2F%2Fa1%2Cwss%3A%2F%2Fa2%2Cwss%3A%2F%2Fa3%2Cwss%3A%2F%2Fa4%2Cwss%3A%2F%2Fa5",
		"带片段":        base + "&d=http%3A%2F%2Fa#x",
	}
	for name, link := range cases {
		_, perr := ParseLink(link)
		assert.Error(t, perr, name)
	}
	_, err = ParseLink(strings.Replace(base, "v=1", "v=2", 1) + "&d=http%3A%2F%2Fa")
	assert.ErrorIs(t, err, ErrLinkVersion)
}

func TestNormalizeURLs(t *testing.T) {
	ok := map[string]string{
		"wss://Relay.Example.com/":    "wss://relay.example.com",
		" ws://10.0.0.2:9000/relay/ ": "ws://10.0.0.2:9000/relay",
		"WSS://[::1]:443":             "wss://[::1]:443",
	}
	for in, want := range ok {
		got, err := NormalizeRelayURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
	for _, bad := range []string{"", "https://a", "wss://", "wss://user@a", "wss://a?x=1", "wss://a#x", "wss://a,b", "wss://a/%2e", "wss://a:0", "wss://a:70000", "wss://a b", strings.Repeat("a", 300)} {
		_, err := NormalizeRelayURL(bad)
		assert.Error(t, err, bad)
	}
	d, err := NormalizeDirectURL("HTTPS://pt.example.com/sub/")
	require.NoError(t, err)
	assert.Equal(t, "https://pt.example.com/sub", d)
	_, err = NormalizeDirectURL("wss://pt.example.com")
	assert.Error(t, err)
}

// 隧道帧：每种类型的编号与内容规则；payload 正好 65514 字节可以，多 1 字节拒绝
func TestFrameRules(t *testing.T) {
	good := []Frame{
		{Type: FrameReqHead, ID: 1, Payload: []byte(`{}`)},
		{Type: FrameReqBody, ID: 1, Payload: make([]byte, MaxFramePayload)},
		{Type: FrameReqEnd, ID: 1},
		{Type: FrameCancel, ID: 1},
		{Type: FrameCancel, ID: 1, Payload: []byte("取消")},
		{Type: FrameRespHead, ID: 2, Payload: []byte(`{}`)},
		{Type: FrameRespBody, ID: 2, Payload: []byte("x")},
		{Type: FrameRespEnd, ID: 2},
		{Type: FramePing},
		{Type: FramePong, Payload: make([]byte, MaxPingPayload)},
		{Type: FrameGoAway, Payload: []byte(`{"reason":"revoked"}`)},
	}
	for _, f := range good {
		b, err := AppendFrame(nil, f)
		require.NoError(t, err, f.Type)
		assert.Len(t, b, FrameHeaderLen+len(f.Payload))
		back, err := ParseFrame(b)
		require.NoError(t, err)
		assert.Equal(t, f.Type, back.Type)
		assert.Equal(t, f.ID, back.ID)
		assert.Equal(t, len(f.Payload), len(back.Payload))
	}
	bad := []Frame{
		{Type: FrameReqBody, ID: 1, Payload: make([]byte, MaxFramePayload+1)},
		{Type: FrameReqHead, ID: 0, Payload: []byte(`{}`)},
		{Type: FrameReqBody, ID: 1},
		{Type: FrameReqEnd, ID: 1, Payload: []byte("x")},
		{Type: FrameRespEnd},
		{Type: FrameCancel, ID: 1, Payload: make([]byte, MaxReasonLen+1)},
		{Type: FrameCancel, ID: 1, Payload: []byte{0xff, 0xfe}},
		{Type: FramePing, ID: 1},
		{Type: FramePong, Payload: make([]byte, MaxPingPayload+1)},
		{Type: FrameGoAway},
		{Type: FrameGoAway, ID: 3, Payload: []byte("x")},
		{Type: 0x7f, ID: 1},
	}
	for _, f := range bad {
		_, err := AppendFrame(nil, f)
		assert.ErrorIs(t, err, ErrFrame, "%+v", f.Type)
	}
	assert.Equal(t, 65519, MaxPlaintext)
	assert.Equal(t, 65514, MaxFramePayload)
	_, err := ParseFrame(make([]byte, 4))
	assert.ErrorIs(t, err, ErrFrame)
	_, err = ParseFrame(append([]byte{byte(FrameReqBody), 0, 0, 0, 1}, make([]byte, MaxFramePayload+1)...))
	assert.ErrorIs(t, err, ErrFrame)
}

// relay 外层帧：最大 65540 字节（帧头加一条 65535 字节的 Noise 消息），多 1 字节拒绝
func TestOuterRules(t *testing.T) {
	assert.Equal(t, 65540, MaxOuterFrame)
	good := []OuterFrame{
		{Type: OuterChallenge, Payload: make([]byte, NonceLen)},
		{Type: OuterAuth, Payload: make([]byte, AuthPayloadLen)},
		{Type: OuterReady},
		{Type: OuterOpen, Stream: 1},
		{Type: OuterData, Stream: 1, Payload: make([]byte, MaxNoiseMessage)},
		{Type: OuterClose, Stream: 2},
		{Type: OuterClose, Stream: 2, Payload: ClosePayload(4429, "too many")},
	}
	for _, f := range good {
		b, err := AppendOuter(nil, f)
		require.NoError(t, err, f.Type)
		back, err := ParseOuter(b)
		require.NoError(t, err)
		assert.Equal(t, f.Type, back.Type)
		assert.Equal(t, f.Stream, back.Stream)
	}
	b, err := AppendOuter(nil, OuterFrame{Type: OuterData, Stream: 1, Payload: make([]byte, MaxNoiseMessage)})
	require.NoError(t, err)
	assert.Len(t, b, MaxOuterFrame)
	_, err = ParseOuter(append(b, 0))
	assert.ErrorIs(t, err, ErrOuter)
	bad := []OuterFrame{
		{Type: OuterChallenge, Stream: 1, Payload: make([]byte, NonceLen)},
		{Type: OuterChallenge, Payload: make([]byte, NonceLen-1)},
		{Type: OuterAuth, Payload: make([]byte, 10)},
		{Type: OuterReady, Payload: []byte("x")},
		{Type: OuterOpen},
		{Type: OuterData, Stream: 1},
		{Type: OuterData, Stream: 1, Payload: make([]byte, MaxNoiseMessage+1)},
		{Type: OuterClose, Stream: 1, Payload: []byte{1}},
		{Type: OuterClose, Stream: 1, Payload: append([]byte{0, 1}, make([]byte, 124)...)},
		{Type: OuterClose, Stream: 0},
		{Type: 0x55, Stream: 1},
	}
	for _, f := range bad {
		_, err := AppendOuter(nil, f)
		assert.ErrorIs(t, err, ErrOuter, "%+v", f.Type)
	}
	code, reason := ParseClosePayload(ClosePayload(4404, strings.Repeat("长", 60)))
	assert.Equal(t, uint16(4404), code)
	assert.LessOrEqual(t, len(reason), 123)
	assert.True(t, strings.HasPrefix(strings.Repeat("长", 60), reason))
}

// relay 认证：签名签进 hostId、质询与 relay 的 origin；换一个 origin、质询或者 hostId 都校验不过
func TestRelayAuth(t *testing.T) {
	k := seedKeys(t, 5)
	nonce := bytes.Repeat([]byte{1}, NonceLen)
	auth := k.RelayAuth(nonce, "wss://relay.example.com")
	require.Len(t, auth, AuthPayloadLen)
	require.NoError(t, VerifyRelayAuth(k.HostID(), nonce, "wss://relay.example.com", auth))
	assert.Error(t, VerifyRelayAuth(k.HostID(), nonce, "wss://evil.example.com", auth))
	assert.Error(t, VerifyRelayAuth(k.HostID(), bytes.Repeat([]byte{2}, NonceLen), "wss://relay.example.com", auth))
	assert.Error(t, VerifyRelayAuth(seedKeys(t, 6).HostID(), nonce, "wss://relay.example.com", auth))
	assert.Error(t, VerifyRelayAuth(k.HostID(), nonce, "wss://relay.example.com", auth[:10]))
	tampered := bytes.Clone(auth)
	tampered[len(tampered)-1] ^= 1
	assert.Error(t, VerifyRelayAuth(k.HostID(), nonce, "wss://relay.example.com", tampered))
}

func TestRelayOrigin(t *testing.T) {
	cases := map[string]string{
		"wss://Relay.Example.com/v1":   "wss://relay.example.com",
		"wss://relay.example.com:443/": "wss://relay.example.com",
		"ws://relay.example.com:80":    "ws://relay.example.com",
		"ws://10.0.0.2:9000/r":         "ws://10.0.0.2:9000",
		"wss://[::1]:8443":             "wss://[::1]:8443",
	}
	for in, want := range cases {
		got, err := RelayOrigin(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := RelayOrigin("https://relay.example.com")
	assert.Error(t, err)
}

func FuzzParseFrame(f *testing.F) {
	for _, seed := range [][]byte{{1, 0, 0, 0, 1, '{', '}'}, {0x20, 0, 0, 0, 0}, {0x30, 0, 0, 0, 0, 'x'}, {0x13, 0, 0, 0, 9}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		fr, err := ParseFrame(b)
		if err != nil {
			return
		}
		again, err := AppendFrame(nil, fr)
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("解出来的帧编不回原样: %v", err)
		}
	})
}

func FuzzParseOuter(f *testing.F) {
	for _, seed := range [][]byte{append([]byte{1, 0, 0, 0, 0}, make([]byte, 32)...), {0x10, 0, 0, 0, 1}, {0x11, 0, 0, 0, 1, 9}, {0x12, 0, 0, 0, 1, 0x11, 0x4d, 'x'}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		fr, err := ParseOuter(b)
		if err != nil {
			return
		}
		again, err := AppendOuter(nil, fr)
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("解出来的外层帧编不回原样: %v", err)
		}
	})
}

func FuzzParseLink(f *testing.F) {
	k := seedKeys(f, 3)
	f.Add(PairingLink{HostID: k.HostID(), HostKey: k.Noise.Public, Secret: make([]byte, 32), Relays: []string{"wss://r.example.com"}, Direct: "http://a:1"}.String())
	f.Add("pttools://pair?v=1")
	f.Fuzz(func(t *testing.T, s string) {
		l, err := ParseLink(s)
		if err != nil {
			return
		}
		back, err := ParseLink(l.String())
		if err != nil {
			t.Fatalf("解出来的链接写回去以后解不开: %v", err)
		}
		if back.HostID != l.HostID || !bytes.Equal(back.Secret, l.Secret) || back.Direct != l.Direct || len(back.Relays) != len(l.Relays) {
			t.Fatalf("写回去以后内容变了")
		}
	})
}
