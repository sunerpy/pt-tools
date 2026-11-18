package feishuapp

import (
	"encoding/json"
	"regexp"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

var (
	// mentionTokenRe 是文字里 @ 的占位符（@_user_N），连同后面的一个空格。按整个记号匹配，@_user_1 不会误伤 @_user_10。
	mentionTokenRe = regexp.MustCompile(`@_user_\d+ ?`)
	// markdownLinkRe 是文字消息里收到的链接：[文本](地址)，只认 http(s) 与 mailto。
	markdownLinkRe = regexp.MustCompile(`\[([^\[\]]*)\]\(((?:https?://|mailto:)[^()\s]*)\)`)
)

// extractText 从接收消息的 content 里取出文字（见「接收消息内容」文档）：只认 text 与 post（富文本里取文字），
// 图片、文件、语音、表情这些忽略，与 Telegram 只认文字一致。取不出文字时返回空串。
func extractText(msgType, content string, mentions []*larkim.MentionEvent, botOpenID string) string {
	var s string
	switch msgType {
	case "text":
		var body struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &body) != nil {
			return ""
		}
		s = body.Text
	case "post":
		s = postText(content)
	default:
		return ""
	}
	s = replaceMentions(s, mentions, botOpenID)
	s = markdownLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := markdownLinkRe.FindStringSubmatch(m)
		return linkText(parts[1], parts[2])
	})
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// linkText 把链接换回文字：文本与地址相同时就是地址本身，邮箱链接取文本，其余写成「文本 (地址)」。
func linkText(text, href string) string {
	switch {
	case text == href || text == "":
		return href
	case strings.HasPrefix(href, "mailto:"):
		return text
	default:
		return text + " (" + href + ")"
	}
}

// postNode 是富文本里的一个节点。接收到的富文本没有 zh_cn 这类语言层。
type postNode struct {
	Tag      string `json:"tag"`
	Text     string `json:"text"`
	Href     string `json:"href"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
}

// postText 把富文本拼成文字：标题放第一行，段落之间换行；只取 text、a、at、code_block 节点。
func postText(content string) string {
	var body struct {
		Title   string       `json:"title"`
		Content [][]postNode `json:"content"`
	}
	if json.Unmarshal([]byte(content), &body) != nil {
		return ""
	}
	lines := make([]string, 0, len(body.Content)+1)
	if t := strings.TrimSpace(body.Title); t != "" {
		lines = append(lines, t)
	}
	for _, para := range body.Content {
		var b strings.Builder
		for _, n := range para {
			switch n.Tag {
			case "text", "code_block":
				b.WriteString(n.Text)
			case "a":
				b.WriteString(linkText(n.Text, n.Href))
			case "at":
				// 收到的 at 节点 user_id 是占位符（@_user_N），交给 replaceMentions 处理
				if strings.HasPrefix(n.UserID, "@_user_") {
					b.WriteString(n.UserID)
				} else if n.UserName != "" {
					b.WriteString("@" + n.UserName)
				}
			}
		}
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "\n")
}

// replaceMentions 处理 @ 的占位符：本机器人（id.open_id 等于 botOpenID）的去掉，别的人与机器人换成「@姓名」，
// 对不上的原样留着。去掉以后，群里的「@_user_1 /help」才能被认成命令。
func replaceMentions(s string, mentions []*larkim.MentionEvent, botOpenID string) string {
	if len(mentions) == 0 || !strings.Contains(s, "@_user_") {
		return s
	}
	type target struct {
		name  string
		isBot bool
	}
	byKey := make(map[string]target, len(mentions))
	for _, m := range mentions {
		if m == nil || m.Key == nil || *m.Key == "" {
			continue
		}
		t := target{name: deref(m.Name)}
		if m.Id != nil && botOpenID != "" && deref(m.Id.OpenId) == botOpenID {
			t.isBot = true
		}
		byKey[*m.Key] = t
	}
	return mentionTokenRe.ReplaceAllStringFunc(s, func(m string) string {
		token, space := strings.CutSuffix(m, " ")
		t, ok := byKey[token]
		switch {
		case !ok:
			return m
		case t.isBot:
			return ""
		case space:
			return "@" + t.name + " "
		default:
			return "@" + t.name
		}
	})
}

// mentionsBot 报告消息有没有 @ 这个应用的机器人：mentioned_type 只分 user 与 bot，被提及的是谁要看 id.open_id。
func mentionsBot(mentions []*larkim.MentionEvent, botOpenID string) bool {
	if botOpenID == "" {
		return false
	}
	for _, m := range mentions {
		if m != nil && m.Id != nil && deref(m.Id.OpenId) == botOpenID {
			return true
		}
	}
	return false
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
