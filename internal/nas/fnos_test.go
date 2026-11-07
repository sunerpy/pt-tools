package nas

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fnosCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// fnosSaved 测一次连接（同保存前的「测试连接」），返回要存进库里的凭据。
func fnosSaved(t *testing.T, f *fakeFnOS) map[string]string {
	t.Helper()
	res, err := testFnOS(fnosCtx(t), f.testInput(), f.roots())
	require.NoError(t, err)
	return res.Extra
}

func fnosAuthErr(t *testing.T, err error) *AuthError {
	t.Helper()
	var ae *AuthError
	require.ErrorAs(t, err, &ae)
	return ae
}

// 签名与解开 secret 用 pyfnos 测试里的两组数核对（tests/test_client.py 的 test_iz_function_calibration 与
// test_decrypt_login_secret）。
func TestFnosVectors(t *testing.T) {
	key, err := base64.StdEncoding.DecodeString("J3sfIMwxGV+SxHpaQFiZbw==")
	require.NoError(t, err)
	assert.Equal(t, "tktNU6xPp/h/RNu3xtIAXg5m0YFVM17nZvT/x6uc2ek=",
		fnosSign(key, []byte(`{"reqid":"68f6d99868f6d996000000020004","req":"user.info"}`)))

	aesKey, err := base64.StdEncoding.DecodeString("OWlTcnVncnNxdDNRcWV0TjNaNWJRajJ3U3loOGRvS1g=")
	require.NoError(t, err)
	iv, err := base64.StdEncoding.DecodeString("Spsb/LIwxCGz5aEbR5lbBQ==")
	require.NoError(t, err)
	data, err := base64.StdEncoding.DecodeString("AEslF1tKUjAMjeQNx+dffr2BwCn9oYjzYgWn9FZysyA=")
	require.NoError(t, err)
	secret, err := fnosDecrypt(aesKey, iv, data)
	require.NoError(t, err)
	assert.Equal(t, "WyYXbAbR4gnL3DgXdiwJbw==", base64.StdEncoding.EncodeToString(secret))

	// 用 finish 解同一份：签名密钥存成 base64，连接记下解出的字节
	c := &fnosConn{}
	sess, err := c.finish("user.login", fnosLoginReply{Token: "t", LongToken: "l", Secret: "AEslF1tKUjAMjeQNx+dffr2BwCn9oYjzYgWn9FZysyA="}, aesKey, iv)
	require.NoError(t, err)
	assert.Equal(t, "WyYXbAbR4gnL3DgXdiwJbw==", sess.Secret)
	assert.Equal(t, secret, c.key)
	assert.Subset(t, c.secrets, []string{"t", "l"}, "登录得到的 token 也从飞牛给的原因里去掉")
	_, err = c.finish("user.login", fnosLoginReply{Token: "t", Secret: "不是 base64"}, aesKey, iv)
	assert.ErrorContains(t, err, "secret 不是 base64")
	_, err = c.finish("user.login", fnosLoginReply{Token: "t", Secret: "AAAA"}, aesKey, iv)
	assert.ErrorContains(t, err, "secret 解不开")
}

func TestFnosCrypt(t *testing.T) {
	key, iv := make([]byte, 32), make([]byte, 16)
	_, _ = rand.Read(key)
	_, _ = rand.Read(iv)
	for _, plain := range [][]byte{{}, []byte("x"), []byte("0123456789abcdef"), []byte(`{"req":"user.login","password":"pw"}`)} {
		out, err := fnosEncrypt(key, iv, plain)
		require.NoError(t, err)
		assert.Zero(t, len(out)%16)
		back, err := fnosDecrypt(key, iv, out)
		require.NoError(t, err)
		assert.Equal(t, plain, back)
	}
	_, err := fnosDecrypt(key, iv, nil)
	assert.ErrorContains(t, err, "长度不对")
	_, err = fnosDecrypt(key, iv, make([]byte, 15))
	assert.ErrorContains(t, err, "长度不对")
	// 解出来的最后一个字节不是合法的填充
	bad, err := fnosEncrypt(key, iv, []byte("0123456789abcde"))
	require.NoError(t, err)
	bad[len(bad)-1] ^= 0xff
	_, err = fnosDecrypt(key, iv, bad)
	assert.ErrorContains(t, err, "填充不对")
	// 最后一个字节像填充（2），前一个却不是
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	raw := []byte("0123456789abcdX\x02")
	enc := make([]byte, len(raw))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(enc, raw)
	_, err = fnosDecrypt(key, iv, enc)
	assert.ErrorContains(t, err, "填充不对")
	_, err = fnosEncrypt([]byte("short"), iv, []byte("x"))
	assert.Error(t, err)
	_, err = fnosDecrypt([]byte("short"), iv, make([]byte, 16))
	assert.Error(t, err)
}

// 请求号与设备号的格式同 pyfnos：请求号是 13 位毫秒时间戳加 12 位十六进制；设备号是三段小写的 base32。
func TestFnosIDs(t *testing.T) {
	a, b := newFnosReqID(), newFnosReqID()
	assert.Regexp(t, `^\d{13}[0-9a-f]{12}$`, a)
	assert.NotEqual(t, a, b)
	did := newFnosDID()
	assert.Regexp(t, `^[a-z2-7]+-[a-z2-7]{15}-[a-z2-7]{15}$`, did)
	assert.NotEqual(t, did, newFnosDID())
}

func TestParseFnosPub(t *testing.T) {
	rsaKey := fakeFnosKey()
	pkix, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	require.NoError(t, err)
	pkcs1 := x509.MarshalPKCS1PublicKey(&rsaKey.PublicKey)
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecDER, err := x509.MarshalPKIXPublicKey(&ec.PublicKey)
	require.NoError(t, err)
	// Go 不再生成短于 1024 位的 RSA 密钥：手写一个 512 位的模数，只用来测长度检查
	smallN := new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 511), big.NewInt(1))
	smallDER, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: smallN, E: 65537})
	require.NoError(t, err)
	b64 := base64.StdEncoding.EncodeToString(pkix)

	for name, in := range map[string]string{
		"PEM PUBLIC KEY":     string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix})),
		"PEM RSA PUBLIC KEY": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: pkcs1})),
		"没有 PEM 头的 base64":   b64,
		"base64 里有换行":        b64[:40] + "\n" + b64[40:],
	} {
		k, err := parseFnosPub(in)
		require.NoError(t, err, name)
		assert.Equal(t, rsaKey.N, k.N, name)
	}
	for name, c := range map[string]struct{ in, want string }{
		"不是 base64": {"%%%", "不是 PEM 也不是 base64"},
		"不是公钥":      {base64.StdEncoding.EncodeToString([]byte("hello")), "解不开这个公钥"},
		"不是 RSA":    {string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: ecDER})), "不是 RSA 公钥"},
		"太短":        {string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: smallDER})), "公钥只有 512 位"},
	} {
		_, err := parseFnosPub(c.in)
		assert.ErrorContains(t, err, c.want, name)
	}
}

func TestCleanFnosMsg(t *testing.T) {
	assert.Equal(t, "第一行", cleanFnosMsg("第一行\n第二行"))
	assert.Equal(t, "密码 *** 不对", cleanFnosMsg("密码 hunter22 不对", "hunter22", "ab"))
	assert.Equal(t, "短的 ab 不去掉", cleanFnosMsg("短的 ab 不去掉", "ab"))
	assert.NotContains(t, cleanFnosMsg("连 https://u:pw@nas:5667/x 失败"), "pw@")
	long := cleanFnosMsg(strings.Repeat("长", 200))
	assert.Equal(t, 161, len([]rune(long)))
	assert.True(t, strings.HasSuffix(long, "…"))
}

