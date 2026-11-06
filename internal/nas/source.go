package nas

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// Source 是一个 NAS 监控来源。full 为真时连存储池、硬盘、告警与应用一起取，否则只取 CPU、内存、网络与负载。
// Collect 返回的错误按类分：*AuthError（errors.Is ErrAuth）、*CertError（ErrCertUntrusted 或 ErrCertChanged）、
// ErrUnsupported，其余是网络、超时与协议错误。Close 断开来源持有的连接。
type Source interface {
	Collect(ctx context.Context, full bool) (*Snapshot, error)
	Close() error
}

// ExtraSource 是采集时会刷新长期凭据的来源（群晖的设备令牌、飞牛的 token）：采集器每次采集以后取一次，
// 有变化时经 Collector.SaveExtra 写回数据库。
type ExtraSource interface {
	// TakeExtra 返回上一次调用以后刷新过的凭据；没有变化时为 nil。
	TakeExtra() map[string]string
}

var (
	// ErrAuth 是认证失败（具体原因见 *AuthError）。
	ErrAuth = errors.New("认证失败")
	// ErrCertUntrusted 是证书链校验不过、也没有钉住的指纹（自签证书要在设置页确认）。
	ErrCertUntrusted = errors.New("证书不受信任，要在「NAS 监控」里确认这张证书")
	// ErrCertChanged 是证书和钉住的那张不一样。
	ErrCertChanged = errors.New("证书和信任的那张不一样（证书换了？）")
	// ErrUnsupported 是这个来源在这里用不了（本机来源只支持 Linux）。
	ErrUnsupported = errors.New("本机来源只支持 Linux")
)

// AuthError 是认证失败：Reason 是给人看的原因，OTP 为真表示要验证码（两步验证）。
type AuthError struct {
	Reason string
	OTP    bool
}

func (e *AuthError) Error() string {
	if e.Reason == "" {
		return ErrAuth.Error()
	}
	return ErrAuth.Error() + "：" + e.Reason
}

// Is 让 errors.Is(err, ErrAuth) 认得 *AuthError。
func (e *AuthError) Is(target error) bool { return target == ErrAuth }

// SourceConfig 是采集器看到的一个来源（密钥已经解密）。URL 是规范化以后的地址；Revision 每次保存加一；
// Broken 是这一行用不了的原因（例如密钥解不开）：采集器不连它，只显示原因。
type SourceConfig struct {
	ID          uint
	Name        string
	Kind        string
	Enabled     bool
	URL         string
	Username    string
	Secret      string
	Extra       map[string]string
	PinSHA256   string
	InsecureAck bool
	Revision    int64
	Broken      string
}

// Config 是采集器要的全部设置：本机来源开没开，与数据库里的各个来源（含停用的，列表上照样显示）。
type Config struct {
	LocalEnabled bool
	Sources      []SourceConfig
}

// PlainRule 是一种来源对 http:// 地址的规则（3.2.3）。
type PlainRule int

const (
	// PlainNever 只收 https://（TrueNAS：经 http 传过的 API Key 会被吊销）
	PlainNever PlainRule = iota
	// PlainWithoutCredentials 不带账号密码时 http:// 直接可用，带了要用户确认明文风险（Glances、群晖）
	PlainWithoutCredentials
	// PlainWithAck http:// 一律要用户确认明文风险（飞牛：明文 WebSocket 上中间人可以换掉登录用的公钥）
	PlainWithAck
)

// 表单里用户名一栏（Kind.Username）。
const (
	UsernameNone     = ""
	UsernameOptional = "optional"
	UsernameRequired = "required"
)

// TestInput 是一次「测试连接」：URL 已经规范化；Extra 是库里的长期凭据（只在地址、账号没变时给）；
// PinSHA256 是这一次要信任的证书指纹（空表示只按系统根证书校验）；OTP 是用户填的验证码。
type TestInput struct {
	URL       *url.URL
	Username  string
	Secret    string
	Extra     map[string]string
	OTP       string
	PinSHA256 string
}

// TestResult 是测试连上以后读到的主机信息；Extra 是登录产生的新的长期凭据（没有时为空）。
type TestResult struct {
	System System
	Extra  map[string]string
}

