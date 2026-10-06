package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// NotificationConf.Enabled 带 gorm default:true：Create 会跳过 Go 的零值 false，
// 原来以 enabled=false 新建的通道存成了启用，返回给调用方的也是 true。
func TestCreateConf_DisabledStaysDisabled(t *testing.T) {
	setupTestKey(t)
	db := setupTestDB(t)
	svc := NewNotificationService(db, &mockNotifyManager{}, 0)

	dto, err := svc.CreateConf(context.Background(), CreateConfReq{
		ChannelType: "webhook", Name: "先不启用", ConfigJSON: []byte(`{"endpoint_url":"http://127.0.0.1:9/x"}`), Enabled: false,
	})
	require.NoError(t, err)
	assert.False(t, dto.Enabled)

	var row models.NotificationConf
	require.NoError(t, db.First(&row, dto.ID).Error)
	assert.False(t, row.Enabled, "库里也是停用")
}

// 新建失败时返回错误，不留半条记录。
func TestCreateConf_WriteFailures(t *testing.T) {
	cases := []struct {
		name, trigger string
		enabled       bool
	}{
		{"insert", `CREATE TRIGGER t BEFORE INSERT ON notification_conf BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`, true},
		{"disable write-back", `CREATE TRIGGER t BEFORE UPDATE OF enabled ON notification_conf BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestKey(t)
			db := setupTestDB(t)
			require.NoError(t, db.Exec(tc.trigger).Error)
			svc := NewNotificationService(db, &mockNotifyManager{}, 0)

			_, err := svc.CreateConf(context.Background(), CreateConfReq{
				ChannelType: "webhook", Name: "n", ConfigJSON: []byte(`{"endpoint_url":"http://127.0.0.1:9/x"}`), Enabled: tc.enabled,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "创建通知通道失败")
			var count int64
			require.NoError(t, db.Model(&models.NotificationConf{}).Count(&count).Error)
			assert.Zero(t, count, "失败时整笔回滚")
		})
	}
}
