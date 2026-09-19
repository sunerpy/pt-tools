// MIT License
// Copyright (c) 2025 pt-tools

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonicalFreeLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"FREE":           "FREE",
		"free":           "FREE",
		"  Free  ":       "FREE",
		"50%":            "PERCENT_50",
		"PERCENT_50":     "PERCENT_50",
		"30%":            "PERCENT_30",
		"_2X_FREE":       "2XFREE",
		"2xfree":         "2XFREE",
		"_2X_PERCENT_50": "2X50",
		"normal":         "NONE",
		"none":           "NONE",
		"":               "NONE",
		"custom":         "CUSTOM", // 认不出来的原样返回（大写），不能静默归到 NONE
	}
	for raw, want := range cases {
		assert.Equal(t, want, CanonicalFreeLevel(raw), "raw=%q", raw)
	}
}

func TestFreeLevelQueryValues(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, []string{"PERCENT_50", "50%"}, FreeLevelQueryValues("PERCENT_50"))
	assert.ElementsMatch(t, []string{"PERCENT_50", "50%"}, FreeLevelQueryValues("50%"),
		"别名进来也要展开成同一组")
	assert.ElementsMatch(t, []string{"NONE", "NORMAL", ""}, FreeLevelQueryValues("none"))
	assert.Equal(t, []string{"CUSTOM"}, FreeLevelQueryValues("custom"),
		"未知档位只匹配它自己：查询照样成立，只是命中为空")
}
