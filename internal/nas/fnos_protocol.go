package nas

// 飞牛 fnOS 网页用的接口（非官方：飞牛没有公开这个接口的文档）。请求与回应照 pyfnos（Timandes/pyfnos，Apache-2.0，
// Home Assistant 的飞牛集成用它）记录的格式写，没有搬它的代码；协议的细节都在这个文件里，飞牛改了协议时只改这里
// （和 fnos_fake_test.go 里的假服务）。
//
// 一条 WebSocket（/websocket?type=main）上收发 JSON 文本：
//   - 每个请求带 reqid，回应带同一个 reqid；回应的 result 是 succ 或 fail，fail 时 errno 是错误码。
//   - 第一个请求 util.crypto.getRSAPub 取 RSA 公钥（pub）与这条连接的会话号（si）。
//   - 登录（user.login）与两步验证（user.2fa.loginVerify）加密发：随机的 AES-256 密钥与 IV，AES-CBC（PKCS#7 填充）
//     加密请求，RSA（PKCS#1 v1.5）加密 AES 密钥，发 {"req":"encrypted","iv","rsa","aes"}。登录成功回 token、longToken
//     与 secret（签名密钥，用这次请求的 AES 密钥与 IV 加密）。
//   - 登录以后的请求都签名：发 base64(HMAC-SHA256(签名密钥, 请求的 JSON)) 紧跟着请求的 JSON。
//   - 重连时用 token 发 user.authToken（用存下的签名密钥签名）；token 过期（errno 135168）时用 longToken 发
//     user.tokenLogin 换新的 token。
//   - {"req":"ping"} 回 {"res":"pong"}：pyfnos 每 30 秒发一次。

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/sunerpy/pt-tools/internal/redact"
)

const (
	// fnosPath 是飞牛网页用的 WebSocket（type=main 是主连接；另有 timer、file 两种，这里不用）
	fnosPath  = "/websocket"
	fnosQuery = "type=main"
	// fnosReadLimit 是一条回应最大多大（容器多的时候容器列表最大）
	fnosReadLimit = 4 << 20
	// fnosPingEvery 是心跳的间隔（同 pyfnos）
	fnosPingEvery = 30 * time.Second
	// fnosDeviceName、fnosDeviceType 是登录时报给飞牛的设备：飞牛的登录设备列表里显示它
	fnosDeviceName = "pt-tools"
	fnosDeviceType = "Browser"
	// fnosErrNotAdmin 是账号不是管理员（pyfnos 的 README：存储的接口要管理员，别的账号回 4352）
	fnosErrNotAdmin = 4352
	// fnosMinRSABits 是认的最短 RSA 公钥
	fnosMinRSABits = 1024
)

var (
	errFnosNotFnOS = errors.New("回的不是飞牛 fnOS 的数据（地址是不是指到了别的网页？飞牛默认端口是 5666（http）、5667（https））")
	errFnosClosed  = errors.New("和飞牛的连接关了")
	errFnosNoLogin = errors.New("还没有登录飞牛")
	// fnosOTPRe 是验证码的格式：飞牛的两步验证是 6 位数字（同 pyfnos 的校验）
	fnosOTPRe = regexp.MustCompile(`^\d{6}$`)
)

// fnosError 是飞牛回的失败（result 为 fail）：errno 是错误码，msg 是它给的原因（已经去掉了凭据）。
type fnosError struct {
	req   string
	errno int
	msg   string
}

func (e *fnosError) Error() string {
	reason := e.msg
	switch {
	case reason == "":
		reason = fmt.Sprintf("错误码 %d", e.errno)
	case e.errno != 0:
		reason = fmt.Sprintf("%s（错误码 %d）", reason, e.errno)
	}
	return fmt.Sprintf("飞牛的 %s 回了错误：%s", e.req, reason)
}

// fnosDecodeError 是回应和预期的样子对不上。
type fnosDecodeError struct {
	req string
	err error
}

