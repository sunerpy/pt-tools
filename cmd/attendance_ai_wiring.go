package cmd

import (
	"context"
	"errors"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// attendanceAI 把 AI 助手接到每日签到：打开了「用 AI 签到」、AI 助手也启用并配好了，
// 要识别验证码或答题的站点签到时才有 AI。取不到时返回的错误写明怎么打开（签到记录里给用户看）。
type attendanceAI struct {
	store *core.ConfigStore
	svc   *assistant.Service
}

func (a attendanceAI) AttendanceSolver(ctx context.Context) (v2.AttendanceSolver, error) {
	enabled, model, err := a.store.AttendanceAI()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errors.New("没有打开「用 AI 签到」：在「自动签到」页的签到设置里打开")
	}
	if err := a.svc.Ready(ctx); err != nil {
		if errors.Is(err, assistant.ErrDisabled) {
			return nil, errors.New("AI 助手没有启用或没有配置好：在「AI 助手设置」里填好接口地址、API Key 和模型并启用")
		}
		return nil, err
	}
	return attendanceAISolver{svc: a.svc, vision: model}, nil
}

// attendanceAISolver 是签到流程看到的 AI：认验证码用签到设置里选的看图模型，答题用助手的模型。
type attendanceAISolver struct {
	svc    *assistant.Service
	vision string
}

func (s attendanceAISolver) ReadCaptcha(ctx context.Context, img v2.CaptchaImage) (string, error) {
	return s.svc.ReadCaptcha(withAttendanceUsage(ctx), s.vision, assistant.CaptchaRequest{
		Image: img.Data, MIME: img.MIME, Length: img.Length, UpperOnly: img.UpperOnly,
	})
}

func (s attendanceAISolver) ChooseAnswer(ctx context.Context, q v2.AttendanceQuestion) ([]int, error) {
	return s.svc.ChooseAnswer(withAttendanceUsage(ctx), q.Question, q.Options, q.Multiple)
}

// withAttendanceUsage 把签到监控放进 ctx 的用量记录接到 AI 助手上：每次模型回答都记一次调用与 token。
func withAttendanceUsage(ctx context.Context) context.Context {
	rec := v2.AttendanceAIUsageFrom(ctx)
	if rec == nil {
		return ctx
	}
	return assistant.WithUsageSink(ctx, func(u assistant.Usage) {
		rec.Add(u.Model, u.PromptTokens, u.CompletionTokens)
	})
}

// wireAttendanceAI 在 AI 助手建好以后接到签到监控上（签到监控先建好，见 wireLoginReminderMonitor）。
func wireAttendanceAI(mgr *scheduler.Manager, store *core.ConfigStore, svc *assistant.Service) {
	if mgr == nil || store == nil || svc == nil {
		return
	}
	if att := mgr.GetAttendanceMonitor(); att != nil {
		att.SetSolver(attendanceAI{store: store, svc: svc})
	}
}
