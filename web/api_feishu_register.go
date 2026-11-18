package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify/adapter/feishuapp"
)

/*
 * 飞书应用机器人「扫码一键创建」：网页开始一次、拿到二维码；用户在飞书里确认以后，后端用拿到的 App ID 与 App Secret
 * 建好通道（启用；管理员是扫码人；平台按扫码人的租户品牌），再把扫码人直接绑定。App Secret 不回给网页、不进日志，
 * 建好以后和别的通道的凭证一样，只在登录以后的通道详情里回显。
 *
 * 只认 session（经 RegisterChatOpsRoutes 的 requireAuth）；不进 App API、MCP、配置网关与远程设备：
 * 它能在扫码人的企业里建应用并拿到凭据，只给登录了网页的管理员用。
 *
 * 一个进程同时只有一个在等扫码的（Serve 只建一次路由）：再开始一个会取消上一个；查状态与取消都带编号，
 * 只作用于那一个，旧标签页取消不了新开的。链接与二维码只在开始接口的回应里给一次。
 */

// 一键创建的状态。
const (
	feishuRegPending  = "pending"  // 等扫码与确认
	feishuRegSaving   = "saving"   // 飞书已经建好应用，正在保存通道与绑定，这一步不能取消
	feishuRegDone     = "done"     // 通道建好（message 不为空时是绑定没成之类的提示）
	feishuRegDenied   = "denied"   // 用户在飞书里取消了
	feishuRegExpired  = "expired"  // 二维码过期
	feishuRegCanceled = "canceled" // 网页取消，或被新开始的取代
	feishuRegFailed   = "failed"   // 其他失败，message 写原因
)

const (
	feishuRegDefaultName = "飞书应用机器人"
	feishuRegNameMax     = 64
	msgFeishuRegGone     = "这个一键创建已经被新的取代或过期"
	msgFeishuRegNotWired = "这个版本没有接入一键创建"
	msgFeishuRegNoLink   = "飞书没有及时返回二维码"
	msgFeishuRegManual   = "可以重新生成二维码，或者改用「填写 App ID 与 App Secret」"
)

// feishuReg 是一次一键创建。链接与二维码只在开始接口的回应里给一次，这里不存；App Secret 也不存。
type feishuReg struct {
	id        string
	name      string
	state     string
	message   string
	expiresAt time.Time
	appID     string
	confID    uint
	endedAt   time.Time // 到终态的时间：终态留 keep 这么久
	cancel    context.CancelFunc
	logf      func(warn bool, format string, args ...any)
}

// view 是查状态与取消的回应：不带链接与二维码。调用方持有 mu。
func (j *feishuReg) view() feishuRegisterStatus {
	return feishuRegisterStatus{ID: j.id, State: j.state, ExpiresAt: j.expiresAt, AppID: j.appID, ConfID: j.confID, Message: j.message}
}

// feishuRegisterStatus 是一次一键创建的状态。
type feishuRegisterStatus struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	AppID     string    `json:"app_id,omitempty"`
	ConfID    uint      `json:"conf_id,omitempty"`
	Message   string    `json:"message,omitempty"`
}

// feishuRegisterStart 是开始接口的回应：链接（带一次性的 user_code）与二维码只在这里给一次。
type feishuRegisterStart struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	QRSVG     string    `json:"qr_svg"`
	ExpiresAt time.Time `json:"expires_at"`
	State     string    `json:"state"`
}

type feishuRegQR struct {
	url      string
	expireIn int
}

// feishuRegistrar 管着当前这一个一键创建（只保留最近一个）。
type feishuRegistrar struct {
	mu  sync.Mutex
	cur *feishuReg

	run    func(ctx context.Context, onQR func(url string, expireIn int)) (*feishuapp.RegisteredApp, error)
	create func(ctx context.Context, req app.CreateConfReq) (app.NotificationConfDTO, error)
	bind   func(ctx context.Context, confID uint, openID string) (app.BindingDTO, error)
	audit  func(ctx context.Context, e app.AuditEntry) error

	// 等待时长（测试里调小）
	linkWait    time.Duration // 开始接口等链接最多多久
	jobTimeout  time.Duration // 一次一键创建最多多久（链接 10 分钟内有效）
	saveTimeout time.Duration // 保存通道、绑定与审计最多多久
	keep        time.Duration // 终态留多久
	now         func() time.Time

	// jobs 数着还在跑的后台协程（测试等它们退出；进程退出时不专门处理，协程最多 jobTimeout，跟着进程结束）
	jobs sync.WaitGroup
}