func (e *fnosDecodeError) Error() string {
	return fmt.Sprintf("飞牛的 %s 回应看不懂：%v", e.req, e.err)
}

func (e *fnosDecodeError) Unwrap() error { return e.err }

// fnosTransport 报告 err 是不是连接本身的问题（断了、超时、连不上）：飞牛回了失败或回应看不懂都不算。
func fnosTransport(err error) bool {
	if err == nil {
		return false
	}
	var fe *fnosError
	var de *fnosDecodeError
	var ae *AuthError
	return !errors.As(err, &fe) && !errors.As(err, &de) && !errors.As(err, &ae)
}

// fnosErrno 是 err 里飞牛的错误码（不是飞牛回的失败时为 0）。
func fnosErrno(err error) int {
	var fe *fnosError
	if errors.As(err, &fe) {
		return fe.errno
	}
	return 0
}

// fnosEnvelope 是每个回应外层的几个字段：没有 result 的回应（pyfnos 说 stor.listDisk 有时不带）只要 errno 为 0 也算成功。
type fnosEnvelope struct {
	Result string  `json:"result"`
	Errno  synoNum `json:"errno"`
	Msg    string  `json:"msg"`
	ErrMsg string  `json:"errmsg"`
}

func (e fnosEnvelope) errno() int {
	v, ok := e.Errno.float()
	if !ok {
		return 0
	}
	return int(v)
}

func (e fnosEnvelope) failed() bool {
	if strings.EqualFold(e.Result, "fail") {
		return true
	}
	return e.Result == "" && e.errno() != 0
}

