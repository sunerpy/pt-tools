package v2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 无穷分享率统一记成 InfiniteRatio：有上传、下载为 0、分享率没认出来（0）或以前的 -1；
// 下载大于 0 而分享率是负数的按上传 / 下载重算；别的不动。
func TestNormalizeRatio(t *testing.T) {
	const gb = int64(1) << 30
	for _, tc := range []struct {
		name        string
		up, down    int64
		ratio, want float64
	}{
		{"只上传、没认出「无限」", 46 * gb, 0, 0, InfiniteRatio},
		{"只上传、以前记的 -1", 46 * gb, 0, -1, InfiniteRatio},
		{"只上传、已经是无穷", 46 * gb, 0, InfiniteRatio, InfiniteRatio},
		{"都为 0：站点写 ---", 0, 0, 0, 0},
		{"有下载、正常分享率", 10 * gb, 5 * gb, 2, 2},
		{"有下载、分享率 0 不动（交给驱动按上传/下载算）", 10 * gb, 5 * gb, 0, 0},
		{"有下载、-1 重算", 10 * gb, 4 * gb, -1, 2.5},
	} {
		info := UserInfo{Uploaded: tc.up, Downloaded: tc.down, Ratio: tc.ratio}
		normalizeRatio(&info)
		assert.Equal(t, tc.want, info.Ratio, tc.name)
	}
}

// 保存用户信息前统一：PTTime 页面写「分享率：无限」、正则没认出来时也存成无穷（2026-10-27）。
func TestUserInfoService_FetchAndSave_InfiniteRatio(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{Repo: repo})
	site := &MockSite{}
	site.On("ID").Return("pttime")
	site.On("GetUserInfo", ctx).Return(UserInfo{Site: "pttime", Username: "u", Uploaded: 46 << 30, Downloaded: 0, Ratio: 0}, nil)
	service.RegisterSite(site)

	info, err := service.FetchAndSave(ctx, "pttime")
	require.NoError(t, err)
	assert.Equal(t, float64(InfiniteRatio), info.Ratio)
	saved, err := repo.Get(ctx, "pttime")
	require.NoError(t, err)
	assert.Equal(t, float64(InfiniteRatio), saved.Ratio)

	// 平均分享率不计入无穷
	stats, err := repo.GetAggregated(ctx)
	require.NoError(t, err)
	assert.Zero(t, stats.AverageRatio)
}
