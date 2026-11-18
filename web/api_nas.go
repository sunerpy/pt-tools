package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/nas"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

// 「NAS 监控」（首页 NAS 概览 H2）：来源的列表、保存、测试连接与重新连接，本机来源的开关与首页显示哪个来源。只认会话；
// App API 与配置网关都读改不到（见 configExcluded）。
//
// 列表只读采集器的内存，不对外发请求。测试连接是用户主动的一次登录：证书不受信任时在发出凭据以前就断开，把证书回给用户确认；
// 登录产生的新长期凭据（群晖的设备令牌、飞牛的 token）只放进内存暂存表，回一个 pending，保存时凭它取出、加密写库。
// 库里的密钥、长期凭据与钉住的证书只给原来那组类型、地址与账号用：改了其中任何一项，测试与保存都不再带它们。

const (
	// nasTestTimeout 是一次测试连接最多等多久
	nasTestTimeout = 20 * time.Second
	// nasRetryTimeout 是「重新连接」最多等多久（采集器单次 10 秒，再留出等上一次采集结束的时间）
	nasRetryTimeout = 30 * time.Second
	// nasReloadTimeout 是保存以后采集器重新读设置最多等多久
	nasReloadTimeout = 10 * time.Second
	// nasTestInterval 是同一个来源两次测试之间至少隔多久（群晖、飞牛按登录失败的次数封号）
	nasTestInterval = 10 * time.Second
	// nasMaxSources 是最多接入几个来源（每个启用的来源一个采集循环）
	nasMaxSources = 20
	// nasMaxSecret 是密钥最长多少字节
	nasMaxSecret = 4096
	// nasMaxName、nasMaxUsername 是名字与用户名最多几个字（同 nas_sources 的列宽）
	nasMaxName     = 64
	nasMaxUsername = 128
)

// 测试连接的结果（nasTestView.Status）。
const (
	nasTestOK          = "ok"
	nasTestCertificate = "certificate"
	nasTestOTP         = "otp_required"
	nasTestAuthFailed  = "auth_failed"
	nasTestError       = "error"
)

// errNASNeedSecret 是改了类型、地址或账号以后没有填新密钥：库里的密钥只给原来那组目标用。
var errNASNeedSecret = nasBad("改了地址或账号，要重新填密钥")

// errNASNotFound 是没有这个来源。
var errNASNotFound = errors.New("没有这个来源")

// nasBadRequest 是请求本身不对：回 400 与原因。
type nasBadRequest struct{ msg string }

func (e *nasBadRequest) Error() string { return e.msg }

func nasBad(msg string) error { return &nasBadRequest{msg: msg} }

// nasView 是 GET /api/nas：首页来源的设置（home_source）与按它选出的来源（home，空表示没有可显示的），本机来源的开关，
// 这个版本能接入的来源类型，以及各来源的状态、最新数据与历史。
type nasView struct {
	HomeSource     string          `json:"home_source"`
	Home           string          `json:"home"`
	LocalEnabled   bool            `json:"local_enabled"`
	LocalSupported bool            `json:"local_supported"`
	Kinds          []nasKindView   `json:"kinds"`
	Sources        []nasSourceView `json:"sources"`
}

// nasKindView 是一种能接入的来源类型：Username 是用户名一栏（none、optional、required），Secret 是要不要密钥
// （required：总要；with_username：填了用户名才要；none：不要），Plain 是 http:// 地址的规则（never、without_credentials、with_ack）。
// Verified 是实验性的类型在哪个版本的真机上验证过，没验证过时不带。
type nasKindView struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Experimental bool   `json:"experimental"`
	Verified     string `json:"verified,omitempty"`
	Username     string `json:"username"`
	Secret       string `json:"secret"`
	Plain        string `json:"plain"`
}

// nasSourceView 是一个来源（本机或数据库里的来源）现在的情况：ID 是 local 或数字编号；不回密钥，只回有没有。
type nasSourceView struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Kind         string        `json:"kind"`
	Enabled      bool          `json:"enabled"`
	Experimental bool          `json:"experimental"`
	URL          string        `json:"url,omitempty"`
	HasSecret    bool          `json:"has_secret"`
	TLSPinSHA256 string        `json:"tls_pin_sha256,omitempty"`
	InsecureAck  bool          `json:"insecure_ack"`
	State        string        `json:"state"`
	Message      string        `json:"message,omitempty"`
	LastOKAt     *time.Time    `json:"last_ok_at,omitempty"`
	NextTryAt    *time.Time    `json:"next_try_at,omitempty"`
	Snapshot     *nas.Snapshot `json:"snapshot"`
	History      []nas.Sample  `json:"history"`
}