func TestFnosEnvelopeAndErrors(t *testing.T) {
	env := func(s string) fnosEnvelope {
		var e fnosEnvelope
		require.NoError(t, json.Unmarshal([]byte(s), &e))
		return e
	}
	assert.False(t, env(`{"result":"succ"}`).failed())
	assert.True(t, env(`{"result":"fail","errno":4352}`).failed())
	assert.True(t, env(`{"result":"FAIL"}`).failed())
	assert.False(t, env(`{"disk":[]}`).failed(), "没有 result、errno 为 0 也算成功（pyfnos 说 stor.listDisk 有时这样）")
	assert.True(t, env(`{"errno":"131073"}`).failed(), "没有 result 时 errno 不为 0 是失败；错误码写成字符串也认")
	assert.Equal(t, 131073, env(`{"errno":"131073"}`).errno())
	assert.Equal(t, 0, env(`{"errno":null}`).errno())

	assert.Equal(t, "飞牛的 stor.general 回了错误：没有权限（错误码 4352）", (&fnosError{req: "stor.general", errno: 4352, msg: "没有权限"}).Error())
	assert.Equal(t, "飞牛的 stor.general 回了错误：错误码 4352", (&fnosError{req: "stor.general", errno: 4352}).Error())
	assert.Equal(t, "飞牛的 x 回了错误：坏了", (&fnosError{req: "x", msg: "坏了"}).Error())
	de := &fnosDecodeError{req: "x", err: errors.New("类型不对")}
	assert.Equal(t, "飞牛的 x 回应看不懂：类型不对", de.Error())
	assert.ErrorContains(t, errors.Unwrap(de), "类型不对")

	assert.False(t, fnosTransport(nil))
	assert.False(t, fnosTransport(&fnosError{}))
	assert.False(t, fnosTransport(de))
	assert.False(t, fnosTransport(&AuthError{}))
	assert.True(t, fnosTransport(context.DeadlineExceeded))
	assert.Equal(t, 4352, fnosErrno(errors.Join(errors.New("x"), &fnosError{errno: 4352})))
	assert.Equal(t, 0, fnosErrno(errors.New("x")))
}

func TestFnOSKind(t *testing.T) {
	k, ok := LookupKind(KindFnOS)
	require.True(t, ok)
	assert.Equal(t, "飞牛", k.Label)
	assert.True(t, k.Experimental)
	assert.Equal(t, UsernameRequired, k.Username)
	assert.True(t, k.RequiresSecret("ptmon"))
	assert.Equal(t, PlainWithAck, k.Plain)

	u, err := ParseURL(KindFnOS, " http://192.168.1.10/ ")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.10:5666", u.String(), "http:// 没写端口时补 5666")
	u, err = ParseURL(KindFnOS, "http://[fd00::10]")
	require.NoError(t, err)
	assert.Equal(t, "http://[fd00::10]:5666", u.String())
	u, err = ParseURL(KindFnOS, "https://NAS.lan")
	require.NoError(t, err)
	assert.Equal(t, "https://nas.lan", u.String(), "https:// 不补端口（多半是反向代理）")
	u, err = ParseURL(KindFnOS, "http://192.168.1.10:8000")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.10:8000", u.String())

	// http:// 一律要确认明文风险：明文的 WebSocket 上中间人可以换掉登录用的公钥
	plain, _ := url.Parse("http://192.168.1.10:5666")
	assert.ErrorIs(t, CheckTransport(k, plain, false, false), ErrPlainNeedsAck)
	assert.ErrorIs(t, CheckTransport(k, plain, true, false), ErrPlainNeedsAck)
	assert.NoError(t, CheckTransport(k, plain, true, true))
	tls, _ := url.Parse("https://192.168.1.10:5667")
	assert.NoError(t, CheckTransport(k, tls, true, false))
}

func TestNewFnOS(t *testing.T) {
	_, err := newFnOS(SourceConfig{URL: "https://nas:5667", Secret: "pw"}, nil)
	assert.ErrorIs(t, err, errFnosNoAccount)
	_, err = newFnOS(SourceConfig{URL: "https://nas:5667", Username: "u"}, nil)
	assert.ErrorIs(t, err, errFnosNoAccount)
	_, err = newFnOS(SourceConfig{URL: "http://nas:5666", Username: "u", Secret: "pw"}, nil)
	assert.ErrorIs(t, err, ErrPlainNeedsAck)
	_, err = newFnOS(SourceConfig{URL: "http://[::1", Username: "u", Secret: "pw"}, nil)
	assert.ErrorContains(t, err, "地址不对")

	src, err := fnosKindT(t).New(SourceConfig{URL: "http://nas:5666", Username: "u", Secret: "pw", InsecureAck: true})
	require.NoError(t, err)
	s := src.(*fnos)
	assert.Nil(t, s.sess, "没有存下的凭据时用密码登录")
	assert.True(t, s.didNew)
	assert.Regexp(t, `^[a-z2-7]+-`, s.did)
	assert.NoError(t, s.Close())
	assert.Nil(t, s.TakeExtra())
	_, err = fnosKindT(t).New(SourceConfig{URL: "http://nas:5666", Username: "u"})
	assert.Error(t, err)

	s, err = newFnOS(SourceConfig{URL: "https://nas:5667", Username: "u", Secret: "pw", Extra: map[string]string{
		fnosTokenKey: "t", fnosSecretKey: "c2VjcmV0", fnosDIDKey: "did-1",
	}}, nil)
	require.NoError(t, err)
	assert.Equal(t, &fnosSession{Token: "t", Secret: "c2VjcmV0"}, s.sess)
	assert.Equal(t, "did-1", s.did)
	assert.False(t, s.didNew)
	assert.Nil(t, fnosSessionOf(map[string]string{fnosTokenKey: "t"}), "没有签名密钥时不算")
	assert.Equal(t, map[string]string{fnosTokenKey: "t", fnosSecretKey: "s", fnosDIDKey: "d"}, fnosExtra(&fnosSession{Token: "t", Secret: "s"}, "d"))
}

func fnosKindT(t *testing.T) Kind {
	t.Helper()
	kind, ok := LookupKind(KindFnOS)
	require.True(t, ok)
	return kind
}

// 测试连接：取公钥、加密登录（密码不明文出现在任何一帧里）、读主机信息；存下 token、longToken、签名密钥与设备号
func TestFnOSTestConnection(t *testing.T) {
	for _, tls := range []bool{false, true} {
		f := newFakeFnOS(t, tls)
		res, err := fnosKindT(t).Test(context.Background(), f.testInput())
		if tls {
			// 没有信任这张自签证书：在握手阶段就断开，不发出任何请求
			require.ErrorIs(t, err, ErrCertUntrusted)
			assert.Empty(t, f.requests())
			res, err = testFnOS(fnosCtx(t), f.testInput(), f.roots())
		}
		require.NoError(t, err)
		assert.Equal(t, System{
			Hostname: "fnos-home", Platform: "fnOS", Version: "0.9.27", Model: "Intel(R) N100", Cores: 4, UptimeSeconds: 1268777,
		}, res.System)
		assert.Equal(t, []string{
			"util.crypto.getRSAPub", "user.login", fnosReqHostName, fnosReqCPU, fnosReqUptime,
		}, f.requests())
		tok := f.token(res.Extra[fnosTokenKey])
		require.NotNil(t, tok)
		assert.Equal(t, tok.long, res.Extra[fnosLongTokenKey])
		assert.Equal(t, base64.StdEncoding.EncodeToString(tok.secret), res.Extra[fnosSecretKey])
		assert.Regexp(t, `^[a-z2-7]+-[a-z2-7]{15}-[a-z2-7]{15}$`, res.Extra[fnosDIDKey])
		login := f.lastLogin()
		assert.Equal(t, res.Extra[fnosDIDKey], login["did"])
		assert.Equal(t, "pt-tools", login["deviceName"])
		for _, fr := range f.allFrames() {
			assert.NotContains(t, fr, f.pass, "密码只在加密的帧里")
		}

		// 地址、账号没变时沿用库里的设备号
		in := f.testInput()
		in.Extra = res.Extra
		again, err := testFnOS(fnosCtx(t), in, f.roots())
		require.NoError(t, err)
		assert.Equal(t, res.Extra[fnosDIDKey], again.Extra[fnosDIDKey])
		assert.NotEqual(t, res.Extra[fnosTokenKey], again.Extra[fnosTokenKey])
	}

	_, err := testFnOS(context.Background(), TestInput{}, nil)
	assert.ErrorContains(t, err, "要填地址")
	u, _ := url.Parse("http://nas:5666")
	_, err = testFnOS(context.Background(), TestInput{URL: u, Username: " ", Secret: "pw"}, nil)
	assert.ErrorIs(t, err, errFnosNoAccount)
}

// 钉住证书：指纹对得上时连，对不上时在握手阶段断开
func TestFnOSPinnedCert(t *testing.T) {
	f := newFakeFnOS(t, true)
	in := f.testInput()
	in.PinSHA256 = Fingerprint(f.ts.Certificate())
	_, err := testFnOS(fnosCtx(t), in, nil)
	require.NoError(t, err)
	in.PinSHA256 = strings.Repeat("ab", 32)
	_, err = testFnOS(fnosCtx(t), in, nil)
	assert.ErrorIs(t, err, ErrCertChanged)
	assert.EqualError(t, err, ErrCertChanged.Error(), "证书的错误原样返回，不套「连不上飞牛」")
}