// cleanFnosMsg 是飞牛给的原因：只要第一行，去掉 URL 里的凭据与 secrets（密码、token），最长 160 个字。
func cleanFnosMsg(msg string, secrets ...string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = redact.Secrets(msg)
	for _, s := range secrets {
		if len(s) >= 4 {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	msg = strings.TrimSpace(msg)
	if r := []rune(msg); len(r) > 160 {
		msg = string(r[:160]) + "…"
	}
	return msg
}

// fnosMsg 是读循环交给一个请求的回应（连接断了时是 err）。
type fnosMsg struct {
	raw []byte
	err error
}

// fnosConn 是到一台飞牛的一条 WebSocket 连接：请求按 reqid 对上回应；一个读循环收全部回应，一个心跳循环每隔一阵发 ping。
type fnosConn struct {
	ws  *websocket.Conn
	pub *rsa.PublicKey
	si  string
	// key 是签名密钥（登录以后才有）；secrets 是要从飞牛的原因里去掉的（密码、token）
	key     []byte
	secrets []string
	cancel  context.CancelFunc
	done    chan struct{}

	mu      sync.Mutex
	pending map[string]chan fnosMsg
	dead    error
}

// dialFnOS 连上 u（http(s)://主机:端口）的 /websocket?type=main，取 RSA 公钥与会话号。证书按 Transport 校验：不认时在握手
// 阶段就断开，返回 *CertError，不会发出任何请求。不走代理、不跟随重定向。ping 为 0 时不发心跳（测试连接用）。
func dialFnOS(ctx context.Context, u *url.URL, pin string, roots *x509.CertPool, ping time.Duration) (*fnosConn, error) {
	target := url.URL{Scheme: "wss", Host: u.Host, Path: fnosPath, RawQuery: fnosQuery}
	if u.Scheme == "http" {
		target.Scheme = "ws"
	}
	client := &http.Client{
		Transport: Transport(u, pin, roots),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ws, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{HTTPClient: client})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fnosDialError(ctx, err, resp)
	}
	ws.SetReadLimit(fnosReadLimit)
	rctx, cancel := context.WithCancel(context.Background())
	c := &fnosConn{ws: ws, cancel: cancel, done: make(chan struct{}), pending: map[string]chan fnosMsg{}}
	go c.readLoop(rctx)
	if ping > 0 {
		go c.heartbeat(rctx, ping)
	}
	if err := c.hello(ctx); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

// fnosDialError 是 WebSocket 没连上的原因。
func fnosDialError(ctx context.Context, err error, resp *http.Response) error {
	var ce *CertError
	if errors.As(err, &ce) {
		return ce
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		switch code := resp.StatusCode; {
		case code >= 300 && code < 400:
			return fnosRedirect(resp)
		case code == http.StatusNotFound || code == http.StatusOK:
			return fmt.Errorf("这个地址没有飞牛的接口（%s 回了 HTTP %d）：地址或端口对吗？飞牛默认端口是 5666（http）、5667（https）", fnosPath, code)
		default:
			return fmt.Errorf("飞牛没有接受 WebSocket 连接（HTTP %d）", code)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	// 拒绝连接、找不到主机这类：只留网络层那一段，不带 coder/websocket 套的几层前缀
	var op *net.OpError
	if errors.As(err, &op) {
		return fmt.Errorf("连不上飞牛：%w", op)
	}
	return fmt.Errorf("连不上飞牛：%w", err)
}

// fnosRedirect 是握手回了重定向：转到 https:// 多半是飞牛开了「强制 HTTPS」；转到别处说明这个地址不是飞牛的接口。
// 原因里只写目标的协议、主机与路径（不带查询参数）。
func fnosRedirect(resp *http.Response) error {
	loc := resp.Header.Get("Location")
	if t, err := url.Parse(loc); err == nil {
		t.User, t.RawQuery, t.ForceQuery, t.Fragment, t.RawFragment = nil, "", false, "", ""
		loc = t.String()
	}
	if l := strings.ToLower(loc); strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "wss://") {
		return fmt.Errorf("飞牛回了重定向（HTTP %d，到 %s）：飞牛开了「强制 HTTPS」，地址改成 https://（默认端口 5667）", resp.StatusCode, loc)
	}
	return fmt.Errorf("这个地址回了重定向（HTTP %d，到 %s），不像飞牛的接口：地址或端口对吗？飞牛默认端口是 5666（http）、5667（https）", resp.StatusCode, loc)
}

// hello 是第一个请求：取登录用的 RSA 公钥与这条连接的会话号（si）。回的不是这两样时这个地址不是飞牛。
func (c *fnosConn) hello(ctx context.Context) error {
	raw, err := c.plain(ctx, "util.crypto.getRSAPub")
	if err != nil {
		var fe *fnosError
		var de *fnosDecodeError
		if errors.As(err, &fe) || errors.As(err, &de) {
			return errFnosNotFnOS
		}
		return err
	}
	var r struct {
		Pub string `json:"pub"`
		SI  string `json:"si"`
	}
	if json.Unmarshal(raw, &r) != nil || strings.TrimSpace(r.Pub) == "" || strings.TrimSpace(r.SI) == "" {
		return errFnosNotFnOS
	}
	pub, err := parseFnosPub(r.Pub)
	if err != nil {
		return fmt.Errorf("飞牛给的登录公钥用不了：%w", err)
	}
	c.pub, c.si = pub, r.SI
	return nil
}

// parseFnosPub 解开飞牛给的 RSA 公钥：PEM（PUBLIC KEY 或 RSA PUBLIC KEY），或者没有 PEM 头的 base64。
func parseFnosPub(s string) (*rsa.PublicKey, error) {
	var der []byte
	if block, _ := pem.Decode([]byte(s)); block != nil {
		der = block.Bytes
	} else {
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
		if err != nil {
			return nil, errors.New("不是 PEM 也不是 base64")
		}
		der = b
	}
	var key *rsa.PublicKey
	if k, err := x509.ParsePKIXPublicKey(der); err == nil {
		rk, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("不是 RSA 公钥")
		}
		key = rk
	} else if rk, err := x509.ParsePKCS1PublicKey(der); err == nil {
		key = rk
	} else {
		return nil, errors.New("解不开这个公钥")
	}
	if key.N.BitLen() < fnosMinRSABits {
		return nil, fmt.Errorf("公钥只有 %d 位", key.N.BitLen())
	}
	return key, nil
}

func (c *fnosConn) readLoop(ctx context.Context) {
	defer close(c.done)
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			c.fail(fmt.Errorf("和飞牛的连接断了：%w", err))
			return
		}
		var head struct {
			ReqID string `json:"reqid"`
		}
		// pong、飞牛自己推的通知与看不懂的消息不管
		if json.Unmarshal(data, &head) != nil || head.ReqID == "" {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[head.ReqID]
		delete(c.pending, head.ReqID)
		c.mu.Unlock()
		if ok {
			ch <- fnosMsg{raw: data}
		}
	}
}

// heartbeat 每隔 every 发一次 ping（飞牛回 pong），连接空着时也不被飞牛断开。写不出去时连接不能再用。
func (c *fnosConn) heartbeat(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			wctx, cancel := context.WithTimeout(ctx, every)
			err := c.ws.Write(wctx, websocket.MessageText, []byte(`{"req":"ping"}`))
			cancel()
			if err != nil {
				c.fail(fmt.Errorf("和飞牛的连接断了：%w", err))
				return
			}
		}
	}
}

