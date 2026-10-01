package downloader

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"sync"
	"time"
)

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	MaxRetries     int           // 最大重试次数
	InitialBackoff time.Duration // 初始退避时间
	MaxBackoff     time.Duration // 最大退避时间
	Multiplier     float64       // 退避时间乘数
}

// DefaultReconnectConfig 默认重连配置
var DefaultReconnectConfig = ReconnectConfig{
	MaxRetries:     5,
	InitialBackoff: time.Second,
	MaxBackoff:     30 * time.Second,
	Multiplier:     2.0,
}

// DownloaderStatus 下载器状态
type DownloaderStatus struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	IsHealthy   bool      `json:"is_healthy"`
	IsDefault   bool      `json:"is_default"`
	LastChecked time.Time `json:"last_checked"`
	ErrorCount  int       `json:"error_count"`
}

// DownloaderManager 下载器管理器
// 负责管理多个下载器实例，支持工厂注册和实例获取
//
// 并发约定：mu 只保护下面这几张 map，任何网络 I/O（工厂建连、Ping、退避睡眠）
// 都必须在 mu 之外进行。慢活由 gates 里的单飞闸门串行化，见 reviveOrCreate。
type DownloaderManager struct {
	mu              sync.RWMutex
	downloaders     map[string]Downloader                // key: downloader name
	factories       map[DownloaderType]DownloaderFactory // 下载器工厂
	configs         map[string]DownloaderConfig          // 下载器配置
	defaultName     string                               // 默认下载器名称
	siteDownloaders map[string]string                    // 站点到下载器的映射
	reconnectConfig ReconnectConfig                      // 重连配置
	errorCounts     map[string]int                       // 错误计数
	lastHealthCheck map[string]time.Time                 // 最后健康检查时间

	// configGen[name] 每次该下载器的配置发生变化（保存、禁用、删除、整表同步）就 +1；
	// instanceGen[name] 记录 downloaders[name] 这个实例是按哪一代配置建出来的。
	// 两者不相等就说明手上的实例已经过期，必须重建 —— 详见 instanceIsCurrent。
	configGen   map[string]uint64
	instanceGen map[string]uint64

	flightsMu sync.Mutex
	flights   map[string]*createFlight // key: downloader name，正在进行的创建
	cooldowns map[string]createFailure // key: downloader name，最近一次创建失败
}

// createFlight 是某个下载器「正在进行的一次创建」。
//
// 多个调用方共享同一次尝试，各自按自己的 ctx 预算等待。放弃等待的调用方不会
// 取消这次尝试：它照样跑完完整的退避策略并把结果登记进实例表，于是下一次请求
// 能直接命中快路径。这样 Web 请求可以按秒级预算返回，而重连的韧性不受影响。
type createFlight struct {
	done chan struct{}
	dl   Downloader
	err  error
}

// createFailure 记录一次失败的创建，用于失败冷却。
//
// gen 是这次失败所依据的配置代次。必须记下来并在**读取时**校验：
// 「检查代次」和「写入冷却」分别在 dm.mu 和 flightsMu 下完成，两者不可能原子完成
// （反过来加锁会和 reviveOrCreate 构成锁序反转）。于是存在这样一个交错 ——
// 旧 flight 失败时代次还是当前的，检查通过；紧接着用户保存新配置（换代 + 清冷却）；
// 旧 flight 最后才把错误写进冷却，正好落在清空之后。
// 读取时比对代次能彻底消掉这个窗口，不依赖任何写入顺序。
type createFailure struct {
	at  time.Time
	err error
	gen uint64
}

// maxSupersededRetries 是「配置变更导致本次建连作废」后重试的上限。
// 每次重试都基于更新后的配置，所以正常只会重试一次；留几次余量是防止用户
// 连续保存时调用方拿到一个纯属时序造成的错误。
const maxSupersededRetries = 3

// defaultFailureCooldown 创建失败后的冷却时长。
// 下载器确实连不上时，这段时间内的请求直接复用上次的错误，不再发起新的尝试 ——
// 否则前端 30s 一次的轮询会让一个死掉的下载器持续制造连接尝试和日志。
// 显式重连、保存配置、从 DB 同步都会清掉冷却，用户改完设置无需等待。
const defaultFailureCooldown = 60 * time.Second

// NewDownloaderManager 创建下载器管理器
func NewDownloaderManager() *DownloaderManager {
	return &DownloaderManager{
		downloaders:     make(map[string]Downloader),
		factories:       make(map[DownloaderType]DownloaderFactory),
		configs:         make(map[string]DownloaderConfig),
		siteDownloaders: make(map[string]string),
		reconnectConfig: DefaultReconnectConfig,
		errorCounts:     make(map[string]int),
		lastHealthCheck: make(map[string]time.Time),
		configGen:       make(map[string]uint64),
		instanceGen:     make(map[string]uint64),
		flights:         make(map[string]*createFlight),
		cooldowns:       make(map[string]createFailure),
	}
}

