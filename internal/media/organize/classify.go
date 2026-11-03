package organize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// ClassifyRule 是一条二级分类：条件之间是「且」，同一个条件里的几个值是「或」，空的条件不限。
type ClassifyRule struct {
	Name string `json:"name"`
	// Genres 是 TMDB 的类型编号（16 动画、99 纪录……）
	Genres []int `json:"genres,omitempty"`
	// Languages 是原始语言（ISO 639-1，例如 zh、ja；TMDB 把粤语记作 cn）
	Languages []string `json:"languages,omitempty"`
	// Countries 是出品国家或地区（ISO 3166-1，例如 CN、US）
	Countries []string `json:"countries,omitempty"`
	// MaxRuntime 与 Keywords 合成一个条件（填了哪个看哪个，两个都填时是「或」）：片长（分钟，剧集是一集）不超过 MaxRuntime，
	// 或者标题（种子名、站点标题）里有其中一个关键字（不分大小写）。片长未知时片长这一半对不上。
	MaxRuntime int      `json:"max_runtime,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
}

// ClassifyNote 说明这次放进哪个分类、为什么：Rule 是对上的第几条规则（从 1 数），0 是都没对上；其余是对上的条件。
type ClassifyNote struct {
	Rule int    `json:"rule"`
	Name string `json:"name,omitempty"`
	// Rules 是这一类（电影或剧集）的规则数
	Rules     int      `json:"rules"`
	Genres    []int    `json:"genres,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Countries []string `json:"countries,omitempty"`
	// Runtime 是对上「片长不超过」时条目的片长（分钟），MaxRuntime 是规则里的上限
	Runtime    int `json:"runtime,omitempty"`
	MaxRuntime int `json:"max_runtime,omitempty"`
	// Keyword 是标题里对上的关键字
	Keyword string `json:"keyword,omitempty"`
}

// ClassifyRules 是电影与剧集各自的二级分类，按顺序取第一条对得上的；都对不上时不分类。
type ClassifyRules struct {
	Movie []ClassifyRule `json:"movie"`
	TV    []ClassifyRule `json:"tv"`
}

// DefaultClassifyRules 是常用的二级分类（和 MoviePilot 默认的分类一样）：最后一条不带条件，其余的都放进它。
var DefaultClassifyRules = ClassifyRules{
	Movie: []ClassifyRule{
		{Name: "动画电影", Genres: []int{16}},
		{Name: "华语电影", Languages: []string{"zh", "cn", "bo", "za"}},
		{Name: "外语电影"},
	},
	TV: []ClassifyRule{
		{Name: "国漫", Genres: []int{16}, Countries: []string{"CN", "TW", "HK"}},
		{Name: "日番", Genres: []int{16}, Countries: []string{"JP"}},
		{Name: "纪录片", Genres: []int{99}},
		{Name: "儿童", Genres: []int{10762}},
		{Name: "综艺", Genres: []int{10764, 10767}},
		// 短剧放在按类型分的几条后面：一集十来分钟的动画、儿童节目先进它们自己的分类
		{Name: "短剧", MaxRuntime: 15, Keywords: []string{"短剧"}},
		{Name: "国产剧", Countries: []string{"CN", "TW", "HK"}},
		{Name: "欧美剧", Countries: []string{"US", "FR", "GB", "DE", "ES", "IT", "NL", "PT", "RU", "UK"}},
		{Name: "日韩剧", Countries: []string{"JP", "KP", "KR", "TH", "IN", "SG"}},
		{Name: "未分类"},
	},
}

// 上限。
const (
	maxClassifyRules   = 50
	maxClassifyValues  = 50
	maxClassifyName    = 32
	maxClassifyRuntime = 600
	maxClassifyKeyword = 32
)

