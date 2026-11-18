package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/sunerpy/pt-tools/internal/assistant"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
)

// aiRecognizeCandidates 是「让 AI 认」最多给模型看几个候选。
const aiRecognizeCandidates = 8

// aiRecognizeResult 是「让 AI 认」的结果。Input 是认的标题（种子名与站点标题，「记成纠正」用它）；Chosen 是选中的候选下标，
// -1 是都不是（或没有候选、没调模型）；选中时 Request 是带上这个条目的整理请求，Preview 是按它算出的整理预览。只认、不整理。
type aiRecognizeResult struct {
	Input            recognize.Input       `json:"input"`
	Candidates       []recognize.Candidate `json:"candidates"`
	Chosen           int                   `json:"chosen"`
	Reason           string                `json:"reason,omitempty"`
	Model            string                `json:"model,omitempty"`
	PromptTokens     int64                 `json:"prompt_tokens,omitempty"`
	CompletionTokens int64                 `json:"completion_tokens,omitempty"`
	Request          *organize.Request     `json:"request,omitempty"`
	Preview          *organize.Plan        `json:"preview,omitempty"`
}

// apiOrganizeAIRecognize handles POST /api/media/organize/ai-recognize：请求体同整理预览（下载器与种子、监控目录的一项或一个路径）
func (s *Server) apiOrganizeAIRecognize(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	var in organize.Request
	if !decodeStrictBody(w, r, &in) {
		return
	}
	s.aiRecognize(w, r, svc, in)
}

// apiMediaHistoryAIRecognize handles POST /api/media/history/{id}/ai-recognize：认这条整理记录所在的种子（不带之前指定的条目）
func (s *Server) apiMediaHistoryAIRecognize(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.organizeService(w)
	if !ok {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	req, err := svc.HistoryRequest(r.Context(), id)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	s.aiRecognize(w, r, svc, req)
}

// aiUnavailable 是「让 AI 认」现在用不了的原因（AI 助手没启动、没启用或没配好），能用时为空串。
func (s *Server) aiUnavailable(ctx context.Context) string {
	const off = "AI 助手没有启用：先在「AI 助手」里配好接口地址、Key 与模型"
	if s.assistant == nil {
		return off
	}
	if err := s.assistant.Ready(ctx); err != nil {
		if errors.Is(err, assistant.ErrDisabled) {
			return off
		}
		return "AI 助手不能用：" + err.Error()
	}
	return ""
}

// apiOrganizeAIStatus handles GET /api/media/organize/ai-status：「让 AI 认」能不能用，不能用时写明原因（不调模型）
func (s *Server) apiOrganizeAIStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.organizeService(w); !ok {
		return
	}
	if msg := s.aiUnavailable(r.Context()); msg != "" {
		writeJSON(w, map[string]any{"ready": false, "reason": msg})
		return
	}
	writeJSON(w, map[string]any{"ready": true})
}

// aiRecognize 搜候选、让助手的模型挑一个，选中时按它算整理预览：助手没配好时 409，没认出条目以外的问题 400，
// 没有候选时不调模型。
func (s *Server) aiRecognize(w http.ResponseWriter, r *http.Request, svc *organize.Service, req organize.Request) {
	rec, ok := s.mediaService(w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), organizeAPITimeout)
	defer cancel()
	if msg := s.aiUnavailable(ctx); msg != "" {
		http.Error(w, msg, http.StatusConflict)
		return
	}
	in, err := svc.RecognizeInput(ctx, req)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	cands, err := rec.Candidates(ctx, in, aiRecognizeCandidates)
	if err != nil {
		writeOrganizeError(w, err)
		return
	}
	out := aiRecognizeResult{Input: in, Candidates: cands, Chosen: -1}
	if len(cands) == 0 {
		out.Reason = "TMDB 上没有搜到候选：换个名字搜，或手动填 TMDB 编号"
		writeJSON(w, out)
		return
	}
	pick := make([]assistant.MediaCandidate, len(cands))
	for i, c := range cands {
		pick[i] = assistant.MediaCandidate{Kind: c.MediaType, Title: c.Title, OriginalTitle: c.OriginalTitle, Year: c.Year, Overview: c.Overview}
	}
	choice, err := s.assistant.ChooseMedia(ctx, in.Title, in.Subtitle, pick)
	if err != nil {
		http.Error(w, "AI 没认成："+err.Error(), http.StatusBadGateway)
		return
	}
	out.Chosen, out.Reason = choice.Index, choice.Reason
	out.Model, out.PromptTokens, out.CompletionTokens = choice.Usage.Model, choice.Usage.PromptTokens, choice.Usage.CompletionTokens
	if choice.Index >= 0 {
		c := cands[choice.Index]
		req.MediaType, req.TMDBID = c.MediaType, c.ID
		plan, err := svc.Preview(ctx, req)
		if err != nil {
			writeOrganizeError(w, err)
			return
		}
		out.Request, out.Preview = &req, plan
	}
	writeJSON(w, out)
}
