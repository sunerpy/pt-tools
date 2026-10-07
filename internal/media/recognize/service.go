// Package recognize 是媒体识别服务：套用识别词、解析标题，再到 TMDB 找对应的条目
// （有 IMDb 编号时直接查找，否则按中英文名和年份搜索打分），支持按 TMDB 编号手动纠正。
// 也管 TMDB 的设置（API Key 与代理加密保存、只写不读）与识别词。
package recognize

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/time/rate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/words"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

var (
	// ErrInvalid 表示参数不对（错误信息写明哪一项）。
	ErrInvalid = errors.New("参数无效")
	// ErrNotFound 表示要改或删的记录不存在。
	ErrNotFound = errors.New("记录不存在")
)

// Cipher 加解密 API Key 与代理地址（生产环境是 ConfigStore 的 EncryptCookie / DecryptCookie）。
type Cipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(cipherText string) (string, error)
}

// Config 是服务的依赖。
type Config struct {
	DB     *gorm.DB
	Cipher Cipher
	// BaseURL 是 TMDB 接口地址；为空时用官方地址（qa 构建可指向假服务）。
	BaseURL string
	// ImageBaseURL 是 TMDB 图片地址（到 /t/p/ 为止）；为空时用官方地址（qa 构建可指向假服务）。
	ImageBaseURL string
	// RatePerSecond 是 TMDB 请求的限速（为 0 时用客户端的默认值）。
	RatePerSecond float64
}

// Service 是媒体识别服务。
type Service struct {
	cfg   Config
	cache *tmdb.DBCache
	mu    sync.Mutex // 串行化设置、纠正与识别词的写入
	// limiter 是所有 TMDB 请求共用的限速额度：每次识别都会新建客户端，额度不能跟着重置
	limiter *rate.Limiter
	// httpMu 保护按代理地址缓存的 http.Client（同一个代理共用连接池）
	httpMu    sync.Mutex
	httpProxy string
	httpOne   *http.Client
}

// New 建一个服务，顺便清掉过期的 TMDB 缓存。
func New(cfg Config) *Service {
	s := &Service{cfg: cfg, cache: tmdb.NewDBCache(cfg.DB), limiter: tmdb.NewLimiter(cfg.RatePerSecond)}
	_ = s.cache.Purge()
	return s
}

// httpClient 是走 proxy 的 http.Client：代理没变时复用上一次的。
func (s *Service) httpClient(proxy string) (*http.Client, error) {
	s.httpMu.Lock()
	defer s.httpMu.Unlock()
	if s.httpOne != nil && s.httpProxy == proxy {
		return s.httpOne, nil
	}
	hc, err := tmdb.NewHTTPClient(proxy)
	if err != nil {
		return nil, err
	}
	s.httpOne, s.httpProxy = hc, proxy
	return hc, nil
}

// Languages 是可选的 TMDB 语言（第一个是默认值）。
var Languages = []string{"zh-CN", "zh-TW", "zh-HK", "en-US", "ja-JP"}

// 长度上限。
const (
	maxInputLen = 512
	maxKeyLen   = 512
	maxWords    = 500
)

// Settings 是给界面看的设置：API Key 只说有没有；代理地址里的密码换成 ***。
type Settings struct {
	HasTMDBKey bool   `json:"has_tmdb_key"`
	Language   string `json:"language"`
	ProxyURL   string `json:"proxy_url"`
}

// SettingsInput 是保存设置的参数。TMDBKey、ProxyURL 为 nil 时不改，为空串时清除。
// ProxyURL 与 Settings 返回的（密码换成 *** 的）地址相同时也不改。
type SettingsInput struct {
	TMDBKey  *string `json:"tmdb_api_key,omitempty"`
	Language string  `json:"language"`
	ProxyURL *string `json:"proxy_url,omitempty"`
}

func (s *Service) load(ctx context.Context) (models.MediaSetting, error) {
	var row models.MediaSetting
	if err := s.cfg.DB.WithContext(ctx).Where("id = 1").Limit(1).Find(&row).Error; err != nil {
		return row, fmt.Errorf("读取媒体识别设置失败: %w", err)
	}
	return row, nil
}

// maskProxy 把代理地址里的密码换成 ***。
func maskProxy(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, ok := u.User.Password(); ok {
		u.User = url.UserPassword(u.User.Username(), "***")
	}
	return strings.Replace(u.String(), "%2A%2A%2A", "***", 1)
}

func (s *Service) view(row models.MediaSetting) Settings {
	out := Settings{HasTMDBKey: row.TMDBKeyEncrypted != "", Language: row.Language}
	if out.Language == "" {
		out.Language = Languages[0]
	}
	if row.ProxyEncrypted != "" {
		if p, err := s.cfg.Cipher.Decrypt(row.ProxyEncrypted); err == nil {
			out.ProxyURL = maskProxy(p)
		}
	}
	return out
}

