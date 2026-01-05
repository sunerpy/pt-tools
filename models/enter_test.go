package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSiteName(t *testing.T) {
	g, err := ValidateSiteName("springsunday")
	require.NoError(t, err)
	require.Equal(t, SiteGroup("springsunday"), g)
	_, err2 := ValidateSiteName("unknown")
	require.Error(t, err2)
}
