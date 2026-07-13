package qbit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQbitConfigGetURL_SchemeAndTrim(t *testing.T) {
	assert.Equal(t, "http://host:8080", (&QBitConfig{URL: "  host:8080/ "}).GetURL())
	assert.Equal(t, "https://q.example", (&QBitConfig{URL: "https://q.example"}).GetURL())
	assert.Equal(t, "", (&QBitConfig{URL: ""}).GetURL())
}

func TestQbitConfigValidate_Branches(t *testing.T) {
	assert.Error(t, (&QBitConfig{URL: ""}).Validate())
	assert.Error(t, (&QBitConfig{URL: "://bad"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "ftp://host"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "http://u:p@host"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "http://host#frag"}).Validate())
	assert.NoError(t, (&QBitConfig{URL: "http://host:8080"}).Validate())
}

func TestQbitEnter_SLoggerNotNil(t *testing.T) {
	assert.NotNil(t, sLogger())
}