// nasSourceEdit 是编辑一个来源用的字段（GET /api/nas/sources/{id} 与保存的回应）：不回密钥与长期凭据，只回有没有。
type nasSourceEdit struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Enabled      bool   `json:"enabled"`
	URL          string `json:"url"`
	Username     string `json:"username"`
	HasSecret    bool   `json:"has_secret"`
	HasExtra     bool   `json:"has_extra"`
	TLSPinSHA256 string `json:"tls_pin_sha256"`
	InsecureAck  bool   `json:"insecure_ack"`
}

// nasSourceInput 是保存一个来源（POST /api/nas/sources、PUT /api/nas/sources/{id}）：Secret 为空表示不改（只在类型、地址、
// 账号都没变时）；ClearSecret 清掉库里的密钥与长期凭据；Pending 是测试连接回的暂存凭据；InsecureAck 是用户这一次确认了明文风险。
type nasSourceInput struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Enabled      bool   `json:"enabled"`
	URL          string `json:"url"`
	Username     string `json:"username"`
	Secret       string `json:"secret"`
	ClearSecret  bool   `json:"clear_secret"`
	TLSPinSHA256 string `json:"tls_pin_sha256"`
	InsecureAck  bool   `json:"insecure_ack"`
	Pending      string `json:"pending"`
}

// nasTestInput 是一次测试连接（POST /api/nas/sources/test）：ID 是编辑中的来源（新建时为 0），TrustSHA256 是用户这一次确认信任的证书指纹。
type nasTestInput struct {
	ID          uint   `json:"id"`
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	Username    string `json:"username"`
	Secret      string `json:"secret"`
	ClearSecret bool   `json:"clear_secret"`
	OTP         string `json:"otp"`
	TrustSHA256 string `json:"trust_sha256"`
	InsecureAck bool   `json:"insecure_ack"`
}

// nasTestView 是测试连接的结果（状态见 nasTest* 常量）：连上时有 System，证书不受信任时有 Certificate；
// 登录产生了新的长期凭据时有 Pending（10 分钟内保存时带上）。
type nasTestView struct {
	Status      string        `json:"status"`
	System      *nas.System   `json:"system,omitempty"`
	Certificate *nas.CertInfo `json:"certificate,omitempty"`
	Message     string        `json:"message,omitempty"`
	Pending     string        `json:"pending,omitempty"`
}

// nasSettingsInput 是 PUT /api/nas/settings：没给的项不改。
type nasSettingsInput struct {
	LocalEnabled *bool   `json:"local_enabled"`
	HomeSource   *string `json:"home_source"`
}

// nasTestLimiter 让同一个来源 nasTestInterval 内只测一次。只在测试真的走到了登录这一步时算数：
// 证书不受信任的那一次在发出凭据以前就断开了，不算（用户点「信任这张证书」以后马上再测）。
type nasTestLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
	// now 是现在的时间（测试里换掉）
	now func() time.Time
}

func (l *nasTestLimiter) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// allow 记下 key 这一次测试；距上一次不到 nasTestInterval 时返回 false。
func (l *nasTestLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	for k, at := range l.last {
		if now.Sub(at) >= nasTestInterval {
			delete(l.last, k)
		}
	}
	if _, ok := l.last[key]; ok {
		return false
	}
	l.last[key] = now
	return true
}

// release 撤掉 key 这一次的记录（这次测试没有发出凭据）。
func (l *nasTestLimiter) release(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.last, key)
}

// SetNASCollector 接上「NAS 监控」的采集器（cmd 建好并启动它，关闭流程里停它）。
func (s *Server) SetNASCollector(c *nas.Collector) { s.nasCollector = c }

