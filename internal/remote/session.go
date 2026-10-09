package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

// 会话（主机端）：握手以后一个读者循环收帧；请求体收齐（REQ_END）以后在单独的 goroutine 里分发，回应按帧写回。
const (
	// MaxInflight 是一条会话里同时进行的请求数上限，再多的请求直接回 429
	MaxInflight = 16
	// MaxRequestBody 是一个请求体的上限
	MaxRequestBody = 16 << 20
	// maxSessionBuffered 是一条会话里所有没处理完的请求体加起来的上限
	maxSessionBuffered = 16 << 20
	// maxHeadLen 是 REQ_HEAD 里 JSON 的上限
	maxHeadLen = 8 << 10
	maxPathLen = 2048
	maxHeaders = 16
	// maxHeaderValue 是一个头的值的上限
	maxHeaderValue = 1024
	// maxPairBody 是配对请求体的上限
	maxPairBody = 4 << 10
	// IdleTimeout 是多久没收到任何帧就断开（App 至少每 30 秒发一次 PING）
	IdleTimeout = 90 * time.Second
	// writeTimeout 是写一个帧的上限：对端一直不读时断开这条会话
	writeTimeout = 30 * time.Second
	// goAwayTimeout 是关会话前发 GOAWAY 最多等多久
	goAwayTimeout = 2 * time.Second
	// PairPath 是配对会话唯一能调用的接口
	PairPath = "/remote/v1/pair"
	// AppAPIPrefix 是设备会话能调用的接口前缀
	AppAPIPrefix = "/api/app/v1/"
)

// 连接方式（Peer.Via、设备的最近在线方式）。
const (
	ViaDirect = "direct"
	ViaRelay  = "relay"
)

var (
	// requestHeaderAllow 是转发给 App API 的请求头；Cookie、Authorization 之类一律不转
	requestHeaderAllow = []string{"Accept", "Accept-Language", "Content-Type", "If-Modified-Since", "If-None-Match"}
	// responseHeaderAllow 是写回给设备的回应头
	responseHeaderAllow = []string{"Allow", "Cache-Control", "Content-Disposition", "Content-Type", "Etag", "Last-Modified", "Retry-After"}
	// allowedMethods 是隧道里能用的方法
	allowedMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

	errProtocol = errors.New("对端违反了协议")
	errAborted  = errors.New("请求已经取消或者会话已经关闭")
)

// Peer 是设备会话的另一端。
type Peer struct {
	Device Device
	// Via 是连接方式：direct 或 relay
	Via string
}

// Dispatcher 处理设备会话里的请求。请求只会是 /api/app/v1/* 的路径，只带白名单里的头（没有 Cookie 与 Authorization）；
// 实现要按 Peer.Device 的权限范围鉴权（web 包把它当成远程设备主体交给 App API）。
type Dispatcher interface {
	ServeRemote(w http.ResponseWriter, r *http.Request, p Peer)
}

// tunnelReq 是一个还没处理完的请求。
type tunnelReq struct {
	head RequestHead
	body []byte
	// limit 是这个请求体的上限：配对请求 4 KiB，别的 16 MiB
	limit      int
	dropped    bool // 已经回了 413，后面的请求体丢掉，等 REQ_END
	dispatched bool
	cancel     context.CancelFunc
}

// session 是主机端的一条会话。
type session struct {
	h    *Host
	conn *secureConn
	raw  MsgConn
	via  string
	mode string
	// peerKey 是设备的 X25519 公钥（握手里拿到的，已经认证过）
	peerKey []byte
	device  Device
	epoch   uint64

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	reqs     map[uint32]*tunnelReq
	buffered int
	// closeAfter 不为空时，处理完当前请求以后用这个原因关掉会话（配对完成）
	closeAfter string

	closeOnce sync.Once
	handlers  sync.WaitGroup
	done      chan struct{}
}

func newSession(h *Host, conn *secureConn, raw MsgConn, via, mode string, peerKey []byte, epoch uint64) *session {
	ctx, cancel := context.WithCancel(h.ctx)
	return &session{
		h: h, conn: conn, raw: raw, via: via, mode: mode, peerKey: peerKey, epoch: epoch,
		ctx: ctx, cancel: cancel, reqs: map[uint32]*tunnelReq{}, done: make(chan struct{}),
	}
}

