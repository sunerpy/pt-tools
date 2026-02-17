package downloader

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// MockDownloader 用于测试的模拟下载器
type MockDownloader struct {
	name    string
	dlType  DownloaderType
	healthy bool
}

func (m *MockDownloader) Authenticate() error                    { return nil }
func (m *MockDownloader) Ping() (bool, error)                    { return m.healthy, nil }
func (m *MockDownloader) GetClientVersion() (string, error)      { return "1.0.0", nil }
func (m *MockDownloader) GetClientStatus() (ClientStatus, error) { return ClientStatus{}, nil }
func (m *MockDownloader) GetClientFreeSpace(ctx context.Context) (int64, error) {
	return 1024 * 1024 * 1024, nil
}
func (m *MockDownloader) GetAllTorrents() ([]Torrent, error)                    { return nil, nil }
func (m *MockDownloader) GetTorrentsBy(filter TorrentFilter) ([]Torrent, error) { return nil, nil }
func (m *MockDownloader) GetTorrent(id string) (Torrent, error)                 { return Torrent{}, nil }
func (m *MockDownloader) AddTorrentEx(url string, opt AddTorrentOptions) (AddTorrentResult, error) {
	return AddTorrentResult{Success: true}, nil
}

func (m *MockDownloader) AddTorrentFileEx(fileData []byte, opt AddTorrentOptions) (AddTorrentResult, error) {
	return AddTorrentResult{Success: true}, nil
}
func (m *MockDownloader) PauseTorrent(id string) error                            { return nil }
func (m *MockDownloader) ResumeTorrent(id string) error                           { return nil }
func (m *MockDownloader) RemoveTorrent(id string, removeData bool) error          { return nil }
func (m *MockDownloader) PauseTorrents(ids []string) error                        { return nil }
func (m *MockDownloader) ResumeTorrents(ids []string) error                       { return nil }
func (m *MockDownloader) RemoveTorrents(ids []string, removeData bool) error      { return nil }
func (m *MockDownloader) SetTorrentCategory(id, category string) error            { return nil }
func (m *MockDownloader) SetTorrentTags(id, tags string) error                    { return nil }
func (m *MockDownloader) SetTorrentSavePath(id, path string) error                { return nil }
func (m *MockDownloader) RecheckTorrent(id string) error                          { return nil }
func (m *MockDownloader) GetTorrentFiles(id string) ([]TorrentFile, error)        { return nil, nil }
func (m *MockDownloader) GetTorrentTrackers(id string) ([]TorrentTracker, error)  { return nil, nil }
func (m *MockDownloader) GetDiskInfo() (DiskInfo, error)                          { return DiskInfo{}, nil }
func (m *MockDownloader) GetSpeedLimit() (SpeedLimit, error)                      { return SpeedLimit{}, nil }
func (m *MockDownloader) SetSpeedLimit(limit SpeedLimit) error                    { return nil }
func (m *MockDownloader) GetClientPaths() ([]string, error)                       { return nil, nil }
func (m *MockDownloader) GetClientLabels() ([]string, error)                      { return nil, nil }
func (m *MockDownloader) AddTorrent(fileData []byte, category, tags string) error { return nil }
func (m *MockDownloader) AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error {
	return nil
}
func (m *MockDownloader) CheckTorrentExists(torrentHash string) (bool, error) { return false, nil }
func (m *MockDownloader) GetDiskSpace(ctx context.Context) (int64, error) {
	return 1024 * 1024 * 1024, nil
}

func (m *MockDownloader) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	return true, nil
}

func (m *MockDownloader) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	return nil
}
func (m *MockDownloader) GetType() DownloaderType { return m.dlType }
func (m *MockDownloader) GetName() string         { return m.name }
func (m *MockDownloader) IsHealthy() bool         { return m.healthy }
func (m *MockDownloader) Close() error            { m.healthy = false; return nil }

// MockConfig 已在 interface_test.go 中定义，此处复用

// MockDownloaderFactory 创建模拟下载器的工厂
func MockDownloaderFactory(config DownloaderConfig, name string) (Downloader, error) {
	return &MockDownloader{
		name:    name,
		dlType:  config.GetType(),
		healthy: true,
	}, nil
}

