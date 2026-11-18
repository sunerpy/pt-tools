# 品牌标识

[返回文档中心](README.md)

pt-tools 的标志是超椭圆底板上的一枚种子徽标：白色的种子里镂空出字母 p。它有两版，按所在表面的明暗选：墨黑版（深色底板、白色种子）放在浅色表面上，纯白版（白色底板、深色种子）放在深色表面上。这个标志是 2026-11-18 为 1.0 选定的，取代原来的终端提示符标志。

## 仓库内的文件

| 文件                                                             | 用途                                                                                     |
| ---------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `web/frontend/public/logo.svg`                                   | 墨黑版，浅色表面用                                                                       |
| `web/frontend/public/logo-dark.svg`                              | 纯白版，深色表面用                                                                       |
| `web/frontend/public/logo-mono.svg`                              | 单色剪影（只有种子与镂空的 p），颜色跟随 `currentColor`                                  |
| `web/frontend/public/wordmark.svg`                               | 横版：纯白徽标加浅色 `pt-tools` 轮廓字，透明底，深色表面用（登录页左侧的品牌栏）         |
| `scripts/brand/favicon.svg`                                      | 32 px 及以下用的墨黑版，图形不缩，只用来出图                                             |
| `scripts/brand/app-icon.svg`                                     | 满铺、不透明的墨黑版，iOS、apple-touch 与 PWA maskable 图标用                            |
| `scripts/brand/adaptive-{foreground,background,monochrome}.svg`  | 安卓自适应图标的三层（108dp 画布，可见区是中间的 72dp）                                  |
| `web/frontend/public/logo.png`                                   | 512 px，外部引用（Unraid 模板的图标指向 main 分支上的这个文件）                          |
| `web/frontend/public/favicon.ico`、`favicon-{16x16,32x32}.png`   | 浏览器标签页图标，ICO 里有 16 / 32 / 48 / 64 / 128 / 256 六个尺寸                        |
| `web/frontend/public/apple-touch-icon.png`                       | 180 px，满铺不透明，iOS 添加到主屏                                                       |
| `web/frontend/public/android-chrome-*.png`                       | 192 / 512 px                                                                             |
| `tools/browser-extension/public/icons/*`                         | 扩展图标 16 / 32 / 48 / 128（MV3 manifest）                                              |
| `apps/mobile/android/app/src/main/res/mipmap-*/ic_launcher*.png` | 安卓旧式图标与自适应图标的三层；`mipmap-anydpi-v26/ic_launcher.xml` 把三层组成自适应图标 |
| `apps/mobile/ios/Runner/Assets.xcassets/AppIcon.appiconset/*`    | iOS 图标，全部满铺不透明                                                                 |
| `apps/mobile/web/favicon.png`、`apps/mobile/web/icons/*`         | App 网页版的图标                                                                         |
| `deploy/fnos/apps/pt-tools/fnos/ICON*.PNG`、`ui/images/*.png`    | 飞牛 fnOS 应用图标（64 / 256）                                                           |
| `deploy/1panel/pt-tools/logo.png`                                | 1Panel 应用图标（512）                                                                   |

位图全部由 `make brand-icons`（`scripts/brand/render.py`）从矢量按目标尺寸直接渲染：32 px 及以下用 `favicon.svg`，48 px 起用 `logo.svg`，要求满铺的图标用 `app-icon.svg`。**不要**拿大图缩小生成小图。

## 唯一事实来源

矢量原件由品牌仓库 `firlab/brand/pt-tools` 的 `build.py` 生成，本仓库里的 SVG（上表 `web/frontend/public/*.svg` 与 `scripts/brand/*.svg`）都是**逐字节复制**过来的。要改标志，改那边并重新运行 `build.py`，把 SVG 复制过来，再运行 `make brand-icons`；不要在本仓库手改 SVG。

## 构成

- 1024 的画布，超椭圆底板铺满画布。
- 墨黑版：底板是 `#31353C` 到 `#131518` 的线性渐变，左上方有一层很淡的白色径向高光，顶边有一道高光描边；种子是 `#C9D2DE` 到 `#FFFFFF` 的渐变，p 从种子里镂空，露出底板。
- 纯白版：底板是 `#FFFFFF` 到 `#EEF1F5` 的渐变，带 `#DCE1E8` 的细描边；种子是深色渐变。
- 图形在 `logo.svg` 与 `app-icon.svg` 里缩到 0.86（四周留白），在 `favicon.svg` 里是 1.0（小尺寸时更清楚）。
- 横版 1084 × 256，透明底；字是 Instrument Sans 600 的轮廓，**不是** text 元素，不用活字体重排。

## 用哪个变体

| 场景                                                                    | 变体                    |
| ----------------------------------------------------------------------- | ----------------------- |
| 浅色表面：明亮主题的侧栏与登录页、README 的浅色模式、文档站浅色页面     | `logo.svg`（墨黑）      |
| 深色表面：暗黑主题、README 的深色模式、文档站深色页面                   | `logo-dark.svg`（纯白） |
| 登录页左侧的品牌栏（所有配色下都是深色）                                | `wordmark.svg`          |
| 只能放一张图的地方：App、浏览器扩展、NAS 应用图标、favicon、apple-touch | 墨黑版                  |
| 只能用单色的地方                                                        | `logo-mono.svg`         |

判断标准是**表面明暗**，不是页面名。中间调背景（灰、彩色照片、渐变）上先做对比度检查再用。网页里用 `PtLogo` 组件，它跟着明亮、暗黑主题在两版之间切换。

## 硬性规则

- **最小尺寸**：标志 16 px，横版宽 96 px。
- **四周留白 ≥ 标志高度的 25%**，从底板边缘量起。
- 不要改色、改透明度、非等比拉伸、旋转、镜像，也不要在原件之外再加阴影、发光或外描边；渐变与高光是原件的一部分，照原样用。
- 引用 SVG 文件，不要用代码重画几何。同一页面放几份时用 `<img>` 引用：内联的话，几份图形的渐变 id 会互相覆盖。