// newFeishuRegistrar 用 ChatOps 的依赖建通道、直接绑定与记审计；没接上一键创建（或缺建通道、绑定的服务）时三个接口回 503。
func newFeishuRegistrar(deps *ChatOpsDeps) *feishuRegistrar {
	r := &feishuRegistrar{
		linkWait: 20 * time.Second, jobTimeout: 15 * time.Minute, saveTimeout: 30 * time.Second,
		keep: 30 * time.Minute, now: time.Now,
	}
	if deps.FeishuRegister != nil && deps.NotificationSvc != nil && deps.BindingSvc != nil {
		r.run = deps.FeishuRegister
		r.create = deps.NotificationSvc.CreateConf
		r.bind = deps.BindingSvc.BindDirect
	}
	if deps.AuditSvc != nil {
		r.audit = deps.AuditSvc.Record
	}
	return r
}

func (r *feishuRegistrar) register(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("POST /api/chatops/feishu-app/register", wrap(r.start))
	mux.Handle("GET /api/chatops/feishu-app/register/{id}", wrap(r.status))
	mux.Handle("POST /api/chatops/feishu-app/register/{id}/cancel", wrap(r.cancelOne))
}

// ready 设好 no-store（开始接口带一次性的链接；查状态是轮询，不能拿到缓存的旧状态），没接上时回 503。
func (r *feishuRegistrar) ready(w http.ResponseWriter) bool {
	w.Header().Set("Cache-Control", "no-store")
	if r.run == nil {
		writeChatopsErr(w, http.StatusServiceUnavailable, "not_wired", msgFeishuRegNotWired)
		return false
	}
	return true
}

type feishuRegisterBody struct {
	Name string `json:"name"`
}

// start 是 POST /api/chatops/feishu-app/register：取消上一个，起一个新的；等到链接（最多 linkWait）回链接与二维码。
func (r *feishuRegistrar) start(w http.ResponseWriter, req *http.Request) {
	if !r.ready(w) {
		return
	}
	var body feishuRegisterBody
	req.Body = http.MaxBytesReader(w, req.Body, 4<<10)
	if err := decodeJSONBody(req, &body); err != nil && !errors.Is(err, io.EOF) {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = feishuRegDefaultName
	}
	if utf8.RuneCountInString(name) > feishuRegNameMax {
		writeChatopsErr(w, http.StatusBadRequest, "invalid_argument", fmt.Sprintf("通道名称最长 %d 个字", feishuRegNameMax))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.jobTimeout)
	j := &feishuReg{id: newFeishuRegID(), name: name, state: feishuRegPending, cancel: cancel, logf: regLogger()}
	// 后台协程拿到链接时送进来一次；没拿到链接就结束时关上（见 runJob）
	qr := make(chan feishuRegQR, 1)

	r.mu.Lock()
	if old := r.cur; old != nil {
		switch old.state {
		case feishuRegPending:
			r.endLocked(old, feishuRegCanceled, "开始了新的一键创建，这个已经取消")
			old.cancel()
		case feishuRegSaving:
			old.logf(false, "id=%s 被新的一键创建取代，但飞书已经建好应用，照常建完通道", shortRegID(old.id))
		}
	}
	r.cur = j
	r.mu.Unlock()
	j.logf(false, "开始 id=%s name=%q", shortRegID(j.id), name)

	r.jobs.Add(1)
	go r.runJob(ctx, j, qr)

	timer := time.NewTimer(r.linkWait)
	defer timer.Stop()
	var link feishuRegQR
	select {
	case l, ok := <-qr:
		if !ok {
			r.endedBeforeLink(w, j)
			return
		}
		link = l
	case <-timer.C:
		r.abort(j, feishuRegFailed, msgFeishuRegNoLink)
		writeChatopsErr(w, http.StatusGatewayTimeout, "register_timeout", msgFeishuRegNoLink)
		return
	case <-req.Context().Done():
		// 网页断开了：链接只在这次回应里给，没人看得到它
		r.abort(j, feishuRegCanceled, "网页没有等到二维码就断开了")
		return
	}

	svg, err := qrSVG(link.url)
	if err != nil {
		r.abort(j, feishuRegFailed, "生成二维码失败")
		writeChatopsErr(w, http.StatusInternalServerError, "qr_failed", "生成二维码失败: "+err.Error())
		return
	}
	r.mu.Lock()
	j.expiresAt = r.now().Add(time.Duration(link.expireIn) * time.Second)
	out := feishuRegisterStart{ID: j.id, URL: link.url, QRSVG: svg, ExpiresAt: j.expiresAt, State: j.state}
	r.mu.Unlock()
	writeJSON(w, out)
}