// registerNASRoutes 挂上「NAS 监控」的接口。
func (s *Server) registerNASRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nas", s.auth(s.apiNAS))
	mux.HandleFunc("POST /api/nas/sources", s.auth(s.apiNASSourceCreate))
	mux.HandleFunc("POST /api/nas/sources/test", s.auth(s.apiNASSourceTest))
	mux.HandleFunc("GET /api/nas/sources/{id}", s.auth(s.apiNASSourceGet))
	mux.HandleFunc("PUT /api/nas/sources/{id}", s.auth(s.apiNASSourceUpdate))
	mux.HandleFunc("DELETE /api/nas/sources/{id}", s.auth(s.apiNASSourceDelete))
	mux.HandleFunc("POST /api/nas/sources/{id}/retry", s.auth(s.apiNASSourceRetry))
	mux.HandleFunc("PUT /api/nas/settings", s.auth(s.apiNASSettings))
}

// nasReady 是库与采集器都在：不在时回 503。
func (s *Server) nasReady(w http.ResponseWriter) (*gorm.DB, *nas.Collector, bool) {
	db, ok := s.healthDB(w)
	if !ok {
		return nil, nil, false
	}
	if s.nasCollector == nil {
		http.Error(w, "NAS 监控没有启动", http.StatusServiceUnavailable)
		return nil, nil, false
	}
	return db, s.nasCollector, true
}

func writeNASError(w http.ResponseWriter, err error) {
	var bad *nasBadRequest
	switch {
	case errors.As(err, &bad):
		http.Error(w, bad.msg, http.StatusBadRequest)
	case errors.Is(err, nas.ErrPendingInvalid):
		http.Error(w, nas.ErrPendingInvalid.Error(), http.StatusBadRequest)
	case errors.Is(err, errNASNotFound), errors.Is(err, nas.ErrNotFound):
		http.Error(w, errNASNotFound.Error(), http.StatusNotFound)
	default:
		http.Error(w, "NAS 监控出错: "+redact.Secrets(err.Error()), http.StatusInternalServerError)
	}
}

// apiNAS handles GET /api/nas
func (s *Server) apiNAS(w http.ResponseWriter, r *http.Request) {
	db, c, ok := s.nasReady(w)
	if !ok {
		return
	}
	view, err := nasViewOf(r.Context(), db, c)
	if err != nil {
		writeNASError(w, err)
		return
	}
	writeJSON(w, view)
}

func nasViewOf(ctx context.Context, db *gorm.DB, c *nas.Collector) (*nasView, error) {
	set, err := models.GetNasSetting(db.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("读取 NAS 监控设置失败: %w", err)
	}
	list := c.List()
	out := &nasView{
		HomeSource: set.HomeSource, Home: nasHome(set.HomeSource, list), LocalEnabled: set.LocalEnabled,
		LocalSupported: nas.LocalSupported, Kinds: nasKindViews(), Sources: make([]nasSourceView, 0, len(list)),
	}
	for _, st := range list {
		out.Sources = append(out.Sources, nasStatusView(st))
	}
	return out, nil
}

// nasUsable 是这个来源能在首页显示：启用了，而且这个版本、这个平台能用。
func nasUsable(st nas.Status) bool { return st.Enabled && st.State != nas.StateUnsupported }

// nasHome 是首页显示哪个来源：设置里选的那个（还在、能用）；没选或选的那个用不了时是第一个能用的来源，
// 没有时是本机来源；都没有时是空（首页显示「接入 NAS」）。
func nasHome(setting string, list []nas.Status) string {
	for _, st := range list {
		if setting != "" && st.ID == setting && nasUsable(st) {
			return st.ID
		}
	}
	for _, st := range list {
		if st.ID != nas.LocalID && nasUsable(st) {
			return st.ID
		}
	}
	for _, st := range list {
		if st.ID == nas.LocalID && nasUsable(st) {
			return st.ID
		}
	}
	return ""
}

func nasKindViews() []nasKindView {
	kinds := nas.Kinds()
	out := make([]nasKindView, 0, len(kinds))
	for _, k := range kinds {
		username := k.Username
		if username == nas.UsernameNone {
			username = "none"
		}
		out = append(out, nasKindView{
			Name: k.Name, Label: k.Label, Experimental: k.Experimental, Verified: k.Verified, Username: username,
			Secret: nasSecretRule(k), Plain: nasPlainRule(k.Plain),
		})
	}
	return out
}

// nasSecretRule 是表单上密钥一栏的规则：按「不填用户名」与「填了用户名」两种情况问这种来源要不要密钥。
func nasSecretRule(k nas.Kind) string {
	without, with := k.RequiresSecret(""), k.RequiresSecret("user")
	switch {
	case !without && with:
		return "with_username"
	case !without && !with:
		return "none"
	default:
		return "required"
	}
}

