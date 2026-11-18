package chatops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitReply(t *testing.T) {
	assert.Equal(t, []string{"短"}, SplitReply("短", ""))
	line := strings.Repeat("字", 100) + "\n"
	long := strings.Repeat(line, 50) // 5050 个字
	parts := SplitReply(long, "")
	require.Len(t, parts, 2)
	for _, p := range parts {
		assert.LessOrEqual(t, len([]rune(p)), ReplyRunes)
		assert.False(t, strings.HasSuffix(p, "\n"))
	}
	assert.Equal(t, strings.ReplaceAll(long, "\n", ""), strings.ReplaceAll(strings.Join(parts, ""), "\n", ""), "分开以后内容不少")
	huge := strings.Repeat("字", ReplyRunes*ReplyParts+100)
	parts = SplitReply(huge, "")
	require.Len(t, parts, ReplyParts)
	assert.Contains(t, parts[len(parts)-1], "后面的省略了")
	for _, p := range parts {
		assert.LessOrEqual(t, len([]rune(p)), ReplyRunes, "注明的话也算在字数里")
	}
	parts = SplitReply(huge, "en")
	assert.Contains(t, parts[len(parts)-1], "the rest is left out")
}
