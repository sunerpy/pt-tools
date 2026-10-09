package remote

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// relay 外层帧（v1）：主机与 relay 之间的那条 WebSocket 里，每条二进制消息是一个外层帧
// [type u8][stream u32 大端][payload]，最大 65540 字节。stream 0 是控制（认证），客户端的流从 1 开始，由 relay 分配。
// 客户端与 relay 之间的 WebSocket 和直连一样，每条二进制消息就是一条 Noise 消息，relay 把它装进 DATA 转给主机。
const (
	// OuterHeaderLen 是外层帧头的长度
	OuterHeaderLen = 5
	// MaxOuterFrame 是一个外层帧的上限：帧头加一条 Noise 消息
	MaxOuterFrame = OuterHeaderLen + MaxNoiseMessage
	// NonceLen 是 relay 认证质询的长度
	NonceLen = 32
	// AuthPayloadLen 是 AUTH 的长度：Ed25519 公钥加签名
	AuthPayloadLen = ed25519.PublicKeySize + ed25519.SignatureSize
	// maxCloseReason 是 CLOSE 原因的上限（和 WebSocket 关闭原因一样是 123 字节）
	maxCloseReason = 123
	// relayAuthContext 是主机签名内容的开头，防止签名被挪到别的协议里用
	relayAuthContext = "pt-tools-relay-v1"
)

// OuterType 是外层帧的类型。
type OuterType uint8

// 外层帧的类型。
const (
	// OuterChallenge 是 relay 发给主机的认证质询（stream 0，32 字节随机数）
	OuterChallenge OuterType = 0x01
	// OuterAuth 是主机的回答（stream 0）：Ed25519 公钥加签名
	OuterAuth OuterType = 0x02
	// OuterReady 是 relay 确认认证通过（stream 0，不带内容）
	OuterReady OuterType = 0x03
	// OuterOpen 是 relay 通知主机有新的客户端流（不带内容）
	OuterOpen OuterType = 0x10
	// OuterData 是一条 Noise 消息，两个方向都有
	OuterData OuterType = 0x11
	// OuterClose 是关掉一个流，两个方向都有；内容可以是空，或者 u16 关闭码加 UTF-8 原因
	OuterClose OuterType = 0x12
)

// relay 用的 WebSocket 关闭码（4000–4999 是应用自定义）。
const (
	CloseProtocol    = 4400
	CloseAuthFailed  = 4401
	CloseHostOffline = 4404
	CloseReplaced    = 4409
	CloseLimited     = 4429
	CloseDisabled    = 4503
)

// ErrOuter 是外层帧格式不对。
var ErrOuter = errors.New("relay 外层帧格式不对")

// OuterFrame 是一个外层帧。
type OuterFrame struct {
	Type    OuterType
	Stream  uint32
	Payload []byte
}

func checkOuter(f OuterFrame) error {
	n := len(f.Payload)
	switch f.Type {
	case OuterChallenge:
		if f.Stream != 0 || n != NonceLen {
			return fmt.Errorf("%w: CHALLENGE 在流 0 上，内容 %d 字节", ErrOuter, NonceLen)
		}
	case OuterAuth:
		if f.Stream != 0 || n != AuthPayloadLen {
			return fmt.Errorf("%w: AUTH 在流 0 上，内容 %d 字节", ErrOuter, AuthPayloadLen)
		}
	case OuterReady:
		if f.Stream != 0 || n != 0 {
			return fmt.Errorf("%w: READY 在流 0 上，不带内容", ErrOuter)
		}
	case OuterOpen:
		if f.Stream == 0 || n != 0 {
			return fmt.Errorf("%w: OPEN 要有流编号、不带内容", ErrOuter)
		}
	case OuterData:
		if f.Stream == 0 || n == 0 || n > MaxNoiseMessage {
			return fmt.Errorf("%w: DATA 要有流编号，内容 1 到 %d 字节", ErrOuter, MaxNoiseMessage)
		}
	case OuterClose:
		if f.Stream == 0 || n == 1 || n > 2+maxCloseReason || (n > 2 && !utf8.Valid(f.Payload[2:])) {
			return fmt.Errorf("%w: CLOSE 要有流编号，内容是空或者关闭码加最多 %d 字节的原因", ErrOuter, maxCloseReason)
		}
	default:
		return fmt.Errorf("%w: 不认识的类型 0x%02x", ErrOuter, byte(f.Type))
	}
	return nil
}

