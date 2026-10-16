package internal

import (
	"fmt"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 免费资格的入场门禁（docs/design/free-expiry-safety.md §4.2、§5.3 与附录 B）。

// 推送入口（AdmissionPolicy.SourceKind）
const (
	SourceKindRSS           = "rss"            // RSS 自动推送
	SourceKindRSSNotify     = "rss_notify"     // RSS 通知里的「立即下载」按钮
	SourceKindManual        = "manual"         // 网页里手动推送
	SourceKindApp           = "app"            // App API（含 MCP、AI 助手）
	SourceKindSubscribe     = "subscribe"      // 媒体订阅
	SourceKindBrush         = "brush"          // 刷流
	SourceKindQbitCompat    = "qbit_compat"    // qB 兼容入口
	SourceKindTransfer      = "transfer"       // 转移做种、辅种（复用已有数据）
	SourceKindDownloaderAdd = "downloader_add" // 下载器页面直接加任务
)

const (
	// SnapshotMaxAge 是要求免费时资格快照最多能旧多久（另外还不能早于本次尝试的开始）
	SnapshotMaxAge = 15 * time.Minute
	// MinSafetyMargin 是要求免费时免费剩余时间至少要超过的量
	MinSafetyMargin = 5 * time.Minute
	// maxSafetyMargin 与免费结束监控的提前量上限一致
	maxSafetyMargin = time.Hour
)

// AdmissionPolicy 是一次推送的入场策略。零值是「不要求免费、不承诺暂停」，即明确的手动推送。
type AdmissionPolicy struct {
	// RequireFree 为 true 时只有本次尝试取得、仍在免费期内的快照才放行
	RequireFree bool
	// PauseOnFreeEnd 为 true 时承诺免费结束时暂停（或按设置删除）
	PauseOnFreeEnd bool
	// SafetyMargin 是免费剩余时间至少要超过的量（见 SafetyMarginFor）
	SafetyMargin time.Duration
	// SourceRSSID 是这个推送所属的 RSS（RSS 自动推送与通知按钮）
	SourceRSSID *uint
	// SourceKind 是推送入口，见 SourceKind* 常量
	SourceKind string
	// AttemptStart 是本次尝试开始的时间（RSS 是这一轮的开始）。RequireFree 时必须给出，快照不能早于它
	AttemptStart time.Time
	// Clock 是调用方的时钟（刷流、订阅在测试里用假时钟）；为空时用 time.Now
	Clock func() time.Time
}

// Now 是门禁判断用的当前时间。
func (p AdmissionPolicy) Now() time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}

// DefaultSafetyMargin 是按全局免费结束提前量算出的安全余量（读不到设置时按 0，即 5 分钟）。
func DefaultSafetyMargin() time.Duration {
	return SafetyMarginFor(freeEndAdvanceMinutes())
}

// SafetyMarginFor 按免费结束提前量（分钟）给出安全余量：不少于 5 分钟，不超过一小时。
func SafetyMarginFor(advanceMinutes int) time.Duration {
	m := time.Duration(advanceMinutes) * time.Minute
	return min(max(m, MinSafetyMargin), maxSafetyMargin)
}

// PushStatus 是推送结果的机器可判定状态（§8.1）。
type PushStatus string

const (
	PushStatusPushed            PushStatus = "pushed"
	PushStatusPushedPaused      PushStatus = "pushed_paused"
	PushStatusMonitorDegraded   PushStatus = "pushed_monitor_degraded"
	PushStatusDuplicate         PushStatus = "skipped_duplicate"
	PushStatusHeldFreeUnknown   PushStatus = "held_free_unknown"
	PushStatusRejectedNotFree   PushStatus = "rejected_not_free"
	PushStatusRejectedExpired   PushStatus = "rejected_free_expired"
	PushStatusRejectedStale     PushStatus = "rejected_free_stale"
	PushStatusRejectedPlacement PushStatus = "rejected_active_placement"
	PushStatusRejectedDisk      PushStatus = "rejected_disk_space"
	PushStatusRejectedTooLarge  PushStatus = "rejected_too_large"
	PushStatusRejectedCapacity  PushStatus = "rejected_site_capacity"
	PushStatusFailed            PushStatus = "failed"
)

