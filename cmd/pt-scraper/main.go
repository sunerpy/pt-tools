// Package main implements the standalone pt-scraper binary.
//
// 独立部署入口：零 pt-tools 全局依赖，仅引入 internal/scraper/* 子系统。
// 支持三种运行模式：
//   - http       HTTP REST API + 前端 Web UI（默认）
//   - mcp-stdio  MCP stdio 传输（给 Claude Desktop 等本地 LLM client）
//   - mcp-http   MCP Streamable HTTP
//   - both       同时 HTTP REST + MCP HTTP
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/glebarez/sqlite"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
	"github.com/sunerpy/pt-tools/internal/scraper/mcp"
	"github.com/sunerpy/pt-tools/internal/scraper/service"
	"github.com/sunerpy/pt-tools/internal/scraper/store"
	"github.com/sunerpy/pt-tools/internal/scraper/web"
)

const version = "v0.29.0-dev"

func main() {
	var (
		mode     = flag.String("mode", "http", "运行模式: http | mcp-stdio | mcp-http | both")
		addr     = flag.String("addr", ":8090", "HTTP REST API 监听地址")
		mcpAddr  = flag.String("mcp-addr", ":8091", "MCP HTTP 监听地址")
		dataDir  = flag.String("data-dir", defaultDataDir(), "数据目录")
		apiKey   = flag.String("api-key", "", "API Key（空则自动生成）")
		showVer  = flag.Bool("version", false, "显示版本")
		logLevel = flag.String("log-level", "info", "日志级别")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("pt-scraper %s\n", version)
		return
	}

	// stdio 模式下所有日志必须走 stderr（stdin/stdout 被 MCP 协议占用）
	log.SetOutput(os.Stderr)
	log.SetPrefix("[pt-scraper] ")
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	_ = logLevel

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}

	dbPath := filepath.Join(*dataDir, "scraper.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	if migrateErr := store.Migrate(db); migrateErr != nil {
		log.Fatalf("迁移失败: %v", migrateErr)
	}
	log.Printf("数据库就绪: %s", dbPath)

	sourceReg := core.NewRegistry[core.MediaScraper]()
	writerReg := core.NewRegistry[core.NfoWriter]()
	connectorReg := core.NewRegistry[core.MediaServerConnector]()

	fuser := service.NewDefaultFuser()
	librarySvc, err := service.NewLibraryService(service.LibraryConfig{DB: db})
	if err != nil {
		log.Fatalf("创建 LibraryService: %v", err)
	}
	scrapeSvc, err := service.NewScrapeService(service.ServiceConfig{
		DB:           db,
		SourceReg:    sourceReg,
		WriterReg:    writerReg,
		ConnectorReg: connectorReg,
		Fuser:        fuser,
	})
	if err != nil {
		log.Fatalf("创建 ScrapeService: %v", err)
	}

	resolvedKey := *apiKey
	if resolvedKey == "" {
		generated, err := web.GenerateAPIKey()
		if err != nil {
			log.Fatalf("生成 API Key 失败: %v", err)
		}
		resolvedKey = generated
		log.Printf("生成随机 API Key: %s（请保存，后续请求需要在 X-API-Key header 中提供）", resolvedKey)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("收到中断信号，退出中...")
		cancel()
	}()

	var wg sync.WaitGroup

	switch *mode {
	case "http":
		runHTTPServer(ctx, &wg, *addr, resolvedKey, scrapeSvc, librarySvc, db, sourceReg, connectorReg)
	case "mcp-stdio":
		runMCPStdio(ctx, scrapeSvc, librarySvc)
	case "mcp-http":
		runMCPHTTP(ctx, &wg, *mcpAddr, scrapeSvc, librarySvc)
	case "both":
		wg.Add(1)
		go func() {
			defer wg.Done()
			runMCPHTTP(ctx, &wg, *mcpAddr, scrapeSvc, librarySvc)
		}()
		runHTTPServer(ctx, &wg, *addr, resolvedKey, scrapeSvc, librarySvc, db, sourceReg, connectorReg)
	default:
		log.Fatalf("未知模式: %s", *mode)
	}

	wg.Wait()
	log.Println("已退出")
}

func runHTTPServer(ctx context.Context, wg *sync.WaitGroup, addr, apiKey string,
	scrapeSvc *service.ScrapeService, librarySvc *service.LibraryService,
	db *gorm.DB, sourceReg *core.Registry[core.MediaScraper],
	connectorReg *core.Registry[core.MediaServerConnector],
) {
	api, err := web.NewAPI(web.APIConfig{
		Scrape:       scrapeSvc,
		Library:      librarySvc,
		DB:           db,
		SourceReg:    sourceReg,
		ConnectorReg: connectorReg,
	})
	if err != nil {
		log.Fatalf("创建 API: %v", err)
	}

	mux := http.NewServeMux()
	authMw := web.APIKeyMiddleware(apiKey)
	api.RegisterRoutes(mux, authMw)

	// 健康检查（无需 auth）- 供 K8s readiness probe 等外部探针使用
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("HTTP REST API 监听: %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server: %v", err)
	}
}

func buildMCPServer(scrapeSvc *service.ScrapeService, librarySvc *service.LibraryService) *mcpsdk.Server {
	return mcp.NewMCPServer(mcp.Deps{
		Scrape:  scrapeSvc,
		Library: librarySvc,
	})
}

func runMCPStdio(ctx context.Context, scrapeSvc *service.ScrapeService, librarySvc *service.LibraryService) {
	server := buildMCPServer(scrapeSvc, librarySvc)
	log.Println("MCP stdio server 启动（通过 stdin/stdout 通信）")
	if err := server.Run(ctx, &mcpsdk.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("MCP stdio: %v", err)
	}
}

func runMCPHTTP(ctx context.Context, _ *sync.WaitGroup, addr string,
	scrapeSvc *service.ScrapeService, librarySvc *service.LibraryService,
) {
	server := buildMCPServer(scrapeSvc, librarySvc)
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server {
		return server
	}, nil)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("MCP Streamable HTTP server 监听: %s", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
		log.Fatalf("MCP HTTP: %v", err)
	}
}

func defaultDataDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".pt-scraper")
	}
	return "./pt-scraper-data"
}
