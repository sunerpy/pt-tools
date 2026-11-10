package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/models"
)

// PendingDir 是待恢复的文件放在数据目录下的哪个子目录。
const PendingDir = "restore-pending"

const (
	readyName       = "ready"
	pendingInfoName = "pending.json"
)

// ActiveKey 是校验恢复时「当前生效的密钥」：设了环境变量 PT_TOOLS_SECRET_KEY 时是它，否则是现在的 secret.key。
type ActiveKey struct {
	// FromEnv 为真表示密钥来自环境变量：恢复以后仍用它，不动 secret.key 文件
	FromEnv bool
	// Key 是当前生效的密钥；读不出（文件坏了）时为空
	Key []byte
}

// KeyAction 是恢复以后用哪把密钥。
type KeyAction string

const (
	// KeyKeepEnv：仍用环境变量里的密钥
	KeyKeepEnv KeyAction = "keep-env"
	// KeyUseBackup：secret.key 换成备份里的
	KeyUseBackup KeyAction = "use-backup"
	// KeyKeepFile：secret.key 不动
	KeyKeepFile KeyAction = "keep-file"
)

// Plan 是校验通过的一份备份的恢复方案。
type Plan struct {
	Name     string    `json:"name"`
	Manifest Manifest  `json:"manifest"`
	Key      KeyAction `json:"key_action"`
	// NoCipher 为真表示备份数据库里没有任何密文，不需要密钥
	NoCipher bool `json:"no_cipher"`
}

// Pending 是已经放好、下次启动生效的恢复。
type Pending struct {
	Plan
	// HasKey 为真表示待恢复里放了备份里的密钥（备份带了密钥、暂存时没用环境变量）。启动时现有的 secret.key 和它不同就换上它：
	// 暂存时两把一样、之后原来的 secret.key 丢了（启动时新生成了一把）的情况也能用备份里的密钥启动。
	HasKey   bool      `json:"has_key"`
	StagedAt time.Time `json:"staged_at"`
}

// unpacked 是解开到临时目录的一份备份。
type unpacked struct {
	dir      string
	manifest Manifest
	db       string
	key      []byte
}

// unpack 把备份解到 workDir 下的临时目录，核对文件、SHA-256 与大小。只认 manifest.json、torrents.db、secret.key 三个文件，别的跳过不写。
func unpack(src, workDir string) (*unpacked, error) {
	f, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("打开备份失败: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%w：不是 tar.gz 文件", ErrInvalid)
	}
	defer gz.Close()
	if err = os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	dir, err := os.MkdirTemp(workDir, ".pt-tools-restore-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	u := &unpacked{dir: dir}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	tr := tar.NewReader(gz)
	var haveManifest bool
	for {
		hdr, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("%w：读不完这个压缩包（文件不完整？）", ErrInvalid)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		switch hdr.Name {
		case manifestName:
			if u.manifest, err = decodeManifest(io.LimitReader(tr, maxManifestSize+1)); err != nil {
				return nil, err
			}
			haveManifest = true
		case dbName:
			u.db = filepath.Join(dir, dbName)
			if err = copyLimited(u.db, tr, maxDBSize); err != nil {
				return nil, err
			}
		case keyName:
			if u.key, err = readKeyEntry(tr); err != nil {
				return nil, err
			}
		}
	}
	switch {
	case !haveManifest:
		return nil, fmt.Errorf("%w：没有 manifest.json，不是 pt-tools 的备份", ErrInvalid)
	case u.manifest.Format > FormatVersion:
		return nil, fmt.Errorf("%w：这份备份是更新版本的 pt-tools 做的（格式 %d），先升级 pt-tools 再恢复", ErrInvalid, u.manifest.Format)
	case u.db == "":
		return nil, fmt.Errorf("%w：备份里没有数据库", ErrInvalid)
	case u.manifest.HasSecret && u.key == nil:
		return nil, fmt.Errorf("%w：manifest 说带了密钥，备份里却没有 secret.key", ErrInvalid)
	case !u.manifest.HasSecret && u.key != nil:
		return nil, fmt.Errorf("%w：manifest 说没带密钥，备份里却有 secret.key", ErrInvalid)
	}
	sum, size, err := fileSHA256(u.db)
	if err != nil {
		return nil, err
	}
	if sum != u.manifest.DBSHA256 || size != u.manifest.DBSize {
		return nil, fmt.Errorf("%w：数据库和 manifest 记的 SHA-256 或大小对不上（文件被改过或不完整）", ErrInvalid)
	}
	ok = true
	return u, nil
}

func (u *unpacked) close() { _ = os.RemoveAll(u.dir) }

