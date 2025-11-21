package mocks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQbitMock_ProcessAndTasks(t *testing.T) {
	q := &QbitMock{}
	require.NoError(t, q.ProcessSingleTorrentFile(context.Background(), "/path/file.torrent", "cat", "tag1,tag2"))
	require.Len(t, q.pushed, 1)
	require.Contains(t, q.pushed[0], "/path/file.torrent|cat|tag1,tag2")
	tasks, n, err := q.GetActiveDownloadTasks(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NotNil(t, tasks)
}
