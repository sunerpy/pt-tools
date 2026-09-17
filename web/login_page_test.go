package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
登录页是全站唯一不在 Vue SPA 里的页面，它的样式表 web/static/style.css 自带一份
调色板副本（SPA 之外拿不到 Vite 编译出来的 CSS）。副本一旦与 theme.scss 漂移，
登录页与主界面就会是两种观感，而这种漂移在编译和 lint 里都看不出来，所以在这里
逐个 token 比对钉住。
*/

// paletteSelectors 是 theme.scss ① 层的 8 套调色板（4 配色 × 明暗），
// 键是去掉全部空白后的选择器文本。
var paletteSelectors = []string{
	`html.light,html.light[data-theme-style="cockpit"]`,
	`html.dark,html.dark[data-theme-style="cockpit"]`,
	`html.light[data-theme-style="atlas"]`,
	`html.dark[data-theme-style="atlas"]`,
	`html.dark[data-theme-style="deck"]`,
	`html.light[data-theme-style="deck"]`,
	`html.dark[data-theme-style="halo"]`,
	`html.light[data-theme-style="halo"]`,
}

var (
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssRuleRe    = regexp.MustCompile(`(?s)([^{}]+)\{([^{}]*)\}`)
	cssTokenRe   = regexp.MustCompile(`(--pt-[a-z0-9-]+)\s*:\s*([^;]+);`)
	cssSpaceRe   = regexp.MustCompile(`\s+`)
)

// parseTokenBlocks 抽出每个规则块里的 --pt-* 自定义属性。选择器按“去掉全部空白”
// 归一，值按“空白折叠成单空格”归一，这样跨文件的换行与缩进差异不算漂移。
// 只做扁平匹配：8 套调色板与 :root 都没有嵌套，块外的 SCSS 嵌套匹配到什么都不会
// 命中下面查的这几个选择器。
func parseTokenBlocks(t *testing.T, path string) map[string]map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "读取 %s", path)
	text := cssCommentRe.ReplaceAllString(string(raw), "")

	blocks := map[string]map[string]string{}
	for _, m := range cssRuleRe.FindAllStringSubmatch(text, -1) {
		selector := cssSpaceRe.ReplaceAllString(m[1], "")
		selector = strings.TrimPrefix(selector, "}")
		tokens := map[string]string{}
		for _, d := range cssTokenRe.FindAllStringSubmatch(m[2], -1) {
			tokens[d[1]] = strings.TrimSpace(cssSpaceRe.ReplaceAllString(d[2], " "))
		}
		if len(tokens) == 0 {
			continue
		}
		if _, dup := blocks[selector]; dup {
			t.Fatalf("%s 里选择器 %q 出现了两次带 token 的块，后一块会覆盖前一块", path, selector)
		}
		blocks[selector] = tokens
	}
	return blocks
}

func TestLoginStylesheetPalettesMatchThemeSource(t *testing.T) {
	spa := parseTokenBlocks(t, "frontend/src/styles/theme.scss")
	login := parseTokenBlocks(t, "static/style.css")

	for _, selector := range paletteSelectors {
		want, ok := spa[selector]
		require.True(t, ok, "theme.scss 里找不到调色板 %s；改了选择器要同步这里的清单", selector)
		got, ok := login[selector]
		require.True(t, ok, "static/style.css 缺少调色板 %s，登录页在这套配色下会掉色", selector)

		assert.Equal(t, want, got,
			"static/style.css 的 %s 与 theme.scss 不一致：登录页与 SPA 会显示成两种观感，"+
				"请把 theme.scss 的该区块逐字复制过去", selector)
	}
}

func TestLoginStylesheetRootTokensAreSubsetOfThemeSource(t *testing.T) {
	spa := parseTokenBlocks(t, "frontend/src/styles/theme.scss")
	login := parseTokenBlocks(t, "static/style.css")

	// :root 允许只取登录页用到的子集，但取到的每个值必须与 SPA 相同。
	require.Contains(t, spa, ":root")
	require.Contains(t, login, ":root")
	require.NotEmpty(t, login[":root"])

	for token, got := range login[":root"] {
		want, ok := spa[":root"][token]
		require.True(t, ok, "static/style.css 的 :root 里 %s 在 theme.scss 中不存在", token)
		assert.Equal(t, want, got, "static/style.css 的 :root 里 %s 与 theme.scss 不一致", token)
	}
}

func TestLoginPageRendersBoardContent(t *testing.T) {
	srv := NewServer(nil, nil)
	rec := httptest.NewRecorder()
	srv.loginHandler(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	// 画板 43 明确要求的文案与结构
	for _, want := range []string{
		`class="login-page"`,
		"RSS 自动推送",
		"磁盘与容量守门",
		"ChatOps 双向指令",
		"单用户模式",
		"欢迎登录",
		"首次启动会自动创建 admin / adminadmin，登录后请立刻在「修改密码」里更换。",
		`src="/wordmark.svg"`,
		`src="/logo.svg"`,
		`id="loginAlert"`,
		`class="login-reveal"`,
	} {
		assert.Contains(t, body, want)
	}

	// 失败态必须走卡内联，不能再回到 alert()
	assert.NotContains(t, body, "alert(")

	// 主题必须在首帧之前定下来，且读的是 SPA 存的那两个键
	assert.Contains(t, body, "localStorage.getItem('theme-style')")
	assert.Contains(t, body, "localStorage.getItem('theme')")
	assert.Contains(t, body, "(prefers-color-scheme: dark)")
	assert.Contains(t, body, "data-theme-style")
}

func TestLoginBrandAssetsAreNotBehindAuth(t *testing.T) {
	srv := NewServer(nil, nil)
	mux := http.NewServeMux()
	distFS := mustSub(staticFS, "static/dist")
	for _, asset := range []string{"logo.svg", "wordmark.svg", "favicon.ico"} {
		mux.HandleFunc("/"+asset, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, distFS, asset)
		})
	}
	mux.HandleFunc("/", srv.auth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 登录页未登录就要显示品牌图，所以这几条路径不能落到 "/" 兜底的 302。
	// 这里不断言 200：make embed-placeholder 构出来的 dist 里没有这些文件，
	// 那种情况下 404 是对的，302 才是错的。
	for _, path := range []string{"/logo.svg", "/wordmark.svg", "/favicon.ico"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		assert.NotEqual(t, http.StatusFound, rec.Code, "%s 被重定向到了登录页", path)
	}
}

func TestDisplayVersion(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"unknown":  "",
		"  ":       "",
		"v0.47.3":  "v0.47.3",
		"0.47.3":   "v0.47.3",
		" 1.0.0  ": "v1.0.0",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, displayVersionOf(in))
		})
	}
}
