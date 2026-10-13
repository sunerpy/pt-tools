package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
	"github.com/sunerpy/pt-tools/models"
)

// ErrUnknownEvent 是不认识的事件类型。
var ErrUnknownEvent = errors.New("没有这种事件")

// TemplateInput 是一种事件的消息模板：空着的一项用内置文字。
type TemplateInput struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// TemplatePreview 是用示例数据渲染出来的标题与正文。
type TemplatePreview struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// TemplateView 是一种事件：目录里的说明与变量，加上当前的模板（没有自定义时 Title、Text 为空）。
type TemplateView struct {
	notify.EventSpec
	Title      string     `json:"title"`
	Text       string     `json:"text"`
	Customized bool       `json:"customized"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

// TemplateCatalog 是消息模板页要的全部数据。
type TemplateCatalog struct {
	CommonVars []notify.Var   `json:"common_vars"`
	Events     []TemplateView `json:"events"`
}

// TemplateService 管全局消息模板：读写数据库，并把内存里的模板集（发送前套用的那份）换成最新的。
type TemplateService interface {
	List(ctx context.Context) (TemplateCatalog, error)
	Save(ctx context.Context, eventType string, in TemplateInput) (TemplateView, error)
	Reset(ctx context.Context, eventType string) error
	Preview(eventType string, in TemplateInput) (TemplatePreview, error)
	// Load 从库里读出全部模板，换掉内存里的（启动时调用）
	Load(ctx context.Context) error
}

type templateService struct {
	db  *gorm.DB
	set *notify.Templates
	// mu 让「写库 → 重新读入」成为一步，两次保存交错时内存里的不会是旧的
	mu sync.Mutex
}

// NewTemplateService 建模板服务。set 是发送前套用的模板集。
func NewTemplateService(db *gorm.DB, set *notify.Templates) TemplateService {
	return &templateService{db: db, set: set}
}

func (s *templateService) rows(ctx context.Context) (map[string]models.NotificationTemplate, error) {
	var rows []models.NotificationTemplate
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取消息模板失败: %w", err)
	}
	out := make(map[string]models.NotificationTemplate, len(rows))
	for _, r := range rows {
		out[r.EventType] = r
	}
	return out, nil
}

func (s *templateService) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reload(ctx)
}

func (s *templateService) reload(ctx context.Context) error {
	rows, err := s.rows(ctx)
	if err != nil {
		return err
	}
	srcs := make([]notify.TemplateSource, 0, len(rows))
	for _, r := range rows {
		srcs = append(srcs, notify.TemplateSource{EventType: r.EventType, Title: r.Title, Text: r.Text})
	}
	s.set.Replace(srcs)
	return nil
}

func (s *templateService) List(ctx context.Context) (TemplateCatalog, error) {
	rows, err := s.rows(ctx)
	if err != nil {
		return TemplateCatalog{}, err
	}
	out := TemplateCatalog{CommonVars: notify.CommonVars, Events: make([]TemplateView, 0, len(notify.Events))}
	for _, spec := range notify.Events {
		out.Events = append(out.Events, view(spec, rows[spec.Type]))
	}
	return out, nil
}

func view(spec notify.EventSpec, row models.NotificationTemplate) TemplateView {
	v := TemplateView{EventSpec: spec, Title: row.Title, Text: row.Text}
	if row.EventType != "" {
		v.Customized = true
		at := row.UpdatedAt
		v.UpdatedAt = &at
	}
	return v
}

// check 用示例数据渲染一遍标题与正文模板：写错了返回 tmpl.ErrInvalid。
func check(eventType string, in TemplateInput) (TemplatePreview, error) {
	if notify.Spec(eventType) == nil {
		return TemplatePreview{}, fmt.Errorf("%w：%s", ErrUnknownEvent, eventType)
	}
	title, err := notify.RenderSample(eventType, tmpl.Title, in.Title)
	if err != nil {
		return TemplatePreview{}, fmt.Errorf("%w（标题）：%s", tmpl.ErrInvalid, strings.TrimPrefix(err.Error(), tmpl.ErrInvalid.Error()+": "))
	}
	text, err := notify.RenderSample(eventType, tmpl.Text, in.Text)
	if err != nil {
		return TemplatePreview{}, fmt.Errorf("%w（正文）：%s", tmpl.ErrInvalid, strings.TrimPrefix(err.Error(), tmpl.ErrInvalid.Error()+": "))
	}
	return TemplatePreview{Title: title, Text: text}, nil
}

func (s *templateService) Preview(eventType string, in TemplateInput) (TemplatePreview, error) {
	return check(eventType, in)
}

func (s *templateService) Save(ctx context.Context, eventType string, in TemplateInput) (TemplateView, error) {
	if _, err := check(eventType, in); err != nil {
		return TemplateView{}, err
	}
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Text) == "" {
		if err := s.Reset(ctx, eventType); err != nil {
			return TemplateView{}, err
		}
		return view(*notify.Spec(eventType), models.NotificationTemplate{}), nil
	}
	row := models.NotificationTemplate{EventType: eventType, Title: in.Title, Text: in.Text, UpdatedAt: time.Now()}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "text", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return TemplateView{}, fmt.Errorf("保存消息模板失败: %w", err)
	}
	if err := s.reload(ctx); err != nil {
		return TemplateView{}, err
	}
	return view(*notify.Spec(eventType), row), nil
}

func (s *templateService) Reset(ctx context.Context, eventType string) error {
	if notify.Spec(eventType) == nil {
		return fmt.Errorf("%w：%s", ErrUnknownEvent, eventType)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.WithContext(ctx).Where("event_type = ?", eventType).Delete(&models.NotificationTemplate{}).Error; err != nil {
		return fmt.Errorf("恢复默认失败: %w", err)
	}
	return s.reload(ctx)
}