// TestDownloaderManagerBasic 测试下载器管理器基本功能
func TestDownloaderManagerBasic(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	// 注册配置
	config := &MockConfig{
		Type:     DownloaderQBittorrent,
		URL:      "http://localhost:8080",
		Username: "admin",
		Password: "password",
	}
	err := dm.RegisterConfig("qbit-default", config, true)
	if err != nil {
		t.Fatalf("failed to register config: %v", err)
	}

	// 获取下载器
	dl, err := dm.GetDownloader("qbit-default")
	if err != nil {
		t.Fatalf("failed to get downloader: %v", err)
	}

	if dl.GetName() != "qbit-default" {
		t.Errorf("expected name 'qbit-default', got '%s'", dl.GetName())
	}
	if dl.GetType() != DownloaderQBittorrent {
		t.Errorf("expected type %s, got %s", DownloaderQBittorrent, dl.GetType())
	}
}

// TestProperty3_DownloaderSelectionPriority 属性测试：下载器选择优先级
// Feature: downloader-site-extensibility, Property 3: Downloader Selection Priority
// 验证站点级别配置优先于全局默认配置
func TestProperty3_DownloaderSelectionPriority(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 生成非空字符串
	nonEmptyStringGen := gen.Identifier().Map(func(s string) string {
		if s == "" {
			return "default"
		}
		return s
	})

	// Property 3.1: 站点有指定下载器时，使用站点指定的下载器
	properties.Property("site-specific downloader takes priority over default", prop.ForAll(
		func(siteName, siteDownloaderSuffix, defaultDownloaderSuffix string) bool {
			// 确保两个下载器名称不同
			siteDownloader := "site-" + siteDownloaderSuffix
			defaultDownloader := "default-" + defaultDownloaderSuffix

			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
			dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

			// 注册默认下载器
			defaultConfig := &MockConfig{
				Type: DownloaderQBittorrent,
				URL:  "http://default:8080",
			}
			dm.RegisterConfig(defaultDownloader, defaultConfig, true)

			// 注册站点专用下载器
			siteConfig := &MockConfig{
				Type: DownloaderTransmission,
				URL:  "http://site-specific:9091",
			}
			dm.RegisterConfig(siteDownloader, siteConfig, false)

			// 设置站点使用特定下载器
			dm.SetSiteDownloader(siteName, siteDownloader)

			// 获取站点下载器
			dl, err := dm.GetDownloaderForSite(siteName)
			if err != nil {
				t.Logf("Error getting downloader for site: %v", err)
				return false
			}

			// 验证返回的是站点指定的下载器，而不是默认下载器
			return dl.GetName() == siteDownloader && dl.GetType() == DownloaderTransmission
		},
		nonEmptyStringGen,
		nonEmptyStringGen,
		nonEmptyStringGen,
	))

	// Property 3.2: 站点没有指定下载器时，使用默认下载器
	properties.Property("default downloader used when site has no specific config", prop.ForAll(
		func(siteName, defaultDownloaderSuffix string) bool {
			defaultDownloader := "default-" + defaultDownloaderSuffix

			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

			// 只注册默认下载器
			defaultConfig := &MockConfig{
				Type: DownloaderQBittorrent,
				URL:  "http://default:8080",
			}
			dm.RegisterConfig(defaultDownloader, defaultConfig, true)

			// 不设置站点下载器映射

			// 获取站点下载器
			dl, err := dm.GetDownloaderForSite(siteName)
			if err != nil {
				t.Logf("Error getting downloader for site: %v", err)
				return false
			}

			// 验证返回的是默认下载器
			return dl.GetName() == defaultDownloader
		},
		nonEmptyStringGen,
		nonEmptyStringGen,
	))

	// Property 3.3: 没有默认下载器且站点没有指定时，返回错误
	properties.Property("error returned when no default and no site-specific config", prop.ForAll(
		func(siteName string) bool {
			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

			// 注册一个非默认下载器
			config := &MockConfig{
				Type: DownloaderQBittorrent,
				URL:  "http://localhost:8080",
			}
			dm.RegisterConfig("some-downloader", config, false) // not default

			// 不设置站点下载器映射，也没有默认下载器

			// 获取站点下载器应该返回错误
			_, err := dm.GetDownloaderForSite(siteName)
			return err != nil
		},
		nonEmptyStringGen,
	))

	properties.TestingRun(t)
}