var (
	langRe    = regexp.MustCompile(`^[a-z]{2,3}$`)
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Classify 是条目的二级分类；没有规则或都对不上时为空串。titles 是关键字要看的标题（种子名、站点标题）。
func (c ClassifyRules) Classify(r tmdb.Result, titles ...string) string {
	return c.Explain(r, titles...).Name
}

// Explain 是条目放进哪个分类与原因（见 ClassifyNote）。
func (c ClassifyRules) Explain(r tmdb.Result, titles ...string) ClassifyNote {
	rules := c.Movie
	if r.MediaType == tmdb.KindTV {
		rules = c.TV
	}
	for i, rule := range rules {
		if n, ok := rule.match(r, titles); ok {
			n.Rule, n.Name, n.Rules = i+1, rule.Name, len(rules)
			return n
		}
	}
	return ClassifyNote{Rules: len(rules)}
}

// match 报告条目对不对得上这条规则；对得上时回对上的条件。
func (rule ClassifyRule) match(r tmdb.Result, titles []string) (ClassifyNote, bool) {
	var n ClassifyNote
	if len(rule.Genres) > 0 {
		for _, g := range rule.Genres {
			if slices.Contains(r.GenreIDs, g) {
				n.Genres = append(n.Genres, g)
			}
		}
		if len(n.Genres) == 0 {
			return n, false
		}
	}
	if len(rule.Languages) > 0 {
		lang := strings.ToLower(r.OriginalLanguage)
		if !slices.Contains(rule.Languages, lang) {
			return n, false
		}
		n.Languages = []string{lang}
	}
	if len(rule.Countries) > 0 {
		for _, c := range rule.Countries {
			if slices.Contains(r.Countries, c) {
				n.Countries = append(n.Countries, c)
			}
		}
		if len(n.Countries) == 0 {
			return n, false
		}
	}
	if rule.MaxRuntime > 0 || len(rule.Keywords) > 0 {
		if rule.MaxRuntime > 0 && r.Runtime > 0 && r.Runtime <= rule.MaxRuntime {
			n.Runtime, n.MaxRuntime = r.Runtime, rule.MaxRuntime
		}
		n.Keyword = titleKeyword(rule.Keywords, titles)
		if n.Runtime == 0 && n.Keyword == "" {
			return n, false
		}
	}
	return n, true
}

// titleKeyword 是标题里有的第一个关键字（不分大小写），没有时为空串。
func titleKeyword(keywords, titles []string) string {
	for _, k := range keywords {
		lk := strings.ToLower(k)
		for _, t := range titles {
			if strings.Contains(strings.ToLower(t), lk) {
				return k
			}
		}
	}
	return ""
}

// clean 检查并规整规则：名称能当目录名、不重复；语言转小写、国家或地区转大写，去掉重复。
func (c ClassifyRules) clean() (ClassifyRules, error) {
	movie, err := cleanRules(c.Movie, "电影")
	if err != nil {
		return ClassifyRules{}, err
	}
	tv, err := cleanRules(c.TV, "剧集")
	if err != nil {
		return ClassifyRules{}, err
	}
	return ClassifyRules{Movie: movie, TV: tv}, nil
}

func cleanRules(in []ClassifyRule, what string) ([]ClassifyRule, error) {
	if len(in) > maxClassifyRules {
		return nil, fmt.Errorf("%w: %s的分类最多 %d 条", ErrInvalid, what, maxClassifyRules)
	}
	out := make([]ClassifyRule, 0, len(in))
	seen := map[string]bool{}
	for _, r := range in {
		name := strings.TrimSpace(r.Name)
		if name == "" || utf8.RuneCountInString(name) > maxClassifyName || transfer.CleanName(name) != name {
			return nil, fmt.Errorf("%w: %s的分类名称「%s」要能当目录名（不能为空、最多 %d 个字，不能有 / \\ : * ? \" < > |）", ErrInvalid, what, name, maxClassifyName)
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: %s的分类「%s」重复了", ErrInvalid, what, name)
		}
		seen[name] = true
		if len(r.Genres) > maxClassifyValues || len(r.Languages) > maxClassifyValues || len(r.Countries) > maxClassifyValues || len(r.Keywords) > maxClassifyValues {
			return nil, fmt.Errorf("%w: 分类「%s」的条件太多", ErrInvalid, name)
		}
		if r.MaxRuntime < 0 || r.MaxRuntime > maxClassifyRuntime {
			return nil, fmt.Errorf("%w: 分类「%s」的片长上限要在 0 到 %d 分钟之间（0 是不看片长）", ErrInvalid, name, maxClassifyRuntime)
		}
		rule := ClassifyRule{Name: name, MaxRuntime: r.MaxRuntime}
		for _, k := range r.Keywords {
			k = strings.TrimSpace(k)
			if k == "" || utf8.RuneCountInString(k) > maxClassifyKeyword {
				return nil, fmt.Errorf("%w: 分类「%s」的关键字不能为空，最多 %d 个字", ErrInvalid, name, maxClassifyKeyword)
			}
			if !slices.Contains(rule.Keywords, k) {
				rule.Keywords = append(rule.Keywords, k)
			}
		}
		for _, g := range r.Genres {
			if g <= 0 {
				return nil, fmt.Errorf("%w: 分类「%s」的类型编号不对", ErrInvalid, name)
			}
			if !slices.Contains(rule.Genres, g) {
				rule.Genres = append(rule.Genres, g)
			}
		}
		for _, l := range r.Languages {
			l = strings.ToLower(strings.TrimSpace(l))
			if !langRe.MatchString(l) {
				return nil, fmt.Errorf("%w: 分类「%s」的语言「%s」要写 ISO 639-1 代码（例如 zh、ja）", ErrInvalid, name, l)
			}
			if !slices.Contains(rule.Languages, l) {
				rule.Languages = append(rule.Languages, l)
			}
		}
		for _, ct := range r.Countries {
			ct = strings.ToUpper(strings.TrimSpace(ct))
			if !countryRe.MatchString(ct) {
				return nil, fmt.Errorf("%w: 分类「%s」的国家或地区「%s」要写 ISO 3166-1 代码（例如 CN、US）", ErrInvalid, name, ct)
			}
			if !slices.Contains(rule.Countries, ct) {
				rule.Countries = append(rule.Countries, ct)
			}
		}
		out = append(out, rule)
	}
	return out, nil
}

// ClassifyView 是二级分类的规则（没设过时两类都是空的）与常用的预设。
type ClassifyView struct {
	Rules   ClassifyRules `json:"rules"`
	Presets ClassifyRules `json:"presets"`
}

// decodeClassify 读存着的规则：空串是没有规则；不是合法的 JSON 时报错（不能当成没有规则，那样会悄悄放错目录）。
func decodeClassify(raw string) (ClassifyRules, error) {
	var c ClassifyRules
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return ClassifyRules{}, fmt.Errorf("二级分类的规则读不出来（在「媒体库 → 二级分类」里重新保存）: %w", err)
		}
	}
	if c.Movie == nil {
		c.Movie = []ClassifyRule{}
	}
	if c.TV == nil {
		c.TV = []ClassifyRule{}
	}
	return c, nil
}

