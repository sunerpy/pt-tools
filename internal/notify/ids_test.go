package notify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeIDs(t *testing.T) {
	assert.Equal(t, []string{"ou_a", "ou_b"}, NormalizeIDs([]string{" ou_a ", "", "ou_b", "ou_a", "  "}))
	assert.Equal(t, []string{}, NormalizeIDs(nil))
}
