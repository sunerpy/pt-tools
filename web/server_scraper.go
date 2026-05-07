package web

import (
	"context"
	"net/http"

	"github.com/sunerpy/pt-tools/global"
	scrapercore "github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	scraperstore "github.com/sunerpy/pt-tools/internal/scraper/store"
	scraperweb "github.com/sunerpy/pt-tools/internal/scraper/web"
)

// registerScraperRoutes 在 pt-tools 主 web server 上挂载 scraper 子系统路由。
// 嵌入模式特点：
//   - 复用 pt-tools 的 session auth（已登录用户自动获得 scraper 权限）
//   - 复用 global.GlobalDB（与主系统共享 SQLite 文件）
//   - 运行独立 store.Migrate 建表，不污染 pt-tools schema_versions
//   - 构造 PersistentQueue 并启动 worker pool（默认 3 worker），
//     解决 queue/taskBuilder 循环依赖：先 NewScrapeService，再 NewPersistentQueue
//     with service.TaskBuilder()，最后 service.SetQueue() 回填
//   - 失败不致命：任一步骤失败只记录 warning，pt-tools 主功能继续运行
func (s *Server) registerScraperRoutes(mux *http.ServeMux) {
	log := global.GetSlogger()
	db := global.GlobalDB
	if db == nil || db.DB == nil {
		if log != nil {
			log.Warn("scraper: global DB 未初始化，跳过 scraper 路由注册")
		}
		return
	}
	if err := scraperstore.Migrate(db.DB); err != nil {
		if log != nil {
			log.Warnw("scraper: 迁移失败，跳过路由注册", "err", err)
		}
		return
	}

	sourceReg := scrapercore.NewRegistry[scrapercore.MediaScraper]()
	writerReg := scrapercore.NewRegistry[scrapercore.NfoWriter]()
	connectorReg := scrapercore.NewRegistry[scrapercore.MediaServerConnector]()

	librarySvc, err := service.NewLibraryService(service.LibraryConfig{DB: db.DB})
	if err != nil {
		if log != nil {
			log.Warnw("scraper: LibraryService 构造失败", "err", err)
		}
		return
	}
	scrapeSvc, err := service.NewScrapeService(service.ServiceConfig{
		DB:           db.DB,
		SourceReg:    sourceReg,
		WriterReg:    writerReg,
		ConnectorReg: connectorReg,
		Fuser:        service.NewDefaultFuser(),
	})
	if err != nil {
		if log != nil {
			log.Warnw("scraper: ScrapeService 构造失败", "err", err)
		}
		return
	}

	queue, err := service.NewPersistentQueue(service.PersistentConfig{
		DB:          db.DB,
		TaskBuilder: scrapeSvc.TaskBuilder(),
	})
	if err != nil {
		if log != nil {
			log.Warnw("scraper: PersistentQueue 构造失败", "err", err)
		}
		return
	}
	if err := queue.Start(context.Background(), 3); err != nil {
		if log != nil {
			log.Warnw("scraper: queue start 失败", "err", err)
		}
		return
	}
	scrapeSvc.SetQueue(queue)

	api, newAPIErr := scraperweb.NewAPI(scraperweb.APIConfig{
		Scrape:       scrapeSvc,
		Library:      librarySvc,
		DB:           db.DB,
		SourceReg:    sourceReg,
		ConnectorReg: connectorReg,
	})
	if newAPIErr != nil {
		if log != nil {
			log.Warnw("scraper: NewAPI 失败", "err", newAPIErr)
		}
		return
	}

	authAdapter := func(next http.HandlerFunc) http.HandlerFunc {
		return s.auth(next)
	}
	api.RegisterRoutes(mux, authAdapter)

	if log != nil {
		log.Info("scraper: 子系统已挂载到 /api/v2/scraper/*（queue workers=3）")
	}
}
