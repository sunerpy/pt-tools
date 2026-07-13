// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"os"
	"path/filepath"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// cleanStaging 清理 downloads 白名单根下所有 tag 子目录（以及根目录本身）里已无意义
// 的 .torrent。相较 #450 的 sweep（只处理当前 RSS 订阅的 tag 目录），特性 D 扫描
// 根下的**全部** tag 子目录（不依赖当前订阅），这是它的增量价值（P6）。
//
// 扫描根固定为 ~/.pt-tools/downloads（白名单内），绝不读取 DownloadDir 配置——
// 若用户把 DownloadDir 配成 .pt-tools 之外的绝对路径，本特性一律不碰那个目录（§2.4）。
//
// defaultStagingRetainHours 是手动清理（特性 D）在 RetainHours<=0（未配置/被清零）时
// 使用的兜底保留期。手动清理是用户明确意图，不应被"自动清理阈值"整类卡死。
const defaultStagingRetainHours = 24

// 判定复用 shouldSweepAnySite（与 #450 shouldSweep 相同规则，但按 hash 跨站点查 DB）。
// RetainHours/MaxRetry 从 ConfigStore 读取；RetainHours<=0 时改用兜底 24h 继续清理（孤立/
// 已推送/达最大重试与 mtime 无关，仍应清；超期未推送用兜底判定），而非整类禁用。
// 注意：#450 的 internal.sweepStagingDir（RSS 自动 sweep）保持 retain<=0 → 禁用的旧语义不变。
func (c *Cleaner) cleanStaging(resolvedRoot string, dryRun bool, cr *CategoryResult) {
	retainHours, maxRetry := c.stagingRetention()
	if retainHours <= 0 {
		retainHours = defaultStagingRetainHours
		cr.Note = "使用默认保留期 24h（未配置 RetainHours）"
	}
	for _, file := range collectTorrentFiles(resolvedRoot) {
		if !shouldSweepAnySite(c.db, file, retainHours, maxRetry) {
			continue
		}
		c.applyDelete(resolvedRoot, file, "暂存种子已无意义（已推送/孤立/达最大重试/超保留期）", dryRun, cr)
	}
}

// stagingRetention 从 ConfigStore 读取 RetainHours 与 MaxRetry；DB 不可用时返回 (0,0) 禁用。
func (c *Cleaner) stagingRetention() (retainHours, maxRetry int) {
	if c.db == nil {
		return 0, 0
	}
	gl, err := core.NewConfigStore(c.db).GetGlobalOnly()
	if err != nil {
		return 0, 0
	}
	return gl.RetainHours, gl.MaxRetry
}

// collectTorrentFiles 递归收集 root 下所有 .torrent（根目录 + 每个 tag 子目录）。
func collectTorrentFiles(root string) []string {
	var out []string
	// 根目录本身。
	if files, err := qbit.GetTorrentFilesPath(root); err == nil {
		out = append(out, files...)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(root, e.Name())
		if files, err := qbit.GetTorrentFilesPath(sub); err == nil {
			out = append(out, files...)
		}
	}
	return out
}
