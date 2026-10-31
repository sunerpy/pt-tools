package core

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils/httpclient"
)

func newSiteProxyStore(t *testing.T) *ConfigStore {
	t.Helper()
	writeTestSecretKey(t)
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	// 每个测试从「关闭」开始，结束时也还原，免得快照漏到别的测试
	httpclient.SetSiteProxyPolicy(nil)
	t.Cleanup(func() { httpclient.SetSiteProxyPolicy(nil) })
	return NewConfigStore(db)
}

func TestSiteProxyDefaults(t *testing.T) {
	s := newSiteProxyStore(t)
	got, err := s.SiteProxy()
	require.NoError(t, err)
	assert.False(t, got.Enabled)
	assert.Equal(t, "", got.ProxyURL)
	assert.Equal(t, httpclient.SiteProxyAll, got.Mode)
	assert.Equal(t, []string{}, got.Sites)
	assert.Nil(t, got.UpdatedAt)

	require.NoError(t, s.LoadSiteProxyPolicy())
	_, decided := httpclient.ResolveSiteProxy("hdsky")
	assert.False(t, decided, "没有设置时不判定")
}

func TestSaveSiteProxyRoundTripAndPolicy(t *testing.T) {
	s := newSiteProxyStore(t)
	saved, err := s.SaveSiteProxy(SiteProxyInput{
		Enabled: true, ProxyURL: " http://user:secret@192.168.3.150:1081 ", Mode: httpclient.SiteProxyInclude,
		Sites: []string{" HDSky ", "hdsky", "", "RousiPro"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"hdsky", "rousipro"}, saved.Sites, "规范化、去重、排序")
	assert.Equal(t, "http://user:secret@192.168.3.150:1081", saved.ProxyURL)
	require.NotNil(t, saved.UpdatedAt)

	var row models.SiteProxySetting
	require.NoError(t, s.db.DB.First(&row, 1).Error)
	assert.NotEmpty(t, row.ProxyEncrypted)
	assert.NotContains(t, row.ProxyEncrypted, "secret", "库里存的不是明文")
	assert.NotContains(t, row.ProxyEncrypted, "192.168.3.150")

	got, err := s.SiteProxy()
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.Equal(t, "http://user:secret@192.168.3.150:1081", got.ProxyURL)
	assert.Equal(t, httpclient.SiteProxyInclude, got.Mode)
	assert.Equal(t, []string{"hdsky", "rousipro"}, got.Sites)

	// 保存以后快照立即是新的
	proxy, decided := httpclient.ResolveSiteProxy("HDSky")
	assert.True(t, decided)
	assert.Equal(t, "http://user:secret@192.168.3.150:1081", proxy)
	proxy, decided = httpclient.ResolveSiteProxy("mteam")
	assert.True(t, decided)
	assert.Equal(t, "", proxy, "没选中的直连")
}

func TestSaveSiteProxyKeepsMaskedAndClears(t *testing.T) {
	s := newSiteProxyStore(t)
	_, err := s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "socks5://user:secret@p.test:1080", Mode: httpclient.SiteProxyAll})
	require.NoError(t, err)

	// 界面交回打了码的地址：不改
	saved, err := s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "socks5://user:***@p.test:1080", Mode: httpclient.SiteProxyExclude, Sites: []string{"hdsky"}})
	require.NoError(t, err)
	assert.Equal(t, "socks5://user:secret@p.test:1080", saved.ProxyURL)
	proxy, decided := httpclient.ResolveSiteProxy("mteam")
	assert.True(t, decided)
	assert.Equal(t, "socks5://user:secret@p.test:1080", proxy)

	// 关闭时照样留着地址、模式与站点
	saved, err = s.SaveSiteProxy(SiteProxyInput{Enabled: false, ProxyURL: "socks5://user:***@p.test:1080", Mode: httpclient.SiteProxyExclude, Sites: []string{"hdsky"}})
	require.NoError(t, err)
	assert.Equal(t, "socks5://user:secret@p.test:1080", saved.ProxyURL)
	_, decided = httpclient.ResolveSiteProxy("mteam")
	assert.False(t, decided, "关闭以后不判定")

	// 空串表示清除
	_, err = s.SaveSiteProxy(SiteProxyInput{Enabled: false, ProxyURL: "", Mode: httpclient.SiteProxyAll})
	require.NoError(t, err)
	got, err := s.SiteProxy()
	require.NoError(t, err)
	assert.Equal(t, "", got.ProxyURL)
	var row models.SiteProxySetting
	require.NoError(t, s.db.DB.First(&row, 1).Error)
	assert.Equal(t, "", row.ProxyEncrypted)
}

