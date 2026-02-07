package v2

import (
	"fmt"
	"sync"
)

// ErrDuplicateSiteID is returned when attempting to register a site with an already-used ID
var ErrDuplicateSiteID = fmt.Errorf("duplicate site definition ID")

// SiteDefinitionRegistry manages site definitions
type SiteDefinitionRegistry struct {
	mu          sync.RWMutex
	definitions map[string]*SiteDefinition
}

var (
	globalDefinitionRegistry *SiteDefinitionRegistry
	definitionRegistryOnce   sync.Once
)

// GetDefinitionRegistry returns the global site definition registry
func GetDefinitionRegistry() *SiteDefinitionRegistry {
	definitionRegistryOnce.Do(func() {
		globalDefinitionRegistry = &SiteDefinitionRegistry{
			definitions: make(map[string]*SiteDefinition),
		}
	})
	return globalDefinitionRegistry
}

// Register adds a site definition to the registry.
// Panics if a definition with the same ID is already registered.
func (r *SiteDefinitionRegistry) Register(def *SiteDefinition) {
	if def == nil || def.ID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.definitions[def.ID]; ok && existing != def {
		panic(fmt.Sprintf("duplicate site definition ID %q: already registered by %q", def.ID, existing.Name))
	}
	r.definitions[def.ID] = def
}

// Get retrieves a site definition by ID
func (r *SiteDefinitionRegistry) Get(siteID string) (*SiteDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[siteID]
	return def, ok
}

// GetOrDefault returns site definition or nil if not found
func (r *SiteDefinitionRegistry) GetOrDefault(siteID string) *SiteDefinition {
	def, ok := r.Get(siteID)
	if ok {
		return def
	}
	return nil
}

// List returns all registered site IDs
func (r *SiteDefinitionRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	return ids
}

// GetAll returns all registered site definitions
func (r *SiteDefinitionRegistry) GetAll() []*SiteDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]*SiteDefinition, 0, len(r.definitions))
	for _, def := range r.definitions {
		defs = append(defs, def)
	}
	return defs
}

// RegisterSiteDefinition is a convenience function for init() registration
func RegisterSiteDefinition(def *SiteDefinition) {
	GetDefinitionRegistry().Register(def)
}