// serve 是读者循环，返回时会话已经关闭。
func (s *session) serve() {
	defer func() {
		s.shutdown("")
		s.handlers.Wait()
		close(s.done)
	}()
	for {
		rctx, cancel := context.WithTimeout(s.ctx, s.h.cfg.IdleTimeout)
		f, err := s.conn.ReadFrame(rctx)
		cancel()
		if err != nil {
			if errors.Is(err, ErrFrame) {
				s.shutdown(GoAwayProtocol)
			}
			return
		}
		if err := s.handle(f); err != nil {
			s.h.logf("[远程访问] 会话违反协议，断开: %v", err)
			s.shutdown(GoAwayProtocol)
			return
		}
	}
}

func (s *session) handle(f Frame) error {
	switch f.Type {
	case FrameReqHead:
		return s.onHead(f)
	case FrameReqBody:
		s.onBody(f)
	case FrameReqEnd:
		s.onEnd(f)
	case FrameCancel:
		s.onCancel(f)
	case FramePing:
		return s.write(Frame{Type: FramePong, Payload: f.Payload})
	case FramePong:
	default:
		return fmt.Errorf("%w: 设备不该发类型 0x%02x", errProtocol, byte(f.Type))
	}
	return nil
}

// write 发一个帧（最多等 writeTimeout）；写失败说明连接断了，关掉会话。
func (s *session) write(f Frame) error {
	ctx, cancel := context.WithTimeout(s.ctx, writeTimeout)
	defer cancel()
	if err := s.conn.WriteFrame(ctx, f); err != nil {
		s.shutdown("")
		return err
	}
	return nil
}

// reject 直接回一个错误（请求还没分发）。
func (s *session) reject(id uint32, status int, code, message string) {
	body, _ := json.Marshal(map[string]string{"error": code, "message": message})
	head, _ := json.Marshal(ResponseHead{Status: status, Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}})
	if s.write(Frame{Type: FrameRespHead, ID: id, Payload: head}) != nil {
		return
	}
	if s.write(Frame{Type: FrameRespBody, ID: id, Payload: body}) != nil {
		return
	}
	_ = s.write(Frame{Type: FrameRespEnd, ID: id})
}

func (s *session) onHead(f Frame) error {
	s.mu.Lock()
	_, dup := s.reqs[f.ID]
	full := len(s.reqs) >= MaxInflight
	s.mu.Unlock()
	if dup {
		return fmt.Errorf("%w: 请求编号 %d 还在用", errProtocol, f.ID)
	}
	if full {
		s.reject(f.ID, http.StatusTooManyRequests, "too_many_requests", fmt.Sprintf("同时进行的请求不能超过 %d 个", MaxInflight))
		return nil
	}
	var head RequestHead
	if len(f.Payload) > maxHeadLen || json.Unmarshal(f.Payload, &head) != nil {
		s.reject(f.ID, http.StatusBadRequest, "bad_request", "请求头格式不对")
		return nil
	}
	if status, code, msg := checkHead(head, s.mode); status != 0 {
		s.reject(f.ID, status, code, msg)
		return nil
	}
	limit := MaxRequestBody
	if s.mode == ModePairing {
		// 配对会话不需要知道配对密钥就能建立：请求体在收包时就按配对请求的上限截住，不让它占着 16 MiB 的额度
		limit = maxPairBody
	}
	s.mu.Lock()
	s.reqs[f.ID] = &tunnelReq{head: head, limit: limit}
	s.mu.Unlock()
	return nil
}

