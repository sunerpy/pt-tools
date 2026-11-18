package assistant

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/openai/openai-go/v3"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
)

// 附件：图片（PNG、JPEG、WebP、GIF，每张最多 5 MB）与文本文件（UTF-8，每个最多 200 KB），一条消息最多 4 个。
// 先 Upload（还没发的只有属主看得到、删得了），发消息时带上编号。文本文件存的是脱敏以后的内容（和用户写的文字一样过 userText），
// 图片原样存。交给模型时，这一轮的附件全给：文本拼进消息，图片用 data URI；以前的消息里的文本在历史的字数以内照给，
// 图片不再给、写一句占位：不能看图的模型碰到图片会拒绝整个请求，换了模型接着聊时不该被以前的图片卡住。
const (
	maxImageBytes = 5 << 20
	maxTextBytes  = 200 << 10
	// MaxAttachments 是一条消息最多带几个附件
	MaxAttachments = 4
	// MaxUploadBytes 是上传一个附件的请求体上限（含 multipart 的头）
	MaxUploadBytes = maxImageBytes + 64<<10
	// maxPending 是一个属主最多有几个还没发的附件
	maxPending = 20
	// pendingTTL 是还没发的附件留多久
	pendingTTL         = 24 * time.Hour
	maxAttachNameRunes = 100
)

// imageMIMEs 是收的图片（按内容认，http.DetectContentType）。
var imageMIMEs = map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true}

// TextExts 是收的文本文件扩展名（内容还要是 UTF-8）。
var TextExts = []string{
	".txt", ".md", ".markdown", ".log", ".json", ".csv", ".tsv", ".yaml", ".yml", ".toml", ".ini", ".conf", ".xml",
	".nfo", ".srt", ".ass",
}

// textMIME 是文本附件存的类型（下载时也按它回）。
const textMIME = "text/plain; charset=utf-8"

// AttachmentView 是一个附件（不带内容）。
type AttachmentView struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	// Kind 是 image 或 text
	Kind string `json:"kind"`
	MIME string `json:"mime"`
	// Size 是上传的文件大小（字节）
	Size int64 `json:"size"`
}

// viewColumns 是读附件但不要内容时选的列。
var viewColumns = []string{"id", "message_id", "name", "kind", "mime", "size"}

func attachmentView(a models.AssistantAttachment) AttachmentView {
	return AttachmentView{ID: a.ID, Name: a.Name, Kind: a.Kind, MIME: a.MIME, Size: a.Size}
}

// errAttachGone 是发消息时附件已经被别的消息认领了（或者删掉了）。
var errAttachGone = errors.New("附件不存在或者已经发过了：重新添加")

// attachmentName 整理浏览器给的文件名：只留最后一段，去掉控制字符，太长的截掉（留着扩展名），过一遍脱敏；没有时叫「附件」。
func attachmentName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name))
	if r := []rune(name); len(r) > maxAttachNameRunes {
		ext := []rune(path.Ext(name))
		if len(ext) > 10 {
			ext = nil
		}
		name = string(r[:maxAttachNameRunes-len(ext)]) + string(ext)
	}
	if name == "" || name == "." || name == ".." {
		return "附件"
	}
	return redact.Text(name)
}

// attachmentText 检查文本附件：扩展名在 TextExts 里，不超过 200 KB，是 UTF-8（去掉开头的 BOM），没有 NUL。
func attachmentText(name string, data []byte) (string, error) {
	if !slices.Contains(TextExts, strings.ToLower(path.Ext(name))) {
		return "", fmt.Errorf("%w: 只能发 PNG、JPEG、WebP、GIF 图片，或者 %s 这些文本文件", ErrInvalid, strings.Join(TextExts, " "))
	}
	if len(data) > maxTextBytes {
		return "", fmt.Errorf("%w: 文本文件最大 %d KB", ErrInvalid, maxTextBytes>>10)
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("%w: 文本文件要是 UTF-8 编码", ErrInvalid)
	}
	return string(data), nil
}

