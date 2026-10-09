package remote

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

// 各层的大小（v1 冻结）。
const (
	// MaxNoiseMessage 是一条 Noise 消息（握手消息或传输消息的密文，含认证标签）的上限
	MaxNoiseMessage = 65535
	// TagLen 是 ChaCha20-Poly1305 认证标签的长度
	TagLen = 16
	// MaxPlaintext 是一条 Noise 传输消息里明文的上限
	MaxPlaintext = MaxNoiseMessage - TagLen
	// FrameHeaderLen 是隧道帧头的长度：[type u8][req_id u32 大端]
	FrameHeaderLen = 5
	// MaxFramePayload 是一个隧道帧 payload 的上限
	MaxFramePayload = MaxPlaintext - FrameHeaderLen
	// MaxPingPayload 是 PING、PONG 的 payload 上限
	MaxPingPayload = 64
	// MaxReasonLen 是 CANCEL 与 GOAWAY 的 payload 上限
	MaxReasonLen = 256
)

// FrameType 是隧道帧的类型。
type FrameType uint8

// 隧道帧的类型。REQ_* 由设备发，RESP_* 由主机发，CANCEL、PING、PONG 两边都发，GOAWAY 只由主机发。
const (
	FrameReqHead  FrameType = 0x01
	FrameReqBody  FrameType = 0x02
	FrameReqEnd   FrameType = 0x03
	FrameCancel   FrameType = 0x04
	FrameRespHead FrameType = 0x11
	FrameRespBody FrameType = 0x12
	FrameRespEnd  FrameType = 0x13
	FramePing     FrameType = 0x20
	FramePong     FrameType = 0x21
	FrameGoAway   FrameType = 0x30
)

// ErrFrame 是隧道帧格式不对。
var ErrFrame = errors.New("隧道帧格式不对")

// Frame 是一个隧道帧：每条 Noise 传输消息的明文恰好是一个帧。
type Frame struct {
	Type FrameType
	// ID 是请求编号：请求与回应的帧不为 0，PING、PONG、GOAWAY 为 0
	ID      uint32
	Payload []byte
}

// checkFrame 检查类型、编号与 payload 的规则，编码与解码共用。
func checkFrame(f Frame) error {
	n := len(f.Payload)
	if n > MaxFramePayload {
		return fmt.Errorf("%w: payload %d 字节，超过 %d", ErrFrame, n, MaxFramePayload)
	}
	switch f.Type {
	case FrameReqHead, FrameReqBody, FrameRespHead, FrameRespBody:
		if f.ID == 0 || n == 0 {
			return fmt.Errorf("%w: 类型 0x%02x 要有请求编号与内容", ErrFrame, byte(f.Type))
		}
	case FrameReqEnd, FrameRespEnd:
		if f.ID == 0 || n != 0 {
			return fmt.Errorf("%w: 类型 0x%02x 要有请求编号、不带内容", ErrFrame, byte(f.Type))
		}
	case FrameCancel:
		if f.ID == 0 || n > MaxReasonLen || !utf8.Valid(f.Payload) {
			return fmt.Errorf("%w: CANCEL 要有请求编号，原因最多 %d 字节 UTF-8", ErrFrame, MaxReasonLen)
		}
	case FramePing, FramePong:
		if f.ID != 0 || n > MaxPingPayload {
			return fmt.Errorf("%w: PING、PONG 的编号是 0，内容最多 %d 字节", ErrFrame, MaxPingPayload)
		}
	case FrameGoAway:
		if f.ID != 0 || n == 0 || n > MaxReasonLen {
			return fmt.Errorf("%w: GOAWAY 的编号是 0，内容 1 到 %d 字节", ErrFrame, MaxReasonLen)
		}
	default:
		return fmt.Errorf("%w: 不认识的类型 0x%02x", ErrFrame, byte(f.Type))
	}
	return nil
}

// AppendFrame 把帧编码以后追加到 dst。
func AppendFrame(dst []byte, f Frame) ([]byte, error) {
	if err := checkFrame(f); err != nil {
		return dst, err
	}
	dst = append(dst, byte(f.Type))
	dst = binary.BigEndian.AppendUint32(dst, f.ID)
	return append(dst, f.Payload...), nil
}

// ParseFrame 解码一个帧。Payload 引用 b 的内容，不复制。
func ParseFrame(b []byte) (Frame, error) {
	if len(b) < FrameHeaderLen || len(b) > MaxPlaintext {
		return Frame{}, fmt.Errorf("%w: 长度 %d 字节", ErrFrame, len(b))
	}
	f := Frame{Type: FrameType(b[0]), ID: binary.BigEndian.Uint32(b[1:FrameHeaderLen]), Payload: b[FrameHeaderLen:]}
	if err := checkFrame(f); err != nil {
		return Frame{}, err
	}
	return f, nil
}

// RequestHead 是 REQ_HEAD 的内容（JSON）。Path 是以 / 开头的路径，可以带查询串；Headers 只有白名单里的头会被转发。
type RequestHead struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

// ResponseHead 是 RESP_HEAD 的内容（JSON）。
type ResponseHead struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GoAway 是 GOAWAY 的内容（JSON）：主机随后关闭这条会话。
type GoAway struct {
	Reason string `json:"reason"`
}

// GOAWAY 的原因。
const (
	// GoAwayRevoked 是这台设备被撤销了
	GoAwayRevoked = "revoked"
	// GoAwayScopeChanged 是这台设备的权限改了，重连以后按新权限
	GoAwayScopeChanged = "scope_changed"
	// GoAwayDisabled 是远程访问关掉了
	GoAwayDisabled = "disabled"
	// GoAwayKeyRotated 是主机密钥换了，所有设备要重新配对
	GoAwayKeyRotated = "key_rotated"
	// GoAwayPaired 是配对完成，设备用正常会话重连
	GoAwayPaired = "paired"
	// GoAwayPairingClosed 是配对窗口关了（过期、取消或输错太多次）
	GoAwayPairingClosed = "pairing_closed"
	// GoAwayShutdown 是 pt-tools 正在退出
	GoAwayShutdown = "shutdown"
	// GoAwayProtocol 是对端违反了协议
	GoAwayProtocol = "protocol_error"
)
