package fakes

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/models"
)

type QbitFake struct{ pushed []string }

func (q *QbitFake) ProcessSingleTorrentFile(ctx context.Context, path, category, tags string) error {
	q.pushed = append(q.pushed, fmt.Sprintf("%s|%s|%s", path, category, tags))
	return nil
}
func (q *QbitFake) GetActiveDownloadTasks(ctx context.Context) (map[string]any, int, error) {
	return map[string]any{}, 0, nil
}

// compile-time check placeholder (align with real client where needed)
var _ = models.TorrentInfo{}