// TestDownloaderManagerMultipleDownloaders 测试多下载器管理
func TestDownloaderManagerMultipleDownloaders(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	// 注册多个下载器
	qbitConfig := &MockConfig{Type: DownloaderQBittorrent, URL: "http://qbit:8080"}
	transConfig := &MockConfig{Type: DownloaderTransmission, URL: "http://trans:9091"}

	dm.RegisterConfig("qbit-1", qbitConfig, true)
	dm.RegisterConfig("trans-1", transConfig, false)

	// 设置站点映射
	dm.SetSiteDownloader("hdsky", "qbit-1")
	dm.SetSiteDownloader("mteam", "trans-1")

	// 验证站点获取正确的下载器
	hdsky, err := dm.GetDownloaderForSite("hdsky")
	if err != nil {
		t.Fatalf("failed to get downloader for hdsky: %v", err)
	}
	if hdsky.GetName() != "qbit-1" {
		t.Errorf("expected qbit-1 for hdsky, got %s", hdsky.GetName())
	}

	mteam, err := dm.GetDownloaderForSite("mteam")
	if err != nil {
		t.Fatalf("failed to get downloader for mteam: %v", err)
	}
	if mteam.GetName() != "trans-1" {
		t.Errorf("expected trans-1 for mteam, got %s", mteam.GetName())
	}

	// 未配置的站点使用默认下载器
	other, err := dm.GetDownloaderForSite("other-site")
	if err != nil {
		t.Fatalf("failed to get downloader for other-site: %v", err)
	}
	if other.GetName() != "qbit-1" {
		t.Errorf("expected default qbit-1 for other-site, got %s", other.GetName())
	}
}

// TestDownloaderManagerListDownloaders 测试列出下载器
func TestDownloaderManagerListDownloaders(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, false)
	dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderTransmission, URL: "http://2"}, false)
	dm.RegisterConfig("dl-3", &MockConfig{Type: DownloaderQBittorrent, URL: "http://3"}, true)

	names := dm.ListDownloaders()
	if len(names) != 3 {
		t.Errorf("expected 3 downloaders, got %d", len(names))
	}

	// 验证所有名称都在列表中
	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}
	for _, expected := range []string{"dl-1", "dl-2", "dl-3"} {
		if !nameMap[expected] {
			t.Errorf("expected %s in list", expected)
		}
	}
}

// TestDownloaderManagerRemoveDownloader 测试移除下载器
func TestDownloaderManagerRemoveDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("to-remove", &MockConfig{Type: DownloaderQBittorrent, URL: "http://remove"}, true)
	dm.SetSiteDownloader("site1", "to-remove")

	// 先获取一次以创建实例
	_, err := dm.GetDownloader("to-remove")
	if err != nil {
		t.Fatalf("failed to get downloader: %v", err)
	}

	// 移除
	err = dm.RemoveDownloader("to-remove")
	if err != nil {
		t.Fatalf("failed to remove downloader: %v", err)
	}

	// 验证已移除
	_, err = dm.GetDownloader("to-remove")
	if err == nil {
		t.Error("expected error after removing downloader")
	}

	// 验证站点映射已清除
	_, err = dm.GetDownloaderForSite("site1")
	if err == nil {
		t.Error("expected error for site after removing its downloader")
	}
}

// TestDownloaderManagerCloseAll 测试关闭所有下载器
func TestDownloaderManagerCloseAll(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, false)
	dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderQBittorrent, URL: "http://2"}, false)

	// 创建实例
	dl1, _ := dm.GetDownloader("dl-1")
	dl2, _ := dm.GetDownloader("dl-2")

	// 关闭所有
	dm.CloseAll()

	// 验证实例已关闭
	if dl1.IsHealthy() {
		t.Error("dl-1 should be unhealthy after CloseAll")
	}
	if dl2.IsHealthy() {
		t.Error("dl-2 should be unhealthy after CloseAll")
	}
}

// StatefulMockDownloader 带状态的模拟下载器，用于测试独立性
type StatefulMockDownloader struct {
	name       string
	dlType     DownloaderType
	healthy    bool
	torrentMap map[string]bool // 存储已添加的种子
}

