package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/models"
)

// WebhookMaxBody 是一次 Webhook 最多读多少字节（Plex 的 multipart 里 payload 在缩略图前面，读到 payload 就不往下读）。
const WebhookMaxBody = 1 << 20

// Webhook 事件归成的几类。
const (
	WebhookAdded         = "added"
	WebhookRemoved       = "removed"
	WebhookPlaybackStart = "playback_start"
	WebhookPlaybackStop  = "playback_stop"
	WebhookOther         = "other"
)

// ErrBadWebhook 表示请求体不是认得的 Webhook（不是 JSON，或 multipart 里没有 data、payload）。
var ErrBadWebhook = errors.New("看不懂的 Webhook 请求")

// WebhookEvent 是解析出的一次 Webhook。Action 是上面几类之一，Name 是服务器写的原名（ItemAdded、library.new、media.play 等）；
// Kind 是 movie、episode、series 或空串（不是这几种）；Title 是这一项的名字，剧集另有 SeriesTitle、Season、Episode。
type WebhookEvent struct {
	Action      string
	Name        string
	ItemID      string
	Kind        string
	Title       string
	SeriesTitle string
	Season      int
	Episode     int
	Year        int
	User        string
	Device      string
	Client      string
}

// webhookActions 是事件名到分类：Emby（library.new、playback.start）、Jellyfin 的 Webhook 插件（ItemAdded、PlaybackStart）、
// Plex（library.new、media.play）。没列的是 other。
var webhookActions = map[string]string{
	"library.new":     WebhookAdded,
	"itemadded":       WebhookAdded,
	"library.deleted": WebhookRemoved,
	"itemdeleted":     WebhookRemoved,
	"playback.start":  WebhookPlaybackStart,
	"playbackstart":   WebhookPlaybackStart,
	"media.play":      WebhookPlaybackStart,
	"playback.stop":   WebhookPlaybackStop,
	"playbackstop":    WebhookPlaybackStop,
	"media.stop":      WebhookPlaybackStop,
}

// ParseWebhook 解析一次 Webhook：请求体是 JSON（Emby 选 application/json、Jellyfin 插件的 Generic 目标，Content-Type 常是
// text/plain），或表单（Emby 的 data 字段）、multipart（Emby 的 data、Plex 的 payload）。按里面的字段认是哪一家，不看服务器的种类。
// 最多读 WebhookMaxBody 字节。
func ParseWebhook(contentType string, body io.Reader) (WebhookEvent, error) {
	body = io.LimitReader(body, WebhookMaxBody+1)
	mt, params, _ := mime.ParseMediaType(contentType)
	var raw []byte
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		r := multipart.NewReader(body, params["boundary"])
		for {
			part, err := r.NextPart()
			if errors.Is(err, io.EOF) {
				return WebhookEvent{}, fmt.Errorf("%w: multipart 里没有 data 或 payload", ErrBadWebhook)
			}
			if err != nil {
				return WebhookEvent{}, fmt.Errorf("%w: multipart 坏了: %w", ErrBadWebhook, err)
			}
			if name := part.FormName(); name == "data" || name == "payload" {
				raw, err = io.ReadAll(part)
				if err != nil {
					return WebhookEvent{}, fmt.Errorf("%w: 读取 %s 失败: %w", ErrBadWebhook, name, err)
				}
				break
			}
		}
	case mt == "application/x-www-form-urlencoded":
		b, err := io.ReadAll(body)
		if err != nil {
			return WebhookEvent{}, fmt.Errorf("%w: 读取请求体失败: %w", ErrBadWebhook, err)
		}
		q, err := url.ParseQuery(string(b))
		if err != nil || q.Get("data") == "" {
			return WebhookEvent{}, fmt.Errorf("%w: 表单里没有 data", ErrBadWebhook)
		}
		raw = []byte(q.Get("data"))
	default:
		b, err := io.ReadAll(body)
		if err != nil {
			return WebhookEvent{}, fmt.Errorf("%w: 读取请求体失败: %w", ErrBadWebhook, err)
		}
		raw = b
	}
	if len(raw) > WebhookMaxBody {
		return WebhookEvent{}, fmt.Errorf("%w: 请求体超过 1 MiB", ErrBadWebhook)
	}
	return parseWebhookJSON(raw)
}

func parseWebhookJSON(raw []byte) (WebhookEvent, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &keys); err != nil {
		return WebhookEvent{}, fmt.Errorf("%w: 不是 JSON 对象", ErrBadWebhook)
	}
	// 键按原样认（encoding/json 不分大小写：Plex 的 user 是布尔，Emby 的 User 是对象）
	var ev WebhookEvent
	var err error
	switch {
	case keys["NotificationType"] != nil:
		ev, err = jellyfinHook(raw)
	case keys["Metadata"] != nil || keys["Account"] != nil || keys["event"] != nil:
		ev, err = plexHook(raw)
	case keys["Event"] != nil:
		ev, err = embyHook(raw)
	default:
		return WebhookEvent{}, fmt.Errorf("%w: 没有事件名（Event、NotificationType 或 event）", ErrBadWebhook)
	}
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("%w: %w", ErrBadWebhook, err)
	}
	ev.Action = webhookActions[strings.ToLower(ev.Name)]
	if ev.Action == "" {
		ev.Action = WebhookOther
	}
	return ev, nil
}

