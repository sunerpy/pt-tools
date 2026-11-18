package cmd

import (
	"context"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web"
	"github.com/sunerpy/pt-tools/web/servarr"
)

// wireServarr 建 Servarr 兼容入口（媒体中心 M10），交给 Web 服务挂在 /servarr/ 下：Seerr 经它把 pt-tools 当成 Radarr、Sonarr 用，
// 请求变成电影订阅与追剧。订阅服务或令牌库没有起来时不建（/servarr/ 回 404）。
func wireServarr(db *gorm.DB, srv *web.Server, subscriber *subscribe.Service, organizer *organize.Service, media *recognize.Service, tokens *apitoken.Store, audit app.AuditService) *servarr.Server {
	if db == nil || srv == nil || subscriber == nil || tokens == nil {
		global.GetSlogger().Warn("Servarr 兼容入口跳过初始化：数据库、订阅或令牌服务未就绪")
		return nil
	}
	d := servarr.Deps{DB: db, Tokens: tokens, Audit: audit, Subs: subscriber}
	if organizer != nil {
		d.Libraries = func(ctx context.Context) ([]models.MediaLibrary, error) {
			views, err := organizer.Libraries(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]models.MediaLibrary, 0, len(views))
			for _, v := range views {
				out = append(out, v.MediaLibrary)
			}
			return out, nil
		}
	}
	if media != nil {
		d.TMDB = media.TMDB
	}
	sv := servarr.New(d)
	srv.SetServarr(sv.Handler())
	return sv
}
