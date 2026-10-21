package cachectl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBypass(t *testing.T) {
	ctx := context.Background()
	assert.False(t, Bypassed(ctx))
	assert.True(t, Bypassed(Bypass(ctx)))
	child, cancel := context.WithCancel(Bypass(ctx))
	defer cancel()
	assert.True(t, Bypassed(child), "派生的 ctx 也跳过")
}
