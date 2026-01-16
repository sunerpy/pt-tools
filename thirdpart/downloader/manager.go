package downloader

import (
	"fmt"
	"math"
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
}

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
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid config for %s: %w", name, err)
	}

	dm.configs[name] = config
	if isDefault {
		dm.defaultName = name
	}

	sLogger().Infof("Registered downloader config: %s (default: %v)", name, isDefault)
	return nil
}

// SetSiteDownloader 设置站点使用的下载器
func (dm *DownloaderManager) SetSiteDownloader(siteName, downloaderName string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.siteDownloaders[siteName] = downloaderName
	sLogger().Infof("Set site %s to use downloader: %s", siteName, downloaderName)
}

// GetDownloader 获取或创建下载器实例
func (dm *DownloaderManager) GetDownloader(name string) (Downloader, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 检查是否已有实例
	if dl, exists := dm.downloaders[name]; exists {
		if dl.IsHealthy() {
			dm.errorCounts[name] = 0 // 重置错误计数
			dm.lastHealthCheck[name] = time.Now()
			return dl, nil
		}
		// Ping to confirm before recreating
		if ok, pingErr := dl.Ping(); ok {
			dm.errorCounts[name] = 0
			dm.lastHealthCheck[name] = time.Now()
			return dl, nil
		} else {
			sLogger().Warnf("Downloader %s is unhealthy (ping failed: %v), recreating...", name, pingErr)
		}
		dl.Close()
		delete(dm.downloaders, name)
	}

	// 获取配置
	config, exists := dm.configs[name]
	if !exists {
		return nil, fmt.Errorf("no config found for downloader: %s", name)
	}

	// 获取工厂
	factory, exists := dm.factories[config.GetType()]
	if !exists {
		return nil, fmt.Errorf("no factory registered for type: %s", config.GetType())
	}

	// 使用指数退避重试创建实例
	dl, err := dm.createWithRetry(name, config, factory)
	if err != nil {
		return nil, err
	}

	dm.downloaders[name] = dl
	dm.errorCounts[name] = 0
	dm.lastHealthCheck[name] = time.Now()
	sLogger().Infof("Created downloader instance: %s", name)
	return dl, nil
}

// createWithRetry 使用指数退避重试创建下载器
func (dm *DownloaderManager) createWithRetry(name string, config DownloaderConfig, factory DownloaderFactory) (Downloader, error) {
	var lastErr error
	backoff := dm.reconnectConfig.InitialBackoff

	for attempt := 0; attempt <= dm.reconnectConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			sLogger().Infof("Retrying to create downloader %s (attempt %d/%d) after %v",
				name, attempt, dm.reconnectConfig.MaxRetries, backoff)
			time.Sleep(backoff)
			// 计算下一次退避时间
			backoff = time.Duration(float64(backoff) * dm.reconnectConfig.Multiplier)
			if backoff > dm.reconnectConfig.MaxBackoff {
				backoff = dm.reconnectConfig.MaxBackoff
			}
		}

		dl, err := factory(config, name)
		if err == nil {
			return dl, nil
		}
		lastErr = err
		dm.errorCounts[name]++
		sLogger().Warnf("Failed to create downloader %s (attempt %d): %v", name, attempt+1, err)
	}

	return nil, fmt.Errorf("failed to create downloader %s after %d attempts: %w",
		name, dm.reconnectConfig.MaxRetries+1, lastErr)
}

// ReconnectDownloader 重新连接指定下载器
func (dm *DownloaderManager) ReconnectDownloader(name string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 关闭现有实例
	if dl, exists := dm.downloaders[name]; exists {
		dl.Close()
		delete(dm.downloaders, name)
	}

	// 获取配置
	config, exists := dm.configs[name]
	if !exists {
		return fmt.Errorf("no config found for downloader: %s", name)
	}

	// 获取工厂
	factory, exists := dm.factories[config.GetType()]
	if !exists {
		return fmt.Errorf("no factory registered for type: %s", config.GetType())
	}

	// 使用指数退避重试创建实例
	dl, err := dm.createWithRetry(name, config, factory)
	if err != nil {
		return err
	}

	dm.downloaders[name] = dl
	dm.errorCounts[name] = 0
	dm.lastHealthCheck[name] = time.Now()
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
	sLogger().Info("All downloaders closed")
}

// RemoveDownloader 移除下载器配置和实例
func (dm *DownloaderManager) RemoveDownloader(name string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 关闭实例
	if dl, exists := dm.downloaders[name]; exists {
		if err := dl.Close(); err != nil {
			sLogger().Errorf("Failed to close downloader %s: %v", name, err)
		}
		delete(dm.downloaders, name)
	}

	// 删除配置
	delete(dm.configs, name)

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

	sLogger().Infof("Removed downloader: %s", name)
	return nil
}