func nasPlainRule(p nas.PlainRule) string {
	switch p {
	case nas.PlainWithoutCredentials:
		return "without_credentials"
	case nas.PlainWithAck:
		return "with_ack"
	default:
		return "never"
	}
}

func nasStatusView(st nas.Status) nasSourceView {
	v := nasSourceView{
		ID: st.ID, Name: st.Name, Kind: st.Kind, Enabled: st.Enabled, Experimental: st.Experimental, URL: st.URL,
		HasSecret: st.HasSecret, TLSPinSHA256: st.PinSHA256, InsecureAck: st.InsecureAck, State: st.State, Message: st.Message,
		LastOKAt: st.LastOKAt, NextTryAt: st.NextTryAt, Snapshot: st.Snapshot, History: st.History,
	}
	if v.History == nil {
		v.History = []nas.Sample{}
	}
	return v
}

func nasEditView(row models.NasSource) nasSourceEdit {
	return nasSourceEdit{
		ID: row.ID, Name: row.Name, Kind: row.Kind, Enabled: row.Enabled, URL: row.URL, Username: row.Username,
		HasSecret: row.SecretEncrypted != "", HasExtra: row.ExtraEncrypted != "", TLSPinSHA256: row.TLSPinSHA256,
		InsecureAck: row.InsecureAck,
	}
}

// nasRow 读一个来源；没有时返回 errNASNotFound。
func nasRow(db *gorm.DB, id uint) (models.NasSource, error) {
	var row models.NasSource
	if err := db.Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return models.NasSource{}, err
	}
	if row.ID == 0 {
		return models.NasSource{}, errNASNotFound
	}
	return row, nil
}

// nasTarget 校验并规范化类型、地址与用户名（保存与测试共用）。
func nasTarget(kind, raw, username string) (nas.Kind, *url.URL, string, error) {
	k, ok := nas.LookupKind(strings.TrimSpace(kind))
	if !ok {
		return nas.Kind{}, nil, "", nasBad(fmt.Sprintf("不支持的来源类型 %q", kind))
	}
	u, err := nas.ParseURL(k.Name, raw)
	if err != nil {
		return nas.Kind{}, nil, "", nasBad(err.Error())
	}
	username = strings.TrimSpace(username)
	switch {
	case k.Username == nas.UsernameNone && username != "":
		return nas.Kind{}, nil, "", nasBad(k.Label + " 不用填用户名")
	case k.Username == nas.UsernameRequired && username == "":
		return nas.Kind{}, nil, "", nasBad("要填用户名")
	case utf8.RuneCountInString(username) > nasMaxUsername:
		return nas.Kind{}, nil, "", nasBad(fmt.Sprintf("用户名最多 %d 个字", nasMaxUsername))
	}
	return k, u, username, nil
}

// nasSameTarget 是这一行的类型、地址（按现在的规则规范化）与账号都和 kind、target、username 一样：
// 只有这时库里的密钥、长期凭据与钉住的证书才能接着用。
func nasSameTarget(row models.NasSource, kind, target, username string) bool {
	if row.Kind != kind || row.Username != username {
		return false
	}
	u, err := nas.ParseURL(row.Kind, row.URL)
	return err == nil && u.String() == target
}

func (s *Server) apiNASSourceGet(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.nasReady(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	row, err := nasRow(db.WithContext(r.Context()), id)
	if err != nil {
		writeNASError(w, err)
		return
	}
	writeJSON(w, nasEditView(row))
}

func (s *Server) apiNASSourceCreate(w http.ResponseWriter, r *http.Request) { s.nasSave(w, r, 0) }

func (s *Server) apiNASSourceUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.nasSave(w, r, id)
}

// nasSave 新建（id 为 0）或修改一个来源，提交以后让采集器只重启这一个来源。
func (s *Server) nasSave(w http.ResponseWriter, r *http.Request, id uint) {
	db, c, ok := s.nasReady(w)
	if !ok {
		return
	}
	var in nasSourceInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	s.nasMu.Lock()
	row, err := s.saveNASSource(r.Context(), db, id, in)
	s.nasMu.Unlock()
	if err != nil {
		writeNASError(w, err)
		return
	}
	s.nasReload(r.Context(), c, strconv.FormatUint(uint64(row.ID), 10))
	writeJSON(w, nasEditView(row))
}

