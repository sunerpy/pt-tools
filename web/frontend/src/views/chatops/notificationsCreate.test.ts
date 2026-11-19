// @vitest-environment happy-dom
/*
 * 钉子：新建通知通道对话框。只出站的通道在新建时就显示全部字段（自建服务器地址、鉴权、「允许内网地址」），
 * 不用先按默认的公共服务器创建再补；换类型时清掉上一个类型填的值（钉钉与飞书都有 webhook_url）；
 * 提交时只发当前类型的字段；必填项与格式不对时不提交。
 * 飞书应用机器人默认扫码一键创建：按开始时给的编号轮询，建好以后打开通道详情；关掉对话框或离开页面时按编号取消。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/api";
import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  logs: vi.fn(),
  stats: vi.fn(),
  recent: vi.fn(),
  regStart: vi.fn(),
  regStatus: vi.fn(),
  regCancel: vi.fn(),
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      notifications: { list: api.list, create: api.create, stats: api.stats, recent: api.recent },
      rssNotifications: { list: api.logs },
      feishuRegister: { start: api.regStart, status: api.regStatus, cancel: api.regCancel },
    },
  };
});

vi.mock("vue-router", async (orig) => {
  const real = await orig<typeof import("vue-router")>();
  return { ...real, useRouter: () => router };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
function inputOf(id: string): HTMLInputElement {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}
function fill(id: string, v: string) {
  const input = inputOf(id);
  input.value = v;
  input.dispatchEvent(new Event("input"));
}
async function pickType(type: string) {
  const opt = q(`channel-type-${type}`);
  if (!opt) throw new Error(`找不到类型 ${type}`);
  opt.click();
  await flush();
  await flush();
}

/** 飞书应用机器人的两种建法（分段控件） */
async function pickMode(label: string) {
  const item = [
    ...document.querySelectorAll<HTMLElement>("[data-testid=feishu-mode] .el-segmented__item"),
  ].find((el) => (el.textContent ?? "").trim() === label);
  if (!item) throw new Error(`找不到「${label}」`);
  item.click();
  await flush();
  await flush();
}

/** 新建对话框是不是开着（关掉以后 el-dialog 的内容还在 DOM 里，只是遮罩被 v-show 藏起来） */
function dialogOpen() {
  const overlay = q("add-channel-dialog")?.closest<HTMLElement>(".el-overlay");
  return !!overlay && overlay.style.display !== "none";
}

function regStarted(id: string) {
  return {
    id,
    url: `https://open.feishu.cn/page/launcher?user_code=${id}`,
    qr_svg: '<svg xmlns="http://www.w3.org/2000/svg"></svg>',
    expires_at: new Date(Date.now() + 600_000).toISOString(),
    state: "pending",
  };
}

