package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// 一份带密钥的备份：manifest 是第一个文件，记着原因、数据库的 SHA-256 与大小、密钥来源；数据库是能打开的快照，
// 密钥与 secret import 写出的格式一样（64 位十六进制）；文件权限 0600（里面有能解开 Cookie 的密钥）。
func TestCreate_ManifestAndContents(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	addCookie(t, db, "hdsky", "uid=1")
	at := time.Date(2026, 11, 2, 4, 30, 0, 0, time.Local)

	info := makeBackup(t, db, e.data, true, ReasonScheduled, at)
	assert.Equal(t, "pt-tools-backup-20261102-043000-scheduled.tar.gz", info.Name)
	assert.Equal(t, filepath.Join(e.data, DefaultDirName, info.Name), info.Path)
	if runtime.GOOS != "windows" {
		st, err := os.Stat(info.Path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
	}
	m := info.Manifest
	assert.Equal(t, FormatVersion, m.Format)
	assert.Equal(t, ReasonScheduled, m.Reason)
	assert.Equal(t, "test", m.AppVersion)
	assert.Equal(t, models.CurrentSchemaVersion, m.SchemaVersion)
	assert.True(t, m.HasSecret)
	assert.Equal(t, SecretFromFile, m.SecretSource)
	assert.True(t, m.CreatedAt.Equal(at))

	order, files := entries(t, info.Path)
	assert.Equal(t, []string{manifestName, dbName, keyName}, order)
	assert.Equal(t, strings.Repeat("01", 32), string(files[keyName]))
	assert.Equal(t, m.DBSize, int64(len(files[dbName])))

	read, err := ReadManifest(info.Path)
	require.NoError(t, err)
	assert.Equal(t, m, read)

	// 快照里有刚写的那一行
	snap := filepath.Join(t.TempDir(), "snap.db")
	require.NoError(t, os.WriteFile(snap, files[dbName], 0o600))
	sdb, done, err := openReadOnly(snap)
	require.NoError(t, err)
	defer done()
	var n int64
	require.NoError(t, sdb.Model(&models.SiteSetting{}).Where("name = ?", "hdsky").Count(&n).Error)
	assert.Equal(t, int64(1), n)
	require.NoError(t, integrityCheck(sdb))

	// 没有留下临时文件
	left, err := os.ReadDir(filepath.Join(e.data, DefaultDirName))
	require.NoError(t, err)
	assert.Len(t, left, 1)
}

func TestCreate_WithoutSecret(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	info := makeBackup(t, db, e.data, false, ReasonManual, time.Now())
	assert.False(t, info.Manifest.HasSecret)
	assert.Empty(t, info.Manifest.SecretSource)
	order, _ := entries(t, info.Path)
	assert.Equal(t, []string{manifestName, dbName}, order)
}

// 运行中备份：VACUUM INTO 取的是一致的快照。写入方每次在一个事务里写两行，快照里的行数一定是双数（不会只有半个事务）。
func TestCreate_ConsistentWhileWriting(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	var stop atomic.Bool
	var wg sync.WaitGroup
	var writeErr error
	started := make(chan struct{})
	var once sync.Once
	signal := func() { once.Do(func() { close(started) }) }
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			if i == 1 {
				signal()
			}
			err := db.Transaction(func(tx *gorm.DB) error {
				for j := 0; j < 2; j++ {
					if err := tx.Create(&models.TorrentInfo{SiteName: "s", TorrentID: fmt.Sprintf("%d-%d", i, j)}).Error; err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				writeErr = err
				signal()
				return
			}
		}
	}()
	<-started // 第一个事务提交以后再备份，备份期间写入一直在跑
	var infos []Info
	for i := 0; i < 3; i++ {
		infos = append(infos, makeBackup(t, db, e.data, false, ReasonManual, time.Now().Add(time.Duration(i)*time.Second)))
	}
	stop.Store(true)
	wg.Wait()
	require.NoError(t, writeErr)
	for _, info := range infos {
		_, files := entries(t, info.Path)
		snap := filepath.Join(t.TempDir(), "snap.db")
		require.NoError(t, os.WriteFile(snap, files[dbName], 0o600))
		sdb, done, err := openReadOnly(snap)
		require.NoError(t, err)
		require.NoError(t, integrityCheck(sdb))
		var n int64
		require.NoError(t, sdb.Model(&models.TorrentInfo{}).Count(&n).Error)
		done()
		assert.Zero(t, n%2, "快照里有半个事务：%d 行", n)
	}
}

// 命令行 --output：给文件名就写成那个文件，已存在时不覆盖；给目录就在里面起名。
func TestCreate_Output(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	out := filepath.Join(t.TempDir(), "my.tar.gz")
	opts := Options{Output: out, Reason: ReasonCLI, AppVersion: "test"}
	info, err := Create(context.Background(), db, opts)
	require.NoError(t, err)
	assert.Equal(t, out, info.Path)
	_, err = Create(context.Background(), db, opts)
	assert.ErrorContains(t, err, "已经存在")

	dir := t.TempDir()
	info, err = Create(context.Background(), db, Options{Output: dir, Reason: ReasonCLI})
	require.NoError(t, err)
	assert.Equal(t, dir, filepath.Dir(info.Path))
	assert.True(t, IsBackupName(info.Name))
	_ = e
}

// 中途失败不留半个文件：读不出密钥、数据库已关闭。
func TestCreate_FailureLeavesNothing(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	dir := filepath.Join(e.data, DefaultDirName)
	_, err := Create(context.Background(), db, Options{
		Dir: dir, Reason: ReasonManual, IncludeSecret: true,
		Key: func() (Key, error) { return Key{}, errors.New("no key") },
	})
	assert.ErrorContains(t, err, "读取密钥失败")
	closeDB(t, db)
	_, err = Create(context.Background(), db, Options{Dir: dir, Reason: ReasonManual})
	assert.Error(t, err)
	left, rerr := os.ReadDir(dir)
	require.NoError(t, rerr)
	assert.Empty(t, left)

	_, err = Create(context.Background(), nil, Options{Dir: dir, Reason: ReasonManual})
	assert.Error(t, err)
	_, err = Create(context.Background(), db, Options{Dir: dir, Reason: "weird"})
	assert.ErrorContains(t, err, "不认识的备份原因")
}

// 保留：定时备份留最新 N 份、升级前留 3 份、手动与命令行不删、读不出 manifest 的文件不动；同一秒的两份不互相覆盖。
func TestListAndPrune(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	dir := filepath.Join(e.data, DefaultDirName)
	base := time.Date(2026, 11, 1, 4, 30, 0, 0, time.Local)
	for i := 0; i < 5; i++ {
		makeBackup(t, db, e.data, false, ReasonScheduled, base.Add(time.Duration(i)*24*time.Hour))
	}
	for i := 0; i < 4; i++ {
		makeBackup(t, db, e.data, false, ReasonPreUpgrade, base.Add(time.Duration(i)*time.Hour))
	}
	m1 := makeBackup(t, db, e.data, false, ReasonManual, base)
	m2 := makeBackup(t, db, e.data, false, ReasonManual, base)
	assert.NotEqual(t, m1.Name, m2.Name, "同一秒的两份备份起了不同的名字")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pt-tools-backup-broken.tar.gz"), []byte("junk"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("mine"), 0o600))

	all, err := List(dir)
	require.NoError(t, err)
	assert.Len(t, all, 12)

	removed, err := Prune(dir, 2)
	require.NoError(t, err)
	assert.Len(t, removed, 4, "删掉 3 份旧的定时备份与 1 份旧的升级前备份")
	all, err = List(dir)
	require.NoError(t, err)
	count := map[Reason]int{}
	broken := 0
	for _, b := range all {
		if b.Err != "" {
			broken++
			continue
		}
		count[b.Manifest.Reason]++
	}
	assert.Equal(t, 2, count[ReasonScheduled])
	assert.Equal(t, 3, count[ReasonPreUpgrade])
	assert.Equal(t, 2, count[ReasonManual])
	assert.Equal(t, 1, broken, "坏文件列出来但不删")
	// 留下的是最新的两份定时备份
	var sched []time.Time
	for _, b := range all {
		if b.Manifest.Reason == ReasonScheduled {
			sched = append(sched, b.Manifest.CreatedAt)
		}
	}
	assert.True(t, sched[0].Equal(base.Add(4*24*time.Hour)))
	assert.True(t, sched[1].Equal(base.Add(3*24*time.Hour)))
	_, err = os.Stat(filepath.Join(dir, "notes.txt"))
	assert.NoError(t, err, "不是备份的文件不碰")
	// 同一时刻的两份：后起名（带 -2）的排在前面
	var manual []string
	for _, b := range all {
		if b.Manifest.Reason == ReasonManual {
			manual = append(manual, b.Name)
		}
	}
	assert.Equal(t, []string{m2.Name, m1.Name}, manual)

	empty, err := List(filepath.Join(e.data, "nope"))
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// 界面传来的名字只认这里起的备份名，路径一律拒绝。
func TestResolveAndDelete(t *testing.T) {
	e := newEnv(t, keyOf(1))
	db := openDB(t)
	defer closeDB(t, db)
	info := makeBackup(t, db, e.data, false, ReasonManual, time.Now())
	dir := filepath.Join(e.data, DefaultDirName)
	for _, bad := range []string{"../torrents.db", "pt-tools-backup-x.tar.gz/../../x", "secret.key", "", filePrefix + fileSuffix, `pt-tools-backup-a\b.tar.gz`} {
		_, err := Resolve(dir, bad)
		assert.ErrorIs(t, err, ErrNotFound, bad)
	}
	_, err := Resolve(dir, "pt-tools-backup-missing.tar.gz")
	assert.ErrorIs(t, err, ErrNotFound)
	// 名字对、却是个目录：不当作备份
	require.NoError(t, os.Mkdir(filepath.Join(dir, "pt-tools-backup-dir.tar.gz"), 0o700))
	_, err = Resolve(dir, "pt-tools-backup-dir.tar.gz")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, Delete(dir, info.Name))
	_, err = os.Stat(info.Path)
	assert.True(t, os.IsNotExist(err))
	assert.ErrorIs(t, Delete(dir, info.Name), ErrNotFound)
}

func TestNameOrder(t *testing.T) {
	b, n := nameOrder("pt-tools-backup-20261102-040000-pre-upgrade.tar.gz")
	assert.Equal(t, "pt-tools-backup-20261102-040000-pre-upgrade", b)
	assert.Equal(t, 1, n)
	b, n = nameOrder("pt-tools-backup-20261102-040000-pre-upgrade-12.tar.gz")
	assert.Equal(t, "pt-tools-backup-20261102-040000-pre-upgrade", b)
	assert.Equal(t, 12, n)
}

func TestReasonLabel(t *testing.T) {
	assert.Equal(t, "定时备份", ReasonScheduled.Label())
	assert.Equal(t, "升级前备份", ReasonPreUpgrade.Label())
	assert.Equal(t, "手动备份", ReasonManual.Label())
	assert.Equal(t, "命令行备份", ReasonCLI.Label())
	assert.Equal(t, "x", Reason("x").Label())
}
