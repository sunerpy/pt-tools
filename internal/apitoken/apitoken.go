// Package apitoken 管理 API 令牌（路线图 M12）：给 App、MCP 与 qB 兼容入口用，网页登录仍然只认 session。
//
// 令牌是 `ptt_<id>_<secret>`，secret 是 32 字节随机数的 base64url。库里只存 secret 的 SHA-256：按 id 找到那一行，
// 再用常量时间比较散列。高熵的随机令牌不需要 bcrypt，每个请求也不用做一次慢散列。
// 格式不对、id 不存在、secret 不对、过期、已撤销一律返回同一个 ErrUnauthorized，不给枚举的线索。
package apitoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// 权限范围：精确匹配，不支持通配。
const (
	ScopeAppRead    = "app:read"
	ScopeAppWrite   = "app:write"
	ScopeMCPRead    = "mcp:read"
	ScopeMCPWrite   = "mcp:write"
	ScopeQbitCompat = "qbit:compat"
)

// Scopes 是所有权限范围，也是令牌里 scope 的排列顺序。
var Scopes = []string{ScopeAppRead, ScopeAppWrite, ScopeMCPRead, ScopeMCPWrite, ScopeQbitCompat}

var (
	ErrInvalid      = errors.New("参数不对")
	ErrNotFound     = errors.New("没有这个令牌")
	ErrUnauthorized = errors.New("令牌无效或已过期")
)

const (
	prefix      = "ptt_"
	secretBytes = 32
	// maxSecretLen 是 secret 部分的长度上限：32 字节的 base64url 是 43 个字符，再长的直接拒绝
	maxSecretLen = 64
	// maxTokens 是令牌数上限
	maxTokens = 50
	// touchEvery 是「最近一次使用」最多多久写一次库
	touchEvery = 5 * time.Minute
	maxDays    = 3650
)

// Token 是一个令牌（不含明文与散列）。
type Token struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Has 报告令牌有没有这个权限范围。
func (t Token) Has(scope string) bool { return slices.Contains(t.Scopes, scope) }

// ValidScope 报告是不是认识的权限范围。
func ValidScope(s string) bool { return slices.Contains(Scopes, s) }

// CreateInput 是新建令牌的内容。ExpiresInDays 为 0 时不过期。
type CreateInput struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expires_in_days"`
}

// Store 存取令牌。
type Store struct {
	db   *gorm.DB
	now  func() time.Time
	rand io.Reader
	// createMu 让「数一数、再插入」成为一步，否则并发新建能越过上限。生产里只有一个 Store（cmd/web.go）。
	createMu sync.Mutex
}

// New 建一个令牌库。
func New(db *gorm.DB) *Store {
	return &Store{db: db, now: time.Now, rand: rand.Reader}
}

func view(r models.APIToken) Token {
	t := Token{ID: r.ID, Name: r.Name, Scopes: strings.Fields(r.Scopes), CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}
	if t.Scopes == nil {
		t.Scopes = []string{}
	}
	if r.ExpiresAt != nil {
		e := r.ExpiresAt.UTC()
		t.ExpiresAt = &e
	}
	if r.LastUsedAt != nil {
		u := r.LastUsedAt.UTC()
		t.LastUsedAt = &u
	}
	return t
}