// NewDownloaderManagerWithConfig 创建带自定义重连配置的下载器管理器
func NewDownloaderManagerWithConfig(reconnectConfig ReconnectConfig) *DownloaderManager {
	return &DownloaderManager{
		downloaders:     make(map[string]Downloader),
		factories:       make(map[DownloaderType]DownloaderFactory),
		configs:         make(map[string]DownloaderConfig),
		siteDownloaders: make(map[string]string),
		reconnectConfig: reconnectConfig,
		errorCounts:     make(map[string]int),
		lastHealthCheck: make(map[string]time.Time),
		configGen:       make(map[string]uint64),
		instanceGen:     make(map[string]uint64),
		flights:         make(map[string]*createFlight),
		cooldowns:       make(map[string]createFailure),
	}
}

// RegisterFactory 注册下载器工厂
func (dm *DownloaderManager) RegisterFactory(dlType DownloaderType, factory DownloaderFactory) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.factories[dlType] = factory
	sLogger().Infof("Registered downloader factory for type: %s", dlType)
}

// RegisterConfig 注册下载器配置
func (dm *DownloaderManager) RegisterConfig(name string, config DownloaderConfig, isDefault bool) error {
	if err := dm.storeConfig(name, config, isDefault); err != nil {
		return err
	}
	// 配置刚被写入，之前那次失败不再代表当前配置：作废冷却，让下一次请求立刻重试
	dm.clearCooldown(name)
	sLogger().Infof("Registered downloader config: %s (default: %v)", name, isDefault)
	return nil
}

func (dm *DownloaderManager) storeConfig(name string, config DownloaderConfig, isDefault bool) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config for %s: %w", name, err)
	}

	// 只有配置真的变了（或本来还没有）才换代：换代等于作废已登记的实例，并让正在飞的
	// 那次建连的结果不再允许入表。而 scheduler 的配置 reload 会对每个启用的下载器
	// 无条件重新注册一遍，无差别换代会让保存任意配置（包括通知渠道这种毫不相关的）
	// 都逼出一轮全体重连。
	//
	// 判据与 applySyncFromDB 里的 configChanged 保持一致，只是两边都是 DownloaderConfig。
	if old, had := dm.configs[name]; !had || configValuesChanged(old, config) {
		dm.bumpConfigGenLocked(name)
	}
	dm.configs[name] = config
	if isDefault {
		dm.defaultName = name
	}
	return nil
}

// configValuesChanged 比较两份配置里影响连接的字段，决定是否需要换代。
//
// isDefault 刻意不参与：defaultName 是独立于连接的状态，把某个下载器设成默认
// 并不改变它连谁，没有理由把它已建好的连接作废。
func configValuesChanged(oldConfig, newConfig DownloaderConfig) bool {
	return oldConfig.GetType() != newConfig.GetType() ||
		oldConfig.GetURL() != newConfig.GetURL() ||
		oldConfig.GetUsername() != newConfig.GetUsername() ||
		oldConfig.GetPassword() != newConfig.GetPassword() ||
		oldConfig.GetAutoStart() != newConfig.GetAutoStart()
}

// SetSiteDownloader 设置站点使用的下载器
func (dm *DownloaderManager) SetSiteDownloader(siteName, downloaderName string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.siteDownloaders[siteName] = downloaderName
	sLogger().Infof("Set site %s to use downloader: %s", siteName, downloaderName)
}

// GetDownloader 获取或创建下载器实例。
//
// 等价于 GetDownloaderContext(context.Background(), name)：愿意为一个连不上的
// 下载器把完整退避序列跑到底。Web 请求这类有自己时间预算的调用方应改用
// GetDownloaderContext，否则会被拖进最长 31s 的退避里。
func (dm *DownloaderManager) GetDownloader(name string) (Downloader, error) {
	return dm.GetDownloaderContext(context.Background(), name)
}

// GetDownloaderContext 获取或创建下载器实例，创建与健康检查阶段受 ctx 约束。
//
// 快路径（实例已存在且自报健康）只拿读锁，IsHealthy 读的是本地标志、不做网络
// I/O，因此任意多个调用方可以并发通过，互不阻塞。
func (dm *DownloaderManager) GetDownloaderContext(ctx context.Context, name string) (Downloader, error) {
	if dl, exists := dm.lookupInstance(name); exists && dl.IsHealthy() {
		dm.markHealthy(name)
		return dl, nil
	}
	return dm.reviveOrCreate(ctx, name)
}