// 登录回应的三种：拿到了凭据、要验证码（绑定了验证器、这台设备不受信任）、要先绑定验证器
func TestFnosLoginReplyKinds(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		name                    string
		r                       fnosLoginReply
		final, challenge, setup bool
	}{
		{"有 token 与 secret", fnosLoginReply{Token: "t", Secret: "s"}, true, false, false},
		{"只有 token", fnosLoginReply{Token: "t"}, false, false, false},
		{"要验证码", fnosLoginReply{AccessToken: "a", IsBindTwofaSecret: &yes}, false, true, false},
		{"要验证码（明说设备不受信任）", fnosLoginReply{AccessToken: "a", IsBindTwofaSecret: &yes, IsTrustedDevice: &no}, false, true, false},
		{"受信任的设备不要验证码", fnosLoginReply{AccessToken: "a", IsBindTwofaSecret: &yes, IsTrustedDevice: &yes}, false, false, false},
		{"要先绑定验证器", fnosLoginReply{AccessToken: "a", IsTwofaEnforced: &yes}, false, false, true},
		{"已经绑定了不算要先绑定", fnosLoginReply{AccessToken: "a", IsTwofaEnforced: &yes, IsBindTwofaSecret: &yes}, false, true, false},
		{"没有 accessToken", fnosLoginReply{IsBindTwofaSecret: &yes, IsTwofaEnforced: &yes}, false, false, false},
	} {
		assert.Equal(t, c.final, c.r.final(), c.name)
		assert.Equal(t, c.challenge, c.r.challenge(), c.name)
		assert.Equal(t, c.setup, c.r.setup(), c.name)
	}
}

// 开了两步验证：先要验证码；格式不对时不发；填错了重填；填对了让飞牛信任这台设备，之后用同一个设备号登录不用再填
func TestFnOSTestTwoFactor(t *testing.T) {
	f := newFakeFnOS(t, false)
	f.with(func() { f.twofa = true })
	_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
	ae := fnosAuthErr(t, err)
	assert.True(t, ae.OTP)
	assert.Contains(t, ae.Reason, "填一次验证码")
	did := f.lastLogin()["did"].(string)

	in := f.testInput()
	in.Extra = map[string]string{fnosDIDKey: did}
	in.OTP = "12345"
	_, err = testFnOS(fnosCtx(t), in, nil)
	ae = fnosAuthErr(t, err)
	assert.True(t, ae.OTP)
	assert.Contains(t, ae.Reason, "6 位数字")
	assert.Zero(t, f.count("user.2fa.loginVerify"), "格式不对时不发")

	in.OTP = "135790"
	_, err = testFnOS(fnosCtx(t), in, nil)
	ae = fnosAuthErr(t, err)
	assert.True(t, ae.OTP)
	assert.Contains(t, ae.Reason, "验证码不对或过期了")

	in.OTP = " " + f.otp + " "
	res, err := testFnOS(fnosCtx(t), in, nil)
	require.NoError(t, err)
	assert.Equal(t, did, res.Extra[fnosDIDKey])
	verify := f.lastLogin()
	assert.Equal(t, "user.2fa.loginVerify", verify["req"])
	assert.Equal(t, true, verify["isTrustedDevice"])
	assert.Equal(t, did, verify["did"])
	for _, fr := range f.allFrames() {
		assert.NotContains(t, fr, f.otp, "验证码也只在加密的帧里")
	}

	in.OTP = ""
	_, err = testFnOS(fnosCtx(t), in, nil)
	require.NoError(t, err, "受信任的设备不用再填验证码")
	assert.Equal(t, 2, f.count("user.2fa.loginVerify"))

	// 飞牛回了成功、却没给 token：当作没接受
	f.with(func() { f.verify = &fnosFakeReply{body: `{"accessToken":"x"}`} })
	f.with(func() { f.trusted = map[string]bool{} })
	in.OTP = f.otp
	_, err = testFnOS(fnosCtx(t), in, nil)
	ae = fnosAuthErr(t, err)
	assert.Contains(t, ae.Reason, "没有接受这个验证码")
	f.with(func() { f.verify = &fnosFakeReply{body: `{"token":5}`} })
	_, err = testFnOS(fnosCtx(t), in, nil)
	var de *fnosDecodeError
	assert.ErrorAs(t, err, &de)
	// 发验证码时连接断了：照原样报，不说验证码不对
	f.with(func() { f.verify = &fnosFakeReply{close: true} })
	_, err = testFnOS(fnosCtx(t), in, nil)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	var ae2 *AuthError
	assert.False(t, errors.As(err, &ae2))
}

func TestFnOSTestSetupRequired(t *testing.T) {
	f := newFakeFnOS(t, false)
	f.with(func() { f.enforced = true })
	_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
	ae := fnosAuthErr(t, err)
	assert.False(t, ae.OTP)
	assert.Contains(t, ae.Reason, "还没有绑定验证器")
}

// 登录失败写飞牛给的原因（去掉了密码），不重试
func TestFnOSTestLoginFailed(t *testing.T) {
	f := newFakeFnOS(t, false)
	in := f.testInput()
	in.Secret = "wrong-pass-1"
	_, err := testFnOS(fnosCtx(t), in, nil)
	ae := fnosAuthErr(t, err)
	assert.Equal(t, "飞牛说：用户名或密码错误（ptmon / ***）（错误码 131073）", ae.Reason)
	assert.NotContains(t, err.Error(), "wrong-pass-1")
	assert.Equal(t, 1, f.count("user.login"))

	f.with(func() { f.login = &fnosFakeReply{errno: 131077} })
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.Equal(t, "飞牛登录失败（错误码 131077）", fnosAuthErr(t, err).Reason)

	// 登录回了成功、却既没有 token 也不是两步验证
	f.with(func() { f.login = &fnosFakeReply{body: `{"uid":1000}`} })
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.ErrorContains(t, err, "回应里没有 token 与 secret")
	f.with(func() { f.login = &fnosFakeReply{body: `{"token":["x"]}`} })
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.ErrorContains(t, err, "飞牛的 user.login 回应看不懂")
}

// 读主机信息回 4352：账号不是管理员，不让保存；别的错误照原样
func TestFnOSTestHostErrors(t *testing.T) {
	f := newFakeFnOS(t, false)
	f.set(fnosReqHostName, fnosFakeReply{errno: fnosErrNotAdmin, msg: "没有权限"})
	_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.Equal(t, fnosNotAdmin, fnosAuthErr(t, err).Reason)

	f.set(fnosReqHostName, fnosFakeReply{errno: 9, msg: "坏了"})
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.EqualError(t, err, "飞牛的 appcgi.sysinfo.getHostName 回了错误：坏了（错误码 9）")

	// CPU 与运行时长读不到时只缺这几项
	f.set(fnosReqHostName, fnosFakeReply{body: fnosHostBody})
	f.set(fnosReqCPU, fnosFakeReply{errno: 9})
	f.set(fnosReqUptime, fnosFakeReply{errno: 9})
	res, err := testFnOS(fnosCtx(t), f.testInput(), nil)
	require.NoError(t, err)
	assert.Equal(t, System{Hostname: "fnos-home", Platform: "fnOS", Version: "0.9.27"}, res.System)

	// 读到一半连接断了
	f.set(fnosReqCPU, fnosFakeReply{close: true})
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	f.set(fnosReqCPU, fnosFakeReply{body: fnosCPUBody})
	f.set(fnosReqUptime, fnosFakeReply{close: true})
	_, err = testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
}