// nasReload 让采集器重新读设置、重启 id 这一个来源；请求断开也照样做完。失败只记日志（设置已经存好，下次启动会读到）。
func (s *Server) nasReload(ctx context.Context, c *nas.Collector, id string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), nasReloadTimeout)
	defer cancel()
	if err := c.Reload(ctx, id); err != nil {
		global.GetSlogger().Warnf("NAS 监控重新读取来源 %s 的设置失败: %v", id, redact.Secrets(err.Error()))
	}
}

// saveNASSource 在一个事务里写这一行（3.2.4）。库里的密钥、长期凭据只在类型、地址与账号都没变时留着；
// 带 pending 时校验它绑定的正是这组新值，事务提交以后才删掉它。
func (s *Server) saveNASSource(ctx context.Context, db *gorm.DB, id uint, in nasSourceInput) (models.NasSource, error) {
	k, u, username, err := nasTarget(in.Kind, in.URL, in.Username)
	if err != nil {
		return models.NasSource{}, err
	}
	name := strings.TrimSpace(in.Name)
	pin := nas.NormalizePin(in.TLSPinSHA256)
	switch {
	case name == "":
		return models.NasSource{}, nasBad("要填名字")
	case utf8.RuneCountInString(name) > nasMaxName:
		return models.NasSource{}, nasBad(fmt.Sprintf("名字最多 %d 个字", nasMaxName))
	case len(in.Secret) > nasMaxSecret:
		return models.NasSource{}, nasBad("密钥太长了")
	case in.ClearSecret && (in.Secret != "" || in.Pending != ""):
		return models.NasSource{}, nasBad("「清除密钥」时不要同时填新密钥或带测试结果")
	case !nas.ValidPin(pin):
		return models.NasSource{}, nasBad("证书指纹要是 64 位十六进制（SHA-256）")
	}
	target := u.String()
	var fresh map[string]string
	var done func(committed bool)
	if in.Pending != "" {
		fresh, done, err = s.nasPending.Claim(in.Pending, nas.PendingKey{Kind: k.Name, URL: target, Username: username, ID: id})
		if err != nil {
			return models.NasSource{}, err
		}
	}
	committed := false
	defer func() {
		if done != nil {
			done(committed)
		}
	}()
	var row models.NasSource
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id != 0 {
			old, rerr := nasRow(tx, id)
			if rerr != nil {
				return rerr
			}
			row = old
		} else {
			var n int64
			if cerr := tx.Model(&models.NasSource{}).Count(&n).Error; cerr != nil {
				return cerr
			}
			if n >= nasMaxSources {
				return nasBad(fmt.Sprintf("最多接入 %d 个来源", nasMaxSources))
			}
		}
		same := id != 0 && nasSameTarget(row, k.Name, target, username)
		secretEnc, extraEnc := "", ""
		if same && !in.ClearSecret {
			secretEnc, extraEnc = row.SecretEncrypted, row.ExtraEncrypted
		}
		if in.Secret != "" {
			enc, eerr := s.store.EncryptCookie(in.Secret)
			if eerr != nil {
				return fmt.Errorf("加密密钥失败: %w", eerr)
			}
			secretEnc = enc
		}
		if fresh != nil {
			enc, eerr := s.encryptNASExtra(fresh)
			if eerr != nil {
				return eerr
			}
			extraEnc = enc
		}
		if secretEnc == "" && k.RequiresSecret(username) {
			if id != 0 && !same && !in.ClearSecret {
				return errNASNeedSecret
			}
			if in.Enabled {
				return nasBad("要填密钥（API Key 或密码）；暂时不填的话先存成停用")
			}
		}
		sends := secretEnc != "" || extraEnc != "" || username != ""
		if terr := nas.CheckTransport(k, u, sends, in.InsecureAck); terr != nil {
			return nasBad(terr.Error())
		}
		row.Name, row.Kind, row.Enabled, row.URL, row.Username = name, k.Name, in.Enabled, target, username
		row.SecretEncrypted, row.ExtraEncrypted, row.TLSPinSHA256 = secretEnc, extraEnc, pin
		row.InsecureAck = in.InsecureAck && u.Scheme == "http"
		row.Revision++
		return tx.Save(&row).Error
	})
	if err != nil {
		return models.NasSource{}, err
	}
	committed = true
	return row, nil
}

