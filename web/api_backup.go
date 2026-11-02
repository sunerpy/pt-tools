package web

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/scheduler"
)

// 备份与恢复（全局设置 → 备份，媒体中心 M0）。只认网页会话（s.auth）：备份文件里有数据库与密钥，
// App API 与 MCP 的配置网关读改不到（见 configExcluded）。恢复只是放进待恢复，重启 pt-tools 以后才换上。

func (s *Server) registerBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/backups", s.auth(s.apiBackups))
	mux.HandleFunc("POST /api/backups", s.auth(s.apiBackupCreate))
	mux.HandleFunc("PUT /api/backups/settings", s.auth(s.apiBackupSettings))
	mux.HandleFunc("GET /api/backups/file/{name}", s.auth(s.apiBackupDownload))
	mux.HandleFunc("DELETE /api/backups/file/{name}", s.auth(s.apiBackupDelete))
	mux.HandleFunc("POST /api/backups/file/{name}/restore", s.auth(s.apiBackupRestore))
	mux.HandleFunc("POST /api/backups/upload", s.auth(s.apiBackupUpload))
	mux.HandleFunc("DELETE /api/backups/pending", s.auth(s.apiBackupCancelRestore))
}

// BackupSettingsView 是备份设置。Dir 为空表示用默认目录 DefaultDir。
type BackupSettingsView struct {
	Enabled       bool   `json:"enabled"`
	Time          string `json:"time"`
	Keep          int    `json:"keep"`
	Dir           string `json:"dir"`
	DefaultDir    string `json:"default_dir"`
	IncludeSecret bool   `json:"include_secret"`
	MinKeep       int    `json:"min_keep"`
	MaxKeep       int    `json:"max_keep"`
}

