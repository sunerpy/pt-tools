// @vitest-environment happy-dom
/*
 * 钉子：飞书应用机器人的详情页。名单是 open_id（ou_ 开头的字符串），不能套用 Telegram 那个只留正整数的
 * parseUserIdList（会把 open_id 全部丢掉）：读回来的数组变成文字，保存时按逗号、空白、换行分开，
 * 去掉首尾空白、去重；有一项不是 ou_ 开头就提示并且不保存。App ID 与 App Secret 必填。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), test: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: { notifications: { get: api.get, update: api.update, test: api.test } },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountDetail() {
  const NotificationDetail = (await import("./NotificationDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/chatops/notifications/:id", component: NotificationDetail }],
  });
  await router.push("/chatops/notifications/7");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

function field(id: string): HTMLInputElement | HTMLTextAreaElement {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return el;
  return (el.querySelector("textarea") ?? el.querySelector("input")) as HTMLInputElement;
}

function fill(id: string, v: string) {
  const el = field(id);
  el.value = v;
  el.dispatchEvent(new Event("input"));
}

const DETAIL = {
  id: 7,
  channel_type: "feishu_app",
  name: "飞书机器人",
  enabled: true,
  app_id: "cli_a1b2",
  app_secret: "sec-value",
  domain: "feishu",
  admin_users: ["ou_admin", "ou_admin2"],
  allowed_users: ["ou_user"],
  default_chat_id: "oc_group",
  proxy_url: "",
};

describe("飞书应用机器人的详情", () => {
  it("回显凭证与名单，保存时名单去空白、去重，按 open_id 数组发出", async () => {
    api.get.mockResolvedValue(DETAIL);
    api.update.mockResolvedValue({ status: "ok" });
    await mountDetail();
    await vi.waitFor(() => expect(field("fs-app-id").value).toBe("cli_a1b2"));
    expect(field("fs-admin-users").value).toBe("ou_admin,ou_admin2");
    expect(field("fs-allowed-users").value).toBe("ou_user");
    expect(field("fs-default-chat").value).toBe("oc_group");
    expect(document.body.textContent).toContain("使用长连接接收事件");
    expect(document.body.textContent).toContain("飞书应用机器人 凭证");
    // 这一页的状态胶囊只看启用与否；「已连接」要到通道列表里看
    expect(document.body.textContent).toContain("通道列表里显示「已连接」以后");
    // 映射卡与安全卡：飞书应用机器人必填 App ID 与 App Secret，App Secret 加密落库
    expect(document.body.textContent).toContain("必填 app_id、app_secret");
    expect(document.body.textContent).toContain("Webhook Key、App Secret 以及代理凭据");
    expect(field("fs-default-chat").placeholder).toContain("on_");

    fill("fs-admin-users", " ou_admin ,ou_new\nou_admin  ou_x ");
    fill("fs-allowed-users", "");
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, payload] = api.update.mock.calls[0]!;
    expect(id).toBe(7);
    expect(payload).toEqual({
      app_id: "cli_a1b2",
      app_secret: "sec-value",
      domain: "feishu",
      admin_users: ["ou_admin", "ou_new", "ou_x"],
      allowed_users: [],
      default_chat_id: "oc_group",
      proxy_url: "",
    });
  });

  it("名单里有不是 ou_ 开头的项时提示并且不保存", async () => {
    api.get.mockResolvedValue(DETAIL);
    await mountDetail();
    await vi.waitFor(() => expect(field("fs-admin-users").value).toBe("ou_admin,ou_admin2"));
    fill("fs-allowed-users", "ou_ok, 12345678");
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("名单里要填 open_id，以 ou_ 开头：12345678 不是"),
    );
    expect(api.update).not.toHaveBeenCalled();
  });

  /*
   * 原来这条等的是 body 全文里出现提示，可这句话一直写在「操作提示文案」卡里，第一次检查就通过，
   * 接着断言 update 没被调用时 handleSaveCredentials 还停在 await validate()：删掉检查也照样通过。
   * 现在查弹出来的 .el-message（卡里的文字不算）：它出现时校验已经结束、走到了显式检查，再断言没有保存。
   * 用只有空白的值：el-form 的必填规则不拦它（留空由表单的「请填写 App ID」拦，happy-dom 里不渲染内联错误），
   * 拦它的只有这一步检查。
   */
  for (const [what, patch] of [
    ["App ID", { app_id: "   " }],
    ["App Secret", { app_secret: "  " }],
  ] as const) {
    it(`${what} 只有空白时提示并且不保存`, async () => {
      api.get.mockResolvedValue({ ...DETAIL, ...patch });
      await mountDetail();
      await vi.waitFor(() => expect(document.body.textContent).toContain("飞书机器人"));
      await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
      buttonByText("保存凭证").click();
      await vi.waitFor(() =>
        expect(toastTexts()).toContain("飞书应用机器人通道需填写 App ID 与 App Secret"),
      );
      expect(api.update).not.toHaveBeenCalled();
    });
  }
});

/** 弹出来的 ElMessage（不含「操作提示文案」卡里的文字） */
function toastTexts(): string[] {
  return [...document.querySelectorAll(".el-message")].map((e) => (e.textContent ?? "").trim());
}
