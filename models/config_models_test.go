package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsGlobal_Defaults(t *testing.T) {
	var g SettingsGlobal
	require.Equal(t, 0, g.MaxRetry)
	require.Equal(t, 0, g.RetainHours)
}
