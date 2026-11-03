package freewatch

import (
	"fmt"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/utils"
)

// Notice 是一条通知的内置标题、正文与事件（消息模板用）。now 是发出的时间。
func Notice(m Message, now time.Time) (title, text string, ev *notify.Event) {
	w := m.Watch
	name := fmt.Sprintf("「%s」（%s）", w.Title, w.SiteName)
	base := map[string]any{"site": w.SiteName, "torrent_title": w.Title, "torrent_id": w.TorrentID}
	switch m.Kind {
	case KindPushed:
		size := ""
		if w.SizeBytes > 0 {
			size = utils.FormatBytes(w.SizeBytes)
		}
		base["downloader"], base["size"] = m.Downloader, size
		text = fmt.Sprintf("%s免费了，已推送到 %s。", name, m.Downloader)
		if snap := m.Snapshot; snap != nil && snap.EndsAt != nil && snap.EndsAt.After(now) {
			end, left := snap.EndsAt.In(time.Local).Format(time.DateTime), remaining(snap.EndsAt.Sub(now))
			base["free_end"], base["free_remaining"] = end, left
			text += fmt.Sprintf("\n免费到 %s（还剩 %s），到时候没下完会暂停。", end, left)
		}
		return "等到免费了，已推送", text, notify.NewEvent(notify.EventFreeWatchPushed, now, base)
	case KindExpired:
		days := max(int64(now.Sub(w.CreatedAt).Round(24*time.Hour)/(24*time.Hour)), 1)
		base["days"] = days
		text = fmt.Sprintf("%s等了 %d 天还没免费，不再等了。", name, days)
		return "等免费到期了", text, notify.NewEvent(notify.EventFreeWatchExpired, now, base)
	default:
		base["fails"], base["error"] = int64(w.Failures), m.Error
		text = fmt.Sprintf("%s连续 %d 次没成，不再等了。最近一次：%s。", name, w.Failures, m.Error)
		return "等免费没成，不再等了", text, notify.NewEvent(notify.EventFreeWatchFailed, now, base)
	}
}

// remaining 是还剩多久的写法（同 RSS 通知：23h59min、45min）。
func remaining(d time.Duration) string {
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return fmt.Sprintf("%dh%dmin", h, m)
	}
	return fmt.Sprintf("%dmin", m)
}
