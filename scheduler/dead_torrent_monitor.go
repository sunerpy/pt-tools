package scheduler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/dlassistant"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/utils"
)

const (
	deadTorrentTick         = 10 * time.Minute
	deadTorrentStartupDelay = 2 * time.Minute
	deadTorrentScanTimeout  = 15 * time.Minute
	// deadTorrentListMax 是通知里逐个列出的种子数，其余只写个数。
	deadTorrentListMax = 5
	// deadTorrentNotifySource 是这类通知在 MonitorNotificationLog 里的来源。
	deadTorrentNotifySource = "downloader_assistant"
)

// DeadTorrentDownloader 是一台要扫描的下载器。
type DeadTorrentDownloader struct {
	Name string
	DL   downloader.Downloader
}

// DeadTorrentMonitorConfig 是失效种子定时扫描的依赖。
type DeadTorrentMonitorConfig struct {
	// Settings 返回是否开启、扫描间隔与接收通知的通道。
	Settings func() (enabled bool, interval time.Duration, channelIDs []uint, err error)
	// Downloaders 返回已启用、能连上的下载器；连不上的放在 errs 里（只记日志）。
	Downloaders func(ctx context.Context) (dls []DeadTorrentDownloader, errs []error)
	Notifier    *MonitorNotifier
	// Resolver 按 tracker 识别站点；为空时用全部内置站点。
	Resolver func() *v2.TrackerResolver
	Clock    sitelogin.Clock
	Logger   *zap.SugaredLogger
	Tick     time.Duration
}

// DeadTorrentMonitor 按设置的间隔扫描各下载器里 tracker 报告未注册或不存在的种子，只发通知，不删种。
// 同一批失效种子只通知一次；新出现失效种子时再通知（列出全部）。
type DeadTorrentMonitor struct {
	cfg DeadTorrentMonitorConfig

	mu       sync.Mutex
	running  bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	lastScan time.Time
	// notified 是每台下载器上次通知过的失效种子；新一轮是它的子集时不再通知（用户删了一部分）。
	notified map[string]map[string]struct{}
}

// NewDeadTorrentMonitor 构造监控，不启动。
func NewDeadTorrentMonitor(cfg DeadTorrentMonitorConfig) *DeadTorrentMonitor {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = deadTorrentTick
	}
	if cfg.Resolver == nil {
		cfg.Resolver = v2.NewTrackerResolver
	}
	return &DeadTorrentMonitor{cfg: cfg, notified: map[string]map[string]struct{}{}}
}

// Start 启动调度循环；重复调用无效果。
func (m *DeadTorrentMonitor) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.wg.Go(func() { m.loop(ctx) })
}

// Stop 停止调度循环并等它退出（包括正在跑的一轮）。
func (m *DeadTorrentMonitor) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	cancel := m.cancel
	m.mu.Unlock()
	cancel()
	m.wg.Wait()
}

func (m *DeadTorrentMonitor) loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(deadTorrentStartupDelay):
		m.RunOnce(ctx)
	}
	ticker := time.NewTicker(m.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.RunOnce(ctx)
		}
	}
}

// RunOnce 在开启且到了间隔时扫描一轮；返回这一轮通知了几台下载器。
func (m *DeadTorrentMonitor) RunOnce(ctx context.Context) int {
	if m == nil || m.cfg.Settings == nil || m.cfg.Downloaders == nil {
		return 0
	}
	enabled, interval, channels, err := m.cfg.Settings()
	if err != nil {
		m.cfg.Logger.Warnf("[失效种子] 读取扫描设置失败: %v", err)
		return 0
	}
	if !enabled || len(channels) == 0 {
		return 0
	}
	now := m.cfg.Clock.Now()
	m.mu.Lock()
	due := m.lastScan.IsZero() || !now.Before(m.lastScan.Add(interval))
	if due {
		m.lastScan = now
	}
	m.mu.Unlock()
	if !due {
		return 0
	}

	ctx, cancel := context.WithTimeout(ctx, deadTorrentScanTimeout)
	defer cancel()
	dls, errs := m.cfg.Downloaders(ctx)
	for _, e := range errs {
		m.cfg.Logger.Warnf("[失效种子] 下载器不可用，跳过: %v", e)
	}
	resolver := m.cfg.Resolver()
	notified := 0
	for _, d := range dls {
		if ctx.Err() != nil {
			break
		}
		dead, _, err := dlassistant.ScanDeadTorrents(ctx, d.DL, resolver)
		if err != nil {
			m.cfg.Logger.Warnf("[失效种子] 扫描 %s 失败: %v", d.Name, err)
			continue
		}
		if m.notify(ctx, d.Name, dead, channels) {
			notified++
		}
	}
	return notified
}

// notify 在有新的失效种子时写一条通知；同一批种子的 EventKey 相同，不会重复发。
func (m *DeadTorrentMonitor) notify(ctx context.Context, dlName string, dead []dlassistant.DeadTorrent, channels []uint) bool {
	current := make(map[string]struct{}, len(dead))
	hashes := make([]string, 0, len(dead))
	for _, d := range dead {
		current[d.Hash] = struct{}{}
		hashes = append(hashes, d.Hash)
	}
	m.mu.Lock()
	prev := m.notified[dlName]
	fresh := false
	for h := range current {
		if _, ok := prev[h]; !ok {
			fresh = true
			break
		}
	}
	m.notified[dlName] = current
	m.mu.Unlock()
	if !fresh || m.cfg.Notifier == nil {
		return false
	}

	sort.Strings(hashes)
	sum := sha1.Sum([]byte(strings.Join(hashes, ",")))
	var total int64
	lines := make([]string, 0, deadTorrentListMax+1)
	for i, d := range dead {
		total += d.Size
		if i < deadTorrentListMax {
			lines = append(lines, "· "+d.Name)
		}
	}
	if len(dead) > deadTorrentListMax {
		lines = append(lines, fmt.Sprintf("· 其余 %d 个省略", len(dead)-deadTorrentListMax))
	}
	text := fmt.Sprintf("%s 里有 %d 个种子（共 %s）的 tracker 报告未注册或不存在。到「下载器助手 → 失效种子」确认后再删除，这里不会自动删。\n%s",
		dlName, len(dead), utils.FormatBytes(total), strings.Join(lines, "\n"))
	_, err := m.cfg.Notifier.Enqueue(ctx, MonitorNotifyEntry{
		Source:   deadTorrentNotifySource,
		Subject:  dlName,
		Kind:     "dead_torrents",
		EventKey: hex.EncodeToString(sum[:8]),
		Title:    "下载器助手：发现失效种子",
		Text:     text,
		ConfIDs:  channels,
	})
	if err != nil {
		m.cfg.Logger.Warnf("[失效种子] 写入通知失败: %v", err)
		return false
	}
	return true
}