// Settings 读设置。
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	row, err := s.load(ctx)
	if err != nil {
		return Settings{}, err
	}
	return s.view(row), nil
}

// SaveSettings 保存设置。
func (s *Service) SaveSettings(ctx context.Context, in SettingsInput) (Settings, error) {
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		lang = Languages[0]
	}
	if !slices.Contains(Languages, lang) {
		return Settings{}, fmt.Errorf("%w: 语言只能是 %s", ErrInvalid, strings.Join(Languages, "、"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.load(ctx)
	if err != nil {
		return Settings{}, err
	}
	row.ID, row.Language = 1, lang
	if in.TMDBKey != nil {
		key := strings.TrimSpace(*in.TMDBKey)
		switch {
		case key == "":
			row.TMDBKeyEncrypted = ""
		case len(key) < 16 || len(key) > maxKeyLen || strings.ContainsAny(key, " \t\r\n"):
			return Settings{}, fmt.Errorf("%w: TMDB API Key 格式不对（v3 的 API Key 或 v4 的读取令牌）", ErrInvalid)
		default:
			enc, err := s.cfg.Cipher.Encrypt(key)
			if err != nil {
				return Settings{}, fmt.Errorf("加密 TMDB API Key 失败: %w", err)
			}
			row.TMDBKeyEncrypted = enc
		}
	}
	if in.ProxyURL != nil {
		proxy := strings.TrimSpace(*in.ProxyURL)
		switch {
		case proxy == "":
			row.ProxyEncrypted = ""
		case proxy == s.view(row).ProxyURL:
			// 界面原样提交了密码换成 *** 的地址：不改
		default:
			if _, err := tmdb.ParseProxyURL(proxy); err != nil {
				return Settings{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
			}
			enc, err := s.cfg.Cipher.Encrypt(proxy)
			if err != nil {
				return Settings{}, fmt.Errorf("加密代理地址失败: %w", err)
			}
			row.ProxyEncrypted = enc
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"tmdb_key_encrypted", "language", "proxy_encrypted", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return Settings{}, fmt.Errorf("保存媒体识别设置失败: %w", err)
	}
	return s.view(row), nil
}

// client 按当前设置建 TMDB 客户端；没有 API Key 时返回 tmdb.ErrNoKey。
func (s *Service) client(ctx context.Context) (*tmdb.Client, error) {
	row, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	if row.TMDBKeyEncrypted == "" {
		return nil, tmdb.ErrNoKey
	}
	key, err := s.cfg.Cipher.Decrypt(row.TMDBKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("解密 TMDB API Key 失败: %w", err)
	}
	proxy := ""
	if row.ProxyEncrypted != "" {
		if proxy, err = s.cfg.Cipher.Decrypt(row.ProxyEncrypted); err != nil {
			return nil, fmt.Errorf("解密代理地址失败: %w", err)
		}
	}
	hc, err := s.httpClient(proxy)
	if err != nil {
		return nil, err
	}
	return tmdb.New(tmdb.Options{
		APIKey: key, BaseURL: s.cfg.BaseURL, ImageBaseURL: s.cfg.ImageBaseURL, Language: s.view(row).Language,
		Cache: s.cache, Limiter: s.limiter, HTTPClient: hc,
	})
}

// TMDB 按当前设置建 TMDB 客户端（整理入库时取详情、季详情与图片）；没有 API Key 时返回 tmdb.ErrNoKey。
func (s *Service) TMDB(ctx context.Context) (*tmdb.Client, error) {
	return s.client(ctx)
}

// Parse 套用启用的识别词后解析一个标题（整理入库时解析每个文件名）。
func (s *Service) Parse(ctx context.Context, title, subtitle string) (meta.Meta, error) {
	m, _, err := s.parse(ctx, Input{Title: title, Subtitle: subtitle})
	return m, err
}

// TestTMDB 用当前设置访问一次 TMDB，确认 API Key 与代理可用。
func (s *Service) TestTMDB(ctx context.Context) error {
	c, err := s.client(ctx)
	if err != nil {
		return err
	}
	return c.Validate(ctx)
}

// Input 是识别的输入。
type Input struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	IMDbID   string `json:"imdb_id"`
}

// Candidate 是一个打过分的候选条目。
type Candidate struct {
	tmdb.Result
	Score float64 `json:"score"`
}

// 识别结果的来源。
const (
	SourceNone     = "none"
	SourceOverride = "override"
	SourceIMDb     = "imdb"
	SourceSearch   = "search"
)

// Result 是识别结果。TMDB 出错时 Meta 照样有，Error 写明原因。
type Result struct {
	Meta       meta.Meta    `json:"meta"`
	Summary    string       `json:"summary"`
	RuleHits   []uint       `json:"rule_hits,omitempty"`
	Match      *tmdb.Result `json:"match,omitempty"`
	Source     string       `json:"source"`
	Score      float64      `json:"score,omitempty"`
	Candidates []Candidate  `json:"candidates,omitempty"`
	OverrideID uint         `json:"override_id,omitempty"`
	Message    string       `json:"message,omitempty"`
	Error      string       `json:"error,omitempty"`
}

func cleanInput(in Input) (Input, error) {
	in.Title, in.Subtitle, in.IMDbID = strings.TrimSpace(in.Title), strings.TrimSpace(in.Subtitle), strings.TrimSpace(in.IMDbID)
	if in.Title == "" && in.Subtitle == "" {
		return in, fmt.Errorf("%w: 标题与副标题不能都为空", ErrInvalid)
	}
	if utf8.RuneCountInString(in.Title) > maxInputLen || utf8.RuneCountInString(in.Subtitle) > maxInputLen {
		return in, fmt.Errorf("%w: 标题与副标题最多 %d 个字", ErrInvalid, maxInputLen)
	}
	return in, nil
}

// parse 套用启用的识别词后解析标题。
func (s *Service) parse(ctx context.Context, in Input) (meta.Meta, []uint, error) {
	var rules []models.MediaWordRule
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rules).Error; err != nil {
		return meta.Meta{}, nil, fmt.Errorf("读取识别词失败: %w", err)
	}
	applied := words.Apply(rules, in.Title, in.Subtitle)
	m := meta.Parse(applied.Title, applied.Subtitle)
	applied.ApplyOffset(&m)
	return m, applied.Hits, nil
}

// Recognize 识别一个标题：先看手动纠正，再按 IMDb 编号查找，最后按名字搜索打分。
func (s *Service) Recognize(ctx context.Context, in Input) (*Result, error) {
	in, err := cleanInput(in)
	if err != nil {
		return nil, err
	}
	m, hits, err := s.parse(ctx, in)
	if err != nil {
		return nil, err
	}
	res := &Result{Meta: m, Summary: m.String(), RuleHits: hits, Source: SourceNone}
	c, err := s.client(ctx)
	if errors.Is(err, tmdb.ErrNoKey) {
		res.Message = "没有填写 TMDB API Key，只做了标题解析"
		return res, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.match(ctx, c, m, v2.NormalizeIMDbID(in.IMDbID), res); err != nil {
		res.Error = err.Error()
	}
	if res.Match == nil && res.Error == "" {
		res.Message = "TMDB 上没有找到可靠的匹配：可以在候选里选一个，或填 TMDB 编号纠正"
	}
	return res, nil
}

func (s *Service) match(ctx context.Context, c *tmdb.Client, m meta.Meta, imdb string, res *Result) error {
	ov, err := s.findOverride(ctx, m)
	if err != nil {
		return err
	}
	if ov.ID != 0 {
		d, detailErr := c.Details(ctx, ov.MediaType, ov.TMDBID)
		if detailErr != nil {
			return detailErr
		}
		res.Match, res.Source, res.OverrideID = d, SourceOverride, ov.ID
		return nil
	}
	if imdb != "" {
		found, findErr := c.Find(ctx, imdb)
		if findErr != nil && !errors.Is(findErr, tmdb.ErrNotFound) {
			return findErr
		}
		if r := pickFound(m, found); r != nil {
			res.Match, res.Source = s.details(ctx, c, *r), SourceIMDb
			return nil
		}
	}
	cands, err := s.search(ctx, c, m, primaryKinds(m))
	if err != nil {
		return err
	}
	// 只靠年份判成电影的（没有季集标记）也可能是剧集：电影里没有可靠匹配时补搜剧集（电影的搜索读缓存）
	if pick(m, cands) == nil && m.Type == meta.TypeMovie {
		if cands, err = s.search(ctx, c, m, []string{tmdb.KindMovie, tmdb.KindTV}); err != nil {
			return err
		}
	}
	best := pick(m, cands)
	if best == nil {
		best = pickByOtherTitles(ctx, c, m, cands)
	}
	if best != nil {
		res.Match, res.Source, res.Score = s.details(ctx, c, best.Result), SourceSearch, best.Score
	}
	if len(cands) > 5 {
		cands = cands[:5]
	}
	res.Candidates = cands
	return nil
}

// findOverride 找这个解析结果的手动纠正：先按英文名、再按中文名找主键，最后按中文名找别名。没有时返回零值。
func (s *Service) findOverride(ctx context.Context, m meta.Meta) (models.MediaOverride, error) {
	en, cn := overrideKey(m, m.NameEN), overrideKey(m, m.NameCN)
	var keys []string
	for _, k := range []string{en, cn} {
		if k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return models.MediaOverride{}, nil
	}
	q := s.cfg.DB.WithContext(ctx).Where("key IN ?", keys)
	if cn != "" {
		q = q.Or("alt_key = ?", cn)
	}
	var rows []models.MediaOverride
	if err := q.Find(&rows).Error; err != nil {
		return models.MediaOverride{}, fmt.Errorf("读取手动纠正失败: %w", err)
	}
	for _, want := range []func(models.MediaOverride) bool{
		func(o models.MediaOverride) bool { return en != "" && o.Key == en },
		func(o models.MediaOverride) bool { return cn != "" && o.Key == cn },
		func(o models.MediaOverride) bool { return cn != "" && o.AltKey == cn },
	} {
		for _, o := range rows {
			if want(o) {
				return o, nil
			}
		}
	}
	return models.MediaOverride{}, nil
}

// otherTitlesTop 是名字对不上时再看英文名与别名的候选数。
const otherTitlesTop = 3

// pickByOtherTitles 用 TMDB 的英文名与别名再比一次：种子名常用英文名或罗马音（Your Name、Sousou no Frieren），
// 中文搜索结果里只有中文名与原名。只看排在前面的几个候选，名字要完全相同、总分过线才认。
func pickByOtherTitles(ctx context.Context, c *tmdb.Client, m meta.Meta, cands []Candidate) *Candidate {
	for i := range cands[:min(otherTitlesTop, len(cands))] {
		titles, err := c.Titles(ctx, cands[i].MediaType, cands[i].ID)
		if err != nil {
			continue
		}
		for _, t := range titles {
			alt := cands[i].Result
			alt.Title, alt.OriginalTitle = t, ""
			if titleScore(m, alt) < 1 {
				continue
			}
			if sc := score(m, alt, i); sc >= AcceptScore {
				out := cands[i]
				out.Score = sc
				return &out
			}
		}
	}
	return nil
}

// primaryKinds 是先搜的类型：剧集只搜剧集，电影先搜电影，类型不明时都搜。
func primaryKinds(m meta.Meta) []string {
	switch m.Type {
	case meta.TypeMovie:
		return []string{tmdb.KindMovie}
	case meta.TypeTV:
		return []string{tmdb.KindTV}
	}
	return []string{tmdb.KindMovie, tmdb.KindTV}
}

// details 用详情补全匹配到的条目（搜索与查找结果里没有 IMDb 编号，中文简介为空时详情里有英文的）；
// 取详情失败时用原来的结果。
func (s *Service) details(ctx context.Context, c *tmdb.Client, r tmdb.Result) *tmdb.Result {
	if d, err := c.Details(ctx, r.MediaType, r.ID); err == nil {
		return d
	}
	return &r
}

// pickFound 从 IMDb 查找结果里取一个：剧集优先取剧集，其余优先取电影。
func pickFound(m meta.Meta, found []tmdb.Result) *tmdb.Result {
	if len(found) == 0 {
		return nil
	}
	want := tmdb.KindMovie
	if m.Type == meta.TypeTV {
		want = tmdb.KindTV
	}
	for i := range found {
		if found[i].MediaType == want {
			return &found[i]
		}
	}
	return &found[0]
}

// search 用中文名与英文名分别搜索 kinds 里的类型，合并后打分排序。
func (s *Service) search(ctx context.Context, c *tmdb.Client, m meta.Meta, kinds []string) ([]Candidate, error) {
	var names []string
	for _, n := range []string{m.NameCN, m.NameEN} {
		if n = strings.TrimSpace(n); n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	type key struct {
		kind string
		id   int
	}
	best := map[key]Candidate{}
	for _, kind := range kinds {
		for _, name := range names {
			found, err := c.Search(ctx, kind, name, m.Year)
			if err != nil {
				return nil, err
			}
			if len(found) == 0 && kind == tmdb.KindMovie && m.Year > 0 {
				// 上映年份可能差一年（各地上映时间不同）：不带年份再搜一次
				if found, err = c.Search(ctx, kind, name, 0); err != nil {
					return nil, err
				}
			}
			for rank, r := range found {
				if rank >= 10 {
					break
				}
				cand := Candidate{Result: r, Score: score(m, r, rank)}
				k := key{r.MediaType, r.ID}
				if old, ok := best[k]; !ok || cand.Score > old.Score {
					best[k] = cand
				}
			}
		}
	}
	out := make([]Candidate, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Popularity != out[j].Popularity {
			return out[i].Popularity > out[j].Popularity
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
