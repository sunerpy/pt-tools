package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

// ErrSiteNotFound 表示请求的站点未在 registry / ConfigStore 中找到。
var ErrSiteNotFound = errors.New("site not found")

// ErrUserInfoUnavailable 表示站点尚未抓取到用户信息，或 driver 不支持。
var ErrUserInfoUnavailable = errors.New("site user info not available")

// SiteSummaryDTO 用于 chatops `/sites` 命令展示已配置的站点列表概览。
type SiteSummaryDTO struct {
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	LastScrapedAt time.Time `json:"last_scraped_at"`
	Status        string    `json:"status"`
}

// UserInfoDTO 是站点用户信息的 chatops 友好快照（字段全部转字符串便于格式化展示）。
type UserInfoDTO struct {
	SiteName   string `json:"site_name"`
	Username   string `json:"username"`
	Uploaded   string `json:"uploaded"`
	Downloaded string `json:"downloaded"`
	Ratio      string `json:"ratio"`
	Bonus      string `json:"bonus"`
	Class      string `json:"class"`
}

// SiteService 为 chatops `/sites` `/userinfo` 等命令提供站点元数据与用户统计。
type SiteService interface {
	ListSites(ctx context.Context) ([]SiteSummaryDTO, error)
	GetSiteUserInfo(ctx context.Context, siteName string) (UserInfoDTO, error)
}

// SiteLister 是 ConfigStore 的最小读视图（便于测试 mock）。*core.ConfigStore 自动满足。
type SiteLister interface {
	ListSites() (map[models.SiteGroup]models.SiteConfig, error)
}

// UserInfoSource 抽象 site/v2 的 UserInfoRepo（实测使用 *v2.DBUserInfoRepo）。
// 仅暴露 chatops 必需的最小读取面，避免上层依赖 v2 全部 API。
type UserInfoSource interface {
	Get(ctx context.Context, site string) (v2.UserInfo, error)
}

type siteService struct {
	store SiteLister
	users UserInfoSource
}

// NewSiteService 注入 *core.ConfigStore 与 v2.UserInfoRepo（典型为 *v2.DBUserInfoRepo）。
// users 可为 nil，此时 GetSiteUserInfo 永远返回 ErrUserInfoUnavailable。
func NewSiteService(store *core.ConfigStore, users UserInfoSource) SiteService {
	return &siteService{store: store, users: users}
}

func newSiteServiceWithDeps(store SiteLister, users UserInfoSource) SiteService {
	return &siteService{store: store, users: users}
}

func (s *siteService) ListSites(ctx context.Context) ([]SiteSummaryDTO, error) {
	if s.store == nil {
		return []SiteSummaryDTO{}, nil
	}
	m, err := s.store.ListSites()
	if err != nil {
		return nil, fmt.Errorf("list sites from config store: %w", err)
	}
	out := make([]SiteSummaryDTO, 0, len(m))
	for sg, cfg := range m {
		name := string(sg)
		enabled := cfg.Enabled != nil && *cfg.Enabled
		dto := SiteSummaryDTO{
			Name:    name,
			Enabled: enabled,
			Status:  statusForSite(enabled),
		}
		if s.users != nil {
			if info, err := s.users.Get(ctx, name); err == nil && info.LastUpdate > 0 {
				dto.LastScrapedAt = time.Unix(info.LastUpdate, 0)
			}
		}
		out = append(out, dto)
	}
	return out, nil
}

func (s *siteService) GetSiteUserInfo(ctx context.Context, siteName string) (UserInfoDTO, error) {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		return UserInfoDTO{}, ErrSiteNotFound
	}
	if s.store != nil {
		m, err := s.store.ListSites()
		if err != nil {
			return UserInfoDTO{}, fmt.Errorf("list sites: %w", err)
		}
		if _, ok := m[models.SiteGroup(siteName)]; !ok {
			return UserInfoDTO{}, ErrSiteNotFound
		}
	}
	if s.users == nil {
		return UserInfoDTO{}, ErrUserInfoUnavailable
	}
	info, err := s.users.Get(ctx, siteName)
	if err != nil {
		if errors.Is(err, v2.ErrSiteNotFound) {
			return UserInfoDTO{}, ErrUserInfoUnavailable
		}
		return UserInfoDTO{}, fmt.Errorf("get user info: %w", err)
	}
	return UserInfoDTO{
		SiteName:   siteName,
		Username:   info.Username,
		Uploaded:   formatBytes(info.Uploaded),
		Downloaded: formatBytes(info.Downloaded),
		Ratio:      fmt.Sprintf("%.3f", info.Ratio),
		Bonus:      fmt.Sprintf("%.0f", info.Bonus),
		Class:      classOf(info),
	}, nil
}

func statusForSite(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func classOf(info v2.UserInfo) string {
	if info.LevelName != "" {
		return info.LevelName
	}
	return info.Rank
}

// formatBytes 将字节数格式化为 1024 进制可读字符串（chatops 展示友好），不依赖外部库。
func formatBytes(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}[exp]
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), suffix)
}