// reviveOrCreate 处理慢路径：Ping 确认、重建、首次创建。
//
// 这三件事都是网络 I/O，绝不能在 dm.mu 里做。之前 createWithRetry
// （最长 1+2+4+8+16=31s 退避，外加每次尝试的连接超时）跑在写锁里，一个连不上的
// 下载器会把整个 manager 串行化：实测每个排队的调用方要多等 61s 且无上界，
// 前端 30s 一次的全局轮询于是把浏览器的连接池占满，所有页面都加载不出数据。
//
// 现在每个下载器同时只有一次进行中的尝试（createFlight），所有调用方共享它，
// 各自按自己的 ctx 预算等待。等不到就按自己的预算返回，尝试继续在后台跑完 ——
// 因为 DownloaderFactory 和 Ping 都不接受 ctx，无法真正中断，只能不去等它。
// 不同下载器之间完全并行。
//
// 建连期间配置被改掉时，这一飞的结果会被丢弃（见 storeInstance），
// 这里按新配置重试，最多 maxSupersededRetries 次。
func (dm *DownloaderManager) reviveOrCreate(ctx context.Context, name string) (Downloader, error) {
	for attempt := 0; ; attempt++ {
		// 配置或工厂缺失是配置错误，不是连接失败：只读一次 map 就能判定，
		// 立即返回原始错误，既不进单飞也不进失败冷却。
		_, _, gen, err := dm.resolveFactory(name)
		if err != nil {
			return nil, err
		}
		if err := dm.cooldownError(name, gen); err != nil {
			return nil, err
		}

		flight, isLeader := dm.joinFlight(name)
		if isLeader {
			go dm.runFlight(name, flight)
		}

		select {
		case <-flight.done:
			// 这一飞是按旧配置跑的，结果已被丢弃；按新配置再来一次
			if errors.Is(flight.err, errConfigSuperseded) && attempt < maxSupersededRetries {
				continue
			}
			if flight.err != nil {
				return nil, flight.err
			}
			return flight.dl, nil
		case <-ctx.Done():
			return nil, fmt.Errorf("等待下载器 %s 就绪超时: %w", name, ctx.Err())
		}
	}
}

// joinFlight 加入（必要时开启）某个下载器的创建尝试。
// 第二个返回值为 true 表示调用方是本次尝试的发起者，需要把它跑起来。
func (dm *DownloaderManager) joinFlight(name string) (*createFlight, bool) {
	dm.flightsMu.Lock()
	defer dm.flightsMu.Unlock()
	if dm.flights == nil {
		dm.flights = make(map[string]*createFlight)
	}
	if flight, exists := dm.flights[name]; exists {
		return flight, false
	}
	flight := &createFlight{done: make(chan struct{})}
	dm.flights[name] = flight
	return flight, true
}

// runFlight 跑完一次创建尝试并广播结果。
// 用 context.Background 而不是发起者的 ctx：尝试的生命周期属于 manager，
// 不属于某一个 Web 请求 —— 请求可以放弃等待，重连不该因此半途而废。
//
// 这是 manager 自己起的 goroutine，工厂或 Ping 在这里 panic 时上面没有任何东西接得住
// （原先建连跑在 net/http 的请求 goroutine 里，panic 会被 http.Server 吞掉），
// 会直接带崩整个进程。所以 defer 里兜住：panic 转成错误，照常结算 flight 并广播 ——
// 否则 flight 记录永远摘不掉，等在 done 上的调用方只能干等到各自的预算耗尽。
func (dm *DownloaderManager) runFlight(name string, flight *createFlight) {
	// panic 时 doRevive 来不及交回它实际依据的代次，就用发起时读到的这一代记冷却。
	// 这一代只会比实际用的旧、不会更新：配置若在中途变了，这条冷却在读取侧按代次比对会被丢掉，
	// 挡不住新配置；配置没变，它就和普通的建连失败一样冷却，不会每次轮询都再 panic 一遍。
	_, _, startGen, _ := dm.resolveFactory(name)

	var (
		dl  Downloader
		gen uint64
		err error
	)
	defer func() {
		if r := recover(); r != nil {
			dl, gen = nil, startGen
			err = fmt.Errorf("下载器 %s 建连时发生 panic: %v", name, r)
			sLogger().Errorf("%v\n%s", err, debug.Stack())
		}

		flight.dl = dl
		flight.err = err

		dm.settleFlight(name, flight, gen, err)

		close(flight.done)
	}()

	dl, gen, err = dm.doRevive(name)
}

