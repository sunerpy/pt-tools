package v2

import (
	"sync"
	"testing"
)

func TestSiteDefinitionRegistry_Register(t *testing.T) {
	// Create a new registry for testing (not using global)
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	def := &SiteDefinition{
		ID:   "test-site",
		Name: "Test Site",
	}

	registry.Register(def)

	// Verify registration
	result, ok := registry.Get("test-site")
	if !ok {
		t.Fatal("expected to find registered site definition")
	}
	if result.Name != "Test Site" {
		t.Errorf("Name = %q, want %q", result.Name, "Test Site")
	}
}

func TestSiteDefinitionRegistry_Register_Nil(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	// Should not panic
	registry.Register(nil)

	// Should not register empty ID
	registry.Register(&SiteDefinition{ID: ""})

	if len(registry.definitions) != 0 {
		t.Errorf("expected 0 definitions, got %d", len(registry.definitions))
	}
}

func TestSiteDefinitionRegistry_Get(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	def := &SiteDefinition{
		ID:   "test-site",
		Name: "Test Site",
	}
	registry.Register(def)

	tests := []struct {
		name     string
		siteID   string
		wantOK   bool
		wantName string
	}{
		{"existing site", "test-site", true, "Test Site"},
		{"non-existing site", "unknown", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := registry.Get(tt.siteID)
			if ok != tt.wantOK {
				t.Errorf("Get(%q) ok = %v, want %v", tt.siteID, ok, tt.wantOK)
			}
			if ok && result.Name != tt.wantName {
				t.Errorf("Get(%q).Name = %q, want %q", tt.siteID, result.Name, tt.wantName)
			}
		})
	}
}

func TestSiteDefinitionRegistry_GetOrDefault(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	def := &SiteDefinition{
		ID:   "test-site",
		Name: "Test Site",
	}
	registry.Register(def)

	tests := []struct {
		name     string
		siteID   string
		wantNil  bool
		wantName string
	}{
		{"existing site", "test-site", false, "Test Site"},
		{"non-existing site", "unknown", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.GetOrDefault(tt.siteID)
			if tt.wantNil {
				if result != nil {
					t.Errorf("GetOrDefault(%q) = %v, want nil", tt.siteID, result)
				}
			} else {
				if result == nil {
					t.Errorf("GetOrDefault(%q) = nil, want non-nil", tt.siteID)
				} else if result.Name != tt.wantName {
					t.Errorf("GetOrDefault(%q).Name = %q, want %q", tt.siteID, result.Name, tt.wantName)
				}
			}
		})
	}
}

func TestSiteDefinitionRegistry_List(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	// Empty registry
	list := registry.List()
	if len(list) != 0 {
		t.Errorf("List() on empty registry = %v, want empty", list)
	}

	// Add some definitions
	registry.Register(&SiteDefinition{ID: "site-a", Name: "Site A"})
	registry.Register(&SiteDefinition{ID: "site-b", Name: "Site B"})
	registry.Register(&SiteDefinition{ID: "site-c", Name: "Site C"})

	list = registry.List()
	if len(list) != 3 {
		t.Errorf("List() length = %d, want 3", len(list))
	}

	// Check all IDs are present
	ids := make(map[string]bool)
	for _, id := range list {
		ids[id] = true
	}
	for _, expected := range []string{"site-a", "site-b", "site-c"} {
		if !ids[expected] {
			t.Errorf("List() missing %q", expected)
		}
	}
}

func TestSiteDefinitionRegistry_GetAll(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	// Empty registry
	all := registry.GetAll()
	if len(all) != 0 {
		t.Errorf("GetAll() on empty registry = %v, want empty", all)
	}

	// Add some definitions
	registry.Register(&SiteDefinition{ID: "site-a", Name: "Site A"})
	registry.Register(&SiteDefinition{ID: "site-b", Name: "Site B"})

	all = registry.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll() length = %d, want 2", len(all))
	}

	// Check all definitions are present
	names := make(map[string]bool)
	for _, def := range all {
		names[def.Name] = true
	}
	for _, expected := range []string{"Site A", "Site B"} {
		if !names[expected] {
			t.Errorf("GetAll() missing definition with name %q", expected)
		}
	}
}

func TestSiteDefinitionRegistry_ConcurrentAccess(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			def := &SiteDefinition{
				ID:   "site-" + string(rune('a'+id%26)),
				Name: "Site",
			}
			registry.Register(def)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			registry.Get("site-" + string(rune('a'+id%26)))
			registry.List()
			registry.GetAll()
		}(i)
	}

	wg.Wait()

	// Should not panic and should have some definitions
	list := registry.List()
	if len(list) == 0 {
		t.Error("expected some definitions after concurrent access")
	}
}

func TestGetDefinitionRegistry_Singleton(t *testing.T) {
	// Get the global registry twice
	registry1 := GetDefinitionRegistry()
	registry2 := GetDefinitionRegistry()

	// Should be the same instance
	if registry1 != registry2 {
		t.Error("GetDefinitionRegistry() should return the same instance")
	}
}

func TestRegisterSiteDefinition_Convenience(t *testing.T) {
	// This tests the convenience function
	def := &SiteDefinition{
		ID:   "convenience-test-site",
		Name: "Convenience Test Site",
	}

	RegisterSiteDefinition(def)

	// Verify it was registered in the global registry
	result, ok := GetDefinitionRegistry().Get("convenience-test-site")
	if !ok {
		t.Fatal("expected to find registered site definition")
	}
	if result.Name != "Convenience Test Site" {
		t.Errorf("Name = %q, want %q", result.Name, "Convenience Test Site")
	}
}

func TestSiteDefinitionRegistry_Overwrite(t *testing.T) {
	registry := &SiteDefinitionRegistry{
		definitions: make(map[string]*SiteDefinition),
	}

	// Register first definition
	def1 := &SiteDefinition{
		ID:   "test-site",
		Name: "Original Name",
	}
	registry.Register(def1)

	// Register with same ID but different name
	def2 := &SiteDefinition{
		ID:   "test-site",
		Name: "Updated Name",
	}
	registry.Register(def2)

	// Should have the updated definition
	result, ok := registry.Get("test-site")
	if !ok {
		t.Fatal("expected to find registered site definition")
	}
	if result.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q", result.Name, "Updated Name")
	}
}
