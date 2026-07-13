// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// CleanCategory 标识一个可清理类别。清理只允许发生在这些类别对应的白名单根内。
type CleanCategory string

const (
	// CategoryLogs 清理 ~/.pt-tools/logs 下的轮转日志备份（保留 base 文件）。
	CategoryLogs CleanCategory = "logs"
	// CategoryStaging 清理 ~/.pt-tools/downloads 下所有 tag 子目录中已无意义的 .torrent。
	CategoryStaging CleanCategory = "staging"
	// CategoryBackups 清理 ~/.pt-tools/backups 下的旧备份（保留最近 N 份）。
	CategoryBackups CleanCategory = "backups"
)

// allCategories 是默认（Categories 为空时）清理的全部类别，顺序固定以便输出稳定。
var allCategories = []CleanCategory{CategoryLogs, CategoryStaging, CategoryBackups}

// defaultKeepBackups 是 backups 保留策略的默认值（保留最近 N 份 *.json）。
const defaultKeepBackups = 5

// CleanOptions 控制一次清理行为。
type CleanOptions struct {
	// Categories 指定要清理的类别；为空表示全部三类。
	Categories []CleanCategory
	// DryRun 为 true 时只计算将删除的文件，不实际删除。
	DryRun bool
	// KeepBackups 为 backups 类别保留的最近份数；<=0 时使用默认值 5。
	KeepBackups int
}

// FileAction 描述一个（将）被删除的文件。
type FileAction struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	Reason    string `json:"reason"`
}

// CategoryResult 汇总单个类别的清理结果。
type CategoryResult struct {
	Category   CleanCategory `json:"category"`
	Deleted    []FileAction  `json:"deleted"` // DryRun 时为"将删除"项
	FreedBytes int64         `json:"freedBytes"`
	Skipped    []string      `json:"skipped"` // 命中红线/越界而被拒绝的路径（含原因）
	Note       string        `json:"note"`    // 例如整类被拒绝的原因
}

// CleanResult 汇总一次清理的全部结果。
type CleanResult struct {
	DryRun     bool             `json:"dryRun"`
	Categories []CategoryResult `json:"categories"`
}

// Cleaner 是共享清理服务。它只在 ~/.pt-tools 下的三个固定白名单根内操作。
type Cleaner struct {
	homeDir string
	db      *models.TorrentDB
	zapCfg  config.Zap
}

// NewCleaner 构造 Cleaner。homeDir 显式注入，便于测试用 t.TempDir() 作为 fake home；
// db 用于 staging 类别的 DB 判定；zapCfg 提供日志保留策略（MaxAge/MaxBackups/Directory）。
func NewCleaner(homeDir string, db *models.TorrentDB, zapCfg config.Zap) *Cleaner {
	return &Cleaner{homeDir: homeDir, db: db, zapCfg: zapCfg}
}

func sLogger() *zap.SugaredLogger {
	if global.GetLogger() == nil {
		return zap.NewNop().Sugar()
	}
	return global.GetSlogger()
}

// Clean 执行清理并返回结果。任何单文件被拒绝（红线/越界）不会中断整体清理。
func (c *Cleaner) Clean(_ context.Context, opts CleanOptions) (*CleanResult, error) {
	// 1) 解析工作目录本身；失败则放弃整体清理（无法安全判定边界）。
	homeWork := filepath.Join(c.homeDir, models.WorkDir)
	resolvedHomeWork, err := filepath.EvalSymlinks(homeWork)
	if err != nil {
		return nil, fmt.Errorf("解析工作目录失败，放弃清理: %w", err)
	}

	cats := opts.Categories
	if len(cats) == 0 {
		cats = allCategories
	}
	// 去重并保持稳定顺序。
	requested := map[CleanCategory]bool{}
	for _, c := range cats {
		requested[c] = true
	}

	result := &CleanResult{DryRun: opts.DryRun}
	for _, cat := range allCategories {
		if !requested[cat] {
			continue
		}
		cr := c.cleanCategory(cat, resolvedHomeWork, opts)
		if cr != nil {
			result.Categories = append(result.Categories, *cr)
		}
	}
	return result, nil
}

// cleanCategory 校验类别白名单根后分派到对应处理器。返回 nil 表示该根不存在（无可清理）。
func (c *Cleaner) cleanCategory(cat CleanCategory, resolvedHomeWork string, opts CleanOptions) *CategoryResult {
	rootName := string(cat)
	if cat == CategoryStaging {
		rootName = "downloads"
	}
	root := filepath.Join(c.homeDir, models.WorkDir, rootName)

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		// 根不存在 → 无可清理，直接跳过（不产生 CategoryResult）。
		return nil
	}

	cr := &CategoryResult{Category: cat}

	// 允许根自身的 symlink 防护：解析后必须仍落在 resolved ~/.pt-tools 之内，
	// 否则整类拒绝清理（momus BLOCKING #2b）。
	if !withinRoot(resolvedHomeWork, resolvedRoot) {
		cr.Note = fmt.Sprintf("类别 %s 拒绝清理：根 %s 解析软链后越出 %s", cat, root, resolvedHomeWork)
		sLogger().Warnf("[清理] %s", cr.Note)
		return cr
	}

	switch cat {
	case CategoryLogs:
		c.cleanLogs(resolvedRoot, opts.DryRun, cr)
	case CategoryStaging:
		c.cleanStaging(resolvedRoot, opts.DryRun, cr)
	case CategoryBackups:
		keep := opts.KeepBackups
		if keep <= 0 {
			keep = defaultKeepBackups
		}
		c.cleanBackups(resolvedRoot, keep, opts.DryRun, cr)
	}
	return cr
}

// candidateInRoot 校验单个候选删除路径是否安全地落在 resolvedRoot 之内，且不命中红线。
// 返回 (ok, reason)：ok=false 时 reason 说明拒绝原因（记录到 Skipped）。
func candidateInRoot(resolvedRoot, candidate string) (bool, string) {
	resolvedCandidate, err := evalSymlinksExisting(candidate)
	if err != nil {
		return false, fmt.Sprintf("%s: 无法解析路径: %v", candidate, err)
	}
	if !withinRoot(resolvedRoot, resolvedCandidate) {
		return false, fmt.Sprintf("%s: 解析后越出白名单根 %s（软链/路径逃逸）", candidate, resolvedRoot)
	}
	if isRedLine(resolvedCandidate) || isRedLine(candidate) {
		return false, fmt.Sprintf("%s: 命中红线保护，拒绝删除", candidate)
	}
	return true, ""
}

// isRedLine 判定 basename 是否为硬编码红线文件（二次保险）。
func isRedLine(p string) bool {
	switch filepath.Base(p) {
	case models.DBFile, // torrents.db
		models.DBFile + "-wal",
		models.DBFile + "-shm",
		"secret.key",
		"all.log", "debug.log", "info.log", "error.log":
		return true
	}
	return false
}

// withinRoot 报告 target 是否位于 root 之内（含 root 自身），基于 filepath.Rel。
func withinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}
