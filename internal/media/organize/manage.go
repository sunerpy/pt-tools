package organize

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 数量上限。
const (
	maxLibraries = 50
	maxPathMaps  = 200
	maxServers   = 20
)

// LibraryView 是给界面看的媒体库：带上实际用的模板与示例渲染的结果。
type LibraryView struct {
	models.MediaLibrary
	EffectiveTemplate string `json:"effective_template"`
	Preview           string `json:"preview"`
}

// LibraryInput 是新建或修改媒体库的请求。
type LibraryInput struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Anime           bool   `json:"anime"`
	Path            string `json:"path"`
	Template        string `json:"template"`
	Mode            string `json:"mode"`
	Scrape          bool   `json:"scrape"`
	ScrapeOverwrite bool   `json:"scrape_overwrite"`
	Enabled         bool   `json:"enabled"`
}

// DefaultTemplate 是这种媒体库的默认模板。
func DefaultTemplate(kind string) string {
	if kind == models.MediaKindTV {
		return transfer.DefaultTVTemplate
	}
	return transfer.DefaultMovieTemplate
}

func effectiveTemplate(l models.MediaLibrary) string {
	if strings.TrimSpace(l.Template) != "" {
		return l.Template
	}
	return DefaultTemplate(l.Kind)
}

func viewLibrary(l models.MediaLibrary) LibraryView {
	v := LibraryView{MediaLibrary: l, EffectiveTemplate: effectiveTemplate(l)}
	v.Preview, _ = transfer.Check(v.EffectiveTemplate, l.Kind)
	return v
}

