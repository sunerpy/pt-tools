// @vitest-environment happy-dom
/*
 * 钉子：导出时预览里排成一行的文字在克隆里不许折行。modern-screenshot 把每个元素的宽高原样写进克隆，
 * 手机上导出的字只要比预览宽一点点，最后一个字就折到下一行、压在下面的字上（「我的 PT 数据统计」的「计」）。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

const shot = vi.hoisted(() => ({ domToBlob: vi.fn() }));
vi.mock("modern-screenshot", () => ({ domToBlob: shot.domToBlob }));

import { posterToBlob } from "./posterImage";

/** 每段文字的行框：[left, top, width]，高 16 */
const LINES: Record<string, [number, number, number][]> = {
  "我的 PT 数据统计": [[0, 0, 206]],
  // 预览里就是两行：第二行回到左边、往下
  两行的说明文字: [
    [0, 40, 200],
    [0, 60, 80],
  ],
  // 小字与后面的粗体在同一行：粗体靠右，顶边因为字号不同略低
  上传: [[0, 100, 24]],
  "+1.6 TB": [[29, 102, 40]],
};

type CloneHook = (cloned: Node) => void;

function stubLineBoxes() {
  return vi.spyOn(Range.prototype, "getClientRects").mockImplementation(function (this: Range) {
    const boxes = LINES[this.startContainer.textContent ?? ""] ?? [];
    const list = boxes.map(([left, top, width]) => ({
      left,
      top,
      width,
      height: 16,
      right: left + width,
      bottom: top + 16,
    }));
    return list as unknown as DOMRectList;
  });
}

function mountPoster(): HTMLElement {
  // happy-dom 的计算样式没有浏览器默认值（white-space 为空、<b> 没有 display），这里按浏览器补上
  const defaults = document.createElement("style");
  defaults.textContent = ".poster * { white-space: normal; } .poster b { display: inline; }";
  document.head.appendChild(defaults);
  const root = document.createElement("div");
  root.className = "poster";
  root.innerHTML = `
    <div class="head">
      <h1 id="one">我的 PT 数据统计</h1>
      <div id="two">两行的说明文字</div>
    </div>
    <div id="mixed">上传<b id="mixed-b">+1.6 TB</b></div>`;
  document.body.appendChild(root);
  return root;
}

/** 像 modern-screenshot 那样：先克隆子节点、再回调父节点 */
function cloneLikeScreenshot(node: HTMLElement, hook: CloneHook): HTMLElement {
  const clone = node.cloneNode(true) as HTMLElement;
  for (const el of Array.from(clone.querySelectorAll("*")).reverse()) hook(el);
  hook(clone);
  return clone;
}

afterEach(() => {
  vi.restoreAllMocks();
  shot.domToBlob.mockReset();
  document.head.innerHTML = "";
  document.body.innerHTML = "";
});

describe("posterToBlob 不让一行的文字在导出里折行", () => {
  it("预览里一行的元素（连同里面的子元素）在克隆里 nowrap，两行的不动，记号不留在页面上", async () => {
    stubLineBoxes();
    const root = mountPoster();
    let marked: string[] = [];
    let clone: HTMLElement | null = null;
    shot.domToBlob.mockImplementation(
      async (node: HTMLElement, opts: { onCloneEachNode: CloneHook }) => {
        marked = Array.from(node.querySelectorAll("[data-poster-one-line]")).map((e) => e.id);
        clone = cloneLikeScreenshot(node, opts.onCloneEachNode);
        return new Blob(["png"], { type: "image/png" });
      },
    );

    await posterToBlob(root, []);

    expect(marked.sort()).toEqual(["mixed", "one"]);
    const c = clone as HTMLElement | null;
    expect(c).not.toBeNull();
    expect(c!.querySelector<HTMLElement>("#one")!.style.whiteSpace).toBe("nowrap");
    expect(c!.querySelector<HTMLElement>("#mixed")!.style.whiteSpace).toBe("nowrap");
    expect(c!.querySelector<HTMLElement>("#mixed-b")!.style.whiteSpace).toBe("nowrap");
    expect(c!.querySelector<HTMLElement>("#two")!.style.whiteSpace).toBe("");
    expect(c!.querySelector<HTMLElement>(".head")!.style.whiteSpace).toBe("");
    expect(c!.querySelectorAll("[data-poster-one-line]")).toHaveLength(0);
    expect(document.querySelectorAll("[data-poster-one-line]")).toHaveLength(0);
  });

  it("导出失败也不在页面上留下记号", async () => {
    stubLineBoxes();
    const root = mountPoster();
    shot.domToBlob.mockRejectedValue(new Error("画不出来"));

    await expect(posterToBlob(root, [])).rejects.toThrow("画不出来");
    expect(document.querySelectorAll("[data-poster-one-line]")).toHaveLength(0);
  });

  it("量不到行框时什么都不改", async () => {
    const root = mountPoster();
    let marked = -1;
    shot.domToBlob.mockImplementation(async (node: HTMLElement) => {
      marked = node.querySelectorAll("[data-poster-one-line]").length;
      return new Blob(["png"], { type: "image/png" });
    });

    await posterToBlob(root, []);
    expect(marked).toBe(0);
  });
});
