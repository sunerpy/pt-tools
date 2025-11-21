package cmd

import (
	"testing"
)

func TestDbCmd_NoOpRun(t *testing.T) {
	dbCmd.Run(dbCmd, []string{})
}

func TestDbCmd_HasPersistentPreRun(t *testing.T) {
	if dbCmd.PersistentPreRun == nil {
		t.Fatalf("expected persistent pre run")
	}
}
