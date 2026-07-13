// MIT License
// Copyright (c) 2025 pt-tools

package maintenance

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// cleanBackups 清理 backups 白名单根下的旧备份：保留最近 keep 份 *.json（按 mtime
// 倒序），删除更旧的。这是新增的 retention（此前 backups 无清理），keep 由
// CleanOptions.KeepBackups 提供，默认 5。
func (c *Cleaner) cleanBackups(resolvedRoot string, keep int, dryRun bool, cr *CategoryResult) {
	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		return
	}
	type backup struct {
		name string
		mod  time.Time
	}
	var list []backup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		list = append(list, backup{name: e.Name(), mod: info.ModTime()})
	}
	// 按 mtime 倒序（最新在前），保留前 keep 个，其余删除。
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })
	for idx, b := range list {
		if idx < keep {
			continue
		}
		candidate := filepath.Join(resolvedRoot, b.name)
		c.applyDelete(resolvedRoot, candidate, "超过保留份数", dryRun, cr)
	}
}