// 不是飞牛的地址：不发账号密码，写清楚原因
func TestFnOSNotFnOS(t *testing.T) {
	f := newFakeFnOS(t, false)
	cases := []struct {
		status   int
		location string
		want     string
	}{
		{http.StatusNotFound, "", "这个地址没有飞牛的接口（/websocket 回了 HTTP 404）"},
		{http.StatusOK, "", "这个地址没有飞牛的接口（/websocket 回了 HTTP 200）"},
		{http.StatusInternalServerError, "", "飞牛没有接受 WebSocket 连接（HTTP 500）"},
		{http.StatusFound, "https://u:pw@nas.lan:5667/websocket?type=main&token=x#y", "飞牛回了重定向（HTTP 302，到 https://nas.lan:5667/websocket）：飞牛开了「强制 HTTPS」"},
		{http.StatusMovedPermanently, "http://other.lan/login?next=x", "这个地址回了重定向（HTTP 301，到 http://other.lan/login），不像飞牛的接口"},
	}
	for _, c := range cases {
		f.with(func() { f.status, f.location = c.status, c.location })
		_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
		assert.ErrorContains(t, err, c.want, c.status)
		assert.NotContains(t, err.Error(), "pw@")
		assert.NotContains(t, err.Error(), "token=x")
	}
	f.with(func() { f.status, f.location = 0, "" })

	for name, c := range map[string]struct {
		hello fnosFakeReply
		want  string
	}{
		"取公钥失败":  {fnosFakeReply{errno: 404}, errFnosNotFnOS.Error()},
		"回应看不懂":  {fnosFakeReply{body: `{"result":5}`}, errFnosNotFnOS.Error()},
		"没有公钥":   {fnosFakeReply{body: `{"si":"x"}`}, errFnosNotFnOS.Error()},
		"没有会话号":  {fnosFakeReply{body: `{"pub":"x"}`}, errFnosNotFnOS.Error()},
		"公钥用不了":  {fnosFakeReply{body: `{"pub":"%%%","si":"x"}`}, "飞牛给的登录公钥用不了"},
		"字段类型不对": {fnosFakeReply{body: `{"pub":1,"si":"x"}`}, errFnosNotFnOS.Error()},
	} {
		f.with(func() { f.hello = &c.hello })
		_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
		assert.ErrorContains(t, err, c.want, name)
	}
	assert.Zero(t, f.count("user.login"), "认不出是飞牛时不登录")
	f.with(func() { f.hello = &fnosFakeReply{close: true} })
	_, err := testFnOS(fnosCtx(t), f.testInput(), nil)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	f.with(func() { f.hello = nil })

	// https:// 地址指到了 http 的端口：TLS 握手不成，写清楚连不上
	in := f.testInput()
	in.URL.Scheme = "https"
	_, err = testFnOS(fnosCtx(t), in, nil)
	assert.ErrorContains(t, err, "连不上飞牛：")

	// 端口上没有服务
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	u, _ := url.Parse(closed.URL)
	_, err = testFnOS(fnosCtx(t), TestInput{URL: u, Username: "u", Secret: "pw"}, nil)
	assert.ErrorContains(t, err, "连不上飞牛：")

	// 测试的时限到了
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = testFnOS(ctx, f.testInput(), nil)
	assert.ErrorIs(t, err, context.Canceled)
}

// 完整采集：用存下的 token 登录（不再发密码），读全部数据并映射
func TestFnOSCollectFull(t *testing.T) {
	f := newFakeFnOS(t, false)
	extra := fnosSaved(t, f)
	f.resetRequests()
	s := f.source(extra)
	snap, err := s.Collect(fnosCtx(t), true)
	require.NoError(t, err)
	assert.Nil(t, s.TakeExtra(), "token 照旧能用，没有新的凭据")
	_, logins, _ := f.counts()
	assert.Equal(t, 1, logins, "只有测试连接时登录过")
	assert.Equal(t, []string{
		"util.crypto.getRSAPub", "user.authToken",
		fnosReqCPU, fnosReqMem, fnosReqGen, fnosReqNet, fnosReqUptime,
		fnosReqHostName, fnosReqStorage, fnosReqStorState, fnosReqDisks, fnosReqDiskMon, fnosReqContainers,
	}, f.requests())

	assert.Equal(t, System{
		Hostname: "fnos-home", Platform: "fnOS", Version: "0.9.27", Model: "Intel(R) N100", Cores: 4, UptimeSeconds: 1268777,
	}, snap.System)
	assert.InDelta(t, 23, snap.CPU.Percent, 1e-9)
	require.NotNil(t, snap.CPU.IOWait)
	assert.InDelta(t, 2, *snap.CPU.IOWait, 1e-9)
	assert.InDelta(t, 1.25, snap.CPU.Load1, 1e-9)
	assert.InDelta(t, 0.9, snap.CPU.Load5, 1e-9)
	assert.InDelta(t, 0.7, snap.CPU.Load15, 1e-9)
	require.NotNil(t, snap.CPU.TempC)
	assert.InDelta(t, 47, *snap.CPU.TempC, 1e-9, "几个温度里最高的；0 不算")
	assert.Equal(t, Memory{Total: 16483282944, Used: 5368709120, Available: 11114573824}, snap.Memory)
	assert.Equal(t, Network{RxBps: 2621440, TxBps: 131072, Interfaces: []string{"enp1s0", "enp2s0"}}, snap.Network)

	healthy := true
	assert.Equal(t, []Pool{
		{Name: "/vol1", Status: "正常", Healthy: &healthy, Size: 3829364490240, Used: 1530000000000, Free: 2299364490240, Disks: 2},
		{Name: "/vol2", Status: "正常", Healthy: &healthy, Size: 480107374182, Used: 100000000000, Free: 380107374182, Disks: 1},
	}, snap.Pools)
	t36, t38, t31 := 36.0, 38.0, 31.0
	assert.Equal(t, []Disk{
		{Name: "sda", Model: "WDC WD40EFRX-68N32N0", Kind: DiskHDD, Size: 4000787030016, TempC: &t36, Pool: "/vol1"},
		{Name: "sdb", Model: "WDC WD40EFRX-68N32N0", Kind: DiskHDD, Size: 4000787030016, TempC: &t38, Pool: "/vol1"},
		{Name: "nvme0n1", Model: "Samsung SSD 980 500GB", Kind: DiskNVMe, Size: 500107862016, Pool: "/vol2"},
		{Name: "sdc", Model: "Samsung SSD 870 EVO 1TB", Kind: DiskSSD, Size: 1000204886016, TempC: &t31},
	}, snap.Disks)
	assert.Equal(t, Apps{Running: 2, Total: 3, UpdatesUnknown: true}, snap.Apps)
	assert.Empty(t, snap.Alerts)
	for _, key := range []string{FacetSystem, FacetCPU, FacetCPUTemp, FacetMemory, FacetNetwork, FacetPools, FacetDisks, FacetApps} {
		assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[key], key)
	}
	assert.Equal(t, Facet{Status: FacetUnsupported, Reason: fnosNoAlerts}, snap.Facets[FacetAlerts])
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "FAKESERIAL", "不读也不存序列号")

	// 请求的参数：存储状态按名字与 uuid 查、硬盘不含热备、容器含停了的、网速只要 netSpeed
	f.mu.Lock()
	params := f.params
	f.mu.Unlock()
	assert.Equal(t, []any{"dm-0", "dm-1"}, params[fnosReqStorState]["name"])
	assert.Equal(t, []any{"trim_1a2b-0", "trim_3c4d-0"}, params[fnosReqStorState]["uuid"])
	assert.Equal(t, true, params[fnosReqDisks]["noHotSpare"])
	assert.Equal(t, true, params[fnosReqContainers]["all"])
	assert.Equal(t, []any{"netSpeed"}, params[fnosReqGen]["item"])
}

// 轻量采集复用连接与登录，只读 CPU、内存、网络与运行时长
func TestFnOSCollectLight(t *testing.T) {
	f := newFakeFnOS(t, false)
	s := f.source(fnosSaved(t, f))
	_, err := s.Collect(fnosCtx(t), true)
	require.NoError(t, err)
	f.resetRequests()
	snap, err := s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.Equal(t, []string{fnosReqCPU, fnosReqMem, fnosReqGen, fnosReqNet, fnosReqUptime}, f.requests())
	upgrades, _, _ := f.counts()
	assert.Equal(t, 2, upgrades, "测试连接一条，采集一条（复用）")
	assert.Equal(t, "", snap.System.Hostname, "轻量采集不读主机名，合并时沿用完整采集的")
	assert.EqualValues(t, 1268777, snap.System.UptimeSeconds)
	assert.NotContains(t, snap.Facets, FacetPools)
	assert.NotContains(t, snap.Facets, FacetAlerts)
}

