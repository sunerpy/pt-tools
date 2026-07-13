package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

func newMemDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	return db
}

func zapNopGormLogger() zapgorm2.Logger {
	return zapgorm2.Logger{ZapLogger: zap.NewNop()}
}

func zapNopLogger() *zap.SugaredLogger { return zap.NewNop().Sugar() }

func cstLoc() *time.Location { return time.FixedZone("CST", 8*3600) }
