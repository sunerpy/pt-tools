package organize

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

// RecognizeInput 是「让 AI 认」要的输入：按整理计划走到按名字识别这一步时用的种子名（监控目录的一项、指定的路径是它的名字）
// 与站点上的标题。只在整理卡在没认出条目时有；认出来了、别的问题（没有媒体库等）、没走到按名字识别（例如没填 TMDB API Key）时
// 返回 ErrInvalid 并写明。不改动任何文件。
func (s *Service) RecognizeInput(ctx context.Context, req Request) (recognize.Input, error) {
	p, err := s.Preview(ctx, req)
	if err != nil {
		return recognize.Input{}, err
	}
	switch {
	case p.Problem == "":
		return recognize.Input{}, fmt.Errorf("%w: 已经认出了条目，不用 AI 认", ErrInvalid)
	case p.FailKind != models.MediaFailRecognize || p.recognizeIn.Title == "":
		return recognize.Input{}, fmt.Errorf("%w: 这次整理不了不是因为没认出条目：%s", ErrInvalid, p.Problem)
	}
	return p.recognizeIn, nil
}

// HistoryRequest 是重新整理这条记录的请求（不带之前指定的条目）：「让 AI 认」从整理历史里认它。
func (s *Service) HistoryRequest(ctx context.Context, id uint) (Request, error) {
	row, err := s.historyRow(ctx, id)
	if err != nil {
		return Request{}, err
	}
	return requestOf(row), nil
}
