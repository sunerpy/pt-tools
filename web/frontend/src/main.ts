import ElementPlus from "element-plus";
// 分页器、日期选择等内置文案默认是英文，界面其余部分全是中文，不给 locale 会混着显示
import zhCn from "element-plus/es/locale/lang/zh-cn";
import { createPinia } from "pinia";
import { createApp } from "vue";
import "element-plus/dist/index.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import "./styles/theme.scss";
import "./styles/shell.css";
import "./styles/shared-components.css";
import "./styles/atoms.css";
import App from "./App.vue";
import router from "./router";
import "./styles/main.css";

const app = createApp(App);

// 全站图标统一走 PtIcon（本地 lucide 子集），不再全局注册 Element 图标组件
app.use(createPinia());
app.use(router);
app.use(ElementPlus, { locale: zhCn });

app.mount("#app");
