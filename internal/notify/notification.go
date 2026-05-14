package notify

import "context"

type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

type Notification struct {
	Title             string            `json:"title"`
	Text              string            `json:"text"`
	Image             string            `json:"image,omitempty"`
	Link              string            `json:"link,omitempty"`
	ChannelType       string            `json:"channel_type,omitempty"`
	SourceConfID      uint              `json:"source_conf_id,omitempty"`
	UserID            string            `json:"user_id,omitempty"`
	Targets           map[string]string `json:"targets,omitempty"`
	Buttons           [][]Button        `json:"buttons,omitempty"`
	DisableWebPreview bool              `json:"disable_web_preview,omitempty"`
	OriginalMessageID string            `json:"original_message_id,omitempty"`
}

type InboundHandler func(ctx context.Context, msg InboundMessage) error

type InboundMessage struct {
	ChannelType   string `json:"channel_type"`
	SourceConfID  uint   `json:"source_conf_id"`
	ChannelUserID string `json:"channel_user_id"`
	Username      string `json:"username"`
	ChatID        string `json:"chat_id"`
	Text          string `json:"text"`
	IsCallback    bool   `json:"is_callback"`
	CallbackData  string `json:"callback_data"`
}
