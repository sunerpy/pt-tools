package v2

import (
	"regexp"
	"strings"
	"time"
)

// cleanLevelName normalizes a level name for comparison
func cleanLevelName(levelName string) string {
	// Remove spaces, underscores, and convert to lowercase
	re := regexp.MustCompile(`[\s_]+`)
	return strings.ToLower(re.ReplaceAllString(levelName, ""))
}

// guessGroupType determines the user group type from level name
func guessGroupType(levelName string) LevelGroupType {
	userLevel := strings.ToLower(levelName)

	// Manager/staff keywords
	managerKeywords := []string{
		"retiree", "养老", "退休",
		"uploader", "发布", "发种", "上传", "种子",
		"helper", "assistant", "助手", "助理",
		"seeder", "保种",
		"transferrer", "转载",
		"forum", "版主",
		"moderator", "admin", "管理",
		"sys", "coder", "开发",
		"staff", "主管",
	}

	// VIP keywords
	vipKeywords := []string{
		"vip", "贵宾", "honor", "荣誉",
	}

	for _, keyword := range managerKeywords {
		if strings.Contains(userLevel, keyword) {
			return LevelGroupManager
		}
	}

	for _, keyword := range vipKeywords {
		if strings.Contains(userLevel, keyword) {
			return LevelGroupVIP
		}
	}

	return LevelGroupUser
}

// matchLevelName checks if user level matches requirement name
func matchLevelName(userLevel, reqName string, nameAka []string) bool {
	cleanedUserLevel := cleanLevelName(userLevel)

	// Check main name
	if strings.Contains(cleanLevelName(reqName), cleanedUserLevel) {
		return true
	}

	// Check alternative names
	for _, aka := range nameAka {
		if strings.Contains(cleanLevelName(aka), cleanedUserLevel) {
			return true
		}
	}

	return false
}

// parseSizeStringToBytes parses a size string like "200GB" to bytes
func parseSizeStringToBytes(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	sizeStr = strings.ReplaceAll(sizeStr, ",", "")
	sizeStr = strings.ReplaceAll(sizeStr, " ", "")

	re := regexp.MustCompile(`([\d.]+)\s*([KMGTP]?i?B?)`)
	matches := re.FindStringSubmatch(strings.ToUpper(sizeStr))
	if len(matches) < 2 {
		return 0
	}

	value := parseFloat(matches[1])
	unit := ""
	if len(matches) > 2 {
		unit = matches[2]
	}

	multiplier := float64(1)
	switch {
	case strings.HasPrefix(unit, "K"):
		multiplier = 1024
	case strings.HasPrefix(unit, "M"):
		multiplier = 1024 * 1024
	case strings.HasPrefix(unit, "G"):
		multiplier = 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "T"):
		multiplier = 1024 * 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "P"):
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	}

	return int64(value * multiplier)
}

// parseISODuration parses ISO 8601 duration to time.Duration
// Supports: P[n]Y[n]M[n]W[n]D (years, months, weeks, days)
func parseISODuration(duration string) time.Duration {
	duration = strings.ToUpper(strings.TrimSpace(duration))
	if !strings.HasPrefix(duration, "P") {
		return 0
	}
	duration = duration[1:] // Remove 'P'

	var totalDays int

	re := regexp.MustCompile(`(\d+)([YMWD])`)
	matches := re.FindAllStringSubmatch(duration, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		value := toInt(match[1])
		unit := match[2]

		switch unit {
		case "Y":
			totalDays += value * 365
		case "M":
			totalDays += value * 30
		case "W":
			totalDays += value * 7
		case "D":
			totalDays += value
		}
	}

	return time.Duration(totalDays) * 24 * time.Hour
}

// checkInterval checks if user has been registered long enough
func checkInterval(joinTime int64, interval string) bool {
	if joinTime == 0 {
		return true // Can't verify, assume met
	}

	requiredDuration := parseISODuration(interval)
	if requiredDuration == 0 {
		return true
	}

	joinDate := time.Unix(joinTime, 0)
	requiredDate := joinDate.Add(requiredDuration)

	return time.Now().After(requiredDate)
}

// isAlternativeMet checks if any alternative requirement is met
func isAlternativeMet(info *UserInfo, alt AlternativeRequirement) bool {
	// Check seedingBonus
	if alt.SeedingBonus > 0 && info.SeedingBonus < alt.SeedingBonus {
		return false
	}

	// Check uploads
	if alt.Uploads > 0 && info.Uploads < alt.Uploads {
		return false
	}

	// Check bonus
	if alt.Bonus > 0 && info.Bonus < alt.Bonus {
		return false
	}

	// Check downloaded
	if alt.Downloaded != "" {
		required := parseSizeStringToBytes(alt.Downloaded)
		if info.Downloaded < required {
			return false
		}
	}

	// Check ratio
	if alt.Ratio > 0 && info.Ratio < alt.Ratio {
		return false
	}

	return true
}

