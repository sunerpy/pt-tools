package telegram

import (
	"encoding/json"
	"testing"

	"github.com/mymmrac/telego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

func mustConfigFromJSON(t *testing.T, raw string) *Config {
	t.Helper()
	cfg := &Config{}
	require.NoError(t, json.Unmarshal([]byte(raw), cfg))
	return cfg
}

func TestConfig_DefaultChatID_AcceptsInt(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": 8576996727}`)

	id, ok := cfg.DefaultChatIDInt()
	require.True(t, ok)
	assert.Equal(t, int64(8576996727), id)

	name, ok := cfg.DefaultChatIDUsername()
	assert.False(t, ok)
	assert.Equal(t, "", name)
}

func TestConfig_DefaultChatID_AcceptsQuotedInt(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": "8576996727"}`)

	id, ok := cfg.DefaultChatIDInt()
	require.True(t, ok, "string-quoted integer must parse as int")
	assert.Equal(t, int64(8576996727), id)

	name, ok := cfg.DefaultChatIDUsername()
	assert.False(t, ok, "quoted-int must NOT also resolve as username")
	assert.Equal(t, "", name)
}

func TestConfig_DefaultChatID_AcceptsUsername(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": "@MyChannel"}`)

	id, ok := cfg.DefaultChatIDInt()
	assert.False(t, ok)
	assert.Zero(t, id)

	name, ok := cfg.DefaultChatIDUsername()
	require.True(t, ok)
	assert.Equal(t, "@MyChannel", name)
}

func TestConfig_DefaultChatID_BareNameGetsAtPrefix(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": "MyChannel"}`)

	name, ok := cfg.DefaultChatIDUsername()
	require.True(t, ok)
	assert.Equal(t, "@MyChannel", name)
}

func TestConfig_DefaultChatID_Empty(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)

	_, okInt := cfg.DefaultChatIDInt()
	_, okName := cfg.DefaultChatIDUsername()
	assert.False(t, okInt)
	assert.False(t, okName)
}

func TestConfig_AllowedUsers_PureInts(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"allowed_users": [123, 456]}`)
	assert.Equal(t, []int64{123, 456}, cfg.AllowedUsersList())
}

func TestConfig_AllowedUsers_MixedTypes(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"allowed_users": [123, "456", "bad", 789]}`)
	assert.Equal(t, []int64{123, 456, 789}, cfg.AllowedUsersList(),
		"bad string entries must be skipped, valid ones (int + quoted int) preserved")
}

func TestConfig_AllowedUsers_Empty(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	assert.Nil(t, cfg.AllowedUsersList())
}

func TestConfig_AdminUsers_QuotedInts(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"admin_users": ["111", "222"]}`)
	assert.Equal(t, []int64{111, 222}, cfg.AdminUsersList())
}

func TestConfig_LegacyStrictDecodeNoLongerFails(t *testing.T) {
	raw := `{"bot_token":"abc","default_chat_id":"@channel","admin_users":["111"]}`
	cfg := &Config{}
	require.NoError(t, json.Unmarshal([]byte(raw), cfg),
		"json.Unmarshal must NOT fail on the user's broken-state config "+
			"(default_chat_id is string, admin_users contains quoted ints)")
	assert.Equal(t, "abc", cfg.BotToken)
	name, ok := cfg.DefaultChatIDUsername()
	require.True(t, ok)
	assert.Equal(t, "@channel", name)
	assert.Equal(t, []int64{111}, cfg.AdminUsersList())
}

func TestResolveChatID_NumericTargets(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	n := notify.Notification{Targets: map[string]string{"chat_id": "12345"}}

	id, err := resolveChatID(n, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{ID: 12345}, id)
}

func TestResolveChatID_UsernameTargets(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	n := notify.Notification{Targets: map[string]string{"chat_id": "@MyChannel"}}

	id, err := resolveChatID(n, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{Username: "@MyChannel"}, id)
	assert.Zero(t, id.ID)
}

func TestResolveChatID_BareUsernameTargets(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	n := notify.Notification{Targets: map[string]string{"chat_id": "MyChannel"}}

	id, err := resolveChatID(n, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{Username: "@MyChannel"}, id)
}

func TestResolveChatID_DefaultChatID_Int(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": 555}`)
	id, err := resolveChatID(notify.Notification{}, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{ID: 555}, id)
}

func TestResolveChatID_DefaultChatID_QuotedInt(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": "555"}`)
	id, err := resolveChatID(notify.Notification{}, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{ID: 555}, id)
}

func TestResolveChatID_DefaultChatID_AsUsername(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"default_chat_id": "@channel"}`)
	id, err := resolveChatID(notify.Notification{}, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{Username: "@channel"}, id)
	assert.Zero(t, id.ID)
}

func TestResolveChatID_NoConfig(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	_, err := resolveChatID(notify.Notification{}, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_chat_id")
}

func TestResolveChatID_UserIDFallback(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)
	n := notify.Notification{UserID: "999"}
	id, err := resolveChatID(n, cfg)
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{ID: 999}, id)
}

func TestParseChatIDString_Empty(t *testing.T) {
	_, err := parseChatIDString("")
	require.Error(t, err)
}

func TestParseChatIDString_TrimsWhitespace(t *testing.T) {
	id, err := parseChatIDString("  555  ")
	require.NoError(t, err)
	assert.Equal(t, telego.ChatID{ID: 555}, id)
}

func TestPermitted_AdmitsBothListsForAllMessages(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{"admin_users":[1],"allowed_users":[2]}`)

	assert.True(t, permitted(1, cfg), "admin user must be admitted")
	assert.True(t, permitted(2, cfg), "allowed user must be admitted (per-command admin gating happens in chain layer)")
	assert.False(t, permitted(3, cfg), "user not in either list must be rejected")
}

func TestPermitted_EmptyLists(t *testing.T) {
	cfg := mustConfigFromJSON(t, `{}`)

	assert.False(t, permitted(1, cfg))
	assert.False(t, permitted(0, cfg))
}

func TestDenyReason_AlwaysNotInWhitelist(t *testing.T) {
	assert.Equal(t, "denied:not_in_whitelist", denyReason())
}