// settleFlight 收尾一次尝试：摘掉 flight 记录，并按结果记/清失败冷却。
//
// 单独成方法是为了能被测试直接调用，复现「旧 flight 在清冷却之后才提交」那个交错
// —— 靠 sleep 去碰这个窗口是碰不稳的。
func (dm *DownloaderManager) settleFlight(
	name string,
	flight *createFlight,
	gen uint64,
	err error,
) {
	dm.flightsMu.Lock()
	defer dm.flightsMu.Unlock()

	if flight != nil {
		if cur, exists := dm.flights[name]; exists && cur == flight {
			delete(dm.flights, name)
		}
	}
	switch {
	case errors.Is(err, errConfigSuperseded):
		// 配置换了，这次失败说明不了新配置能不能连上：既不记冷却，也不清冷却
	case err != nil:
		dm.cooldowns[name] = createFailure{at: time.Now(), err: err, gen: gen}
	default:
		delete(dm.cooldowns, name)
	}
}

// errConfigSuperseded 表示这次建连跑完时，它依据的配置已经被改掉了。
// 不是连接失败，所以不进失败冷却；reviveOrCreate 会按新配置重试一次。
var errConfigSuperseded = errors.New("下载器配置在建连期间已变更")

// doRevive 是一次创建尝试的实际内容：Ping 确认已有实例，否则重建。
// 全程不持有 dm.mu，只在读写那几张 map 时短暂加锁。
// 第二个返回值是本次尝试所依据的配置代次，供 settleFlight 记进失败冷却。
// 复用已有实例的那两条早返回没有建连、也就没有代次，返回 0。
func (dm *DownloaderManager) doRevive(name string) (Downloader, uint64, error) {
	if dl, exists := dm.lookupInstance(name); exists {
		if dl.IsHealthy() {
			dm.markHealthy(name)
			return dl, 0, nil
		}
		ok, pingErr := dl.Ping()
		if ok {
			dm.markHealthy(name)
			return dl, 0, nil
		}
		sLogger().Warnf("Downloader %s is unhealthy (ping failed: %v), recreating...", name, pingErr)
		dm.dropInstance(name, dl)
	}

	// reviveOrCreate 已经确认过配置存在，这里重新读一次是为了拿到值本身和它的代次
	config, factory, gen, err := dm.resolveFactory(name)
	if err != nil {
		return nil, 0, err
	}

	dl, err := dm.createWithRetry(name, config, factory)
	if err != nil {
		// 失败也要过代次检查。这次失败是对着旧配置得出的，说明不了新配置行不行；
		// 若原样返回，settleFlight 会把它写进 60s 冷却，于是用户刚改好的配置在一分钟内
		// 每次请求都直接吃这个旧错误，连一次都不去试。
		//
		// 这道检查抓的是「已经明确换代」的情形。它和写冷却之间仍有窗口（两把不同的锁），
		// 兜底在读取侧：冷却带着 gen，cooldownError 会把过期的那条丢掉。
		if !dm.genIsCurrent(name, gen) {
			sLogger().Warnf("Discarded downloader %s connect failure: config changed while connecting", name)
			return nil, gen, errConfigSuperseded
		}
		return nil, gen, err
	}

	// 退避序列最长约 31s，用户完全有时间在这期间改完配置点保存。
	// 那样这个实例连的是旧端点，必须就地关掉，不能让它进实例表接收后续操作。
	superseded, ok := dm.storeInstance(name, dl, gen)
	if !ok {
		dl.Close()
		sLogger().Warnf("Discarded downloader instance %s: config changed while connecting", name)
		return nil, gen, errConfigSuperseded
	}
	// 被顶掉的那个旧实例只能在这里回收：它按上一代配置建出来，lookupInstance
	// 已经不认它了，没有别的路径会关它。Close 触发网络 I/O，所以放在锁外。
	if superseded != nil {
		if err := superseded.Close(); err != nil {
			sLogger().Warnf("Failed to close superseded downloader instance %s: %v", name, err)
		}
	}
	sLogger().Infof("Created downloader instance: %s", name)
	return dl, gen, nil
}

// cooldownError 处在失败冷却期内时返回上次的错误，让调用方立刻失败。
// 没有它的话，一个连不上的下载器会被每一次轮询重新尝试，退避日志刷个不停。
// curGen 是调用方在进 flightsMu 之前读到的当前配置代次。
// 由调用方传进来而不是在这里读：读它要 dm.mu，而这里已经持有 flightsMu，
// 反序加锁会和 reviveOrCreate 构成锁序反转。
func (dm *DownloaderManager) cooldownError(name string, curGen uint64) error {
	dm.flightsMu.Lock()
	defer dm.flightsMu.Unlock()

	// 已经有尝试在跑，就去等它，不看冷却
	if _, inFlight := dm.flights[name]; inFlight {
		return nil
	}
	failure, exists := dm.cooldowns[name]
	if !exists {
		return nil
	}
	// 这条冷却是对着旧配置得出的，对当前配置没有参考价值 —— 丢掉，让这次请求真的去试。
	// 这就是那个「检查代次」与「写冷却」之间的窗口的兜底：无论旧 flight 什么时候提交，
	// 只要它带的代次过期，读到它的人就当它不存在。
	if failure.gen != curGen {
		delete(dm.cooldowns, name)
		return nil
	}
	elapsed := time.Since(failure.at)
	if elapsed >= defaultFailureCooldown {
		delete(dm.cooldowns, name)
		return nil
	}
	return fmt.Errorf("下载器 %s 暂不可用，%s 前创建失败，%s 后重试: %w",
		name, elapsed.Truncate(time.Second), (defaultFailureCooldown - elapsed).Truncate(time.Second), failure.err)
}