// Kind 是一种来源类型。Username 是表单里用户名一栏（UsernameNone、Optional、Required）；NeedsSecret 是填了这个用户名时
// 要不要密钥（为空表示总要）；Plain 是 http:// 地址的规则；ParseURL 在通用规则之后再按这种来源检查地址（可为空）。
type Kind struct {
	Name         string
	Label        string
	Experimental bool
	Username     string
	NeedsSecret  func(username string) bool
	Plain        PlainRule
	ParseURL     func(u *url.URL) error
	New          func(cfg SourceConfig) (Source, error)
	Test         func(ctx context.Context, in TestInput) (*TestResult, error)
}

// RequiresSecret 是填了 username 时这种来源要不要密钥。
func (k Kind) RequiresSecret(username string) bool {
	if k.NeedsSecret == nil {
		return true
	}
	return k.NeedsSecret(username)
}

var (
	kindsMu sync.RWMutex
	kinds   = map[string]Kind{}
)

// RegisterKind 登记一种来源类型（各来源在自己文件的 init 里登记），返回撤销登记的函数（测试用）。
// 名字是 local 或已经登记过时 panic：这是写代码时的错误。
func RegisterKind(k Kind) (unregister func()) {
	if k.Name == "" || k.Name == LocalID || k.New == nil || k.Test == nil {
		panic(fmt.Sprintf("nas: 来源类型 %q 不完整", k.Name))
	}
	kindsMu.Lock()
	defer kindsMu.Unlock()
	if _, dup := kinds[k.Name]; dup {
		panic(fmt.Sprintf("nas: 来源类型 %q 登记了两次", k.Name))
	}
	kinds[k.Name] = k
	return func() {
		kindsMu.Lock()
		defer kindsMu.Unlock()
		delete(kinds, k.Name)
	}
}

// LookupKind 是登记过的一种来源类型。
func LookupKind(name string) (Kind, bool) {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	k, ok := kinds[name]
	return k, ok
}

// Kinds 是登记过的全部来源类型，按名字排。
func Kinds() []Kind {
	kindsMu.RLock()
	defer kindsMu.RUnlock()
	out := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// errURLShape 是地址里多了东西：各来源的路径由 pt-tools 拼，用户只填协议、主机和端口。
var errURLShape = errors.New("地址只要协议、主机和端口，例如 https://192.168.1.10")

// ParseURL 规范化一种来源的地址（保存与测试共用）：去掉首尾空白与末尾的 /，只收 http(s)；拒绝用户名密码、查询参数、#
// 与路径；主机名转成小写。只收 https:// 的来源拒绝 http://。传输上的要求（明文带凭据要确认）见 CheckTransport。
func ParseURL(kindName, raw string) (*url.URL, error) {
	k, ok := LookupKind(kindName)
	if !ok {
		return nil, fmt.Errorf("不支持的来源类型 %q", kindName)
	}
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		return nil, errors.New("要填地址")
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("地址要以 https:// 或 http:// 开头，例如 https://192.168.1.10")
	}
	if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		strings.Contains(s, "#") || (u.Path != "" && u.Path != "/") {
		return nil, errURLShape
	}
	if u.Hostname() == "" {
		return nil, errors.New("地址里没有主机名")
	}
	u.Path, u.RawPath = "", ""
	u.Host = strings.ToLower(u.Host)
	if u.Scheme == "http" && k.Plain == PlainNever {
		return nil, fmt.Errorf("%s 只能用 https:// 地址", k.Label)
	}
	if k.ParseURL != nil {
		if err := k.ParseURL(u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

// ErrPlainNeedsAck 是要以明文（http://）发出凭据、而用户没有确认风险。
var ErrPlainNeedsAck = errors.New("要用 http:// 连接并发送账号密码，先勾选「我知道账号密码会以明文经过局域网，可能被截获」，或者改用 https://")

// CheckTransport 按这一次实际要发出的凭据校验传输（3.2.3）：https:// 总可以；http:// 按这种来源的规则，
// sendsCredentials 是这次会不会发出账号、密码或令牌，ack 是用户这一次勾没勾明文风险。
func CheckTransport(k Kind, u *url.URL, sendsCredentials, ack bool) error {
	if u.Scheme == "https" {
		return nil
	}
	switch k.Plain {
	case PlainWithoutCredentials:
		if !sendsCredentials || ack {
			return nil
		}
		return ErrPlainNeedsAck
	case PlainWithAck:
		if ack {
			return nil
		}
		return ErrPlainNeedsAck
	default:
		return fmt.Errorf("%s 只能用 https:// 地址", k.Label)
	}
}