func (m *StatefulMockDownloader) Authenticate() error                    { return nil }
func (m *StatefulMockDownloader) Ping() (bool, error)                    { return m.healthy, nil }
func (m *StatefulMockDownloader) GetClientVersion() (string, error)      { return "1.0.0", nil }
func (m *StatefulMockDownloader) GetClientStatus() (ClientStatus, error) { return ClientStatus{}, nil }
func (m *StatefulMockDownloader) GetClientFreeSpace(ctx context.Context) (int64, error) {
	return 1024 * 1024 * 1024, nil
}
func (m *StatefulMockDownloader) GetAllTorrents() ([]Torrent, error) { return nil, nil }
func (m *StatefulMockDownloader) GetTorrentsBy(filter TorrentFilter) ([]Torrent, error) {
	return nil, nil
}
func (m *StatefulMockDownloader) GetTorrent(id string) (Torrent, error) { return Torrent{}, nil }
func (m *StatefulMockDownloader) AddTorrentEx(url string, opt AddTorrentOptions) (AddTorrentResult, error) {
	return AddTorrentResult{Success: true}, nil
}

func (m *StatefulMockDownloader) AddTorrentFileEx(fileData []byte, opt AddTorrentOptions) (AddTorrentResult, error) {
	hash := string(fileData)
	m.torrentMap[hash] = true
	return AddTorrentResult{Success: true, Hash: hash}, nil
}
func (m *StatefulMockDownloader) PauseTorrent(id string) error                   { return nil }
func (m *StatefulMockDownloader) ResumeTorrent(id string) error                  { return nil }
func (m *StatefulMockDownloader) RemoveTorrent(id string, removeData bool) error { return nil }
func (m *StatefulMockDownloader) PauseTorrents(ids []string) error               { return nil }
func (m *StatefulMockDownloader) ResumeTorrents(ids []string) error              { return nil }
func (m *StatefulMockDownloader) RemoveTorrents(ids []string, removeData bool) error {
	return nil
}
func (m *StatefulMockDownloader) SetTorrentCategory(id, category string) error     { return nil }
func (m *StatefulMockDownloader) SetTorrentTags(id, tags string) error             { return nil }
func (m *StatefulMockDownloader) SetTorrentSavePath(id, path string) error         { return nil }
func (m *StatefulMockDownloader) RecheckTorrent(id string) error                   { return nil }
func (m *StatefulMockDownloader) GetTorrentFiles(id string) ([]TorrentFile, error) { return nil, nil }
func (m *StatefulMockDownloader) GetTorrentTrackers(id string) ([]TorrentTracker, error) {
	return nil, nil
}
func (m *StatefulMockDownloader) GetDiskInfo() (DiskInfo, error)       { return DiskInfo{}, nil }
func (m *StatefulMockDownloader) GetSpeedLimit() (SpeedLimit, error)   { return SpeedLimit{}, nil }
func (m *StatefulMockDownloader) SetSpeedLimit(limit SpeedLimit) error { return nil }
func (m *StatefulMockDownloader) GetClientPaths() ([]string, error)    { return nil, nil }
func (m *StatefulMockDownloader) GetClientLabels() ([]string, error)   { return nil, nil }
func (m *StatefulMockDownloader) AddTorrent(fileData []byte, category, tags string) error {
	hash := string(fileData) // 简化：使用数据作为hash
	m.torrentMap[hash] = true
	return nil
}

func (m *StatefulMockDownloader) AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error {
	hash := string(fileData) // 简化：使用数据作为hash
	m.torrentMap[hash] = true
	return nil
}

func (m *StatefulMockDownloader) CheckTorrentExists(torrentHash string) (bool, error) {
	return m.torrentMap[torrentHash], nil
}

func (m *StatefulMockDownloader) GetDiskSpace(ctx context.Context) (int64, error) {
	return 1024 * 1024 * 1024, nil
}

func (m *StatefulMockDownloader) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	return true, nil
}

func (m *StatefulMockDownloader) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	return nil
}
func (m *StatefulMockDownloader) GetType() DownloaderType { return m.dlType }
func (m *StatefulMockDownloader) GetName() string         { return m.name }
func (m *StatefulMockDownloader) IsHealthy() bool         { return m.healthy }
func (m *StatefulMockDownloader) Close() error            { m.healthy = false; return nil }
func (m *StatefulMockDownloader) GetTorrentCount() int    { return len(m.torrentMap) }

// StatefulMockDownloaderFactory 创建带状态的模拟下载器
func StatefulMockDownloaderFactory(config DownloaderConfig, name string) (Downloader, error) {
	return &StatefulMockDownloader{
		name:       name,
		dlType:     config.GetType(),
		healthy:    true,
		torrentMap: make(map[string]bool),
	}, nil
}

