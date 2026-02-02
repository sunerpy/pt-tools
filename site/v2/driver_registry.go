package v2

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

type DriverFactory func(config SiteConfig, logger *zap.Logger) (Site, error)

type driverRegistry struct {
	mu        sync.RWMutex
	factories map[string]DriverFactory
}

var globalDriverRegistry = &driverRegistry{
	factories: make(map[string]DriverFactory),
}

func RegisterDriverForSchema(schema string, factory DriverFactory) {
	globalDriverRegistry.mu.Lock()
	defer globalDriverRegistry.mu.Unlock()
	globalDriverRegistry.factories[schema] = factory
}

func GetDriverFactoryForSchema(schema string) (DriverFactory, bool) {
	globalDriverRegistry.mu.RLock()
	defer globalDriverRegistry.mu.RUnlock()
	factory, ok := globalDriverRegistry.factories[schema]
	return factory, ok
}

func ListRegisteredSchemas() []string {
	globalDriverRegistry.mu.RLock()
	defer globalDriverRegistry.mu.RUnlock()
	schemas := make([]string, 0, len(globalDriverRegistry.factories))
	for schema := range globalDriverRegistry.factories {
		schemas = append(schemas, schema)
	}
	return schemas
}

func CreateSiteFromDefinition(def *SiteDefinition, config SiteConfig, logger *zap.Logger) (Site, error) {
	if def.CreateDriver != nil {
		return def.CreateDriver(config, logger)
	}

	factory, ok := GetDriverFactoryForSchema(def.Schema.String())
	if !ok {
		return nil, fmt.Errorf("no driver registered for schema: %s", def.Schema)
	}

	return factory(config, logger)
}