// isSiteRequirementMet checks if user meets level requirements
func isSiteRequirementMet(info *UserInfo, req SiteLevelRequirement) bool {
	// Check interval (join time)
	if req.Interval != "" && !checkInterval(info.JoinDate, req.Interval) {
		return false
	}

	// Check downloaded
	if req.Downloaded != "" {
		required := parseSizeStringToBytes(req.Downloaded)
		if info.Downloaded < required {
			return false
		}
	}

	// Check uploaded
	if req.Uploaded != "" {
		required := parseSizeStringToBytes(req.Uploaded)
		if info.Uploaded < required {
			return false
		}
	}

	// Check ratio
	if req.Ratio > 0 && info.Ratio < req.Ratio {
		return false
	}

	// Check bonus
	if req.Bonus > 0 && info.Bonus < req.Bonus {
		return false
	}

	// Check seedingBonus
	if req.SeedingBonus > 0 && info.SeedingBonus < req.SeedingBonus {
		return false
	}

	// Check uploads
	if req.Uploads > 0 && info.Uploads < req.Uploads {
		return false
	}

	// Check seeding count
	if req.Seeding > 0 && info.Seeding < req.Seeding {
		return false
	}

	// Check seeding size
	if req.SeedingSize != "" {
		required := parseSizeStringToBytes(req.SeedingSize)
		if info.SeederSize < required {
			return false
		}
	}

	// Check alternative requirements (OR logic)
	if len(req.Alternative) > 0 {
		anyMet := false
		for _, alt := range req.Alternative {
			if isAlternativeMet(info, alt) {
				anyMet = true
				break
			}
		}
		if !anyMet {
			return false
		}
	}

	return true
}

// getMaxUserLevelID returns the maximum level ID for regular users
func getMaxUserLevelID(requirements []SiteLevelRequirement) int {
	maxID := 0
	for _, req := range requirements {
		groupType := req.GroupType
		if groupType == "" {
			groupType = LevelGroupUser
		}
		if groupType == LevelGroupUser && req.ID > maxID {
			maxID = req.ID
		}
	}
	return maxID
}

// GuessUserLevelID determines user level from info and requirements
func GuessUserLevelID(info *UserInfo, requirements []SiteLevelRequirement) int {
	if info.LevelName == "" && len(requirements) == 0 {
		return -1
	}

	// 1. Try exact name match
	if info.LevelName != "" {
		for _, req := range requirements {
			if matchLevelName(info.LevelName, req.Name, req.NameAka) {
				return req.ID
			}
		}
	}

	// 2. Try group type match (vip, manager)
	if info.LevelName != "" {
		groupType := guessGroupType(info.LevelName)
		if groupType != LevelGroupUser {
			for _, req := range requirements {
				if req.GroupType == groupType {
					return req.ID
				}
			}
			// Return default for special groups
			if groupType == LevelGroupVIP {
				return MinVipLevelID
			}
			return MinManagerLevelID
		}
	}

	// 3. Match by requirements
	maxUserLevel := getMaxUserLevelID(requirements)
	for i, req := range requirements {
		// Skip non-user levels
		groupType := req.GroupType
		if groupType == "" {
			groupType = LevelGroupUser
		}
		if groupType != LevelGroupUser {
			continue
		}

		if !isSiteRequirementMet(info, req) {
			if i > 0 {
				// Return previous level
				for j := i - 1; j >= 0; j-- {
					prevGroupType := requirements[j].GroupType
					if prevGroupType == "" {
						prevGroupType = LevelGroupUser
					}
					if prevGroupType == LevelGroupUser {
						return requirements[j].ID
					}
				}
			}
			return -1
		}
	}

	return maxUserLevel
}