// encryptNASExtra 把长期凭据加密成 extra_encrypted 存的样子（格式见 nas.EncodeExtra；空的存成空串）。
func (s *Server) encryptNASExtra(extra map[string]string) (string, error) {
	plain, err := nas.EncodeExtra(extra)
	if err != nil || plain == "" {
		return "", err
	}
	enc, err := s.store.EncryptCookie(plain)
	if err != nil {
		return "", fmt.Errorf("加密登录凭据失败: %w", err)
	}
	return enc, nil
}

// apiNASSourceDelete handles DELETE /api/nas/sources/{id}：删这一行（首页正显示它时改回自动）、停掉它的采集、丢掉绑定它的暂存凭据。
func (s *Server) apiNASSourceDelete(w http.ResponseWriter, r *http.Request) {
	db, c, ok := s.nasReady(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	key := strconv.FormatUint(uint64(id), 10)
	s.nasMu.Lock()
	err := db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ?", id).Delete(&models.NasSource{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNASNotFound
		}
		set, serr := models.GetNasSetting(tx)
		if serr != nil {
			return serr
		}
		if set.HomeSource != key {
			return nil
		}
		set.HomeSource = ""
		return models.SaveNasSetting(tx, set)
	})
	s.nasMu.Unlock()
	if err != nil {
		writeNASError(w, err)
		return
	}
	s.nasPending.DropID(id)
	s.nasReload(r.Context(), c, key)
	writeJSON(w, map[string]any{"ok": true})
}

// nasTestPlan 是校验过的一次测试：发给哪种来源、带哪些凭据，以及登录产生的新凭据绑定到哪组目标。
type nasTestPlan struct {
	kind  nas.Kind
	key   nas.PendingKey
	input nas.TestInput
}

// limitKey 是限速用的键：编辑中的来源按编号，新建的按类型、规范化后的地址与账号。
func (p *nasTestPlan) limitKey() string {
	if p.key.ID != 0 {
		return "id:" + strconv.FormatUint(uint64(p.key.ID), 10)
	}
	return "new:" + p.key.Kind + "\x00" + p.key.URL + "\x00" + p.key.Username
}

