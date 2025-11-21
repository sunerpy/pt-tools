package site

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sunerpy/pt-tools/models"
)

func TestDefaultReferer(t *testing.T) {
	r := NewDefaultReferer(models.CMCT)
	assert.NotEmpty(t, r.GetReferer())
}
