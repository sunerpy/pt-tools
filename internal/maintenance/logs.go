// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// cleanLogs 清理 logs 白名单根下的轮转日志备份。
//
// 规则与 config.Zap.PruneOldLogs 完全一致（不引入分叉）：对每个 base
// (all/debug/info/error) 的备份 <base>-*.log(.gz) 按 mtime 倒序，删除超过
// MaxBackups 数量或早于 MaxAge 的备份；base 文件本身（all.log 等）永不进入候选，
// 且作为红线二次保险由 candidateInRoot 拦截。删除均经 candidateInRoot 白名单校验。
func (c *Cleaner) cleanLogs(resolvedRoot string, dryRun bool, cr *CategoryResult) {
	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		return
	}

	type backup struct {
		name string
		mod  time.Time
	}
	groups := map[string][]backup{}
	for _, base := range []string{"all", "debug", "info", "error"} {
		prefix := base + "-"
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			if !strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz") {
				continue
			}
			info, ierr := e.Info()
			if ierr != nil {
				continue
			}
			groups[base] = append(groups[base], backup{name: name, mod: info.ModTime()})
		}
	}

	cutoff := time.Now().Add(-time.Duration(c.zapCfg.MaxAge) * 24 * time.Hour)
	for _, list := range groups {
		sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
		for idx, b := range list {
			tooMany := c.zapCfg.MaxBackups > 0 && idx >= c.zapCfg.MaxBackups
			tooOld := c.zapCfg.MaxAge > 0 && b.mod.Before(cutoff)
			if !tooMany && !tooOld {
				continue
			}
			reason := "超过保留份数"
			if tooOld {
				reason = "超过保留天数"
			}
			candidate := filepath.Join(resolvedRoot, b.name)
			c.applyDelete(resolvedRoot, candidate, reason, dryRun, cr)
		}
	}
}

// applyDelete 对单个候选执行白名单校验 + （非 dry-run 时）删除，并记录到 CategoryResult。
func (c *Cleaner) applyDelete(resolvedRoot, candidate, reason string, dryRun bool, cr *CategoryResult) {
	ok, why := candidateInRoot(resolvedRoot, candidate)
	if !ok {
		cr.Skipped = append(cr.Skipped, why)
		sLogger().Warnf("[清理] 跳过: %s", why)
		return
	}
	size := fileSize(candidate)
	if !dryRun {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			cr.Skipped = append(cr.Skipped, candidate+": 删除失败: "+err.Error())
			sLogger().Errorf("[清理] 删除失败: %s, %v", candidate, err)
			return
		}
	}
	cr.Deleted = append(cr.Deleted, FileAction{Path: candidate, SizeBytes: size, Reason: reason})
	cr.FreedBytes += size
}
