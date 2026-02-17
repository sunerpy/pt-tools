package downloader

import (
	"context"
	"errors"
	"time"
)

// DownloaderType 定义下载器类型
type DownloaderType string

const (
	DownloaderQBittorrent  DownloaderType = "qbittorrent"
	DownloaderTransmission DownloaderType = "transmission"
)

// TorrentState 种子状态
type TorrentState string

const (
	TorrentDownloading TorrentState = "downloading"
	TorrentSeeding     TorrentState = "seeding"
	TorrentPaused      TorrentState = "paused"
	TorrentQueued      TorrentState = "queued"
	TorrentChecking    TorrentState = "checking"
	TorrentError       TorrentState = "error"
	TorrentUnknown     TorrentState = "unknown"
)

// Common errors
var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrConnectionFailed     = errors.New("connection failed")
	ErrTorrentNotFound      = errors.New("torrent not found")
	ErrInsufficientSpace    = errors.New("insufficient disk space")
	ErrInvalidConfig        = errors.New("invalid configuration")
)

// ClientStatus 下载器客户端状态
type ClientStatus struct {
	UpSpeed int64 // 上传速度 (bytes/s)
	UpData  int64 // 总上传量 (bytes)
	DlSpeed int64 // 下载速度 (bytes/s)
	DlData  int64 // 总下载量 (bytes)
}

// Torrent 种子信息
type Torrent struct {
	ID              string       // 种子ID
	InfoHash        string       // 种子哈希
	Name            string       // 种子名称
	Progress        float64      // 下载进度 (0.0-1.0)
	IsCompleted     bool         // 是否已完成
	Ratio           float64      // 分享率
	DateAdded       int64        // 添加时间 (Unix timestamp)
	SavePath        string       // 保存路径
	Label           string       // 标签
	Category        string       // 分类 (qBit: category, TR: labels[0])
	Tags            string       // 标签 (qBit: tags, TR: labels joined)
	State           TorrentState // 状态
	TotalSize       int64        // 总大小 (bytes)
	UploadSpeed     int64        // 上传速度 (bytes/s)
	DownloadSpeed   int64        // 下载速度 (bytes/s)
	ETA             int64        // 预计剩余时间(秒), -1=无限, 0=完成
	SeedingTime     int64        // 做种时间(秒)
	Tracker         string       // 主 Tracker URL
	CompletionOn    int64        // 完成时间 (Unix timestamp), 0=未完成
	NumSeeds        int          // 种子数
	NumPeers        int          // 下载者数
	Availability    float64      // 可用性 (0.0-1.0+)
	ContentPath     string       // 内容路径 (文件/文件夹的完整路径)
	TotalUploaded   int64        // 总上传量 (bytes)
	TotalDownloaded int64        // 总下载量 (bytes)
	Raw             any          // 原始数据
	ClientID        string       // 客户端ID
}

// TorrentFile 种子内的文件信息
type TorrentFile struct {
	Index    int     // 文件索引
	Name     string  // 文件路径/名称
	Size     int64   // 文件大小 (bytes)
	Progress float64 // 下载进度 (0.0-1.0)
	Priority int     // 优先级 (0=不下载, 1=普通, 6=高, 7=最高)
}

// TorrentTracker Tracker 信息
type TorrentTracker struct {
	URL     string // Tracker URL
	Status  int    // 状态 (0=禁用, 1=未联系, 2=工作中, 3=更新中, 4=出错)
	Peers   int    // Peer 数
	Seeds   int    // Seed 数
	Leeches int    // Leech 数
	Message string // 状态消息
}

// SpeedLimit 速度限制
type SpeedLimit struct {
	DownloadLimit int64 // 下载限速 (bytes/s), 0=不限
	UploadLimit   int64 // 上传限速 (bytes/s), 0=不限
	LimitEnabled  bool  // 是否启用限速
}

// DiskInfo 磁盘信息
type DiskInfo struct {
	Path      string // 路径
	FreeSpace int64  // 可用空间 (bytes)
	TotalSize int64  // 总空间 (bytes), 0 表示未知
}

