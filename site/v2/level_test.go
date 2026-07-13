package v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanLevelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Power User", "poweruser"},
		{"with underscore", "Power_User", "poweruser"},
		{"multiple spaces", "Power  User", "poweruser"},
		{"mixed case", "POWER USER", "poweruser"},
		{"already clean", "poweruser", "poweruser"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanLevelName(tt.input)
			if result != tt.expected {
				t.Errorf("cleanLevelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGuessGroupType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LevelGroupType
	}{
		{"regular user", "Power User", LevelGroupUser},
		{"vip", "VIP", LevelGroupVIP},
		{"vip chinese", "贵宾", LevelGroupVIP},
		{"honor", "Honor Member", LevelGroupVIP},
		{"admin", "Administrator", LevelGroupManager},
		{"moderator", "Moderator", LevelGroupManager},
		{"uploader", "Uploader", LevelGroupManager},
		{"retiree", "Retiree", LevelGroupManager},
		{"staff", "Staff", LevelGroupManager},
		{"helper", "Helper", LevelGroupManager},
		{"empty", "", LevelGroupUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guessGroupType(tt.input)
			if result != tt.expected {
				t.Errorf("guessGroupType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchLevelName(t *testing.T) {
	tests := []struct {
		name      string
		userLevel string
		reqName   string
		nameAka   []string
		expected  bool
	}{
		{"exact match", "poweruser", "Power User", nil, true},
		{"partial match", "power", "Power User", nil, true},
		{"aka match", "elite", "精英", []string{"Elite"}, true},
		{"no match", "admin", "Power User", nil, false},
		{"case insensitive", "POWERUSER", "power user", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchLevelName(tt.userLevel, tt.reqName, tt.nameAka)
			if result != tt.expected {
				t.Errorf("matchLevelName(%q, %q, %v) = %v, want %v",
					tt.userLevel, tt.reqName, tt.nameAka, result, tt.expected)
			}
		})
	}
}

func TestParseSizeStringToBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"bytes", "100", 100},
		{"kilobytes", "1KB", 1024},
		{"megabytes", "1MB", 1024 * 1024},
		{"gigabytes", "1GB", 1024 * 1024 * 1024},
		{"terabytes", "1TB", 1024 * 1024 * 1024 * 1024},
		{"with decimal", "1.5GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"with space", "100 MB", 100 * 1024 * 1024},
		{"lowercase", "500mb", 500 * 1024 * 1024},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSizeStringToBytes(tt.input)
			if result != tt.expected {
				t.Errorf("parseSizeStringToBytes(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseISODuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"5 weeks", "P5W", 5 * 7 * 24 * time.Hour},
		{"10 weeks", "P10W", 10 * 7 * 24 * time.Hour},
		{"1 year", "P1Y", 365 * 24 * time.Hour},
		{"1 month", "P1M", 30 * 24 * time.Hour},
		{"7 days", "P7D", 7 * 24 * time.Hour},
		{"combined", "P1Y2M3W4D", (365 + 60 + 21 + 4) * 24 * time.Hour},
		{"lowercase", "p5w", 5 * 7 * 24 * time.Hour},
		{"invalid", "invalid", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseISODuration(tt.input)
			if result != tt.expected {
				t.Errorf("parseISODuration(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCheckInterval(t *testing.T) {
	now := time.Now()
	oneWeekAgo := now.Add(-7 * 24 * time.Hour).Unix()
	tenWeeksAgo := now.Add(-10 * 7 * 24 * time.Hour).Unix()

	tests := []struct {
		name     string
		joinTime int64
		interval string
		expected bool
	}{
		{"zero join time", 0, "P5W", true},
		{"empty interval", oneWeekAgo, "", true},
		{"met requirement", tenWeeksAgo, "P5W", true},
		{"not met requirement", oneWeekAgo, "P5W", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkInterval(tt.joinTime, tt.interval)
			if result != tt.expected {
				t.Errorf("checkInterval(%d, %q) = %v, want %v",
					tt.joinTime, tt.interval, result, tt.expected)
			}
		})
	}
}

func TestIsAlternativeMet(t *testing.T) {
	tests := []struct {
		name     string
		info     *UserInfo
		alt      AlternativeRequirement
		expected bool
	}{
		{
			name: "all requirements met",
			info: &UserInfo{
				SeedingBonus: 200000,
				Uploads:      10,
				Bonus:        50000,
			},
			alt: AlternativeRequirement{
				SeedingBonus: 100000,
				Uploads:      5,
			},
			expected: true,
		},
		{
			name: "seedingBonus not met",
			info: &UserInfo{
				SeedingBonus: 50000,
				Uploads:      10,
			},
			alt: AlternativeRequirement{
				SeedingBonus: 100000,
				Uploads:      5,
			},
			expected: false,
		},
		{
			name: "uploads not met",
			info: &UserInfo{
				SeedingBonus: 200000,
				Uploads:      2,
			},
			alt: AlternativeRequirement{
				SeedingBonus: 100000,
				Uploads:      5,
			},
			expected: false,
		},
		{
			name: "bonus requirement",
			info: &UserInfo{
				Bonus: 100000,
			},
			alt: AlternativeRequirement{
				Bonus: 50000,
			},
			expected: true,
		},
		{
			name: "downloaded requirement",
			info: &UserInfo{
				Downloaded: 500 * 1024 * 1024 * 1024, // 500GB
			},
			alt: AlternativeRequirement{
				Downloaded: "200GB",
			},
			expected: true,
		},
		{
			name: "ratio requirement",
			info: &UserInfo{
				Ratio: 3.0,
			},
			alt: AlternativeRequirement{
				Ratio: 2.0,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAlternativeMet(tt.info, tt.alt)
			if result != tt.expected {
				t.Errorf("isAlternativeMet() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsSiteRequirementMet(t *testing.T) {
	tenWeeksAgo := time.Now().Add(-10 * 7 * 24 * time.Hour).Unix()

	tests := []struct {
		name     string
		info     *UserInfo
		req      SiteLevelRequirement
		expected bool
	}{
		{
			name: "all requirements met",
			info: &UserInfo{
				JoinDate:   tenWeeksAgo,
				Downloaded: 300 * 1024 * 1024 * 1024, // 300GB
				Uploaded:   500 * 1024 * 1024 * 1024, // 500GB
				Ratio:      3.0,
				Bonus:      100000,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Uploaded:   "400GB",
				Ratio:      2.0,
				Bonus:      50000,
			},
			expected: true,
		},
		{
			name: "interval not met",
			info: &UserInfo{
				JoinDate:   time.Now().Add(-2 * 7 * 24 * time.Hour).Unix(),
				Downloaded: 300 * 1024 * 1024 * 1024,
				Ratio:      3.0,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Ratio:      2.0,
			},
			expected: false,
		},
		{
			name: "downloaded not met",
			info: &UserInfo{
				JoinDate:   tenWeeksAgo,
				Downloaded: 100 * 1024 * 1024 * 1024, // 100GB
				Ratio:      3.0,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Ratio:      2.0,
			},
			expected: false,
		},
		{
			name: "ratio not met",
			info: &UserInfo{
				JoinDate:   tenWeeksAgo,
				Downloaded: 300 * 1024 * 1024 * 1024,
				Ratio:      1.5,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Ratio:      2.0,
			},
			expected: false,
		},
		{
			name: "alternative met",
			info: &UserInfo{
				JoinDate:     tenWeeksAgo,
				Downloaded:   300 * 1024 * 1024 * 1024,
				Ratio:        3.0,
				SeedingBonus: 200000,
				Uploads:      10,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Ratio:      2.0,
				Alternative: []AlternativeRequirement{
					{SeedingBonus: 100000, Uploads: 5},
					{SeedingBonus: 150000},
				},
			},
			expected: true,
		},
		{
			name: "alternative not met",
			info: &UserInfo{
				JoinDate:     tenWeeksAgo,
				Downloaded:   300 * 1024 * 1024 * 1024,
				Ratio:        3.0,
				SeedingBonus: 50000,
				Uploads:      2,
			},
			req: SiteLevelRequirement{
				Interval:   "P5W",
				Downloaded: "200GB",
				Ratio:      2.0,
				Alternative: []AlternativeRequirement{
					{SeedingBonus: 100000, Uploads: 5},
					{SeedingBonus: 150000},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSiteRequirementMet(tt.info, tt.req)
			if result != tt.expected {
				t.Errorf("isSiteRequirementMet() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetMaxUserLevelID(t *testing.T) {
	requirements := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User"},
		{ID: 3, Name: "Elite User"},
		{ID: 100, Name: "VIP", GroupType: LevelGroupVIP},
		{ID: 200, Name: "Admin", GroupType: LevelGroupManager},
	}

	result := getMaxUserLevelID(requirements)
	if result != 3 {
		t.Errorf("getMaxUserLevelID() = %d, want 3", result)
	}
}

func TestGuessUserLevelID(t *testing.T) {
	requirements := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "200GB", Ratio: 2.0},
		{ID: 3, Name: "Elite User", Downloaded: "500GB", Ratio: 2.5},
		{ID: 100, Name: "VIP", GroupType: LevelGroupVIP},
	}

	tests := []struct {
		name     string
		info     *UserInfo
		expected int
	}{
		{
			name: "match by name",
			info: &UserInfo{
				LevelName: "Power User",
			},
			expected: 2,
		},
		{
			name: "match VIP by name",
			info: &UserInfo{
				LevelName: "VIP",
			},
			expected: 100,
		},
		{
			name: "match by requirements - level 1",
			info: &UserInfo{
				Downloaded: 100 * 1024 * 1024 * 1024, // 100GB
				Ratio:      1.5,
			},
			expected: 1, // First level (User) has no requirements, so user is at level 1
		},
		{
			name: "match by requirements - level 2",
			info: &UserInfo{
				Downloaded: 300 * 1024 * 1024 * 1024, // 300GB
				Ratio:      2.5,
			},
			expected: 2,
		},
		{
			name: "match by requirements - level 3",
			info: &UserInfo{
				Downloaded: 600 * 1024 * 1024 * 1024, // 600GB
				Ratio:      3.0,
			},
			expected: 3,
		},
		{
			name:     "empty info",
			info:     &UserInfo{},
			expected: 1, // Default to first level when no info
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GuessUserLevelID(tt.info, requirements)
			if result != tt.expected {
				t.Errorf("GuessUserLevelID() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCalculateSiteLevelProgress(t *testing.T) {
	requirements := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "200GB", Ratio: 2.0, Bonus: 50000},
		{ID: 3, Name: "Elite User", Downloaded: "500GB", Ratio: 2.5, Bonus: 100000},
	}

	tests := []struct {
		name           string
		info           *UserInfo
		expectCurrent  string
		expectNext     string
		expectHasUnmet bool
	}{
		{
			name: "at level 1, progress to level 2",
			info: &UserInfo{
				LevelID:    1,
				Downloaded: 100 * 1024 * 1024 * 1024, // 100GB
				Ratio:      1.5,
				Bonus:      25000,
			},
			expectCurrent:  "User",
			expectNext:     "Power User",
			expectHasUnmet: true,
		},
		{
			name: "at level 2, progress to level 3",
			info: &UserInfo{
				LevelID:    2,
				Downloaded: 300 * 1024 * 1024 * 1024, // 300GB
				Ratio:      2.2,
				Bonus:      60000,
			},
			expectCurrent:  "Power User",
			expectNext:     "Elite User",
			expectHasUnmet: true,
		},
		{
			name: "at max level",
			info: &UserInfo{
				LevelID:    3,
				Downloaded: 600 * 1024 * 1024 * 1024,
				Ratio:      3.0,
				Bonus:      150000,
			},
			expectCurrent:  "Elite User",
			expectNext:     "",
			expectHasUnmet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateSiteLevelProgress(tt.info, requirements)
			if result == nil {
				t.Fatal("CalculateSiteLevelProgress() returned nil")
				return
			}

			if result.CurrentLevel != nil && result.CurrentLevel.Name != tt.expectCurrent {
				t.Errorf("CurrentLevel.Name = %q, want %q", result.CurrentLevel.Name, tt.expectCurrent)
			}

			if tt.expectNext == "" {
				if result.NextLevel != nil {
					t.Errorf("NextLevel should be nil, got %v", result.NextLevel)
				}
			} else {
				if result.NextLevel == nil {
					t.Errorf("NextLevel should not be nil")
				} else if result.NextLevel.Name != tt.expectNext {
					t.Errorf("NextLevel.Name = %q, want %q", result.NextLevel.Name, tt.expectNext)
				}
			}

			hasUnmet := len(result.UnmetRequirements) > 0
			if hasUnmet != tt.expectHasUnmet {
				t.Errorf("hasUnmet = %v, want %v", hasUnmet, tt.expectHasUnmet)
			}
		})
	}
}

func TestGetSiteNextLevelUnmet(t *testing.T) {
	requirements := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "200GB", Ratio: 2.0, Bonus: 50000},
	}

	info := &UserInfo{
		LevelID:      1,
		Downloaded:   100 * 1024 * 1024 * 1024, // 100GB
		Ratio:        1.5,
		Bonus:        25000,
		BonusPerHour: 100,
	}

	result := GetSiteNextLevelUnmet(info, requirements)

	// Check downloaded unmet
	if _, ok := result["downloaded"]; !ok {
		t.Error("expected 'downloaded' in unmet requirements")
	}

	// Check ratio unmet
	if _, ok := result["ratio"]; !ok {
		t.Error("expected 'ratio' in unmet requirements")
	}

	// Check bonus unmet
	if _, ok := result["bonus"]; !ok {
		t.Error("expected 'bonus' in unmet requirements")
	}

	// Check bonusNeededHours is calculated
	if _, ok := result["bonusNeededHours"]; !ok {
		t.Error("expected 'bonusNeededHours' in unmet requirements")
	}
}

func levelReqs() []SiteLevelRequirement {
	return []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", NameAka: []string{"高级用户"}, Downloaded: "100GB", Ratio: 2.0, Bonus: 1000, Interval: "P5W", SeedingBonus: 500},
		{ID: 3, Name: "Elite User", Downloaded: "500GB", Ratio: 3.0},
		{ID: 10, Name: "VIP", GroupType: LevelGroupVIP},
	}
}

func TestGuessUserLevelID_NameMatch(t *testing.T) {
	info := &UserInfo{LevelName: "Power User"}
	assert.Equal(t, 2, GuessUserLevelID(info, levelReqs()))
}

func TestGuessUserLevelID_AkaMatch(t *testing.T) {
	info := &UserInfo{LevelName: "高级用户"}
	assert.Equal(t, 2, GuessUserLevelID(info, levelReqs()))
}

func TestGuessUserLevelID_EmptyNoReqs(t *testing.T) {
	assert.Equal(t, -1, GuessUserLevelID(&UserInfo{}, nil))
}

func TestGuessUserLevelID_ByRequirements(t *testing.T) {
	info := &UserInfo{
		Downloaded:   150 * 1024 * 1024 * 1024,
		Ratio:        2.5,
		Bonus:        2000,
		SeedingBonus: 600,
		JoinDate:     time.Now().Add(-100 * 24 * time.Hour).Unix(),
	}
	id := GuessUserLevelID(info, levelReqs())
	assert.GreaterOrEqual(t, id, 1)
}

func TestGetSiteNextLevelUnmet_Cov3(t *testing.T) {
	info := &UserInfo{
		LevelID:      2,
		Downloaded:   10 * 1024 * 1024 * 1024,
		Ratio:        1.0,
		Bonus:        100,
		BonusPerHour: 10,
	}
	unmet := GetSiteNextLevelUnmet(info, levelReqs())
	require.NotNil(t, unmet)
	assert.Contains(t, unmet, "downloaded")
	assert.Contains(t, unmet, "ratio")
}

func TestGetSiteNextLevelUnmet_NoNext(t *testing.T) {
	info := &UserInfo{LevelID: 3}
	unmet := GetSiteNextLevelUnmet(info, levelReqs())
	assert.Empty(t, unmet)
}

func TestCalculateSiteLevelProgress_Cov3(t *testing.T) {
	info := &UserInfo{
		LevelID:    1,
		Downloaded: 50 * 1024 * 1024 * 1024,
		Ratio:      1.0,
		Bonus:      100,
	}
	progress := CalculateSiteLevelProgress(info, levelReqs())
	require.NotNil(t, progress)
	assert.NotNil(t, progress.CurrentLevel)
	assert.LessOrEqual(t, progress.ProgressPercent, float64(100))
}

func TestCalculateSiteLevelProgress_NoReqs(t *testing.T) {
	assert.Nil(t, CalculateSiteLevelProgress(&UserInfo{}, nil))
}

func TestCalculateSiteLevelProgress_MaxLevel(t *testing.T) {
	info := &UserInfo{LevelID: 3}
	progress := CalculateSiteLevelProgress(info, levelReqs())
	require.NotNil(t, progress)
	assert.Equal(t, float64(100), progress.ProgressPercent)
}

func TestIsSiteRequirementMet_ExtendedFields(t *testing.T) {
	req := SiteLevelRequirement{
		Uploads:     5,
		Seeding:     10,
		SeedingSize: "1TB",
	}
	// not met — nothing set
	assert.False(t, isSiteRequirementMet(&UserInfo{}, req))
	// met
	info := &UserInfo{
		Uploads:    6,
		Seeding:    11,
		SeederSize: 2 * 1024 * 1024 * 1024 * 1024,
	}
	assert.True(t, isSiteRequirementMet(info, req))
}

// ---------------------------------------------------------------------------
// nexusphp_driver.go — getUserInfoWithDefinition full pipeline
// ---------------------------------------------------------------------------

func TestGuessUserLevelID_VIPGroup(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 100, Name: "VIP", GroupType: LevelGroupVIP},
	}
	info := &UserInfo{LevelName: "VIP"}
	assert.Equal(t, 100, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_VIPGroup_NoMatchingReq(t *testing.T) {
	reqs := []SiteLevelRequirement{{ID: 1, Name: "User"}}
	info := &UserInfo{LevelName: "贵宾"}
	assert.Equal(t, MinVipLevelID, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_ManagerGroup(t *testing.T) {
	reqs := []SiteLevelRequirement{{ID: 1, Name: "User"}}
	info := &UserInfo{LevelName: "Administrator"}
	assert.Equal(t, MinManagerLevelID, GuessUserLevelID(info, reqs))
}

func TestGuessUserLevelID_PreviousLevelOnUnmet(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{ID: 2, Name: "Power User", Downloaded: "1PB"},
	}
	info := &UserInfo{Downloaded: 1024}
	got := GuessUserLevelID(info, reqs)
	assert.Equal(t, 1, got)
}

// ---------------------------------------------------------------------------
// executeProcess — critical error propagation via full pipeline
// ---------------------------------------------------------------------------

func TestGetSiteNextLevelUnmet_ExtendedBranches(t *testing.T) {
	reqs := []SiteLevelRequirement{
		{ID: 1, Name: "User"},
		{
			ID:           2,
			Name:         "Power User",
			Downloaded:   "100GB",
			Uploaded:     "500GB",
			Ratio:        3.0,
			Bonus:        10000,
			SeedingBonus: 5000,
			Interval:     "P52W",
		},
	}
	info := &UserInfo{
		LevelID:             1,
		Downloaded:          10 * 1024 * 1024 * 1024,
		Uploaded:            1024,
		Ratio:               1.0,
		Bonus:               100,
		BonusPerHour:        10,
		SeedingBonus:        100,
		SeedingBonusPerHour: 5,
		JoinDate:            time.Now().Add(-24 * time.Hour).Unix(),
	}
	unmet := GetSiteNextLevelUnmet(info, reqs)
	assert.Contains(t, unmet, "downloaded")
	assert.Contains(t, unmet, "uploaded")
	assert.Contains(t, unmet, "ratio")
	assert.Contains(t, unmet, "bonus")
	assert.Contains(t, unmet, "bonusNeededHours")
	assert.Contains(t, unmet, "seedingBonus")
	assert.Contains(t, unmet, "seedingBonusNeededHours")
	assert.Contains(t, unmet, "interval")
}

// ---------------------------------------------------------------------------
// isAlternativeMet — each field branch
// ---------------------------------------------------------------------------

func TestIsAlternativeMet_Branches(t *testing.T) {
	alt := AlternativeRequirement{SeedingBonus: 100, Uploads: 5, Bonus: 1000, Downloaded: "10GB", Ratio: 2.0}
	// all met
	ok := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 20 * 1024 * 1024 * 1024, Ratio: 3.0}
	assert.True(t, isAlternativeMet(ok, alt))
	// downloaded not met
	bad := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 1, Ratio: 3.0}
	assert.False(t, isAlternativeMet(bad, alt))
	// ratio not met
	bad2 := &UserInfo{SeedingBonus: 200, Uploads: 10, Bonus: 2000, Downloaded: 20 * 1024 * 1024 * 1024, Ratio: 1.0}
	assert.False(t, isAlternativeMet(bad2, alt))
}

// ---------------------------------------------------------------------------
// getUserInfoLegacy — full 2-step legacy pipeline
// ---------------------------------------------------------------------------