// token 过期：用 longToken 换一个新的（签名密钥不变），交给采集器存下；以后的重连用新的 token
func TestFnOSTokenExpired(t *testing.T) {
	f := newFakeFnOS(t, false)
	extra := fnosSaved(t, f)
	f.expire(extra[fnosTokenKey])
	s := f.source(extra)
	_, err := s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	fresh := s.TakeExtra()
	require.NotNil(t, fresh)
	assert.NotEqual(t, extra[fnosTokenKey], fresh[fnosTokenKey])
	assert.Equal(t, extra[fnosSecretKey], fresh[fnosSecretKey])
	assert.Equal(t, extra[fnosLongTokenKey], fresh[fnosLongTokenKey])
	assert.Equal(t, extra[fnosDIDKey], fresh[fnosDIDKey])
	assert.Nil(t, s.TakeExtra(), "交出一次以后清空")
	assert.Equal(t, 1, f.count("user.tokenLogin"))

	// 飞牛换 token 时也给了新的 longToken：一起存下
	f2 := newFakeFnOS(t, false)
	extra2 := fnosSaved(t, f2)
	f2.expire(extra2[fnosTokenKey])
	f2.with(func() { f2.rotate = true })
	s2 := f2.source(extra2)
	_, err = s2.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	rotated := s2.TakeExtra()
	require.NotNil(t, rotated)
	assert.NotEqual(t, extra2[fnosLongTokenKey], rotated[fnosLongTokenKey])
	assert.Equal(t, extra2[fnosSecretKey], rotated[fnosSecretKey])
	_, logins, _ := f.counts()
	assert.Equal(t, 1, logins, "不用再发密码")

	// 断开以后重连用新的 token
	f.dropConns()
	require.Eventually(t, func() bool { return s.conn == nil || s.conn.broken() }, 5*time.Second, 10*time.Millisecond)
	f.resetRequests()
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	f.mu.Lock()
	assert.Equal(t, fresh[fnosTokenKey], f.params["user.authToken"]["token"])
	f.mu.Unlock()
	assert.Zero(t, f.count("user.tokenLogin"))

	// 换的回应里没有新 token：当作换不成，同一条连接上改用密码登录（签名密钥跟着换）
	f.expire(fresh[fnosTokenKey])
	f.with(func() { f.refresh = &fnosFakeReply{body: `{"uid":1000}`} })
	f.dropConns()
	require.Eventually(t, s.conn.broken, 5*time.Second, 10*time.Millisecond)
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	again := s.TakeExtra()
	require.NotNil(t, again)
	assert.NotEqual(t, fresh[fnosSecretKey], again[fnosSecretKey])
	assert.Equal(t, fresh[fnosDIDKey], again[fnosDIDKey])
	_, logins, _ = f.counts()
	assert.Equal(t, 2, logins)
}

// token 与 longToken 都不认了（比如在飞牛上删了这台设备）：用密码登录一次，存下新的凭据
func TestFnOSTokenRevoked(t *testing.T) {
	f := newFakeFnOS(t, false)
	extra := fnosSaved(t, f)
	f.revoke(extra[fnosTokenKey])
	s := f.source(extra)
	snap, err := s.Collect(fnosCtx(t), true)
	require.NoError(t, err)
	assert.Equal(t, "fnos-home", snap.System.Hostname)
	fresh := s.TakeExtra()
	require.NotNil(t, fresh)
	assert.NotEqual(t, extra[fnosSecretKey], fresh[fnosSecretKey], "密码登录换了签名密钥")
	assert.Equal(t, extra[fnosDIDKey], fresh[fnosDIDKey])
	_, logins, _ := f.counts()
	assert.Equal(t, 2, logins)

	// 没有 longToken 时 token 不认就直接用密码
	f2 := newFakeFnOS(t, false)
	extra2 := fnosSaved(t, f2)
	f2.revoke(extra2[fnosTokenKey])
	delete(extra2, fnosLongTokenKey)
	s2 := f2.source(extra2)
	_, err = s2.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.Zero(t, f2.count("user.tokenLogin"))
	assert.NotNil(t, s2.TakeExtra())
}

// 密码改了：token 也不能用时，登录失败停住（不反复试）；下一次直接用密码，不再试那个 token
func TestFnOSCollectAuthFailed(t *testing.T) {
	f := newFakeFnOS(t, false)
	extra := fnosSaved(t, f)
	s := f.source(extra)
	f.revoke(extra[fnosTokenKey])
	f.with(func() { f.pass = "changed-pass" })
	_, err := s.Collect(fnosCtx(t), false)
	ae := fnosAuthErr(t, err)
	assert.Contains(t, ae.Reason, "用户名或密码错误")
	assert.NotContains(t, ae.Reason, "s3cret-pass")
	assert.Nil(t, s.conn)
	assert.Nil(t, s.sess)
	assert.Nil(t, s.TakeExtra())
	f.resetRequests()
	_, err = s.Collect(fnosCtx(t), false)
	require.Error(t, err)
	assert.Zero(t, f.count("user.authToken"))
}

// 采集时飞牛要验证码（token 失效、设备不再受信任）：停住，写到哪里重新测
func TestFnOSCollectNeedsOTP(t *testing.T) {
	f := newFakeFnOS(t, false)
	extra := fnosSaved(t, f)
	f.revoke(extra[fnosTokenKey])
	f.with(func() { f.twofa = true })
	s := f.source(extra)
	_, err := s.Collect(fnosCtx(t), false)
	ae := fnosAuthErr(t, err)
	assert.True(t, ae.OTP)
	assert.Contains(t, ae.Reason, "到「NAS 监控」里编辑这个来源、填验证码重新测一次")
}

// 账号不是管理员：存储与容器记没有权限，别的照读
func TestFnOSCollectNotAdmin(t *testing.T) {
	f := newFakeFnOS(t, false)
	f.with(func() { f.admin = false })
	s := f.source(fnosSaved(t, f))
	snap, err := s.Collect(fnosCtx(t), true)
	require.NoError(t, err)
	for key, what := range map[string]string{FacetPools: "存储空间", FacetDisks: "硬盘", FacetApps: "容器"} {
		assert.Equal(t, Facet{Status: FacetDenied, Reason: "这个账号没有读" + what + "的权限：飞牛只让管理员读，见「添加来源」里飞牛的准备步骤"}, snap.Facets[key], key)
	}
	assert.Equal(t, Facet{Status: FacetOK}, snap.Facets[FacetCPU])
	assert.Empty(t, snap.Pools)
	assert.Empty(t, snap.Disks)
	assert.Zero(t, f.count(fnosReqStorState))
	assert.Zero(t, f.count(fnosReqDiskMon))
}

