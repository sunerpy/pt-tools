package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/utils"
)

func TestMTTorrentDetail_FreeAndFinish(t *testing.T) {
	lg := zap.NewNop().Sugar()
	// Use CST timezone for consistency - CanbeFinished parses time as CST via utils.ParseTimeInCST
	futureTime := time.Now().In(utils.CSTLocation).Add(1 * time.Hour).Format("2006-01-02 15:04:05")
	d := MTTorrentDetail{Size: "1048576", Status: &Status{Discount: "free", DiscountEndTime: futureTime, ID: "id"}}
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

func TestMTTorrentDetail_CanbeFinished_TimeInsufficient(t *testing.T) {
	logger := zapNopLogger()
	future := time.Now().In(cstLoc()).Add(2 * time.Second).Format("2006-01-02 15:04:05")
	d := MTTorrentDetail{
		Status: &Status{ID: "1", DiscountEndTime: future},
		Size:   "2199023255552",
	}
	assert.False(t, d.CanbeFinished(logger, true, 1, 0))
}

func TestMTTorrentDetail_MetadataGetters(t *testing.T) {
	d := MTTorrentDetail{
		Name:       "Movie CN",
		SmallDescr: "副标题",
		Size:       "1073741824", // 1 GiB
		Status:     &Status{Discount: "FREE"},
	}
	assert.Equal(t, "Movie CN", d.GetName())
	assert.Equal(t, "副标题", d.GetSubTitle())
	assert.Equal(t, int64(1073741824), d.GetSizeBytes())
	assert.Equal(t, "FREE", d.GetFreeLevel())

	// unparsable size → 0
	bad := MTTorrentDetail{Size: "not-a-number"}
	assert.Equal(t, int64(0), bad.GetSizeBytes())

	// no status → "failed"
	assert.Equal(t, "failed", MTTorrentDetail{}.GetFreeLevel())
	// status present but empty discount → "failed"
	assert.Equal(t, "failed", MTTorrentDetail{Status: &Status{}}.GetFreeLevel())
}

func TestMTTorrentDetail_IsFree(t *testing.T) {
	assert.True(t, MTTorrentDetail{Status: &Status{Discount: "free"}}.IsFree())
	assert.True(t, MTTorrentDetail{Status: &Status{PromotionRule: &PromotionRule{Discount: "FREE"}}}.IsFree())
	assert.False(t, MTTorrentDetail{Status: &Status{Discount: "50%"}}.IsFree())
	assert.False(t, MTTorrentDetail{}.IsFree())
}

func TestMTTorrentDetail_CanbeFinished(t *testing.T) {
	logger := zap.NewNop().Sugar()

	// nil status → false
	assert.False(t, MTTorrentDetail{}.CanbeFinished(logger, false, 0, 0))

	// unparsable size → false
	assert.False(t, MTTorrentDetail{Status: &Status{ID: "1"}, Size: "abc"}.CanbeFinished(logger, false, 0, 0))

	// size within limit, no speed check → true
	ok := MTTorrentDetail{Status: &Status{ID: "1"}, Size: "1048576"} // 1 MiB
	assert.True(t, ok.CanbeFinished(logger, false, 0, 5))

	// size exceeds limit → false
	big := MTTorrentDetail{Status: &Status{ID: "1"}, Size: "5368709120"} // 5 GiB
	assert.False(t, big.CanbeFinished(logger, false, 0, 1))

	// speed check but DiscountEndTime empty → false
	noEnd := MTTorrentDetail{Status: &Status{ID: "1", DiscountEndTime: ""}, Size: "1048576"}
	assert.False(t, noEnd.CanbeFinished(logger, true, 10, 0))

	// speed check with bad time format → false
	badTime := MTTorrentDetail{Status: &Status{ID: "1", DiscountEndTime: "not-a-time"}, Size: "1048576"}
	assert.False(t, badTime.CanbeFinished(logger, true, 10, 0))
}

func TestMTTorrentDetail_GetFreeEndTime(t *testing.T) {
	// valid CST time string
	d := MTTorrentDetail{Status: &Status{DiscountEndTime: "2030-01-02 15:04:05"}}
	got := d.GetFreeEndTime()
	require.NotNil(t, got)

	// invalid time string → nil
	bad := MTTorrentDetail{Status: &Status{DiscountEndTime: "bad"}}
	assert.Nil(t, bad.GetFreeEndTime())
}