// readKeyEntry 读备份里的 secret.key：64 位十六进制的 AES-256 密钥。
func readKeyEntry(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxKeySize+1))
	if err != nil || len(raw) > maxKeySize {
		return nil, fmt.Errorf("%w：读不出 secret.key", ErrInvalid)
	}
	key, err := hex.DecodeString(string(bytes.TrimSpace(raw)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w：备份里的 secret.key 不是 64 位十六进制的 AES-256 密钥", ErrInvalid)
	}
	return key, nil
}

func copyLimited(dst string, r io.Reader, limit int64) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%w：解出数据库失败：%v", ErrInvalid, err)
	}
	if n > limit {
		return fmt.Errorf("%w：数据库超过 %d GiB", ErrInvalid, limit>>30)
	}
	return nil
}

// check 校验解开的备份：数据库版本不比当前程序新、integrity_check 通过，再按密钥规则定恢复以后用哪把密钥，
// 并用那把密钥解开备份数据库里每个加密列的密文各一条。
//
// 密钥规则（网页、命令行与启动时都按它校验）：
//   - 环境变量设了密钥：恢复后仍用环境变量，不动 secret.key。备份库里有密文、环境变量的密钥却解不开时拒绝；
//   - 没有环境变量、备份带了密钥：恢复后用备份里的密钥（和现在的一样时不换）；
//   - 没有环境变量、备份没带密钥：备份库里没有密文，或者现在的 secret.key 解得开，才允许恢复，secret.key 不动。
func (u *unpacked) check(active ActiveKey) (Plan, error) {
	plan := Plan{Manifest: u.manifest}
	if u.manifest.SchemaVersion > models.CurrentSchemaVersion {
		return plan, fmt.Errorf("%w：这份备份的数据库版本是 %d，当前程序只认到 %d，先升级 pt-tools 再恢复",
			ErrInvalid, u.manifest.SchemaVersion, models.CurrentSchemaVersion)
	}
	db, done, err := openReadOnly(u.db)
	if err != nil {
		return plan, fmt.Errorf("%w：%v", ErrInvalid, err)
	}
	defer done()
	if err = integrityCheck(db); err != nil {
		return plan, err
	}
	var key []byte
	switch {
	case active.FromEnv:
		plan.Key, key = KeyKeepEnv, active.Key
	case u.manifest.HasSecret:
		plan.Key, key = KeyUseBackup, u.key
		if bytes.Equal(u.key, active.Key) {
			plan.Key = KeyKeepFile
		}
	default:
		plan.Key, key = KeyKeepFile, active.Key
	}
	decrypt := func(string) ([]byte, error) { return nil, errCannotDecrypt }
	if len(key) == 32 {
		decrypt = withKey(key)
	}
	plan.NoCipher, err = cipherCheck(db, decrypt)
	if err == nil {
		return plan, nil
	}
	if !errors.Is(err, errCannotDecrypt) {
		return plan, err
	}
	switch {
	case active.FromEnv:
		return plan, fmt.Errorf("%w：环境变量 PT_TOOLS_SECRET_KEY 的密钥解不开这份备份里的%s；恢复以后启动时也会因为密钥对不上而拒绝启动",
			ErrKeyMismatch, errDetail(err))
	case u.manifest.HasSecret:
		return plan, fmt.Errorf("%w：备份里的密钥解不开它自己数据库里的%s", ErrInvalid, errDetail(err))
	default:
		return plan, fmt.Errorf("%w：这份备份没带密钥，现在的密钥解不开它保存的凭证（%s）。"+
			"先用 pt-tools secret import 导入做备份时的密钥再恢复",
			ErrKeyMismatch, errDetail(err))
	}
}

func errDetail(err error) string {
	var s string
	if err != nil {
		s = err.Error()
	}
	if p := errCannotDecrypt.Error() + "："; len(s) > len(p) && s[:len(p)] == p {
		return s[len(p):]
	}
	return s
}

// Verify 校验一份备份并给出恢复方案，不改任何东西。workDir 放临时解开的文件，用完删掉。
func Verify(src, workDir string, active ActiveKey) (Plan, error) {
	u, err := unpack(src, workDir)
	if err != nil {
		return Plan{}, err
	}
	defer u.close()
	plan, err := u.check(active)
	plan.Name = filepath.Base(src)
	return plan, err
}

