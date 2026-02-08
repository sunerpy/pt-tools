import * as ElementPlusIconsVue from "@element-plus/icons-vue";
import ElementPlus from "element-plus";
import { createPinia } from "pinia";
import { type Component, createApp } from "vue";
import "element-plus/dist/index.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import "./styles/theme.scss";
import "./styles/common-page.css";
import "./styles/app-layout.css";
import "./styles/dashboard.css";
import "./styles/form-page.css";
import "./styles/table-page.css";
import "./styles/shared-components.css";
import App from "./App.vue";
import router from "./router";
import "./styles/main.css";

const app = createApp(App);

// 注册所有图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component as Component);
}

app.use(createPinia());
app.use(router);
app.use(ElementPlus);

app.mount("#app");
