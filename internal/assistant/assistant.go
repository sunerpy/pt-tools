// Package assistant 是 pt-tools 内置的 AI 助手（路线图 M19）：经 OpenAI 兼容的 Chat Completions 接口（官方 openai-go）
// 调大模型，工具是每次运行在进程内连上的 MCP 服务（internal/mcp 的同一套工具，经 App API v1 执行）。
//
// Service 是运行、待确认操作与保留期清理的唯一所有者：
//   - 每个会话同时只有一次运行，运行在后台；进度经事件流（递增序号）交给网页（SSE）、App（长轮询）与 ChatOps；
//   - 写工具不让模型自己确认：给模型的参数里没有 confirm，每个写操作建一条待确认记录，结论只经数据库条件更新得出，
//     批准了才带 confirm=true 调用一次；
//   - 落库与下发的只有脱敏以后的内容，模型给的原始参数只在运行的内存里。
package assistant

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/mcp"
	"github.com/sunerpy/pt-tools/models"
)

// 对外的错误（web 按它们选状态码）。
var (
	ErrDisabled  = errors.New("AI 助手没有启用或没有配置好")
	ErrNotFound  = errors.New("会话或操作不存在")
	ErrBusy      = errors.New("这个会话正在回答：等它结束或先停止")
	ErrTooMany   = errors.New("同时进行的回答太多，稍后再试")
	ErrInvalid   = errors.New("参数不对")
	ErrForbidden = errors.New("没有确认改动类操作的权限")
	ErrDecided   = errors.New("这个操作已经处理过了")
	ErrClosed    = errors.New("AI 助手正在关闭")
)

// 停止运行的原因（context 的 cause）。
var (
	errCanceled = errors.New("用户停止了回答")
	errShutdown = errors.New("pt-tools 正在关闭")
)

// Actor 是用助手的一方。Owner 是带命名空间的属主：web:<用户名>、token:<令牌编号>、device:<设备编号>、chatops:<绑定编号>；
// 会话一律按属主过滤，别人的会话当作不存在。
type Actor struct {
	Owner string
	// Entry 是入口：models.AssistantEntryWeb、AssistantEntryApp、AssistantEntryChatOps
	Entry string
	// Name 是显示名（记进审计的参数）
	Name string
	// CanWrite 为假时模型拿不到写工具，也不能确认写操作
	CanWrite bool
	// CanConfig 为真时模型拿得到配置工具（get_config；能写时还有 change_config）：网页登录、管理员绑定、有 app:config 的令牌
	CanConfig bool
	// Notify 不为空时，运行把消息、待确认、确认结论与结束交给它（ChatOps 经它异步回复）；文字增量不给。
	// 关机取消的运行不再调用它。
	Notify func(ctx context.Context, ev Event)
}

func (a Actor) valid() bool {
	return a.Owner != "" && len(a.Owner) <= 64 && strings.Contains(a.Owner, ":")
}

// Cipher 加解密 API Key 与代理地址（生产环境是 ConfigStore 的 EncryptCookie / DecryptCookie）。
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(enc string) (string, error)
}

// ToolServer 建一次运行用的 MCP 服务（web.Server.AssistantToolServer）：调用方固定成 c，审计通道是 assistant。
type ToolServer func(c mcp.Caller) *sdk.Server

// Config 是服务的依赖。
type Config struct {
	DB     *gorm.DB
	Cipher Cipher
	Tools  ToolServer
	// ResolveURL 把种子下载地址解析成站点与种子编号（v2.TrackerResolver.ResolveDownloadURL）：用户贴的地址与 push_torrent
	// 的 torrent_url 在交给模型、建待确认以前先换掉，passkey 不出 pt-tools
	ResolveURL func(rawURL string) (site, torrentID string, ok bool)
	// Version 是 pt-tools 的版本（MCP 客户端的实现信息）
	Version string
	// Now 为空时用 time.Now（测试里换掉）
	Now func() time.Time
	// HTTPClient 不为空时用它调模型（测试用；设置里的代理不再生效）
	HTTPClient *http.Client
	// VoiceHTTPClient 不为空时调 Voltip 用它（测试里接假服务）；为空时用不走代理的客户端
	VoiceHTTPClient *http.Client
}