// endedBeforeLink 回应拿到链接以前就结束的作业：被取代或取消时 409，其他 502。
func (r *feishuRegistrar) endedBeforeLink(w http.ResponseWriter, j *feishuReg) {
	r.mu.Lock()
	state, msg := j.state, j.message
	r.mu.Unlock()
	if state == feishuRegCanceled {
		writeChatopsErr(w, http.StatusConflict, "canceled", msg)
		return
	}
	writeChatopsErr(w, http.StatusBadGateway, "register_failed", msg)
}

// abort 在作业还在等扫码时把它结束在 state，并取消它。
func (r *feishuRegistrar) abort(j *feishuReg, state, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j.state == feishuRegPending {
		r.endLocked(j, state, msg)
		j.cancel()
	}
}

func (r *feishuRegistrar) endLocked(j *feishuReg, state, msg string) {
	j.state, j.message, j.endedAt = state, msg, r.now()
}

// lookupLocked 找编号对得上的当前这一个；终态留够 keep 的清掉。调用方持有 mu。
func (r *feishuRegistrar) lookupLocked(id string) *feishuReg {
	if j := r.cur; j != nil && !j.endedAt.IsZero() && r.now().Sub(j.endedAt) > r.keep {
		r.cur = nil
	}
	if r.cur == nil || id == "" || r.cur.id != id {
		return nil
	}
	return r.cur
}

// status 是 GET /api/chatops/feishu-app/register/{id}。
func (r *feishuRegistrar) status(w http.ResponseWriter, req *http.Request) {
	if !r.ready(w) {
		return
	}
	r.mu.Lock()
	j := r.lookupLocked(req.PathValue("id"))
	var out feishuRegisterStatus
	if j != nil {
		out = j.view()
	}
	r.mu.Unlock()
	if j == nil {
		writeChatopsErr(w, http.StatusNotFound, "not_found", msgFeishuRegGone)
		return
	}
	writeJSON(w, out)
}

// cancelOne 是 POST /api/chatops/feishu-app/register/{id}/cancel：只在 pending 时起作用。
func (r *feishuRegistrar) cancelOne(w http.ResponseWriter, req *http.Request) {
	if !r.ready(w) {
		return
	}
	r.mu.Lock()
	j := r.lookupLocked(req.PathValue("id"))
	var out feishuRegisterStatus
	if j != nil {
		if j.state == feishuRegPending {
			r.endLocked(j, feishuRegCanceled, "已取消")
			j.cancel()
		}
		out = j.view()
	}
	r.mu.Unlock()
	if j == nil {
		writeChatopsErr(w, http.StatusNotFound, "not_found", msgFeishuRegGone)
		return
	}
	writeJSON(w, out)
}

// runJob 是一次一键创建的后台协程：等飞书的结果；拿到凭据（飞书那边应用已经建好）以后建通道并绑定。
// 链接只往 qr 送一次（SDK 本来也只给一次）；没给链接就结束时，先写好状态再关上 qr，开始接口按状态回应。
func (r *feishuRegistrar) runJob(ctx context.Context, j *feishuReg, qr chan<- feishuRegQR) {
	defer r.jobs.Done()
	defer j.cancel()

	var once sync.Once
	noLink := func() { once.Do(func() { close(qr) }) }
	a, err := r.run(ctx, func(url string, expireIn int) {
		once.Do(func() { qr <- feishuRegQR{url: url, expireIn: expireIn} })
	})
	if err != nil {
		r.mu.Lock()
		if j.state == feishuRegPending {
			state, msg := feishuRegOutcome(err)
			r.endLocked(j, state, msg)
		}
		state, msg := j.state, j.message
		r.mu.Unlock()
		noLink()
		j.logf(false, "结束 id=%s state=%s message=%q", shortRegID(j.id), state, msg)
		return
	}

	// 应用已经建在飞书了：之后的取消与「被新的取代」都不再影响它，丢掉凭据反而留下一个没人管的应用
	r.mu.Lock()
	was, current := j.state, r.cur == j
	j.state, j.message, j.endedAt, j.appID = feishuRegSaving, "", time.Time{}, a.AppID
	r.mu.Unlock()
	noLink()
	if was != feishuRegPending || !current {
		j.logf(false, "id=%s 已经不是在等的那一个，但飞书已经建好应用（App ID %s），照常建完通道", shortRegID(j.id), a.AppID)
	}

	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), r.saveTimeout)
	defer cancelSave()
	state, msg, confID := r.save(saveCtx, j, a)
	*a = feishuapp.RegisteredApp{}

	r.mu.Lock()
	r.endLocked(j, state, msg)
	j.confID = confID
	r.mu.Unlock()
	j.logf(state != feishuRegDone, "结束 id=%s state=%s app_id=%s conf_id=%d message=%q", shortRegID(j.id), state, j.appID, confID, msg)
}

