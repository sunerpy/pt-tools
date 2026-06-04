package models

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type v9SiteLoginState struct {
	ID                       uint `gorm:"primaryKey"`
	SiteName                 string
	LastLoginAt              *time.Time
	LastAccessAt             *time.Time
	LastVisitAt              *time.Time
	LastProbeAt              *time.Time
	LastProbeStatus          string
	LastProbeError           string
	ConsecutiveProbeFailures int
	ProbeJitterSeconds       int
	BanThresholdDays         int
	RemindBeforeDays         int
	ReminderCron             string
	NotificationChannelIDs   string
	LastReminderTier         string
	LastReminderSentAt       *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

func (v9SiteLoginState) TableName() string {
	return "site_login_states"
}

func setupV9MigrationDB(t *testing.T) (*gorm.DB, map[string]*time.Time) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "v9.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建 v9 测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&SchemaVersion{}, &v9SiteLoginState{}); err != nil {
		t.Fatalf("迁移 v9 基础表失败: %v", err)
	}
	if err := db.Create(&SchemaVersion{Version: 9, Description: "test v9", AppVersion: "test"}).Error; err != nil {
		t.Fatalf("写入 v9 版本失败: %v", err)
	}

	loginTimes := make(map[string]*time.Time)
	for idx := 1; idx <= 5; idx++ {
		siteName := fmt.Sprintf("site-%d", idx)
		var lastLoginAt *time.Time
		if idx%2 == 1 {
			loginTime := time.Date(2026, time.May, idx, 10, 30, 0, 0, time.UTC)
			lastLoginAt = &loginTime
			loginTimes[siteName] = &loginTime
		}

		state := v9SiteLoginState{
			SiteName:         siteName,
			LastLoginAt:      lastLoginAt,
			BanThresholdDays: 30,
			RemindBeforeDays: 10,
			ReminderCron:     "0 10,22 * * *",
			LastReminderTier: "none",
		}
		if err := db.Create(&state).Error; err != nil {
			t.Fatalf("写入登录状态 %s 失败: %v", siteName, err)
		}
	}

	return db, loginTimes
}

func assertV10Columns(t *testing.T, db *gorm.DB, wantPresent bool) {
	t.Helper()

	columnNames := []string{"ApiLastLoginAt", "CookieLastLoginAt", "ProbeMode", "LastConsistencyCheck"}
	for _, columnName := range columnNames {
		if present := db.Migrator().HasColumn(&SiteLoginState{}, columnName); present != wantPresent {
			t.Fatalf("HasColumn(%s) = %v, want %v", columnName, present, wantPresent)
		}
	}
}

func TestMigrationV9ToV10Forward(t *testing.T) {
	db, loginTimes := setupV9MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("v9→v10 迁移失败: %v", err)
	}

	assertV10Columns(t, db, true)

	var states []SiteLoginState
	if err := db.Order("id ASC").Find(&states).Error; err != nil {
		t.Fatalf("查询登录状态失败: %v", err)
	}
	if len(states) != 5 {
		t.Fatalf("登录状态行数 = %d, want 5", len(states))
	}
	for _, state := range states {
		wantLoginAt := loginTimes[state.SiteName]
		if wantLoginAt == nil {
			if state.ApiLastLoginAt != nil {
				t.Fatalf("站点 %s ApiLastLoginAt = %v, want NULL", state.SiteName, state.ApiLastLoginAt)
			}
		} else if state.ApiLastLoginAt == nil || !state.ApiLastLoginAt.Equal(*wantLoginAt) {
			t.Fatalf("站点 %s ApiLastLoginAt = %v, want %v", state.SiteName, state.ApiLastLoginAt, *wantLoginAt)
		}
		if state.CookieLastLoginAt != nil {
			t.Fatalf("站点 %s CookieLastLoginAt = %v, want NULL", state.SiteName, state.CookieLastLoginAt)
		}
		if state.ProbeMode != "auto" {
			t.Fatalf("站点 %s ProbeMode = %q, want auto", state.SiteName, state.ProbeMode)
		}
		if state.LastConsistencyCheck != "" {
			t.Fatalf("站点 %s LastConsistencyCheck = %q, want empty", state.SiteName, state.LastConsistencyCheck)
		}
	}

	version, err := sm.GetCurrentVersion()
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if version != 10 {
		t.Fatalf("schema version = %d, want 10", version)
	}
	if hooks.backupCalls.Load() < 1 {
		t.Fatal("backup hook 未调用")
	}
}

