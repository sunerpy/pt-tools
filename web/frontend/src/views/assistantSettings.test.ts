// @vitest-environment happy-dom
/*
 * 钉子：AI 助手设置页（路线图 M19）。API Key 只写不读：没填新的就不发 api_key，点「清除」发空串；
 * 换了接口地址又没填新 Key 时提前写明 Key 会被清掉；测试连接按表单里的值发，结果写在卡里。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ settings: vi.fn(), saveSettings: vi.fn(), testSettings: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, assistantApi: { ...real.assistantApi, ...api } };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

function saved(over: Record<string, unknown> = {}) {
  return {
    enabled: true,
    provider: "deepseek",
    base_url: "https://api.deepseek.com",
    model: "deepseek-flash",
    has_api_key: true,
    proxy_url: "",
    chatops: false,
    max_steps: 8,
    confirm_minutes: 10,
    retention_days: 30,
    ...over,
  };
}

function typeInto(id: string, value: string) {
  const el = q(id)!;
  const input = (el.tagName === "INPUT" ? el : el.querySelector("input")!) as HTMLInputElement;
  input.value = value;
  input.dispatchEvent(new Event("input"));
}

async function mountPage(s = saved()) {
  api.settings.mockResolvedValue(s);
  const Page = (await import("./AssistantSettings.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(q("ass-settings")).not.toBeNull());
  await flush();
}

describe("AI 助手设置", () => {
  it("没填新 Key：保存时不发 api_key（存着的 Key 不动）", async () => {
    await mountPage();
    api.saveSettings.mockResolvedValue(saved());
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    const body = api.saveSettings.mock.calls[0]![0];
    expect(body).not.toHaveProperty("api_key");
    expect(body).toMatchObject({
      enabled: true,
      base_url: "https://api.deepseek.com",
      model: "deepseek-flash",
    });
    expect(document.body.textContent).toContain("已保存 Key");
  });

  it("填了新 Key 就发它；点「清除」发空串", async () => {
    await mountPage();
    api.saveSettings.mockResolvedValue(saved());
    typeInto("ass-key", " sk-new ");
    await flush();
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0].api_key).toBe("sk-new");

    await flush();
    q("ass-key-clear")!.click();
    await flush();
    expect(document.body.textContent).toContain("没有 Key");
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(2));
    expect(api.saveSettings.mock.calls[1]![0].api_key).toBe("");
  });

  it("换了接口地址又没填新 Key：写明保存时会清掉 Key", async () => {
    await mountPage();
    expect(q("ass-key-drop")).toBeNull();
    typeInto("ass-base-url", "https://api.moonshot.cn/v1");
    await flush();
    expect(q("ass-key-drop")!.textContent).toContain("清掉现在的 API Key");
    typeInto("ass-key", "sk-moon");
    await flush();
    expect(q("ass-key-drop")).toBeNull();
  });

  it("测试连接按表单里的值发，结果写在卡里；失败写原因", async () => {
    await mountPage();
    api.testSettings.mockResolvedValue({ model: "deepseek-flash", reply: "OK", latency_ms: 321 });
    q("ass-test")!.click();
    await vi.waitFor(() => expect(q("ass-tested")).not.toBeNull());
    expect(api.testSettings.mock.calls[0]![0]).not.toHaveProperty("api_key");
    expect(q("ass-tested")!.textContent).toContain("连接正常");
    expect(q("ass-tested")!.textContent).toContain("321 毫秒");

    api.testSettings.mockRejectedValue(new Error("模型服务回 401：bad key（检查 API Key）"));
    q("ass-test")!.click();
    await vi.waitFor(() => expect(q("ass-tested")!.textContent).toContain("没有连上"));
    expect(q("ass-tested")!.textContent).toContain("检查 API Key");
  });

  it("第一次打开（什么都没存）：按 DeepSeek 预设填好地址与模型", async () => {
    await mountPage(
      saved({ enabled: false, provider: "", base_url: "", model: "", has_api_key: false }),
    );
    const url = q("ass-base-url")!;
    const input = (url.tagName === "INPUT" ? url : url.querySelector("input")!) as HTMLInputElement;
    expect(input.value).toBe("https://api.deepseek.com");
    expect(document.body.textContent).toContain("没有 Key");
  });
});