// Classify 读二级分类的规则。
func (s *Service) Classify(ctx context.Context) (ClassifyView, error) {
	row, err := s.loadSettings(ctx)
	if err != nil {
		return ClassifyView{}, err
	}
	rules, err := decodeClassify(row.ClassifyRules)
	if err != nil {
		return ClassifyView{}, err
	}
	return ClassifyView{Rules: rules, Presets: DefaultClassifyRules}, nil
}

// SaveClassify 保存二级分类的规则（整份替换）。
func (s *Service) SaveClassify(ctx context.Context, in ClassifyRules) (ClassifyView, error) {
	rules, err := in.clean()
	if err != nil {
		return ClassifyView{}, err
	}
	b, err := json.Marshal(rules)
	if err != nil {
		return ClassifyView{}, err
	}
	s.setMu.Lock()
	defer s.setMu.Unlock()
	row, err := s.loadSettings(ctx)
	if err != nil {
		return ClassifyView{}, err
	}
	row.ClassifyRules = string(b)
	row.UpdatedAt = s.cfg.Now()
	if err := s.cfg.DB.WithContext(ctx).Save(&row).Error; err != nil {
		return ClassifyView{}, fmt.Errorf("保存二级分类失败: %w", err)
	}
	return ClassifyView{Rules: rules, Presets: DefaultClassifyRules}, nil
}

// classify 是这次整理放进的分类：媒体库没开「按二级分类放进子目录」、没有规则或都对不上时为空串。有规则时 p.Classify 写明原因。
// 规则要用详情里才有的字段（电影的出品国家或地区）：识别时详情没取到、用的是搜索结果的，这里再取一次详情（多半读缓存）。
// 取不到时这次不整理（写明原因、过一会儿再试），免得按不全的条目放错目录。第二个返回值为假时 p.Problem 写明了原因。
func (s *Service) classify(ctx context.Context, p *Plan, lib *models.MediaLibrary) (string, bool) {
	if lib == nil || !lib.ClassifyDir {
		return "", true
	}
	row, err := s.loadSettings(ctx)
	if err != nil {
		p.FailKind = models.MediaFailLibrary
		p.Problem, p.retryable = "读取二级分类失败："+err.Error(), true
		return "", false
	}
	rules, err := decodeClassify(row.ClassifyRules)
	if err != nil {
		p.FailKind = models.MediaFailLibrary
		p.Problem = err.Error()
		return "", false
	}
	list := rules.Movie
	if p.Match.MediaType == tmdb.KindTV {
		list = rules.TV
	}
	if len(list) == 0 {
		return "", true
	}
	if p.tmdbClient == nil {
		p.FailKind = models.MediaFailRecognize
		p.Problem, p.retryable = "按二级分类放进子目录要用 TMDB 的详情：TMDB 暂时不能访问", true
		return "", false
	}
	d, err := p.tmdbClient.Details(ctx, p.Match.MediaType, p.Match.ID)
	if err != nil {
		p.FailKind = models.MediaFailRecognize
		p.Problem, p.retryable = "取 TMDB 详情失败（二级分类要用）："+err.Error(), !errors.Is(err, tmdb.ErrNotFound)
		return "", false
	}
	p.Match = d
	note := rules.Explain(*d, p.Name, p.siteTitle)
	p.Classify = &note
	return note.Name, true
}
