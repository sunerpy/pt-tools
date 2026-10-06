package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:                 "0 B",
		1023:              "1023 B",
		1024:              "1.00 KiB",
		1536:              "1.50 KiB",
		5 << 30:           "5.00 GiB",
		3 << 40:           "3.00 TiB",
		-(2 << 20):        "-2.00 MiB",
		1<<62 + (1 << 61): "6.00 EiB",
	}
	for in, want := range cases {
		assert.Equal(t, want, FormatBytes(in), "%d", in)
	}
}
