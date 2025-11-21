package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMTTorrentDetail_FreeAndFinish(t *testing.T) {
	lg := zap.NewNop().Sugar()
	d := MTTorrentDetail{Size: "1048576", Status: &Status{Discount: "free", DiscountEndTime: time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05"), ID: "id"}}
	if !d.IsFree() {
		t.Fatalf("free")
	}
	if !d.CanbeFinished(lg, true, 1024, 1) {
		t.Fatalf("finish")
	}
	if d.GetFreeEndTime() == nil {
		t.Fatalf("end")
	}
	if d.GetFreeLevel() == "" {
		t.Fatalf("level")
	}
}

func TestMTTorrentDetail_NotFreeAndInvalid(t *testing.T) {
	lg := zap.NewNop().Sugar()
	d := MTTorrentDetail{Size: "1048576", Status: &Status{Discount: "none", DiscountEndTime: "", ID: "id"}}
	if d.IsFree() {
		t.Fatalf("notfree")
	}
	if d.CanbeFinished(lg, true, 1024, 1) {
		t.Fatalf("shouldfail")
	}
	if d.GetFreeEndTime() != nil {
		t.Fatalf("endnotnil")
	}
}

func TestMTTorrentDetail_FinishAndFreeHelpers(t *testing.T) {
	lg := zap.NewNop().Sugar()
	d := MTTorrentDetail{Status: &Status{Discount: "free"}}
	require.True(t, d.IsFree())
	d2 := MTTorrentDetail{Status: &Status{ID: "id", DiscountEndTime: "bad"}, Size: "1024"}
	require.False(t, d2.CanbeFinished(lg, true, 10, 1))
	d3 := MTTorrentDetail{Status: &Status{ID: "id", DiscountEndTime: time.Now().Add(time.Hour).Format("2006-01-02 15:04:05")}, Size: "x"}
	require.False(t, d3.CanbeFinished(lg, true, 10, 1))
	d4 := MTTorrentDetail{Status: &Status{DiscountEndTime: time.Now().Add(time.Hour).Format("2006-01-02 15:04:05")}}
	require.NotNil(t, d4.GetFreeEndTime())
	require.Equal(t, "free", MTTorrentDetail{Status: &Status{Discount: "free"}}.GetFreeLevel())
}

func TestMTTorrentDetail_Getters_Table(t *testing.T) {
	now := time.Now().Add(time.Hour).Format("2006-01-02 15:04:05")
	cases := []struct {
		in     MTTorrentDetail
		free   bool
		endNil bool
		level  string
	}{
		{MTTorrentDetail{Status: &Status{Discount: "free", DiscountEndTime: now}, Size: "1024"}, true, false, "free"},
		{MTTorrentDetail{Status: &Status{Discount: "none", DiscountEndTime: ""}, Size: "1024"}, false, true, "none"},
	}
	for _, c := range cases {
		require.Equal(t, c.free, c.in.IsFree())
		if c.endNil {
			require.Nil(t, c.in.GetFreeEndTime())
		} else {
			require.NotNil(t, c.in.GetFreeEndTime())
		}
		require.Equal(t, c.level, c.in.GetFreeLevel())
	}
}

func TestIsInLowerCaseSet(t *testing.T) {
	assert.True(t, isInLowerCaseSet("FREE", []string{"free"}))
	assert.False(t, isInLowerCaseSet("x", []string{"free"}))
}
