package models

import "time"

// MediaSetting 是媒体识别的设置（只有一行，ID 为 1）。TMDB 的 API Key 加密保存、只写不读；
// 代理地址可能带用户名密码，也加密保存，为空时用环境变量里的代理（与站点请求相同）。
type MediaSetting struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TMDBKeyEncrypted string    `gorm:"type:text;default:''" json:"-"`
	Language         string    `gorm:"size:16;default:''" json:"language"`
	ProxyEncrypted   string    `gorm:"type:text;default:''" json:"-"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (MediaSetting) TableName() string { return "media_settings" }

// 识别词的种类。
const (
	MediaWordBlock   = "block"   // 屏蔽：从标题与副标题里去掉
	MediaWordReplace = "replace" // 替换：换成另一段文字
	MediaWordOffset  = "offset"  // 集数偏移：标题或副标题匹配时，集数加上偏移量（可为负）
)

// MediaWordRule 是识别词：解析标题之前按顺序套用（集数偏移在解析之后加上）。列的默认值一律是零值。
type MediaWordRule struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Kind        string    `gorm:"size:16;not null;default:''" json:"kind"`
	Pattern     string    `gorm:"size:256;not null;default:''" json:"pattern"`
	Replacement string    `gorm:"size:256;not null;default:''" json:"replacement"`
	Offset      int       `gorm:"not null;default:0" json:"offset"`
	IsRegex     bool      `gorm:"not null;default:false" json:"is_regex"`
	Enabled     bool      `gorm:"not null;default:false" json:"enabled"`
	Note        string    `gorm:"size:256;not null;default:''" json:"note"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MediaWordRule) TableName() string { return "media_word_rules" }

// MediaOverride 是手动纠正：解析结果相同（类型、名字，电影另加年份）的标题固定识别成这个 TMDB 条目。
type MediaOverride struct {
	ID  uint   `gorm:"primaryKey" json:"id"`
	Key string `gorm:"size:255;not null;uniqueIndex" json:"key"`
	// AltKey 是用中文名算出的别名（有英文名时 Key 用英文名）：副标题里有没有中文名都能命中
	AltKey    string    `gorm:"size:255;not null;default:'';index" json:"alt_key"`
	Label     string    `gorm:"size:255;not null;default:''" json:"label"`
	TMDBID    int       `gorm:"column:tmdb_id;not null;default:0" json:"tmdb_id"`
	MediaType string    `gorm:"size:8;not null;default:''" json:"media_type"`
	Title     string    `gorm:"size:255;not null;default:''" json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MediaOverride) TableName() string { return "media_overrides" }

// MediaCache 是 TMDB 响应的缓存，到期后不再使用。
type MediaCache struct {
	Key       string    `gorm:"primaryKey;size:255" json:"key"`
	Value     string    `gorm:"type:text;not null;default:''" json:"value"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
}

func (MediaCache) TableName() string { return "media_cache" }
