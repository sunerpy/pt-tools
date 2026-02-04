package definitions

import (
	"testing"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestHDSkyDefinition(t *testing.T) {
	// Verify HDSky definition is registered
	def, ok := v2.GetDefinitionRegistry().Get("hdsky")
	if !ok {
		t.Fatal("HDSky definition not found in registry")
	}

	// Verify basic properties
	if def.Name != "HDSky" {
		t.Errorf("Name = %q, want %q", def.Name, "HDSky")
	}
	if def.Schema != "NexusPHP" {
		t.Errorf("Schema = %q, want %q", def.Schema, "NexusPHP")
	}
	if len(def.URLs) == 0 {
		t.Error("URLs should not be empty")
	}
	if def.UserInfo == nil {
		t.Error("UserInfo should not be nil")
	}
	if len(def.LevelRequirements) != 9 {
		t.Errorf("LevelRequirements count = %d, want 9", len(def.LevelRequirements))
	}
}

func TestHDSkyLevelRequirements(t *testing.T) {
	// Test new requirements (after 2025-03-01)
	newReqs := GetHDSkyLevelRequirements(time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC).Unix())
	if len(newReqs) != 9 {
		t.Errorf("New requirements count = %d, want 9", len(newReqs))
	}
	// New requirements should have bonus requirement for Power User
	if newReqs[1].Bonus == 0 {
		t.Error("New Power User requirements should have bonus requirement")
	}

	// Test old requirements (before 2025-03-01)
	oldReqs := GetHDSkyLevelRequirements(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	if len(oldReqs) != 9 {
		t.Errorf("Old requirements count = %d, want 9", len(oldReqs))
	}
	// Old requirements should NOT have bonus requirement for Power User
	if oldReqs[1].Bonus != 0 {
		t.Error("Old Power User requirements should NOT have bonus requirement")
	}
}

func TestSpringSundayDefinition(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("springsunday")
	if !ok {
		t.Fatal("SpringSunday definition not found in registry")
	}

	if def.Name != "SpringSunday" {
		t.Errorf("Name = %q, want %q", def.Name, "SpringSunday")
	}
	if len(def.Aka) == 0 {
		t.Error("Aka should not be empty")
	}

	// Verify alternative requirements exist
	hasAlternative := false
	for _, req := range def.LevelRequirements {
		if len(req.Alternative) > 0 {
			hasAlternative = true
			break
		}
	}
	if !hasAlternative {
		t.Error("SpringSunday should have alternative requirements")
	}
}

func TestMTeamDefinition(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("mteam")
	if !ok {
		t.Fatal("MTeam definition not found in registry")
	}

	if def.Name != "M-Team - TP" {
		t.Errorf("Name = %q, want %q", def.Name, "M-Team - TP")
	}
	if def.Schema != "mTorrent" {
		t.Errorf("Schema = %q, want %q", def.Schema, "mTorrent")
	}
	if len(def.URLs) < 2 {
		t.Error("MTeam should have multiple URLs")
	}
	if len(def.LevelRequirements) != 10 {
		t.Errorf("LevelRequirements count = %d, want 10", len(def.LevelRequirements))
	}
}

func TestMTeamLevelIDMap(t *testing.T) {
	tests := []struct {
		roleID   string
		expected string
	}{
		{"1", "User"},
		{"2", "Power User"},
		{"9", "Nexus Master"},
		{"10", "VIP"},
		{"99", "99"}, // Unknown role returns the ID
	}

	for _, tt := range tests {
		t.Run(tt.roleID, func(t *testing.T) {
			result := GetMTeamLevelName(tt.roleID)
			if result != tt.expected {
				t.Errorf("GetMTeamLevelName(%q) = %q, want %q", tt.roleID, result, tt.expected)
			}
		})
	}
}

func TestHDDolbyDefinition(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("hddolby")
	if !ok {
		t.Fatal("HDDolby definition not found in registry")
	}

	if def.Name != "HD Dolby" {
		t.Errorf("Name = %q, want %q", def.Name, "HD Dolby")
	}
	if def.Schema != "HDDolby" {
		t.Errorf("Schema = %q, want %q", def.Schema, "HDDolby")
	}
	if def.AuthMethod != "cookie_and_api_key" {
		t.Errorf("AuthMethod = %q, want %q", def.AuthMethod, "cookie_and_api_key")
	}

	// Verify seedingBonus requirements exist
	hasSeedingBonus := false
	for _, req := range def.LevelRequirements {
		if req.SeedingBonus > 0 {
			hasSeedingBonus = true
			break
		}
	}
	if !hasSeedingBonus {
		t.Error("HDDolby should have seedingBonus requirements")
	}
}

func TestNovaHDDefinition(t *testing.T) {
	def, ok := v2.GetDefinitionRegistry().Get("novahd")
	if !ok {
		t.Fatal("NovaHD definition not found in registry")
	}

	if def.Name != "NovaHD" {
		t.Errorf("Name = %q, want %q", def.Name, "NovaHD")
	}
	if def.Schema != "NexusPHP" {
		t.Errorf("Schema = %q, want %q", def.Schema, "NexusPHP")
	}
	if len(def.URLs) == 0 || def.URLs[0] != "https://pt.novahd.top/" {
		t.Errorf("URLs = %v, want [https://pt.novahd.top/]", def.URLs)
	}
	if def.UserInfo == nil {
		t.Error("UserInfo should not be nil")
	}
	if def.DetailParser == nil {
		t.Error("DetailParser should not be nil")
	}
	if len(def.LevelRequirements) != 9 {
		t.Errorf("LevelRequirements count = %d, want 9", len(def.LevelRequirements))
	}

	if def.DetailParser.DiscountSelector != "h1 font.free, h1 font[class]" {
		t.Errorf("DiscountSelector = %q, want %q", def.DetailParser.DiscountSelector, "h1 font.free, h1 font[class]")
	}
	if def.DetailParser.EndTimeSelector != "h1 span[title]" {
		t.Errorf("EndTimeSelector = %q, want %q", def.DetailParser.EndTimeSelector, "h1 span[title]")
	}
}

func TestAllDefinitionsRegistered(t *testing.T) {
	registry := v2.GetDefinitionRegistry()
	expectedSites := []string{"hdsky", "springsunday", "mteam", "hddolby", "novahd", "ourbits"}

	for _, siteID := range expectedSites {
		if _, ok := registry.Get(siteID); !ok {
			t.Errorf("Site %q not found in registry", siteID)
		}
	}
}

func TestOurBitsDefinition(t *testing.T) {
	// Verify OurBits definition is registered
	def, ok := v2.GetDefinitionRegistry().Get("ourbits")
	if !ok {
		t.Fatal("OurBits definition not found in registry")
	}

	// Verify basic properties
	if def.Name != "OurBits" {
		t.Errorf("Name = %q, want %q", def.Name, "OurBits")
	}
	if def.Schema != "NexusPHP" {
		t.Errorf("Schema = %q, want %q", def.Schema, "NexusPHP")
	}
	if len(def.URLs) == 0 {
		t.Error("URLs should not be empty")
	}
	if def.UserInfo == nil {
		t.Error("UserInfo should not be nil")
	}
	if len(def.LevelRequirements) != 11 {
		t.Errorf("LevelRequirements count = %d, want 11", len(def.LevelRequirements))
	}
}

func TestDefinitionUserInfoConfig(t *testing.T) {
	tests := []struct {
		siteID       string
		expectSteps  int
		expectFields bool
	}{
		{"hdsky", 3, true},
		{"springsunday", 3, true},
		{"mteam", 4, true},
		{"ourbits", 3, true},
	}

	registry := v2.GetDefinitionRegistry()

	for _, tt := range tests {
		t.Run(tt.siteID, func(t *testing.T) {
			def, ok := registry.Get(tt.siteID)
			if !ok {
				t.Fatalf("Site %q not found", tt.siteID)
			}

			if def.UserInfo == nil {
				t.Fatal("UserInfo should not be nil")
			}

			if len(def.UserInfo.Process) != tt.expectSteps {
				t.Errorf("Process steps = %d, want %d", len(def.UserInfo.Process), tt.expectSteps)
			}

			if tt.expectFields && len(def.UserInfo.Selectors) == 0 {
				t.Error("Selectors should not be empty")
			}
		})
	}
}

func TestLevelProgressCalculation(t *testing.T) {
	def, _ := v2.GetDefinitionRegistry().Get("hdsky")

	// Create a user at level 1
	info := &v2.UserInfo{
		LevelID:    1,
		Downloaded: 100 * 1024 * 1024 * 1024, // 100GB
		Ratio:      1.5,
		Bonus:      300000,
	}

	progress := v2.CalculateSiteLevelProgress(info, def.LevelRequirements)
	if progress == nil {
		t.Fatal("Progress should not be nil")
		return
	}

	if progress.CurrentLevel == nil {
		t.Fatal("CurrentLevel should not be nil")
		return
	}
	if progress.CurrentLevel.Name != "User" {
		t.Errorf("CurrentLevel.Name = %q, want %q", progress.CurrentLevel.Name, "User")
	}

	if progress.NextLevel == nil {
		t.Fatal("NextLevel should not be nil")
	}
	if progress.NextLevel.Name != "Power User" {
		t.Errorf("NextLevel.Name = %q, want %q", progress.NextLevel.Name, "Power User")
	}

	// Should have unmet requirements
	if len(progress.UnmetRequirements) == 0 {
		t.Error("Should have unmet requirements")
	}
}
