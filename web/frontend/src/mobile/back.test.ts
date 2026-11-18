// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";
import { ElDrawer, ElOption, ElSelect } from "element-plus";
import { mountView, type MountedView } from "@/test-mount";
import { closeTopLayer, installBack } from "./back";

let view: MountedView | undefined;
afterEach(() => {
  view?.unmount();
  view = undefined;
  delete window.ptMobileBack;
});

async function flush() {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve();
    await nextTick();
  }
}

/** 试搜索的两层：面板（里面有一个下拉）与面板上的操作单 */
const panel = ref(false);
const sheet = ref(false);
const pick = ref("a");
const Layers = defineComponent({
  setup: () => () => [
    h("div", { "data-testid": "page" }, "page"),
    h(
      ElDrawer,
      {
        modelValue: panel.value,
        "onUpdate:modelValue": (v: boolean) => (panel.value = v),
        appendToBody: true,
        title: "panel",
      },
      () =>
        h(
          ElSelect,
          {
            modelValue: pick.value,
            "onUpdate:modelValue": (v: string) => (pick.value = v),
            teleported: false,
            "data-testid": "pick",
          },
          () => [h(ElOption, { value: "a", label: "A" }), h(ElOption, { value: "b", label: "B" })],
        ),
    ),
    h(
      ElDrawer,
      {
        modelValue: sheet.value,
        "onUpdate:modelValue": (v: boolean) => (sheet.value = v),
        appendToBody: true,
        withHeader: false,
      },
      () => h("p", "sheet"),
    ),
  ],
});
async function mount() {
  panel.value = false;
  sheet.value = false;
  view = mountView(Layers);
  await flush();
}
const pickInput = () => document.querySelector<HTMLInputElement>("[data-testid=pick] input")!;

describe("返回先关掉最上面一层", () => {
  it("什么都没开着：不关、交给 App 离开", async () => {
    await mount();
    expect(closeTopLayer()).toBe(false);
  });

  it("操作单开在面板上：先关操作单，再关面板，然后才轮到 App", async () => {
    await mount();
    panel.value = true;
    await flush();
    sheet.value = true;
    await flush();
    expect(closeTopLayer()).toBe(true);
    // 抽屉在 closeDelay（0）的计时器里才真的关：等一个宏任务
    await vi.waitFor(() => expect(sheet.value).toBe(false));
    await flush();
    expect(panel.value).toBe(true);
    expect(closeTopLayer()).toBe(true);
    await vi.waitFor(() => expect(panel.value).toBe(false));
  });

  it("面板里的下拉展开着：只收起下拉，面板还开着", async () => {
    await mount();
    panel.value = true;
    await flush();
    document.querySelector<HTMLElement>("[data-testid=pick] .el-select__wrapper")!.click();
    await flush();
    expect(pickInput().getAttribute("aria-expanded")).toBe("true");
    expect(closeTopLayer()).toBe(true);
    await flush();
    expect(pickInput().getAttribute("aria-expanded")).toBe("false");
    expect(panel.value).toBe(true);
    expect(pick.value).toBe("a");
  });

  it("只剩在关闭动画里的遮罩：不算开着", () => {
    document.body.innerHTML =
      '<div class="el-overlay el-drawer-fade-leave-active el-drawer-fade-leave-to"></div>' +
      '<div class="el-overlay" style="display: none"></div>';
    expect(closeTopLayer()).toBe(false);
  });

  it("App 经 window.ptMobileBack 调用，拿到布尔值", async () => {
    await mount();
    installBack();
    expect(window.ptMobileBack?.()).toBe(false);
    panel.value = true;
    await flush();
    expect(window.ptMobileBack?.()).toBe(true);
    await vi.waitFor(() => expect(panel.value).toBe(false));
  });
});