func hashOf(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// cleanScopes 检查并规整权限范围：去重，按 Scopes 的顺序排。
func cleanScopes(in []string) ([]string, error) {
	var out []string
	for _, s := range in {
		if !ValidScope(s) {
			return nil, fmt.Errorf("%w: 不认识的权限范围 %q", ErrInvalid, s)
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: 至少选一个权限范围", ErrInvalid)
	}
	slices.SortFunc(out, func(a, b string) int { return slices.Index(Scopes, a) - slices.Index(Scopes, b) })
	return out, nil
}

// Create 新建令牌，返回令牌与明文（明文只在这里给一次）。
func (s *Store) Create(ctx context.Context, in CreateInput, by string) (Token, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return Token{}, "", fmt.Errorf("%w: 名字要填，最多 64 个字", ErrInvalid)
	}
	scopes, err := cleanScopes(in.Scopes)
	if err != nil {
		return Token{}, "", err
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > maxDays {
		return Token{}, "", fmt.Errorf("%w: 有效期要在 0（不过期）到 %d 天之间", ErrInvalid, maxDays)
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	var n int64
	if err := s.db.WithContext(ctx).Model(&models.APIToken{}).Count(&n).Error; err != nil {
		return Token{}, "", fmt.Errorf("读取令牌失败: %w", err)
	}
	if n >= maxTokens {
		return Token{}, "", fmt.Errorf("%w: 令牌最多 %d 个，先撤销用不着的", ErrInvalid, maxTokens)
	}
	buf := make([]byte, secretBytes)
	if _, err := io.ReadFull(s.rand, buf); err != nil {
		return Token{}, "", fmt.Errorf("生成令牌失败: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	now := s.now().UTC()
	row := models.APIToken{
		Name: name, SecretHash: hashOf(secret), Scopes: strings.Join(scopes, " "),
		CreatedBy: truncate(by, 64), CreatedAt: now,
	}
	if in.ExpiresInDays > 0 {
		e := now.Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour)
		row.ExpiresAt = &e
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Token{}, "", fmt.Errorf("保存令牌失败: %w", err)
	}
	return view(row), prefix + strconv.FormatUint(uint64(row.ID), 10) + "_" + secret, nil
}

// List 列出令牌（新的在前）。
func (s *Store) List(ctx context.Context) ([]Token, error) {
	var rows []models.APIToken
	if err := s.db.WithContext(ctx).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取令牌失败: %w", err)
	}
	out := make([]Token, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

// Revoke 撤销令牌（删掉，立即生效）。
func (s *Store) Revoke(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).Delete(&models.APIToken{}, id)
	if res.Error != nil {
		return fmt.Errorf("撤销令牌失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// parse 拆开 `ptt_<id>_<secret>`。secret 是 base64url，自己也可能带下划线，所以只按 id 后面第一个下划线拆。
func parse(plain string) (uint, string, bool) {
	rest, ok := strings.CutPrefix(plain, prefix)
	if !ok {
		return 0, "", false
	}
	idPart, secret, ok := strings.Cut(rest, "_")
	if !ok || idPart == "" || secret == "" || len(secret) > maxSecretLen {
		return 0, "", false
	}
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil || id == 0 {
		return 0, "", false
	}
	return uint(id), secret, true
}

// Verify 校验明文令牌，对的时候返回令牌（顺带记下最近一次使用，5 分钟内最多写一次库）。
func (s *Store) Verify(ctx context.Context, plain string) (Token, error) {
	id, secret, ok := parse(plain)
	if !ok {
		return Token{}, ErrUnauthorized
	}
	var row models.APIToken
	if err := s.db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		return Token{}, fmt.Errorf("读取令牌失败: %w", err)
	}
	if row.ID == 0 || subtle.ConstantTimeCompare([]byte(hashOf(secret)), []byte(row.SecretHash)) != 1 {
		return Token{}, ErrUnauthorized
	}
	now := s.now().UTC()
	if row.ExpiresAt != nil && !now.Before(*row.ExpiresAt) {
		return Token{}, ErrUnauthorized
	}
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= touchEvery {
		// 条件里再判断一次：几个请求同时到时只有一个写（时间都按 UTC 存，字符串比较就是时间先后）
		if err := s.db.WithContext(ctx).Model(&models.APIToken{}).
			Where("id = ? AND (last_used_at IS NULL OR last_used_at <= ?)", row.ID, now.Add(-touchEvery)).
			Update("last_used_at", now).Error; err == nil {
			row.LastUsedAt = &now
		}
	}
	return view(row), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
