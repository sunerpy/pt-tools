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
		loginFootNote,
		`src="/wordmark.svg"`,
		`src="/logo.svg"`,
		`id="loginAlert"`,
		`class="login-reveal"`,
	} {
		assert.Contains(t, body, want)
	}

	// 失败态必须走卡内联，不能再回到 alert()
	assert.NotContains(t, body, "alert(")

	// 未登录就能看到这一页：不能把默认口令印在上面。何况设置了 PT_ADMIN_USER / PT_ADMIN_PASS
	// 时，「会自动创建 admin / adminadmin」这句话本身就是错的。
	assert.NotContains(t, body, "adminadmin", "登录页不能公开默认密码")

	// 主题必须在首帧之前定下来，且读的是 SPA 存的那两个键
	assert.Contains(t, body, "localStorage.getItem('theme-style')")
	assert.Contains(t, body, "localStorage.getItem('theme')")
	assert.Contains(t, body, "(prefers-color-scheme: dark)")
	assert.Contains(t, body, "data-theme-style")
}

// loginFootNote 是登录卡底部的提示：只指路、不写口令字面量。
const loginFootNote = "首次登录的初始账号见安装文档；登录后请立刻在「修改密码」里更换。"

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

/*
登录页 <head> 里那段首帧主题脚本自称与 stores/theme.ts 的读取逻辑「一一对应」，
但两边是两份代码：SPA 后来把默认模式从深色改成了明亮，这里没跟上，
于是没有存储偏好的新用户先看到深色登录页、登录后又是浅色界面。
这条测试把「没有偏好时的默认模式 / 默认配色、认得的配色清单、旧配色迁移表」四件事钉住。
*/
func TestLoginThemeScriptMatchesThemeStore(t *testing.T) {
	raw, err := os.ReadFile("frontend/src/stores/theme.ts")
	require.NoError(t, err)
	store := string(raw)

	fnBody := func(name string) string {
		i := strings.Index(store, "function "+name+"(")
		require.GreaterOrEqual(t, i, 0, "theme.ts 里找不到 %s", name)
		j := strings.Index(store[i:], "\n}\n")
		require.Greater(t, j, 0)
		return store[i : i+j]
	}
	lastReturn := func(body string) string {
		m := regexp.MustCompile(`return "([a-z]+)";`).FindAllStringSubmatch(body, -1)
		require.NotEmpty(t, m)
		return m[len(m)-1][1]
	}
	storeMode := lastReturn(fnBody("readMode"))
	storePalette := lastReturn(fnBody("readPalette"))

	var storePalettes []string
	for _, m := range regexp.MustCompile(`value: "([a-z]+)",\n\s+label:`).FindAllStringSubmatch(
		store[strings.Index(store, "export const PALETTES"):strings.Index(store, "export const MODES")], -1,
	) {
		storePalettes = append(storePalettes, m[1])
	}
	require.NotEmpty(t, storePalettes)

	legacyBlock := store[strings.Index(store, "const LEGACY_PALETTE"):]
	legacyBlock = legacyBlock[:strings.Index(legacyBlock, "};")]
	storeLegacy := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+): "(\w+)"`).FindAllStringSubmatch(legacyBlock, -1) {
		storeLegacy[m[1]] = m[2]
	}
	require.NotEmpty(t, storeLegacy)

	script := loginHTML[strings.Index(loginHTML, "<script>"):strings.Index(loginHTML, "</script>")]
	pick := func(re string) string {
		m := regexp.MustCompile(re).FindStringSubmatch(script)
		require.Len(t, m, 2, "登录页脚本里找不到 %s", re)
		return m[1]
	}
	assert.Equal(t, storeMode, pick(`var mode = '([a-z]+)';`), "没有存储偏好时的默认模式")
	assert.Equal(t, storePalette, pick(`var palette = '([a-z]+)';`), "没有存储偏好时的默认配色")

	var loginPalettes []string
	for _, m := range regexp.MustCompile(`'([a-z]+)'`).FindAllStringSubmatch(pick(`var PALETTES = \[([^\]]*)\]`), -1) {
		loginPalettes = append(loginPalettes, m[1])
	}
	assert.Equal(t, storePalettes, loginPalettes, "认得的配色清单")

	loginLegacy := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+): '(\w+)'`).FindAllStringSubmatch(pick(`var LEGACY = \{([^}]*)\}`), -1) {
		loginLegacy[m[1]] = m[2]
	}
	assert.Equal(t, storeLegacy, loginLegacy, "旧配色迁移表")
}