// clearCooldown 清掉失败冷却。配置变更和显式重连必须立即生效，不能等冷却过期。
//
// 只拿 flightsMu，不碰 dm.mu：调用方可以在持有 dm.mu 时安全调用，
// 但反过来（持 flightsMu 再拿 dm.mu）不允许，否则和 reviveOrCreate 构成锁序反转。
func (dm *DownloaderManager) clearCooldown(name string) {
	dm.flightsMu.Lock()
	defer dm.flightsMu.Unlock()
	delete(dm.cooldowns, name)
}

// clearAllCooldowns 清空全部失败冷却，用于整表同步这类配置全量变更
func (dm *DownloaderManager) clearAllCooldowns() {
	dm.flightsMu.Lock()
	defer dm.flightsMu.Unlock()
	dm.cooldowns = make(map[string]createFailure)
}

// lookupInstance 只读地取一次实例表。
//
// 第二个返回值同时要求「存在」且「是按当前代配置建的」：配置改过之后，手上那个
// 按旧 URL/凭据建出来的实例必须当作不存在，否则暂停、删除、加种会发到旧端点上去。
func (dm *DownloaderManager) lookupInstance(name string) (Downloader, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	dl, exists := dm.downloaders[name]
	if !exists {
		return nil, false
	}
	return dl, dm.instanceGen[name] == dm.configGen[name]
}

// genIsCurrent 判断手上这一代配置是否仍是最新的一代。
// 建连结束时（无论成功还是失败）都要问一次：期间配置可能已经被改掉了。
func (dm *DownloaderManager) genIsCurrent(name string, gen uint64) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.configGen[name] == gen
}

// bumpConfigGenLocked 让该下载器的配置进入新的一代，调用方必须已持有 dm.mu 写锁。
//
// 效果有两层：已登记的实例立刻被视为过期（lookupInstance 不再返回它），
// 正在后台跑的那次建连即使成功也进不了实例表（storeInstance 会拒绝并关掉它）。
// 配置内容真的变更、禁用、删除、整表同步都要调它 —— 只清失败冷却是不够的，
// 冷却只影响「下一次尝试何时开始」，不影响「已经在飞的那一次会写回什么」。
// 反过来，内容没变时不许调：那只会把健康实例白白作废（见 storeConfig）。
func (dm *DownloaderManager) bumpConfigGenLocked(name string) {
	dm.configGen[name]++
}

// markHealthy 记录一次成功的健康确认
func (dm *DownloaderManager) markHealthy(name string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.errorCounts[name] = 0
	dm.lastHealthCheck[name] = time.Now()
}

// dropInstance 丢弃一个确认不可用的实例。Close 可能触发网络 I/O，放在锁外。
func (dm *DownloaderManager) dropInstance(name string, dl Downloader) {
	dm.mu.Lock()
	if cur, exists := dm.downloaders[name]; exists && cur == dl {
		delete(dm.downloaders, name)
		delete(dm.instanceGen, name)
	}
	dm.mu.Unlock()
	dl.Close()
}

// storeInstance 登记新建成功的实例。
//
// gen 是这次建连开始时读到的配置代次。期间配置若被改过（代次已经往前走），
// 这个实例就是按旧配置建的，不能入表 —— 第二个返回值为 false，由调用方关掉它。
//
// 第一个返回值是被这次登记顶掉的旧实例（没有则为 nil），必须由调用方关闭。
// 代次一抬升，lookupInstance 就把表里那个旧实例当作不存在，于是它既不会走
// dropInstance、也不满足 applySyncFromDB 的「配置变更」关闭条件，只会在这里被
// 静默覆盖；不交回去关就是一条连接泄漏。scheduler 的配置 reload 会对每个启用的
// 下载器重新注册一遍，泄漏会按重连次数累积。
// 不在这里直接关是因为 Close 会触发网络 I/O，不能在 dm.mu 里做。
func (dm *DownloaderManager) storeInstance(name string, dl Downloader, gen uint64) (Downloader, bool) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if dm.configGen[name] != gen {
		return nil, false
	}
	prev := dm.downloaders[name]
	if prev == dl {
		// 同一个实例重新登记（代次未变的重入），没有东西被顶掉
		prev = nil
	}
	dm.downloaders[name] = dl
	dm.instanceGen[name] = gen
	dm.errorCounts[name] = 0
	dm.lastHealthCheck[name] = time.Now()
	return prev, true
}

