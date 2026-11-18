package web

import (
	"sync"
	"time"
)

// 首页 NAS 状态的速度柱（媒体中心 M8）：每算完一次全部下载器的总速度记一条采样，存在内存里，重启清空。
// 不另外轮询：外壳的状态栏本来就每 30 秒取一次 transfer-stats，没人开着网页时不采样。

const (
	// speedHistoryLen 是最多留几条采样
	speedHistoryLen = 20
	// speedSlot 是一格多长：同一格里只记第一条（几个标签页一起开着时不会一拍记几条）
	speedSlot = 30 * time.Second
)

// SpeedSample 是一拍全部下载器合成的总速度（字节/秒）。Answered 是这一拍连上了的下载器台数，Total 是启用的台数：
// Answered < Total 时总和只含连上的那几台。
type SpeedSample struct {
	At       time.Time `json:"at"`
	Down     int64     `json:"down"`
	Up       int64     `json:"up"`
	Answered int       `json:"answered"`
	Total    int       `json:"total"`
}

// speedHistory 是最近 speedHistoryLen 条采样（旧的在前）。
type speedHistory struct {
	mu   sync.Mutex
	list []SpeedSample
}

// record 记一条采样：和上一条在同一格（speedSlot）里时不记；超过 speedHistoryLen 条时丢掉最旧的。
func (h *speedHistory) record(s SpeedSample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n := len(h.list); n > 0 && h.list[n-1].At.Truncate(speedSlot).Equal(s.At.Truncate(speedSlot)) {
		return
	}
	h.list = append(h.list, s)
	if over := len(h.list) - speedHistoryLen; over > 0 {
		h.list = append([]SpeedSample(nil), h.list[over:]...)
	}
}

// snapshot 是采样的副本（旧的在前）。
func (h *speedHistory) snapshot() []SpeedSample {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]SpeedSample{}, h.list...)
}