// CalculateSiteLevelProgress calculates progress to next level
func CalculateSiteLevelProgress(info *UserInfo, requirements []SiteLevelRequirement) *SiteLevelProgressInfo {
	if len(requirements) == 0 {
		return nil
	}

	currentLevelID := info.LevelID
	if currentLevelID == 0 {
		currentLevelID = GuessUserLevelID(info, requirements)
	}

	var currentLevel, nextLevel *SiteLevelRequirement
	for i, req := range requirements {
		if req.ID == currentLevelID {
			currentLevel = &requirements[i]
			// Find next user level
			for j := i + 1; j < len(requirements); j++ {
				groupType := requirements[j].GroupType
				if groupType == "" {
					groupType = LevelGroupUser
				}
				if groupType == LevelGroupUser {
					nextLevel = &requirements[j]
					break
				}
			}
			break
		}
	}

	progress := &SiteLevelProgressInfo{
		CurrentLevel:      currentLevel,
		NextLevel:         nextLevel,
		UnmetRequirements: make(map[string]interface{}),
		ProgressPercent:   100,
	}

	if nextLevel == nil {
		return progress
	}

	// Calculate unmet requirements
	unmet := GetSiteNextLevelUnmet(info, requirements)
	progress.UnmetRequirements = unmet

	// Calculate progress percentage (simplified)
	totalReqs := 0
	metReqs := 0

	if nextLevel.Downloaded != "" {
		totalReqs++
		required := parseSizeStringToBytes(nextLevel.Downloaded)
		if info.Downloaded >= required {
			metReqs++
		}
	}
	if nextLevel.Ratio > 0 {
		totalReqs++
		if info.Ratio >= nextLevel.Ratio {
			metReqs++
		}
	}
	if nextLevel.Bonus > 0 {
		totalReqs++
		if info.Bonus >= nextLevel.Bonus {
			metReqs++
		}
	}
	if nextLevel.SeedingBonus > 0 {
		totalReqs++
		if info.SeedingBonus >= nextLevel.SeedingBonus {
			metReqs++
		}
	}
	if nextLevel.Interval != "" {
		totalReqs++
		if checkInterval(info.JoinDate, nextLevel.Interval) {
			metReqs++
		}
	}

	if totalReqs > 0 {
		progress.ProgressPercent = float64(metReqs) / float64(totalReqs) * 100
	}

	return progress
}

// GetSiteNextLevelUnmet returns unmet requirements for next level
func GetSiteNextLevelUnmet(info *UserInfo, requirements []SiteLevelRequirement) map[string]interface{} {
	unmet := make(map[string]interface{})

	currentLevelID := info.LevelID
	if currentLevelID == 0 {
		currentLevelID = GuessUserLevelID(info, requirements)
	}

	// Find next level
	var nextLevel *SiteLevelRequirement
	foundCurrent := false
	for i, req := range requirements {
		if req.ID == currentLevelID {
			foundCurrent = true
			continue
		}
		if foundCurrent {
			groupType := req.GroupType
			if groupType == "" {
				groupType = LevelGroupUser
			}
			if groupType == LevelGroupUser {
				nextLevel = &requirements[i]
				break
			}
		}
	}

	if nextLevel == nil {
		return unmet
	}

	// Check each requirement
	if nextLevel.Downloaded != "" {
		required := parseSizeStringToBytes(nextLevel.Downloaded)
		if info.Downloaded < required {
			unmet["downloaded"] = required - info.Downloaded
		}
	}

	if nextLevel.Uploaded != "" {
		required := parseSizeStringToBytes(nextLevel.Uploaded)
		if info.Uploaded < required {
			unmet["uploaded"] = required - info.Uploaded
		}
	}

	if nextLevel.Ratio > 0 && info.Ratio < nextLevel.Ratio {
		unmet["ratio"] = nextLevel.Ratio - info.Ratio
	}

	if nextLevel.Bonus > 0 && info.Bonus < nextLevel.Bonus {
		unmet["bonus"] = nextLevel.Bonus - info.Bonus
		// Calculate time needed if bonusPerHour is available
		if info.BonusPerHour > 0 {
			hoursNeeded := (nextLevel.Bonus - info.Bonus) / info.BonusPerHour
			unmet["bonusNeededHours"] = hoursNeeded
		}
	}

	if nextLevel.SeedingBonus > 0 && info.SeedingBonus < nextLevel.SeedingBonus {
		unmet["seedingBonus"] = nextLevel.SeedingBonus - info.SeedingBonus
		if info.SeedingBonusPerHour > 0 {
			hoursNeeded := (nextLevel.SeedingBonus - info.SeedingBonus) / info.SeedingBonusPerHour
			unmet["seedingBonusNeededHours"] = hoursNeeded
		}
	}

	if nextLevel.Interval != "" && !checkInterval(info.JoinDate, nextLevel.Interval) {
		requiredDuration := parseISODuration(nextLevel.Interval)
		joinDate := time.Unix(info.JoinDate, 0)
		requiredDate := joinDate.Add(requiredDuration)
		remaining := time.Until(requiredDate)
		if remaining > 0 {
			unmet["interval"] = remaining.String()
		}
	}

	return unmet
}
