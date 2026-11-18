package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/crypto"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// checkedChannel 的配置要有非空的 key；plainSeen 记下检查时拿到的配置（应是明文）。
type checkedChannel struct {
	notify.Channel
	plainSeen *[]string
}

func (c *checkedChannel) CheckConfig(conf *models.NotificationConf) error {
	*c.plainSeen = append(*c.plainSeen, conf.ConfigJSON)
	var cfg struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(conf.ConfigJSON), &cfg); err != nil || cfg.Key == "" {
		return errors.New("key 为空")
	}
	return nil
}

// uncheckedChannel 不实现 ConfigChecker：保存时不检查。
type uncheckedChannel struct{ notify.Channel }

func newCheckedService(t *testing.T) (*notificationService, *[]string) {
	t.Helper()
	setupTestKey(t)
	seen := &[]string{}
	reg := notify.NewRegistry()
	reg.Register("checked", func() notify.Channel { return &checkedChannel{plainSeen: seen} })
	reg.Register("unchecked", func() notify.Channel { return &uncheckedChannel{} })
	svc := NewNotificationService(setupTestDB(t), &mockNotifyManager{}, 0).(*notificationService)
	svc.registry = reg
	return svc, seen
}

func storedConfig(t *testing.T, svc *notificationService, id uint) (string, string) {
	t.Helper()
	var row models.NotificationConf
	require.NoError(t, svc.db.First(&row, id).Error)
	plain, err := crypto.Decrypt(row.ConfigJSON)
	require.NoError(t, err)
	return row.ChannelType, string(plain)
}

func TestCreateConf_ChecksConfig(t *testing.T) {
	svc, seen := newCheckedService(t)
	ctx := context.Background()

	_, err := svc.CreateConf(ctx, CreateConfReq{ChannelType: "checked", Name: "a", ConfigJSON: []byte(`{"key":""}`), Enabled: true})
	require.ErrorIs(t, err, ErrInvalidConf)
	assert.EqualError(t, err, "通道配置无效: key 为空")
	var n int64
	require.NoError(t, svc.db.Model(&models.NotificationConf{}).Count(&n).Error)
	assert.Zero(t, n, "没通过检查不落库")

	dto, err := svc.CreateConf(ctx, CreateConfReq{ChannelType: "checked", Name: "a", ConfigJSON: []byte(`{"key":"k"}`), Enabled: true})
	require.NoError(t, err)
	assert.NotZero(t, dto.ID)
	assert.Equal(t, []string{`{"key":""}`, `{"key":"k"}`}, *seen, "检查拿到的是明文")

	// 不支持检查的适配器、没注册的类型：照旧放行
	_, err = svc.CreateConf(ctx, CreateConfReq{ChannelType: "unchecked", Name: "b", ConfigJSON: []byte(`{}`)})
	require.NoError(t, err)
	_, err = svc.CreateConf(ctx, CreateConfReq{ChannelType: "telegram", Name: "c", ConfigJSON: []byte(`{}`)})
	require.NoError(t, err)
}

func TestUpdateConf_ChecksMergedConfig(t *testing.T) {
	svc, seen := newCheckedService(t)
	ctx := context.Background()
	dto, err := svc.CreateConf(ctx, CreateConfReq{ChannelType: "checked", Name: "a", ConfigJSON: []byte(`{"key":"k","x":"1"}`), Enabled: true})
	require.NoError(t, err)

	// 只发部分字段：按合并后的配置检查
	require.NoError(t, svc.UpdateConf(ctx, dto.ID, UpdateConfReq{ConfigJSON: []byte(`{"x":"2"}`)}))
	assert.JSONEq(t, `{"key":"k","x":"2"}`, (*seen)[len(*seen)-1])

	err = svc.UpdateConf(ctx, dto.ID, UpdateConfReq{ConfigJSON: []byte(`{"key":""}`)})
	require.ErrorIs(t, err, ErrInvalidConf)
	_, plain := storedConfig(t, svc, dto.ID)
	assert.JSONEq(t, `{"key":"k","x":"2"}`, plain, "没通过检查时库里的配置不变")

	// 只改名字、开关：不检查
	name, off := "renamed", false
	before := len(*seen)
	require.NoError(t, svc.UpdateConf(ctx, dto.ID, UpdateConfReq{Name: &name, Enabled: &off}))
	assert.Len(t, *seen, before)
}

func TestUpdateConf_ChecksConfigUnderNewType(t *testing.T) {
	svc, _ := newCheckedService(t)
	ctx := context.Background()
	dto, err := svc.CreateConf(ctx, CreateConfReq{ChannelType: "unchecked", Name: "a", ConfigJSON: []byte(`{"x":"1"}`)})
	require.NoError(t, err)

	// 改类型不改配置：旧配置按新类型检查
	typ := "checked"
	err = svc.UpdateConf(ctx, dto.ID, UpdateConfReq{ChannelType: &typ})
	require.ErrorIs(t, err, ErrInvalidConf)
	gotType, plain := storedConfig(t, svc, dto.ID)
	assert.Equal(t, "unchecked", gotType, "没通过检查时类型不变")
	assert.JSONEq(t, `{"x":"1"}`, plain)

	require.NoError(t, svc.UpdateConf(ctx, dto.ID, UpdateConfReq{ChannelType: &typ, ConfigJSON: []byte(`{"key":"k"}`)}))
	gotType, plain = storedConfig(t, svc, dto.ID)
	assert.Equal(t, "checked", gotType)
	assert.JSONEq(t, `{"x":"1","key":"k"}`, plain)

	missing := "checked"
	require.ErrorIs(t, svc.UpdateConf(ctx, 999, UpdateConfReq{ChannelType: &missing}), ErrConfNotFound)
}
