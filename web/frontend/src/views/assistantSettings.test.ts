// @vitest-environment happy-dom
/*
 * 钉子：AI 助手设置页（路线图 M19）。API Key 只写不读：没填新的就不发 api_key，点「清除」发空串；
 * 换了接口地址又没填新 Key 时提前写明 Key 会被清掉；测试连接按表单里的值发，结果写在卡里。
 * 语音输入（Voltip）的访问令牌同样只写不读；测试语音输入读出的处理方式进下拉框；测大模型时不带语音那几项。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  testSettings: vi.fn(),
  listModels: vi.fn(),
  voiceTest: vi.fn(),
}));
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

/** 打开一个下拉框，给出它自己的选项（按输入框的 aria-controls 找：别的下拉框的选项也可能在页面上） */
function openOptions(id: string): () => Element[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return () => [
    ...(document.getElementById(listId ?? "")?.querySelectorAll(".el-select-dropdown__item") ?? []),
  ];
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

  it("语音输入：没配过时语言先填中文；令牌只写不读，没填新的不发，点「清除」发空串", async () => {
    await mountPage();
    api.saveSettings.mockResolvedValue(saved());
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    let body = api.saveSettings.mock.calls[0]![0];
    expect(body).toMatchObject({ voice_base_url: "", voice_model: "", voice_language: "zh" });
    expect(body).not.toHaveProperty("voice_token");
    expect(document.body.textContent).toContain("没有令牌");

    view?.unmount();
    vi.clearAllMocks();
    await mountPage(
      saved({
        voice_base_url: "http://127.0.0.1:47840/v1",
        has_voice_token: true,
        voice_model: "voltip:raw",
        voice_language: "",
      }),
    );
    expect(document.body.textContent).toContain("已保存令牌");
    api.saveSettings.mockResolvedValue(saved());
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    body = api.saveSettings.mock.calls[0]![0];
    expect(body).toMatchObject({
      voice_base_url: "http://127.0.0.1:47840/v1",
      voice_model: "voltip:raw",
      voice_language: "",
    });
    expect(body).not.toHaveProperty("voice_token");

    await flush();
    typeInto("ass-voice-token", " voltip-new-token ");
    await flush();
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(2));
    expect(api.saveSettings.mock.calls[1]![0].voice_token).toBe("voltip-new-token");
  });

  it("语音输入：点「清除」发空串；换了 Voltip 的地址又没填新令牌，写明会清掉", async () => {
    await mountPage(saved({ voice_base_url: "http://127.0.0.1:47840/v1", has_voice_token: true }));
    expect(q("ass-voice-token-drop")).toBeNull();
    typeInto("ass-voice-url", "http://192.168.1.5:47840/v1/");
    await flush();
    expect(q("ass-voice-token-drop")!.textContent).toContain("清掉现在的访问令牌");
    typeInto("ass-voice-url", "http://127.0.0.1:47840/v1/");
    await flush();
    expect(q("ass-voice-token-drop")).toBeNull();

    q("ass-voice-token-clear")!.click();
    await flush();
    expect(document.body.textContent).toContain("没有令牌");
    api.saveSettings.mockResolvedValue(saved());
    q("ass-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0].voice_token).toBe("");
  });

  it("测试语音输入：按表单发，Voltip 列出的处理方式进下拉框（名字在前、标识在后）；失败写原因", async () => {
    await mountPage(saved({ voice_base_url: "http://127.0.0.1:47840/v1", has_voice_token: true }));
    api.voiceTest.mockResolvedValue({
      models: [
        { id: "voltip", name: "默认处理方式" },
        { id: "voltip:raw", name: "不使用 AI 润色" },
        { id: "voltip:scene=coding", name: "voltip:scene=coding" },
      ],
      latency_ms: 12,
    });
    q("ass-voice-test")!.click();
    await vi.waitFor(() => expect(q("ass-voice-tested")).not.toBeNull());
    expect(api.voiceTest.mock.calls[0]![0]).toMatchObject({
      voice_base_url: "http://127.0.0.1:47840/v1",
    });
    expect(api.voiceTest.mock.calls[0]![0]).not.toHaveProperty("voice_token");
    expect(q("ass-voice-tested")!.textContent).toContain("3 种处理方式");
    expect(q("ass-voice-tested")!.textContent).toContain("12 毫秒");
    const items = openOptions("ass-voice-model");
    await vi.waitFor(() => expect(items().length).toBe(3));
    // 每个选项：名字一段，名字和标识不一样时标识另一段
    expect(
      items().map((o) => [...o.querySelectorAll("span")].map((sp) => sp.textContent?.trim())),
    ).toEqual([
      ["默认处理方式", "voltip"],
      ["不使用 AI 润色", "voltip:raw"],
      ["voltip:scene=coding"],
    ]);

    api.voiceTest.mockRejectedValue(new Error("Voltip 不认这个访问令牌：重新复制一遍令牌"));
    q("ass-voice-test")!.click();
    await vi.waitFor(() => expect(q("ass-voice-tested")!.textContent).toContain("没有连上"));
    expect(q("ass-voice-tested")!.textContent).toContain("不认这个访问令牌");
  });

  it("测大模型的连接、读模型列表时不带语音输入那几项", async () => {
    await mountPage(saved({ voice_base_url: "http://127.0.0.1:47840/v1", has_voice_token: true }));
    typeInto("ass-voice-token", "voltip-new-token");
    await flush();
    api.testSettings.mockResolvedValue({ model: "deepseek-flash", reply: "OK", latency_ms: 1 });
    api.listModels.mockResolvedValue({ models: [], total: 0 });
    q("ass-test")!.click();
    q("ass-list-models")!.click();
    await vi.waitFor(() => expect(api.listModels).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(api.testSettings).toHaveBeenCalledTimes(1));
    for (const body of [api.testSettings.mock.calls[0]![0], api.listModels.mock.calls[0]![0]]) {
      for (const k of ["voice_base_url", "voice_token", "voice_model", "voice_language"]) {
        expect(body).not.toHaveProperty(k);
      }
    }
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

  it("选了有免费额度的服务商：写明免费额度、隐私、要不要代理，给申请 Key 的链接", async () => {
    await mountPage(
      saved({
        provider: "google",
        base_url: "https://generativelanguage.googleapis.com/v1beta/openai",
        model: "gemini-3.8-flash",
      }),
    );
    expect(q("ass-free")!.textContent).toContain("免费额度");
    expect(q("ass-free")!.textContent).toContain("以服务商的页面为准");
    expect(q("ass-privacy")!.textContent).toContain("Google");
    expect(q("ass-proxy-hint")!.textContent).toContain("代理");
    const link = q("ass-console") as HTMLAnchorElement;
    expect(link.href).toBe("https://aistudio.google.com/apikey");
    expect(link.target).toBe("_blank");
    expect(link.rel).toContain("noopener");
  });

  it("DeepSeek 没有免费额度这一条，也不提示代理，只给申请 Key 的链接", async () => {
    await mountPage();
    expect(q("ass-free")).toBeNull();
    expect(q("ass-proxy-hint")).toBeNull();
    expect((q("ass-console") as HTMLAnchorElement).href).toBe(
      "https://platform.deepseek.com/api_keys",
    );
  });

  it("获取模型列表：按表单里的值请求，读到的模型并进下拉框（常用的在前）", async () => {
    await mountPage();
    api.listModels.mockResolvedValue({ models: ["deepseek-flash", "deepseek-other"], total: 5 });
    q("ass-list-models")!.click();
    await vi.waitFor(() => expect(api.listModels).toHaveBeenCalledTimes(1));
    const body = api.listModels.mock.calls[0]![0];
    expect(body).toMatchObject({ base_url: "https://api.deepseek.com" });
    expect(body).not.toHaveProperty("api_key");
    await vi.waitFor(() => expect(q("ass-listed")!.textContent).toContain("服务商列出 5 个模型"));
    expect(q("ass-listed")!.textContent).toContain("2");
    const items = openOptions("ass-model");
    await vi.waitFor(() => expect(items().length).toBe(3));
    expect(items().map((o) => o.textContent?.trim())).toEqual([
      "deepseek-flash",
      "deepseek-v4-pro",
      "deepseek-other",
    ]);
  });

  it("获取模型列表失败：写出原因", async () => {
    await mountPage();
    api.listModels.mockRejectedValue(new Error("模型服务回 401：bad key（检查 API Key）"));
    q("ass-list-models")!.click();
    await vi.waitFor(() => expect(q("ass-listed")!.textContent).toContain("检查 API Key"));
    expect(q("ass-listed")!.className).toContain("ass-tip--warn");
  });

  it("获取模型列表还没回来就换了服务商：晚到的结果不写进新服务商的下拉框", async () => {
    await mountPage();
    let resolve!: (v: { models: string[]; total: number }) => void;
    api.listModels.mockReturnValue(new Promise((r) => (resolve = r)));
    q("ass-list-models")!.click();
    await vi.waitFor(() => expect(api.listModels).toHaveBeenCalledTimes(1));
    const sel = q("ass-provider")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    const groq = await vi.waitFor(() => {
      const el = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => o.textContent?.trim() === "Groq",
      );
      expect(el).toBeTruthy();
      return el!;
    });
    groq.click();
    await vi.waitFor(() => expect(q("ass-console")!.getAttribute("href")).toContain("groq.com"));
    resolve({ models: ["deepseek-late"], total: 1 });
    await flush();
    await flush();
    expect(q("ass-listed")).toBeNull();
    const items = openOptions("ass-model");
    await vi.waitFor(() => expect(items().length).toBeGreaterThan(0));
    expect(items().map((o) => o.textContent?.trim())).not.toContain("deepseek-late");
  });
});
