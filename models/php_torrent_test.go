package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestPHPTorrentInfo_MetadataGetters(t *testing.T) {
	p := PHPTorrentInfo{
		Title:    "Some.Movie.2026",
		SubTitle: "中文字幕",
		SizeMB:   2048, // 2 GB
		Discount: DISCOUNT_FREE,
	}
	assert.Equal(t, "Some.Movie.2026", p.GetName())
	assert.Equal(t, "中文字幕", p.GetSubTitle())
	assert.Equal(t, int64(2048*1024*1024), p.GetSizeBytes())
	assert.Equal(t, "free", p.GetFreeLevel())

	// empty discount → "failed"
	pEmpty := PHPTorrentInfo{}
	assert.Equal(t, "failed", pEmpty.GetFreeLevel())
	assert.False(t, pEmpty.IsFree())

	assert.True(t, PHPTorrentInfo{Discount: DISCOUNT_TWO_X_FREE}.IsFree())
}

func TestPHPTorrentInfo_CanbeFinished_Extra(t *testing.T) {
	logger := zap.NewNop().Sugar()

	// size within limit, no speed check → true
	p := PHPTorrentInfo{SizeMB: 1024}
	assert.True(t, p.CanbeFinished(logger, false, 0, 2))

	// size exceeds limit → false
	big := PHPTorrentInfo{SizeMB: 5 * 1024}
	assert.False(t, big.CanbeFinished(logger, false, 0, 2))

	// speed check with plenty of time → true
	future := PHPTorrentInfo{SizeMB: 1, EndTime: time.Now().Add(24 * time.Hour)}
	assert.True(t, future.CanbeFinished(logger, true, 10, 0))

	// speed check but free window already gone → cannot finish
	past := PHPTorrentInfo{SizeMB: 1024 * 1024, EndTime: time.Now().Add(-time.Hour)}
	assert.False(t, past.CanbeFinished(logger, true, 1, 0))
}

func TestPHPTorrentInfo_GetFreeEndTime_Extra(t *testing.T) {
	end := time.Now().Add(time.Hour)
	p := PHPTorrentInfo{EndTime: end}
	got := p.GetFreeEndTime()
	require.NotNil(t, got)
	assert.Equal(t, end, *got)
}