// Upload 存一个还没发的附件。name 是浏览器给的文件名；图片按内容认，文本文件按扩展名认。
func (s *Service) Upload(ctx context.Context, a Actor, name string, data []byte) (AttachmentView, error) {
	if !a.valid() {
		return AttachmentView{}, ErrNotFound
	}
	if len(data) == 0 {
		return AttachmentView{}, fmt.Errorf("%w: 文件是空的", ErrInvalid)
	}
	name = attachmentName(name)
	row := models.AssistantAttachment{Owner: a.Owner, Name: name, Size: int64(len(data)), CreatedAt: s.now()}
	if mime := http.DetectContentType(data); imageMIMEs[mime] {
		if len(data) > maxImageBytes {
			return AttachmentView{}, fmt.Errorf("%w: 图片最大 %d MB", ErrInvalid, maxImageBytes>>20)
		}
		row.Kind, row.MIME, row.Data = models.AssistantAttachImage, mime, data
	} else {
		text, err := attachmentText(name, data)
		if err != nil {
			return AttachmentView{}, err
		}
		row.Kind, row.MIME, row.Data = models.AssistantAttachText, textMIME, []byte(s.userText(text))
	}
	var pending int64
	if err := s.db(ctx).Model(&models.AssistantAttachment{}).Where("owner = ? AND message_id = 0", a.Owner).
		Count(&pending).Error; err != nil {
		return AttachmentView{}, fmt.Errorf("读取附件失败: %w", err)
	}
	if pending >= maxPending {
		return AttachmentView{}, fmt.Errorf("%w: 还没发的附件太多（最多 %d 个）：先发出去或者删掉一些", ErrInvalid, maxPending)
	}
	if err := s.db(ctx).Create(&row).Error; err != nil {
		return AttachmentView{}, fmt.Errorf("保存附件失败: %w", err)
	}
	return attachmentView(row), nil
}

// Attachment 读属主的一个附件，连同内容。
func (s *Service) Attachment(ctx context.Context, a Actor, id uint) (models.AssistantAttachment, error) {
	if !a.valid() || id == 0 {
		return models.AssistantAttachment{}, ErrNotFound
	}
	var rows []models.AssistantAttachment
	if err := s.db(ctx).Where("id = ? AND owner = ?", id, a.Owner).Limit(1).Find(&rows).Error; err != nil {
		return models.AssistantAttachment{}, fmt.Errorf("读取附件失败: %w", err)
	}
	if len(rows) == 0 {
		return models.AssistantAttachment{}, ErrNotFound
	}
	return rows[0], nil
}

// DeleteAttachment 删掉一个还没发的附件；发出去的跟着会话删。
func (s *Service) DeleteAttachment(ctx context.Context, a Actor, id uint) error {
	if !a.valid() || id == 0 {
		return ErrNotFound
	}
	res := s.db(ctx).Where("id = ? AND owner = ? AND message_id = 0", id, a.Owner).Delete(&models.AssistantAttachment{})
	if res.Error != nil {
		return fmt.Errorf("删除附件失败: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	if _, err := s.Attachment(ctx, a, id); err != nil {
		return err
	}
	return fmt.Errorf("%w: 已经发出去的附件跟着会话删", ErrInvalid)
}

// pendingAttachments 读属主还没发的这些附件（不带内容，按编号排）。去重以后最多 MaxAttachments 个；有不存在的、发过的回 ErrInvalid。
func (s *Service) pendingAttachments(ctx context.Context, a Actor, ids []uint) ([]models.AssistantAttachment, []uint, error) {
	uniq := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !slices.Contains(uniq, id) {
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return nil, nil, nil
	}
	if len(uniq) > MaxAttachments {
		return nil, nil, fmt.Errorf("%w: 一条消息最多带 %d 个附件", ErrInvalid, MaxAttachments)
	}
	var rows []models.AssistantAttachment
	if err := s.db(ctx).Select(viewColumns).Where("id IN ? AND owner = ? AND message_id = 0", uniq, a.Owner).
		Order("id").Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("读取附件失败: %w", err)
	}
	if len(rows) != len(uniq) {
		return nil, nil, fmt.Errorf("%w: %s", ErrInvalid, errAttachGone.Error())
	}
	return rows, uniq, nil
}

// addUserMessage 在一个事务里落用户消息、认领还没发的附件，再发 message 事件（带附件）。
func (s *Service) addUserMessage(ctx context.Context, m models.AssistantMessage, owner string, ids []uint) (MessageView, error) {
	m.CreatedAt = s.now()
	var atts []models.AssistantAttachment
	err := s.db(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		res := tx.Model(&models.AssistantAttachment{}).Where("id IN ? AND owner = ? AND message_id = 0", ids, owner).
			Updates(map[string]any{"message_id": m.ID, "conversation_id": m.ConversationID})
		if res.Error != nil {
			return res.Error
		}
		if int(res.RowsAffected) != len(ids) {
			return errAttachGone
		}
		return tx.Select(viewColumns).Where("message_id = ?", m.ID).Order("id").Find(&atts).Error
	})
	if errors.Is(err, errAttachGone) {
		return MessageView{}, fmt.Errorf("%w: %s", ErrInvalid, errAttachGone.Error())
	}
	if err != nil {
		return MessageView{}, fmt.Errorf("保存消息失败: %w", err)
	}
	v := messageView(m)
	for _, a := range atts {
		v.Attachments = append(v.Attachments, attachmentView(a))
	}
	s.hub.publish(m.ConversationID, Event{Type: EventMessage, RunID: m.RunID, Message: &v, At: m.CreatedAt})
	return v, nil
}