// IsFreeRejection 报告状态是不是免费资格门禁的拒绝（不是磁盘、容量这类暂时问题）。
func (s PushStatus) IsFreeRejection() bool {
	switch s {
	case PushStatusHeldFreeUnknown, PushStatusRejectedNotFree, PushStatusRejectedExpired, PushStatusRejectedStale:
		return true
	}
	return false
}

// AdmissionDecision 是门禁的结论。
type AdmissionDecision struct {
	Allow bool
	// Status 是不放行时的状态
	Status PushStatus
	// Monitor：承诺了到期暂停且结束时间可信，要登记免费结束监控（先暂停加入，记账成功后再开始）
	Monitor bool
	// KeepPaused：承诺了到期暂停但结束时间不可信，只能暂停加入，不能自动开始
	KeepPaused bool
	// Reason 是给日志、页面与 last_error 的说明
	Reason string
}

// CheckAdmission 是加入下载器之前的最后一道纯函数判定。
func CheckAdmission(p AdmissionPolicy, snap *v2.PromotionSnapshot, now time.Time) AdmissionDecision {
	if !p.RequireFree {
		d := AdmissionDecision{Allow: true}
		if p.PauseOnFreeEnd {
			switch {
			case snap == nil || snap.Validity == v2.FreeUnknown || snap.Validity == "":
				d.KeepPaused = true
				d.Reason = "免费结束时间未知，无法保证到期暂停，暂停加入、不自动开始"
			case snap.Validity == v2.FreeUntil && snap.EndsAt != nil && snap.EndsAt.After(now):
				d.Monitor = true
			}
		}
		return d
	}

	margin := p.SafetyMargin
	if margin < MinSafetyMargin {
		margin = MinSafetyMargin
	}
	// 已知的免费结束时间已经过去或剩余不超过安全余量：不用等重新读取，读得再新也不会放行
	if snap != nil && snap.EndsAt != nil && v2.IsFreeTorrent(snap.Level) && snap.EndsAt.Sub(now) <= margin &&
		(snap.Validity == v2.FreeUntil || snap.Validity == v2.FreeNotFree) {
		if snap.EndsAt.After(now) {
			return reject(PushStatusRejectedExpired, fmt.Sprintf("免费剩余时间不超过 %d 分钟，不推送", int(margin/time.Minute)))
		}
		return reject(PushStatusRejectedExpired, "免费已结束，不推送")
	}
	switch {
	case snap == nil || snap.VerifiedAt.IsZero():
		return reject(PushStatusRejectedStale, "没有本次读取的免费资格，不推送")
	case p.AttemptStart.IsZero() || snap.VerifiedAt.Before(p.AttemptStart):
		return reject(PushStatusRejectedStale, "免费资格不是本次读取的，不推送")
	case now.Sub(snap.VerifiedAt) > SnapshotMaxAge:
		return reject(PushStatusRejectedStale, fmt.Sprintf("免费资格读取已超过 %d 分钟，不推送", int(SnapshotMaxAge/time.Minute)))
	}
	switch snap.Validity {
	case v2.FreePermanent:
		return AdmissionDecision{Allow: true}
	case v2.FreeUntil:
		if snap.FreeFor(now, margin) {
			return AdmissionDecision{Allow: true, Monitor: p.PauseOnFreeEnd}
		}
		return reject(PushStatusRejectedExpired, fmt.Sprintf("免费剩余时间不超过 %d 分钟，不推送", int(margin/time.Minute)))
	case v2.FreeUnknown:
		return reject(PushStatusHeldFreeUnknown, "免费结束时间未知，暂不推送")
	}
	return reject(PushStatusRejectedNotFree, "不是免费，不推送")
}

func reject(status PushStatus, reason string) AdmissionDecision {
	return AdmissionDecision{Status: status, Reason: reason}
}
