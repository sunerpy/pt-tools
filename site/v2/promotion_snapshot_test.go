package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 规范快照：零结束时间不等于永久；结束时间已过不再是免费；只有驱动明确写了才是长期免费
func TestTorrentItemPromotion(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(2*time.Hour), now.Add(-time.Minute)
	cases := []struct {
		name string
		item TorrentItem
		want FreeValidity
		ends *time.Time
	}{
		{"免费有结束时间", TorrentItem{DiscountLevel: DiscountFree, DiscountEndTime: future}, FreeUntil, &future},
		{"免费没有结束时间", TorrentItem{DiscountLevel: DiscountFree}, FreeUnknown, nil},
		{"免费已结束", TorrentItem{DiscountLevel: Discount2xFree, DiscountEndTime: past}, FreeNotFree, &past},
		{"结束时间恰好是现在", TorrentItem{DiscountLevel: DiscountFree, DiscountEndTime: now}, FreeNotFree, &now},
		{"驱动明确长期免费", TorrentItem{DiscountLevel: DiscountFree, DiscountValidity: FreePermanent, DiscountEndTime: future}, FreePermanent, nil},
		{"驱动明确未知", TorrentItem{DiscountLevel: DiscountFree, DiscountValidity: FreeUnknown, DiscountEndTime: future}, FreeUnknown, &future},
		{"半价有结束时间", TorrentItem{DiscountLevel: DiscountPercent50, DiscountEndTime: future}, FreeNotFree, &future},
		{"没有优惠", TorrentItem{DiscountLevel: DiscountNone}, FreeNotFree, nil},
		{"非免费不受驱动标记影响", TorrentItem{DiscountLevel: DiscountNone, DiscountValidity: FreePermanent}, FreeNotFree, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.item.Promotion(now)
			assert.Equal(t, c.want, s.Validity)
			assert.Equal(t, now, s.VerifiedAt)
			if c.ends == nil {
				assert.Nil(t, s.EndsAt)
			} else if assert.NotNil(t, s.EndsAt) {
				assert.True(t, c.ends.Equal(*s.EndsAt))
			}
		})
	}
	s := (&TorrentItem{DiscountLevel: DiscountFree, DiscountSource: "promotion_rule", DiscountErr: "endTime: invalid"}).Promotion(now)
	assert.Equal(t, "promotion_rule", s.Source)
	assert.Equal(t, "endTime: invalid", s.Err)
}

func TestPromotionSnapshotFreeFor(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	margin := 5 * time.Minute
	assert.True(t, PromotionSnapshot{Validity: FreePermanent}.FreeFor(now, margin))
	assert.True(t, PromotionSnapshot{Validity: FreeUntil, EndsAt: at(6 * time.Minute)}.FreeFor(now, margin))
	assert.False(t, PromotionSnapshot{Validity: FreeUntil, EndsAt: at(5 * time.Minute)}.FreeFor(now, margin), "剩余时间等于提前量也不放行")
	assert.False(t, PromotionSnapshot{Validity: FreeUntil, EndsAt: at(-time.Second)}.FreeFor(now, margin))
	assert.False(t, PromotionSnapshot{Validity: FreeUntil}.FreeFor(now, margin), "没有结束时间的 free_until 不成立")
	assert.False(t, PromotionSnapshot{Validity: FreeUnknown, EndsAt: at(time.Hour)}.FreeFor(now, margin))
	assert.False(t, PromotionSnapshot{Validity: FreeNotFree}.FreeFor(now, margin))
}