// TestProperty11_MultipleDownloaderIndependence 属性测试：多下载器独立性
// Feature: downloader-site-extensibility, Property 11: Multiple Downloader Independence
// 验证每个下载器维护独立状态
func TestProperty11_MultipleDownloaderIndependence(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// Property 11.1: 向一个下载器添加种子不影响其他下载器
	properties.Property("adding torrent to one downloader does not affect others", prop.ForAll(
		func(torrentData string) bool {
			if torrentData == "" {
				return true
			}

			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, StatefulMockDownloaderFactory)
			dm.RegisterFactory(DownloaderTransmission, StatefulMockDownloaderFactory)

			// 注册两个下载器
			dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, true)
			dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderTransmission, URL: "http://2"}, false)

			// 获取两个下载器
			dl1, err := dm.GetDownloader("dl-1")
			if err != nil {
				return false
			}
			dl2, err := dm.GetDownloader("dl-2")
			if err != nil {
				return false
			}

			// 向dl-1添加种子
			err = dl1.AddTorrent([]byte(torrentData), "test", "")
			if err != nil {
				return false
			}

			// 验证dl-1有种子
			exists1, _ := dl1.CheckTorrentExists(torrentData)
			if !exists1 {
				return false
			}

			// 验证dl-2没有种子
			exists2, _ := dl2.CheckTorrentExists(torrentData)
			if exists2 {
				return false // dl-2不应该有这个种子
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property 11.2: 每个下载器的健康状态独立
	properties.Property("each downloader has independent health status", prop.ForAll(
		func(closeFirst bool) bool {
			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, StatefulMockDownloaderFactory)
			dm.RegisterFactory(DownloaderTransmission, StatefulMockDownloaderFactory)

			dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, true)
			dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderTransmission, URL: "http://2"}, false)

			dl1, _ := dm.GetDownloader("dl-1")
			dl2, _ := dm.GetDownloader("dl-2")

			// 初始状态都健康
			if !dl1.IsHealthy() || !dl2.IsHealthy() {
				return false
			}

			// 关闭其中一个
			if closeFirst {
				dl1.Close()
				// dl-1不健康，dl-2仍然健康
				return !dl1.IsHealthy() && dl2.IsHealthy()
			} else {
				dl2.Close()
				// dl-2不健康，dl-1仍然健康
				return dl1.IsHealthy() && !dl2.IsHealthy()
			}
		},
		gen.Bool(),
	))

	// Property 11.3: 多个下载器可以同时添加不同种子
	properties.Property("multiple downloaders can add different torrents simultaneously", prop.ForAll(
		func(torrent1, torrent2 string) bool {
			if torrent1 == "" || torrent2 == "" || torrent1 == torrent2 {
				return true
			}

			dm := NewDownloaderManager()
			dm.RegisterFactory(DownloaderQBittorrent, StatefulMockDownloaderFactory)
			dm.RegisterFactory(DownloaderTransmission, StatefulMockDownloaderFactory)

			dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, true)
			dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderTransmission, URL: "http://2"}, false)

			dl1, _ := dm.GetDownloader("dl-1")
			dl2, _ := dm.GetDownloader("dl-2")

			// 向不同下载器添加不同种子
			dl1.AddTorrent([]byte(torrent1), "cat1", "")
			dl2.AddTorrent([]byte(torrent2), "cat2", "")

			// 验证各自只有自己的种子
			exists1in1, _ := dl1.CheckTorrentExists(torrent1)
			exists2in1, _ := dl1.CheckTorrentExists(torrent2)
			exists1in2, _ := dl2.CheckTorrentExists(torrent1)
			exists2in2, _ := dl2.CheckTorrentExists(torrent2)

			return exists1in1 && !exists2in1 && !exists1in2 && exists2in2
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// TestDownloaderManagerReconnect 测试重连功能
func TestDownloaderManagerReconnect(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	// 获取下载器
	dl, err := dm.GetDownloader("test-dl")
	if err != nil {
		t.Fatalf("failed to get downloader: %v", err)
	}

	// 关闭下载器
	dl.Close()
	if dl.IsHealthy() {
		t.Error("downloader should be unhealthy after close")
	}

	// 重连
	err = dm.ReconnectDownloader("test-dl")
	if err != nil {
		t.Fatalf("failed to reconnect: %v", err)
	}

	// 获取新实例
	newDl, err := dm.GetDownloader("test-dl")
	if err != nil {
		t.Fatalf("failed to get downloader after reconnect: %v", err)
	}

	if !newDl.IsHealthy() {
		t.Error("new downloader should be healthy")
	}
}