// fail 让连接不能再用：在等的请求都拿到 err。
func (c *fnosConn) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead == nil {
		c.dead = err
	}
	for id, ch := range c.pending {
		ch <- fnosMsg{err: c.dead}
		delete(c.pending, id)
	}
}

// broken 报告连接是不是已经断了。
func (c *fnosConn) broken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dead != nil
}

// close 断开连接，等读循环退出（最多 2 秒）。
func (c *fnosConn) close() {
	c.fail(errFnosClosed)
	_ = c.ws.CloseNow()
	c.cancel()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
}

func (c *fnosConn) forget(reqid string) {
	c.mu.Lock()
	delete(c.pending, reqid)
	c.mu.Unlock()
}

// newFnosReqID 是请求号：毫秒时间戳加 12 位随机的十六进制（同 pyfnos：25 个字，飞牛的前端不超过 28 个字）。
func newFnosReqID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return strconv.FormatInt(time.Now().UnixMilli(), 10) + hex.EncodeToString(b)
}

// newFnosDID 是设备号：同 pyfnos 的格式（时间戳、两段随机数，base32、小写、去掉填充）。存进长期凭据，以后都用它：
// 飞牛据此认出受信任的设备。
func newFnosDID() string {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	part := func() string {
		b := make([]byte, 9)
		_, _ = rand.Read(b)
		return enc.EncodeToString(b)
	}
	t := enc.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixMilli(), 10)))
	return strings.ToLower(t + "-" + part() + "-" + part())
}