// Stage 校验一份备份，通过后把数据库（要换密钥时还有密钥）放进 <dataDir>/restore-pending/，下次启动时生效。
// 已有的待恢复被替换。name 是给人看的备份名（上传的文件没有备份目录里的名字）。
func Stage(src, name, dataDir string, active ActiveKey, now time.Time) (Pending, error) {
	u, err := unpack(src, dataDir)
	if err != nil {
		return Pending{}, err
	}
	defer u.close()
	plan, err := u.check(active)
	if err != nil {
		return Pending{}, err
	}
	plan.Name = name
	p := Pending{Plan: plan, HasKey: !active.FromEnv && u.key != nil, StagedAt: now.UTC().Truncate(time.Second)}

	tmp, err := os.MkdirTemp(dataDir, ".restore-pending-*")
	if err != nil {
		return Pending{}, fmt.Errorf("创建待恢复目录失败: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err = os.Rename(u.db, filepath.Join(tmp, dbName)); err != nil {
		return Pending{}, fmt.Errorf("放置待恢复的数据库失败: %w", err)
	}
	if p.HasKey {
		if err = writeFileSync(filepath.Join(tmp, keyName), []byte(hex.EncodeToString(u.key))); err != nil {
			return Pending{}, err
		}
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Pending{}, err
	}
	if err := writeFileSync(filepath.Join(tmp, pendingInfoName), raw); err != nil {
		return Pending{}, err
	}
	// ready 最后写：没有它的目录是半成品，启动时直接删掉
	if err := writeFileSync(filepath.Join(tmp, readyName), []byte(p.StagedAt.Format(time.RFC3339))); err != nil {
		return Pending{}, err
	}
	target := filepath.Join(dataDir, PendingDir)
	if err := os.RemoveAll(target); err != nil {
		return Pending{}, fmt.Errorf("替换原来的待恢复失败: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return Pending{}, fmt.Errorf("放置待恢复失败: %w", err)
	}
	ok = true
	syncDir(dataDir)
	return p, nil
}

// readKeyFile 读一个 secret.key（64 位十六进制的 AES-256 密钥）。
func readKeyFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(string(bytes.TrimSpace(raw)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("不是 64 位十六进制的 AES-256 密钥")
	}
	return key, nil
}

func writeFileSync(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", filepath.Base(path), err)
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("写入 %s 失败: %w", filepath.Base(path), err)
	}
	return nil
}

// ReadPending 读已经放好的待恢复；没有时返回 nil。半成品（没有 ready）也当作没有。
func ReadPending(dataDir string) (*Pending, error) {
	dir := filepath.Join(dataDir, PendingDir)
	if _, err := os.Stat(filepath.Join(dir, readyName)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取待恢复失败: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, pendingInfoName))
	if err != nil {
		return nil, fmt.Errorf("读取待恢复失败: %w", err)
	}
	var p Pending
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("读取待恢复失败: %w", err)
	}
	return &p, nil
}

// CancelPending 取消还没生效的恢复（删掉 restore-pending）。
func CancelPending(dataDir string) error {
	if err := os.RemoveAll(filepath.Join(dataDir, PendingDir)); err != nil {
		return fmt.Errorf("取消恢复失败: %w", err)
	}
	return nil
}

// ApplyResult 是启动时恢复的结果。
type ApplyResult struct {
	// Name 是待恢复的备份名；没有待恢复时为空
	Name string
	// Applied 为真表示换上了
	Applied bool
	// Err 是没换上的原因：文件已经放回原位，按原来的数据继续启动
	Err error
	// PreRestoreDir 是换下来的原文件放在哪
	PreRestoreDir string
}

// fileOps 是 ApplyPending 用到的文件操作，测试里换掉来模拟某一步失败。
type fileOps struct {
	rename func(oldpath, newpath string) error
	mkdir  func(path string) error
	verify func(dbPath string) error
}

var ops = fileOps{
	rename: os.Rename,
	mkdir:  func(p string) error { return os.MkdirAll(p, 0o700) },
	verify: verifyRestored,
}

type moved struct{ from, to string }

