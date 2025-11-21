package global

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitAndGetLogger(t *testing.T) {
	InitLogger(zap.NewNop())
	require.NotNil(t, GetLogger())
	require.NotNil(t, GetSlogger())
}
