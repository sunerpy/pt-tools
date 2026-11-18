package scheduler

import (
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// brushOwnedHashes 是这台下载器（编号 dlID）上刷流任务正在管的种子（info hash，小写），按刷流记录认、不按 pt-tools-brush 标签：
// 记录是 active、下载器编号对得上；编号为 0 的旧记录按所属任务的下载器。dlID 为空（设置里查不到这台下载器）时没有记录对得上，返回空。
// 自动删种与做种竞争度跳过这些种子，由刷流任务自己的删种规则管；刷流交还（删了任务、摘掉任务标签、点了保留）以后就不在里面了。
func brushOwnedHashes(db *gorm.DB, dlID *uint) (map[string]bool, error) {
	if dlID == nil {
		return map[string]bool{}, nil
	}
	return models.NewBrushRepository(db).ActiveHashesOn(*dlID)
}
