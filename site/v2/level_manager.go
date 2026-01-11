// Package v2 provides level management for PT sites
package v2

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// LevelRequirement defines the requirements for a user level
type LevelRequirement struct {
	// Level is the level name (e.g., "Power User", "Elite", "Master")
	Level string
	// MinUpload is the minimum upload amount in bytes
	MinUpload int64
	// MinRatio is the minimum ratio required
	MinRatio float64
	// MinDays is the minimum account age in days
	MinDays int
	// MinSeedingTorrents is the minimum number of seeding torrents
	MinSeedingTorrents int
	// MinSeedingTime is the minimum total seeding time in hours
	MinSeedingTime int
	// Order is the level order (higher = better)
	Order int
}

// LevelManager manages user levels and calculates progress
type LevelManager struct {
	// levels maps site kind to level requirements
	levels map[SiteKind][]LevelRequirement
	mu     sync.RWMutex
}

// NewLevelManager creates a new level manager with default configurations
func NewLevelManager() *LevelManager {
	lm := &LevelManager{
		levels: make(map[SiteKind][]LevelRequirement),
	}
	lm.initDefaultLevels()
	return lm
}

// Size constants
const (
	KB int64 = 1024
	MB int64 = 1024 * KB
	GB int64 = 1024 * MB
	TB int64 = 1024 * GB
)

// initDefaultLevels initializes default level configurations for common site types
func (lm *LevelManager) initDefaultLevels() {
	// NexusPHP default levels (common across many Chinese PT sites)
	lm.levels[SiteNexusPHP] = []LevelRequirement{
		{Level: "User", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
		{Level: "Power User", MinUpload: 50 * GB, MinRatio: 1.05, MinDays: 14, Order: 1},
		{Level: "Elite User", MinUpload: 120 * GB, MinRatio: 1.55, MinDays: 28, Order: 2},
		{Level: "Crazy User", MinUpload: 300 * GB, MinRatio: 2.05, MinDays: 56, Order: 3},
		{Level: "Insane User", MinUpload: 500 * GB, MinRatio: 2.55, MinDays: 84, Order: 4},
		{Level: "Veteran User", MinUpload: 750 * GB, MinRatio: 3.05, MinDays: 112, Order: 5},
		{Level: "Extreme User", MinUpload: 1 * TB, MinRatio: 3.55, MinDays: 140, Order: 6},
		{Level: "Ultimate User", MinUpload: 1536 * GB, MinRatio: 4.05, MinDays: 168, Order: 7},
		{Level: "Nexus Master", MinUpload: 3 * TB, MinRatio: 4.55, MinDays: 365, Order: 8},
	}

	// MTorrent (M-Team) levels
	lm.levels[SiteMTorrent] = []LevelRequirement{
		{Level: "User", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
		{Level: "Power User", MinUpload: 100 * GB, MinRatio: 1.5, MinDays: 14, Order: 1},
		{Level: "Elite User", MinUpload: 500 * GB, MinRatio: 2.0, MinDays: 28, Order: 2},
		{Level: "Crazy User", MinUpload: 1 * TB, MinRatio: 2.5, MinDays: 56, Order: 3},
		{Level: "Insane User", MinUpload: 2 * TB, MinRatio: 3.0, MinDays: 84, Order: 4},
		{Level: "Veteran User", MinUpload: 5 * TB, MinRatio: 3.5, MinDays: 168, Order: 5},
		{Level: "Extreme User", MinUpload: 10 * TB, MinRatio: 4.0, MinDays: 365, Order: 6},
	}

	// Unit3D default levels
	lm.levels[SiteUnit3D] = []LevelRequirement{
		{Level: "User", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
		{Level: "Uploader", MinUpload: 25 * GB, MinRatio: 0.8, MinDays: 7, Order: 1},
		{Level: "Trusted", MinUpload: 100 * GB, MinRatio: 1.0, MinDays: 30, Order: 2},
		{Level: "Moderator", MinUpload: 500 * GB, MinRatio: 1.5, MinDays: 90, Order: 3},
	}

	// Gazelle default levels
	lm.levels[SiteGazelle] = []LevelRequirement{
		{Level: "User", MinUpload: 0, MinRatio: 0, MinDays: 0, Order: 0},
		{Level: "Member", MinUpload: 10 * GB, MinRatio: 0.65, MinDays: 7, Order: 1},
		{Level: "Power User", MinUpload: 25 * GB, MinRatio: 1.0, MinDays: 14, Order: 2},
		{Level: "Elite", MinUpload: 100 * GB, MinRatio: 1.5, MinDays: 28, Order: 3},
		{Level: "Torrent Master", MinUpload: 500 * GB, MinRatio: 2.0, MinDays: 56, Order: 4},
		{Level: "Power TM", MinUpload: 1 * TB, MinRatio: 2.5, MinDays: 112, Order: 5},
		{Level: "Elite TM", MinUpload: 2 * TB, MinRatio: 3.0, MinDays: 224, Order: 6},
	}
}

// SetLevels sets custom level requirements for a site kind
func (lm *LevelManager) SetLevels(kind SiteKind, levels []LevelRequirement) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	// Sort levels by order
	sorted := make([]LevelRequirement, len(levels))
	copy(sorted, levels)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Order < sorted[j].Order
	})

	lm.levels[kind] = sorted
}

