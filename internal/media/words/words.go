// Package words 套用识别词：解析标题之前屏蔽或替换文字，解析之后按规则偏移集数。
package words

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/models"
)

// 识别词的长度与偏移上限。
const (
	MaxPatternLen = 256
	MaxOffset     = 9999
)

// Validate 检查一条识别词：种类、非空的匹配文字、正则能编译、集数偏移不为 0 且在范围内。
func Validate(r models.MediaWordRule) error {
	switch r.Kind {
	case models.MediaWordBlock, models.MediaWordReplace, models.MediaWordOffset:
	default:
		return fmt.Errorf("识别词种类无效: %q（可选 block、replace、offset）", r.Kind)
	}
	if strings.TrimSpace(r.Pattern) == "" {
		return errors.New("匹配文字不能为空")
	}
	if utf8.RuneCountInString(r.Pattern) > MaxPatternLen || utf8.RuneCountInString(r.Replacement) > MaxPatternLen {
		return fmt.Errorf("匹配文字与替换文字最多 %d 个字", MaxPatternLen)
	}
	if r.IsRegex {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("正则表达式无效: %w", err)
		}
	}
	if r.Kind == models.MediaWordOffset && (r.Offset == 0 || r.Offset > MaxOffset || r.Offset < -MaxOffset) {
		return fmt.Errorf("集数偏移要在 -%d 到 %d 之间，且不为 0", MaxOffset, MaxOffset)
	}
	return nil
}

// compile 把一条识别词编成正则：纯文字按字面匹配、不分大小写。
func compile(r models.MediaWordRule) (*regexp.Regexp, error) {
	if r.IsRegex {
		return regexp.Compile(r.Pattern)
	}
	return regexp.Compile(`(?i)` + regexp.QuoteMeta(r.Pattern))
}

// Applied 是套用识别词之后的标题与副标题，以及命中的规则与合计的集数偏移。
type Applied struct {
	Title    string
	Subtitle string
	Offset   int
	Hits     []uint
}

var spacesRe = regexp.MustCompile(`\s{2,}`)

func tidy(s string) string { return strings.TrimSpace(spacesRe.ReplaceAllString(s, " ")) }

// Apply 按顺序套用识别词（调用方只传启用的）。检查不过的规则跳过。
func Apply(rules []models.MediaWordRule, title, subtitle string) Applied {
	out := Applied{Title: tidy(title), Subtitle: tidy(subtitle)}
	for _, r := range rules {
		if Validate(r) != nil {
			continue
		}
		re, err := compile(r)
		if err != nil {
			continue
		}
		hit := re.MatchString(out.Title) || re.MatchString(out.Subtitle)
		if !hit {
			continue
		}
		switch r.Kind {
		case models.MediaWordBlock:
			out.Title = tidy(re.ReplaceAllString(out.Title, " "))
			out.Subtitle = tidy(re.ReplaceAllString(out.Subtitle, " "))
		case models.MediaWordReplace:
			repl := r.Replacement
			if !r.IsRegex {
				repl = strings.ReplaceAll(repl, "$", "$$")
			}
			out.Title = tidy(re.ReplaceAllString(out.Title, repl))
			out.Subtitle = tidy(re.ReplaceAllString(out.Subtitle, repl))
		case models.MediaWordOffset:
			out.Offset += r.Offset
		}
		out.Hits = append(out.Hits, r.ID)
	}
	return out
}

// ApplyOffset 把合计的集数偏移加到解析结果上；减到 1 以下的集数当作没有。
func (a Applied) ApplyOffset(m *meta.Meta) {
	if a.Offset == 0 || m == nil {
		return
	}
	shift := func(n int) int {
		if n == 0 {
			return 0
		}
		if n += a.Offset; n < 1 {
			return 0
		}
		return n
	}
	m.Episode = shift(m.Episode)
	m.EpisodeEnd = shift(m.EpisodeEnd)
	if m.EpisodeEnd != 0 && m.EpisodeEnd <= m.Episode {
		m.EpisodeEnd = 0
	}
}
