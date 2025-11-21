package mocks

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/models"
)

type QbitMock struct{ pushed []string }

func (q *QbitMock) ProcessSingleTorrentFile(ctx context.Context, path, category, tags string) error {
	q.pushed = append(q.pushed, fmt.Sprintf("%s|%s|%s", path, category, tags))
	return nil
}

func (q *QbitMock) GetActiveDownloadTasks(ctx context.Context) (map[string]any, int, error) {
	return map[string]any{}, 0, nil
}

var _ = models.TorrentInfo{}
