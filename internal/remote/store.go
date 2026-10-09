package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

const (
	// maxDevices 是没撤销的设备数上限
	maxDevices = 50
	// maxDeviceName 是设备名的字符数上限
	maxDeviceName = 64
	// defaultDeviceName 是配对时没给名字的设备
	defaultDeviceName = "未命名设备"
)

// 设备的两种权限：完全控制（默认）与只读。
var (
	ScopesFull = []string{apitoken.ScopeAppRead, apitoken.ScopeAppWrite}
	ScopesRead = []string{apitoken.ScopeAppRead}
)

var (
	// ErrInvalid 是参数不对
	ErrInvalid = errors.New("参数不对")
	// ErrDeviceNotFound 是没有这台设备
	ErrDeviceNotFound = errors.New("没有这台设备")
	// ErrDeviceRevoked 是设备已经撤销，不能再改
	ErrDeviceRevoked = errors.New("这台设备已经撤销")
	// ErrDeviceActive 是设备还没撤销，不能删记录
	ErrDeviceActive = errors.New("先撤销这台设备，才能删掉记录")
	// ErrNoCipher 是没有可用的加密密钥，主机密钥没法落库
	ErrNoCipher = errors.New("没有可用的加密密钥，不能保存主机密钥")
)

// Cipher 加解密落库的主机密钥（生产用 ConfigStore 的 AES 密钥，和站点 Cookie 同一把）。
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(text string) (string, error)
}

// Settings 是远程访问的设置。
type Settings struct {
	Enabled   bool     `json:"enabled"`
	Relays    []string `json:"relays"`
	DirectURL string   `json:"direct_url"`
}

