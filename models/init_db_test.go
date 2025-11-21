package models

import (
	"testing"

	"go.uber.org/zap"
	"moul.io/zapgorm2"
)

func TestNewDB_InitializesAndMigrates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg := zapgorm2.Logger{ZapLogger: zap.NewNop()}
	db, err := NewDB(lg)
	if err != nil {
		t.Fatalf("newdb: %v", err)
	}
	if db == nil || db.DB == nil {
		t.Fatalf("db nil")
	}
}