// ErrDownloaderNotConfigured 表示 manager 里根本没有这个名字的下载器配置：
// 从未注册、已被删除，或在从库同步时已被移除 / 禁用。
//
// 它说的是「这台下载器不在了」，不是「暂时连不上」。调用方要据此区分两种处理 ——
// 例如删除暂停种子时，只有这种情况才允许只清 pt-tools 里的孤儿记录；获取超时、
// 失败冷却、建连失败都意味着下载器里的任务还在，不能当孤儿处理。
// 必须用 errors.Is 判断：它会被 flight 与失败冷却层层包装。
var ErrDownloaderNotConfigured = errors.New("no config found for downloader")

// resolveFactory 取出配置、对应工厂和当前配置代次，只在锁内读 map，不调用工厂
func (dm *DownloaderManager) resolveFactory(
	name string,
) (DownloaderConfig, DownloaderFactory, uint64, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	config, exists := dm.configs[name]
	if !exists {
		// 文本与原先的 "no config found for downloader: <name>" 一致，只是现在能被 errors.Is 认出
		return nil, nil, 0, fmt.Errorf("%w: %s", ErrDownloaderNotConfigured, name)
	}
	factory, exists := dm.factories[config.GetType()]
	if !exists {
		return nil, nil, 0, fmt.Errorf("no factory registered for type: %s", config.GetType())
	}
	return config, factory, dm.configGen[name], nil
}

// bumpErrorCount 递增错误计数。createWithRetry 现在跑在锁外，这张 map 的写入
// 必须自己拿锁，否则和 GetAllDownloaderStatus 等读取方构成数据竞争。
func (dm *DownloaderManager) bumpErrorCount(name string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.errorCounts[name]++
}

// createWithRetry 使用指数退避重试创建下载器。
//
// 只允许由 runFlight 调用：它会睡眠并建连，最长约 31s 退避加每次尝试的连接超时，
// 因此绝不能在 dm.mu 里跑，也绝不能让 Web 请求直接等它 —— 请求通过
// GetDownloaderContext 用自己的预算等待 flight，等不到就先返回。
func (dm *DownloaderManager) createWithRetry(
	name string,
	config DownloaderConfig,
	factory DownloaderFactory,
) (Downloader, error) {
	var lastErr error
	backoff := dm.reconnectConfig.InitialBackoff

	for attempt := 0; attempt <= dm.reconnectConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			sLogger().Infof("Retrying to create downloader %s (attempt %d/%d) after %v",
				name, attempt, dm.reconnectConfig.MaxRetries, backoff)
			time.Sleep(backoff)
			// 计算下一次退避时间
			backoff = min(time.Duration(float64(backoff)*dm.reconnectConfig.Multiplier), dm.reconnectConfig.MaxBackoff)
		}

		dl, err := factory(config, name)
		if err == nil {
			return dl, nil
		}
		lastErr = err
		dm.bumpErrorCount(name)
		sLogger().Warnf("Failed to create downloader %s (attempt %d): %v", name, attempt+1, err)
	}

	return nil, fmt.Errorf("failed to create downloader %s after %d attempts: %w",
		name, dm.reconnectConfig.MaxRetries+1, lastErr)
}

// ReconnectDownloader 重新连接指定下载器
func (dm *DownloaderManager) ReconnectDownloader(name string) error {
	return dm.ReconnectDownloaderContext(context.Background(), name)
}

// ReconnectDownloaderContext 重新连接指定下载器，等待阶段受 ctx 约束。
//
// 先丢掉旧实例并清掉失败冷却（用户点重连就是要立刻重试，不该等冷却过期），
// 然后借用与普通获取相同的单飞通道，避免重连和轮询并发建出两个实例。
func (dm *DownloaderManager) ReconnectDownloaderContext(ctx context.Context, name string) error {
	if _, _, _, err := dm.resolveFactory(name); err != nil {
		return err
	}

	// 关闭现有实例。Close 可能触发网络 I/O，放在锁外。
	if old, exists := dm.lookupInstance(name); exists {
		dm.dropInstance(name, old)
	}
	dm.clearCooldown(name)

	if _, err := dm.reviveOrCreate(ctx, name); err != nil {
		return err
	}

	sLogger().Infof("Reconnected downloader: %s", name)
	return nil
}

