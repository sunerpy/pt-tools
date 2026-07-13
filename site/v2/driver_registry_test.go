package v2

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCreateSiteFromDefinition_CustomDriver(t *testing.T) {
	called := false
	def := &SiteDefinition{
		ID:     "customdef",
		Schema: SchemaRousi,
		CreateDriver: func(config SiteConfig, logger *zap.Logger) (Site, error) {
			called = true
			return nil, errors.New("custom invoked")
		},
	}
	_, err := CreateSiteFromDefinition(def, SiteConfig{ID: "customdef"}, zap.NewNop())
	require.Error(t, err)
	assert.True(t, called)
}

func TestCreateSiteFromDefinition_NoFactory(t *testing.T) {
	def := &SiteDefinition{ID: "x", Schema: Schema("nonexistent-schema")}
	_, err := CreateSiteFromDefinition(def, SiteConfig{ID: "x"}, zap.NewNop())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no driver registered")
}

func TestCreateSiteFromDefinition_FactoryDispatch(t *testing.T) {
	def := &SiteDefinition{ID: "hdsky", Schema: SchemaNexusPHP}
	opts, _ := json.Marshal(NexusPHPOptions{Cookie: "c=1"})
	site, err := CreateSiteFromDefinition(def, SiteConfig{ID: "hdsky", Name: "HDSky", BaseURL: "https://hdsky.me", Options: opts}, zap.NewNop())
	require.NoError(t, err)
	require.NotNil(t, site)
}

// ---------------------------------------------------------------------------
// failover.go — GetCurrentURL empty, GetCurrentBaseURL
// ---------------------------------------------------------------------------

func TestListRegisteredSchemas(t *testing.T) {
	schemas := ListRegisteredSchemas()
	assert.NotEmpty(t, schemas)
	found := false
	for _, s := range schemas {
		if s == "NexusPHP" {
			found = true
		}
	}
	assert.True(t, found)
}
