package assistant

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

// 签到用的 AI：认签到验证码、答签到题（站点的签到流程在 site/v2，经 cmd 接上）。
// 用 AI 助手的接口地址、Key 与代理，助手没启用或没配好时返回 ErrDisabled。认验证码要能看图的模型，
// 由签到设置单独选（空时用助手的模型）；答题只要文字，用助手的模型。

// signInTimeout 是一次识别或答题等模型的上限。
const signInTimeout = 90 * time.Second

// Ready 报告 AI 助手能不能用（启用了、接口地址与模型都填了、Key 能解开）。
func (s *Service) Ready(ctx context.Context) error {
	_, err := s.effectiveSettings(ctx)
	return err
}

// CaptchaRequest 是一张要认的验证码。
type CaptchaRequest struct {
	Image []byte
	MIME  string
	// Length 大于 0 时告诉模型一共几个字符，并按它从回答里挑
	Length int
	// UpperOnly 为真时告诉模型只有大写字母和数字
	UpperOnly bool
}

// ReadCaptcha 让模型读出验证码图片上的字符，只回字母和数字；读不出时回空串。visionModel 为空时用 AI 助手的模型。
func (s *Service) ReadCaptcha(ctx context.Context, visionModel string, req CaptchaRequest) (string, error) {
	eff, err := s.effectiveSettings(ctx)
	if err != nil {
		return "", err
	}
	prompt := "图片是网站签到用的验证码。只回答图片上的字符本身：字母和数字，不要解释，不要加空格和标点。"
	if req.UpperOnly {
		prompt = "图片是网站签到用的验证码，只有大写英文字母和数字。只回答图片上的字符本身（字母用大写），不要解释，不要加空格和标点。"
	}
	if req.Length > 0 {
		prompt += fmt.Sprintf("验证码一共 %d 个字符。", req.Length)
	}
	data := "data:" + req.MIME + ";base64," + base64.StdEncoding.EncodeToString(req.Image)
	reply, err := s.signInAsk(ctx, eff.llm, visionModelOr(visionModel, eff.llm.Model), openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
		openai.TextContentPart(prompt),
		openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: data}),
	}))
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 &&
			apiErr.StatusCode != http.StatusUnauthorized && apiErr.StatusCode != http.StatusForbidden &&
			apiErr.StatusCode != http.StatusTooManyRequests {
			return "", &TestError{Message: describeError(err, eff.llm.APIKey) + "（这个模型可能不能看图：在「自动签到」页的签到设置里换一个能看图的模型）"}
		}
		return "", modelError(err, eff.llm.APIKey)
	}
	text := captchaText(reply, req.Length)
	if req.UpperOnly {
		text = strings.ToUpper(text)
	}
	return text, nil
}

// modelError 把调模型的错误换成给人看的一句话（不带 Key）；设置不对（ErrInvalid）原样返回。
func modelError(err error, key string) error {
	if errors.Is(err, ErrInvalid) {
		return err
	}
	return &TestError{Message: describeError(err, key)}
}

func visionModelOr(visionModel, fallback string) string {
	if m := strings.TrimSpace(visionModel); m != "" {
		return m
	}
	return fallback
}

// ChooseAnswer 让助手的模型回答签到选择题，返回选中的选项下标（从 0 开始）；单选只取第一个。
func (s *Service) ChooseAnswer(ctx context.Context, question string, options []string, multiple bool) ([]int, error) {
	eff, err := s.effectiveSettings(ctx)
	if err != nil {
		return nil, err
	}
	kind, how := "单选", "只回答一个选项编号，不要解释"
	if multiple {
		kind, how = "多选", "只回答选项编号，几个编号用英文逗号分开（例如 1,3），不要解释"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "下面是 PT 站点每日签到的一道%s题，请选出正确答案。%s。\n\n题目：%s\n选项：\n", kind, how, question)
	for i, o := range options {
		fmt.Fprintf(&b, "%d. %s\n", i+1, o)
	}
	reply, err := s.signInAsk(ctx, eff.llm, eff.llm.Model, openai.UserMessage(b.String()))
	if err != nil {
		return nil, modelError(err, eff.llm.APIKey)
	}
	picked := parseChoices(reply, len(options), multiple)
	if len(picked) == 0 {
		return nil, fmt.Errorf("模型的回答里没有有效的选项编号：%q", truncateRunes(stripThinking(reply), 60))
	}
	return picked, nil
}

// Usage 是一次模型调用用掉的 token：服务商在回应里报的，没报时为 0。
type Usage struct {
	Model            string
	PromptTokens     int64
	CompletionTokens int64
}

type usageSinkKey struct{}

// WithUsageSink 让这个 ctx 里的签到调用（认验证码、答题）每拿到一次回答，就把模型与用量交给 sink（签到详情里记 token）。
func WithUsageSink(ctx context.Context, sink func(Usage)) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, usageSinkKey{}, sink)
}

func reportUsage(ctx context.Context, model string, u openai.CompletionUsage) {
	if sink, ok := ctx.Value(usageSinkKey{}).(func(Usage)); ok {
		sink(Usage{Model: model, PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens})
	}
}