// apiNASSourceTest handles POST /api/nas/sources/test（3.2.4）。
func (s *Server) apiNASSourceTest(w http.ResponseWriter, r *http.Request) {
	db, _, ok := s.nasReady(w)
	if !ok {
		return
	}
	var in nasTestInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	plan, err := s.nasTestPlanOf(r.Context(), db, in)
	if err != nil {
		writeNASError(w, err)
		return
	}
	limit := plan.limitKey()
	if !s.nasTests.allow(limit) {
		http.Error(w, "刚测过，过几秒再试", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nasTestTimeout)
	defer cancel()
	res, err := plan.kind.Test(ctx, plan.input)
	view := nasTestResultView(res, err, plan.input.Secret, plan.input.OTP)
	if view.Status == nasTestCertificate {
		s.nasTests.release(limit)
	}
	if view.Status == nasTestOK && res != nil && len(res.Extra) > 0 {
		token, perr := s.nasPending.Put(plan.key, res.Extra)
		if perr != nil {
			writeNASError(w, perr)
			return
		}
		view.Pending = token
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, view)
}

// nasTestPlanOf 校验一次测试并决定带哪些凭据：带 id 且类型、地址与账号都没变（也没要清除）时，没填的密钥、长期凭据与
// 没给的证书指纹用库里的；否则一概不用库里的，要密钥的来源没填新密钥时回 400、不连目标。
func (s *Server) nasTestPlanOf(ctx context.Context, db *gorm.DB, in nasTestInput) (*nasTestPlan, error) {
	k, u, username, err := nasTarget(in.Kind, in.URL, in.Username)
	if err != nil {
		return nil, err
	}
	pin := nas.NormalizePin(in.TrustSHA256)
	switch {
	case len(in.Secret) > nasMaxSecret:
		return nil, nasBad("密钥太长了")
	case in.ClearSecret && in.Secret != "":
		return nil, nasBad("「清除密钥」时不要同时填新密钥")
	case !nas.ValidPin(pin):
		return nil, nasBad("证书指纹要是 64 位十六进制（SHA-256）")
	}
	target := u.String()
	ti := nas.TestInput{URL: u, Username: username, Secret: in.Secret, OTP: strings.TrimSpace(in.OTP), PinSHA256: pin}
	if in.ID != 0 {
		row, rerr := nasRow(db.WithContext(ctx), in.ID)
		if rerr != nil {
			return nil, rerr
		}
		same := nasSameTarget(row, k.Name, target, username)
		switch {
		case same && !in.ClearSecret:
			if err = s.fillNASTestFromRow(&ti, row); err != nil {
				return nil, err
			}
		case !in.ClearSecret && ti.Secret == "" && k.RequiresSecret(username):
			return nil, errNASNeedSecret
		}
	}
	if ti.Secret == "" && k.RequiresSecret(username) {
		return nil, nasBad("要填密钥（API Key 或密码）")
	}
	sends := ti.Secret != "" || len(ti.Extra) > 0 || username != "" || ti.OTP != ""
	if err = nas.CheckTransport(k, u, sends, in.InsecureAck); err != nil {
		return nil, nasBad(err.Error())
	}
	return &nasTestPlan{kind: k, key: nas.PendingKey{Kind: k.Name, URL: target, Username: username, ID: in.ID}, input: ti}, nil
}

// fillNASTestFromRow 用库里这一行补上测试没填的密钥、长期凭据与证书指纹（只在类型、地址与账号都没变时调用）。
// 长期凭据解不开时不带（来源会重新登录）；密钥解不开时要用户重新填。
func (s *Server) fillNASTestFromRow(ti *nas.TestInput, row models.NasSource) error {
	if ti.Secret == "" && row.SecretEncrypted != "" {
		plain, err := s.store.DecryptCookie(row.SecretEncrypted)
		if err != nil {
			return nasBad("保存的密钥解不开（加密密钥换了？），请重新填写")
		}
		ti.Secret = plain
	}
	if row.ExtraEncrypted != "" {
		if plain, err := s.store.DecryptCookie(row.ExtraEncrypted); err == nil {
			if extra, derr := nas.DecodeExtra(plain); derr == nil {
				ti.Extra = extra
			}
		}
	}
	if ti.PinSHA256 == "" {
		ti.PinSHA256 = row.TLSPinSHA256
	}
	return nil
}

// nasTestResultView 把一次测试的结果写成回应；原因去掉凭证与这一次用的密钥、验证码。
func nasTestResultView(res *nas.TestResult, err error, secrets ...string) nasTestView {
	if err == nil {
		if res == nil {
			return nasTestView{Status: nasTestError, Message: "来源没有返回结果"}
		}
		sys := res.System
		return nasTestView{Status: nasTestOK, System: &sys}
	}
	var ce *nas.CertError
	var ae *nas.AuthError
	switch {
	case errors.As(err, &ce):
		cert := ce.Cert
		return nasTestView{Status: nasTestCertificate, Certificate: &cert, Message: ce.Error()}
	case errors.As(err, &ae) && ae.OTP:
		// 弹窗在原因上面写「要验证码」：原因不再带「认证失败：」
		msg := ae.Reason
		if msg == "" {
			msg = ae.Error()
		}
		return nasTestView{Status: nasTestOTP, Message: nasScrub(msg, secrets...)}
	case errors.As(err, &ae):
		return nasTestView{Status: nasTestAuthFailed, Message: nasScrub(ae.Error(), secrets...)}
	case errors.Is(err, context.DeadlineExceeded):
		return nasTestView{Status: nasTestError, Message: fmt.Sprintf("%d 秒内没有回应", int(nasTestTimeout/time.Second))}
	default:
		return nasTestView{Status: nasTestError, Message: nasScrub(err.Error(), secrets...)}
	}
}

// nasScrub 去掉原因里的凭证：先把这一次用的密钥、验证码原文换成 ***（短于 4 个字的不换，免得误伤普通文字），再按 redact.Secrets 的规则遮。
func nasScrub(msg string, secrets ...string) string {
	for _, sec := range secrets {
		if len(sec) >= 4 {
			msg = strings.ReplaceAll(msg, sec, "***")
		}
	}
	return redact.Secrets(msg)
}

// apiNASSourceRetry handles POST /api/nas/sources/{id}/retry：「重新连接」，只试一次；id 是 local 或来源的编号。
func (s *Server) apiNASSourceRetry(w http.ResponseWriter, r *http.Request) {
	_, c, ok := s.nasReady(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id != nas.LocalID {
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 {
			http.Error(w, "编号无效", http.StatusBadRequest)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), nasRetryTimeout)
	defer cancel()
	st, err := c.Retry(ctx, id)
	switch {
	case errors.Is(err, nas.ErrNotFound):
		writeNASError(w, err)
	case errors.Is(err, nas.ErrNoLoop):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, fmt.Sprintf("%d 秒内没有连完，稍后看列表里的状态", int(nasRetryTimeout/time.Second)), http.StatusGatewayTimeout)
	default:
		writeJSON(w, nasStatusView(st))
	}
}

// apiNASSettings handles PUT /api/nas/settings：本机来源的开关与首页显示哪个来源；回应同 GET /api/nas。
func (s *Server) apiNASSettings(w http.ResponseWriter, r *http.Request) {
	db, c, ok := s.nasReady(w)
	if !ok {
		return
	}
	var in nasSettingsInput
	if !decodeStrictBody(w, r, &in) {
		return
	}
	s.nasMu.Lock()
	localChanged, err := saveNASSettings(r.Context(), db, in)
	s.nasMu.Unlock()
	if err != nil {
		writeNASError(w, err)
		return
	}
	if localChanged {
		s.nasReload(r.Context(), c, nas.LocalID)
	}
	view, err := nasViewOf(r.Context(), db, c)
	if err != nil {
		writeNASError(w, err)
		return
	}
	writeJSON(w, view)
}

// saveNASSettings 写设置；返回本机来源的开关有没有变（变了要让采集器重启本机来源）。
func saveNASSettings(ctx context.Context, db *gorm.DB, in nasSettingsInput) (localChanged bool, err error) {
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		set, gerr := models.GetNasSetting(tx)
		if gerr != nil {
			return gerr
		}
		if in.HomeSource != nil {
			home, herr := nasHomeSetting(tx, *in.HomeSource)
			if herr != nil {
				return herr
			}
			set.HomeSource = home
		}
		if in.LocalEnabled != nil && *in.LocalEnabled != set.LocalEnabled {
			set.LocalEnabled, localChanged = *in.LocalEnabled, true
		}
		return models.SaveNasSetting(tx, set)
	})
	if err != nil {
		return false, err
	}
	return localChanged, nil
}