// TestDownloaderManagerBackoff 测试指数退避计算
func TestDownloaderManagerBackoff(t *testing.T) {
	dm := NewDownloaderManager()

	// 测试退避时间计算
	backoff0 := dm.CalculateBackoff(0)
	if backoff0 != time.Second {
		t.Errorf("expected 1s for attempt 0, got %v", backoff0)
	}

	backoff1 := dm.CalculateBackoff(1)
	if backoff1 != 2*time.Second {
		t.Errorf("expected 2s for attempt 1, got %v", backoff1)
	}

	backoff2 := dm.CalculateBackoff(2)
	if backoff2 != 4*time.Second {
		t.Errorf("expected 4s for attempt 2, got %v", backoff2)
	}

	// 测试最大退避时间
	backoff10 := dm.CalculateBackoff(10)
	if backoff10 > 30*time.Second {
		t.Errorf("backoff should not exceed max, got %v", backoff10)
	}
}

// TestDownloaderManagerStatus 测试获取所有下载器状态
func TestDownloaderManagerStatus(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	dm.RegisterConfig("dl-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://1"}, true)
	dm.RegisterConfig("dl-2", &MockConfig{Type: DownloaderTransmission, URL: "http://2"}, false)

	// 创建实例
	dm.GetDownloader("dl-1")
	dm.GetDownloader("dl-2")

	// 获取状态
	statuses := dm.GetAllDownloaderStatus()
	if len(statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(statuses))
	}

	// 验证状态内容
	statusMap := make(map[string]DownloaderStatus)
	for _, s := range statuses {
		statusMap[s.Name] = s
	}

	if s, ok := statusMap["dl-1"]; ok {
		if !s.IsDefault {
			t.Error("dl-1 should be default")
		}
		if !s.IsHealthy {
			t.Error("dl-1 should be healthy")
		}
		if s.Type != string(DownloaderQBittorrent) {
			t.Errorf("dl-1 type should be qbittorrent, got %s", s.Type)
		}
	} else {
		t.Error("dl-1 not found in statuses")
	}

	if s, ok := statusMap["dl-2"]; ok {
		if s.IsDefault {
			t.Error("dl-2 should not be default")
		}
		if s.Type != string(DownloaderTransmission) {
			t.Errorf("dl-2 type should be transmission, got %s", s.Type)
		}
	} else {
		t.Error("dl-2 not found in statuses")
	}
}

// TestNewDownloaderManagerWithConfig 测试使用配置创建下载器管理器
func TestNewDownloaderManagerWithConfig(t *testing.T) {
	reconnectConfig := ReconnectConfig{
		MaxRetries:     5,
		InitialBackoff: 2 * time.Second,
		MaxBackoff:     60 * time.Second,
	}

	dm := NewDownloaderManagerWithConfig(reconnectConfig)
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	// 注册配置
	dm.RegisterConfig("qbit-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://qbit:8080"}, true)
	dm.RegisterConfig("trans-1", &MockConfig{Type: DownloaderTransmission, URL: "http://trans:9091"}, false)

	// 验证配置已注册
	names := dm.ListDownloaders()
	if len(names) != 2 {
		t.Errorf("expected 2 downloaders, got %d", len(names))
	}

	// 验证默认下载器
	dl, err := dm.GetDownloaderForSite("any-site")
	if err != nil {
		t.Fatalf("failed to get default downloader: %v", err)
	}
	if dl.GetName() != "qbit-1" {
		t.Errorf("expected default downloader 'qbit-1', got '%s'", dl.GetName())
	}
}

// TestGetDefaultDownloader 测试获取默认下载器
func TestGetDefaultDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	// 没有默认下载器时应该返回错误
	_, err := dm.GetDefaultDownloader()
	if err == nil {
		t.Error("expected error when no default downloader")
	}

	// 注册默认下载器
	dm.RegisterConfig("default-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://default"}, true)

	// 现在应该能获取默认下载器
	dl, err := dm.GetDefaultDownloader()
	if err != nil {
		t.Fatalf("failed to get default downloader: %v", err)
	}
	if dl.GetName() != "default-dl" {
		t.Errorf("expected 'default-dl', got '%s'", dl.GetName())
	}
}

