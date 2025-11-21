package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"go.uber.org/zap"
)

func TestTaskListCmd_PrintsForDate(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	listCmd.Flags().Set("date", "2024-12-05")
	listCmd.Run(listCmd, []string{})
}

func TestTaskListCmd_DefaultDate(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	// reset or unset is not necessary; call without setting to use default
	listCmd.Run(listCmd, []string{})
}

func TestTaskCmd_UsageWithoutSubcommand(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	global.GlobalDB = db
	// call root of taskCmd to exercise Usage path
	taskCmd.Run(taskCmd, []string{"unknown"})
}