// AppendOuter 把外层帧编码以后追加到 dst。
func AppendOuter(dst []byte, f OuterFrame) ([]byte, error) {
	if err := checkOuter(f); err != nil {
		return dst, err
	}
	dst = append(dst, byte(f.Type))
	dst = binary.BigEndian.AppendUint32(dst, f.Stream)
	return append(dst, f.Payload...), nil
}

// ParseOuter 解码一个外层帧。Payload 引用 b 的内容，不复制。
func ParseOuter(b []byte) (OuterFrame, error) {
	if len(b) < OuterHeaderLen || len(b) > MaxOuterFrame {
		return OuterFrame{}, fmt.Errorf("%w: 长度 %d 字节", ErrOuter, len(b))
	}
	f := OuterFrame{Type: OuterType(b[0]), Stream: binary.BigEndian.Uint32(b[1:OuterHeaderLen]), Payload: b[OuterHeaderLen:]}
	if err := checkOuter(f); err != nil {
		return OuterFrame{}, err
	}
	return f, nil
}

// ClosePayload 是 CLOSE 的内容：u16 关闭码（大端）加原因；原因超长时截断在字符边界上。
func ClosePayload(code uint16, reason string) []byte {
	for len(reason) > maxCloseReason {
		_, size := utf8.DecodeLastRuneInString(reason)
		reason = reason[:len(reason)-size]
	}
	return append(binary.BigEndian.AppendUint16(nil, code), reason...)
}

// ParseClosePayload 读 CLOSE 的内容；空内容的关闭码是 0。
func ParseClosePayload(p []byte) (code uint16, reason string) {
	if len(p) < 2 {
		return 0, ""
	}
	return binary.BigEndian.Uint16(p), string(p[2:])
}

// RelayAuthMessage 是主机向 relay 证明身份时签名的内容："pt-tools-relay-v1" || hostId || nonce || relay origin。
// 签进 origin，别的 relay 拿到这个签名也冒充不了这台主机。
func RelayAuthMessage(hostID string, nonce []byte, origin string) []byte {
	msg := make([]byte, 0, len(relayAuthContext)+len(hostID)+len(nonce)+len(origin))
	msg = append(msg, relayAuthContext...)
	msg = append(msg, hostID...)
	msg = append(msg, nonce...)
	return append(msg, origin...)
}

// RelayAuth 是主机对质询的回答（AUTH 的内容）：Ed25519 公钥加签名。
func (k *HostKeys) RelayAuth(nonce []byte, origin string) []byte {
	sig := ed25519.Sign(k.Sign, RelayAuthMessage(k.HostID(), nonce, origin))
	return append(append(make([]byte, 0, AuthPayloadLen), k.SignPublic()...), sig...)
}

// VerifyRelayAuth 是 relay 端的校验：公钥推导出的 hostId 要对得上，签名要对。
func VerifyRelayAuth(hostID string, nonce []byte, origin string, payload []byte) error {
	if len(payload) != AuthPayloadLen || len(nonce) != NonceLen {
		return fmt.Errorf("%w: AUTH 长度不对", ErrOuter)
	}
	pub := ed25519.PublicKey(payload[:ed25519.PublicKeySize])
	if HostIDOf(pub) != hostID {
		return errors.New("公钥和 hostId 对不上")
	}
	if !ed25519.Verify(pub, RelayAuthMessage(hostID, nonce, origin), payload[ed25519.PublicKeySize:]) {
		return errors.New("签名不对")
	}
	return nil
}

// RelayOrigin 是 relay 地址的 origin：scheme://主机[:端口]，小写，去掉默认端口（ws 的 80、wss 的 443）。
// 主机按它连上的地址签名，relay 按自己对外的地址校验。
func RelayOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "ws" && scheme != "wss") || u.Hostname() == "" {
		return "", fmt.Errorf("%w: relay 地址要以 ws:// 或 wss:// 开头", ErrURL)
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" && !defaultPort(scheme, p) {
		host += ":" + p
	}
	return scheme + "://" + host, nil
}

// defaultPort 报告 port 是不是这个协议的默认端口（ws 的 80、wss 的 443）。
func defaultPort(scheme, port string) bool {
	return (scheme == "ws" && port == "80") || (scheme == "wss" && port == "443")
}
