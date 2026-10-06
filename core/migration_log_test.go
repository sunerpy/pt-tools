package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunerpy/pt-tools/core/migration"
)

// 迁移成功时 Errors 里也可能有单个站点更新失败：原来只在失败分支打印，成功分支只说「配置迁移成功」。
func TestLogMigrationResult_ReportsPerSiteErrorsOnSuccess(t *testing.T) {
	observed, logs := observer.New(zapcore.InfoLevel)
	log := zap.New(observed).Sugar()

	logMigrationResult(log, &migration.MigrationResult{
		Success: true, SitesMigrated: 2,
		Errors: []string{"更新站点 hdsky 失败: database is locked"},
	})

	errs := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "hdsky")
	assert.Equal(t, 1, logs.FilterMessageSnippet("配置迁移完成").Len())
}

func TestLogMigrationResult_Failure(t *testing.T) {
	observed, logs := observer.New(zapcore.InfoLevel)
	log := zap.New(observed).Sugar()

	logMigrationResult(log, &migration.MigrationResult{Message: "创建备份失败: read-only file system"})

	errs := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "配置迁移失败")
}

func TestLogMigrationResult_NilIsNoop(t *testing.T) {
	observed, logs := observer.New(zapcore.DebugLevel)
	logMigrationResult(zap.New(observed).Sugar(), nil)
	assert.Zero(t, logs.Len())
}
