package assistant

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var tlouReview = CandidateReview{
	Kind: "tv", Title: "最后生还者", OriginalTitle: "The Last of Us", Year: 2023, Season: 2,
	Aliases:      []string{"最后生还者", "TLOU", " ", "美末"},
	TorrentTitle: "The.Last.of.Us.S02E01.2160p.WEB-DL.H265-GRP", TorrentSubtitle: "最后生还者 第二季 第01集",
	Parsed: "The Last of Us S02E01 2160p WEB-DL",
}

func TestReviewCandidate(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "是\n名字、季与集都对得上", usage: [2]int64{120, 8}})
	got, err := e.svc.ReviewCandidate(context.Background(), tlouReview)
	require.NoError(t, err)
	assert.Equal(t, ReviewVerdict{Match: true, Reason: "名字、季与集都对得上", Usage: Usage{Model: "fake-model", PromptTokens: 120, CompletionTokens: 8}}, got)
	reqs := e.llm.requests()
	require.Len(t, reqs, 1)
	assert.Empty(t, reqs[0].Tools, "不带工具")
	prompt := reqs[0].Messages[0]["content"].(string)
	assert.Contains(t, prompt, "请判断这个种子是不是这部剧的这一季")
	assert.Contains(t, prompt, "条目：剧集：最后生还者（The Last of Us，2023），第 2 季\n")
	assert.Contains(t, prompt, "别名：TLOU、美末\n", "和名字一样的、空的不写")
	assert.Contains(t, prompt, "种子标题：The.Last.of.Us.S02E01.2160p.WEB-DL.H265-GRP\n副标题：最后生还者 第二季 第01集\n从标题解析出：The Last of Us S02E01 2160p WEB-DL\n")

	e.llm.push(reply{text: "不是"})
	movie, err := e.svc.ReviewCandidate(context.Background(), CandidateReview{Kind: "movie", Title: "沙丘2", TorrentTitle: "Dune.1984.1080p"})
	require.NoError(t, err)
	assert.False(t, movie.Match)
	assert.Empty(t, movie.Reason)
	prompt = e.llm.requests()[1].Messages[0]["content"].(string)
	assert.Contains(t, prompt, "请判断这个种子是不是这部电影")
	assert.Contains(t, prompt, "条目：电影：沙丘2\n")
	assert.NotContains(t, prompt, "别名")
	assert.NotContains(t, prompt, "副标题")

	_, err = e.svc.ReviewCandidate(context.Background(), CandidateReview{Title: "x"})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestParseReview(t *testing.T) {
	for _, c := range []struct {
		reply  string
		match  bool
		reason string
		ok     bool
	}{
		{"是\n对得上", true, "对得上", true},
		{"不是\n这是第一季", false, "这是第一季", true},
		{"**不是**，年份差了十年", false, "年份差了十年", true},
		{"否。是另一部同名电影", false, "是另一部同名电影", true},
		{"<think>比一下……</think>\n是，名字一样", true, "名字一样", true},
		{"Yes. Same show and season", true, "Same show and season", true},
		{"No\nwrong season", false, "wrong season", true},
		{"Nothing to say", false, "", false},
		{"看不出来", false, "", false},
		{"", false, "", false},
	} {
		match, reason, ok := parseReview(c.reply)
		assert.Equal(t, c.ok, ok, c.reply)
		assert.Equal(t, c.match, match, c.reply)
		assert.Equal(t, c.reason, reason, c.reply)
	}
}

// 回答看不懂时是 ErrReviewUnclear；模型出错时的错误不带 Key；种子标题先遮掉像密钥的长串。
func TestReviewCandidateErrors(t *testing.T) {
	e := newEnv(t)
	e.llm.push(reply{text: "这个嘛"})
	_, err := e.svc.ReviewCandidate(context.Background(), tlouReview)
	require.ErrorIs(t, err, ErrReviewUnclear)

	e.llm.push(reply{status: http.StatusInternalServerError, body: "upstream down"})
	_, err = e.svc.ReviewCandidate(context.Background(), tlouReview)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testKey)

	e.llm.push(reply{text: "是"})
	in := tlouReview
	in.TorrentTitle = "Show.S02 passkey=0123456789abcdef0123456789abcdef"
	_, err = e.svc.ReviewCandidate(context.Background(), in)
	require.NoError(t, err)
	reqs := e.llm.requests()
	assert.NotContains(t, reqs[len(reqs)-1].Raw, "0123456789abcdef0123456789abcdef")
}