// Service 是 AI 助手服务。
type Service struct {
	cfg Config
	hub *hub

	settingsMu sync.Mutex

	// models 是对话页的模型列表缓存（ChatModels）
	modelsMu sync.Mutex
	models   modelCache
	// modelsLoading 是后台正在读模型列表（refreshModelsSoon）
	modelsLoading bool
	// toolsTokens 是上次连工具时工具定义估算的 token（算上下文用量用，见 toolsOverhead）
	toolsTokens atomic.Int64
	// noUsage 是不认 stream_options 的接口地址（这个进程里试过，见 callModel）
	noUsageMu sync.Mutex
	noUsage   map[string]bool

	mu      sync.Mutex
	runs    map[uint]*run
	purging map[uint]bool
	closing bool
	wg      sync.WaitGroup

	startOnce sync.Once
	stopOnce  sync.Once
	stop      chan struct{}

	// confirmUnit 是确认等待的单位（一分钟；测试里调小）
	confirmUnit time.Duration
	// firstClean 是启动以后第一次清理的延迟（一分钟；测试里调小）
	firstClean time.Duration
	// newCaptchaText 是「测试看图」画的验证码内容（为空时随机；测试里固定）
	newCaptchaText func() string
}

// 运行的上限。
const (
	// maxRuns 是全部会话同时进行的运行数上限
	maxRuns = 16
	// maxUserRunes 是一条用户消息的字数上限
	maxUserRunes = 4000
	// maxTitleRunes 是会话标题的字数上限（取第一条消息的开头）
	maxTitleRunes = 40
)

// New 建服务。Start 以后才清理保留期外的会话。
func New(cfg Config) *Service {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		cfg: cfg, hub: newHub(cfg.Now()),
		runs: map[uint]*run{}, purging: map[uint]bool{}, stop: make(chan struct{}), confirmUnit: time.Minute,
		firstClean: time.Minute,
	}
}

func (s *Service) now() time.Time { return s.cfg.Now() }

func (s *Service) db(ctx context.Context) *gorm.DB { return s.cfg.DB.WithContext(ctx) }

// Start 把上次留下的待确认标成过期（运行不跨重启），再起每天一次的保留期清理。
func (s *Service) Start(ctx context.Context) error {
	if err := s.expirePending(ctx, "restart"); err != nil {
		return err
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.loop()
	})
	return nil
}

// expirePending 把全部待确认标成过期。
func (s *Service) expirePending(ctx context.Context, by string) error {
	now := s.now()
	err := s.db(ctx).Model(&models.AssistantConfirmation{}).Where("status = ?", models.AssistantConfirmPending).
		Updates(map[string]any{"status": models.AssistantConfirmExpired, "decided_by": by, "decided_at": &now}).Error
	if err != nil {
		return fmt.Errorf("标记过期的待确认失败: %w", err)
	}
	return nil
}

// Close 停收新的运行，取消进行中的运行并在 ctx 的时限内等它们退出（含关掉 MCP 会话），最后把待确认标成过期。
func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	runs := make([]*run, 0, len(s.runs))
	for _, r := range s.runs {
		runs = append(runs, r)
	}
	s.mu.Unlock()
	for _, r := range runs {
		r.cancel(errShutdown)
	}
	s.stopOnce.Do(func() { close(s.stop) })
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	var err error
	select {
	case <-done:
	case <-ctx.Done():
		// 时限到的同一刻可能刚好都退出了（没有运行时 Wait 立即返回）：再给一点点时间
		select {
		case <-done:
		case <-time.After(20 * time.Millisecond):
			err = fmt.Errorf("等 AI 助手的运行退出超时: %w", ctx.Err())
		}
	}
	// 最后标过期：调用方的时限还没到就用它；已经到了再给一秒（不让关机的单步时限失效）
	ectx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ectx, cancel = context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
	}
	if e := s.expirePending(ectx, "shutdown"); e != nil && err == nil {
		err = e
	}
	return err
}

// loop 是后台任务：每天清理一次保留期外的会话，每 10 分钟丢掉闲着的事件缓冲。
func (s *Service) loop() {
	defer s.wg.Done()
	sweep := time.NewTicker(10 * time.Minute)
	defer sweep.Stop()
	clean := time.NewTimer(s.firstClean)
	defer clean.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-sweep.C:
			s.hub.sweep(s.now(), s.isRunning)
		case <-clean.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if n, err := s.Cleanup(ctx); err != nil {
				global.GetSlogger().Warnf("[AI 助手] 清理过期会话失败: %v", err)
			} else if n > 0 {
				global.GetSlogger().Infof("[AI 助手] 清理了 %d 个过期会话", n)
			}
			cancel()
			clean.Reset(24 * time.Hour)
		}
	}
}

// isRunning 报告会话有没有进行中的运行。
func (s *Service) isRunning(conv uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[conv] != nil
}

// ownerBusy 报告属主有没有进行中的运行（不管哪个会话）。
func (s *Service) ownerBusy(owner string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.actor.Owner == owner {
			return true
		}
	}
	return false
}

// activeRun 是会话进行中的运行（没有时为 nil）。
func (s *Service) activeRun(conv uint) *run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[conv]
}
