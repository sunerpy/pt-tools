package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	scrapercore "github.com/sunerpy/pt-tools/internal/scraper/core"
)

// TestRegisterScraperWriters_AllFourDialects 验证 4 种 NFO 方言都成功注册到 registry。
// 这是对之前 blocker 的回归测试：writerReg 为空会导致 scrape 在 writing_nfo 阶段
// 无法找到 writer，整个刮削失败。
func TestRegisterScraperWriters_AllFourDialects(t *testing.T) {
	reg := scrapercore.NewRegistry[scrapercore.NfoWriter]()
	require.NoError(t, registerScraperWriters(reg))

	expected := []string{"kodi", "jellyfin", "emby", "universal"}
	for _, name := range expected {
		t.Run(name, func(t *testing.T) {
			assert.True(t, reg.Has(name), "%s writer should be registered", name)
			writer, err := reg.Get(name)
			require.NoError(t, err)
			require.NotNil(t, writer)
			assert.Equal(t, name, writer.Dialect(), "Dialect() should return %q", name)
		})
	}

	names := reg.List()
	assert.ElementsMatch(t, expected, names)
}

// TestRegisterScraperWriters_Idempotent 确认重复注册会返回错误（保护性测试）。
func TestRegisterScraperWriters_Idempotent(t *testing.T) {
	reg := scrapercore.NewRegistry[scrapercore.NfoWriter]()
	require.NoError(t, registerScraperWriters(reg))
	require.Error(t, registerScraperWriters(reg), "double registration should fail")
}