func TestSaveSiteProxyValidation(t *testing.T) {
	s := newSiteProxyStore(t)
	cases := []struct {
		name string
		in   SiteProxyInput
		want string
	}{
		{name: "坏地址", in: SiteProxyInput{Enabled: true, ProxyURL: "ftp://user:secret@p.test", Mode: httpclient.SiteProxyAll}, want: "代理地址要以"},
		{name: "没有主机", in: SiteProxyInput{Enabled: true, ProxyURL: "http://", Mode: httpclient.SiteProxyAll}, want: "代理地址格式不对"},
		{name: "坏模式", in: SiteProxyInput{Enabled: true, ProxyURL: "http://p.test:1", Mode: "some"}, want: "不认识的模式"},
		{name: "开启却没地址", in: SiteProxyInput{Enabled: true, Mode: httpclient.SiteProxyAll}, want: "开启时要填代理地址"},
		{name: "只有选中的却没选", in: SiteProxyInput{Enabled: true, ProxyURL: "http://p.test:1", Mode: httpclient.SiteProxyInclude, Sites: []string{" "}}, want: "没有选站点"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.SaveSiteProxy(tc.in)
			require.ErrorIs(t, err, ErrSiteProxyInvalid)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "secret", "错误里没有密码")
		})
	}
	var n int64
	require.NoError(t, s.db.DB.Model(&models.SiteProxySetting{}).Count(&n).Error)
	assert.Zero(t, n, "校验失败不写库")

	// 「除了选中的」不选站点等于全部站点
	_, err := s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "http://p.test:1", Mode: httpclient.SiteProxyExclude})
	require.NoError(t, err)
	proxy, decided := httpclient.ResolveSiteProxy("hdsky")
	assert.True(t, decided)
	assert.Equal(t, "http://p.test:1", proxy)
}

func TestLoadSiteProxyPolicyUnreadable(t *testing.T) {
	s := newSiteProxyStore(t)
	require.NoError(t, s.db.DB.Create(&models.SiteProxySetting{ID: 1, Enabled: true, ProxyEncrypted: "not-a-ciphertext", Mode: httpclient.SiteProxyAll, SitesJSON: "[]"}).Error)
	httpclient.SetSiteProxyPolicy(&httpclient.SiteProxyPolicy{Enabled: true, Proxy: "http://old.test:1", Mode: httpclient.SiteProxyAll})

	got, err := s.SiteProxy()
	require.NoError(t, err)
	assert.True(t, got.ProxyUnreadable)
	assert.Equal(t, "", got.ProxyURL)

	err = s.LoadSiteProxyPolicy()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "解不开")
	_, decided := httpclient.ResolveSiteProxy("hdsky")
	assert.False(t, decided, "解不开时按关闭处理")

	// 交回空地址、开启：要重新填
	_, err = s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "", Mode: httpclient.SiteProxyAll})
	require.ErrorIs(t, err, ErrSiteProxyInvalid)
	_, err = s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "http://new.test:1", Mode: httpclient.SiteProxyAll})
	require.NoError(t, err)
	require.NoError(t, s.LoadSiteProxyPolicy())
	proxy, _ := httpclient.ResolveSiteProxy("hdsky")
	assert.Equal(t, "http://new.test:1", proxy)
}

func TestSaveSiteProxyConcurrentSnapshotMatchesDB(t *testing.T) {
	s := newSiteProxyStore(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			_, err := s.SaveSiteProxy(SiteProxyInput{Enabled: true, ProxyURL: "http://p.test:" + strings.Repeat("1", i%4+1), Mode: httpclient.SiteProxyAll})
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	got, err := s.SiteProxy()
	require.NoError(t, err)
	proxy, decided := httpclient.ResolveSiteProxy("hdsky")
	assert.True(t, decided)
	assert.Equal(t, got.ProxyURL, proxy, "快照就是库里最后保存的那一份")
}
