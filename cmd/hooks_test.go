package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
)

func TestPersistentCheckCfg_DownloadDirEmpty_ExitPath(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	cmd := &cobra.Command{}
	defer func() { _ = recover() }()
	PersistentCheckCfg(cmd, []string{})
}

func TestPersistentCheckCfg_InitToolsFail(t *testing.T) {
	// simulate initTools failure by setting GlobalDB nil and intercepting exit
	global.GlobalDB = nil
	defer func() { _ = recover() }()
	PersistentCheckCfg(&cobra.Command{}, []string{})
}