// TestGetDownloaderHealth 测试获取下载器健康状态
func TestGetDownloaderHealth(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	// 不存在的下载器
	_, err := dm.GetDownloaderHealth("non-existent")
	if err == nil {
		t.Error("expected error for non-existent downloader")
	}

	// 注册并获取下载器
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)
	dm.GetDownloader("test-dl") // 创建实例

	// 获取健康状态
	healthy, err := dm.GetDownloaderHealth("test-dl")
	if err != nil {
		t.Fatalf("failed to get health: %v", err)
	}
	if !healthy {
		t.Error("expected downloader to be healthy")
	}
}

// TestGetErrorCount 测试获取错误计数
func TestGetErrorCount(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	// 不存在的下载器
	count := dm.GetErrorCount("non-existent")
	if count != 0 {
		t.Errorf("expected 0 for non-existent downloader, got %d", count)
	}

	// 注册下载器
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	// 初始错误计数应该为0
	count = dm.GetErrorCount("test-dl")
	if count != 0 {
		t.Errorf("expected 0 error count, got %d", count)
	}
}

// TestRegisterConfigInvalidType 测试注册无效类型的配置
func TestRegisterConfigInvalidType(t *testing.T) {
	dm := NewDownloaderManager()
	// 不注册任何工厂

	config := &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}
	err := dm.RegisterConfig("test-dl", config, true)
	// 应该成功注册配置，但获取时会失败
	if err != nil {
		t.Fatalf("RegisterConfig should succeed: %v", err)
	}

	// 获取时应该失败因为没有工厂
	_, err = dm.GetDownloader("test-dl")
	if err == nil {
		t.Error("expected error when no factory registered")
	}
}

// FailingMockDownloaderFactory 创建会失败的模拟下载器工厂
var failCount int

func FailingMockDownloaderFactory(config DownloaderConfig, name string) (Downloader, error) {
	failCount++
	if failCount <= 2 {
		return nil, ErrConnectionFailed
	}
	return &MockDownloader{
		name:    name,
		dlType:  config.GetType(),
		healthy: true,
	}, nil
}

// TestCreateWithRetry_Success 测试重试成功
func TestCreateWithRetry_Success(t *testing.T) {
	failCount = 0 // 重置计数器

	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     3,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, FailingMockDownloaderFactory)
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	// 前两次失败，第三次成功
	dl, err := dm.GetDownloader("test-dl")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if dl.GetName() != "test-dl" {
		t.Errorf("expected name 'test-dl', got '%s'", dl.GetName())
	}
}

// AlwaysFailingFactory 总是失败的工厂
func AlwaysFailingFactory(config DownloaderConfig, name string) (Downloader, error) {
	return nil, ErrConnectionFailed
}

// TestCreateWithRetry_AllFail 测试所有重试都失败
func TestCreateWithRetry_AllFail(t *testing.T) {
	dm := NewDownloaderManagerWithConfig(ReconnectConfig{
		MaxRetries:     2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     10 * time.Millisecond,
		Multiplier:     2.0,
	})
	dm.RegisterFactory(DownloaderQBittorrent, AlwaysFailingFactory)
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	_, err := dm.GetDownloader("test-dl")
	if err == nil {
		t.Error("expected error after all retries failed")
	}
}

// TestReconnectDownloader_NoConfig 测试重连不存在的下载器
func TestReconnectDownloader_NoConfig(t *testing.T) {
	dm := NewDownloaderManager()

	err := dm.ReconnectDownloader("non-existent")
	if err == nil {
		t.Error("expected error for non-existent downloader")
	}
}

// TestReconnectDownloader_NoFactory 测试重连没有工厂的下载器
func TestReconnectDownloader_NoFactory(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	err := dm.ReconnectDownloader("test-dl")
	if err == nil {
		t.Error("expected error when no factory registered")
	}
}

// TestGetDownloader_UnhealthyRecreate 测试不健康的下载器重新创建
func TestGetDownloader_UnhealthyRecreate(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterConfig("test-dl", &MockConfig{Type: DownloaderQBittorrent, URL: "http://test"}, true)

	// 获取下载器
	dl, err := dm.GetDownloader("test-dl")
	if err != nil {
		t.Fatalf("failed to get downloader: %v", err)
	}

	// 关闭使其不健康
	dl.Close()

	// 再次获取应该重新创建
	newDl, err := dm.GetDownloader("test-dl")
	if err != nil {
		t.Fatalf("failed to get downloader after unhealthy: %v", err)
	}
	if !newDl.IsHealthy() {
		t.Error("new downloader should be healthy")
	}
}