// roundTrip 发出一帧，等 reqid 的回应或 ctx 结束；飞牛回了失败时是 *fnosError，回应不是 JSON 时是 *fnosDecodeError，
// 其余是连接的问题（写不出去时连接随之不能再用）。
func (c *fnosConn) roundTrip(ctx context.Context, reqid, req string, frame []byte) ([]byte, error) {
	ch := make(chan fnosMsg, 1)
	c.mu.Lock()
	if c.dead != nil {
		err := c.dead
		c.mu.Unlock()
		return nil, err
	}
	c.pending[reqid] = ch
	c.mu.Unlock()
	if err := c.ws.Write(ctx, websocket.MessageText, frame); err != nil {
		c.forget(reqid)
		// 写失败时 coder/websocket 已经关了连接
		c.fail(fmt.Errorf("和飞牛的连接断了：%w", err))
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	var m fnosMsg
	select {
	case m = <-ch:
	case <-ctx.Done():
		c.forget(reqid)
		return nil, ctx.Err()
	}
	if m.err != nil {
		return nil, m.err
	}
	var env fnosEnvelope
	if err := json.Unmarshal(m.raw, &env); err != nil {
		return nil, &fnosDecodeError{req: req, err: err}
	}
	if env.failed() {
		msg := env.Msg
		if msg == "" {
			msg = env.ErrMsg
		}
		return nil, &fnosError{req: req, errno: env.errno(), msg: cleanFnosMsg(msg, c.secrets...)}
	}
	return m.raw, nil
}

// plain 发一个不签名的请求（登录以前的 util.crypto.getRSAPub）。
func (c *fnosConn) plain(ctx context.Context, req string) ([]byte, error) {
	reqid := newFnosReqID()
	frame, err := json.Marshal(map[string]string{"reqid": reqid, "req": req})
	if err != nil {
		return nil, err
	}
	return c.roundTrip(ctx, reqid, req, frame)
}

// call 发一个签名的请求，把回应整个解进 out（为空时不解）：回应的数据有的在 data 里，有的在最外层，由 out 的样子决定。
func (c *fnosConn) call(ctx context.Context, req string, params map[string]any, out any) error {
	if len(c.key) == 0 {
		return errFnosNoLogin
	}
	reqid := newFnosReqID()
	payload := make(map[string]any, len(params)+2)
	for k, v := range params {
		payload[k] = v
	}
	payload["req"], payload["reqid"] = req, reqid
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("飞牛的 %s 的参数写不成 JSON：%w", req, err)
	}
	raw, err := c.roundTrip(ctx, reqid, req, append([]byte(fnosSign(c.key, body)), body...))
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &fnosDecodeError{req: req, err: err}
	}
	return nil
}

