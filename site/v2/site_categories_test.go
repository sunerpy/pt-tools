package v2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSiteCategoryConfig(t *testing.T) {
	cfg := GetSiteCategoryConfig("mteam")
	require.NotNil(t, cfg)
	assert.Equal(t, "mteam", cfg.SiteID)
	assert.NotEmpty(t, cfg.Categories)

	assert.Nil(t, GetSiteCategoryConfig("nonexistent-site"))

	all := GetAllSiteCategoryConfigs()
	assert.NotEmpty(t, all)
}
