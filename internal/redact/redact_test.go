package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecrets(t *testing.T) {
	assert.Equal(t, `下载种子失败: Get "https://pt.example/download.php?…": EOF`,
		Secrets(`下载种子失败: Get "https://pt.example/download.php?id=1&passkey=abc": EOF`))
	assert.Equal(t, "https://***@pt.example/x", Secrets("https://user:pass@pt.example/x"))
	assert.Equal(t, `{"passkey":"***","name":"a"}`, Secrets(`{"passkey":"0123","name":"a"}`))
	assert.Equal(t, "cookie=*** 其他", Secrets("cookie=uid=1 其他"))
	assert.NotContains(t, Secrets("https://unit3d.example/torrent/download/12.abcdefabcdefabcdefabcdefabcdefab"), "abcdefabcdefabcdefabcdefabcdefab")
	assert.Equal(t, "没有地址的错误", Secrets("没有地址的错误"))
}

// Credentials 不遮 info hash 这类长编号（工具结果里要留着），Secrets 会遮
func TestCredentialsKeepsLongIDs(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	in := `{"task_id":"` + hash + `","url":"https://pt.example/d?passkey=x"}`
	out := Credentials(in)
	assert.Contains(t, out, hash)
	assert.NotContains(t, out, "passkey=x")
	assert.NotContains(t, Secrets(in), hash)
}