// hookKind 是条目种类的统一写法。
func hookKind(t string) string {
	switch strings.ToLower(t) {
	case "movie":
		return models.MediaServerItemMovie
	case "episode":
		return models.MediaServerItemEpisode
	case "series", "show":
		return models.MediaServerItemSeries
	}
	return ""
}

func embyHook(raw []byte) (WebhookEvent, error) {
	var b struct {
		Event string `json:"Event"`
		Item  *struct {
			ID                string `json:"Id"`
			Name              string `json:"Name"`
			Type              string `json:"Type"`
			SeriesName        string `json:"SeriesName"`
			ParentIndexNumber *int   `json:"ParentIndexNumber"`
			IndexNumber       *int   `json:"IndexNumber"`
			ProductionYear    int    `json:"ProductionYear"`
		} `json:"Item"`
		User *struct {
			Name string `json:"Name"`
		} `json:"User"`
		Session *struct {
			DeviceName string `json:"DeviceName"`
			Client     string `json:"Client"`
		} `json:"Session"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return WebhookEvent{}, err
	}
	ev := WebhookEvent{Name: b.Event}
	if it := b.Item; it != nil {
		ev.ItemID, ev.Kind, ev.Title, ev.SeriesTitle, ev.Year = it.ID, hookKind(it.Type), it.Name, it.SeriesName, it.ProductionYear
		ev.Season, ev.Episode = intOr(it.ParentIndexNumber), intOr(it.IndexNumber)
	}
	if b.User != nil {
		ev.User = b.User.Name
	}
	if b.Session != nil {
		ev.Device, ev.Client = b.Session.DeviceName, b.Session.Client
	}
	return ev, nil
}

// flexInt 认数字与写成字符串的数字（Jellyfin 插件的模板里常给变量加引号），空串是 0。
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	// 模板没填对（不是数字）时当作没有，不让整个请求失败
	*n = 0
	if v, err := strconv.Atoi(strings.Trim(strings.TrimSpace(string(b)), `"`)); err == nil {
		*n = flexInt(v)
	}
	return nil
}

func jellyfinHook(raw []byte) (WebhookEvent, error) {
	var b struct {
		NotificationType     string  `json:"NotificationType"`
		ItemID               string  `json:"ItemId"`
		ItemType             string  `json:"ItemType"`
		Name                 string  `json:"Name"`
		SeriesName           string  `json:"SeriesName"`
		SeasonNumber         flexInt `json:"SeasonNumber"`
		EpisodeNumber        flexInt `json:"EpisodeNumber"`
		Year                 flexInt `json:"Year"`
		NotificationUsername string  `json:"NotificationUsername"`
		DeviceName           string  `json:"DeviceName"`
		ClientName           string  `json:"ClientName"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return WebhookEvent{}, err
	}
	return WebhookEvent{
		Name: b.NotificationType, ItemID: b.ItemID, Kind: hookKind(b.ItemType), Title: b.Name, SeriesTitle: b.SeriesName,
		Season: int(b.SeasonNumber), Episode: int(b.EpisodeNumber), Year: int(b.Year),
		User: b.NotificationUsername, Device: b.DeviceName, Client: b.ClientName,
	}, nil
}

func plexHook(raw []byte) (WebhookEvent, error) {
	var b struct {
		Event   string `json:"event"`
		Account *struct {
			Title string `json:"title"`
		} `json:"Account"`
		Player *struct {
			Title string `json:"title"`
		} `json:"Player"`
		Metadata *struct {
			RatingKey        string `json:"ratingKey"`
			Type             string `json:"type"`
			Title            string `json:"title"`
			GrandparentTitle string `json:"grandparentTitle"`
			ParentIndex      *int   `json:"parentIndex"`
			Index            *int   `json:"index"`
			Year             int    `json:"year"`
		} `json:"Metadata"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return WebhookEvent{}, err
	}
	ev := WebhookEvent{Name: b.Event, Client: "Plex"}
	if m := b.Metadata; m != nil {
		ev.ItemID, ev.Kind, ev.Title, ev.Year = m.RatingKey, hookKind(m.Type), m.Title, m.Year
		if ev.Kind == models.MediaServerItemEpisode {
			ev.SeriesTitle, ev.Season, ev.Episode = m.GrandparentTitle, intOr(m.ParentIndex), intOr(m.Index)
		}
	}
	if b.Account != nil {
		ev.User = b.Account.Title
	}
	if b.Player != nil {
		ev.Device = b.Player.Title
	}
	return ev, nil
}
