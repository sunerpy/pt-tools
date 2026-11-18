package nas

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtraEncoding(t *testing.T) {
	s, err := EncodeExtra(nil)
	require.NoError(t, err)
	assert.Empty(t, s, "没有时是空串")
	s, err = EncodeExtra(map[string]string{"did": "d1", "token": "t"})
	require.NoError(t, err)
	assert.Equal(t, `{"did":"d1","token":"t"}`, s)
	got, err := DecodeExtra(s)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"did": "d1", "token": "t"}, got)
	got, err = DecodeExtra("")
	require.NoError(t, err)
	assert.Nil(t, got)
	_, err = DecodeExtra("not json")
	assert.ErrorContains(t, err, "登录凭据的格式不对")
}
