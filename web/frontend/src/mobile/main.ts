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
const english = new URLSearchParams(location.search).get("lang")?.startsWith("en") ?? false;
document.documentElement.lang = english ? "en" : "zh-CN";
document.documentElement.classList.toggle(
  "dark",
  new URLSearchParams(location.search).get("dark") === "1",
);
createApp(MobileApp)
  .use(ElementPlus, { locale: english ? en : zhCn })
  .mount("#app");