// isAbs 报告路径是不是绝对路径（Unix 的 /…，或 Windows 的盘符与 \\ 开头）。
func isAbs(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

func cleanDir(p, what string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !isAbs(p) {
		return "", fmt.Errorf("%w: %s要填绝对路径", ErrInvalid, what)
	}
	if len(p) > 1024 || !utf8.ValidString(p) {
		return "", fmt.Errorf("%w: %s太长", ErrInvalid, what)
	}
	c := filepath.Clean(p)
	if c == "/" || c == `\` || (len(c) <= 3 && len(c) >= 2 && c[1] == ':') {
		return "", fmt.Errorf("%w: %s不能是根目录", ErrInvalid, what)
	}
	return c, nil
}

func (in LibraryInput) clean() (models.MediaLibrary, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return models.MediaLibrary{}, fmt.Errorf("%w: 名称不能为空，最多 64 个字", ErrInvalid)
	}
	if in.Kind != models.MediaKindMovie && in.Kind != models.MediaKindTV {
		return models.MediaLibrary{}, fmt.Errorf("%w: 类型要选电影（movie）或剧集（tv）", ErrInvalid)
	}
	path, err := cleanDir(in.Path, "库目录")
	if err != nil {
		return models.MediaLibrary{}, err
	}
	mode, err := transfer.NormalizeMode(in.Mode)
	if err != nil {
		return models.MediaLibrary{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	tpl := strings.TrimSpace(in.Template)
	if utf8.RuneCountInString(tpl) > 1024 {
		return models.MediaLibrary{}, fmt.Errorf("%w: 模板最多 1024 个字", ErrInvalid)
	}
	if tpl != "" {
		if _, err := transfer.Check(tpl, in.Kind); err != nil {
			return models.MediaLibrary{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	return models.MediaLibrary{
		Name: name, Kind: in.Kind, Anime: in.Anime, Path: path, Template: tpl, Mode: mode,
		Scrape: in.Scrape, ScrapeOverwrite: in.ScrapeOverwrite && in.Scrape, Enabled: in.Enabled,
	}, nil
}

// Libraries 列出媒体库。
func (s *Service) Libraries(ctx context.Context) ([]LibraryView, error) {
	var rows []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	out := make([]LibraryView, 0, len(rows))
	for _, r := range rows {
		out = append(out, viewLibrary(r))
	}
	return out, nil
}

func isUnique(err error) bool {
	return err != nil && (errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE constraint failed"))
}

// SaveLibrary 新建（id 为 0）或修改媒体库。
func (s *Service) SaveLibrary(ctx context.Context, id uint, in LibraryInput) (LibraryView, error) {
	row, err := in.clean()
	if err != nil {
		return LibraryView{}, err
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	if id == 0 {
		var n int64
		if err := db.Model(&models.MediaLibrary{}).Count(&n).Error; err != nil {
			return LibraryView{}, fmt.Errorf("读取媒体库失败: %w", err)
		}
		if n >= maxLibraries {
			return LibraryView{}, fmt.Errorf("%w: 最多 %d 个媒体库", ErrInvalid, maxLibraries)
		}
		if err := db.Create(&row).Error; err != nil {
			if isUnique(err) {
				return LibraryView{}, fmt.Errorf("%w: 已经有叫「%s」的媒体库", ErrInvalid, row.Name)
			}
			return LibraryView{}, fmt.Errorf("保存媒体库失败: %w", err)
		}
		return viewLibrary(row), nil
	}
	res := db.Model(&models.MediaLibrary{}).Where("id = ?", id).Select(
		"name", "kind", "anime", "path", "template", "mode", "scrape", "scrape_overwrite", "enabled", "updated_at",
	).Updates(&row)
	if res.Error != nil {
		if isUnique(res.Error) {
			return LibraryView{}, fmt.Errorf("%w: 已经有叫「%s」的媒体库", ErrInvalid, row.Name)
		}
		return LibraryView{}, fmt.Errorf("保存媒体库失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return LibraryView{}, ErrNotFound
	}
	var out models.MediaLibrary
	if err := db.First(&out, id).Error; err != nil {
		return LibraryView{}, fmt.Errorf("读取媒体库失败: %w", err)
	}
	return viewLibrary(out), nil
}

// DeleteLibrary 删除媒体库（库里的文件与整理记录不动）。
func (s *Service) DeleteLibrary(ctx context.Context, id uint) error {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaLibrary{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除媒体库失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckItem 是一项检查的结果。
type CheckItem struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// LibraryCheckInput 是检查库目录的请求。
type LibraryCheckInput struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

func probeName() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return ".pt-tools-probe-" + hex.EncodeToString(b[:])
}

// CheckLibrary 检查库目录：存在、可写；硬链接时用每条路径映射的 pt-tools 路径试建一个硬链接，
// 确认下载目录与库在同一个文件系统。检查完删掉试建的文件。
func (s *Service) CheckLibrary(ctx context.Context, in LibraryCheckInput) ([]CheckItem, error) {
	dir, err := cleanDir(in.Path, "库目录")
	if err != nil {
		return nil, err
	}
	mode, err := transfer.NormalizeMode(in.Mode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var items []CheckItem
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return append(items, CheckItem{Name: "库目录存在", Message: fmt.Sprintf("pt-tools 里找不到 %s（Docker 里要把媒体库挂进容器）", dir)}), nil
	case !info.IsDir():
		return append(items, CheckItem{Name: "库目录存在", Message: dir + " 不是目录"}), nil
	}
	items = append(items, CheckItem{Name: "库目录存在", OK: true})
	probe := filepath.Join(dir, probeName())
	if err := os.WriteFile(probe, []byte("pt-tools"), 0o644); err != nil {
		return append(items, CheckItem{Name: "库目录可写", Message: err.Error()}), nil
	}
	defer os.Remove(probe)
	items = append(items, CheckItem{Name: "库目录可写", OK: true})
	if mode != models.MediaModeHardlink {
		return items, nil
	}
	var maps []models.MediaPathMap
	if err := s.cfg.DB.WithContext(ctx).Order("downloader_id, id").Find(&maps).Error; err != nil {
		return nil, fmt.Errorf("读取路径映射失败: %w", err)
	}
	if len(maps) == 0 {
		return append(items, CheckItem{
			Name: "硬链接", OK: true,
			Message: "没有路径映射，没有可试的下载目录：下载目录与 pt-tools 看到的路径相同时，也可以加一条两边相同的映射来检查",
		}), nil
	}
	seen := map[string]bool{}
	for _, m := range maps {
		if seen[m.LocalPrefix] {
			continue
		}
		seen[m.LocalPrefix] = true
		items = append(items, probeLink(m.LocalPrefix, dir))
	}
	return items, nil
}

// probeLink 在 from 里建一个文件，再硬链接到 to 里，报告能不能建。
func probeLink(from, to string) CheckItem {
	item := CheckItem{Name: "硬链接：" + from}
	src := filepath.Join(from, probeName())
	if err := os.WriteFile(src, []byte("pt-tools"), 0o644); err != nil {
		item.Message = fmt.Sprintf("在下载目录里建文件失败（pt-tools 要能写下载目录才能试）: %v", err)
		return item
	}
	defer os.Remove(src)
	dst := filepath.Join(to, probeName())
	_, err := transfer.Transfer(src, dst, models.MediaModeHardlink)
	_ = os.Remove(dst)
	if err != nil {
		item.Message = err.Error()
		return item
	}
	item.OK = true
	return item
}

// PathMapInput 是新建或修改路径映射的请求。
type PathMapInput struct {
	DownloaderID     uint   `json:"downloader_id"`
	DownloaderPrefix string `json:"downloader_prefix"`
	LocalPrefix      string `json:"local_prefix"`
}

// PathMaps 列出路径映射。
func (s *Service) PathMaps(ctx context.Context) ([]models.MediaPathMap, error) {
	var rows []models.MediaPathMap
	if err := s.cfg.DB.WithContext(ctx).Order("downloader_id, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取路径映射失败: %w", err)
	}
	return rows, nil
}

// SavePathMap 新建（id 为 0）或修改路径映射。
func (s *Service) SavePathMap(ctx context.Context, id uint, in PathMapInput) (models.MediaPathMap, error) {
	prefix := strings.TrimSpace(in.DownloaderPrefix)
	if prefix == "" || len(prefix) > 512 {
		return models.MediaPathMap{}, fmt.Errorf("%w: 下载器里的路径不能为空，最多 512 个字符", ErrInvalid)
	}
	local, err := cleanDir(in.LocalPrefix, "pt-tools 里的路径")
	if err != nil {
		return models.MediaPathMap{}, err
	}
	if len(local) > 512 {
		return models.MediaPathMap{}, fmt.Errorf("%w: pt-tools 里的路径最多 512 个字符", ErrInvalid)
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	var dl int64
	if cntErr := db.Model(&models.DownloaderSetting{}).Where("id = ?", in.DownloaderID).Count(&dl).Error; cntErr != nil {
		return models.MediaPathMap{}, fmt.Errorf("读取下载器失败: %w", cntErr)
	}
	if dl == 0 {
		return models.MediaPathMap{}, fmt.Errorf("%w: 下载器不存在", ErrInvalid)
	}
	row := models.MediaPathMap{DownloaderID: in.DownloaderID, DownloaderPrefix: prefix, LocalPrefix: local}
	if id == 0 {
		var n int64
		if cntErr := db.Model(&models.MediaPathMap{}).Count(&n).Error; cntErr != nil {
			return models.MediaPathMap{}, fmt.Errorf("读取路径映射失败: %w", cntErr)
		}
		if n >= maxPathMaps {
			return models.MediaPathMap{}, fmt.Errorf("%w: 最多 %d 条路径映射", ErrInvalid, maxPathMaps)
		}
		err = db.Create(&row).Error
	} else {
		res := db.Model(&models.MediaPathMap{}).Where("id = ?", id).
			Select("downloader_id", "downloader_prefix", "local_prefix", "updated_at").Updates(&row)
		err = res.Error
		if err == nil && res.RowsAffected == 0 {
			return models.MediaPathMap{}, ErrNotFound
		}
		row.ID = id
	}
	if isUnique(err) {
		return models.MediaPathMap{}, fmt.Errorf("%w: 这个下载器已经有「%s」的映射", ErrInvalid, prefix)
	}
	if err != nil {
		return models.MediaPathMap{}, fmt.Errorf("保存路径映射失败: %w", err)
	}
	var out models.MediaPathMap
	if err := db.First(&out, row.ID).Error; err != nil {
		return models.MediaPathMap{}, fmt.Errorf("读取路径映射失败: %w", err)
	}
	return out, nil
}

// DeletePathMap 删除路径映射。
func (s *Service) DeletePathMap(ctx context.Context, id uint) error {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaPathMap{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除路径映射失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// localPath 把下载器里的路径换成 pt-tools 里的路径；没有匹配的映射时原样返回。
func (s *Service) localPath(ctx context.Context, dlID uint, p string) (string, bool, error) {
	var rows []models.MediaPathMap
	if err := s.cfg.DB.WithContext(ctx).Where("downloader_id = ?", dlID).Find(&rows).Error; err != nil {
		return "", false, fmt.Errorf("读取路径映射失败: %w", err)
	}
	maps := make([]models.DownloaderPathMap, 0, len(rows))
	for _, r := range rows {
		maps = append(maps, models.DownloaderPathMap{SourcePrefix: r.DownloaderPrefix, TargetPrefix: r.LocalPrefix})
	}
	out, ok := models.MapTransferPath(maps, p)
	return out, ok, nil
}

// ServerView 是给界面看的媒体服务器：Token 只说有没有。
type ServerView struct {
	models.MediaServer
	HasToken bool `json:"has_token"`
}

// ServerInput 是新建或修改媒体服务器的请求。Token 为空（nil）时保留原来的。
type ServerInput struct {
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	URL          string  `json:"url"`
	Token        *string `json:"token,omitempty"`
	Enabled      bool    `json:"enabled"`
	RefreshMode  string  `json:"refresh_mode"`
	LocalPrefix  string  `json:"local_prefix"`
	ServerPrefix string  `json:"server_prefix"`
}

func viewServer(r models.MediaServer) ServerView {
	return ServerView{MediaServer: r, HasToken: r.TokenEncrypted != ""}
}

func (in ServerInput) clean() (models.MediaServer, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return models.MediaServer{}, fmt.Errorf("%w: 名称不能为空，最多 64 个字", ErrInvalid)
	}
	switch in.Kind {
	case models.MediaServerEmby, models.MediaServerJellyfin, models.MediaServerPlex:
	default:
		return models.MediaServer{}, fmt.Errorf("%w: 种类要选 emby、jellyfin 或 plex", ErrInvalid)
	}
	u, err := server.CheckURL(in.URL)
	if err != nil {
		return models.MediaServer{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	mode := in.RefreshMode
	switch mode {
	case "":
		mode = models.MediaRefreshPath
	case models.MediaRefreshPath, models.MediaRefreshLibrary:
	default:
		return models.MediaServer{}, fmt.Errorf("%w: 刷新方式要选 path 或 library", ErrInvalid)
	}
	local, remote := strings.TrimSpace(in.LocalPrefix), strings.TrimSpace(in.ServerPrefix)
	if (local == "") != (remote == "") || len(local) > 512 || len(remote) > 512 {
		return models.MediaServer{}, fmt.Errorf("%w: 路径映射的两边要一起填（两边相同时都留空）", ErrInvalid)
	}
	return models.MediaServer{
		Name: name, Kind: in.Kind, URL: u, Enabled: in.Enabled, RefreshMode: mode, LocalPrefix: local, ServerPrefix: remote,
	}, nil
}

// Servers 列出媒体服务器。
func (s *Service) Servers(ctx context.Context) ([]ServerView, error) {
	var rows []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	out := make([]ServerView, 0, len(rows))
	for _, r := range rows {
		out = append(out, viewServer(r))
	}
	return out, nil
}

// SaveServer 新建（id 为 0）或修改媒体服务器。Token 加密保存；新建时必须填。
func (s *Service) SaveServer(ctx context.Context, id uint, in ServerInput) (ServerView, error) {
	row, err := in.clean()
	if err != nil {
		return ServerView{}, err
	}
	cols := []string{"name", "kind", "url", "enabled", "refresh_mode", "local_prefix", "server_prefix", "updated_at"}
	if in.Token != nil {
		tok := strings.TrimSpace(*in.Token)
		if tok == "" {
			return ServerView{}, fmt.Errorf("%w: API Key 或 Token 不能为空", ErrInvalid)
		}
		if _, cfgErr := server.New(server.Config{Kind: row.Kind, URL: row.URL, Token: tok}); cfgErr != nil {
			return ServerView{}, fmt.Errorf("%w: %w", ErrInvalid, cfgErr)
		}
		if row.TokenEncrypted, err = s.cfg.Cipher.Encrypt(tok); err != nil {
			return ServerView{}, fmt.Errorf("加密 Token 失败: %w", err)
		}
		cols = append(cols, "token_encrypted")
	} else if id == 0 {
		return ServerView{}, fmt.Errorf("%w: 新建时要填 API Key 或 Token", ErrInvalid)
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	db := s.cfg.DB.WithContext(ctx)
	if id == 0 {
		var n int64
		if cntErr := db.Model(&models.MediaServer{}).Count(&n).Error; cntErr != nil {
			return ServerView{}, fmt.Errorf("读取媒体服务器失败: %w", cntErr)
		}
		if n >= maxServers {
			return ServerView{}, fmt.Errorf("%w: 最多 %d 个媒体服务器", ErrInvalid, maxServers)
		}
		err = db.Create(&row).Error
	} else {
		res := db.Model(&models.MediaServer{}).Where("id = ?", id).Select(cols).Updates(&row)
		err = res.Error
		if err == nil && res.RowsAffected == 0 {
			return ServerView{}, ErrNotFound
		}
		row.ID = id
	}
	if isUnique(err) {
		return ServerView{}, fmt.Errorf("%w: 已经有叫「%s」的媒体服务器", ErrInvalid, row.Name)
	}
	if err != nil {
		return ServerView{}, fmt.Errorf("保存媒体服务器失败: %w", err)
	}
	var out models.MediaServer
	if err := db.First(&out, row.ID).Error; err != nil {
		return ServerView{}, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	return viewServer(out), nil
}

// DeleteServer 删除媒体服务器。
func (s *Service) DeleteServer(ctx context.Context, id uint) error {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaServer{}, id)
	if res.Error != nil {
		return fmt.Errorf("删除媒体服务器失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ServerTestInput 是测试媒体服务器的请求：ID 不为 0 且没填 Token 时用保存的 Token（可以先改地址再测）。
type ServerTestInput struct {
	ID    uint    `json:"id"`
	Kind  string  `json:"kind"`
	URL   string  `json:"url"`
	Token *string `json:"token,omitempty"`
}

// TestServer 访问一次媒体服务器，返回服务器名称与版本。
func (s *Service) TestServer(ctx context.Context, in ServerTestInput) (server.Info, error) {
	token := ""
	if in.Token != nil {
		token = strings.TrimSpace(*in.Token)
	}
	if token == "" && in.ID > 0 {
		var row models.MediaServer
		if err := s.cfg.DB.WithContext(ctx).Where("id = ?", in.ID).Limit(1).Find(&row).Error; err != nil {
			return server.Info{}, fmt.Errorf("读取媒体服务器失败: %w", err)
		}
		if row.ID == 0 {
			return server.Info{}, ErrNotFound
		}
		if row.TokenEncrypted != "" {
			var err error
			if token, err = s.cfg.Cipher.Decrypt(row.TokenEncrypted); err != nil {
				return server.Info{}, fmt.Errorf("解密 Token 失败: %w", err)
			}
		}
	}
	c, err := server.New(server.Config{Kind: in.Kind, URL: in.URL, Token: token, HTTPClient: s.cfg.ServerHTTP})
	if err != nil {
		return server.Info{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return c.Test(ctx)
}

// serverClient 按保存的配置建客户端。
func (s *Service) serverClient(r models.MediaServer) (server.Client, error) {
	token, err := s.cfg.Cipher.Decrypt(r.TokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("解密 Token 失败: %w", err)
	}
	return server.New(server.Config{Kind: r.Kind, URL: r.URL, Token: token, HTTPClient: s.cfg.ServerHTTP})
}

// serverPath 把 pt-tools 里的路径换成媒体服务器里看到的路径。
func serverPath(r models.MediaServer, p string) string {
	if r.LocalPrefix == "" {
		return p
	}
	out, _ := models.MapTransferPath([]models.DownloaderPathMap{{SourcePrefix: r.LocalPrefix, TargetPrefix: r.ServerPrefix}}, p)
	return out
}