// BackupItemView 是备份目录里的一份备份。Error 非空表示读不出它的说明（坏文件），只能删掉。
type BackupItemView struct {
	Name          string     `json:"name"`
	Size          int64      `json:"size"`
	CreatedAt     *time.Time `json:"created_at"`
	Reason        string     `json:"reason"`
	ReasonLabel   string     `json:"reason_label"`
	AppVersion    string     `json:"app_version"`
	SchemaVersion int        `json:"schema_version"`
	HasSecret     bool       `json:"has_secret"`
	SecretSource  string     `json:"secret_source,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// BackupResultView 是最近一次备份的结果：Error 为空表示成功。
type BackupResultView struct {
	At          time.Time  `json:"at"`
	Reason      string     `json:"reason"`
	ReasonLabel string     `json:"reason_label"`
	Name        string     `json:"name"`
	Error       string     `json:"error,omitempty"`
	LastOKAt    *time.Time `json:"last_ok_at"`
}

// RestoreResultView 是最近一次启动时恢复的结果：Error 为空表示换上了，换下来的原文件在 Dir。
type RestoreResultView struct {
	At    time.Time `json:"at"`
	Name  string    `json:"name"`
	Dir   string    `json:"dir,omitempty"`
	Error string    `json:"error,omitempty"`
}

// PendingRestoreView 是放好了、重启以后生效的恢复。KeyAction：keep-env 仍用环境变量的密钥，
// use-backup 换成备份里的密钥，keep-file 不动 secret.key（NoCipher 时备份里没有密文，不需要密钥）。
type PendingRestoreView struct {
	Name          string    `json:"name"`
	StagedAt      time.Time `json:"staged_at"`
	CreatedAt     time.Time `json:"created_at"`
	Reason        string    `json:"reason"`
	ReasonLabel   string    `json:"reason_label"`
	AppVersion    string    `json:"app_version"`
	SchemaVersion int       `json:"schema_version"`
	KeyAction     string    `json:"key_action"`
	NoCipher      bool      `json:"no_cipher"`
}

// BackupsView 是备份页：设置、最近一次备份与恢复、下一次定时备份、备份列表与待恢复。
type BackupsView struct {
	Settings BackupSettingsView `json:"settings"`
	// Dir 是现在生效的备份目录
	Dir string `json:"dir"`
	// EnvKey 为真表示密钥来自环境变量 PT_TOOLS_SECRET_KEY（恢复时不动 secret.key）
	EnvKey bool `json:"env_key"`
	// NextAt 是下一次定时备份大约什么时候；关着时为 null
	NextAt  *time.Time          `json:"next_at"`
	Last    *BackupResultView   `json:"last"`
	Restore *RestoreResultView  `json:"restore"`
	Pending *PendingRestoreView `json:"pending"`
	Backups []BackupItemView    `json:"backups"`
	// ListError 是读不出备份目录的原因（设置照常返回）
	ListError string `json:"list_error,omitempty"`
}

// BackupSettingsRequest 是保存备份设置的请求。
type BackupSettingsRequest struct {
	Enabled       bool   `json:"enabled"`
	Time          string `json:"time"`
	Keep          int    `json:"keep"`
	Dir           string `json:"dir"`
	IncludeSecret bool   `json:"include_secret"`
}

func (s *Server) apiBackups(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	s.writeBackups(w)
}

func (s *Server) writeBackups(w http.ResponseWriter) {
	view, err := s.backupsView(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, view)
}

func (s *Server) backupsView(now time.Time) (BackupsView, error) {
	cfg, err := s.store.BackupSettings()
	if err != nil {
		return BackupsView{}, err
	}
	view := BackupsView{
		Settings: BackupSettingsView{
			Enabled: cfg.Enabled, Time: cfg.Time, Keep: cfg.Keep, Dir: cfg.Dir, DefaultDir: cfg.DefaultDir,
			IncludeSecret: cfg.IncludeSecret, MinKeep: backup.MinKeep, MaxKeep: backup.MaxKeep,
		},
		Dir:     cfg.BackupDir(),
		EnvKey:  core.ActiveBackupKey().FromEnv,
		Backups: []BackupItemView{},
	}
	sched := scheduler.BackupSchedule{Enabled: cfg.Enabled, Time: cfg.Time, Day: cfg.ScheduledDay}
	if next := scheduler.NextBackupAt(sched, now); !next.IsZero() {
		view.NextAt = &next
	}
	if l := cfg.Last; l != nil {
		view.Last = &BackupResultView{
			At: l.At, Reason: string(l.Reason), ReasonLabel: l.Reason.Label(), Name: l.Name, Error: l.Error, LastOKAt: l.LastOKAt,
		}
	}
	if r := cfg.Restore; r != nil {
		view.Restore = &RestoreResultView{At: r.At, Name: r.Name, Dir: r.Dir, Error: r.Error}
	}
	if p, perr := core.PendingRestore(); perr != nil {
		global.GetSlogger().Warnf("读取待恢复失败: %v", perr)
	} else if p != nil {
		view.Pending = pendingView(*p)
	}
	list, err := backup.List(view.Dir)
	if err != nil {
		view.ListError = err.Error()
		return view, nil
	}
	for _, b := range list {
		view.Backups = append(view.Backups, backupItemView(b))
	}
	return view, nil
}

func backupItemView(b backup.Info) BackupItemView {
	item := BackupItemView{Name: b.Name, Size: b.Size, Error: b.Err}
	if b.Err != "" {
		return item
	}
	m := b.Manifest
	created := m.CreatedAt
	item.CreatedAt = &created
	item.Reason, item.ReasonLabel = string(m.Reason), m.Reason.Label()
	item.AppVersion, item.SchemaVersion = m.AppVersion, m.SchemaVersion
	item.HasSecret, item.SecretSource = m.HasSecret, m.SecretSource
	return item
}

func pendingView(p backup.Pending) *PendingRestoreView {
	m := p.Manifest
	return &PendingRestoreView{
		Name: p.Name, StagedAt: p.StagedAt, CreatedAt: m.CreatedAt, Reason: string(m.Reason), ReasonLabel: m.Reason.Label(),
		AppVersion: m.AppVersion, SchemaVersion: m.SchemaVersion, KeyAction: string(p.Key), NoCipher: p.NoCipher,
	}
}

func (s *Server) apiBackupSettings(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	var in BackupSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}
	if _, err := s.store.SaveBackupSettings(core.BackupSettingsInput{
		Enabled: in.Enabled, Time: in.Time, Keep: in.Keep, Dir: in.Dir, IncludeSecret: in.IncludeSecret,
	}); err != nil {
		if errors.Is(err, core.ErrBackupSettingsInvalid) {
			// 只回原因：界面会在前面写「备份设置没有保存：」
			http.Error(w, strings.TrimPrefix(err.Error(), core.ErrBackupSettingsInvalid.Error()+"："), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeBackups(w)
}

// apiBackupCreate 是「立即备份」：按现在的设置做一份手动备份（手动备份不会被自动删掉）。
func (s *Server) apiBackupCreate(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	if !s.backupMu.TryLock() {
		http.Error(w, "正在备份或恢复，稍后再试", http.StatusConflict)
		return
	}
	defer s.backupMu.Unlock()
	info, err := s.store.RunBackup(r.Context(), backup.ReasonManual)
	if err != nil {
		http.Error(w, "备份失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	global.GetSlogger().Infof("网页上做了一份备份：%s", info.Path)
	s.writeBackups(w)
}

// apiBackupDownload 下载备份目录里的一份备份（只认这里起的文件名）。
func (s *Server) apiBackupDownload(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	name := r.PathValue("name")
	p, err := s.store.BackupFile(name)
	if err != nil {
		writeBackupError(w, err)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, "打开备份失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "打开备份失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) apiBackupDelete(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	if err := s.store.DeleteBackup(r.PathValue("name")); err != nil {
		writeBackupError(w, err)
		return
	}
	s.writeBackups(w)
}

// apiBackupRestore 校验备份目录里的一份备份，通过后放进待恢复（重启以后生效）。
func (s *Server) apiBackupRestore(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	if !s.backupMu.TryLock() {
		http.Error(w, "正在备份或恢复，稍后再试", http.StatusConflict)
		return
	}
	defer s.backupMu.Unlock()
	p, err := s.store.StageRestoreBackup(r.PathValue("name"))
	if err != nil {
		writeBackupError(w, err)
		return
	}
	global.GetSlogger().Infof("网页上放好了待恢复的备份 %s，重启 pt-tools 以后生效", p.Name)
	s.writeBackups(w)
}

// apiBackupUpload 收一个上传的备份文件（请求体就是文件本身，最大 2 GiB，?name= 是原来的文件名），
// 校验通过后放进待恢复（重启以后生效）；上传的文件用完就删。
func (s *Server) apiBackupUpload(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	if !s.backupMu.TryLock() {
		http.Error(w, "正在备份或恢复，稍后再试", http.StatusConflict)
		return
	}
	defer s.backupMu.Unlock()
	// 服务的 ReadTimeout 是 5 分钟；大文件在慢的线路上传不完，这个请求放宽到 1 小时
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(time.Hour))
	if r.ContentLength > backup.MaxUploadSize {
		http.Error(w, "备份文件超过 2 GiB", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := core.DataDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = os.MkdirAll(data, 0o700); err != nil {
		http.Error(w, "创建数据目录失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmp, err := os.CreateTemp(data, ".pt-tools-upload-*.tar.gz")
	if err != nil {
		http.Error(w, "保存上传的文件失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, http.MaxBytesReader(w, r.Body, backup.MaxUploadSize))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "备份文件超过 2 GiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "上传没有完成: "+err.Error(), http.StatusBadRequest)
		return
	}
	p, err := core.StageRestore(tmp.Name(), uploadName(r.URL.Query().Get("name")))
	if err != nil {
		writeBackupError(w, err)
		return
	}
	global.GetSlogger().Infof("网页上传了备份 %s 并放进待恢复，重启 pt-tools 以后生效", p.Name)
	s.writeBackups(w)
}

// uploadName 是上传文件给人看的名字：只取文件名，去掉控制字符，最长 200 个字符。
func uploadName(raw string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, path.Base(strings.ReplaceAll(raw, `\`, "/")))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "上传的备份"
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	return name
}

func (s *Server) apiBackupCancelRestore(w http.ResponseWriter, _ *http.Request) {
	if s.store == nil {
		http.Error(w, "配置存储未初始化", http.StatusServiceUnavailable)
		return
	}
	if err := core.CancelRestore(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.writeBackups(w)
}

// writeBackupError 把备份与恢复的错误换成状态码：没有这份备份 404，备份文件不能用或密钥对不上 400（原因写给人看）。
func writeBackupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backup.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, backup.ErrInvalid), errors.Is(err, backup.ErrKeyMismatch):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
