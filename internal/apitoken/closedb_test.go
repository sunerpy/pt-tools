package apitoken

import "gorm.io/gorm"

// closeTestGorm 关掉测试库：Windows 上打开着的文件删不掉，不关的话 t.TempDir() 的清理会失败。
func closeTestGorm(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