// Device 是配对过的设备（不含公钥）。
type Device struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	LastSeenVia string     `json:"last_seen_via,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// Has 报告设备有没有这个权限范围。
func (d Device) Has(scope string) bool { return slices.Contains(d.Scopes, scope) }

// Store 读写远程访问的设置、主机密钥与设备表。
type Store struct {
	db     *gorm.DB
	cipher Cipher
	now    func() time.Time
}

// NewStore 用 db 与 cipher 建 Store。cipher 为 nil 时不能生成主机密钥（远程访问开不了）。
func NewStore(db *gorm.DB, cipher Cipher) *Store {
	return &Store{db: db, cipher: cipher, now: time.Now}
}

// writeDB 是写库用的 DB。用调用方给的 ctx：Host 给的是不随请求取消、但有上限的 ctx（boundedContext），
// 这里不再去掉它的取消与期限，免得一次卡住的写永远不返回。
func (s *Store) writeDB(ctx context.Context) *gorm.DB { return s.db.WithContext(ctx) }

func (s *Store) row(ctx context.Context) (models.RemoteSetting, error) {
	var r models.RemoteSetting
	if err := s.db.WithContext(ctx).Where("id = 1").Limit(1).Find(&r).Error; err != nil {
		return r, fmt.Errorf("读取远程访问设置失败: %w", err)
	}
	if r.ID == 0 {
		return models.RemoteSetting{ID: 1}, nil
	}
	return r, nil
}

func ensureRow(db *gorm.DB) error {
	var r models.RemoteSetting
	return db.FirstOrCreate(&r, models.RemoteSetting{ID: 1}).Error
}

// Settings 返回当前设置。
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	r, err := s.row(ctx)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{Enabled: r.Enabled, DirectURL: r.DirectURL, Relays: []string{}}
	if r.RelaysJSON != "" {
		if err := json.Unmarshal([]byte(r.RelaysJSON), &out.Relays); err != nil {
			return Settings{}, fmt.Errorf("relay 列表格式不对: %w", err)
		}
	}
	return out, nil
}

// NormalizeSettings 检查并规整设置：relay 地址去重、最多 4 个；直连地址可以为空。
func NormalizeSettings(in Settings) (Settings, error) {
	out := Settings{Enabled: in.Enabled, Relays: []string{}}
	for _, raw := range in.Relays {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		u, err := NormalizeRelayURL(raw)
		if err != nil {
			return Settings{}, fmt.Errorf("%w：%v", ErrInvalid, err)
		}
		if !slices.Contains(out.Relays, u) {
			out.Relays = append(out.Relays, u)
		}
	}
	if len(out.Relays) > MaxRelays {
		return Settings{}, fmt.Errorf("%w：relay 最多 %d 个", ErrInvalid, MaxRelays)
	}
	if strings.TrimSpace(in.DirectURL) != "" {
		u, err := NormalizeDirectURL(in.DirectURL)
		if err != nil {
			return Settings{}, fmt.Errorf("%w：%v", ErrInvalid, err)
		}
		out.DirectURL = u
	}
	return out, nil
}

// SaveSettings 校验并保存设置，返回规整以后写进去的设置（不读回来：写成功以后不能因为读失败让调用方以为没写）。
func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	in, err := NormalizeSettings(in)
	if err != nil {
		return Settings{}, err
	}
	relays, err := json.Marshal(in.Relays)
	if err != nil {
		return Settings{}, err
	}
	db := s.writeDB(ctx)
	if err := ensureRow(db); err != nil {
		return Settings{}, fmt.Errorf("保存远程访问设置失败: %w", err)
	}
	cols := map[string]any{"enabled": in.Enabled, "relays_json": string(relays), "direct_url": in.DirectURL}
	if err := db.Model(&models.RemoteSetting{}).Where("id = 1").Updates(cols).Error; err != nil {
		return Settings{}, fmt.Errorf("保存远程访问设置失败: %w", err)
	}
	return in, nil
}

// HostKeys 读出主机密钥；还没有时 create 为真就生成一套存下来，否则返回 nil。
func (s *Store) HostKeys(ctx context.Context, create bool) (*HostKeys, error) {
	r, err := s.row(ctx)
	if err != nil {
		return nil, err
	}
	if r.HostKeysEncrypted != "" {
		return s.decryptKeys(r.HostKeysEncrypted)
	}
	if !create {
		return nil, nil
	}
	return s.saveNewKeys(ctx, func(*gorm.DB) error { return nil }, true)
}

// RotateHostKeys 换一套主机密钥，并撤销所有设备（它们记着旧的主机公钥，已经连不上了）。
func (s *Store) RotateHostKeys(ctx context.Context) (*HostKeys, error) {
	return s.saveNewKeys(ctx, func(tx *gorm.DB) error {
		return tx.Model(&models.RemoteDevice{}).Where("revoked_at IS NULL").Update("revoked_at", s.now()).Error
	}, false)
}

// saveNewKeys 生成新密钥并在一个事务里落库（连同 also）。onlyIfEmpty 为真时库里已经有密钥就用库里的（两个调用同时生成时不互相覆盖）。
func (s *Store) saveNewKeys(ctx context.Context, also func(tx *gorm.DB) error, onlyIfEmpty bool) (*HostKeys, error) {
	if s.cipher == nil {
		return nil, ErrNoCipher
	}
	keys, err := GenerateHostKeys(nil)
	if err != nil {
		return nil, err
	}
	plain, err := keys.Marshal()
	if err != nil {
		return nil, err
	}
	enc, err := s.cipher.Encrypt(string(plain))
	if err != nil {
		return nil, fmt.Errorf("加密主机密钥失败: %w", err)
	}
	var existing string
	err = s.writeDB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := ensureRow(tx); e != nil {
			return e
		}
		if onlyIfEmpty {
			var r models.RemoteSetting
			if e := tx.Where("id = 1").Limit(1).Find(&r).Error; e != nil {
				return e
			}
			if r.HostKeysEncrypted != "" {
				existing = r.HostKeysEncrypted
				return nil
			}
		}
		if e := tx.Model(&models.RemoteSetting{}).Where("id = 1").Update("host_keys_encrypted", enc).Error; e != nil {
			return e
		}
		return also(tx)
	})
	if err != nil {
		return nil, fmt.Errorf("保存主机密钥失败: %w", err)
	}
	if existing != "" {
		return s.decryptKeys(existing)
	}
	return keys, nil
}

func (s *Store) decryptKeys(enc string) (*HostKeys, error) {
	if s.cipher == nil {
		return nil, ErrNoCipher
	}
	plain, err := s.cipher.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("解密主机密钥失败: %w", err)
	}
	return ParseHostKeys([]byte(plain))
}

func deviceView(r models.RemoteDevice) Device {
	d := Device{
		ID: r.ID, Name: r.Name, Scopes: strings.Fields(r.Scopes), CreatedAt: r.CreatedAt,
		LastSeenAt: r.LastSeenAt, LastSeenVia: r.LastSeenVia, RevokedAt: r.RevokedAt,
	}
	if d.Scopes == nil {
		d.Scopes = []string{}
	}
	return d
}

// NormalizeScopes 检查设备的权限：只读（app:read）或完全控制（app:read app:write）。
func NormalizeScopes(in []string) ([]string, error) {
	read, write := false, false
	for _, sc := range in {
		switch sc {
		case apitoken.ScopeAppRead:
			read = true
		case apitoken.ScopeAppWrite:
			write = true
		default:
			return nil, fmt.Errorf("%w：设备的权限只能是 app:read 或 app:write，不认识 %q", ErrInvalid, sc)
		}
	}
	switch {
	case read && write:
		return slices.Clone(ScopesFull), nil
	case read:
		return slices.Clone(ScopesRead), nil
	}
	return nil, fmt.Errorf("%w：设备至少要有 app:read 权限", ErrInvalid)
}

// NormalizeDeviceName 去掉首尾空白，最多 64 个字符，不能有控制字符；空名字换成「未命名设备」。
func NormalizeDeviceName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return defaultDeviceName, nil
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxDeviceName || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w：设备名最多 %d 个字符，不能有控制字符", ErrInvalid, maxDeviceName)
	}
	return name, nil
}

// Devices 列出所有设备（没撤销的在前，各自按编号）。
func (s *Store) Devices(ctx context.Context) ([]Device, error) {
	var rows []models.RemoteDevice
	if err := s.db.WithContext(ctx).Order("revoked_at IS NOT NULL, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取设备失败: %w", err)
	}
	out := make([]Device, 0, len(rows))
	for _, r := range rows {
		out = append(out, deviceView(r))
	}
	return out, nil
}

// Device 读一台设备。
func (s *Store) Device(ctx context.Context, id uint) (Device, error) {
	var r models.RemoteDevice
	if err := s.db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&r).Error; err != nil {
		return Device{}, fmt.Errorf("读取设备失败: %w", err)
	}
	if r.ID == 0 {
		return Device{}, ErrDeviceNotFound
	}
	return deviceView(r), nil
}

// ActiveDeviceByKey 按公钥找没撤销的设备，没有时返回 nil。
func (s *Store) ActiveDeviceByKey(ctx context.Context, pub []byte) (*Device, error) {
	var r models.RemoteDevice
	if err := s.db.WithContext(ctx).Where("public_key = ? AND revoked_at IS NULL", EncodeKey(pub)).Limit(1).Find(&r).Error; err != nil {
		return nil, fmt.Errorf("读取设备失败: %w", err)
	}
	if r.ID == 0 {
		return nil, nil
	}
	d := deviceView(r)
	return &d, nil
}

// CreateDevice 记下配对成功的设备。同一把公钥已经有没撤销的设备时拒绝（那台设备握手时就是正常会话，走不到配对）。
func (s *Store) CreateDevice(ctx context.Context, name string, pub []byte, scopes []string) (Device, error) {
	name, err := NormalizeDeviceName(name)
	if err != nil {
		return Device{}, err
	}
	if scopes, err = NormalizeScopes(scopes); err != nil {
		return Device{}, err
	}
	if len(pub) != KeyLen {
		return Device{}, fmt.Errorf("%w：设备公钥长度不对", ErrInvalid)
	}
	key := EncodeKey(pub)
	var row models.RemoteDevice
	err = s.writeDB(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("public_key = ? AND revoked_at IS NULL", key).Limit(1).Find(&row).Error; e != nil {
			return e
		}
		if row.ID != 0 {
			return fmt.Errorf("%w：这台设备已经配对过", ErrInvalid)
		}
		var n int64
		if e := tx.Model(&models.RemoteDevice{}).Where("revoked_at IS NULL").Count(&n).Error; e != nil {
			return e
		}
		if n >= maxDevices {
			return fmt.Errorf("%w：最多 %d 台设备，先撤销不用的", ErrInvalid, maxDevices)
		}
		row = models.RemoteDevice{Name: name, PublicKey: key, Scopes: strings.Join(scopes, " ")}
		return tx.Create(&row).Error
	})
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return Device{}, err
		}
		return Device{}, fmt.Errorf("保存设备失败: %w", err)
	}
	return deviceView(row), nil
}

// UpdateDevice 改名字或权限（nil 表示不改）。返回改后的设备与权限有没有变；撤销了的设备不能改。
func (s *Store) UpdateDevice(ctx context.Context, id uint, name *string, scopes []string) (Device, bool, error) {
	cols := map[string]any{}
	if name != nil {
		n, err := NormalizeDeviceName(*name)
		if err != nil {
			return Device{}, false, err
		}
		cols["name"] = n
	}
	var newScopes string
	if scopes != nil {
		sc, err := NormalizeScopes(scopes)
		if err != nil {
			return Device{}, false, err
		}
		newScopes = strings.Join(sc, " ")
		cols["scopes"] = newScopes
	}
	before, err := s.Device(ctx, id)
	if err != nil {
		return Device{}, false, err
	}
	if before.RevokedAt != nil {
		return Device{}, false, ErrDeviceRevoked
	}
	if len(cols) > 0 {
		if err = s.writeDB(ctx).Model(&models.RemoteDevice{}).Where("id = ? AND revoked_at IS NULL", id).Updates(cols).Error; err != nil {
			return Device{}, false, fmt.Errorf("保存设备失败: %w", err)
		}
	}
	after, err := s.Device(ctx, id)
	if err != nil {
		return Device{}, false, err
	}
	changed := scopes != nil && newScopes != strings.Join(before.Scopes, " ")
	return after, changed, nil
}

// RevokeDevice 撤销设备（已经撤销的不变）。
func (s *Store) RevokeDevice(ctx context.Context, id uint) (Device, error) {
	if _, err := s.Device(ctx, id); err != nil {
		return Device{}, err
	}
	if err := s.writeDB(ctx).Model(&models.RemoteDevice{}).Where("id = ? AND revoked_at IS NULL", id).Update("revoked_at", s.now()).Error; err != nil {
		return Device{}, fmt.Errorf("撤销设备失败: %w", err)
	}
	return s.Device(ctx, id)
}

// DeleteDevice 删掉撤销了的设备的记录。
func (s *Store) DeleteDevice(ctx context.Context, id uint) error {
	d, err := s.Device(ctx, id)
	if err != nil {
		return err
	}
	if d.RevokedAt == nil {
		return ErrDeviceActive
	}
	if err := s.writeDB(ctx).Where("id = ? AND revoked_at IS NOT NULL", id).Delete(&models.RemoteDevice{}).Error; err != nil {
		return fmt.Errorf("删除设备失败: %w", err)
	}
	return nil
}

// TouchDevice 记下设备最近一次连上的时间与方式（direct 或 relay）。
func (s *Store) TouchDevice(ctx context.Context, id uint, via string, at time.Time) error {
	return s.writeDB(ctx).Model(&models.RemoteDevice{}).Where("id = ?", id).
		Updates(map[string]any{"last_seen_at": at, "last_seen_via": via}).Error
}
