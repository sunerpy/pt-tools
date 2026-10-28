import { createApp } from "vue";
import ElementPlus from "element-plus";
import zhCn from "element-plus/es/locale/lang/zh-cn";
import en from "element-plus/es/locale/lang/en";
import "element-plus/dist/index.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import "../styles/theme.scss";
import "../styles/shared-components.css";
import "../styles/atoms.css";
import "./mobile.css";
import MobileApp from "./MobileApp.vue";
import { appThemeOf } from "./theme";
const english = new URLSearchParams(location.search).get("lang")?.startsWith("en") ?? false;
document.documentElement.lang = english ? "en" : "zh-CN";
// App 的主题（mobile.css 按 html[data-app-theme] 换面、线、字与主色）；没传时按 dark 退回明亮或暗黑。
// 配色只挂在 html.light / html.dark 上（见 styles/theme.scss），两个类都要设：只设 dark 时浅色页拿不到任何配色
const query = new URLSearchParams(location.search);
const appTheme = appThemeOf(query.get("theme"), query.get("dark") === "1");
const dark = appTheme === "dark" || appTheme === "graphite";
document.documentElement.dataset.appTheme = appTheme;
document.documentElement.classList.toggle("dark", dark);
document.documentElement.classList.toggle("light", !dark);
document.documentElement.style.colorScheme = dark ? "dark" : "light";
createApp(MobileApp)
  .use(ElementPlus, { locale: english ? en : zhCn })
  .mount("#app");
