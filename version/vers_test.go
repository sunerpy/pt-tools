package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionDefaults(t *testing.T) {
	assert.NotEmpty(t, Version)
	assert.NotEmpty(t, BuildTime)
	assert.NotEmpty(t, CommitID)
}
