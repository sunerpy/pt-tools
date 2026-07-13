package transmission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrConfigGetURL_SchemeAndTrim(t *testing.T) {
	c := &TransmissionConfig{URL: "  host:9091/  "}
	assert.Equal(t, "http://host:9091", c.GetURL())

	c2 := &TransmissionConfig{URL: "https://tr.example:9091"}
	assert.Equal(t, "https://tr.example:9091", c2.GetURL())

	c3 := &TransmissionConfig{URL: ""}
	assert.Equal(t, "", c3.GetURL())
}

func TestTrConfigValidate_Branches(t *testing.T) {
	assert.Error(t, (&TransmissionConfig{URL: ""}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "://bad"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "ftp://host"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "http://user:pass@host"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "http://host#frag"}).Validate())
	assert.NoError(t, (&TransmissionConfig{URL: "http://host:9091"}).Validate())
}

func TestTrEnter_SLoggerNotNil(t *testing.T) {
	assert.NotNil(t, sLogger())
}
