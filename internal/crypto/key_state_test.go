package crypto

import (
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 启动时恢复失败要回滚：内存里的密钥状态原样还原，不读磁盘，「本进程新生成了密钥」的标记也要回来，
// 否则之后的启动检查看到的不是恢复前的状态（见 core.checkGeneratedSecretKey）。
func TestSnapshotRestoreKeyState(t *testing.T) {
	useTempHome(t)
	initKey()
	path, generated, loadErr := KeyStatus()
	require.True(t, generated)
	cipher, err := Encrypt([]byte("cookie"))
	require.NoError(t, err)
	before := SnapshotKeyState()

	// 换一把密钥文件并重新读取：新生成的标记被清掉，旧密文解不开
	other := make([]byte, 32)
	other[0] = 7
	require.NoError(t, os.WriteFile(path, []byte(hex.EncodeToString(other)), 0o600))
	ReloadKey()
	_, generated, _ = KeyStatus()
	assert.False(t, generated)
	_, err = Decrypt(cipher)
	assert.Error(t, err)

	RestoreKeyState(before)
	p2, g2, e2 := KeyStatus()
	assert.Equal(t, path, p2)
	assert.True(t, g2)
	assert.Equal(t, loadErr, e2)
	plain, err := Decrypt(cipher)
	require.NoError(t, err)
	assert.Equal(t, "cookie", string(plain))

	// 还原不读磁盘：文件里仍是那把别的密钥
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(other), string(data))
}

// 密钥文件坏了时的状态（没有加密器、有读取错误）也原样还原。
func TestRestoreKeyState_KeepsLoadError(t *testing.T) {
	home := useTempHome(t)
	require.NoError(t, os.MkdirAll(home+"/.pt-tools", 0o700))
	require.NoError(t, os.WriteFile(home+"/.pt-tools/secret.key", []byte("broken"), 0o600))
	initKey()
	_, _, loadErr := KeyStatus()
	require.Error(t, loadErr)
	before := SnapshotKeyState()

	require.NoError(t, os.WriteFile(home+"/.pt-tools/secret.key", []byte(hex.EncodeToString(make([]byte, 32))), 0o600))
	ReloadKey()
	_, _, err := KeyStatus()
	require.NoError(t, err)

	RestoreKeyState(before)
	_, _, err = KeyStatus()
	assert.Equal(t, loadErr, err)
	_, encErr := Encrypt([]byte("x"))
	assert.ErrorIs(t, encErr, errNoKey)
}

// 校验备份时要用备份里的密钥解它的密文，不能先换掉进程里的密钥。
func TestDecryptWithKey(t *testing.T) {
	useTempHome(t)
	initKey()
	cipher, err := Encrypt([]byte("x"))
	require.NoError(t, err)
	key, err := ExportKey()
	require.NoError(t, err)

	plain, err := DecryptWithKey(key, cipher)
	require.NoError(t, err)
	assert.Equal(t, "x", string(plain))

	wrong := append([]byte(nil), key...)
	wrong[0] ^= 0xff
	_, err = DecryptWithKey(wrong, cipher)
	assert.Error(t, err)
	_, err = DecryptWithKey([]byte("short"), cipher)
	assert.Error(t, err)
	_, err = DecryptWithKey(key, "not base64!")
	assert.Error(t, err)
}