// GetDownloaderForSite 获取站点对应的下载器
// 如果站点有指定下载器则使用，否则使用默认下载器
func (dm *DownloaderManager) GetDownloaderForSite(siteName string) (Downloader, error) {
	dm.mu.RLock()
	downloaderName, exists := dm.siteDownloaders[siteName]
	if !exists {
		downloaderName = dm.defaultName
	}
	dm.mu.RUnlock()

	if downloaderName == "" {
		return nil, fmt.Errorf("no downloader configured for site %s and no default set", siteName)
	}

	return dm.GetDownloader(downloaderName)
}

// GetDefaultDownloader 获取默认下载器
func (dm *DownloaderManager) GetDefaultDownloader() (Downloader, error) {
	dm.mu.RLock()
	defaultName := dm.defaultName
	dm.mu.RUnlock()

	if defaultName == "" {
		return nil, fmt.Errorf("no default downloader configured")
	}

	return dm.GetDownloader(defaultName)
}

// ListDownloaders 列出所有已注册的下载器配置
func (dm *DownloaderManager) ListDownloaders() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	names := make([]string, 0, len(dm.configs))
	for name := range dm.configs {
		names = append(names, name)
	}
	return names
}

// GetDownloaderHealth 获取下载器健康状态
func (dm *DownloaderManager) GetDownloaderHealth(name string) (bool, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dl, exists := dm.downloaders[name]
	if !exists {
		return false, fmt.Errorf("downloader %s not instantiated", name)
	}

	healthy := dl.IsHealthy()
	dm.lastHealthCheck[name] = time.Now()
	if !healthy {
		dm.errorCounts[name]++
	} else {
		dm.errorCounts[name] = 0
	}

	return healthy, nil
}

// GetAllDownloaderStatus 获取所有下载器状态
func (dm *DownloaderManager) GetAllDownloaderStatus() []DownloaderStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	statuses := make([]DownloaderStatus, 0, len(dm.configs))
	for name, config := range dm.configs {
		status := DownloaderStatus{
			Name:       name,
			Type:       string(config.GetType()),
			IsDefault:  name == dm.defaultName,
			ErrorCount: dm.errorCounts[name],
		}

		if dl, exists := dm.downloaders[name]; exists {
			status.IsHealthy = dl.IsHealthy()
		}

		if lastCheck, exists := dm.lastHealthCheck[name]; exists {
			status.LastChecked = lastCheck
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// GetErrorCount 获取下载器错误计数
func (dm *DownloaderManager) GetErrorCount(name string) int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.errorCounts[name]
}

// CalculateBackoff 计算指数退避时间
func (dm *DownloaderManager) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return dm.reconnectConfig.InitialBackoff
	}
	backoff := float64(dm.reconnectConfig.InitialBackoff) * math.Pow(dm.reconnectConfig.Multiplier, float64(attempt))
	if time.Duration(backoff) > dm.reconnectConfig.MaxBackoff {
		return dm.reconnectConfig.MaxBackoff
	}
	return time.Duration(backoff)
}

// CloseAll 关闭所有下载器实例
func (dm *DownloaderManager) CloseAll() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for name, dl := range dm.downloaders {
		if err := dl.Close(); err != nil {
			sLogger().Errorf("Failed to close downloader %s: %v", name, err)
		}
	}
	dm.downloaders = make(map[string]Downloader)
	dm.instanceGen = make(map[string]uint64)
	// 全部换代：关停期间仍在飞的建连即使成功，也不会把实例塞回刚清空的表里
	for name := range dm.configs {
		dm.bumpConfigGenLocked(name)
	}
	sLogger().Info("All downloaders closed")
}

// RemoveDownloader 移除下载器配置和实例
func (dm *DownloaderManager) RemoveDownloader(name string) error {
	if err := dm.removeState(name); err != nil {
		return err
	}
	// 同名下载器以后可能被重新加进来，别让旧的失败冷却继续生效
	dm.clearCooldown(name)
	sLogger().Infof("Removed downloader: %s", name)
	return nil
}

func (dm *DownloaderManager) removeState(name string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 关闭实例
	if dl, exists := dm.downloaders[name]; exists {
		if err := dl.Close(); err != nil {
			sLogger().Errorf("Failed to close downloader %s: %v", name, err)
		}
		delete(dm.downloaders, name)
	}
	delete(dm.instanceGen, name)

	// 删除配置。换代是为了拦住正在飞的那次建连：它连的是刚被删掉的下载器，
	// 成功了也不能塞回实例表，否则删除之后还能对它下发操作。
	delete(dm.configs, name)
	dm.bumpConfigGenLocked(name)

	// 如果是默认下载器，清除默认设置
	if dm.defaultName == name {
		dm.defaultName = ""
	}

	// 清除站点映射
	for site, dlName := range dm.siteDownloaders {
		if dlName == name {
			delete(dm.siteDownloaders, site)
		}
	}

	return nil
}

