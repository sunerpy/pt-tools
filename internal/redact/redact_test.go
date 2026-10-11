package redact

import (
	"strings"
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

func TestTextKeepsIDsButMasksCredentials(t *testing.T) {
	hash := "abcdef0123456789abcdef0123456789abcdef01"
	cases := map[string]string{
		"暂停 " + hash: "暂停 " + hash,
		"https://x.example/dl?passkey=SECRETSECRET 结束": "https://x.example/dl?… 结束",
		"https://u:p@x.example/a":                      "https://***@x.example/a",
		"token=abc123 与 password=hunter2":              "token=*** 与 password=***",
		"我的 key 是 sk-proj-abcdefghijklmnop1234 别外传":    "我的 key 是 *** 别外传",
		"令牌 ptt_12_abcdefghijklmnopqrstuv":             "令牌 ***",
		"我的 passkey 0123456789abcdef0123456789abcdef":  "我的 passkey ***",
		"v2 的 " + strings.Repeat("ab", 32):             "v2 的 " + strings.Repeat("ab", 32),
		"令牌 AbCdEfGhIjKlMnOpQrStUvWxYz":                "令牌 ***",
		"标题 The.Matrix.1999.2160p.UHD.BluRay":          "标题 The.Matrix.1999.2160p.UHD.BluRay",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Text("https://x.example/rss/0123456789abcdef0123456789abcdef/feed"); strings.Contains(got, "0123456789abcdef0123456789abcdef") {
		t.Errorf("路径里像密钥的段落要遮：%q", got)
	}
}