// checkHead 检查方法、路径与头；配对会话只能 POST /remote/v1/pair，设备会话只能调用 /api/app/v1/*。
func checkHead(h RequestHead, mode string) (status int, code, message string) {
	if !slices.Contains(allowedMethods, h.Method) {
		return http.StatusMethodNotAllowed, "method_not_allowed", "不支持这个方法"
	}
	if len(h.Path) == 0 || len(h.Path) > maxPathLen || h.Path[0] != '/' || strings.IndexFunc(h.Path, func(r rune) bool { return r <= ' ' || r >= 0x7f || r == '#' }) >= 0 {
		return http.StatusBadRequest, "bad_request", "路径不对"
	}
	p, rawQuery, _ := strings.Cut(h.Path, "?")
	if !validPath(p) {
		return http.StatusBadRequest, "bad_request", "路径不对"
	}
	if _, err := url.ParseQuery(rawQuery); err != nil {
		return http.StatusBadRequest, "bad_request", "查询串不对"
	}
	if len(h.Headers) > maxHeaders {
		return http.StatusBadRequest, "bad_request", "请求头太多"
	}
	for k, v := range h.Headers {
		if len(k) == 0 || len(k) > 64 || len(v) > maxHeaderValue || strings.ContainsAny(k+v, "\r\n\x00") {
			return http.StatusBadRequest, "bad_request", "请求头不对"
		}
	}
	switch mode {
	case ModePairing:
		if h.Method != http.MethodPost || p != PairPath {
			return http.StatusForbidden, "forbidden", "配对会话只能配对"
		}
	case ModeDevice:
		if !strings.HasPrefix(p, AppAPIPrefix) {
			return http.StatusForbidden, "forbidden", "远程设备只能调用 App API（/api/app/v1/）"
		}
	default:
		return http.StatusForbidden, "forbidden", "不认识的会话"
	}
	return 0, "", ""
}

// validPath 只接受规整的路径：没有百分号转义，没有 . 与 .. 段、空段和末尾的 /，字符限于 RFC 3986 的 pchar。
func validPath(p string) bool {
	if p == "" || p[0] != '/' || path.Clean(p) != p {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("-._~!$&'()*+,;=:@/", c) >= 0:
		default:
			return false
		}
	}
	return true
}

func (s *session) onBody(f Frame) {
	s.mu.Lock()
	r := s.reqs[f.ID]
	if r == nil || r.dropped || r.dispatched {
		s.mu.Unlock()
		return
	}
	tooBig := len(r.body)+len(f.Payload) > r.limit || s.buffered+len(f.Payload) > maxSessionBuffered
	if tooBig {
		s.buffered -= len(r.body)
		r.body, r.dropped = nil, true
	} else {
		r.body = append(r.body, f.Payload...)
		s.buffered += len(f.Payload)
	}
	s.mu.Unlock()
	if tooBig {
		s.reject(f.ID, http.StatusRequestEntityTooLarge, "body_too_large", fmt.Sprintf("请求体不能超过 %d 字节", r.limit))
	}
}

func (s *session) onEnd(f Frame) {
	s.mu.Lock()
	r := s.reqs[f.ID]
	if r == nil || r.dispatched {
		s.mu.Unlock()
		return
	}
	if r.dropped {
		delete(s.reqs, f.ID)
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	r.dispatched, r.cancel = true, cancel
	s.handlers.Add(1)
	s.mu.Unlock()
	go s.dispatch(ctx, f.ID, r)
}

func (s *session) onCancel(f Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.reqs[f.ID]
	if r == nil {
		return
	}
	if r.dispatched {
		// 处理函数照样跑完，只是之后不再给这个编号发帧；finish 时删掉
		r.cancel()
		return
	}
	s.buffered -= len(r.body)
	delete(s.reqs, f.ID)
}

// finish 是一个请求处理完了：释放请求体占的额度。
func (s *session) finish(id uint32) {
	s.mu.Lock()
	r := s.reqs[id]
	if r != nil {
		s.buffered -= len(r.body)
		delete(s.reqs, id)
	}
	reason := s.closeAfter
	s.mu.Unlock()
	if r != nil && r.cancel != nil {
		r.cancel()
	}
	if reason != "" {
		s.shutdown(reason)
	}
}

func (s *session) dispatch(ctx context.Context, id uint32, r *tunnelReq) {
	defer s.handlers.Done()
	defer s.finish(id)
	w := &tunnelWriter{s: s, id: id, ctx: ctx, header: http.Header{}, head: r.head.Method == http.MethodHead}
	defer func() {
		if p := recover(); p != nil {
			if p != http.ErrAbortHandler {
				s.h.logf("[远程访问] 处理 %s %s 时出错: %v", r.head.Method, r.head.Path, p)
			}
			w.abort()
		}
	}()
	req, err := buildRequest(ctx, r.head, r.body, s.via)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad_request", "请求不对")
		w.finish()
		return
	}
	switch s.mode {
	case ModePairing:
		s.servePair(w, req)
	case ModeDevice:
		s.h.cfg.Dispatcher.ServeRemote(w, req, Peer{Device: s.device, Via: s.via})
	}
	w.finish()
}