// save 建通道并把扫码人直接绑定，返回终态、提示与通道编号。
func (r *feishuRegistrar) save(ctx context.Context, j *feishuReg, a *feishuapp.RegisteredApp) (state, msg string, confID uint) {
	domain := feishuapp.DomainFeishu
	if a.Brand == feishuapp.DomainLark {
		domain = feishuapp.DomainLark
	}
	openID := strings.TrimSpace(a.OpenID)
	admins := []string{}
	if strings.HasPrefix(openID, "ou_") {
		admins = []string{openID}
	}
	// 只有字符串与字符串数组，编码不会失败
	raw, _ := json.Marshal(map[string]any{
		"app_id": a.AppID, "app_secret": a.AppSecret, "domain": domain,
		"admin_users": admins, "allowed_users": []string{}, "default_chat_id": "", "proxy_url": "",
	})
	conf, err := r.create(ctx, app.CreateConfReq{ChannelType: feishuapp.ChannelType, Name: j.name, ConfigJSON: raw, Enabled: true})
	if err != nil {
		return feishuRegFailed, fmt.Sprintf("飞书应用已经建好（App ID %s），但保存通道失败：%s；请在开发者后台复制 App Secret 手动新建通道", a.AppID, err.Error()), 0
	}
	if len(admins) == 0 {
		return feishuRegDone, "通道已经建好，但飞书没有给出扫码人的 open_id，管理员没有填：请拿到 open_id 填进通道详情的「管理员 open_id」，再到绑定页绑定", conf.ID
	}
	_, berr := r.bind(ctx, conf.ID, openID)
	if r.audit != nil {
		e := app.AuditEntry{
			NotificationConfID: conf.ID, ChannelType: feishuapp.ChannelType, ChannelUserID: openID,
			Command: "bind", Args: map[string]any{"via": "feishu_register"}, Result: bindAuditResult(berr),
		}
		if aerr := r.audit(ctx, e); aerr != nil {
			j.logf(true, "id=%s 记审计失败: %v", shortRegID(j.id), aerr)
		}
	}
	if berr != nil {
		return feishuRegDone, "通道已经建好，绑定失败：" + berr.Error() + "，请在绑定页手动绑定", conf.ID
	}
	return feishuRegDone, "", conf.ID
}

// feishuRegOutcome 按一键创建返回的错误定终态与原因。
func feishuRegOutcome(err error) (state, msg string) {
	switch {
	case errors.Is(err, feishuapp.ErrRegisterDenied):
		return feishuRegDenied, err.Error()
	case errors.Is(err, feishuapp.ErrRegisterExpired):
		return feishuRegExpired, err.Error()
	case errors.Is(err, context.Canceled):
		return feishuRegCanceled, "已取消"
	default:
		return feishuRegFailed, err.Error() + "。" + msgFeishuRegManual
	}
}

// newFeishuRegID 是 16 字节的随机编号（hex）。crypto/rand.Read 从 Go 1.24 起不返回错误。
func newFeishuRegID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// shortRegID 是日志里写的编号前 8 位：够对上同一次的开始与结束。
func shortRegID(id string) string { return id[:8] }

// regLogger 取开始这一刻的日志器（后台协程不再读全局变量）。日志写状态、App ID 与通道编号，
// 不写链接（带一次性的 user_code）与 App Secret。
func regLogger() func(warn bool, format string, args ...any) {
	l := global.GetSloggerSafe()
	return func(warn bool, format string, args ...any) {
		if l == nil {
			return
		}
		if warn {
			l.Warnf("[飞书一键创建] "+format, args...)
			return
		}
		l.Infof("[飞书一键创建] "+format, args...)
	}
}
