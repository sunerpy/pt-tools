package v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithFreshAfter(t *testing.T) {
	_, ok := FreshAfterFrom(context.Background())
	assert.False(t, ok)

	at := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	got, ok := FreshAfterFrom(WithFreshAfter(context.Background(), at))
	assert.True(t, ok)
	assert.True(t, at.Equal(got))

	// 零时间等于没有要求
	_, ok = FreshAfterFrom(WithFreshAfter(context.Background(), time.Time{}))
	assert.False(t, ok)
}
