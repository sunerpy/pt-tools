# pt-tools 界面规范

## Tokens

沿用 web/frontend/src/styles/theme.scss、atoms.css、shell.css 中的 CSS 变量；不新增品牌配色、字体或阴影体系。Flutter 沿用 app.dart 的 Material 3 与 brandTeal / brandOrange。

## Primitives

网页使用现有 Element Plus、PtPanel、PtDataState、PtRowCard 及海报组件。手机嵌入页单列呈现，不加载桌面导航；表单标签清楚，触摸操作至少 44px，页面不产生横向滚动。原生导航由 Flutter 负责，Vue 不重复绘制全站外壳。

## States

区分 loading、empty、zero、error、partial、perm；读取失败不显示为空数据。保存中禁止重复提交，成功后刷新原生来源，失败保留输入。只读设备隐藏写操作，服务端仍校验权限。

## Motion

沿用现有组件的交互反馈，不新增装饰性动效。返回离开有未保存改动的表单时确认；系统返回与页面返回行为一致。

## Accessibility

支持键盘、系统返回、明暗主题与系统语言。输入控件有可访问标签，错误用文字解释，不仅依赖颜色。海报默认遮蔽用户名、站点名和图标，分享前显示实际导出预览。

## Accepted debt

本次只扩展 Android 手机业务入口，不重做桌面设计；WebView 在真机上的键盘、滚动、PNG 渲染与系统分享必须独立验收，浏览器截图不替代设备验收。
