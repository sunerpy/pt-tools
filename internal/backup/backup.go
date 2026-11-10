// Package backup 做 pt-tools 数据的备份与恢复（媒体中心方案 M0）。
//
// 一份备份是一个 tar.gz：manifest.json、torrents.db（VACUUM INTO 取的一致快照，服务运行中也能做）、
// secret.key（选了带上密钥时）。恢复分两段：网页或命令行先校验，通过后放进 <数据目录>/restore-pending/；
// 下次启动、打开数据库以前换上，换完用当前生效的密钥复验，任何一步失败都把原来的文件放回去、按原来的数据启动。
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// FormatVersion 是备份文件格式的版本，manifest 里记着；比它新的备份不认。
const FormatVersion = 1

const (
	manifestName = "manifest.json"
	dbName       = models.DBFile
	keyName      = "secret.key"
	filePrefix   = "pt-tools-backup-"
	fileSuffix   = ".tar.gz"

	// 解包时各个文件的上限：manifest 与密钥都很小；数据库按 8 GiB 封顶，防止压缩炸弹把磁盘写满
	maxManifestSize = 1 << 20
	maxKeySize      = 1 << 10
	maxDBSize       = 8 << 30
)

// 默认值与范围（设置页与定时任务共用）。
const (
	DefaultTime     = "04:30"
	DefaultKeep     = 7
	MinKeep         = 1
	MaxKeep         = 60
	PreUpgradeKeep  = 3
	DefaultDirName  = "backups"
	MaxUploadSize   = 2 << 30
	SecretFromFile  = "file"
	SecretFromEnv   = "env"
	timestampLayout = "20060102-150405"
	// UnknownAppVersion 是 manifest 里不知道数据库是哪个版本写的：从不记版本号的旧版本升上来时，升级前备份记它。
	UnknownAppVersion = "unknown"
)

// Reason 是做这份备份的原因。
type Reason string

const (
	ReasonScheduled  Reason = "scheduled"
	ReasonPreUpgrade Reason = "pre-upgrade"
	ReasonManual     Reason = "manual"
	ReasonCLI        Reason = "cli"
)

// Label 是给人看的原因。
func (r Reason) Label() string {
	switch r {
	case ReasonScheduled:
		return "定时备份"
	case ReasonPreUpgrade:
		return "升级前备份"
	case ReasonManual:
		return "手动备份"
	case ReasonCLI:
		return "命令行备份"
	}
	return string(r)
}

func validReason(r Reason) bool {
	switch r {
	case ReasonScheduled, ReasonPreUpgrade, ReasonManual, ReasonCLI:
		return true
	}
	return false
}

// Manifest 是备份里的说明：格式版本、做备份的程序版本与数据库版本、时间、原因、数据库的 SHA-256 与大小、带没带密钥与密钥来源。
type Manifest struct {
	Format        int       `json:"format"`
	AppVersion    string    `json:"app_version"`
	SchemaVersion int       `json:"schema_version"`
	CreatedAt     time.Time `json:"created_at"`
	Reason        Reason    `json:"reason"`
	DBSHA256      string    `json:"db_sha256"`
	DBSize        int64     `json:"db_size"`
	HasSecret     bool      `json:"has_secret"`
	// SecretSource 是密钥从哪来的：file（secret.key）或 env（PT_TOOLS_SECRET_KEY）；不带密钥时为空
	SecretSource string `json:"secret_source,omitempty"`
}

// Info 是备份目录里的一份备份。Err 非空表示读不出它的 manifest（坏文件也列出来，让人能删掉）。
type Info struct {
	Name     string   `json:"name"`
	Path     string   `json:"-"`
	Size     int64    `json:"size"`
	Manifest Manifest `json:"manifest"`
	Err      string   `json:"error,omitempty"`
}

// Key 是要放进备份的密钥与它的来源（SecretFromFile 或 SecretFromEnv）。
type Key struct {
	Bytes  []byte
	Source string
}