// attachmentViews 是这些消息的附件（不带内容），按消息编号分好。
func (s *Service) attachmentViews(ctx context.Context, msgIDs []uint) (map[uint][]AttachmentView, error) {
	out := map[uint][]AttachmentView{}
	if len(msgIDs) == 0 {
		return out, nil
	}
	var rows []models.AssistantAttachment
	if err := s.db(ctx).Select(viewColumns).Where("message_id IN ?", msgIDs).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取附件失败: %w", err)
	}
	for _, a := range rows {
		out[a.MessageID] = append(out[a.MessageID], attachmentView(a))
	}
	return out, nil
}

// attachmentMeta 是这些用户消息的附件（不带内容），按消息编号分好。
func (s *Service) attachmentMeta(ctx context.Context, msgIDs []uint) (map[uint][]models.AssistantAttachment, error) {
	out := map[uint][]models.AssistantAttachment{}
	if len(msgIDs) == 0 {
		return out, nil
	}
	var rows []models.AssistantAttachment
	if err := s.db(ctx).Select(viewColumns).Where("message_id IN ?", msgIDs).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取附件失败: %w", err)
	}
	for _, a := range rows {
		out[a.MessageID] = append(out[a.MessageID], a)
	}
	return out, nil
}

// fillAttachmentData 读这些附件的内容，填进 atts（只读要给模型的那些：这一轮的，与以前放得下的文本）。
func (s *Service) fillAttachmentData(ctx context.Context, atts map[uint][]models.AssistantAttachment, need map[uint]bool) error {
	if len(need) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(need))
	for id := range need {
		ids = append(ids, id)
	}
	var data []models.AssistantAttachment
	if err := s.db(ctx).Select("id", "data").Where("id IN ?", ids).Find(&data).Error; err != nil {
		return fmt.Errorf("读取附件失败: %w", err)
	}
	byID := make(map[uint][]byte, len(data))
	for _, d := range data {
		byID[d.ID] = d.Data
	}
	for msg, list := range atts {
		for i := range list {
			list[i].Data = byID[list[i].ID]
		}
		atts[msg] = list
	}
	return nil
}

// textSize 是附件里文本的大小（按上传的大小估，算历史的字数用）。
func textSize(atts []models.AssistantAttachment) int {
	n := 0
	for _, a := range atts {
		if a.Kind == models.AssistantAttachText {
			n += int(a.Size)
		}
	}
	return n
}

// fence 是把 text 包成代码块用的围栏：比内容里最长的一串反引号多一个，至少三个。
func fence(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

// sizeText 是给模型看的文件大小。
func sizeText(n int64) string {
	if n < 1<<10 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// userParam 是交给模型的一条用户消息：文本附件拼在文字后面（omitText 时只写占位），current 这一条的图片用 data URI，
// 以前的图片写一句占位。没有图片时是一段文字（不是所有兼容服务都认分段的内容）。
func userParam(content string, atts []models.AssistantAttachment, current, omitText bool) (openai.ChatCompletionMessageParamUnion, bool) {
	var b strings.Builder
	b.WriteString(content)
	note := func(format string, args ...any) {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, format, args...)
	}
	var images []openai.ChatCompletionContentPartUnionParam
	for _, a := range atts {
		switch {
		case a.Kind == models.AssistantAttachText && !omitText:
			text := string(a.Data)
			f := fence(text)
			note("附件「%s」（%s）的内容：\n%s\n%s\n%s", a.Name, sizeText(a.Size), f, text, f)
		case a.Kind == models.AssistantAttachText:
			note("（附件「%s」，%s：太长了，这次没有再附上内容）", a.Name, sizeText(a.Size))
		case current && len(a.Data) > 0:
			images = append(images, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL: "data:" + a.MIME + ";base64," + base64.StdEncoding.EncodeToString(a.Data),
			}))
		default:
			note("（这条消息附了图片「%s」，这次没有再附上）", a.Name)
		}
	}
	if len(images) == 0 {
		return openai.UserMessage(b.String()), false
	}
	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, len(images)+1)
	if b.Len() > 0 {
		parts = append(parts, openai.TextContentPart(b.String()))
	}
	return openai.UserMessage(append(parts, images...)), true
}

// cleanPendingAttachments 删掉传了一天还没发的附件。
func (s *Service) cleanPendingAttachments(ctx context.Context) error {
	if err := s.db(ctx).Where("message_id = 0 AND created_at < ?", s.now().Add(-pendingTTL)).
		Delete(&models.AssistantAttachment{}).Error; err != nil {
		return fmt.Errorf("清理没发的附件失败: %w", err)
	}
	return nil
}