// buildRequest 由隧道里的请求建出 http.Request：只带白名单里的头，Host 是 remote。
func buildRequest(ctx context.Context, head RequestHead, body []byte, via string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, head.Method, head.Path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = "remote"
	req.RemoteAddr = "remote-" + via
	req.RequestURI = head.Path
	for k, v := range head.Headers {
		if ck := http.CanonicalHeaderKey(k); slices.Contains(requestHeaderAllow, ck) {
			req.Header.Set(ck, v)
		}
	}
	return req, nil
}

// tunnelWriter 把处理函数的回应写成 RESP_HEAD、RESP_BODY、RESP_END。
type tunnelWriter struct {
	s         *session
	id        uint32
	ctx       context.Context
	header    http.Header
	head      bool // HEAD 请求：不写回应体
	wroteHead bool
	failed    bool
}

func (w *tunnelWriter) Header() http.Header { return w.header }

func (w *tunnelWriter) WriteHeader(code int) {
	if w.wroteHead || (code >= 100 && code < 200) {
		return
	}
	if code < 200 || code > 999 {
		code = http.StatusInternalServerError
	}
	w.wroteHead = true
	head := ResponseHead{Status: code, Headers: map[string]string{}}
	for _, k := range responseHeaderAllow {
		if v := w.header.Get(k); v != "" && len(v) <= maxHeaderValue {
			head.Headers[k] = v
		}
	}
	b, _ := json.Marshal(head)
	_ = w.send(Frame{Type: FrameRespHead, ID: w.id, Payload: b})
}

func (w *tunnelWriter) Write(p []byte) (int, error) {
	if !w.wroteHead {
		if w.header.Get("Content-Type") == "" && len(p) > 0 {
			w.header.Set("Content-Type", http.DetectContentType(p))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.failed {
		return 0, errAborted
	}
	if w.head {
		return len(p), nil
	}
	n := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), MaxFramePayload)]
		if err := w.send(Frame{Type: FrameRespBody, ID: w.id, Payload: chunk}); err != nil {
			return n, err
		}
		n += len(chunk)
		p = p[len(chunk):]
	}
	return n, nil
}

// Flush 什么也不做：每次 Write 都已经发出去了。
func (w *tunnelWriter) Flush() {}

func (w *tunnelWriter) send(f Frame) error {
	if w.failed {
		return errAborted
	}
	if w.ctx.Err() != nil {
		// 设备取消了这个请求，或者会话在关：之后不再给这个编号发帧
		w.failed = true
		return errAborted
	}
	if err := w.s.write(f); err != nil {
		w.failed = true
		return err
	}
	return nil
}

// finish 是处理函数返回了：没写过回应头时补一个 200，再发 RESP_END。
func (w *tunnelWriter) finish() {
	if !w.wroteHead {
		w.WriteHeader(http.StatusOK)
	}
	_ = w.send(Frame{Type: FrameRespEnd, ID: w.id})
}

// abort 是处理函数出错了：回应头还没发时回 500，发了就用 CANCEL 告诉设备回应不完整。
func (w *tunnelWriter) abort() {
	if w.failed {
		return
	}
	if !w.wroteHead {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "处理请求时出错")
		w.finish()
		return
	}
	_ = w.send(Frame{Type: FrameCancel, ID: w.id, Payload: []byte("internal error")})
	w.failed = true
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

// pairRequest 是 POST /remote/v1/pair 的请求体。
type pairRequest struct {
	Secret string `json:"secret"`
	Name   string `json:"name"`
}

// 配对结果的审计写法（和 App API 的审计一样：success、denied:…、error:…）。
const (
	auditPairOK         = "success"
	auditPairBadBody    = "denied:invalid_body"
	auditPairBadName    = "denied:invalid_name"
	auditPairBadSecret  = "denied:secret"
	auditPairClosed     = "denied:pairing_closed"
	auditPairRejected   = "denied:rejected"
	auditPairSaveFailed = "error:save_device"
)

