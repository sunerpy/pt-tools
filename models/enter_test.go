package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSiteName(t *testing.T) {
	g, err := ValidateSiteName("cmct")
	require.NoError(t, err)
	require.Equal(t, SiteGroup("cmct"), g)
	_, err2 := ValidateSiteName("unknown")
	require.Error(t, err2)
}
