package organize

import (
	"context"
	"errors"
	"fmt"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// IssueView 是给界面看的待处理问题：带上服务器的名字（服务器删了时为空）。
type IssueView struct {
	models.MediaIssue
	ServerName string `json:"server_name,omitempty"`
}

// Issues 列出待处理问题（见 serverindex.Issues）。
func (s *Service) Issues(ctx context.Context, f serverindex.IssueFilter) ([]IssueView, error) {
	rows, err := serverindex.Issues(ctx, s.cfg.DB, f)
	if err != nil {
		return nil, err
	}
	var servers []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Select("id", "name").Find(&servers).Error; err != nil {
		return nil, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	names := map[uint]string{}
	for _, srv := range servers {
		names[srv.ID] = srv.Name
	}
	out := make([]IssueView, 0, len(rows))
	for _, r := range rows {
		out = append(out, IssueView{MediaIssue: r, ServerName: names[r.ServerID]})
	}
	return out, nil
}

// DismissIssue 忽略一个还开着的问题（问题消失以前保持忽略）。
func (s *Service) DismissIssue(ctx context.Context, id uint) (models.MediaIssue, error) {
	row, err := serverindex.Dismiss(ctx, s.cfg.DB, id, s.cfg.Now())
	if errors.Is(err, serverindex.ErrIssueNotFound) {
		return row, ErrNotFound
	}
	return row, err
}
