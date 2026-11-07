package nas

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
)

// KindFnOS 是飞牛 fnOS 来源（实验性：只用假服务测过，没在真机上验证）：飞牛网页用的 WebSocket 接口（飞牛没有公开它的
// 文档），用账号密码登录，开了两步验证的账号第一次测试时填验证码。登录得到的 token 与签名密钥加密存下，之后用它们连接，
// token 过期时用 longToken 换新的，都不行时才再用密码登录。
//
// 协议在 fnos_protocol.go，读哪些数据、怎么映射在 fnos_data.go。
const KindFnOS = "fnos"

func init() {
	RegisterKind(Kind{
		Name:         KindFnOS,
		Label:        "飞牛",
		Experimental: true,
		Username:     UsernameRequired,
		Plain:        PlainWithAck,
		ParseURL:     fnosParseURL,
		New: func(cfg SourceConfig) (Source, error) {
			s, err := newFnOS(cfg, nil)
			if err != nil {
				return nil, err
			}
			return s, nil
		},
		Test: func(ctx context.Context, in TestInput) (*TestResult, error) { return testFnOS(ctx, in, nil) },
	})
}

const (
	// fnosHTTPPort 是飞牛网页的 http 端口：http:// 地址没写端口时补上（https:// 多半是反向代理或改过的端口，不补）
	fnosHTTPPort = "5666"
	// 长期凭据（Extra）里的键：token、longToken、签名密钥（base64）与设备号
	fnosTokenKey     = "token"
	fnosLongTokenKey = "long_token"
	fnosSecretKey    = "secret"
	fnosDIDKey       = "did"
)

var errFnosNoAccount = errors.New("要填飞牛的用户名和密码")

// fnosNotAdmin 是账号读不到主机信息时的原因（飞牛回 4352）。
const fnosNotAdmin = "这个账号读不到飞牛的主机信息：要用管理员账号，见「添加来源」里飞牛的准备步骤"

// fnosParseURL：http:// 地址没写端口时补上飞牛的 http 端口 5666。
func fnosParseURL(u *url.URL) error {
	if u.Scheme == "http" && u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), fnosHTTPPort)
	}
	return nil
}

// fnosSessionOf 是库里存的登录凭据；token 或签名密钥缺一样时为空（用密码登录）。
func fnosSessionOf(extra map[string]string) *fnosSession {
	s := &fnosSession{Token: extra[fnosTokenKey], LongToken: extra[fnosLongTokenKey], Secret: extra[fnosSecretKey]}
	if s.Token == "" || s.Secret == "" {
		return nil
	}
	return s
}

// fnosExtra 是要存进库里的长期凭据。
func fnosExtra(s *fnosSession, did string) map[string]string {
	x := map[string]string{fnosTokenKey: s.Token, fnosSecretKey: s.Secret, fnosDIDKey: did}
	if s.LongToken != "" {
		x[fnosLongTokenKey] = s.LongToken
	}
	return x
}

// fnos 是一个飞牛来源：一条 WebSocket 连接在采集之间复用（有心跳），断了时重新连接、用存下的 token 登录。
type fnos struct {
	base  *url.URL
	user  string
	pass  string
	pin   string
	roots *x509.CertPool

	mu   sync.Mutex
	conn *fnosConn
	sess *fnosSession
	// did 是设备号（存进长期凭据，以后都用它）；didNew 为真是这一次才生成的、还没存下
	did    string
	didNew bool
	// fresh 是登录或换 token 得到的新凭据，等 TakeExtra 交出
	fresh map[string]string
}

func newFnOS(cfg SourceConfig, roots *x509.CertPool) (*fnos, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("地址不对：%w", err)
	}
	if strings.TrimSpace(cfg.Username) == "" || cfg.Secret == "" {
		return nil, errFnosNoAccount
	}
	if u.Scheme != "https" && !cfg.InsecureAck {
		return nil, ErrPlainNeedsAck
	}
	s := &fnos{
		base: u, user: cfg.Username, pass: cfg.Secret, pin: cfg.PinSHA256, roots: roots,
		sess: fnosSessionOf(cfg.Extra), did: cfg.Extra[fnosDIDKey],
	}
	if s.did == "" {
		s.did, s.didNew = newFnosDID(), true
	}
	return s, nil
}

