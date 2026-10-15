package organize

import (
	"context"
	"encoding/json"
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
		{Name: "国产剧", Countries: []string{"CN", "TW", "HK"}},
		{Name: "欧美剧", Countries: []string{"US", "FR", "GB", "DE", "ES", "IT", "NL", "PT", "RU", "UK"}},
		{Name: "日韩剧", Countries: []string{"JP", "KP", "KR", "TH", "IN", "SG"}},
		{Name: "未分类"},
	},
}

// 上限。
const (
	maxClassifyRules  = 50
	maxClassifyValues = 50
	maxClassifyName   = 32
)

var (
	langRe    = regexp.MustCompile(`^[a-z]{2,3}$`)
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Classify 是条目的二级分类；没有规则或都对不上时为空串。
func (c ClassifyRules) Classify(r tmdb.Result) string {
	rules := c.Movie
	if r.MediaType == tmdb.KindTV {
		rules = c.TV
	}
	for _, rule := range rules {
		if rule.matches(r) {
			return rule.Name
		}
	}
	return ""
}

func (rule ClassifyRule) matches(r tmdb.Result) bool {
	if len(rule.Genres) > 0 && !slices.ContainsFunc(rule.Genres, func(g int) bool { return slices.Contains(r.GenreIDs, g) }) {
		return false
	}
	if len(rule.Languages) > 0 && !slices.Contains(rule.Languages, strings.ToLower(r.OriginalLanguage)) {
		return false
	}
	if len(rule.Countries) > 0 && !slices.ContainsFunc(rule.Countries, func(c string) bool { return slices.Contains(r.Countries, c) }) {
		return false
	}
	return true
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
		if len(r.Genres) > maxClassifyValues || len(r.Languages) > maxClassifyValues || len(r.Countries) > maxClassifyValues {
			return nil, fmt.Errorf("%w: 分类「%s」的条件太多", ErrInvalid, name)
		}
		rule := ClassifyRule{Name: name}
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

func decodeClassify(raw string) ClassifyRules {
	var c ClassifyRules
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	if c.Movie == nil {
		c.Movie = []ClassifyRule{}
	}
	if c.TV == nil {
		c.TV = []ClassifyRule{}
	}
	return c
}

// Classify 读二级分类的规则。
func (s *Service) Classify(ctx context.Context) (ClassifyView, error) {
	row, err := s.loadSettings(ctx)
	if err != nil {
		return ClassifyView{}, err
	}
	return ClassifyView{Rules: decodeClassify(row.ClassifyRules), Presets: DefaultClassifyRules}, nil
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
	return ClassifyView{Rules: decodeClassify(row.ClassifyRules), Presets: DefaultClassifyRules}, nil
}

// classifyFor 是这次整理放进的分类：媒体库没开「按二级分类放进子目录」、没有规则或都对不上时为空串。
func (s *Service) classifyFor(ctx context.Context, lib *models.MediaLibrary, r tmdb.Result) (string, error) {
	if lib == nil || !lib.ClassifyDir {
		return "", nil
	}
	row, err := s.loadSettings(ctx)
	if err != nil {
		return "", err
	}
	return decodeClassify(row.ClassifyRules).Classify(r), nil
}