// fnosSign 是一个请求的签名：base64(HMAC-SHA256(签名密钥, 请求的 JSON))。
func fnosSign(key, body []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// fnosEncrypt 是 AES-256-CBC 加密（PKCS#7 填充）。
func fnosEncrypt(key, iv, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	buf := append(bytes.Clone(plain), bytes.Repeat([]byte{byte(pad)}, pad)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	return buf, nil
}

// fnosDecrypt 是 fnosEncrypt 的反过来：长度不是整块或填充不对时返回错误。
func fnosDecrypt(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("长度不对")
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	pad := int(out[len(out)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(out[len(out)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, errors.New("填充不对")
	}
	return out[:len(out)-pad], nil
}

// encrypted 加密发一个登录阶段的请求（payload 里再加上 reqid、req 与 si），返回回应与这次用的 AES 密钥、IV
// （回应里的 secret 用它们解开）。
func (c *fnosConn) encrypted(ctx context.Context, req string, payload map[string]any) (raw, key, iv []byte, err error) {
	reqid := newFnosReqID()
	payload["reqid"], payload["req"], payload["si"] = reqid, req, c.si
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, nil, err
	}
	key, iv = make([]byte, 32), make([]byte, aes.BlockSize)
	if _, err = rand.Read(key); err != nil {
		return nil, nil, nil, err
	}
	if _, err = rand.Read(iv); err != nil {
		return nil, nil, nil, err
	}
	data, err := fnosEncrypt(key, iv, plain)
	if err != nil {
		return nil, nil, nil, err
	}
	//nolint:staticcheck // SA1019：飞牛的登录只认 RSA PKCS#1 v1.5 包的密钥（它的网页前端也这样发）
	wrapped, err := rsa.EncryptPKCS1v15(rand.Reader, c.pub, key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("用飞牛给的公钥加密失败：%w", err)
	}
	frame, err := json.Marshal(map[string]string{
		"req": "encrypted",
		"iv":  base64.StdEncoding.EncodeToString(iv),
		"rsa": base64.StdEncoding.EncodeToString(wrapped),
		"aes": base64.StdEncoding.EncodeToString(data),
	})
	if err != nil {
		return nil, nil, nil, err
	}
	raw, err = c.roundTrip(ctx, reqid, req, frame)
	return raw, key, iv, err
}

// fnosSession 是登录得到的长期凭据：Secret 是 base64 的签名密钥。
type fnosSession struct {
	Token     string
	LongToken string
	Secret    string
}

// fnosLoginReply 是 user.login 与 user.2fa.loginVerify 的回应：成功时有 token 与 secret；开了两步验证的账号先回
// accessToken 与两步验证的情况（pyfnos 的 specs/twofa-login）。
type fnosLoginReply struct {
	Token             string `json:"token"`
	LongToken         string `json:"longToken"`
	Secret            string `json:"secret"`
	AccessToken       string `json:"accessToken"`
	IsBindTwofaSecret *bool  `json:"isBindTwofaSecret"`
	IsTwofaEnforced   *bool  `json:"isTwofaEnforced"`
	IsTrustedDevice   *bool  `json:"isTrustedDevice"`
}

func (r fnosLoginReply) final() bool { return r.Token != "" && r.Secret != "" }

// challenge 是账号绑定了验证器、这台设备不受信任：要验证码。
func (r fnosLoginReply) challenge() bool {
	return r.AccessToken != "" && r.IsBindTwofaSecret != nil && *r.IsBindTwofaSecret &&
		(r.IsTrustedDevice == nil || !*r.IsTrustedDevice)
}

// setup 是飞牛要求这个账号开两步验证、它还没绑定验证器。
func (r fnosLoginReply) setup() bool {
	return r.AccessToken != "" && r.IsTwofaEnforced != nil && *r.IsTwofaEnforced &&
		(r.IsBindTwofaSecret == nil || !*r.IsBindTwofaSecret)
}

// login 用账号密码登录这条连接（otp 不为空时用它完成两步验证，并让飞牛信任这台设备）。test 为真是「测试连接」：
// 要验证码时让用户填；采集时要验证码说明 token 与受信任的设备都失效了，停下来等用户重新测。
func (c *fnosConn) login(ctx context.Context, user, pass, otp, did string, test bool) (*fnosSession, error) {
	c.secrets = append(c.secrets, pass)
	raw, key, iv, err := c.encrypted(ctx, "user.login", map[string]any{
		"user": user, "password": pass, "stay": true,
		"deviceType": fnosDeviceType, "deviceName": fnosDeviceName, "did": did,
	})
	if err != nil {
		return nil, fnosLoginError(err)
	}
	var r fnosLoginReply
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &fnosDecodeError{req: "user.login", err: err}
	}
	switch {
	case r.final():
		return c.finish("user.login", r, key, iv)
	case r.challenge():
		return c.verify(ctx, r.AccessToken, otp, did, test)
	case r.setup():
		return nil, &AuthError{Reason: "飞牛要求这个账号开两步验证，它还没有绑定验证器：先用这个账号登录一次飞牛网页，按提示绑定好再测试"}
	}
	return nil, &fnosDecodeError{req: "user.login", err: errors.New("回应里没有 token 与 secret，也不是两步验证")}
}

// verify 用验证码完成两步验证（user.2fa.loginVerify），并让飞牛信任这台设备：以后用同一个设备号登录不用再填。
func (c *fnosConn) verify(ctx context.Context, access, otp, did string, test bool) (*fnosSession, error) {
	if otp == "" {
		if !test {
			return nil, &AuthError{Reason: "飞牛要验证码（token 与受信任的设备都失效了，或者账号刚开了两步验证），到「NAS 监控」里编辑这个来源、填验证码重新测一次", OTP: true}
		}
		return nil, &AuthError{Reason: "这个账号开了两步验证，填一次验证码；以后 pt-tools 用登录得到的 token 连接，不用再填", OTP: true}
	}
	if !fnosOTPRe.MatchString(otp) {
		return nil, &AuthError{Reason: "验证码是 6 位数字", OTP: true}
	}
	c.secrets = append(c.secrets, otp)
	raw, key, iv, err := c.encrypted(ctx, "user.2fa.loginVerify", map[string]any{
		"code": otp, "isTrustedDevice": true, "accessToken": access, "stay": 1,
		"deviceName": fnosDeviceName, "deviceType": fnosDeviceType, "did": did,
	})
	if err != nil {
		if fnosTransport(err) {
			return nil, err
		}
		return nil, &AuthError{Reason: "验证码不对或过期了，重新输入", OTP: true}
	}
	var r fnosLoginReply
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, &fnosDecodeError{req: "user.2fa.loginVerify", err: err}
	}
	if !r.final() {
		return nil, &AuthError{Reason: "飞牛没有接受这个验证码，重新输入", OTP: true}
	}
	return c.finish("user.2fa.loginVerify", r, key, iv)
}

// finish 解开回应里的签名密钥，记下这条连接登录成功了。
func (c *fnosConn) finish(req string, r fnosLoginReply, key, iv []byte) (*fnosSession, error) {
	data, err := base64.StdEncoding.DecodeString(r.Secret)
	if err != nil {
		return nil, &fnosDecodeError{req: req, err: errors.New("secret 不是 base64")}
	}
	secret, err := fnosDecrypt(key, iv, data)
	if err != nil || len(secret) == 0 {
		return nil, &fnosDecodeError{req: req, err: errors.New("secret 解不开")}
	}
	c.key = secret
	c.secrets = append(c.secrets, r.Token, r.LongToken)
	return &fnosSession{Token: r.Token, LongToken: r.LongToken, Secret: base64.StdEncoding.EncodeToString(secret)}, nil
}

// fnosLoginError 是登录请求本身失败了（用户名或密码不对、账号停用…）：飞牛的错误码没有公开的列表，原因写飞牛给的话。
// 都是认证失败，采集器停下来，不再用同一组账号反复试（飞牛按登录失败的次数锁账号）。
func fnosLoginError(err error) error {
	var fe *fnosError
	if !errors.As(err, &fe) {
		return err
	}
	if fe.msg != "" {
		return &AuthError{Reason: fmt.Sprintf("飞牛说：%s（错误码 %d）", fe.msg, fe.errno)}
	}
	return &AuthError{Reason: fmt.Sprintf("飞牛登录失败（错误码 %d）", fe.errno)}
}

// resume 用存下的 token 与签名密钥登录这条连接（user.authToken）；token 不认时（过期是错误码 135168，pyfnos 只在这时换；
// 这里别的错误码也试一次）用 longToken 换一个新的（user.tokenLogin），
// 换到了时改 s 并返回 true。都不行时返回飞牛的错误（调用方改用密码登录）；连接的问题原样返回。
func (c *fnosConn) resume(ctx context.Context, s *fnosSession, did string) (bool, error) {
	key, err := base64.StdEncoding.DecodeString(s.Secret)
	if err != nil || len(key) == 0 || s.Token == "" {
		return false, &AuthError{Reason: "存下的 token 不完整"}
	}
	c.key = key
	c.secrets = append(c.secrets, s.Token, s.LongToken)
	err = c.call(ctx, "user.authToken", map[string]any{"main": true, "token": s.Token, "si": c.si}, nil)
	if err == nil || fnosTransport(err) || s.LongToken == "" {
		return false, err
	}
	var r struct {
		Token     string `json:"token"`
		LongToken string `json:"longToken"`
	}
	err = c.call(ctx, "user.tokenLogin", map[string]any{
		"deviceType": fnosDeviceType, "deviceName": fnosDeviceName, "did": did, "si": c.si, "token": s.LongToken,
	}, &r)
	if err != nil {
		return false, err
	}
	if r.Token == "" {
		return false, &fnosDecodeError{req: "user.tokenLogin", err: errors.New("回应里没有新的 token")}
	}
	s.Token = r.Token
	if r.LongToken != "" {
		s.LongToken = r.LongToken
	}
	c.secrets = append(c.secrets, s.Token, s.LongToken)
	return true, nil
}
