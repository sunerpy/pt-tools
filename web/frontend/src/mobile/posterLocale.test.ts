// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { createApp, defineComponent, h, provide, ref } from "vue";
import { PosterLocale, usePosterWords } from "@/components/exportPoster/locale";
describe("poster locale", () => {
  it("keeps desktop text and whitespace unchanged by default", () => {
    const root = document.createElement("div");
    const app = createApp(
      defineComponent({
        setup() {
          const t = usePosterWords();
          return () => h("span", t(" 总上传 "));
        },
      }),
    );
    app.mount(root);
    expect(root.textContent).toBe(" 总上传 ");
    app.unmount();
  });
  it("translates optional English poster labels", () => {
    const root = document.createElement("div");
    const Child = defineComponent({
      setup() {
        const t = usePosterWords();
        return () => h("span", t("总上传") + "/" + t("下载（计费）"));
      },
    });
    const app = createApp(
      defineComponent({
        setup() {
          provide(PosterLocale, ref("en"));
          return () => h(Child);
        },
      }),
    );
    app.mount(root);
    expect(root.textContent).toBe("Uploaded/Credited download");
    app.unmount();
  });
});