func TestMigrationV9ToV10Rollback(t *testing.T) {
	db, _ := setupV9MigrationDB(t)
	failingBackup := func(_ *gorm.DB, table string) (string, error) {
		return "", fmt.Errorf("backup %s failed", table)
	}
	sm := NewSchemaManagerWithHooks(db, "test", failingBackup, nil, nil)

	err := sm.RunMigrations()
	if err == nil {
		t.Fatal("期望 backup 失败导致迁移返回错误")
	}

	version, versionErr := sm.GetCurrentVersion()
	if versionErr != nil {
		t.Fatalf("获取版本失败: %v", versionErr)
	}
	if version != 9 {
		t.Fatalf("schema version = %d, want 9", version)
	}
	assertV10Columns(t, db, false)
}

func TestMigrationV9ToV10Idempotent(t *testing.T) {
	db, _ := setupV9MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	backupCallsAfterFirstRun := hooks.backupCalls.Load()
	if backupCallsAfterFirstRun < 1 {
		t.Fatal("首次迁移 backup hook 未调用")
	}

	if err := sm.migrateV9ToV10(db); err != nil {
		t.Fatalf("重复执行 v10 迁移失败: %v", err)
	}
	if backupCallsAfterSecondRun := hooks.backupCalls.Load(); backupCallsAfterSecondRun != backupCallsAfterFirstRun {
		t.Fatalf("重复迁移 backup 调用数 = %d, want %d", backupCallsAfterSecondRun, backupCallsAfterFirstRun)
	}

	var probeModeCount int64
	if err := db.Model(&SiteLoginState{}).Where("probe_mode = ?", "auto").Count(&probeModeCount).Error; err != nil {
		t.Fatalf("统计 probe_mode 失败: %v", err)
	}
	if probeModeCount != 5 {
		t.Fatalf("probe_mode=auto 行数 = %d, want 5", probeModeCount)
	}
}

func TestMigrationV9ToV10NilHook(t *testing.T) {
	db, _ := setupV9MigrationDB(t)
	sm := NewSchemaManager(db, "test")

	err := sm.RunMigrations()
	if err == nil {
		t.Fatal("期望 nil backup hook 导致迁移失败")
	}
	if !strings.Contains(err.Error(), "backup hook") {
		t.Fatalf("错误 = %v, want mention backup hook", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("错误 = %v, want migration hook error", err)
	}

	version, versionErr := sm.GetCurrentVersion()
	if versionErr != nil {
		t.Fatalf("获取版本失败: %v", versionErr)
	}
	if version != 9 {
		t.Fatalf("schema version = %d, want 9", version)
	}
	assertV10Columns(t, db, false)
}

func TestMigrationV9ToV10WritesMigrationState(t *testing.T) {
	db, _ := setupV9MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	before := time.Now().UTC().Add(-1 * time.Second)
	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	after := time.Now().UTC().Add(1 * time.Second)

	var rows []MigrationState
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("查询 migration_states 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("MigrationState 行数 = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.SchemaVersion != 10 {
		t.Fatalf("SchemaVersion = %d, want 10", row.SchemaVersion)
	}
	if row.CompletedAt.Before(before) || row.CompletedAt.After(after) {
		t.Fatalf("CompletedAt = %v, want within [%v, %v]", row.CompletedAt, before, after)
	}
	if row.BroadcastSent {
		t.Fatal("BroadcastSent = true, want false on fresh migration")
	}

	if completedAt, ok := GetLatestMigrationCompletedAt(db); !ok {
		t.Fatal("GetLatestMigrationCompletedAt returned ok=false after migration")
	} else if completedAt.IsZero() {
		t.Fatal("GetLatestMigrationCompletedAt returned zero time after migration")
	}
}

func TestMigrationV9ToV10MigrationStateIdempotent(t *testing.T) {
	db, _ := setupV9MigrationDB(t)
	hooks := &spyHooks{}
	sm := NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)

	if err := sm.RunMigrations(); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}

	if err := MarkBroadcastSent(db, 10); err != nil {
		t.Fatalf("MarkBroadcastSent 失败: %v", err)
	}

	if err := sm.migrateV9ToV10(db); err != nil {
		t.Fatalf("重复执行 v10 迁移失败: %v", err)
	}

	var count int64
	if err := db.Model(&MigrationState{}).Count(&count).Error; err != nil {
		t.Fatalf("统计 MigrationState 失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("重复迁移后 MigrationState 行数 = %d, want 1", count)
	}

	state, ok := GetMigrationState(db, 10)
	if !ok {
		t.Fatal("GetMigrationState v10 not found")
	}
	if !state.BroadcastSent {
		t.Fatal("BroadcastSent = false after re-run, want true (must preserve sentinel)")
	}
}
