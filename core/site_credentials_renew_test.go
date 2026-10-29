package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
)

// BYR 续期换发的 Cookie 存回站点设置：设置里存着的是这个会话用过的某一组时才换；已经被扩展推来的新 Cookie 换掉了就不覆盖。
// 只写 Cookie 密文、不发 ConfigChanged（在用的实例已经换上新值，不用重建实例、重载调度）。
func TestSaveRenewedSiteCookie(t *testing.T) {
	writeTestSecretKey(t)
	store := NewConfigStore(newTempDB(t))
	old := "auth_token=A0; refresh_token=R0; session_id=S"
	enc, err := store.EncryptCookie(old)
	require.NoError(t, err)
	require.NoError(t, store.db.DB.Create(&models.SiteSetting{
		Name: "byrpt", AuthMethod: "cookie", Enabled: true, CookieEncrypted: enc,
	}).Error)
	stored := func() string {
		t.Helper()
		var row models.SiteSetting
		require.NoError(t, store.db.DB.Where("name = ?", "byrpt").First(&row).Error)
		plain, decErr := store.SiteCookiePlaintext(row)
		require.NoError(t, decErr)
		return plain
	}

	_, ch, cancel := events.Subscribe(4)
	defer cancel()

	renewed := "auth_token=A1; refresh_token=R1; session_id=S"
	ok, err := store.SaveRenewedSiteCookie("byrpt", []string{old, renewed}, renewed)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, renewed, stored())
	select {
	case e := <-ch:
		t.Fatalf("续期存 Cookie 不发事件，收到 %v", e.Type)
	default:
	}

	// 别的会话（它没用过设置里现在这组）续了期：不覆盖
	ok, err = store.SaveRenewedSiteCookie("byrpt", []string{old, "auth_token=A9; refresh_token=R9"}, "auth_token=A9; refresh_token=R9")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, renewed, stored())

	// 设置里存的是会话早些时候用过的一组（续期以后存回去之前又续了一次）：照样换成最新的
	latest := "auth_token=A2; refresh_token=R2; session_id=S"
	ok, err = store.SaveRenewedSiteCookie("byrpt", []string{old, renewed, "auth_token=A1b; refresh_token=R1b; session_id=S", latest}, latest)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, latest, stored())

	// 空的不存；没有这个站点时报错
	ok, err = store.SaveRenewedSiteCookie("byrpt", []string{latest}, "  ")
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = store.SaveRenewedSiteCookie("nosuch", []string{""}, "a=1")
	assert.Error(t, err)

	var row models.SiteSetting
	require.NoError(t, store.db.DB.Where("name = ?", "byrpt").First(&row).Error)
	assert.Empty(t, row.Cookie, "不写旧的明文列")
	assert.True(t, row.Enabled, "启用状态不动")
}
