package feishuapp

import (
	"encoding/json"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/assert"
)

func ptr(s string) *string { return &s }

func mention(key, openID, name, typ string) *larkim.MentionEvent {
	return &larkim.MentionEvent{Key: ptr(key), Id: &larkim.UserId{OpenId: ptr(openID)}, Name: ptr(name), MentionedType: ptr(typ)}
}

func textJSON(s string) string {
	b, _ := json.Marshal(map[string]string{"text": s})
	return string(b)
}

func TestExtractText_Text(t *testing.T) {
	assert.Equal(t, "/help", extractText("text", textJSON("  /help  "), nil, "ou_bot"))
	assert.Equal(t, "", extractText("text", textJSON("   "), nil, "ou_bot"), "空了就忽略")
	assert.Equal(t, "", extractText("text", `{bad`, nil, "ou_bot"))
	assert.Equal(t, "", extractText("image", `{"image_key":"img_x"}`, nil, "ou_bot"), "图片、文件、语音这些不认")
	assert.Equal(t, "", extractText("text", `{"text":123}`, nil, "ou_bot"))
}

// 群里的 @ 会变成 @_user_N：本机器人的占位符去掉（这样 /status 才认得出是命令），别的人与机器人换成 @姓名。
func TestExtractText_Mentions(t *testing.T) {
	bot := mention("@_user_1", "ou_bot", "pt-tools", "bot")
	other := mention("@_user_2", "ou_other_bot", "别的机器人", "bot")
	person := mention("@_user_3", "ou_person", "张三", "user")

	assert.Equal(t, "/status", extractText("text", textJSON("@_user_1 /status"), []*larkim.MentionEvent{bot}, "ou_bot"))
	assert.Equal(t, "@别的机器人 /delete x", extractText("text", textJSON("@_user_1 @_user_2 /delete x"),
		[]*larkim.MentionEvent{bot, other}, "ou_bot"), "只去掉本机器人的占位符")
	assert.Equal(t, "帮我问下 @张三 进度", extractText("text", textJSON("@_user_1 帮我问下 @_user_3 进度"),
		[]*larkim.MentionEvent{bot, person}, "ou_bot"))
	assert.Equal(t, "@pt-tools /status", extractText("text", textJSON("@_user_1 /status"), []*larkim.MentionEvent{bot}, ""),
		"不知道本机器人是谁时一律换成姓名")
	assert.Equal(t, "@_user_9 hi", extractText("text", textJSON("@_user_9 hi"), []*larkim.MentionEvent{nil, {Key: nil}}, "ou_bot"),
		"字段缺失的提及原样留着")
	assert.Equal(t, "/s @_user_10", extractText("text", textJSON("@_user_1 /s @_user_10"), []*larkim.MentionEvent{bot}, "ou_bot"),
		"@_user_1 不能误伤 @_user_10")
}

// 文字消息里的链接收到时是 [文本](地址)：不换回去的话 /addrss 拿到的参数是一段 Markdown。
func TestExtractText_Links(t *testing.T) {
	rss := "https://pt.example/torrentrss.php?rows=10&linktype=dl&passkey=abc123"
	assert.Equal(t, "/addrss "+rss, extractText("text", textJSON("/addrss ["+rss+"]("+rss+")"), nil, ""))
	assert.Equal(t, "看 文档 (https://x.example/z)", extractText("text", textJSON("看 [文档](https://x.example/z)"), nil, ""))
	assert.Equal(t, "发到 a@b.com", extractText("text", textJSON("发到 [a@b.com](mailto:a@b.com)"), nil, ""))
	assert.Equal(t, "[x](y) 原样", extractText("text", textJSON("[x](y) 原样"), nil, ""), "不是链接的方括号不动")
}

func TestExtractText_Post(t *testing.T) {
	post := `{"title":"标题","content":[
		[{"tag":"at","user_id":"@_user_1","user_name":"pt-tools"},{"tag":"text","text":" /addrss "},{"tag":"a","text":"https://pt.example/rss?a=1&passkey=k","href":"https://pt.example/rss?a=1&passkey=k"}],
		[{"tag":"text","text":"第二段 "},{"tag":"a","text":"说明","href":"https://x.example/doc"},{"tag":"img","image_key":"img_1"},{"tag":"emotion","emoji_type":"SMILE"}],
		[{"tag":"code_block","language":"GO","text":"fmt.Println(1)"}],
		[{"tag":"at","user_id":"@_user_2","user_name":"张三"},{"tag":"at","user_id":"ou_unknown","user_name":"李四"}]
	]}`
	mentions := []*larkim.MentionEvent{mention("@_user_1", "ou_bot", "pt-tools", "bot"), mention("@_user_2", "ou_p", "张三", "user")}
	got := extractText("post", post, mentions, "ou_bot")
	assert.Equal(t, "标题\n/addrss https://pt.example/rss?a=1&passkey=k\n第二段 说明 (https://x.example/doc)\nfmt.Println(1)\n@张三@李四", got)

	assert.Equal(t, "", extractText("post", `{"content":"bad"}`, nil, ""))
	assert.Equal(t, "只有标题", extractText("post", `{"title":"只有标题","content":[]}`, nil, ""))
}
