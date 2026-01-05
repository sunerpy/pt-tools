package v2

// Level ID constants for special user groups
const (
	// MinVipLevelID is the minimum level ID for VIP users
	MinVipLevelID = 100
	// MinManagerLevelID is the minimum level ID for manager/staff users
	MinManagerLevelID = 200
)

// LevelGroupType represents the type of user level group
type LevelGroupType string

const (
	// LevelGroupUser represents regular user levels
	LevelGroupUser LevelGroupType = "user"
	// LevelGroupVIP represents VIP/donor levels
	LevelGroupVIP LevelGroupType = "vip"
	// LevelGroupManager represents staff/manager levels
	LevelGroupManager LevelGroupType = "manager"
)

// SiteLevelRequirement defines requirements for a user level (site-specific)
// This is different from LevelRequirement in level_manager.go which is for generic level management
type SiteLevelRequirement struct {
	// ID is the numeric level identifier
	ID int `json:"id"`
	// Name is the level name (e.g., "Power User")
	Name string `json:"name"`
	// NameAka contains alternative names for matching
	NameAka []string `json:"nameAka,omitempty"`
	// GroupType: "user", "vip", "manager"
	GroupType LevelGroupType `json:"groupType,omitempty"`

	// Requirements (all optional)
	// Interval is ISO 8601 duration (e.g., "P5W" for 5 weeks)
	Interval string `json:"interval,omitempty"`
	// Downloaded is size string (e.g., "200GB")
	Downloaded string `json:"downloaded,omitempty"`
	// Uploaded is size string
	Uploaded string `json:"uploaded,omitempty"`
	// Ratio is minimum ratio
	Ratio float64 `json:"ratio,omitempty"`
	// Bonus is minimum bonus points
	Bonus float64 `json:"bonus,omitempty"`

	// Extended requirements
	// SeedingBonus is seeding bonus points requirement
	SeedingBonus float64 `json:"seedingBonus,omitempty"`
	// Uploads is number of uploads requirement
	Uploads int `json:"uploads,omitempty"`
	// Seeding is number of seeding torrents requirement
	Seeding int `json:"seeding,omitempty"`
	// SeedingSize is total seeding size requirement
	SeedingSize string `json:"seedingSize,omitempty"`

	// Alternative contains OR-based alternative requirements
	Alternative []AlternativeRequirement `json:"alternative,omitempty"`

	// Privilege describes the privileges granted at this level
	Privilege string `json:"privilege,omitempty"`
}

// AlternativeRequirement for OR-based requirements
type AlternativeRequirement struct {
	SeedingBonus float64 `json:"seedingBonus,omitempty"`
	Uploads      int     `json:"uploads,omitempty"`
	Bonus        float64 `json:"bonus,omitempty"`
	Downloaded   string  `json:"downloaded,omitempty"`
	Ratio        float64 `json:"ratio,omitempty"`
}

// SiteLevelProgressInfo represents progress towards the next user level
type SiteLevelProgressInfo struct {
	// CurrentLevel is the current level requirement
	CurrentLevel *SiteLevelRequirement `json:"currentLevel,omitempty"`
	// NextLevel is the next level requirement
	NextLevel *SiteLevelRequirement `json:"nextLevel,omitempty"`
	// UnmetRequirements contains what's still needed
	UnmetRequirements map[string]interface{} `json:"unmetRequirements,omitempty"`
	// ProgressPercent is the overall progress percentage (0-100)
	ProgressPercent float64 `json:"progressPercent"`
}
