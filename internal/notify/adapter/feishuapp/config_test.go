package feishuapp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestCheckConfig(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "最小配置", raw: `{"app_id":"cli_a1","app_secret":"s3cret"}`},
		{name: "完整配置", raw: `{"app_id":" cli_a1 ","app_secret":"s3cret","domain":"lark",
			"admin_users":["ou_a","ou_a"," ou_b "],"allowed_users":["ou_c",""],"default_chat_id":"oc_g",
			"proxy_url":"socks5://127.0.0.1:1080"}`},
		{name: "默认会话可以是个人", raw: `{"app_id":"cli_a1","app_secret":"s","default_chat_id":"ou_x"}`},
		{name: "默认会话可以是 union_id", raw: `{"app_id":"cli_a1","app_secret":"s","default_chat_id":"on_x"}`},
		{name: "名单为 null", raw: `{"app_id":"cli_a1","app_secret":"s","admin_users":null}`},
		{name: "http 代理", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"http://user:pw@proxy:3128"}`},
		{name: "配置为空", raw: ``, wantErr: "App ID"},
		{name: "不是 JSON", raw: `{`, wantErr: "JSON"},
		{name: "缺 app_id", raw: `{"app_secret":"s"}`, wantErr: "App ID 不能为空"},
		{name: "app_id 中间有空白", raw: `{"app_id":"cli a1","app_secret":"s"}`, wantErr: "App ID 中间不能有空白"},
		{name: "缺 app_secret", raw: `{"app_id":"cli_a1","app_secret":"  "}`, wantErr: "App Secret 不能为空"},
		{name: "domain 不对", raw: `{"app_id":"cli_a1","app_secret":"s","domain":"dingtalk"}`, wantErr: "feishu"},
		{name: "管理员不是 ou_", raw: `{"app_id":"cli_a1","app_secret":"s","admin_users":["u_123"]}`, wantErr: "以 ou_ 开头"},
		{name: "允许名单不是 ou_", raw: `{"app_id":"cli_a1","app_secret":"s","allowed_users":["on_1"]}`, wantErr: "以 ou_ 开头"},
		{name: "名单是数字", raw: `{"app_id":"cli_a1","app_secret":"s","admin_users":[123]}`, wantErr: "字符串数组"},
		{name: "名单是对象", raw: `{"app_id":"cli_a1","app_secret":"s","allowed_users":{"a":1}}`, wantErr: "字符串数组"},
		{name: "默认会话前缀不对", raw: `{"app_id":"cli_a1","app_secret":"s","default_chat_id":"123"}`, wantErr: "oc_"},
		{name: "默认会话是数字", raw: `{"app_id":"cli_a1","app_secret":"s","default_chat_id":123}`, wantErr: "oc_"},
		{name: "代理协议不对", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"ftp://proxy:21"}`, wantErr: "代理地址"},
		{name: "https 代理不支持长连接", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"https://proxy:443"}`, wantErr: "代理地址"},
		{name: "代理没有主机", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"http://"}`, wantErr: "代理地址"},
		// 长连接用的 gorilla/websocket 不会给 socks5 补默认端口：拨号报 missing port in address
		{name: "socks5 代理没写端口", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"socks5://127.0.0.1"}`, wantErr: "端口"},
		{name: "http 代理没写端口", raw: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"http://proxy.lan"}`, wantErr: "端口"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := New().CheckConfig(&models.NotificationConf{ChannelType: ChannelType, ConfigJSON: tc.raw})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCheckConfig_NilConf(t *testing.T) {
	require.Error(t, New().CheckConfig(nil))
}

// 代理地址可能带用户名与密码：校验失败时不能把它写进错误（错误会显示在页面上、写进日志）。
func TestCheckConfig_ProxyErrorHidesValue(t *testing.T) {
	err := New().CheckConfig(&models.NotificationConf{ConfigJSON: `{"app_id":"cli_a1","app_secret":"s","proxy_url":"ftp://user:hunter2@proxy"}`})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestParseConfig_Normalizes(t *testing.T) {
	cfg, err := parseConfig(`{"app_id":" cli_a1 ","app_secret":" s3cret ","admin_users":["ou_a","ou_a"," ou_b "],
		"allowed_users":["ou_c",""],"default_chat_id":" oc_g ","proxy_url":" http://p:1 "}`)
	require.NoError(t, err)
	assert.Equal(t, "cli_a1", cfg.AppID)
	assert.Equal(t, "s3cret", cfg.AppSecret)
	assert.Equal(t, DomainFeishu, cfg.Domain)
	assert.Equal(t, []string{"ou_a", "ou_b"}, cfg.AdminUsers)
	assert.Equal(t, []string{"ou_c"}, cfg.AllowedUsers)
	assert.Equal(t, "oc_g", cfg.DefaultChatID)
	assert.Equal(t, "http://p:1", cfg.ProxyURL)
	assert.True(t, cfg.permitted("ou_a"))
	assert.True(t, cfg.permitted("ou_c"))
	assert.False(t, cfg.permitted("ou_z"))
}

func TestConfig_PermittedEmptyListsDenyAll(t *testing.T) {
	cfg, err := parseConfig(`{"app_id":"cli_a1","app_secret":"s"}`)
	require.NoError(t, err)
	assert.False(t, cfg.permitted("ou_a"))
	assert.False(t, cfg.permitted(""))
}

func TestBaseURL(t *testing.T) {
	assert.Equal(t, "https://open.feishu.cn", baseURL(&Config{Domain: DomainFeishu}))
	assert.Equal(t, "https://open.larksuite.com", baseURL(&Config{Domain: DomainLark}))
}

func TestProxyFunc(t *testing.T) {
	cfg, err := parseConfig(`{"app_id":"cli_a1","app_secret":"s","proxy_url":"socks5://127.0.0.1:1080"}`)
	require.NoError(t, err)
	u, err := proxyFunc(cfg)(nil)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "socks5://127.0.0.1:1080", u.String())
	assert.NotNil(t, proxyFunc(&Config{}), "没有通道代理时跟环境变量")
}

func TestCheckConfig_LongBadIDShortened(t *testing.T) {
	long := "x" + strings.Repeat("y", 100)
	err := New().CheckConfig(&models.NotificationConf{ConfigJSON: `{"app_id":"cli_a1","app_secret":"s","admin_users":["` + long + `"]}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "…")
	assert.NotContains(t, err.Error(), long)
}