// AddTorrentOptions 添加种子的选项
// 用于统一控制种子添加行为，替代原有的分散参数
type AddTorrentOptions struct {
	// AddAtPaused 是否以暂停状态添加种子
	// true: 种子添加后处于暂停状态，需要手动启动
	// false: 种子添加后自动开始下载（默认行为）
	// 注意：此参数替代了原有的 autoStart 配置，逻辑相反
	// autoStart=true 等价于 AddAtPaused=false
	AddAtPaused bool

	// SavePath 保存路径（可选）
	// 空字符串表示使用下载器默认路径
	SavePath string

	// Category 种子分类（可选）
	Category string

	// Tags 种子标签（可选，逗号分隔）
	Tags string

	// UploadSpeedLimitMB 上传速度限制 (MB/s)
	// 0 表示不限制
	UploadSpeedLimitMB int

	// AdvanceOptions 高级选项（可选）
	// 用于传递客户端特定的高级配置
	AdvanceOptions map[string]any
}

// AddTorrentResult 添加种子的结果
type AddTorrentResult struct {
	Success bool   // 是否成功
	Message any    // 消息（错误信息或成功信息）
	ID      string // 种子ID（成功时返回）
	Hash    string // 种子哈希
}

// TorrentFilter 种子过滤条件
type TorrentFilter struct {
	IDs      []string      // 按ID过滤
	Hashes   []string      // 按哈希过滤
	Complete *bool         // 按完成状态过滤
	State    *TorrentState // 按状态过滤
}

// DownloaderConfig 下载器配置接口
type DownloaderConfig interface {
	// GetType 获取下载器类型
	GetType() DownloaderType
	// GetURL 获取下载器 URL
	GetURL() string
	// GetUsername 获取用户名
	GetUsername() string
	// GetPassword 获取密码
	GetPassword() string
	// GetAutoStart 获取是否自动开始下载
	// Deprecated: 请使用 AddTorrentOptions.AddAtPaused 替代
	// autoStart=true 等价于 AddAtPaused=false
	GetAutoStart() bool
	// Validate 验证配置是否有效
	Validate() error
}

// ToAddTorrentOptions 将配置转换为 AddTorrentOptions
// 用于向后兼容，将 autoStart 映射到 AddAtPaused
func ToAddTorrentOptions(config DownloaderConfig, category, tags, savePath string) AddTorrentOptions {
	return AddTorrentOptions{
		AddAtPaused: !config.GetAutoStart(), // autoStart=true -> AddAtPaused=false
		SavePath:    savePath,
		Category:    category,
		Tags:        tags,
	}
}

