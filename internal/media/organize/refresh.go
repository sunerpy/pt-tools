package organize

import (
	"context" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// refreshDirs 是要通知媒体服务器扫描的目录：电影是文件所在目录，剧集是剧集目录（平铺时是文件所在目录）。
func refreshDirs(p *Plan, created []*PlanItem) []string {
	var dirs []string
	for _, it := range created {
		d := filepath.Dir(it.Target)
		if p.Match.MediaType == tmdb.KindTV {
			if sd := p.showDir(it.Target); sd != "" {
				d = sd
			}
		}
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// refreshServers 通知启用的媒体服务器扫描新目录，结果记在服务器上。
func (s *Service) refreshServers(ctx context.Context, p *Plan, created []*PlanItem) []string {
	var servers []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&servers).Error; err != nil {
		return []string{"读取媒体服务器失败：" + err.Error()}
	}
	dirs := refreshDirs(p, created)
	var msgs []string
	for _, srv := range servers {
		c, err := s.serverClient(srv)
		if err == nil {
			if srv.RefreshMode == models.MediaRefreshLibrary {
				err = c.RefreshAll(ctx)
			} else {
				paths := make([]string, 0, len(dirs))
				var unmapped []string
				for _, d := range dirs {
					if sp, ok := serverPath(srv, d); ok {
						paths = append(paths, sp)
					} else {
						unmapped = append(unmapped, d)
					}
				}
				if len(paths) > 0 {
					err = c.RefreshPaths(ctx, paths)
				}
				if err == nil && len(unmapped) > 0 {
					err = fmt.Errorf("%s 不在路径映射 %s → %s 里，没有通知", strings.Join(unmapped, "、"), srv.LocalPrefix, srv.ServerPrefix)
				}
			}
		}
		now := s.cfg.Now()
		upd := map[string]any{"last_refresh_at": &now, "last_error": ""}
		if err != nil {
			upd["last_error"] = truncate(err.Error(), 1024)
			msgs = append(msgs, fmt.Sprintf("通知媒体服务器「%s」失败：%v", srv.Name, err))
		}
		s.cfg.DB.WithContext(ctx).Model(&models.MediaServer{}).Where("id = ?", srv.ID).Updates(upd)
	}
	return msgs
}
