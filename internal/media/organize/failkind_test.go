package organize

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// 放文件出错时整理记录按错误分类：目标已有别的文件、源文件不在了、跨文件系统与没有权限、其他；之后整理成了，分类清掉
func TestFailKindOfTransferErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		kind string
	}{
		{"exists", fmt.Errorf("放文件: %w", transfer.ErrTargetExists), models.MediaFailExists},
		{"source", transfer.ErrSourceMissing, models.MediaFailSource},
		{"cross-device", transfer.ErrCrossDevice, models.MediaFailFilesystem},
		{"permission", &os.PathError{Op: "link", Path: "/x", Err: os.ErrPermission}, models.MediaFailFilesystem},
		{"other", errors.New("磁盘满了"), models.MediaFailOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.defaultLibraries()
			e.settings(SettingsInput{MinVideoMB: 1})
			e.oppenheimer()
			old := transferDetail
			t.Cleanup(func() { transferDetail = old })
			transferDetail = func(string, string, string) (transfer.Outcome, transfer.Detail, error) {
				return 0, transfer.Detail{}, tc.err
			}
			_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			rows := e.history()
			require.Len(t, rows, 1)
			assert.NotEqual(t, models.MediaTransferDone, rows[0].Status)
			assert.Equal(t, tc.kind, rows[0].FailKind)

			transferDetail = old
			res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
			require.NoError(t, err)
			require.Equal(t, 1, res.Created)
			rows = e.history()
			require.Len(t, rows, 1)
			assert.Equal(t, models.MediaTransferDone, rows[0].Status)
			assert.Empty(t, rows[0].FailKind, "整理成的记录没有分类")
		})
	}
}

// 分类表与常量对得上，页面按这个顺序列
func TestMediaFailKinds(t *testing.T) {
	assert.Equal(t, []string{"recognize", "library", "path", "filesystem", "exists", "source", "disc", "other"}, models.MediaFailKinds)
}
