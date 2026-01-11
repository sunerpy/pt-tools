// Package v2 provides database-backed user info repository
package v2

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// UserInfoRecord represents the database model for user info
type UserInfoRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Site       string    `gorm:"uniqueIndex;size:64;not null" json:"site"`
	Username   string    `gorm:"size:128" json:"username"`
	UserID     string    `gorm:"size:64" json:"userId"`
	Rank       string    `gorm:"size:64" json:"rank"`
	Uploaded   int64     `json:"uploaded"`
	Downloaded int64     `json:"downloaded"`
	Ratio      float64   `json:"ratio"`
	Seeding    int       `json:"seeding"`
	Leeching   int       `json:"leeching"`
	Bonus      float64   `json:"bonus"`
	JoinDate   int64     `json:"joinDate"`
	LastAccess int64     `json:"lastAccess"`
	LastUpdate int64     `json:"lastUpdate"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	// Extended fields
	LevelName           string  `gorm:"size:64" json:"levelName"`
	LevelID             int     `json:"levelId"`
	BonusPerHour        float64 `json:"bonusPerHour"`
	SeedingBonus        float64 `json:"seedingBonus"`
	SeedingBonusPerHour float64 `json:"seedingBonusPerHour"`
	UnreadMessageCount  int     `json:"unreadMessageCount"`
	TotalMessageCount   int     `json:"totalMessageCount"`
	SeederCount         int     `json:"seederCount"`
	SeederSize          int64   `json:"seederSize"`
	LeecherCount        int     `json:"leecherCount"`
	LeecherSize         int64   `json:"leecherSize"`
	HnRUnsatisfied      int     `json:"hnrUnsatisfied"`
	HnRPreWarning       int     `json:"hnrPreWarning"`
	TrueUploaded        int64   `json:"trueUploaded"`
	TrueDownloaded      int64   `json:"trueDownloaded"`
	Uploads             int     `json:"uploads"`
}

// TableName returns the table name for UserInfoRecord
func (UserInfoRecord) TableName() string {
	return "user_info"
}

// ToUserInfo converts UserInfoRecord to UserInfo
func (r *UserInfoRecord) ToUserInfo() UserInfo {
	return UserInfo{
		Site:                r.Site,
		Username:            r.Username,
		UserID:              r.UserID,
		Rank:                r.Rank,
		Uploaded:            r.Uploaded,
		Downloaded:          r.Downloaded,
		Ratio:               r.Ratio,
		Seeding:             r.Seeding,
		Leeching:            r.Leeching,
		Bonus:               r.Bonus,
		JoinDate:            r.JoinDate,
		LastAccess:          r.LastAccess,
		LastUpdate:          r.LastUpdate,
		LevelName:           r.LevelName,
		LevelID:             r.LevelID,
		BonusPerHour:        r.BonusPerHour,
		SeedingBonus:        r.SeedingBonus,
		SeedingBonusPerHour: r.SeedingBonusPerHour,
		UnreadMessageCount:  r.UnreadMessageCount,
		TotalMessageCount:   r.TotalMessageCount,
		SeederCount:         r.SeederCount,
		SeederSize:          r.SeederSize,
		LeecherCount:        r.LeecherCount,
		LeecherSize:         r.LeecherSize,
		HnRUnsatisfied:      r.HnRUnsatisfied,
		HnRPreWarning:       r.HnRPreWarning,
		TrueUploaded:        r.TrueUploaded,
		TrueDownloaded:      r.TrueDownloaded,
		Uploads:             r.Uploads,
	}
}

// FromUserInfo creates a UserInfoRecord from UserInfo
func FromUserInfo(info UserInfo) UserInfoRecord {
	return UserInfoRecord{
		Site:                info.Site,
		Username:            info.Username,
		UserID:              info.UserID,
		Rank:                info.Rank,
		Uploaded:            info.Uploaded,
		Downloaded:          info.Downloaded,
		Ratio:               info.Ratio,
		Seeding:             info.Seeding,
		Leeching:            info.Leeching,
		Bonus:               info.Bonus,
		JoinDate:            info.JoinDate,
		LastAccess:          info.LastAccess,
		LastUpdate:          info.LastUpdate,
		LevelName:           info.LevelName,
		LevelID:             info.LevelID,
		BonusPerHour:        info.BonusPerHour,
		SeedingBonus:        info.SeedingBonus,
		SeedingBonusPerHour: info.SeedingBonusPerHour,
		UnreadMessageCount:  info.UnreadMessageCount,
		TotalMessageCount:   info.TotalMessageCount,
		SeederCount:         info.SeederCount,
		SeederSize:          info.SeederSize,
		LeecherCount:        info.LeecherCount,
		LeecherSize:         info.LeecherSize,
		HnRUnsatisfied:      info.HnRUnsatisfied,
		HnRPreWarning:       info.HnRPreWarning,
		TrueUploaded:        info.TrueUploaded,
		TrueDownloaded:      info.TrueDownloaded,
		Uploads:             info.Uploads,
	}
}

// DBUserInfoRepo is a database-backed implementation of UserInfoRepo
type DBUserInfoRepo struct {
	db *gorm.DB
}

// NewDBUserInfoRepo creates a new database-backed user info repository
func NewDBUserInfoRepo(db *gorm.DB) (*DBUserInfoRepo, error) {
	// Auto-migrate the table
	if err := db.AutoMigrate(&UserInfoRecord{}); err != nil {
		return nil, err
	}
	return &DBUserInfoRepo{db: db}, nil
}

// Save stores user info for a site (upsert)
func (r *DBUserInfoRepo) Save(ctx context.Context, info UserInfo) error {
	if info.Site == "" {
		return ErrSiteNotFound
	}

	info.LastUpdate = time.Now().Unix()
	record := FromUserInfo(info)

	// Use upsert: update if exists, insert if not
	return r.db.WithContext(ctx).
		Where("site = ?", info.Site).
		Assign(record).
		FirstOrCreate(&record).Error
}

// Get retrieves user info for a specific site
func (r *DBUserInfoRepo) Get(ctx context.Context, site string) (UserInfo, error) {
	var record UserInfoRecord
	err := r.db.WithContext(ctx).Where("site = ?", site).First(&record).Error
	if err == gorm.ErrRecordNotFound {
		return UserInfo{}, ErrSiteNotFound
	}
	if err != nil {
		return UserInfo{}, err
	}
	return record.ToUserInfo(), nil
}

// ListAll retrieves all stored user info
func (r *DBUserInfoRepo) ListAll(ctx context.Context) ([]UserInfo, error) {
	var records []UserInfoRecord
	if err := r.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]UserInfo, len(records))
	for i, record := range records {
		result[i] = record.ToUserInfo()
	}
	return result, nil
}

// ListBySites retrieves user info for specific sites
func (r *DBUserInfoRepo) ListBySites(ctx context.Context, sites []string) ([]UserInfo, error) {
	var records []UserInfoRecord
	if err := r.db.WithContext(ctx).Where("site IN ?", sites).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]UserInfo, len(records))
	for i, record := range records {
		result[i] = record.ToUserInfo()
	}
	return result, nil
}

// Delete removes user info for a site
func (r *DBUserInfoRepo) Delete(ctx context.Context, site string) error {
	result := r.db.WithContext(ctx).Where("site = ?", site).Delete(&UserInfoRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSiteNotFound
	}
	return nil
}

// GetAggregated calculates aggregated statistics
func (r *DBUserInfoRepo) GetAggregated(ctx context.Context) (AggregatedStats, error) {
	records, err := r.ListAll(ctx)
	if err != nil {
		return AggregatedStats{}, err
	}

	stats := AggregatedStats{
		LastUpdate:   time.Now().Unix(),
		PerSiteStats: records,
		SiteCount:    len(records),
	}

	var totalRatio float64
	var ratioCount int

	for _, info := range records {
		stats.TotalUploaded += info.Uploaded
		stats.TotalDownloaded += info.Downloaded
		stats.TotalSeeding += info.Seeding
		stats.TotalLeeching += info.Leeching
		stats.TotalBonus += info.Bonus

		// Aggregate extended fields
		stats.TotalBonusPerHour += info.BonusPerHour
		stats.TotalSeedingBonus += info.SeedingBonus
		stats.TotalUnreadMessages += info.UnreadMessageCount
		stats.TotalSeederSize += info.SeederSize
		stats.TotalLeecherSize += info.LeecherSize

		// Only count valid ratios for average
		if info.Ratio > 0 && info.Ratio < 1000 {
			totalRatio += info.Ratio
			ratioCount++
		}
	}

	if ratioCount > 0 {
		stats.AverageRatio = totalRatio / float64(ratioCount)
	}

	return stats, nil
}

// DeleteAll removes all user info records
func (r *DBUserInfoRepo) DeleteAll(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&UserInfoRecord{}).Error
}

// Count returns the number of stored user info entries
func (r *DBUserInfoRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&UserInfoRecord{}).Count(&count).Error
	return count, err
}