// servePair 处理配对：核对配对密钥，记下设备，回设备信息；之后会话用 GOAWAY paired 关掉，设备重连成正常会话。
// 每个结果都记审计（还没有设备时设备编号是 0）。
func (s *session) servePair(w http.ResponseWriter, r *http.Request) {
	if !s.h.pairings.open() {
		s.h.audit(r.Context(), 0, "remote:pair", auditPairClosed)
		writeJSONError(w, http.StatusGone, "pairing_closed", ErrPairingClosed.Error())
		s.setCloseAfter(GoAwayPairingClosed)
		return
	}
	var in pairRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxPairBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || !errors.Is(dec.Decode(&struct{}{}), io.EOF) {
		s.pairFailed(r, auditPairBadBody)
		writeJSONError(w, http.StatusBadRequest, "invalid_body", "请求格式错误")
		return
	}
	name, err := NormalizeDeviceName(in.Name)
	if err != nil {
		s.pairFailed(r, auditPairBadName)
		writeJSONError(w, http.StatusBadRequest, "invalid_name", err.Error())
		return
	}
	// 格式不对的密钥也算输错一次
	secret, _ := DecodeKey(in.Secret)
	dev, closedNow, err := s.h.pairings.redeem(secret, func(scopes []string) (Device, error) {
		// 在配对窗口的锁里写设备表：取消窗口、换新窗口不会插到核对与写库之间
		ctx, cancel := boundedContext(r.Context())
		defer cancel()
		return s.h.cfg.Store.CreateDevice(ctx, name, s.peerKey, scopes)
	})
	switch {
	case errors.Is(err, ErrPairingSecret):
		s.h.audit(r.Context(), 0, "remote:pair", auditPairBadSecret)
		writeJSONError(w, http.StatusUnauthorized, "invalid_secret", err.Error())
		if closedNow {
			// 输错到了上限：窗口作废，这条与别的配对会话都关掉
			s.setCloseAfter(GoAwayPairingClosed)
			s.h.closePairingSessions(s)
		}
		return
	case errors.Is(err, ErrPairingClosed):
		s.h.audit(r.Context(), 0, "remote:pair", auditPairClosed)
		writeJSONError(w, http.StatusGone, "pairing_closed", err.Error())
		s.setCloseAfter(GoAwayPairingClosed)
		return
	case errors.Is(err, ErrInvalid):
		s.h.audit(r.Context(), 0, "remote:pair", auditPairRejected)
		writeJSONError(w, http.StatusConflict, "rejected", err.Error())
		return
	case err != nil:
		s.h.logf("[远程访问] 保存配对的设备失败: %v", err)
		s.h.audit(r.Context(), 0, "remote:pair", auditPairSaveFailed)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "保存设备失败")
		return
	}
	s.h.audit(r.Context(), dev.ID, "remote:pair", auditPairOK)
	s.h.paired(dev, s)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"device": dev})
	s.setCloseAfter(GoAwayPaired)
}

// pairFailed 记下一次请求体或设备名不对的配对请求：计入窗口的失败次数（到 5 次窗口作废，配对会话全部关掉）并记审计。
func (s *session) pairFailed(r *http.Request, result string) {
	s.h.audit(r.Context(), 0, "remote:pair", result)
	if s.h.pairings.fail() {
		s.setCloseAfter(GoAwayPairingClosed)
		s.h.closePairingSessions(s)
	}
}

func (s *session) setCloseAfter(reason string) {
	s.mu.Lock()
	s.closeAfter = reason
	s.mu.Unlock()
}

// shutdown 关掉会话：reason 不为空时先尽力发 GOAWAY（最多等 2 秒），再关连接。可以重复调用。
func (s *session) shutdown(reason string) {
	s.closeOnce.Do(func() {
		if reason != "" {
			payload, _ := json.Marshal(GoAway{Reason: reason})
			ctx, cancel := context.WithTimeout(context.Background(), goAwayTimeout)
			_ = s.conn.WriteFrame(ctx, Frame{Type: FrameGoAway, Payload: payload})
			cancel()
		}
		s.cancel()
		s.raw.Close(1000, reason)
		s.h.sessions.remove(s)
		s.h.sessions.release(s.mode)
		if s.mode == ModeDevice {
			s.h.touch(s.device.ID, s.via)
		}
	})
}