// testFnOS 是「测试连接」：连上、取公钥（认出是不是飞牛），用账号密码登录（填了验证码时完成两步验证，并让飞牛信任这台设备），
// 读主机信息。登录得到的 token、签名密钥与设备号放进 TestResult.Extra，保存时存下，采集器之后用它们连接。
// 地址、账号没变时沿用库里的设备号：飞牛认得这台受信任的设备，不用再填验证码。
func testFnOS(ctx context.Context, in TestInput, roots *x509.CertPool) (*TestResult, error) {
	if in.URL == nil {
		return nil, errors.New("要填地址")
	}
	if strings.TrimSpace(in.Username) == "" || in.Secret == "" {
		return nil, errFnosNoAccount
	}
	did := in.Extra[fnosDIDKey]
	if did == "" {
		did = newFnosDID()
	}
	c, err := dialFnOS(ctx, in.URL, in.PinSHA256, roots, 0)
	if err != nil {
		return nil, err
	}
	defer c.close()
	sess, err := c.login(ctx, in.Username, in.Secret, strings.TrimSpace(in.OTP), did, true)
	if err != nil {
		return nil, err
	}
	sys, err := fnosTestSystem(ctx, c)
	if err != nil {
		return nil, err
	}
	return &TestResult{System: sys, Extra: fnosExtra(sess, did)}, nil
}

// TakeExtra 交出登录或换 token 得到的新凭据（上一次调用以后没有新的时为 nil）。
func (s *fnos) TakeExtra() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.fresh
	s.fresh = nil
	return x
}

// Close 断开连接。不注销 token：注销以后存下的 token 就不能再用，下次又要用密码登录。
func (s *fnos) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drop()
	return nil
}

func (s *fnos) drop() {
	if s.conn != nil {
		s.conn.close()
		s.conn = nil
	}
}

// publish 记下要存的凭据（s.sess 刚登录或换过）。
func (s *fnos) publish() {
	s.fresh, s.didNew = fnosExtra(s.sess, s.did), false
}

// ensure 保证有一条登录了的连接：没有或已经断了时重新连接并登录。
func (s *fnos) ensure(ctx context.Context) error {
	if s.conn != nil && !s.conn.broken() {
		return nil
	}
	s.drop()
	c, err := dialFnOS(ctx, s.base, s.pin, s.roots, fnosPingEvery)
	if err != nil {
		return err
	}
	if err := s.auth(ctx, c); err != nil {
		c.close()
		return err
	}
	s.conn = c
	return nil
}

// auth 登录新连上的 c：先用存下的 token（过期时用 longToken 换新的）；飞牛不认时忘掉它们、改用密码登录。
// 密码登录失败是 *AuthError：采集器停下来，不再用同一组账号反复试（飞牛按登录失败的次数锁账号）。
func (s *fnos) auth(ctx context.Context, c *fnosConn) error {
	if s.sess != nil {
		refreshed, err := c.resume(ctx, s.sess, s.did)
		if err == nil {
			if refreshed || s.didNew {
				s.publish()
			}
			return nil
		}
		if fnosTransport(err) {
			return err
		}
		// token 不能再用了：这一次在同一条连接上改用密码登录；登录没成时下一次也直接用密码
		s.sess = nil
	}
	sess, err := c.login(ctx, s.user, s.pass, "", s.did, false)
	if err != nil {
		return err
	}
	s.sess = sess
	s.publish()
	return nil
}

// Collect 采集一次：每次读 CPU、内存、网络与运行时长；完整采集再读主机信息、存储空间、硬盘与容器。
// 连不上、证书与认证失败时整次失败；某个请求被飞牛拒绝（比如账号不是管理员）时只记在那一类上。
func (s *fnos) Collect(ctx context.Context, full bool) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	snap, err := collectFnOS(ctx, s.conn, full)
	if err != nil {
		// 连接断了、超时或请求全被拒绝：这条连接不再用，下一次重新连接、登录
		s.drop()
		return nil, err
	}
	return snap, nil
}