// Options 是做一份备份的参数。
type Options struct {
	// Dir 是备份目录：按时间与原因起名放在这里
	Dir string
	// Output 非空时写到这个文件（命令行 --output）；是已有的目录时在里面起名；文件已经存在时报错，不覆盖
	Output        string
	Reason        Reason
	IncludeSecret bool
	// AppVersion 是这份数据所属的程序版本（升级前备份写上一个版本）
	AppVersion string
	// Key 在带上密钥时调用
	Key func() (Key, error)
	// Now 为空时用 time.Now
	Now func() time.Time
}

// ErrInvalid 是备份文件本身有问题（不是 pt-tools 的备份、内容被改过、数据库损坏、版本太新……）。
var ErrInvalid = errors.New("备份文件不能用")

// ErrKeyMismatch 是按密钥规则不能恢复：恢复以后生效的密钥解不开备份里加密保存的凭证（Cookie、通知凭证、API Key 等）。
var ErrKeyMismatch = errors.New("密钥对不上")

// ErrNotFound 是备份目录里没有这份备份（或者文件名不是这里起的）。
var ErrNotFound = errors.New("备份不存在")

// nameMu 让起名与改名串行：同一秒做两份备份时不会互相覆盖。
var nameMu sync.Mutex

// Create 做一份备份：先用 VACUUM INTO 取一份一致的快照（运行中也能做，期间别的数据库操作稍等），再打成 tar.gz。
// 先写 .part，Sync 后改名，中途失败不留半个文件。
func Create(ctx context.Context, db *gorm.DB, opts Options) (Info, error) {
	if db == nil {
		return Info{}, errors.New("数据库未初始化")
	}
	if !validReason(opts.Reason) {
		return Info{}, fmt.Errorf("不认识的备份原因 %q", opts.Reason)
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	at := now()
	workDir := opts.Dir
	if opts.Output != "" {
		workDir = filepath.Dir(opts.Output)
		if st, err := os.Stat(opts.Output); err == nil && st.IsDir() {
			workDir = opts.Output
		}
	}
	if strings.TrimSpace(workDir) == "" {
		return Info{}, errors.New("没有指定备份目录")
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return Info{}, fmt.Errorf("创建备份目录失败: %w", err)
	}

	var key Key
	if opts.IncludeSecret {
		if opts.Key == nil {
			return Info{}, errors.New("要带上密钥，却没有给出密钥")
		}
		var err error
		if key, err = opts.Key(); err != nil {
			return Info{}, fmt.Errorf("读取密钥失败: %w", err)
		}
		if len(key.Bytes) != 32 {
			return Info{}, fmt.Errorf("密钥长度不对：%d 字节", len(key.Bytes))
		}
	}

	tmpDir, err := os.MkdirTemp(workDir, ".pt-tools-backup-*")
	if err != nil {
		return Info{}, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	snapshot := filepath.Join(tmpDir, dbName)
	if err = db.WithContext(ctx).Exec("VACUUM INTO ?", snapshot).Error; err != nil {
		return Info{}, fmt.Errorf("取数据库快照失败: %w", err)
	}
	sum, size, err := fileSHA256(snapshot)
	if err != nil {
		return Info{}, err
	}
	schema, err := snapshotSchemaVersion(snapshot)
	if err != nil {
		return Info{}, err
	}
	m := Manifest{
		Format: FormatVersion, AppVersion: opts.AppVersion, SchemaVersion: schema, CreatedAt: at.UTC().Truncate(time.Millisecond),
		Reason: opts.Reason, DBSHA256: sum, DBSize: size, HasSecret: opts.IncludeSecret,
	}
	if opts.IncludeSecret {
		m.SecretSource = key.Source
	}

	part, err := os.CreateTemp(workDir, ".pt-tools-backup-*.part")
	if err != nil {
		return Info{}, fmt.Errorf("创建备份文件失败: %w", err)
	}
	partPath := part.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(partPath)
		}
	}()
	if err = writeArchive(part, m, snapshot, key, opts.IncludeSecret); err != nil {
		_ = part.Close()
		return Info{}, err
	}
	if err = part.Sync(); err != nil {
		_ = part.Close()
		return Info{}, fmt.Errorf("写入备份文件失败: %w", err)
	}
	if err = part.Close(); err != nil {
		return Info{}, fmt.Errorf("写入备份文件失败: %w", err)
	}
	if err = os.Chmod(partPath, 0o600); err != nil {
		return Info{}, fmt.Errorf("设置备份文件权限失败: %w", err)
	}

	nameMu.Lock()
	defer nameMu.Unlock()
	final, err := targetPath(opts, workDir, at)
	if err != nil {
		return Info{}, err
	}
	if err = os.Rename(partPath, final); err != nil {
		return Info{}, fmt.Errorf("保存备份文件失败: %w", err)
	}
	keep = true
	syncDir(filepath.Dir(final))
	st, err := os.Stat(final)
	if err != nil {
		return Info{}, fmt.Errorf("读取备份文件失败: %w", err)
	}
	return Info{Name: filepath.Base(final), Path: final, Size: st.Size(), Manifest: m}, nil
}

