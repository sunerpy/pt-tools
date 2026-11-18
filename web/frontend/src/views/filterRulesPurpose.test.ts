// @vitest-environment happy-dom
/*
 * 钉子：过滤规则的「用途」（#556）。列表有一列用途；老接口不返回 purpose 的规则按「下载」显示；
 * 编辑这种规则时下拉框落在「下载」而不是空着，保存时把 purpose 一起发出去。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  hits: vi.fn(),
  update: vi.fn(),
  create: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, filterRulesApi: { ...real.filterRulesApi, ...api } };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function rule(over: Record<string, unknown>) {
  return {
    name: "规则",
    pattern: "x",
    pattern_type: "keyword",
    match_field: "both",
    require_free: false,
    enabled: true,
    priority: 100,
    ...over,
  };
}

async function mountPage() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => Response.json({})),
  );
  api.list.mockResolvedValue([
    rule({ id: 1, name: "只通知", purpose: "notify" }),
    rule({ id: 2, name: "老规则" }),
    rule({ id: 3, name: "两者", purpose: "both" }),
  ]);
  api.hits.mockResolvedValue({ hits: {} });
  api.update.mockResolvedValue({});
  const Page = (await import("./FilterRules.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(q("rule-purpose-1")).not.toBeNull());
}

describe("过滤规则的用途", () => {
  it("列表有一列用途，没有 purpose 的老规则按下载显示", async () => {
    await mountPage();
    expect(q("rule-purpose-1")!.textContent).toBe("通知");
    expect(q("rule-purpose-2")!.textContent).toBe("下载");
    expect(q("rule-purpose-3")!.textContent).toBe("下载与通知");
  });

  it("编辑老规则：用途落在下载，保存时一起发出", async () => {
    await mountPage();
    document.querySelector<HTMLButtonElement>('[aria-label="编辑规则 老规则"]')!.click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("下载 —— 控制是否推送到下载器"),
    );
    const save = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
      (b) => b.textContent?.trim() === "保存",
    )!;
    save.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![0]).toBe(2);
    expect(api.update.mock.calls[0]![1]).toMatchObject({ name: "老规则", purpose: "download" });
  });
});
