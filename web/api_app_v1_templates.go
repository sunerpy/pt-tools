package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/notify/tmpl"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// App API 的消息模板（路线图 M22）：读目录与当前模板、保存、恢复默认、用示例数据预览。和网页上的「消息模板」是同一份：
// 通知、ChatOps 命令回复、AI 助手回复三组事件，保存前用示例数据校验。读与预览要 app:read，保存与恢复默认要 app:write。
// 模板的原文（title、text）是管理用的配置，里面写了什么都有可能，只给有 app:write 的调用者（编辑要用，按写的样子给）；
// 只读的调用者拿到的这两项是空的。示例值、内置文字的样子与预览的结果经 appRedact 遮住像凭证的内容。

// AppTemplateGroup 是事件的一组：notify（通知）、reply（ChatOps 命令回复）、assistant（AI 助手回复）。
type AppTemplateGroup struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// AppTemplateVar 是一个模板变量。sample 是示例值写成的文字（列表没有示例，字段在 item 里）。
type AppTemplateVar struct {
	Key    string           `json:"key"`
	Type   string           `json:"type"`
	Desc   string           `json:"desc"`
	Sample string           `json:"sample,omitempty"`
	Item   []AppTemplateVar `json:"item,omitempty"`
}

// AppTemplate 是一种事件与它现在的模板；没有自定义时 title、text 是空的，发的是内置文字（sample_title、sample_text 是它的样子）。
// title、text 只给有 app:write 的调用者，只读的调用者看 customized 知道有没有自定义。no_title 的事件只有正文。
type AppTemplate struct {
	Type        string           `json:"type"`
	Group       string           `json:"group"`
	Name        string           `json:"name"`
	Desc        string           `json:"desc"`
	NoTitle     bool             `json:"no_title"`
	Vars        []AppTemplateVar `json:"vars"`
	SampleTitle string           `json:"sample_title"`
	SampleText  string           `json:"sample_text"`
	Title       string           `json:"title"`
	Text        string           `json:"text"`
	Customized  bool             `json:"customized"`
	UpdatedAt   int64            `json:"updated_at,omitempty"`
}

// AppTemplateCatalog 是 GET /templates 的回应。
type AppTemplateCatalog struct {
	Groups     []AppTemplateGroup `json:"groups"`
	CommonVars []AppTemplateVar   `json:"common_vars"`
	Events     []AppTemplate      `json:"events"`
}

// AppTemplateInput 是 PUT /templates/{type} 的请求：空着的一项用内置文字，两项都空等于恢复默认。
type AppTemplateInput struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// AppTemplatePreviewInput 是 POST /templates/preview 的请求。
type AppTemplatePreviewInput struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// AppTemplatePreview 是按示例数据渲染出来的标题与正文（只有正文的事件 title 是空的）。
type AppTemplatePreview struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

func appTemplateVars(vars []notify.Var) []AppTemplateVar {
	out := make([]AppTemplateVar, 0, len(vars))
	for _, v := range vars {
		av := AppTemplateVar{Key: v.Key, Type: string(v.Type), Desc: v.Desc}
		if v.Type == notify.VarList {
			av.Item = appTemplateVars(v.Item)
		} else {
			av.Sample = appRedact(appSampleText(v.Sample))
		}
		out = append(out, av)
	}
	return out
}

// appSampleText 把示例值写成文字：数字按十进制、真假写成 true/false。
func appSampleText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// appTemplateView 是给 App 的一种事件；withSource 为 false 时不带模板原文。
func appTemplateView(v app.TemplateView, withSource bool) AppTemplate {
	out := AppTemplate{
		Type: v.Type, Group: v.Group, Name: v.Name, Desc: v.Desc, NoTitle: v.NoTitle, Vars: appTemplateVars(v.Vars),
		SampleTitle: appRedact(v.SampleTitle), SampleText: appRedact(v.SampleText), Customized: v.Customized,
	}
	if withSource {
		out.Title, out.Text = v.Title, v.Text
	}
	if v.UpdatedAt != nil {
		out.UpdatedAt = v.UpdatedAt.Unix()
	}
	return out
}

// appTemplateService 取模板服务；没有时已经写了 503。
func (s *Server) appTemplateService(w http.ResponseWriter) (app.TemplateService, bool) {
	if s.chatopsDeps == nil || s.chatopsDeps.TemplateSvc == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "消息模板服务没有启动")
		return nil, false
	}
	return s.chatopsDeps.TemplateSvc, true
}

// appTemplateError 写模板的错误：没有这种事件 404，模板写得不对 400（写明哪里不对），别的 500。
func appTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrUnknownEvent):
		appError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, tmpl.ErrInvalid):
		appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	default:
		global.GetSlogger().Warnf("[App API] 读写消息模板失败: %v", err)
		appError(w, http.StatusInternalServerError, "internal", "读写消息模板失败")
	}
}

func (s *Server) appTemplates(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appTemplateService(w)
	if !ok {
		return
	}
	cat, err := svc.List(r.Context())
	if err != nil {
		appTemplateError(w, err)
		return
	}
	out := AppTemplateCatalog{
		Groups:     make([]AppTemplateGroup, 0, len(cat.Groups)),
		CommonVars: appTemplateVars(cat.CommonVars),
		Events:     make([]AppTemplate, 0, len(cat.Events)),
	}
	for _, g := range cat.Groups {
		out.Groups = append(out.Groups, AppTemplateGroup{Key: g.Key, Name: g.Name, Desc: g.Desc})
	}
	withSource := middleware.PrincipalFrom(r.Context()).Has(apitoken.ScopeAppWrite)
	for _, v := range cat.Events {
		out.Events = append(out.Events, appTemplateView(v, withSource))
	}
	appJSON(w, out)
}

func (s *Server) appTemplateSave(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appTemplateService(w)
	if !ok {
		return
	}
	var in AppTemplateInput
	if !appDecode(w, r, &in) {
		return
	}
	v, err := svc.Save(r.Context(), r.PathValue("type"), app.TemplateInput{Title: in.Title, Text: in.Text})
	if err != nil {
		appTemplateError(w, err)
		return
	}
	appJSON(w, appTemplateView(v, true))
}

func (s *Server) appTemplateReset(w http.ResponseWriter, r *http.Request) {
	if !appNoBody(w, r) {
		return
	}
	svc, ok := s.appTemplateService(w)
	if !ok {
		return
	}
	if err := svc.Reset(r.Context(), r.PathValue("type")); err != nil {
		appTemplateError(w, err)
		return
	}
	appJSON(w, map[string]bool{"ok": true})
}

func (s *Server) appTemplatePreview(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appTemplateService(w)
	if !ok {
		return
	}
	var in AppTemplatePreviewInput
	if !appDecode(w, r, &in) {
		return
	}
	p, err := svc.Preview(in.Type, app.TemplateInput{Title: in.Title, Text: in.Text})
	if err != nil {
		appTemplateError(w, err)
		return
	}
	appJSON(w, AppTemplatePreview{Title: appRedact(p.Title), Text: appRedact(p.Text)})
}
