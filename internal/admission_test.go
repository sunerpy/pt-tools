package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func snapAt(validity v2.FreeValidity, verified time.Time, ends *time.Time) *v2.PromotionSnapshot {
	level := v2.DiscountFree
	if validity == v2.FreeNotFree {
		level = v2.DiscountNone
	}
	return &v2.PromotionSnapshot{Level: level, Validity: validity, VerifiedAt: verified, EndsAt: ends}
}

// 最后一道门禁：要求免费时只认本次尝试取得的、还在免费期内的快照；不要求免费时照常放行，不伪造免费承诺
func TestCheckAdmission(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	start := now.Add(-2 * time.Minute)
	in := func(d time.Duration) *time.Time { v := now.Add(d); return &v }
	strict := AdmissionPolicy{RequireFree: true, SafetyMargin: 5 * time.Minute, AttemptStart: start}
	strictPause := strict
	strictPause.PauseOnFreeEnd = true
	loose := AdmissionPolicy{}
	loosePause := AdmissionPolicy{PauseOnFreeEnd: true}

	cases := []struct {
		name   string
		policy AdmissionPolicy
		snap   *v2.PromotionSnapshot
		want   AdmissionDecision
	}{
		{"要求免费：免费到两小时后", strict, snapAt(v2.FreeUntil, now, in(2*time.Hour)), AdmissionDecision{Allow: true}},
		{"要求免费且到期暂停：登记监控", strictPause, snapAt(v2.FreeUntil, now, in(2*time.Hour)), AdmissionDecision{Allow: true, Monitor: true}},
		{"要求免费：剩余时间不超过提前量", strict, snapAt(v2.FreeUntil, now, in(5*time.Minute)), AdmissionDecision{Status: PushStatusRejectedExpired}},
		{"要求免费：长期免费", strictPause, snapAt(v2.FreePermanent, now, nil), AdmissionDecision{Allow: true}},
		{"要求免费：结束时间未知", strict, snapAt(v2.FreeUnknown, now, nil), AdmissionDecision{Status: PushStatusHeldFreeUnknown}},
		{"要求免费：不是免费", strict, snapAt(v2.FreeNotFree, now, nil), AdmissionDecision{Status: PushStatusRejectedNotFree}},
		{"要求免费：免费已结束", strict, &v2.PromotionSnapshot{Level: v2.DiscountFree, Validity: v2.FreeNotFree, VerifiedAt: now, EndsAt: in(-time.Minute)}, AdmissionDecision{Status: PushStatusRejectedExpired}},
		{"要求免费：没有快照", strict, nil, AdmissionDecision{Status: PushStatusRejectedStale}},
		{"要求免费：快照早于本次尝试", strict, snapAt(v2.FreeUntil, start.Add(-time.Second), in(2*time.Hour)), AdmissionDecision{Status: PushStatusRejectedStale}},
		{"要求免费：快照陈旧但记下的结束时间已过", strict, snapAt(v2.FreeUntil, start.Add(-time.Hour), in(-time.Minute)), AdmissionDecision{Status: PushStatusRejectedExpired}},
		{"要求免费：快照陈旧且记下的剩余时间不够", strict, snapAt(v2.FreeUntil, start.Add(-time.Hour), in(3*time.Minute)), AdmissionDecision{Status: PushStatusRejectedExpired}},
		{"要求免费：没有验证时间的旧记录、结束时间已过", strict, &v2.PromotionSnapshot{Level: v2.DiscountFree, Validity: v2.FreeNotFree, EndsAt: in(-time.Hour)}, AdmissionDecision{Status: PushStatusRejectedExpired}},
		{"要求免费：快照陈旧的不是免费仍按陈旧处理", strict, snapAt(v2.FreeNotFree, start.Add(-time.Hour), nil), AdmissionDecision{Status: PushStatusRejectedStale}},
		{"要求免费：快照超过 15 分钟", AdmissionPolicy{RequireFree: true, AttemptStart: now.Add(-time.Hour)}, snapAt(v2.FreeUntil, now.Add(-16*time.Minute), in(2*time.Hour)), AdmissionDecision{Status: PushStatusRejectedStale}},
		{"要求免费：没给本次尝试的开始时间", AdmissionPolicy{RequireFree: true}, snapAt(v2.FreeUntil, now, in(2*time.Hour)), AdmissionDecision{Status: PushStatusRejectedStale}},
		{"不要求免费：不是免费也放行", loose, snapAt(v2.FreeNotFree, now, nil), AdmissionDecision{Allow: true}},
		{"不要求免费：没有快照也放行", loose, nil, AdmissionDecision{Allow: true}},
		{"不要求免费、到期暂停：免费到某时就登记", loosePause, snapAt(v2.FreeUntil, now.Add(-time.Hour), in(time.Hour)), AdmissionDecision{Allow: true, Monitor: true}},
		{"不要求免费、到期暂停：结束时间未知就暂停加入", loosePause, snapAt(v2.FreeUnknown, now, nil), AdmissionDecision{Allow: true, KeepPaused: true}},
		{"不要求免费、到期暂停：没有快照就暂停加入", loosePause, nil, AdmissionDecision{Allow: true, KeepPaused: true}},
		{"不要求免费、到期暂停：不是免费不用暂停", loosePause, snapAt(v2.FreeNotFree, now, nil), AdmissionDecision{Allow: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CheckAdmission(c.policy, c.snap, now)
			got.Reason = ""
			assert.Equal(t, c.want, got)
		})
	}
	// 拒绝时写明原因（给日志、页面与 last_error）
	assert.NotEmpty(t, CheckAdmission(strict, nil, now).Reason)
	assert.NotEmpty(t, CheckAdmission(strict, snapAt(v2.FreeUnknown, now, nil), now).Reason)
}

func TestSafetyMarginFor(t *testing.T) {
	assert.Equal(t, 5*time.Minute, SafetyMarginFor(0))
	assert.Equal(t, 5*time.Minute, SafetyMarginFor(3))
	assert.Equal(t, 10*time.Minute, SafetyMarginFor(10))
	assert.Equal(t, 60*time.Minute, SafetyMarginFor(90), "提前量最多一小时")
}
