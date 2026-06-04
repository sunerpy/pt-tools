package cmd

import (
	"context"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type loginReminderConfLister struct {
	db *gorm.DB
}

func (l loginReminderConfLister) ListNotificationConfs(ctx context.Context) ([]models.NotificationConf, error) {
	var confs []models.NotificationConf
	if err := l.db.WithContext(ctx).Where("enabled = ?", true).Find(&confs).Error; err != nil {
		return nil, err
	}
	out := make([]models.NotificationConf, 0, len(confs))
	for i := range confs {
		conf := confs[i]
		if conf.ConfigJSON != "" {
			plain, derr := crypto.Decrypt(conf.ConfigJSON)
			if derr != nil {
				global.GetSlogger().Warnf("登录提醒通道配置解密失败 conf_id=%d type=%s: %v", conf.ID, conf.ChannelType, derr)
				continue
			}
			conf.ConfigJSON = string(plain)
		}
		out = append(out, conf)
	}
	return out, nil
}

type loginReminderDecryptor struct {
	store *core.ConfigStore
}

func (d loginReminderDecryptor) Decrypt(setting models.SiteSetting) (string, error) {
	if setting.CookieEncrypted == "" {
		return "", nil
	}
	return d.store.DecryptCookie(setting.CookieEncrypted)
}

type loginReminderResolver struct {
	registry  *v2.SiteRegistry
	decryptor loginReminderDecryptor
}

func (r loginReminderResolver) Resolve(setting models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	cookie, err := r.decryptor.Decrypt(setting)
	if err != nil {
		return nil, nil, err
	}
	site, err := r.registry.CreateSite(
		setting.Name,
		v2.SiteCredentials{
			Cookie:  cookie,
			APIKey:  setting.APIKey,
			Passkey: setting.Passkey,
		},
		setting.APIUrl,
	)
	if err != nil {
		return nil, nil, err
	}
	def, _ := v2.GetDefinitionRegistry().Get(setting.Name)
	return def, site, nil
}

func wireLoginReminderMonitor(
	mgr *scheduler.Manager,
	store *core.ConfigStore,
	siteRegistry *v2.SiteRegistry,
	bs *chatopsBootstrap,
) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		global.GetSlogger().Warn("登录提醒监控器跳过初始化：数据库未就绪")
		return
	}
	db := global.GlobalDB.DB

	registry := notify.DefaultRegistry()
	if bs != nil && bs.registry != nil {
		registry = bs.registry
	}
	router := notify.NewRouter(registry, nil, loginReminderConfLister{db: db})

	decryptor := loginReminderDecryptor{store: store}
	resolver := loginReminderResolver{registry: siteRegistry, decryptor: decryptor}

	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:        db,
		Router:    router,
		Resolver:  resolver,
		Decryptor: decryptor,
		Logger:    global.GetSlogger(),
	})
	mgr.SetLoginReminderMonitor(mon)
	mon.Start()
	global.GetSlogger().Info("登录提醒监控器已初始化并启动")
}
