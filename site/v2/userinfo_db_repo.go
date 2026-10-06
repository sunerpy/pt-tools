// Package v2 provides database-backed user info repository
package v2

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	// now / loc 决定快照的日期（进程时区）；测试用 SetClock 换掉
	now func() time.Time
	loc *time.Location
}

var _ UserInfoHistoryRepo = (*DBUserInfoRepo)(nil)

// NewDBUserInfoRepo creates a new database-backed user info repository
func NewDBUserInfoRepo(db *gorm.DB) (*DBUserInfoRepo, error) {
	// Auto-migrate the table
	if err := db.AutoMigrate(&UserInfoRecord{}, &UserInfoDailySnapshot{}); err != nil {
		return nil, err
	}
	return &DBUserInfoRepo{db: db, now: time.Now, loc: time.Local}, nil
}

// SetClock 换掉取当前时间的函数与计算日期的时区（测试用）。
func (r *DBUserInfoRepo) SetClock(now func() time.Time, loc *time.Location) {
	if now != nil {
		r.now = now
	}
	if loc != nil {
		r.loc = loc
	}
}

func (r *DBUserInfoRepo) clock() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}

func (r *DBUserInfoRepo) dayOf(t time.Time) string {
	loc := r.loc
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Format(dateLayout)
}

// Today 是仓库时区的当天日期（YYYY-MM-DD）。
func (r *DBUserInfoRepo) Today() string {
	return r.dayOf(r.clock())
}

// Save stores user info for a site (upsert)
//
// 记录与当天的快照在同一个事务里写：快照写失败时记录也回滚，调用方（UserInfoService.FetchAndSave）
// 据此返回 ErrUserInfoPersist。同一天之后任意一次成功获取都会再次 upsert 当天的快照。
func (r *DBUserInfoRepo) Save(ctx context.Context, info UserInfo) error {
	if info.Site == "" {
		return ErrSiteNotFound
	}

	now := r.clock()
	info.LastUpdate = now.Unix()
	record := FromUserInfo(info)
	snapshot := UserInfoDailySnapshot{
		Site:       info.Site,
		Date:       r.dayOf(now),
		Uploaded:   info.Uploaded,
		Downloaded: info.Downloaded,
		Bonus:      info.Bonus,
		Ratio:      info.Ratio,
		Seeding:    info.Seeding,
		SeederSize: info.SeederSize,
		CapturedAt: now.Unix(),
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use upsert: update if exists, insert if not
		if err := tx.Where("site = ?", info.Site).Assign(record).FirstOrCreate(&record).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "site"}, {Name: "date"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"uploaded", "downloaded", "bonus", "ratio", "seeding", "seeder_size", "captured_at", "updated_at",
			}),
		}).Create(&snapshot).Error
	})
}

// ListSnapshots 返回 [from, to]（含两端）之间的快照，按站点、日期升序；site 为空时返回所有站点。
func (r *DBUserInfoRepo) ListSnapshots(ctx context.Context, site, from, to string) ([]UserInfoDailySnapshot, error) {
	q := r.db.WithContext(ctx).Where("date >= ? AND date <= ?", from, to)
	if site != "" {
		q = q.Where("site = ?", site)
	}
	var out []UserInfoDailySnapshot
	if err := q.Order("site ASC, date ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// SnapshotBaselines 返回每站在 before 之前（不含）最近的一份快照。
func (r *DBUserInfoRepo) SnapshotBaselines(ctx context.Context, before string) (map[string]UserInfoDailySnapshot, error) {
	var rows []UserInfoDailySnapshot
	err := r.db.WithContext(ctx).Raw(`SELECT s.* FROM user_info_daily_snapshot s
		WHERE s.date = (SELECT MAX(date) FROM user_info_daily_snapshot WHERE site = s.site AND date < ?)`, before).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]UserInfoDailySnapshot, len(rows))
	for _, row := range rows {
		out[row.Site] = row
	}
	return out, nil
}

// PruneSnapshots 删除日期早于 before 的快照，返回删除的行数。
func (r *DBUserInfoRepo) PruneSnapshots(ctx context.Context, before string) (int64, error) {
	res := r.db.WithContext(ctx).Where("date < ?", before).Delete(&UserInfoDailySnapshot{})
	return res.RowsAffected, res.Error
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
