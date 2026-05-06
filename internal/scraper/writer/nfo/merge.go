package nfo

import "os"

// mergeWithExisting 根据 mergeMode 合并旧 NFO 和新 NFO。
//
// 当前实现是 Phase 1 MVP：
//   - 旧文件不存在：直接返回新内容
//   - 旧文件存在：直接覆盖为新内容
//
// TODO: Phase 2 实现完整 merge：
//   - 解析旧 NFO 的 locked/lockedfields
//   - MergeFillEmpty: 仅填充旧 NFO 中为空的字段
//   - MergeOverwrite: 新数据完全覆盖
//   - MergeOnlyLocked: 仅保留 locked 字段，其余覆盖
func mergeWithExisting(existingPath string, newContent []byte, _ string) ([]byte, error) {
	if _, err := os.Stat(existingPath); err != nil {
		if os.IsNotExist(err) {
			return newContent, nil
		}
		return nil, err
	}

	return newContent, nil
}
