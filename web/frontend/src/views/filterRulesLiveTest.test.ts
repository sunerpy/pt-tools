// @vitest-environment happy-dom
/*
 * 钉子：过滤规则页的试跑卡能直接选 RSS 现拉一次（以前只有编辑弹窗里能选，卡上的试跑永远用历史记录）；
 * 结果弹窗写明用的是哪个数据源，换了数据源马上重新试；示例 RSS 不列，地址是占位符的列出来但不能选。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  hits: vi.fn(),
  test: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, filterRulesApi: { ...real.filterRulesApi, ...api } };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  // 弹窗 teleport 到 body：上一个用例没关的弹窗不能留给下一个
  document.body.innerHTML = "";
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const flush = () => new Promise((r) => setTimeout(r, 0));

/** 打开一个下拉框，给出它自己的选项 */
function openOptions(id: string): () => HTMLElement[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return () => [
    ...(document
      .getElementById(listId ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ];
}

async function pick(id: string, text: string) {
  const options = openOptions(id);
  await vi.waitFor(() => {
    const opt = options().find((o) => (o.textContent ?? "").includes(text));
    expect(opt).toBeTruthy();
    opt!.click();
  });
  await flush();
}

const SITES = {
  mteam: { rss: [{ id: 7, name: "电影", url: "https://rss.m-team.cc/api/rss/k" }] },
  hdsky: {
    rss: [
      { id: 8, name: "HDSky", url: "https://hdsky.xxx/torrentrss.php?xxx" },
      { id: 9, name: "样例订阅", url: "https://hdsky.me/rss", is_example: true },
    ],
  },
};

async function mountPage() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json(SITES)),
  );
  api.list.mockResolvedValue([
    {
      id: 1,
      name: "4K",
      pattern: "2160p",
      pattern_type: "keyword",
      match_field: "both",
      require_free: false,
      enabled: true,
      priority: 100,
    },
  ]);
  api.hits.mockResolvedValue({ hits: {} });
  const Page = (await import("./FilterRules.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(q("rules-test-panel")).not.toBeNull());
  await flush();
}

describe("过滤规则页的试跑", () => {
  it("卡上选 RSS 现拉；示例不列，占位地址不能选；结果写明数据源，换了马上重试", async () => {
    await mountPage();
    api.test.mockResolvedValueOnce({
      match_count: 1,
      total_count: 30,
      source: "rss",
      source_name: "电影（mteam）",
      detail_failed: 2,
      matches: [
        {
          title: "Movie 2160p",
          tag: "",
          is_free: false,
          size_gb: 20,
          decision: "skipped",
          reason: "匹配规则要求免费，但种子非免费",
          source: "",
          detail_missing: true,
        },
      ],
    });

    await pick("rules-test-rule", "4K");
    const options = openOptions("rules-test-rss");
    await vi.waitFor(() => expect(options().length).toBe(2));
    const texts = options().map((o) => o.textContent ?? "");
    expect(texts.some((t) => t.includes("样例订阅"))).toBe(false);
    const placeholder = options().find((o) => (o.textContent ?? "").includes("HDSky"))!;
    expect(placeholder.textContent).toContain("占位地址");
    expect(placeholder.className).toContain("is-disabled");
    await pick("rules-test-rss", "电影（mteam）");

    q("rules-test-run")!.click();
    await vi.waitFor(() => expect(api.test).toHaveBeenCalledTimes(1));
    expect(api.test.mock.calls[0]![0]).toMatchObject({ pattern: "2160p", rss_id: 7 });
    await vi.waitFor(() => expect(q("rules-result-source")).not.toBeNull());
    expect(q("rules-result-source")!.textContent).toContain("现拉的 RSS「电影（mteam）」");
    expect(q("rules-result-summary")!.textContent).toContain("共测试 30 条记录");
    expect(q("rules-result-source")!.textContent).toContain("2 条没取到详情");
    expect(q("rules-result")!.textContent).toContain("没取到详情");

    // 结果弹窗里清掉数据源：马上改用最近入库的记录再试一次，卡上的下拉跟着变
    api.test.mockResolvedValueOnce({
      match_count: 0,
      total_count: 500,
      source: "history",
      source_name: "最近入库的种子记录（最多 500 条）",
      matches: [],
    });
    const sel = q("rules-result-rss")!;
    sel.dispatchEvent(new MouseEvent("mouseenter"));
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).dispatchEvent(
      new MouseEvent("mouseenter", { bubbles: true }),
    );
    await flush();
    const clear = sel.querySelector<HTMLElement>(".el-select__clear");
    expect(clear).not.toBeNull();
    clear!.click();
    await vi.waitFor(() => expect(api.test).toHaveBeenCalledTimes(2));
    expect(api.test.mock.calls[1]![0].rss_id).toBeUndefined();
    await vi.waitFor(() =>
      expect(q("rules-result-source")!.textContent).toContain(
        "最近入库的种子记录（最多 500 条） · 不联网",
      ),
    );
  });

  it("RSS 取不到时结果弹窗里也能换数据源", async () => {
    await mountPage();
    const { ApiError } = await import("@/api");
    api.test.mockRejectedValueOnce(
      new ApiError(502, "获取 RSS「电影」失败：rss.m-team.cc 断开了连接（connection reset）"),
    );
    await pick("rules-test-rule", "4K");
    await pick("rules-test-rss", "电影（mteam）");
    q("rules-test-run")!.click();
    await vi.waitFor(() => expect(q("rules-result")!.textContent).toContain("connection reset"));
    expect(q("rules-result-rss")).not.toBeNull();
  });
});