// targetPath 是备份文件的最终路径：给了 Output 文件就用它（已存在时报错），否则在目录里按时间与原因起一个没被占用的名字。
func targetPath(opts Options, dir string, at time.Time) (string, error) {
	if opts.Output != "" {
		if st, err := os.Stat(opts.Output); err == nil && st.IsDir() {
			return freeName(opts.Output, at, opts.Reason)
		}
		if _, err := os.Lstat(opts.Output); err == nil {
			return "", fmt.Errorf("%s 已经存在，不覆盖", opts.Output)
		}
		return opts.Output, nil
	}
	return freeName(dir, at, opts.Reason)
}

func freeName(dir string, at time.Time, reason Reason) (string, error) {
	base := filePrefix + at.Local().Format(timestampLayout) + "-" + string(reason)
	for i := 1; i < 1000; i++ {
		name := base + fileSuffix
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", base, i, fileSuffix)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		}
	}
	return "", errors.New("同一秒里的备份太多了")
}

func writeArchive(w io.Writer, m Manifest, snapshot string, key Key, withKey bool) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err = writeEntry(tw, manifestName, raw, m.CreatedAt); err != nil {
		return err
	}
	f, err := os.Open(snapshot)
	if err != nil {
		return fmt.Errorf("读取数据库快照失败: %w", err)
	}
	defer f.Close()
	if err := tw.WriteHeader(&tar.Header{Name: dbName, Mode: 0o600, Size: m.DBSize, ModTime: m.CreatedAt, Typeflag: tar.TypeReg}); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	if withKey {
		// 与 pt-tools secret import 写出的格式一样：64 位十六进制
		if err := writeEntry(tw, keyName, []byte(hex.EncodeToString(key.Bytes)), m.CreatedAt); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	return nil
}