// ApplyPending 在启动时、打开数据库与启动检查以前换上待恢复的文件（只在 Web 服务进程里调用）。envKey 为真表示
// 这个进程用的是环境变量 PT_TOOLS_SECRET_KEY 的密钥：这时不动 secret.key 文件。
//
// 要不要换密钥：待恢复里有备份的密钥、没有用环境变量、现有的 secret.key 和它不同（包括没有、读不出、启动时新生成的）时换。
//  1. 记下内存里的密钥状态；把现有的数据库（-wal、-shm，要换密钥时还有 secret.key）挪进 <dataDir>/backups/pre-restore-<时间>/；
//  2. 放入待恢复的文件；换了密钥文件时重新读取密钥，没换时不碰密钥状态；
//  3. 只读打开换上的数据库，用当前生效的密钥解开每个加密列的密文各一条；
//  4. 通过后删掉 restore-pending。
//
// 第 1–3 步任何一步失败：原文件挪回原位，换过密钥的原样还原密钥状态，删掉 restore-pending，结果里写明原因，按原来的数据继续启动。
// 挪回也失败时返回错误：调用方必须停止启动，不在这种状态下打开任何一份数据库。
func ApplyPending(dataDir string, envKey bool, now time.Time) (ApplyResult, error) {
	pend := filepath.Join(dataDir, PendingDir)
	p, readErr := ReadPending(dataDir)
	if readErr != nil {
		_ = os.RemoveAll(pend)
		return ApplyResult{Err: readErr}, nil
	}
	if p == nil {
		// 没有待恢复；残留的半成品（没写完 ready）删掉
		if _, statErr := os.Stat(pend); statErr == nil {
			_ = os.RemoveAll(pend)
		}
		return ApplyResult{}, nil
	}
	res := ApplyResult{Name: p.Name}
	swapKey := false
	if p.HasKey && !envKey {
		want, err := readKeyFile(filepath.Join(pend, keyName))
		if err != nil {
			_ = os.RemoveAll(pend)
			res.Err = fmt.Errorf("待恢复的文件不完整，备份里的密钥读不出来: %w", err)
			return res, nil
		}
		cur, err := readKeyFile(filepath.Join(dataDir, keyName))
		swapKey = err != nil || !bytes.Equal(cur, want)
	}
	snap := crypto.SnapshotKeyState()
	pre := filepath.Join(dataDir, DefaultDirName, "pre-restore-"+now.Local().Format(timestampLayout))
	res.PreRestoreDir = pre

	var done []moved
	keySwapped := false
	step := func() error {
		if err := ops.mkdir(pre); err != nil {
			return fmt.Errorf("创建 %s 失败: %w", pre, err)
		}
		names := []string{dbName, dbName + "-wal", dbName + "-shm"}
		if swapKey {
			names = append(names, keyName)
		}
		for _, n := range names {
			src := filepath.Join(dataDir, n)
			if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err := ops.rename(src, filepath.Join(pre, n)); err != nil {
				return fmt.Errorf("挪走现有的 %s 失败: %w", n, err)
			}
			done = append(done, moved{src, filepath.Join(pre, n)})
		}
		if err := ops.rename(filepath.Join(pend, dbName), filepath.Join(dataDir, dbName)); err != nil {
			return fmt.Errorf("放入备份的数据库失败: %w", err)
		}
		done = append(done, moved{filepath.Join(pend, dbName), filepath.Join(dataDir, dbName)})
		if swapKey {
			if err := ops.rename(filepath.Join(pend, keyName), filepath.Join(dataDir, keyName)); err != nil {
				return fmt.Errorf("放入备份的密钥失败: %w", err)
			}
			done = append(done, moved{filepath.Join(pend, keyName), filepath.Join(dataDir, keyName)})
			crypto.ReloadKey()
			keySwapped = true
		}
		return ops.verify(filepath.Join(dataDir, dbName))
	}
	err := step()
	if err == nil {
		_ = os.RemoveAll(pend)
		res.Applied = true
		return res, nil
	}
	res.Err = err
	// 回滚：从后往前，把放进来的挪回 restore-pending、把挪走的放回原位
	var rbErrs []error
	for i := len(done) - 1; i >= 0; i-- {
		if rerr := ops.rename(done[i].to, done[i].from); rerr != nil {
			rbErrs = append(rbErrs, rerr)
		}
	}
	if keySwapped {
		crypto.RestoreKeyState(snap)
	}
	if len(rbErrs) == 0 {
		// 文件都放回去了，pre-restore 这时是空目录（有东西时 Remove 不删）
		_ = os.Remove(pre)
	}
	if len(rbErrs) > 0 {
		return res, fmt.Errorf("恢复备份失败（%v），而且没能把原来的文件放回去（%w）：原来的文件在 %s，要恢复的文件在 %s；"+
			"为免打开错的数据库，pt-tools 停止启动。手动把原来的文件放回 %s 以后再启动",
			err, errors.Join(rbErrs...), pre, pend, dataDir)
	}
	_ = os.RemoveAll(pend)
	return res, nil
}

// verifyRestored 是第 3 步：只读打开刚换上的数据库，用当前生效的密钥（换过密钥文件的已经重新读取）解开每个加密列的密文各一条。
func verifyRestored(dbPath string) error {
	if _, _, loadErr := crypto.KeyStatus(); loadErr != nil {
		return fmt.Errorf("换上的密钥读不出来: %w", loadErr)
	}
	db, done, err := openReadOnly(dbPath)
	if err != nil {
		return err
	}
	defer done()
	if _, err := cipherCheck(db, crypto.Decrypt); err != nil {
		if errors.Is(err, errCannotDecrypt) {
			return fmt.Errorf("当前生效的密钥解不开换上的数据库里的%s", errDetail(err))
		}
		return err
	}
	return nil
}