// 某个请求被拒绝或回应不对：只记在那一类上
func TestFnOSCollectPartial(t *testing.T) {
	cases := []struct {
		name  string
		set   map[string]fnosFakeReply
		check func(t *testing.T, snap *Snapshot)
	}{
		{"存储状态读不到", map[string]fnosFakeReply{fnosReqStorState: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			require.Len(t, snap.Pools, 2)
			assert.Empty(t, snap.Pools[0].Status)
			assert.Nil(t, snap.Pools[0].Healthy)
			assert.Zero(t, snap.Pools[0].Disks)
			assert.Empty(t, snap.Disks[0].Pool)
			assert.Equal(t, FacetOK, snap.Facets[FacetPools].Status)
		}},
		{"硬盘温度读不到", map[string]fnosFakeReply{fnosReqDiskMon: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			require.Len(t, snap.Disks, 4)
			assert.Nil(t, snap.Disks[0].TempC)
			assert.Equal(t, FacetOK, snap.Facets[FacetDisks].Status)
		}},
		{"网卡列表读不到", map[string]fnosFakeReply{fnosReqNet: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			assert.Empty(t, snap.Network.Interfaces)
			assert.EqualValues(t, 2621440, snap.Network.RxBps)
			assert.Equal(t, FacetOK, snap.Facets[FacetNetwork].Status)
		}},
		{"容器读不到", map[string]fnosFakeReply{fnosReqContainers: {errno: 7, msg: "Docker 没有启动"}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛的 appcgi.dockermgr.containerList 回了错误：Docker 没有启动（错误码 7）"}, snap.Facets[FacetApps])
			assert.Equal(t, Apps{}, snap.Apps)
		}},
		{"没给网速", map[string]fnosFakeReply{fnosReqGen: {body: `{"data":{"item":{}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给网速"}, snap.Facets[FacetNetwork])
			assert.Zero(t, snap.Network.RxBps)
			assert.Empty(t, snap.Network.Interfaces, "没有网速时不读网卡")
		}},
		{"网速读不到", map[string]fnosFakeReply{fnosReqGen: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, FacetError, snap.Facets[FacetNetwork].Status)
		}},
		{"没给 CPU 占用", map[string]fnosFakeReply{fnosReqCPU: {body: `{"data":{"cpu":{"name":"N100","temp":[0]}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给 CPU 占用"}, snap.Facets[FacetCPU])
			assert.Equal(t, FacetUnsupported, snap.Facets[FacetCPUTemp].Status)
			assert.Nil(t, snap.CPU.TempC)
			assert.Equal(t, "N100", snap.System.Model)
		}},
		{"没给 CPU", map[string]fnosFakeReply{fnosReqCPU: {body: `{"data":{}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给 CPU 的数据"}, snap.Facets[FacetCPU])
			assert.Equal(t, snap.Facets[FacetCPU], snap.Facets[FacetCPUTemp])
		}},
		{"CPU 的回应看不懂", map[string]fnosFakeReply{fnosReqCPU: {body: `{"data":{"cpu":"x"}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, FacetError, snap.Facets[FacetCPU].Status)
			assert.Contains(t, snap.Facets[FacetCPU].Reason, "飞牛的 appcgi.resmon.cpu 回应看不懂")
		}},
		{"CPU 的数写成字符串", map[string]fnosFakeReply{fnosReqCPU: {body: `{"data":{"cpu":{"core":"8","thread":"x","busy":{"all":"130"},"temp":["52.5",200]}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.InDelta(t, 100, snap.CPU.Percent, 1e-9, "封顶 100")
			assert.Nil(t, snap.CPU.IOWait)
			assert.Equal(t, 8, snap.System.Cores, "没有线程数时用核数")
			require.NotNil(t, snap.CPU.TempC)
			assert.InDelta(t, 52.5, *snap.CPU.TempC, 1e-9, "离谱的温度不算")
		}},
		{"没给内存", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给内存的数据"}, snap.Facets[FacetMemory])
		}},
		{"没给内存总量", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{"mem":{"available":1}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给内存总量"}, snap.Facets[FacetMemory])
		}},
		{"没给可用内存时用 used", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{"mem":{"total":1000,"used":"1500"}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Memory{Total: 1000, Used: 1000}, snap.Memory)
		}},
		{"没给内存用量", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{"mem":{"total":1000}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给内存用量"}, snap.Facets[FacetMemory])
		}},
		{"内存读不到", map[string]fnosFakeReply{fnosReqMem: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, FacetError, snap.Facets[FacetMemory].Status)
		}},
		{"没给存储空间的列表", map[string]fnosFakeReply{fnosReqStorage: {body: `{"block":[]}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给存储空间的列表"}, snap.Facets[FacetPools])
			require.Len(t, snap.Disks, 4, "硬盘照读")
			assert.Empty(t, snap.Disks[0].Pool)
		}},
		{"没有存储空间", map[string]fnosFakeReply{fnosReqStorage: {body: `{"array":[]}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Empty(t, snap.Pools)
			assert.Equal(t, FacetOK, snap.Facets[FacetPools].Status)
		}},
		{"没给硬盘的列表", map[string]fnosFakeReply{fnosReqDisks: {body: `{}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给硬盘的列表"}, snap.Facets[FacetDisks])
			assert.Empty(t, snap.Disks)
		}},
		{"硬盘列表没带 result", map[string]fnosFakeReply{fnosReqDisks: {body: fnosDisksBody, noResult: true}}, func(t *testing.T, snap *Snapshot) {
			assert.Len(t, snap.Disks, 4)
			assert.Equal(t, FacetOK, snap.Facets[FacetDisks].Status)
		}},
		{"没有名字的硬盘不算", map[string]fnosFakeReply{fnosReqDisks: {body: `{"disk":[{"name":" "},{"name":"sde","type":"usb"}]}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, []Disk{{Name: "sde", Kind: DiskUnknown}}, snap.Disks)
		}},
		{"没给容器的列表", map[string]fnosFakeReply{fnosReqContainers: {body: `{}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给容器的列表"}, snap.Facets[FacetApps])
		}},
		{"没有容器", map[string]fnosFakeReply{fnosReqContainers: {body: `{"rsp":[]}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Apps{UpdatesUnknown: true}, snap.Apps)
			assert.Equal(t, FacetOK, snap.Facets[FacetApps].Status)
		}},
		{"主机信息读不到", map[string]fnosFakeReply{fnosReqHostName: {errno: 9}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, FacetError, snap.Facets[FacetSystem].Status)
			assert.Equal(t, "fnOS", snap.System.Platform)
			assert.Empty(t, snap.System.Hostname)
		}},
		{"运行时长读不到或不对", map[string]fnosFakeReply{fnosReqUptime: {body: `{"data":{"uptime":-5}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Zero(t, snap.System.UptimeSeconds)
		}},
		{"线程数优先于核数，负的负载算 0", map[string]fnosFakeReply{fnosReqCPU: {body: `{"data":{"cpu":{"core":4,"thread":8,"busy":{"all":5},
			"loadavg":{"avg1min":-1,"avg5min":"x","avg15min":0.5}}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, 8, snap.System.Cores)
			assert.Zero(t, snap.CPU.Load1)
			assert.Zero(t, snap.CPU.Load5)
			assert.InDelta(t, 0.5, snap.CPU.Load15, 1e-9)
		}},
		{"可用内存比总量大", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{"mem":{"total":1000,"available":1500}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Memory{Total: 1000, Used: 0, Available: 1000}, snap.Memory)
		}},
		{"内存总量是 0", map[string]fnosFakeReply{fnosReqMem: {body: `{"data":{"mem":{"total":0,"available":0}}}`}}, func(t *testing.T, snap *Snapshot) {
			assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛没给内存总量"}, snap.Facets[FacetMemory])
		}},
		{"空闲比总量大", map[string]fnosFakeReply{fnosReqStorage: {body: `{"array":[{"name":"dm-0","uuid":"trim_1a2b-0","mountpoint":"/vol1",
			"frsize":200,"fssize":100}]}`}}, func(t *testing.T, snap *Snapshot) {
			require.Len(t, snap.Pools, 1)
			assert.EqualValues(t, 100, snap.Pools[0].Size)
			assert.EqualValues(t, 100, snap.Pools[0].Free)
			assert.Zero(t, snap.Pools[0].Used)
		}},
		// 扩容过的存储空间底下有几个阵列，用的是同几块盘上的不同分区
		{
			"一块盘的几个分区只算一块",
			map[string]fnosFakeReply{fnosReqStorState: {body: `{"state":[{"name":"dm-0","uuid":"trim_1a2b-0","md":[
			{"raidDisks":2,"arrayState":"clean","syncAction":"idle","disk":[{"name":"sda3","arrState":"in_sync"},{"name":"sdb3","arrState":"in_sync"}]},
			{"raidDisks":2,"arrayState":"clean","syncAction":"idle","disk":[{"name":"sda4","arrState":"in_sync"},{"name":"sdb4","arrState":"in_sync"}]}]}]}`}},
			func(t *testing.T, snap *Snapshot) {
				require.Len(t, snap.Pools, 2)
				assert.Equal(t, 2, snap.Pools[0].Disks)
				assert.Equal(t, "/vol1", snap.Disks[0].Pool)
			},
		},
		{
			"失败的原因写在 errmsg 里",
			map[string]fnosFakeReply{fnosReqContainers: {body: `{"result":"fail","errno":7,"errmsg":"Docker 没有启动"}`}},
			func(t *testing.T, snap *Snapshot) {
				assert.Equal(t, Facet{Status: FacetError, Reason: "飞牛的 appcgi.dockermgr.containerList 回了错误：Docker 没有启动（错误码 7）"}, snap.Facets[FacetApps])
			},
		},
		{"不相干的消息不影响", map[string]fnosFakeReply{fnosReqCPU: {body: fnosCPUBody, noise: true}}, func(t *testing.T, snap *Snapshot) {
			assert.InDelta(t, 23, snap.CPU.Percent, 1e-9)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fk := newFakeFnOS(t, false)
			s := fk.source(fnosSaved(t, fk))
			for req, r := range c.set {
				fk.set(req, r)
			}
			snap, err := s.Collect(fnosCtx(t), true)
			require.NoError(t, err)
			c.check(t, snap)
		})
	}
}

// 这一次的请求全被拒绝：整次采集失败，连接不再用，下一次重新连接、登录
func TestFnOSAllRejected(t *testing.T) {
	fk := newFakeFnOS(t, false)
	s := fk.source(fnosSaved(t, fk))
	for _, req := range []string{fnosReqCPU, fnosReqMem, fnosReqGen, fnosReqNet, fnosReqUptime} {
		fk.set(req, fnosFakeReply{errno: 4099, msg: "没有登录"})
	}
	_, err := s.Collect(fnosCtx(t), false)
	assert.ErrorContains(t, err, "飞牛拒绝了这一次的全部请求，下一次重新登录：飞牛的 appcgi.resmon.cpu 回了错误：没有登录（错误码 4099）")
	assert.Nil(t, s.conn)
	fk.set(fnosReqCPU, fnosFakeReply{body: fnosCPUBody})
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	upgrades, _, _ := fk.counts()
	assert.Equal(t, 3, upgrades, "测试连接、第一次与重连各一条")
}

// 连接断了：下一次采集重新连接，用 token 登录
func TestFnOSReconnect(t *testing.T) {
	fk := newFakeFnOS(t, false)
	s := fk.source(fnosSaved(t, fk))
	_, err := s.Collect(fnosCtx(t), true)
	require.NoError(t, err)
	conn := s.conn
	fk.dropConns()
	require.Eventually(t, conn.broken, 5*time.Second, 10*time.Millisecond)
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.NotSame(t, conn, s.conn)
	_, logins, _ := fk.counts()
	assert.Equal(t, 1, logins, "重连用 token，不发密码")

	// 采集到一半连接断了：这一次失败，连接不再用
	fk.set(fnosReqMem, fnosFakeReply{close: true})
	_, err = s.Collect(fnosCtx(t), false)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	assert.Nil(t, s.conn)
	fk.set(fnosReqMem, fnosFakeReply{body: fnosMemBody})
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)

	// 连不上：整次失败
	fk.ts.Close()
	fk.dropConns()
	require.Eventually(t, s.conn.broken, 5*time.Second, 10*time.Millisecond)
	_, err = s.Collect(fnosCtx(t), false)
	assert.ErrorContains(t, err, "连不上飞牛")
}

// 用 token 登录时连接断了：这一次失败，存下的 token 留着，下一次照旧先用它（不改用密码）
func TestFnOSAuthTransport(t *testing.T) {
	fk := newFakeFnOS(t, false)
	extra := fnosSaved(t, fk)
	s := fk.source(extra)
	fk.with(func() { fk.resume = &fnosFakeReply{close: true} })
	_, err := s.Collect(fnosCtx(t), false)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	assert.NotNil(t, s.sess, "连接的问题不丢掉 token")
	assert.Nil(t, s.conn)
	_, logins, _ := fk.counts()
	assert.Equal(t, 1, logins, "不改用密码")

	fk.with(func() { fk.resume = nil })
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.Nil(t, s.TakeExtra())

	// 改用密码登录时连接断了：这一次失败；token 已经不能用，下一次直接用密码
	fk.revoke(extra[fnosTokenKey])
	fk.dropConns()
	require.Eventually(t, s.conn.broken, 5*time.Second, 10*time.Millisecond)
	fk.with(func() { fk.login = &fnosFakeReply{close: true} })
	_, err = s.Collect(fnosCtx(t), false)
	assert.ErrorContains(t, err, "和飞牛的连接断了")
	assert.Nil(t, s.sess)

	fk.with(func() { fk.login = nil })
	fk.resetRequests()
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.Zero(t, fk.count("user.authToken"))
	assert.NotNil(t, s.TakeExtra())
}

// 某个请求一直没有回应：采集的时限到了，连接不再用
func TestFnOSTimeout(t *testing.T) {
	fk := newFakeFnOS(t, false)
	s := fk.source(fnosSaved(t, fk))
	_, err := s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	fk.set(fnosReqMem, fnosFakeReply{drop: true})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = s.Collect(ctx, false)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Nil(t, s.conn)
}

// 心跳：每隔一阵发 ping，飞牛回 pong（不进请求的回应）
func TestFnOSHeartbeat(t *testing.T) {
	fk := newFakeFnOS(t, false)
	c, err := dialFnOS(fnosCtx(t), fk.url(), "", nil, 20*time.Millisecond)
	require.NoError(t, err)
	defer c.close()
	require.Eventually(t, func() bool {
		_, _, pings := fk.counts()
		return pings >= 2
	}, 5*time.Second, 10*time.Millisecond)
	assert.False(t, c.broken())
	// 对面断开以后心跳写不出去，连接不能再用
	fk.dropConns()
	require.Eventually(t, c.broken, 5*time.Second, 10*time.Millisecond)
}

// 连接本身：没登录不能发签名请求；关掉以后在等的请求与新请求都拿到原因；ctx 结束时连接不再用
func TestFnosConn(t *testing.T) {
	fk := newFakeFnOS(t, false)
	c, err := dialFnOS(fnosCtx(t), fk.url(), "", nil, 0)
	require.NoError(t, err)
	assert.ErrorIs(t, c.call(fnosCtx(t), fnosReqCPU, nil, nil), errFnosNoLogin)
	_, err = c.login(fnosCtx(t), fk.user, fk.pass, "", newFnosDID(), true)
	require.NoError(t, err)
	assert.NoError(t, c.call(fnosCtx(t), fnosReqCPU, nil, nil), "out 为空时不解回应")
	assert.ErrorContains(t, c.call(fnosCtx(t), fnosReqCPU, map[string]any{"bad": make(chan int)}, nil), "写不成 JSON")

	fk.set(fnosReqMem, fnosFakeReply{drop: true})
	done := make(chan error, 1)
	go func() { done <- c.call(context.Background(), fnosReqMem, nil, nil) }()
	require.Eventually(t, func() bool { return fk.count(fnosReqMem) == 1 }, 5*time.Second, 10*time.Millisecond)
	c.close()
	select {
	case waited := <-done:
		assert.ErrorIs(t, waited, errFnosClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("关掉连接以后在等的请求没有返回")
	}
	assert.ErrorIs(t, c.call(fnosCtx(t), fnosReqCPU, nil, nil), errFnosClosed)
	c.close() // 关两次没事

	c2, err := dialFnOS(fnosCtx(t), fk.url(), "", nil, 0)
	require.NoError(t, err)
	defer c2.close()
	_, err = c2.login(fnosCtx(t), fk.user, fk.pass, "", newFnosDID(), true)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c2.call(ctx, fnosReqCPU, nil, nil), context.Canceled)
	// 写的时候 ctx 已经结束：coder/websocket 要么没写，要么写了以后关掉连接，两种都让这条连接不能再用
	require.Eventually(t, c2.broken, 5*time.Second, 10*time.Millisecond)
}

// resume：存下的凭据不完整时不发请求，调用方改用密码登录；设备号是这一次才生成的时存下它
func TestFnOSResume(t *testing.T) {
	fk := newFakeFnOS(t, false)
	extra := fnosSaved(t, fk)
	c, err := dialFnOS(fnosCtx(t), fk.url(), "", nil, 0)
	require.NoError(t, err)
	defer c.close()
	_, err = c.resume(fnosCtx(t), &fnosSession{Token: "t", Secret: "%%%"}, "did")
	assert.Equal(t, "存下的 token 不完整", fnosAuthErr(t, err).Reason)
	_, err = c.resume(fnosCtx(t), &fnosSession{Secret: extra[fnosSecretKey]}, "did")
	fnosAuthErr(t, err)

	// 库里的凭据缺了设备号：用 token 登录成功以后把新生成的设备号存下
	delete(extra, fnosDIDKey)
	s := fk.source(extra)
	_, err = s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	fresh := s.TakeExtra()
	require.NotNil(t, fresh)
	assert.Equal(t, s.did, fresh[fnosDIDKey])
	assert.Equal(t, extra[fnosTokenKey], fresh[fnosTokenKey])

	// 签名密钥坏了：改用密码登录
	extra[fnosSecretKey] = "%%%"
	s2 := fk.source(extra)
	_, err = s2.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	assert.NotEqual(t, "%%%", s2.TakeExtra()[fnosSecretKey])
}

func TestMDHealth(t *testing.T) {
	disks := func(states ...string) []struct {
		Name     string `json:"name"`
		ArrState string `json:"arrState"`
	} {
		out := make([]struct {
			Name     string `json:"name"`
			ArrState string `json:"arrState"`
		}, len(states))
		for i, s := range states {
			out[i].Name, out[i].ArrState = "sd"+string(rune('a'+i))+"1", s
		}
		return out
	}
	num := func(s string) synoNum {
		var n synoNum
		require.NoError(t, n.UnmarshalJSON([]byte(s)))
		return n
	}
	yes, no := true, false
	cases := []struct {
		name string
		md   fnosMD
		want fnosHealth
	}{
		{"正常", fnosMD{RaidDisks: num("2"), ArrayState: "clean", SyncAction: "idle", Disk: disks("in_sync", "in_sync")}, fnosHealth{"正常", &yes}},
		{"active 也正常", fnosMD{ArrayState: "Active", Disk: disks("in_sync")}, fnosHealth{"正常", &yes}},
		{"成员坏了", fnosMD{RaidDisks: num("2"), ArrayState: "clean", Disk: disks("in_sync", "faulty")}, fnosHealth{"降级", &no}},
		{"少一块盘", fnosMD{RaidDisks: num("2"), ArrayState: "clean", Disk: disks("in_sync")}, fnosHealth{"降级", &no}},
		{"备用盘不算缺", fnosMD{ArrayState: "clean", Disk: disks("in_sync", "in_sync", "spare")}, fnosHealth{"正常", &yes}},
		{"降级时在重建", fnosMD{RaidDisks: num("2"), ArrayState: "active", SyncAction: "recover", Disk: disks("in_sync", "spare")}, fnosHealth{"重建中", &no}},
		{"重建（成员都算同步）", fnosMD{ArrayState: "active", SyncAction: "recover", Disk: disks("in_sync", "in_sync")}, fnosHealth{"重建中", &no}},
		{"同步中", fnosMD{ArrayState: "active", SyncAction: "resync", Disk: disks("in_sync", "in_sync")}, fnosHealth{"同步中", nil}},
		{"调整中", fnosMD{ArrayState: "active", SyncAction: "reshape", Disk: disks("in_sync", "in_sync")}, fnosHealth{"调整中", nil}},
		{"校验中照常健康", fnosMD{ArrayState: "clean", SyncAction: "check", Disk: disks("in_sync", "in_sync")}, fnosHealth{"校验中", &yes}},
		{"已损毁", fnosMD{ArrayState: "broken", Disk: disks("in_sync")}, fnosHealth{"已损毁", &no}},
		{"没在运行", fnosMD{ArrayState: "inactive"}, fnosHealth{"没在运行", &no}},
		{"不认得的状态", fnosMD{ArrayState: "readonly", Disk: disks("in_sync")}, fnosHealth{"readonly", nil}},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, mdHealth(c.md), c.name)
	}

	// 一个存储空间底下几个阵列：状态取最差的；硬盘按成员盘去重
	view := fnosPools([]fnosStorage{{Name: "dm-0", Mountpoint: "/vol1"}, {Name: "dm-1"}}, &fnosStorState{State: []struct {
		Name string   `json:"name"`
		UUID string   `json:"uuid"`
		MD   []fnosMD `json:"md"`
	}{
		{Name: "dm-0", MD: []fnosMD{
			{ArrayState: "clean", Disk: disks("in_sync", "in_sync")},
			{ArrayState: "active", SyncAction: "resync", Disk: disks("in_sync", "in_sync")},
		}},
		{Name: "dm-1", MD: []fnosMD{
			{ArrayState: "clean", Disk: disks("in_sync")},
			{ArrayState: "clean", RaidDisks: num("2"), Disk: disks("in_sync")},
		}},
	}})
	require.Len(t, view.pools, 2)
	assert.Equal(t, "同步中", view.pools[0].Status)
	assert.Nil(t, view.pools[0].Healthy)
	assert.Equal(t, 2, view.pools[0].Disks)
	assert.Equal(t, "dm-1", view.pools[1].Name, "没有挂载点时用设备名")
	assert.Equal(t, "降级", view.pools[1].Status)
	assert.Equal(t, &no, view.pools[1].Healthy)
	assert.Equal(t, 1, view.pools[1].Disks)
	assert.Equal(t, map[string]string{"sda": "/vol1", "sdb": "/vol1"}, view.diskOf, "两个存储空间都有的盘记第一个")

	// 有 uuid 的按 uuid 对；对不上的不写状态
	view = fnosPools([]fnosStorage{{Name: "dm-0", UUID: "u-1"}}, &fnosStorState{State: []struct {
		Name string   `json:"name"`
		UUID string   `json:"uuid"`
		MD   []fnosMD `json:"md"`
	}{{Name: "dm-0", UUID: "u-2", MD: []fnosMD{{ArrayState: "clean"}}}}})
	assert.Empty(t, view.pools[0].Status)
	// 有状态、却没有阵列
	view = fnosPools([]fnosStorage{{Name: "dm-0"}}, &fnosStorState{State: []struct {
		Name string   `json:"name"`
		UUID string   `json:"uuid"`
		MD   []fnosMD `json:"md"`
	}{{Name: "dm-0"}}})
	assert.Empty(t, view.pools[0].Status)
	assert.Nil(t, view.pools[0].Healthy)
}

func TestFnosDiskNames(t *testing.T) {
	for in, want := range map[string]string{
		"sda1": "sda", "/dev/sdb3": "sdb", "sdaa12": "sdaa", "nvme0n1p2": "nvme0n1", "nvme1n1": "nvme1n1",
		"vdb1": "vdb", "md0": "md0", " sdc ": "sdc", "": "",
	} {
		assert.Equal(t, want, fnosDiskOf(in), in)
	}
	for _, c := range []struct{ name, typ, protocol, want string }{
		{"nvme0n1", "SSD", "NVMe", DiskNVMe},
		{"sda", "NVME", "", DiskNVMe},
		{"nvme1n1", "", "", DiskNVMe},
		{"sda", "SSD", "SATA", DiskSSD},
		{"sdb", "HDD", "SAS", DiskHDD},
		{"sdc", "USB", "USB", DiskUnknown},
	} {
		assert.Equal(t, c.want, fnosDiskKind(c.name, c.typ, c.protocol), c.name+c.typ)
	}
}

// fnosReqIDRe 核对请求号的样子（同 TestFnosIDs）：确保签名的请求带上了它。
var fnosReqIDRe = regexp.MustCompile(`"reqid":"\d{13}[0-9a-f]{12}"`)

func TestFnosSignedFrames(t *testing.T) {
	fk := newFakeFnOS(t, false)
	s := fk.source(fnosSaved(t, fk))
	_, err := s.Collect(fnosCtx(t), false)
	require.NoError(t, err)
	signed := 0
	for _, fr := range fk.allFrames() {
		if strings.HasPrefix(fr, "{") {
			continue
		}
		signed++
		require.Greater(t, len(fr), 44)
		_, err := base64.StdEncoding.DecodeString(fr[:44])
		assert.NoError(t, err, "前 44 个字是签名")
		assert.Regexp(t, fnosReqIDRe, fr[44:])
	}
	assert.Equal(t, 9, signed, "测试连接读 3 个，采集是 user.authToken 加 5 个数据请求")
}

// 采集到一半连接断了，断在哪个请求都一样：整次采集失败、连接不再用，不记成某一类读不到
func TestFnOSCollectDropped(t *testing.T) {
	for _, req := range []string{
		fnosReqCPU, fnosReqMem, fnosReqGen, fnosReqNet, fnosReqUptime, fnosReqHostName,
		fnosReqStorage, fnosReqStorState, fnosReqDisks, fnosReqDiskMon, fnosReqContainers,
	} {
		t.Run(req, func(t *testing.T) {
			fk := newFakeFnOS(t, false)
			s := fk.source(fnosSaved(t, fk))
			fk.set(req, fnosFakeReply{close: true})
			snap, err := s.Collect(fnosCtx(t), true)
			assert.Nil(t, snap)
			assert.ErrorContains(t, err, "和飞牛的连接断了")
			assert.Nil(t, s.conn)
		})
	}
}

// 写不出去（连接已经关了）：去掉在等的请求号，连接记为不能再用；ctx 已经结束时回 ctx 的错误
func TestFnosWriteFails(t *testing.T) {
	fk := newFakeFnOS(t, false)
	for _, cancelled := range []bool{false, true} {
		u := fk.url()
		u.Scheme, u.Path, u.RawQuery = "ws", fnosPath, fnosQuery
		ws, _, err := websocket.Dial(fnosCtx(t), u.String(), nil)
		require.NoError(t, err)
		require.NoError(t, ws.CloseNow())
		// 没有读循环：连接是不是断了只由这一次写决定
		c := &fnosConn{ws: ws, pending: map[string]chan fnosMsg{}}
		ctx := fnosCtx(t)
		if cancelled {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		_, err = c.roundTrip(ctx, "r1", "x", []byte(`{}`))
		if cancelled {
			assert.ErrorIs(t, err, context.Canceled)
		} else {
			assert.ErrorIs(t, err, net.ErrClosed)
		}
		assert.True(t, c.broken())
		assert.Empty(t, c.pending)
		// 断了以后不再写
		_, err = c.roundTrip(fnosCtx(t), "r2", "x", []byte(`{}`))
		assert.ErrorContains(t, err, "和飞牛的连接断了")
	}
}