// Downloader 下载器核心接口
// 所有下载器实现（qBittorrent、Transmission 等）都必须实现此接口
type Downloader interface {
	// Authenticate 认证连接到下载器
	// 返回错误如果认证失败
	Authenticate() error

	// Ping 检查下载器连接是否正常
	Ping() (bool, error)

	// GetClientVersion 获取下载器版本
	GetClientVersion() (string, error)

	// GetClientStatus 获取下载器状态（上传/下载速度等）
	GetClientStatus() (ClientStatus, error)

	// GetClientFreeSpace 获取下载器所在磁盘的可用空间
	// 返回可用空间（字节）
	GetClientFreeSpace(ctx context.Context) (int64, error)

	// GetAllTorrents 获取所有种子列表
	GetAllTorrents() ([]Torrent, error)

	// GetTorrentsBy 根据过滤条件获取种子列表
	GetTorrentsBy(filter TorrentFilter) ([]Torrent, error)

	// GetTorrent 获取单个种子信息
	GetTorrent(id string) (Torrent, error)

	// AddTorrentEx 添加种子到下载器（新接口）
	// url: 种子URL或磁力链接
	// opt: 添加选项
	AddTorrentEx(url string, opt AddTorrentOptions) (AddTorrentResult, error)

	// AddTorrentFileEx 添加种子文件到下载器（新接口）
	// fileData: 种子文件的二进制数据
	// opt: 添加选项
	AddTorrentFileEx(fileData []byte, opt AddTorrentOptions) (AddTorrentResult, error)

	// PauseTorrent 暂停种子
	PauseTorrent(id string) error

	// ResumeTorrent 恢复种子
	ResumeTorrent(id string) error

	// RemoveTorrent 删除种子
	// removeData: 是否同时删除数据文件
	RemoveTorrent(id string, removeData bool) error

	// === 批量操作 ===
	PauseTorrents(ids []string) error
	ResumeTorrents(ids []string) error
	RemoveTorrents(ids []string, removeData bool) error

	// === 修改操作 ===
	SetTorrentCategory(id, category string) error
	SetTorrentTags(id, tags string) error
	SetTorrentSavePath(id, path string) error

	// === 维护操作 ===
	RecheckTorrent(id string) error

	// === 详情查询 ===
	GetTorrentFiles(id string) ([]TorrentFile, error)
	GetTorrentTrackers(id string) ([]TorrentTracker, error)

	// === 全局状态 ===
	GetDiskInfo() (DiskInfo, error)
	GetSpeedLimit() (SpeedLimit, error)
	SetSpeedLimit(limit SpeedLimit) error

	// GetClientPaths 获取下载器配置的保存路径列表
	GetClientPaths() ([]string, error)

	// GetClientLabels 获取下载器配置的标签列表
	GetClientLabels() ([]string, error)

	// GetType 获取下载器类型
	GetType() DownloaderType

	// GetName 获取下载器实例名称
	GetName() string

	// IsHealthy 检查下载器是否健康可用
	IsHealthy() bool

	// Close 关闭下载器连接，释放资源
	Close() error

	// ============ 以下为向后兼容的旧接口 ============

	// AddTorrent 添加种子到下载器（旧接口，向后兼容）
	// Deprecated: 请使用 AddTorrentFileEx 替代
	AddTorrent(fileData []byte, category, tags string) error

	// AddTorrentWithPath 添加种子到下载器并指定下载路径（旧接口，向后兼容）
	// Deprecated: 请使用 AddTorrentFileEx 替代
	AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error

	// CheckTorrentExists 检查种子是否已存在于下载器中
	// torrentHash: 种子的 info hash
	// 返回 true 如果种子存在
	CheckTorrentExists(torrentHash string) (bool, error)

	// GetDiskSpace 获取下载器所在磁盘的可用空间
	// Deprecated: 请使用 GetClientFreeSpace 替代
	GetDiskSpace(ctx context.Context) (int64, error)

	// CanAddTorrent 检查是否有足够空间添加指定大小的种子
	// fileSize: 种子文件大小（字节）
	CanAddTorrent(ctx context.Context, fileSize int64) (bool, error)

	// ProcessSingleTorrentFile 处理单个种子文件
	// filePath: 种子文件路径
	// category: 分类
	// tags: 标签
	ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error
}

// DownloaderFactory 下载器工厂函数类型
// 用于根据配置创建下载器实例
type DownloaderFactory func(config DownloaderConfig, name string) (Downloader, error)

// GenericConfig 通用下载器配置
// 用于不需要特定实现的场景，如健康检查、临时实例创建等
type GenericConfig struct {
	Type      DownloaderType `json:"type"`
	URL       string         `json:"url"`
	Username  string         `json:"username"`
	Password  string         `json:"password"`
	AutoStart bool           `json:"auto_start"`
}

// GetType 获取下载器类型
func (c *GenericConfig) GetType() DownloaderType {
	return c.Type
}

// GetURL 获取下载器 URL
func (c *GenericConfig) GetURL() string {
	return c.URL
}

// GetUsername 获取用户名
func (c *GenericConfig) GetUsername() string {
	return c.Username
}

// GetPassword 获取密码
func (c *GenericConfig) GetPassword() string {
	return c.Password
}

// GetAutoStart 获取是否自动开始下载
func (c *GenericConfig) GetAutoStart() bool {
	return c.AutoStart
}

// Validate 验证配置是否有效
func (c *GenericConfig) Validate() error {
	if c.URL == "" {
		return ErrInvalidConfig
	}
	if c.Type == "" {
		return ErrInvalidConfig
	}
	return nil
}

// NewGenericConfig 创建通用下载器配置
func NewGenericConfig(dlType DownloaderType, url, username, password string, autoStart bool) *GenericConfig {
	return &GenericConfig{
		Type:      dlType,
		URL:       url,
		Username:  username,
		Password:  password,
		AutoStart: autoStart,
	}
}

// DownloadTaskInfo 下载任务信息
type DownloadTaskInfo struct {
	Name          string
	Hash          string
	SizeLeft      int64
	DownloadSpeed int64
	ETA           time.Duration
}