async function openDialog() {
  api.list.mockResolvedValue([]);
  api.logs.mockResolvedValue({ items: [] });
  api.stats.mockResolvedValue({ since: "", days: 7, channels: [] });
  api.recent.mockResolvedValue({ items: [] });
  api.create.mockResolvedValue({ id: 1 });
  const Page = (await import("./Notifications.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
  q("add-channel-btn")!.click();
  await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
}

describe("新建通知通道", () => {
  it("Bark：新建时就能填服务器地址与「允许内网地址」，只发 Bark 的字段", async () => {
    await openDialog();
    await pickType("bark");
    for (const id of [
      "new-device_key",
      "new-server_url",
      "new-allow_private",
      "new-group",
      "new-sound",
    ]) {
      expect(q(id), id).not.toBeNull();
    }
    fill("name-input", "家里的 Bark");
    fill("new-device_key", " dev-key ");
    fill("new-server_url", "http://192.168.1.5:8080");
    q("new-allow_private")!.click();
    await flush();
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "bark",
      name: "家里的 Bark",
      enabled: true,
      device_key: "dev-key",
      server_url: "http://192.168.1.5:8080",
      allow_private: true,
    });
  });

  it("换类型清掉上一个类型填的值：钉钉的 Webhook 地址不会带到飞书", async () => {
    await openDialog();
    fill("name-input", "群机器人");
    await pickType("dingtalk");
    fill("new-webhook_url", "https://oapi.dingtalk.com/robot/send?access_token=abc");
    await pickType("feishu");
    expect(inputOf("new-webhook_url").value).toBe("");
    expect(inputOf("name-input").value, "名称留着").toBe("群机器人");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("请填写Webhook 地址"));
    expect(api.create).not.toHaveBeenCalled();

    // 换到 Bark 再换回钉钉：之前填的 device_key 也不会留着
    await pickType("bark");
    fill("new-device_key", "dev");
    await pickType("dingtalk");
    fill("new-webhook_url", "https://oapi.dingtalk.com/robot/send?access_token=abc");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "dingtalk",
      name: "群机器人",
      enabled: true,
      webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=abc",
    });
  });

  it("格式不对时不提交，并写明原因", async () => {
    await openDialog();
    fill("name-input", "ntfy");
    await pickType("ntfy");
    fill("new-topic", "a.b");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("Topic 只能是字母"));
    expect(api.create).not.toHaveBeenCalled();
  });

  it("Telegram 只发 bot_token", async () => {
    await openDialog();
    fill("name-input", "tg");
    fill("bot-token-input", "123:abc");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "telegram",
      name: "tg",
      enabled: true,
      bot_token: "123:abc",
    });
  });

  it("飞书应用机器人：只发 App ID、App Secret 与平台，平台默认飞书", async () => {
    await openDialog();
    fill("name-input", "飞书");
    await pickType("feishu_app");
    await pickMode("填写 App ID 与 App Secret");
    // 名单外提示要等订阅了「接收消息 v2.0」、发布了版本以后事件才推过来：先写前置步骤，再写拿 open_id
    expect(document.body.textContent).toContain("添加「接收消息 v2.0」并发布版本");
    expect(document.body.textContent).toContain("私聊机器人发一条消息，它会回你的 open_id");
    fill("new-app_id", " cli_a1b2 ");
    fill("new-app_secret", " sec ");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "feishu_app",
      name: "飞书",
      enabled: true,
      app_id: "cli_a1b2",
      app_secret: "sec",
      domain: "feishu",
    });
  });

  it("飞书应用机器人：可以选 Lark 国际版", async () => {
    await openDialog();
    fill("name-input", "Lark");
    await pickType("feishu_app");
    await pickMode("填写 App ID 与 App Secret");
    fill("new-app_id", "cli_x");
    fill("new-app_secret", "s");
    q("domain-lark")!.click();
    await flush();
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toMatchObject({
      channel_type: "feishu_app",
      domain: "lark",
    });
  });

  it("飞书应用机器人：缺 App ID 或 App Secret 时不提交", async () => {
    await openDialog();
    fill("name-input", "飞书");
    await pickType("feishu_app");
    await pickMode("填写 App ID 与 App Secret");
    fill("new-app_id", "cli_x");
    buttonByText("创建通道").click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("飞书应用机器人通道需填写 App ID 与 App Secret"),
    );
    fill("new-app_id", "");
    fill("new-app_secret", "s");
    buttonByText("创建通道").click();
    await flush();
    expect(api.create).not.toHaveBeenCalled();
  });

  it("飞书应用机器人：默认扫码一键创建；按编号轮询，建好以后刷新列表并打开通道详情", async () => {
    await openDialog();
    await pickType("feishu_app");
    expect(q("feishu-mode")!.querySelector(".is-selected")?.textContent).toContain(
      "扫码一键创建（推荐）",
    );
    expect(inputOf("name-input").value, "名称默认「飞书应用机器人」").toBe("飞书应用机器人");
    expect(q("new-app_id"), "扫码模式不显示手动填写的字段").toBeNull();
    const text = document.body.textContent ?? "";
    expect(text).toContain("用要接 pt-tools 的那个企业的飞书账号扫码");
    expect(text).toContain("应用名在飞书的确认页上可以改");
    expect(text).toContain("建好以后不用发布版本");
    expect(text).toContain("扫码的人自动成为管理员并直接绑定");
    expect(text).toContain("只在通道详情页里看得到");

    api.regStart.mockResolvedValue(regStarted("reg-1"));
    api.regStatus
      .mockResolvedValueOnce({ id: "reg-1", state: "pending", expires_at: "" })
      .mockResolvedValue({
        id: "reg-1",
        state: "done",
        expires_at: "",
        app_id: "cli_x",
        conf_id: 7,
      });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(api.regStart).toHaveBeenCalledWith("飞书应用机器人"));
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    expect(q("feishu-reg-qr")!.querySelector("img")!.getAttribute("src")).toContain(
      "data:image/svg+xml",
    );
    expect(q("feishu-reg-open")!.getAttribute("href")).toBe(
      "https://open.feishu.cn/page/launcher?user_code=reg-1",
    );
    expect(q("feishu-reg-open")!.textContent).toContain("在浏览器里打开");
    expect(q("feishu-reg-state")!.textContent).toContain("等待扫码与确认");
    expect(document.body.textContent).toContain("10 分钟内有效");

    await vi.waitFor(() => expect(router.push).toHaveBeenCalledWith("/chatops/notifications/7"), {
      timeout: 8000,
    });
    expect(api.regStatus.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(
      api.regStatus.mock.calls.every((c) => c[0] === "reg-1"),
      "按编号查",
    ).toBe(true);
    expect(document.querySelector(".el-message--success")?.textContent).toContain(
      "已经建好通道，正在连接飞书",
    );
    expect(api.list, "建好以后刷新通道列表").toHaveBeenCalledTimes(2);
    expect(api.regCancel, "建好了不取消").not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
  });

  it("飞书应用机器人：建好了但绑定没成，用警告提示原文再打开通道详情", async () => {
    await openDialog();
    await pickType("feishu_app");
    fill("name-input", "团队飞书");
    const message = "通道已经建好，绑定失败：database is locked，请在绑定页手动绑定";
    api.regStart.mockResolvedValue(regStarted("reg-2"));
    api.regStatus.mockResolvedValue({
      id: "reg-2",
      state: "done",
      expires_at: "",
      conf_id: 8,
      message,
    });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(api.regStart).toHaveBeenCalledWith("团队飞书"));
    await vi.waitFor(() => expect(router.push).toHaveBeenCalledWith("/chatops/notifications/8"), {
      timeout: 8000,
    });
    expect(document.querySelector(".el-message--warning")?.textContent).toContain(message);
    expect(document.querySelector(".el-message--success")).toBeNull();
  });

  it("飞书应用机器人：过期以后可以重新生成，写明之前的二维码已经作废", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart
      .mockResolvedValueOnce(regStarted("reg-3"))
      .mockResolvedValueOnce(regStarted("reg-4"));
    api.regStatus.mockImplementation(async (id: string) =>
      id === "reg-3"
        ? { id, state: "expired", expires_at: "", message: "二维码过期了" }
        : { id, state: "pending", expires_at: "" },
    );
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-reason")?.textContent).toContain("二维码过期了"), {
      timeout: 8000,
    });
    expect(q("feishu-reg-state")!.textContent).toContain("已过期");
    expect(document.body.textContent).not.toContain("作废的二维码不要再扫");
    buttonByText("重新生成").click();
    await vi.waitFor(() => expect(api.regStart).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(q("feishu-reg-open")!.getAttribute("href")).toContain("reg-4"));
    expect(document.body.textContent).toContain(
      "作废的二维码不要再扫；扫过的话，先看通道列表里有没有刚建好的通道，没有再到开发者后台删掉多出来的应用",
    );
    expect(q("feishu-reg-reason")).toBeNull();
    await vi.waitFor(() => expect(api.regStatus).toHaveBeenCalledWith("reg-4"), { timeout: 8000 });
    expect(api.regCancel, "过期的不用取消").not.toHaveBeenCalled();
  });

  it("飞书应用机器人：被别处新开的取代以后写明作废，可以重新生成", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-5"));
    api.regStatus.mockRejectedValue(
      new ApiError(
        404,
        JSON.stringify({ error: "not_found", detail: "这个一键创建已经被新的取代或过期" }),
      ),
    );
    buttonByText("生成二维码").click();
    await vi.waitFor(
      () =>
        expect(q("feishu-reg-reason")?.textContent).toContain("这个一键创建已经被新的取代或过期"),
      { timeout: 8000 },
    );
    expect(q("feishu-reg-state")!.textContent).toContain("已作废");
    // 被取代时那一个可能已经建好了：先看列表（这时刷新一次），没有再去删应用
    expect(document.body.textContent).toContain(
      "作废的二维码不要再扫；扫过的话，先看通道列表里有没有刚建好的通道，没有再到开发者后台删掉多出来的应用",
    );
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(buttonByText("重新生成")).toBeTruthy();
    buttonByText("关闭").click();
    await flush();
    expect(api.regCancel, "已经作废的不用再取消").not.toHaveBeenCalled();
  });

  it("飞书应用机器人：开始失败时写明原因，不显示二维码", async () => {
    await openDialog();
    await pickType("feishu_app");
    const detail =
      "飞书一键创建失败: dial tcp: connection refused。可以重新生成二维码，或者改用「填写 App ID 与 App Secret」";
    api.regStart.mockRejectedValue(new Error(JSON.stringify({ error: "register_failed", detail })));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain(detail));
    expect(q("feishu-reg-qr")).toBeNull();
    expect(api.regStatus).not.toHaveBeenCalled();
  });

  it("飞书应用机器人：关掉对话框时按编号取消还在等的那一个，并写明二维码作废", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-6"));
    api.regStatus.mockResolvedValue({ id: "reg-6", state: "pending", expires_at: "" });
    api.regCancel.mockResolvedValue({ id: "reg-6", state: "canceled", expires_at: "" });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-6"));
    expect(api.regCancel).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(document.body.textContent).toContain("二维码已经作废，不要再扫"));
    expect(dialogOpen()).toBe(false);
    // 再打开对话框：不接着原来的二维码
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(dialogOpen()).toBe(true));
    await pickType("feishu_app");
    expect(q("feishu-reg-qr")).toBeNull();
    expect(buttonByText("生成二维码")).toBeTruthy();
  });

  it("飞书应用机器人：关掉对话框那一刻飞书已经建好应用：不说作废，问到建好以后刷新列表并打开详情", async () => {
    await openDialog();
    await pickType("feishu_app");
    let server = "pending";
    api.regStart.mockResolvedValue(regStarted("reg-8"));
    api.regStatus.mockImplementation(async (id: string) =>
      server === "done"
        ? { id, state: "done", expires_at: "", app_id: "cli_x", conf_id: 9 }
        : { id, state: server, expires_at: "" },
    );
    // 取消的那一刻后端已经拿到凭据：取消不起作用，回 saving
    api.regCancel.mockImplementation(async (id: string) => {
      server = "done";
      return { id, state: "saving", expires_at: "", app_id: "cli_x" };
    });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-8"));
    await vi.waitFor(() => expect(router.push).toHaveBeenCalledWith("/chatops/notifications/9"), {
      timeout: 8000,
    });
    expect(document.querySelector(".el-message--success")?.textContent).toContain(
      "已经建好通道，正在连接飞书",
    );
    expect(document.body.textContent).not.toContain("二维码已经作废");
    expect(api.list, "建好以后刷新通道列表").toHaveBeenCalledTimes(2);
    expect(api.regCancel).toHaveBeenCalledTimes(1);
  });

  it("飞书应用机器人：关掉对话框时后端已经建好：提示并打开详情；对话框又打开了就不跳走", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-9"));
    api.regStatus.mockResolvedValue({ id: "reg-9", state: "pending", expires_at: "" });
    let release!: (v: unknown) => void;
    api.regCancel.mockReturnValue(new Promise((r) => (release = r)));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-9"));
    // 取消的回应还在路上，用户又打开了对话框
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
    release({ id: "reg-9", state: "done", expires_at: "", app_id: "cli_y", conf_id: 11 });
    await vi.waitFor(() =>
      expect(document.querySelector(".el-message--success")?.textContent).toContain(
        "已经建好通道，正在连接飞书",
      ),
    );
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(router.push, "对话框开着，不跳走").not.toHaveBeenCalled();
    expect(dialogOpen(), "也不关掉又打开的对话框").toBe(true);
    expect(document.body.textContent).not.toContain("二维码已经作废");
  });

  it("飞书应用机器人：关掉对话框以后保存通道失败：写明飞书那边已经建好的应用怎么处理", async () => {
    await openDialog();
    await pickType("feishu_app");
    const message =
      "飞书应用已经建好（App ID cli_z），但保存通道失败：database is locked；请在开发者后台复制 App Secret 手动新建通道";
    api.regStart.mockResolvedValue(regStarted("reg-10"));
    api.regStatus.mockResolvedValue({ id: "reg-10", state: "pending", expires_at: "" });
    api.regCancel.mockResolvedValue({
      id: "reg-10",
      state: "failed",
      expires_at: "",
      app_id: "cli_z",
      message,
    });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() =>
      expect(document.querySelector(".el-message--warning")?.textContent).toContain(message),
    );
    expect(document.body.textContent).not.toContain("二维码已经作废");
    expect(router.push).not.toHaveBeenCalled();
  });

  it("飞书应用机器人：取消两次都没送到：不说作废，也不断言结果，请用户过一会儿看列表", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-11"));
    api.regStatus.mockResolvedValue({ id: "reg-11", state: "pending", expires_at: "" });
    api.regCancel.mockRejectedValue(new Error("Network Error"));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(
      () =>
        expect(document.querySelector(".el-message--warning")?.textContent).toContain(
          "没能取消这个二维码（网络出错）：扫过的话，过一会儿看看通道列表里有没有刚建好的通道",
        ),
      { timeout: 6000 },
    );
    expect(api.regCancel, "隔 2 秒再试一次").toHaveBeenCalledTimes(2);
    expect(document.body.textContent).not.toContain("二维码已经作废");
    expect(document.body.textContent).not.toContain("通道照常会建好");
  });

  it("飞书应用机器人：取消第一次没送到、再试一次成功：按回应提示", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-22"));
    api.regStatus.mockResolvedValue({ id: "reg-22", state: "pending", expires_at: "" });
    api.regCancel
      .mockRejectedValueOnce(new Error("Network Error"))
      .mockResolvedValue({ id: "reg-22", state: "canceled", expires_at: "", message: "已取消" });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(
      () => expect(document.body.textContent).toContain("二维码已经作废，不要再扫"),
      {
        timeout: 6000,
      },
    );
    expect(api.regCancel).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).not.toContain("没能取消");
  });

  it("飞书应用机器人：取消时那一个已经被别处取代（404）：刷新列表，提示先看列表再删应用", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-12"));
    api.regStatus.mockResolvedValue({ id: "reg-12", state: "pending", expires_at: "" });
    api.regCancel.mockRejectedValue(
      new ApiError(
        404,
        JSON.stringify({ error: "not_found", detail: "这个一键创建已经被新的取代或过期" }),
      ),
    );
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-12"));
    // 被取代时那一个可能已经建好了：刷新列表；提示不叫人直接删应用
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        "这个二维码已经被新的取代或过期：扫过的话，先看通道列表里有没有刚建好的通道，没有再到开发者后台删掉多出来的应用",
      ),
    );
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(api.regCancel).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("二维码已经作废，不要再扫");
  });

  it("飞书应用机器人：生成二维码的请求在路上时不能换类型、创建方式与名称", async () => {
    await openDialog();
    await pickType("feishu_app");
    let release!: (v: unknown) => void;
    api.regStart.mockReturnValue(new Promise((r) => (release = r)));
    api.regStatus.mockResolvedValue({ id: "reg-13", state: "pending", expires_at: "" });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(api.regStart).toHaveBeenCalled());
    await flush();
    const modes = [...q("feishu-mode")!.querySelectorAll(".el-segmented__item")];
    expect(modes.length).toBe(2);
    expect(
      modes.every((el) => el.classList.contains("is-disabled")),
      "创建方式",
    ).toBe(true);
    expect(inputOf("name-input").disabled, "名称").toBe(true);
    expect(
      document
        .querySelector<HTMLElement>("[data-testid=add-channel-dialog] .el-select")!
        .classList.contains("is-disabled") ||
        !!document.querySelector(
          "[data-testid=add-channel-dialog] .el-select__wrapper.is-disabled",
        ),
      "通道类型",
    ).toBe(true);
    release(regStarted("reg-13"));
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
  });

  it("飞书应用机器人：取消的回应迟到、已经又生成了新的二维码：不提示作废，也不动新的", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart
      .mockResolvedValueOnce(regStarted("reg-14"))
      .mockResolvedValueOnce(regStarted("reg-15"));
    api.regStatus.mockImplementation(async (id: string) => ({
      id,
      state: "pending",
      expires_at: "",
    }));
    let release!: (v: unknown) => void;
    api.regCancel.mockReturnValueOnce(new Promise((r) => (release = r)));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-14"));
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
    await pickType("feishu_app");
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-open")?.getAttribute("href")).toContain("reg-15"));
    release({ id: "reg-14", state: "canceled", expires_at: "", message: "已取消" });
    await flush();
    await flush();
    expect(document.body.textContent, "迟到的作废提示会被当成新二维码的").not.toContain(
      "二维码已经作废",
    );
    expect(q("feishu-reg-open")!.getAttribute("href")).toContain("reg-15");
    expect(api.regCancel, "新的那一个不取消").toHaveBeenCalledTimes(1);
  });

  it("飞书应用机器人：取消的回应迟到说已经建好、新的二维码正在显示：照常提示建好并刷新列表，不动新的", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart
      .mockResolvedValueOnce(regStarted("reg-20"))
      .mockResolvedValueOnce(regStarted("reg-21"));
    api.regStatus.mockImplementation(async (id: string) => ({
      id,
      state: "pending",
      expires_at: "",
    }));
    let release!: (v: unknown) => void;
    api.regCancel.mockReturnValueOnce(new Promise((r) => (release = r)));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-20"));
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
    await pickType("feishu_app");
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-open")?.getAttribute("href")).toContain("reg-21"));
    release({ id: "reg-20", state: "done", expires_at: "", app_id: "cli_a", conf_id: 12 });
    await vi.waitFor(() =>
      expect(document.querySelector(".el-message--success")?.textContent).toContain(
        "已经建好通道，正在连接飞书",
      ),
    );
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(router.push, "对话框开着，不跳走").not.toHaveBeenCalled();
    expect(q("feishu-reg-open")!.getAttribute("href"), "新的二维码还在").toContain("reg-21");
    expect(api.regCancel, "新的那一个不取消").toHaveBeenCalledTimes(1);
    expect(document.body.textContent).not.toContain("二维码已经作废");
  });

  it("飞书应用机器人：取消的回应迟到、对话框又打开了：不提示作废", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-16"));
    api.regStatus.mockResolvedValue({ id: "reg-16", state: "pending", expires_at: "" });
    let release!: (v: unknown) => void;
    api.regCancel.mockReturnValueOnce(new Promise((r) => (release = r)));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-16"));
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
    release({ id: "reg-16", state: "canceled", expires_at: "", message: "已取消" });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("二维码已经作废");
  });

  it("飞书应用机器人：取消的回应迟到、已经离开页面：不提示作废", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-17"));
    api.regStatus.mockResolvedValue({ id: "reg-17", state: "pending", expires_at: "" });
    let release!: (v: unknown) => void;
    api.regCancel.mockReturnValueOnce(new Promise((r) => (release = r)));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-17"));
    view!.unmount();
    view = null;
    release({ id: "reg-17", state: "canceled", expires_at: "", message: "已取消" });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("二维码已经作废");
  });

  it("飞书应用机器人：取消没送到、已经又生成了新的二维码：不提示没能取消", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart
      .mockResolvedValueOnce(regStarted("reg-18"))
      .mockResolvedValueOnce(regStarted("reg-19"));
    api.regStatus.mockImplementation(async (id: string) => ({
      id,
      state: "pending",
      expires_at: "",
    }));
    let fail!: (e: unknown) => void;
    api.regCancel
      .mockReturnValueOnce(new Promise((_, rej) => (fail = rej)))
      .mockRejectedValue(new Error("Network Error"));
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    buttonByText("关闭").click();
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-18"));
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
    await pickType("feishu_app");
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-open")?.getAttribute("href")).toContain("reg-19"));
    fail(new Error("Network Error"));
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledTimes(2), { timeout: 6000 });
    await flush();
    await flush();
    expect(document.body.textContent).not.toContain("没能取消这个二维码");
    expect(q("feishu-reg-open")!.getAttribute("href"), "新的二维码还在").toContain("reg-19");
  });

  it("飞书应用机器人：生成二维码的请求在路上时关掉对话框再打开：新对话框不被锁住，旧请求回来时被取消", async () => {
    await openDialog();
    await pickType("feishu_app");
    let release!: (v: unknown) => void;
    api.regStart.mockReturnValueOnce(new Promise((r) => (release = r)));
    api.regCancel.mockResolvedValue({ id: "reg-23", state: "canceled", expires_at: "" });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(api.regStart).toHaveBeenCalled());
    await flush();
    expect(inputOf("name-input").disabled, "请求在路上时锁住").toBe(true);
    buttonByText("取消").click();
    await vi.waitFor(() => expect(dialogOpen()).toBe(false));
    q("add-channel-btn")!.click();
    await vi.waitFor(() => expect(dialogOpen()).toBe(true));
    await flush();
    expect(inputOf("name-input").disabled, "新打开的对话框不被上一轮的请求锁住").toBe(false);
    release(regStarted("reg-23"));
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-23"));
    await flush();
    expect(inputOf("name-input").disabled).toBe(false);
    expect(q("feishu-reg-qr"), "上一轮的二维码不显示").toBeNull();
  });

  it("飞书应用机器人：离开页面时也按编号取消", async () => {
    await openDialog();
    await pickType("feishu_app");
    api.regStart.mockResolvedValue(regStarted("reg-7"));
    api.regStatus.mockResolvedValue({ id: "reg-7", state: "pending", expires_at: "" });
    api.regCancel.mockResolvedValue({ id: "reg-7", state: "canceled", expires_at: "" });
    buttonByText("生成二维码").click();
    await vi.waitFor(() => expect(q("feishu-reg-qr")).not.toBeNull());
    view!.unmount();
    view = null;
    await vi.waitFor(() => expect(api.regCancel).toHaveBeenCalledWith("reg-7"));
  });

  it("后端拒绝时提示里只有原因，不是整段 JSON", async () => {
    await openDialog();
    fill("name-input", "Bark");
    await pickType("bark");
    fill("new-device_key", "k");
    fill("new-server_url", "http://192.168.1.5");
    const detail = "通道配置无效: Bark 服务器地址不能用：192.168.1.5 是本机或内网地址";
    api.create.mockRejectedValue(new Error(JSON.stringify({ error: "invalid_argument", detail })));
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain(detail));
    expect(document.body.textContent).not.toContain("invalid_argument");
  });
});