// signInAsk 发一次不带工具的对话，回第一条回答的正文。错误原样返回（调用方按场景说明）。
func (s *Service) signInAsk(ctx context.Context, c llmConfig, model string, msg openai.ChatCompletionMessageParamUnion) (string, error) {
	chat, err := s.chatService(c)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, signInTimeout)
	defer cancel()
	res, err := chat.New(ctx, openai.ChatCompletionNewParams{Model: model, Messages: []openai.ChatCompletionMessageParamUnion{msg}})
	if err != nil {
		return "", err
	}
	reportUsage(ctx, model, res.Usage)
	if len(res.Choices) == 0 {
		return "", errors.New("模型没有回答")
	}
	return res.Choices[0].Message.Content, nil
}

var (
	thinkingPattern = regexp.MustCompile(`(?is)<think>.*?</think>`)
	alnumPattern    = regexp.MustCompile(`[A-Za-z0-9]+`)
	numberPattern   = regexp.MustCompile(`\d+`)
	// letterPattern 是单独写的大写字母选项（「选 B」）；小写不认，免得把英文里的 a 当成选项
	letterPattern = regexp.MustCompile(`(?:^|[^A-Za-z])([A-H])(?:[^A-Za-z]|$)`)
)

// stripThinking 去掉有些模型写在正文里的 <think>…</think>。
func stripThinking(s string) string {
	return strings.TrimSpace(thinkingPattern.ReplaceAllString(s, ""))
}

// captchaText 从回答里挑出验证码：知道字数时取最后一个刚好这么长的字母数字串，
// 没有就把所有字母数字连起来看长度对不对（模型有时一个字一个空格）；不知道字数时取最后一串。
func captchaText(reply string, length int) string {
	reply = stripThinking(reply)
	tokens := alnumPattern.FindAllString(reply, -1)
	if len(tokens) == 0 {
		return ""
	}
	if length <= 0 {
		return tokens[len(tokens)-1]
	}
	for i := len(tokens) - 1; i >= 0; i-- {
		if len(tokens[i]) == length {
			return tokens[i]
		}
	}
	if joined := strings.Join(tokens, ""); len(joined) == length {
		return joined
	}
	return ""
}

// parseChoices 把回答里的选项编号（1 起）换成下标；没有数字时认单独写的字母 A–H。超出范围、重复的不要。
func parseChoices(reply string, n int, multiple bool) []int {
	reply = stripThinking(reply)
	var picked []int
	seen := map[int]bool{}
	add := func(i int) bool {
		if i < 0 || i >= n || seen[i] {
			return false
		}
		seen[i] = true
		picked = append(picked, i)
		return !multiple
	}
	for _, m := range numberPattern.FindAllString(reply, -1) {
		v, err := strconv.Atoi(m)
		if err == nil && add(v-1) {
			return picked
		}
	}
	if len(picked) > 0 {
		return picked
	}
	for _, m := range letterPattern.FindAllStringSubmatch(reply, -1) {
		if add(int(m[1][0] - 'A')) {
			return picked
		}
	}
	return picked
}

// truncateRunes 截到 limit 个字。
func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}

// VisionTestResult 是「测试看图」的结果：现画一张验证码让模型认。Mismatch 是认错了几个字（字数不对时按缺的也算）。
type VisionTestResult struct {
	Model     string `json:"model"`
	Expected  string `json:"expected"`
	Got       string `json:"got"`
	OK        bool   `json:"ok"`
	Mismatch  int    `json:"mismatch"`
	LatencyMs int64  `json:"latency_ms"`
}

// mismatchCount 数两串按位置有几个字不同（不分大小写），长度差也算进去。
func mismatchCount(a, b string) int {
	ra, rb := []rune(strings.ToUpper(a)), []rune(strings.ToUpper(b))
	n := max(len(ra), len(rb))
	diff := 0
	for i := range n {
		if i >= len(ra) || i >= len(rb) || ra[i] != rb[i] {
			diff++
		}
	}
	return diff
}

// TestVision 用现画的 6 位验证码试一下看图模型（为空时用助手的模型）认不认得出。
func (s *Service) TestVision(ctx context.Context, visionModel string) (VisionTestResult, error) {
	eff, err := s.effectiveSettings(ctx)
	if err != nil {
		return VisionTestResult{}, err
	}
	gen := s.newCaptchaText
	if gen == nil {
		gen = randomCaptchaText
	}
	text := gen()
	start := time.Now()
	got, err := s.ReadCaptcha(ctx, visionModel, CaptchaRequest{Image: renderCaptcha(text), MIME: "image/png", Length: len(text), UpperOnly: true})
	if err != nil {
		return VisionTestResult{}, err
	}
	return VisionTestResult{
		Model: visionModelOr(visionModel, eff.llm.Model), Expected: text, Got: got,
		OK: strings.EqualFold(got, text), Mismatch: mismatchCount(text, got), LatencyMs: time.Since(start).Milliseconds(),
	}, nil
}
