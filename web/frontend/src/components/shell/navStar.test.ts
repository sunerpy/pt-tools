// @vitest-environment happy-dom
/*
 * 钉子：导航列底部、账号行上面有一行 Star 提示（用户 2026-11-09 要的），整行是链接，
 * 在新标签页打开 GitHub 仓库，不带 opener。导航列在抽屉形态（手机）里也是这一份组件，
 * 所以两种形态都看得到它。
 */
import ElementPlus from "element-plus";
import { createPinia, setActivePinia } from "pinia";
import { afterEach, describe, expect, it } from "vitest";
import { type App as VueApp, createApp } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import AppNav from "./AppNav.vue";

let app: VueApp | undefined;
let host: HTMLElement | undefined;

afterEach(() => {
  app?.unmount();
  host?.remove();
  app = undefined;
  host = undefined;
});

async function mountNav(drawer: boolean) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:any(.*)*", component: { render: () => null } }],
  });
  await router.push("/home");
  await router.isReady();
  const pinia = createPinia();
  setActivePinia(pinia);
  host = document.createElement("div");
  document.body.appendChild(host);
  app = createApp(AppNav, { activePath: "/home", drawer, visible: true });
  app.use(ElementPlus).use(pinia).use(router);
  app.mount(host);
  return host;
}

describe("导航列的 Star 提示", () => {
  for (const drawer of [false, true]) {
    it(`${drawer ? "抽屉" : "钉住"}形态：账号行上面一行链接，新标签页打开仓库`, async () => {
      const root = await mountNav(drawer);
      const link = root.querySelector<HTMLAnchorElement>("a.pt-nav__star");
      expect(link).not.toBeNull();
      expect(link!.getAttribute("href")).toBe("https://github.com/sunerpy/pt-tools");
      expect(link!.getAttribute("target")).toBe("_blank");
      expect(link!.getAttribute("rel")).toContain("noopener");
      // 在逗号处断成两行
      const lines = [...link!.querySelectorAll(".pt-nav__star-text > span")].map((el) =>
        el.textContent?.trim(),
      );
      expect(lines).toEqual(["如果 pt-tools 对你有帮助，", "还请动动小手到 GitHub 点个 Star"]);
      // 在导航滚动区外面、紧挨着账号行：列表再长，它也一直在底部看得见
      expect(link!.closest(".pt-nav__scroll")).toBeNull();
      expect(link!.nextElementSibling?.classList.contains("pt-nav__account")).toBe(true);
    });
  }
});
