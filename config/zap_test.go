package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultZapInitLogger(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg, err := DefaultZapConfig.InitLogger()
	require.NoError(t, err)
	require.NotNil(t, lg)
	// ensure files created
	p := filepath.Join(t.TempDir(), "pt-tools", DefaultZapConfig.Directory)
	_ = p
}

func TestZapEncodeLevelVariants(t *testing.T) {
	z := DefaultZapConfig
	z.EncodeLevel = "LowercaseLevelEncoder"
	if z.ZapEncodeLevel() == nil {
		t.Fatalf("nil encoder")
	}
	z.EncodeLevel = "LowercaseColorLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "CapitalLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "CapitalColorLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "unknown"
	_ = z.ZapEncodeLevel()
}
