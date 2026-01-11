package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
)

func TestTaskCmd_BasicUsage(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	// just invoke Run path
	taskCmd.Run(taskCmd, []string{})
}