// GetLevels returns the level requirements for a site kind
func (lm *LevelManager) GetLevels(kind SiteKind) []LevelRequirement {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	levels, ok := lm.levels[kind]
	if !ok {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]LevelRequirement, len(levels))
	copy(result, levels)
	return result
}

// GetCurrentLevel determines the user's current level based on their stats
func (lm *LevelManager) GetCurrentLevel(kind SiteKind, upload int64, ratio float64, days int) string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	levels, ok := lm.levels[kind]
	if !ok || len(levels) == 0 {
		return "Unknown"
	}

	currentLevel := levels[0].Level
	for _, level := range levels {
		if upload >= level.MinUpload && ratio >= level.MinRatio && days >= level.MinDays {
			currentLevel = level.Level
		} else {
			break
		}
	}

	return currentLevel
}

// CalculateNextLevel calculates progress towards the next level
func (lm *LevelManager) CalculateNextLevel(kind SiteKind, upload int64, ratio float64, days int) *LevelProgress {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	levels, ok := lm.levels[kind]
	if !ok || len(levels) == 0 {
		return nil
	}

	// Find current and next level
	var currentIdx int
	for i, level := range levels {
		if upload >= level.MinUpload && ratio >= level.MinRatio && days >= level.MinDays {
			currentIdx = i
		} else {
			break
		}
	}

	progress := &LevelProgress{
		CurrentLevel: levels[currentIdx].Level,
	}

	// Check if already at max level
	if currentIdx >= len(levels)-1 {
		progress.NextLevel = ""
		progress.ProgressPercent = 100
		return progress
	}

	nextLevel := levels[currentIdx+1]
	progress.NextLevel = nextLevel.Level

	// Calculate what's needed
	if nextLevel.MinUpload > upload {
		progress.UploadNeeded = nextLevel.MinUpload - upload
	}

	if nextLevel.MinRatio > ratio {
		progress.RatioNeeded = nextLevel.MinRatio - ratio
	}

	if nextLevel.MinDays > days {
		progress.TimeNeeded = time.Duration(nextLevel.MinDays-days) * 24 * time.Hour
	}

	// Calculate overall progress (average of all requirements)
	var uploadProgress, ratioProgress, daysProgress float64

	if nextLevel.MinUpload > 0 {
		uploadProgress = min(100, float64(upload)/float64(nextLevel.MinUpload)*100)
	} else {
		uploadProgress = 100
	}

	if nextLevel.MinRatio > 0 {
		ratioProgress = min(100, ratio/nextLevel.MinRatio*100)
	} else {
		ratioProgress = 100
	}

	if nextLevel.MinDays > 0 {
		daysProgress = min(100, float64(days)/float64(nextLevel.MinDays)*100)
	} else {
		daysProgress = 100
	}

	progress.ProgressPercent = (uploadProgress + ratioProgress + daysProgress) / 3

	return progress
}

// FormatBytes formats bytes to human readable string
func FormatBytes(bytes int64) string {
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
