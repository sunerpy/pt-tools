package models

import "time"

// QbitCompatSetting 是 qB 兼容入口的设置（只有 ID 1 一行，路线图 M13）。监听地址不在这里：它是启动参数。
type QbitCompatSetting struct {
	ID uint `gorm:"primaryKey" json:"-"`
	// DownloaderID 是兼容入口背后的下载器；0 表示默认下载器
	DownloaderID uint `gorm:"not null;default:0" json:"downloader_id"`
	// FullControl 打开后，写接口能动下载器里的全部种子；关着时只动经兼容入口加的
	FullControl bool `gorm:"not null;default:false" json:"full_control"`
	// Categories 是客户端用 createCategory 建的分类（JSON：名字 → 保存目录）；加种子时没给目录就用它
	Categories string `gorm:"type:text;not null;default:''" json:"-"`
	// Tags 是客户端用 createTags 建的标签（JSON 数组）
	Tags      string    `gorm:"type:text;not null;default:''" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (QbitCompatSetting) TableName() string { return "qbit_compat_settings" }

// QbitCompatTorrent 记下经 qB 兼容入口加进下载器的种子：没打开完全控制时，写接口只动这些。
// 只在种子确实加进下载器（不是原来就在）以后才记，经兼容入口删掉时一起删。
// AddedAt 是下载器给这个种子的添加时间：只认添加时间一样的那一个（删掉以后从别处加回来的时间不一样）；
// 加的时候下载器还没列出它时是 0，第一次看到、添加时间就在 CreatedAt 前后时补上。
type QbitCompatTorrent struct {
	ID           uint      `gorm:"primaryKey"`
	DownloaderID uint      `gorm:"not null;uniqueIndex:idx_qbit_compat_owner"`
	InfoHash     string    `gorm:"size:64;not null;uniqueIndex:idx_qbit_compat_owner"`
	AddedAt      int64     `gorm:"not null;default:0"`
	CreatedAt    time.Time `gorm:"not null"`
}

func (QbitCompatTorrent) TableName() string { return "qbit_compat_torrents" }
