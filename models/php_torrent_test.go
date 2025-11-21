package models

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPHPTorrentInfo_FlagsAndFinish(t *testing.T) {
	info := PHPTorrentInfo{Discount: DISCOUNT_FREE, SizeMB: 1024}
	if !info.IsFree() {
		t.Fatalf("IsFree")
	}
	lg := zap.NewNop().Sugar()
	if !info.CanbeFinished(lg, false, 0, 0) {
		t.Fatalf("should finish when disabled")
	}
}

func TestPHPTorrentInfo_FinishAndLevels(t *testing.T) {
	lg := zap.NewNop().Sugar()
	end := time.Now().Add(30 * time.Minute)
	info := PHPTorrentInfo{Discount: DISCOUNT_FREE, SizeMB: 256, EndTime: end, Title: "t"}
	if !info.CanbeFinished(lg, true, 1024, 1) {
		t.Fatalf("finish expected")
	}
	if info.GetFreeEndTime() == nil {
		t.Fatalf("end time nil")
	}
	if info.GetFreeLevel() != string(DISCOUNT_FREE) {
		t.Fatalf("level")
	}
	info2 := PHPTorrentInfo{Discount: DISCOUNT_NONE}
	if info2.IsFree() {
		t.Fatalf("not free")
	}
	if info2.GetFreeLevel() == "" {
		t.Fatalf("level empty")
	}
}
