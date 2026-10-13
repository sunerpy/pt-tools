// @vitest-environment happy-dom
/*
 * 钉子：Webhook 通道详情的「请求」一节（路线图 M21）。改方法、格式、请求体以后按示例数据防抖预览；
 * 套用预设填进方法、格式与请求体，并提示地址的样子；请求头不是 JSON 对象时就地写明，保存不提交；
 * 保存时连同方法、格式与请求体一起发出；GET 不带请求体。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  test: vi.fn(),
  list: vi.fn(),
  preview: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      notifications: { get: api.get, update: api.update, test: api.test },
      templates: { list: api.list },
      webhook: { preview: api.preview },
    },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function field(id: string): HTMLInputElement | HTMLTextAreaElement {
  const el = q(id)!;
  if (el.tagName === "INPUT" || el.tagName === "TEXTAREA") return el as HTMLInputElement;
  return (el.querySelector("textarea") ?? el.querySelector("input"))!;
}

function typeInto(id: string, value: string) {
  const el = field(id);
  el.value = value;
  el.dispatchEvent(new Event("input"));
}

// 预设下拉框的选项带说明（.wh-preset__desc）：格式下拉框里也有「纯文本」，按说明区分
async function pickPreset(label: string) {
  const sel = q("wh-preset")!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const item = await vi.waitFor(() => {
    const el = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => o.querySelector(".wh-preset__desc") && o.textContent?.trim().startsWith(label),
    );
    expect(el).toBeTruthy();
    return el!;
  });
  item.click();
}

// 方法下拉框：选项只有名字（POST、GET…），按整段文字找
async function pickOption(id: string, label: string) {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const item = await vi.waitFor(() => {
    const el = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => o.textContent?.trim() === label,
    );
    expect(el).toBeTruthy();
    return el!;
  });
  item.click();
}

async function mountDetail(over: Record<string, unknown> = {}) {
  api.get.mockResolvedValue({
    id: 7,
    channel_type: "webhook",
    name: "钩子",
    enabled: true,
    endpoint_url: "https://example.com/hook",
    headers: '{"X-A": "1"}',
    ...over,
  });
  api.list.mockResolvedValue({
    groups: [],
    common_vars: [],
    events: [
      { type: "rss.matched", group: "notify", name: "RSS 命中过滤规则" },
      { type: "test", group: "notify", name: "通道测试" },
      { type: "reply.status", group: "reply", name: "/status 状态概览", no_title: true },
      { type: "assistant.reply", group: "assistant", name: "AI 助手的回复", no_title: true },
    ],
  });
  api.preview.mockResolvedValue({
    method: "POST",
    url: "https://example.com/hook",
    content_type: "application/json",
    body: '{"event_type":"notification"}',
  });
  api.update.mockResolvedValue({ status: "ok" });
  const NotificationDetail = (await import("./NotificationDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/chatops/notifications/:id", component: NotificationDetail }],
  });
  await router.push("/chatops/notifications/7");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() => expect(field("wh-url").value).toBe("https://example.com/hook"));
}

describe("Webhook 的请求", () => {
  it("打开就按示例数据预览一次；套用预设填进请求体并提示地址，改了以后重新预览", async () => {
    await mountDetail();
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenCalledWith({
        type: "rss.matched",
        method: "POST",
        endpoint_url: "https://example.com/hook",
        content_type: "json",
        headers: { "X-A": "1" },
        body_template: "",
        success_template: "",
        sample_response: "",
        sample_status: 200,
      }),
    );
    await vi.waitFor(() =>
      expect(q("wh-preview")!.textContent).toContain(
        "POST https://example.com/hook\nContent-Type: application/json",
      ),
    );
    // 提示里的模板写法原样显示（写在模板里的 {{…}} 曾被格式化工具改成 {{ urlquery.site }}）
    expect(document.body.textContent).toContain("?site={{urlquery .site}}");
    expect(document.body.textContent).toContain("{{json .title}}");

    await pickPreset("Discord");
    await vi.waitFor(() => expect(field("wh-body").value).toContain('{"content":'));
    expect(q("wh-preset-hint")!.textContent).toContain("https://discord.com/api/webhooks/");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(
        expect.objectContaining({ body_template: expect.stringContaining('{"content":') }),
      ),
    );

    // 预览出错时写明原因
    api.preview.mockRejectedValue(
      new Error(JSON.stringify({ error: "invalid_template", detail: "请求体不是合法的 JSON" })),
    );
    typeInto("wh-body", '{"a": {{.title}}}');
    await vi.waitFor(() =>
      expect(q("wh-preview-error")!.textContent).toContain("请求体不是合法的 JSON"),
    );
  });

  it("请求头不是 JSON 对象时就地写明，保存不提交；改对以后连同方法、格式与请求体一起保存", async () => {
    await mountDetail();
    typeInto("wh-headers", '{"X-A": 1}');
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("请求头要是 JSON 对象，键和值都是文字"),
    );
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await new Promise((r) => setTimeout(r, 50));
    expect(api.update).not.toHaveBeenCalled();

    typeInto("wh-headers", '{"X-A": "2"}');
    await pickPreset("纯文本");
    await vi.waitFor(() => expect(field("wh-body").value).toBe("{{.title}}\n\n{{.text}}"));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).toMatchObject({
      endpoint_url: "https://example.com/hook",
      headers: '{"X-A": "2"}',
      method: "POST",
      content_type: "text",
      body_template: "{{.title}}\n\n{{.text}}",
    });
  });

  it("已有请求体模板时改成 GET：预览与保存都按不带请求体发；没保存前切回来模板还在", async () => {
    await mountDetail({ body_template: '{"a": {{json .title}}}' });
    await vi.waitFor(() => expect(field("wh-body").value).toBe('{"a": {{json .title}}}'));
    await pickOption("wh-method", "GET");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(
        expect.objectContaining({ method: "GET", body_template: "" }),
      ),
    );
    await pickOption("wh-method", "POST");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(
        expect.objectContaining({ method: "POST", body_template: '{"a": {{json .title}}}' }),
      ),
    );
    expect(field("wh-body").value).toBe('{"a": {{json .title}}}');

    await pickOption("wh-method", "GET");
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).toMatchObject({ method: "GET", body_template: "" });
  });

  it("成功条件：套用 PushPlus 预设带上成功条件，拿一份响应试，保存时一起发出", async () => {
    await mountDetail();
    await pickPreset("PushPlus");
    await vi.waitFor(() => expect(field("wh-success").value).toContain("eq .json.code 200"));
    api.preview.mockResolvedValue({
      method: "POST",
      url: "https://example.com/hook",
      content_type: "application/json",
      body: "{}",
      success: false,
      success_reason: "webhook 响应不满足成功条件：无效的用户token",
    });
    typeInto("wh-sample-response", '{"code":903,"msg":"无效的用户token"}');
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(
        expect.objectContaining({
          success_template: expect.stringContaining("eq .json.code 200"),
          sample_response: '{"code":903,"msg":"无效的用户token"}',
        }),
      ),
    );
    await vi.waitFor(() =>
      expect(q("wh-success-note")!.textContent).toContain(
        "算发送失败（webhook 响应不满足成功条件：无效的用户token）",
      ),
    );
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).toMatchObject({
      success_template: expect.stringContaining("eq .json.code 200"),
    });
    expect(api.update.mock.calls[0]![1]).not.toHaveProperty("sample_response");

    // 状态码随预览一起发；空响应也显示试算结果
    const status = field("wh-sample-status") as HTMLInputElement;
    status.value = "201";
    status.dispatchEvent(new Event("input"));
    status.dispatchEvent(new Event("change"));
    typeInto("wh-sample-response", "");
    api.preview.mockResolvedValue({
      method: "POST",
      url: "https://example.com/hook",
      content_type: "application/json",
      body: "{}",
      success: true,
    });
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(
        expect.objectContaining({ sample_status: 201, sample_response: "" }),
      ),
    );
    await vi.waitFor(() => expect(q("wh-success-note")!.textContent).toContain("算发送成功"));
  });

  it("GET 不带请求体：格式、预设与请求体都不能改", async () => {
    await mountDetail({ method: "GET" });
    await vi.waitFor(() => expect((field("wh-body") as HTMLTextAreaElement).disabled).toBe(true));
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenCalledWith(expect.objectContaining({ method: "GET" })),
    );
  });
});

describe("Webhook 预览的事件", () => {
  it("只列通知：命令回复与助手回复到不了 Webhook", async () => {
    await mountDetail();
    await pickOption("wh-preview-event", "通道测试");
    const names = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].map(
      (o) => o.textContent?.trim(),
    );
    expect(names).toContain("RSS 命中过滤规则");
    expect(names).not.toContain("/status 状态概览");
    expect(names).not.toContain("AI 助手的回复");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith(expect.objectContaining({ type: "test" })),
    );
  });
});
