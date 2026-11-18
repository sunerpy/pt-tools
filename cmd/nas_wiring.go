package cmd

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/nas"
	"github.com/sunerpy/pt-tools/models"
)

// nasCipher 是来源密钥的加解密（生产里是 storeCipher，与站点 Cookie 同一把密钥）。
type nasCipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(text string) (string, error)
}

// nasBrokenSecret 是库里的密钥解不开时这个来源显示的原因（加密密钥换了或丢了）。
const nasBrokenSecret = "保存的密钥解不开（加密密钥换了？），请在「NAS 监控」里重新填写"

// newNASCollector 建「NAS 监控」的采集器（首页 NAS 概览 H2）：从库里读设置与来源、解密密钥交给采集器；来源刷新出的
// 长期凭据只在这一行的 revision 没变时写回。由 runWeb 启动、交给 web.Server，并放进关闭流程。
func newNASCollector(db *gorm.DB, store *core.ConfigStore) *nas.Collector {
	return newNASCollectorWith(db, storeCipher{store: store})
}

func newNASCollectorWith(db *gorm.DB, cipher nasCipher) *nas.Collector {
	return &nas.Collector{
		Load: func(ctx context.Context) (nas.Config, error) { return loadNASConfig(ctx, db, cipher) },
		SaveExtra: func(ctx context.Context, id uint, revision int64, extra map[string]string) error {
			return saveNASExtra(ctx, db, cipher, id, revision, extra)
		},
		Logf: func(format string, args ...any) { global.GetSlogger().Warnf(format, args...) },
	}
}

// loadNASConfig 读本机来源的开关与全部来源（含停用的），解密密钥与刷新出的凭据。
// 密钥解不开的来源照样列出，只是标成用不了；刷新出的凭据解不开时丢掉（下次登录重新取）。
func loadNASConfig(ctx context.Context, db *gorm.DB, cipher nasCipher) (nas.Config, error) {
	if db == nil {
		return nas.Config{}, fmt.Errorf("数据库没有初始化")
	}
	db = db.WithContext(ctx)
	set, err := models.GetNasSetting(db)
	if err != nil {
		return nas.Config{}, fmt.Errorf("读取 NAS 监控设置失败: %w", err)
	}
	var rows []models.NasSource
	if err = db.Order("id").Find(&rows).Error; err != nil {
		return nas.Config{}, fmt.Errorf("读取 NAS 监控来源失败: %w", err)
	}
	cfg := nas.Config{LocalEnabled: set.LocalEnabled, Sources: make([]nas.SourceConfig, 0, len(rows))}
	for _, r := range rows {
		sc := nas.SourceConfig{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Enabled: r.Enabled, URL: r.URL, Username: r.Username,
			PinSHA256: r.TLSPinSHA256, InsecureAck: r.InsecureAck, Revision: r.Revision,
		}
		if r.SecretEncrypted != "" {
			if sc.Secret, err = cipher.Decrypt(r.SecretEncrypted); err != nil {
				sc.Secret, sc.Broken = "", nasBrokenSecret
			}
		}
		if r.ExtraEncrypted != "" {
			if extra, xerr := decodeNASExtra(cipher, r.ExtraEncrypted); xerr == nil {
				sc.Extra = extra
			} else {
				global.GetSlogger().Warnf("NAS 来源 %s 保存的登录凭据解不开，丢掉重新登录: %v", r.Name, xerr)
			}
		}
		cfg.Sources = append(cfg.Sources, sc)
	}
	return cfg, nil
}

// decodeNASExtra 解开刷新出的长期凭据（加密的 JSON，格式见 nas.EncodeExtra）。
func decodeNASExtra(cipher nasCipher, text string) (map[string]string, error) {
	plain, err := cipher.Decrypt(text)
	if err != nil {
		return nil, err
	}
	return nas.DecodeExtra(plain)
}

// encodeNASExtra 把长期凭据加密成 extra_encrypted 存的样子（空的存成空串）。
func encodeNASExtra(cipher nasCipher, extra map[string]string) (string, error) {
	plain, err := nas.EncodeExtra(extra)
	if err != nil || plain == "" {
		return "", err
	}
	return cipher.Encrypt(plain)
}

// saveNASExtra 写回来源刷新出的长期凭据：只在库里这一行的 revision 还是 revision 时写（用户刚改了设置时不盖掉），
// 写回不改 revision。
func saveNASExtra(ctx context.Context, db *gorm.DB, cipher nasCipher, id uint, revision int64, extra map[string]string) error {
	enc, err := encodeNASExtra(cipher, extra)
	if err != nil {
		return fmt.Errorf("加密登录凭据失败: %w", err)
	}
	return db.WithContext(ctx).Model(&models.NasSource{}).
		Where("id = ? AND revision = ?", id, revision).
		Update("extra_encrypted", enc).Error
}