// TestGetDownloader_NoConfig 测试获取不存在配置的下载器
func TestGetDownloader_NoConfig(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	_, err := dm.GetDownloader("non-existent")
	if err == nil {
		t.Error("expected error for non-existent config")
	}
}

// TestRegisterConfig_InvalidConfig 测试注册无效配置
func TestRegisterConfig_InvalidConfig(t *testing.T) {
	dm := NewDownloaderManager()

	// 空 URL 的配置应该验证失败
	config := &MockConfig{Type: DownloaderQBittorrent, URL: ""}
	err := dm.RegisterConfig("test-dl", config, true)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestSyncFromDB_AddNewDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	records := []DownloaderDBRecord{
		{Name: "qbit-1", Type: DownloaderQBittorrent, URL: "http://localhost:8080", Enabled: true, IsDefault: true},
	}

	dm.SyncFromDB(records)

	if len(dm.configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(dm.configs))
	}
	if dm.defaultName != "qbit-1" {
		t.Errorf("expected default 'qbit-1', got '%s'", dm.defaultName)
	}
}

func TestSyncFromDB_RemoveDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("qbit-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://localhost:8080"}, true)
	_, _ = dm.GetDownloader("qbit-1")

	if len(dm.downloaders) != 1 {
		t.Fatalf("expected 1 instance before sync")
	}

	dm.SyncFromDB([]DownloaderDBRecord{})

	if len(dm.configs) != 0 {
		t.Errorf("expected 0 configs after sync, got %d", len(dm.configs))
	}
	if len(dm.downloaders) != 0 {
		t.Errorf("expected 0 instances after sync, got %d", len(dm.downloaders))
	}
}

func TestSyncFromDB_UpdateDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("qbit-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://old:8080"}, true)
	dl, _ := dm.GetDownloader("qbit-1")

	if !dl.IsHealthy() {
		t.Fatal("downloader should be healthy before update")
	}

	records := []DownloaderDBRecord{
		{Name: "qbit-1", Type: DownloaderQBittorrent, URL: "http://new:8080", Enabled: true, IsDefault: true},
	}
	dm.SyncFromDB(records)

	if len(dm.downloaders) != 0 {
		t.Errorf("expected old instance to be closed and removed, got %d", len(dm.downloaders))
	}

	newDl, err := dm.GetDownloader("qbit-1")
	if err != nil {
		t.Fatalf("failed to get downloader after sync: %v", err)
	}
	if newDl == dl {
		t.Error("expected new instance after config change")
	}
}

func TestSyncFromDB_ChangeDefault(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)
	dm.RegisterFactory(DownloaderTransmission, MockDownloaderFactory)

	records := []DownloaderDBRecord{
		{Name: "qbit-1", Type: DownloaderQBittorrent, URL: "http://qbit:8080", Enabled: true, IsDefault: true},
		{Name: "trans-1", Type: DownloaderTransmission, URL: "http://trans:9091", Enabled: true, IsDefault: false},
	}
	dm.SyncFromDB(records)

	if dm.defaultName != "qbit-1" {
		t.Errorf("expected default 'qbit-1', got '%s'", dm.defaultName)
	}

	records[0].IsDefault = false
	records[1].IsDefault = true
	dm.SyncFromDB(records)

	if dm.defaultName != "trans-1" {
		t.Errorf("expected default 'trans-1' after change, got '%s'", dm.defaultName)
	}
}

func TestSyncFromDB_DisabledDownloader(t *testing.T) {
	dm := NewDownloaderManager()
	dm.RegisterFactory(DownloaderQBittorrent, MockDownloaderFactory)

	dm.RegisterConfig("qbit-1", &MockConfig{Type: DownloaderQBittorrent, URL: "http://localhost:8080"}, true)
	_, _ = dm.GetDownloader("qbit-1")

	records := []DownloaderDBRecord{
		{Name: "qbit-1", Type: DownloaderQBittorrent, URL: "http://localhost:8080", Enabled: false, IsDefault: true},
	}
	dm.SyncFromDB(records)

	if len(dm.configs) != 0 {
		t.Errorf("disabled downloader should be removed, got %d configs", len(dm.configs))
	}
}