// CreateFromConfig 从配置创建临时下载器实例（不注册到管理器）
// 调用方需要负责调用 Close() 释放资源
func (dm *DownloaderManager) CreateFromConfig(config DownloaderConfig, name string) (Downloader, error) {
	dm.mu.RLock()
	factory, exists := dm.factories[config.GetType()]
	dm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no factory registered for type: %s", config.GetType())
	}

	return factory(config, name)
}

// HasFactory 检查是否已注册指定类型的工厂
func (dm *DownloaderManager) HasFactory(dlType DownloaderType) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	_, exists := dm.factories[dlType]
	return exists
}

// DownloaderDBRecord 表示数据库中的下载器配置记录
type DownloaderDBRecord struct {
	Name      string
	Type      DownloaderType
	URL       string
	Username  string
	Password  string
	IsDefault bool
	Enabled   bool
	AutoStart bool
}

// SyncFromDB 从数据库记录同步下载器配置
// 处理新增、删除、更新三种情况，确保内存状态与数据库一致
func (dm *DownloaderManager) SyncFromDB(records []DownloaderDBRecord) {
	dm.applySyncFromDB(records)
	// 整张配置表刚被重放了一遍，之前的失败冷却一律作废：
	// 用户改完下载器地址点保存，下一次请求就该真的去连，而不是继续吃上次的错误。
	dm.clearAllCooldowns()
}

func (dm *DownloaderManager) applySyncFromDB(records []DownloaderDBRecord) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	dbConfigs := make(map[string]DownloaderDBRecord)
	var newDefaultName string

	for _, r := range records {
		if !r.Enabled {
			continue
		}
		dbConfigs[r.Name] = r
		if r.IsDefault {
			newDefaultName = r.Name
		}
	}

	for name, dl := range dm.downloaders {
		dbRecord, existsInDB := dbConfigs[name]
		if !existsInDB {
			sLogger().Infof("[SyncFromDB] 删除下载器: %s (已从数据库移除或禁用)", name)
			dl.Close()
			delete(dm.downloaders, name)
			delete(dm.instanceGen, name)
			delete(dm.configs, name)
			delete(dm.errorCounts, name)
			delete(dm.lastHealthCheck, name)
			dm.bumpConfigGenLocked(name)
			continue
		}

		oldConfig, hasOldConfig := dm.configs[name]
		if hasOldConfig && dm.configChanged(oldConfig, dbRecord) {
			sLogger().Infof("[SyncFromDB] 更新下载器: %s (配置已变更)", name)
			dl.Close()
			delete(dm.downloaders, name)
			delete(dm.instanceGen, name)
		}
	}

	for name := range dm.configs {
		if _, existsInDB := dbConfigs[name]; !existsInDB {
			delete(dm.configs, name)
			delete(dm.errorCounts, name)
			delete(dm.lastHealthCheck, name)
			dm.bumpConfigGenLocked(name)
		}
	}

	for name, r := range dbConfigs {
		if _, exists := dm.factories[r.Type]; !exists {
			sLogger().Warnf("[SyncFromDB] 跳过下载器 %s: 未知类型 %s", name, r.Type)
			continue
		}
		config := NewGenericConfig(r.Type, r.URL, r.Username, r.Password, r.AutoStart)
		// 只有真的变了（或本来就没有）才换代。SyncFromDB 每收到一次配置事件就整表
		// 重放一遍，无差别换代会把所有健康实例作废，逼出一轮毫无必要的重连。
		//
		// 这里必须独立判断，不能只靠上面那个 downloaders 循环：一个正在重连、
		// 还没有实例的下载器恰好不在那张表里，而它正是最需要拦的情形 ——
		// 实例之所以不存在，就是因为建连还在退避中。
		if old, had := dm.configs[name]; !had || dm.configChanged(old, r) {
			dm.bumpConfigGenLocked(name)
		}
		dm.configs[name] = config
	}

	dm.defaultName = newDefaultName

	for site, dlName := range dm.siteDownloaders {
		if _, exists := dbConfigs[dlName]; !exists {
			delete(dm.siteDownloaders, site)
		}
	}

	sLogger().Infof("[SyncFromDB] 同步完成: 配置数=%d, 默认=%s", len(dm.configs), dm.defaultName)
}

func (dm *DownloaderManager) configChanged(oldConfig DownloaderConfig, newRecord DownloaderDBRecord) bool {
	if oldConfig.GetType() != newRecord.Type {
		return true
	}
	if oldConfig.GetURL() != newRecord.URL {
		return true
	}
	if oldConfig.GetUsername() != newRecord.Username {
		return true
	}
	if oldConfig.GetPassword() != newRecord.Password {
		return true
	}
	if oldConfig.GetAutoStart() != newRecord.AutoStart {
		return true
	}
	return false
}
