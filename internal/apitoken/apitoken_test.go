package apitoken

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

type env struct {
	db    *gorm.DB
	store *Store
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.APIToken{}))
	e := &env{db: db, now: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	e.store = New(db)
	e.store.now = func() time.Time { return e.now }
	return e
}

// 新建：明文是 ptt_<id>_<secret>，只给一次；库里只有 secret 的 SHA-256，列表里没有明文与散列
func TestCreateAndList(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tok, plain, err := e.store.Create(ctx, CreateInput{Name: " 手机 ", Scopes: []string{ScopeAppWrite, ScopeAppRead, ScopeAppRead}, ExpiresInDays: 30}, "admin")
	require.NoError(t, err)
	assert.Equal(t, "手机", tok.Name)
	assert.Equal(t, []string{ScopeAppRead, ScopeAppWrite}, tok.Scopes, "去重并按固定顺序")
	require.NotNil(t, tok.ExpiresAt)
	assert.Equal(t, e.now.Add(30*24*time.Hour), *tok.ExpiresAt)
	assert.Equal(t, "admin", tok.CreatedBy)

	assert.True(t, strings.HasPrefix(plain, "ptt_"))
	id, secret, ok := parse(plain)
	require.True(t, ok)
	assert.Equal(t, tok.ID, id)
	assert.Len(t, secret, 43, "32 字节的 base64url")

	var row models.APIToken
	require.NoError(t, e.db.First(&row, tok.ID).Error)
	sum := sha256.Sum256([]byte(secret))
	assert.Equal(t, hex.EncodeToString(sum[:]), row.SecretHash)
	assert.NotContains(t, row.SecretHash, secret)

	_, plain2, err := e.store.Create(ctx, CreateInput{Name: "MCP", Scopes: []string{ScopeMCPRead}}, "admin")
	require.NoError(t, err)
	assert.NotEqual(t, plain, plain2)
	list, err := e.store.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "MCP", list[0].Name, "新的在前")
	assert.Nil(t, list[0].ExpiresAt, "不过期")
}

func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i, in := range []CreateInput{
		{Name: "", Scopes: []string{ScopeAppRead}},
		{Name: strings.Repeat("名", 65), Scopes: []string{ScopeAppRead}},
		{Name: "x"},
		{Name: "x", Scopes: []string{"app:*"}},
		{Name: "x", Scopes: []string{"admin"}},
		{Name: "x", Scopes: []string{ScopeAppRead}, ExpiresInDays: -1},
		{Name: "x", Scopes: []string{ScopeAppRead}, ExpiresInDays: 3651},
	} {
		_, _, err := e.store.Create(ctx, in, "admin")
		require.ErrorIs(t, err, ErrInvalid, "第 %d 个", i)
	}
	for i := range maxTokens {
		_, _, err := e.store.Create(ctx, CreateInput{Name: "t", Scopes: []string{ScopeAppRead}}, "admin")
		require.NoError(t, err, i)
	}
	_, _, err := e.store.Create(ctx, CreateInput{Name: "t", Scopes: []string{ScopeAppRead}}, "admin")
	require.ErrorIs(t, err, ErrInvalid, "令牌数有上限")
}

// 校验：对的令牌拿到 id、名字与 scope；格式不对、id 不存在、secret 不对、过期、撤销以后都是同一个 ErrUnauthorized
func TestVerify(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tok, plain, err := e.store.Create(ctx, CreateInput{Name: "手机", Scopes: []string{ScopeAppRead}, ExpiresInDays: 1}, "admin")
	require.NoError(t, err)
	got, err := e.store.Verify(ctx, plain)
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)
	assert.Equal(t, []string{ScopeAppRead}, got.Scopes)

	_, secret, _ := parse(plain)
	for _, bad := range []string{
		"", "ptt_", "ptt_x_" + secret, "ptt_99_" + secret, "ptt_" + itoa(tok.ID) + "_", "ptt_" + itoa(tok.ID) + "_" + secret + "x",
		"Bearer " + plain, strings.TrimPrefix(plain, "ptt_"), "ptt_" + itoa(tok.ID) + "_" + strings.Repeat("a", 200),
	} {
		_, verr := e.store.Verify(ctx, bad)
		require.ErrorIs(t, verr, ErrUnauthorized, bad)
	}

	e.now = e.now.Add(25 * time.Hour)
	_, err = e.store.Verify(ctx, plain)
	require.ErrorIs(t, err, ErrUnauthorized, "过期")

	_, plain2, err := e.store.Create(ctx, CreateInput{Name: "x", Scopes: []string{ScopeAppRead}}, "admin")
	require.NoError(t, err)
	id2, _, _ := parse(plain2)
	require.NoError(t, e.store.Revoke(ctx, id2))
	_, err = e.store.Verify(ctx, plain2)
	require.ErrorIs(t, err, ErrUnauthorized, "撤销以后立即失效")
	require.ErrorIs(t, e.store.Revoke(ctx, id2), ErrNotFound)
}

// 最近一次使用：第一次用就记下，之后 5 分钟内不再写库
func TestVerifyTouchesLastUsedThrottled(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tok, plain, err := e.store.Create(ctx, CreateInput{Name: "x", Scopes: []string{ScopeAppRead}}, "admin")
	require.NoError(t, err)
	assert.Nil(t, tok.LastUsedAt)
	first := e.now
	_, err = e.store.Verify(ctx, plain)
	require.NoError(t, err)
	lastUsed := func() time.Time {
		var row models.APIToken
		require.NoError(t, e.db.First(&row, tok.ID).Error)
		require.NotNil(t, row.LastUsedAt)
		return row.LastUsedAt.UTC()
	}
	assert.Equal(t, first, lastUsed())
	e.now = e.now.Add(4 * time.Minute)
	_, err = e.store.Verify(ctx, plain)
	require.NoError(t, err)
	assert.Equal(t, first, lastUsed(), "5 分钟内不写")
	e.now = e.now.Add(2 * time.Minute)
	_, err = e.store.Verify(ctx, plain)
	require.NoError(t, err)
	assert.Equal(t, e.now, lastUsed())
}

func TestHasScope(t *testing.T) {
	tok := Token{Scopes: []string{ScopeAppRead}}
	assert.True(t, tok.Has(ScopeAppRead))
	assert.False(t, tok.Has(ScopeAppWrite))
	assert.False(t, tok.Has("app:*"))
	assert.True(t, ValidScope(ScopeQbitCompat))
	assert.False(t, ValidScope("APP:READ"), "精确匹配")
}

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }

// 上限在并发新建时也守得住：已有 49 个时同时来 8 个，只有 1 个成功，总数正好 50
func TestCreateLimitConcurrent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := range maxTokens - 1 {
		_, _, err := e.store.Create(ctx, CreateInput{Name: "t" + strconv.Itoa(i), Scopes: []string{ScopeAppRead}}, "admin")
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 8 {
		wg.Go(func() {
			if _, _, err := e.store.Create(ctx, CreateInput{Name: "并发", Scopes: []string{ScopeAppRead}}, "admin"); err == nil {
				ok.Add(1)
			}
		})
	}
	wg.Wait()
	var n int64
	require.NoError(t, e.db.Model(&models.APIToken{}).Count(&n).Error)
	assert.EqualValues(t, maxTokens, n)
	assert.EqualValues(t, 1, ok.Load())
}