func writeEntry(tw *tar.Writer, name string, data []byte, at time.Time) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: at, Typeflag: tar.TypeReg}); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("读取数据库快照失败: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("读取数据库快照失败: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// syncDir 把目录项落盘（改名以后），失败不影响结果：有的文件系统不支持对目录 Sync。
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// IsBackupName 报告 name 是不是这里起的备份文件名（只有文件名，不含目录）。
func IsBackupName(name string) bool {
	return name == filepath.Base(name) && strings.HasPrefix(name, filePrefix) && strings.HasSuffix(name, fileSuffix) &&
		!strings.ContainsAny(name, `/\`) && name != filePrefix+fileSuffix
}

// List 列出备份目录里的备份，最新的在前。目录不存在时返回空。
func List(dir string) ([]Info, error) {
	// 备份目录是个文件时报错：Windows 上 ReadDir 一个文件回的是「不存在」，会被当成还没有备份
	if st, err := os.Stat(dir); err == nil && !st.IsDir() {
		return nil, fmt.Errorf("读取备份目录失败: %s 不是目录", dir)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}
	out := []Info{}
	for _, e := range entries {
		if e.IsDir() || !IsBackupName(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		info := Info{Name: e.Name(), Path: p}
		if st, err := e.Info(); err == nil {
			info.Size = st.Size()
		}
		if m, err := ReadManifest(p); err != nil {
			info.Err = err.Error()
		} else {
			info.Manifest = m
		}
		out = append(out, info)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := out[i].Manifest.CreatedAt, out[j].Manifest.CreatedAt
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		bi, ni := nameOrder(out[i].Name)
		bj, nj := nameOrder(out[j].Name)
		if bi != bj {
			return bi > bj
		}
		return ni > nj
	})
	return out, nil
}

// nameOrder 把备份名拆成起名的部分与同一秒里的序号（freeName 加的 -2、-3……，没有时是 1），时间相同的按它排：后起的在前。
func nameOrder(name string) (string, int) {
	base := strings.TrimSuffix(name, fileSuffix)
	if i := strings.LastIndexByte(base, '-'); i > 0 {
		if n, err := strconv.Atoi(base[i+1:]); err == nil && n > 1 {
			return base[:i], n
		}
	}
	return base, 1
}

// ReadManifest 只读备份里的 manifest（它是第一个文件，不用解开整个包）。
func ReadManifest(path string) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("打开备份失败: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w：不是 tar.gz 文件", ErrInvalid)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != manifestName {
		return Manifest{}, fmt.Errorf("%w：第一个文件不是 manifest.json", ErrInvalid)
	}
	return decodeManifest(io.LimitReader(tr, maxManifestSize+1))
}

func decodeManifest(r io.Reader) (Manifest, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w：读不出 manifest.json", ErrInvalid)
	}
	if len(raw) > maxManifestSize {
		return Manifest{}, fmt.Errorf("%w：manifest.json 太大", ErrInvalid)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w：manifest.json 格式不对", ErrInvalid)
	}
	if m.Format <= 0 {
		return Manifest{}, fmt.Errorf("%w：manifest.json 里没有格式版本", ErrInvalid)
	}
	return m, nil
}

// Prune 按保留规则删旧备份：定时备份留最新 keepScheduled 份，升级前备份留最新 PreUpgradeKeep 份，手动与命令行的不删。
// 读不出 manifest 的文件不动。返回删掉的文件名。
func Prune(dir string, keepScheduled int) ([]string, error) {
	if keepScheduled < MinKeep {
		keepScheduled = MinKeep
	}
	all, err := List(dir)
	if err != nil {
		return nil, err
	}
	limits := map[Reason]int{ReasonScheduled: keepScheduled, ReasonPreUpgrade: PreUpgradeKeep}
	seen := map[Reason]int{}
	removed := []string{}
	var errs []error
	for _, b := range all { // 最新的在前
		if b.Err != "" {
			continue
		}
		limit, ok := limits[b.Manifest.Reason]
		if !ok {
			continue
		}
		seen[b.Manifest.Reason]++
		if seen[b.Manifest.Reason] <= limit {
			continue
		}
		if err := os.Remove(b.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("删除旧备份 %s 失败: %w", b.Name, err))
			continue
		}
		removed = append(removed, b.Name)
	}
	return removed, errors.Join(errs...)
}

// Resolve 把界面传来的文件名换成备份目录里的路径：只认这里起的名字，不认路径。
func Resolve(dir, name string) (string, error) {
	if !IsBackupName(name) {
		return "", fmt.Errorf("%w：不认识的备份文件名 %q", ErrNotFound, name)
	}
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w：%s", ErrNotFound, name)
		}
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("%w：%s 不是普通文件", ErrNotFound, name)
	}
	return p, nil
}

// Delete 删掉备份目录里的一份备份。
func Delete(dir, name string) error {
	p, err := Resolve(dir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("删除备份失败: %w", err)
	}
	return nil
}