// nasHomeSetting 校验首页来源的设置：空（自动）、local，或一个存在的来源编号（规范成十进制）。
func nasHomeSetting(tx *gorm.DB, raw string) (string, error) {
	home := strings.TrimSpace(raw)
	if home == "" || home == nas.LocalID {
		return home, nil
	}
	n, err := strconv.ParseUint(home, 10, 64)
	if err != nil || n == 0 {
		return "", nasBad("首页来源要是 local 或来源的编号")
	}
	if _, err = nasRow(tx, uint(n)); err != nil {
		if errors.Is(err, errNASNotFound) {
			return "", nasBad("没有这个来源")
		}
		return "", err
	}
	return strconv.FormatUint(n, 10), nil
}

// homeNAS 是首页的 nas 块：首页显示的那个来源（同 GET /api/nas 的一项，没有时为空），与其余启用的来源的名字与状态。
type homeNAS struct {
	Source *nasSourceView `json:"source"`
	Others []homeNASOther `json:"others"`
}

// homeNASOther 是首页上没在显示的一个来源。
type homeNASOther struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	State string `json:"state"`
}

// homeNASBlock 算首页的 nas 块：只读采集器的内存与设置，不对外发请求。
func (s *Server) homeNASBlock(ctx context.Context, db *gorm.DB) (*homeNAS, error) {
	if s.nasCollector == nil {
		return nil, errors.New("NAS 监控没有启动")
	}
	set, err := models.GetNasSetting(db.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("读取 NAS 监控设置失败: %w", err)
	}
	list := s.nasCollector.List()
	home := nasHome(set.HomeSource, list)
	out := &homeNAS{Others: []homeNASOther{}}
	for _, st := range list {
		switch {
		case st.ID == home:
			v := nasStatusView(st)
			out.Source = &v
		case nasUsable(st):
			out.Others = append(out.Others, homeNASOther{ID: st.ID, Name: st.Name, Kind: st.Kind, State: st.State})
		}
	}
	return out, nil
}
