// @vitest-environment happy-dom
/*
 * 钉子：添加 / 编辑 NAS 来源的弹窗（首页 NAS 概览 H2，方案 3.2.4）。字段跟着类型表的规则；测试连接带上填了的字段；
 * 证书不受信任时列出证书，信任以后带指纹再测、保存时钉住；要验证码时带验证码再测；测试通过的 pending 保存时带上；
 * 改了地址（或账号、密钥）以后测试结果与 pending 作废，改了地址时信任的证书与明文确认也作废；
 * 编辑时密钥留空是不改，改了地址时写明要重新填；清除密钥与新密钥、pending 互斥；请求失败写在表单下面。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import type { NasKind, NasSourceEdit } from "@/api";
import { type MountedView, mountView } from "@/test-mount";
import NasSourceDialog from "./NasSourceDialog.vue";

const api = vi.hoisted(() => ({
  getSource: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  test: vi.fn(),
}));
const ui = vi.hoisted(() => ({ success: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, nasApi: { ...real.nasApi, ...api } };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: vi.fn(),
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = async () => {
  await new Promise((r) => setTimeout(r, 0));
  await nextTick();
};
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const text = (id: string) => q(id)?.textContent?.replace(/\s+/g, " ").trim() ?? "";
const inputOf = (id: string) => {
  const el = q(id)!;
  return (el.matches("input") ? el : el.querySelector("input")) as HTMLInputElement;
};

async function type(id: string, value: string) {
  const input = inputOf(id);
  input.value = value;
  input.dispatchEvent(new Event("input"));
  await flush();
}

async function check(id: string) {
  inputOf(id).click();
  await flush();
}

async function click(id: string) {
  q(id)!.click();
  await flush();
}

/** 打开下拉框 id，点文字是 label 的选项 */
async function pick(id: string, label: string) {
  q(id)!.click();
  await flush();
  const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
    (el) => el.textContent?.trim() === label,
  );
  if (!opt) throw new Error(`下拉框 ${id} 里没有「${label}」`);
  opt.click();
  await flush();
}

const KINDS: NasKind[] = [
  {
    name: "truenas",
    label: "TrueNAS",
    experimental: false,
    username: "none",
    secret: "required",
    plain: "never",
  },
  {
    name: "glances",
    label: "Glances",
    experimental: false,
    username: "optional",
    secret: "with_username",
    plain: "without_credentials",
  },
  {
    name: "fnos",
    label: "飞牛",
    experimental: true,
    username: "required",
    secret: "required",
    plain: "with_ack",
  },
];

const SHA = "ab".repeat(32);
const SHA2 = "cd".repeat(32);

function edit(over: Partial<NasSourceEdit> = {}): NasSourceEdit {
  return {
    id: 7,
    name: "TrueNAS",
    kind: "truenas",
    enabled: true,
    url: "https://192.168.1.2",
    username: "",
    has_secret: true,
    has_extra: false,
    tls_pin_sha256: SHA,
    insecure_ack: false,
    ...over,
  };
}

function mountDialog(editId: number | null = null, kinds: NasKind[] = KINDS) {
  const open = ref(true);
  const id = ref<number | null>(editId);
  const saved = vi.fn();
  const Comp = defineComponent({
    setup() {
      return () =>
        h(NasSourceDialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          editId: id.value,
          kinds,
          onSaved: saved,
        });
    },
  });
  view = mountView(Comp);
  return { open, id, saved };
}

describe("添加来源", () => {
  it("名字默认是类型名，换类型时跟着换；没有用户名一栏的类型密钥叫 API Key", async () => {
    mountDialog();
    await flush();
    expect(inputOf("ns-name").value).toBe("TrueNAS");
    expect(q("ns-username"), "TrueNAS 没有用户名一栏").toBeNull();
    expect(document.querySelector(".el-dialog")!.textContent).toContain("API Key");
    expect(inputOf("ns-secret").placeholder).toBe("必填");

    await pick("ns-kind", "Glances");
    expect(inputOf("ns-name").value).toBe("Glances");
    expect(q("ns-username")).not.toBeNull();
    expect(document.querySelector(".el-dialog")!.textContent).toContain("密码");
    expect(inputOf("ns-secret").placeholder, "不开认证时不用密钥").toBe("可以不填");
    await type("ns-username", "admin");
    expect(inputOf("ns-secret").placeholder).toBe("必填");

    await type("ns-name", "书房的 NAS");
    await pick("ns-kind", "飞牛（实验性）");
    expect(inputOf("ns-name").value, "改过的名字不跟着换").toBe("书房的 NAS");
    expect(q("ns-experimental")).not.toBeNull();
  });

  it("测试连接带上填了的字段；连上以后保存带上 pending", async () => {
    const { saved, open } = mountDialog();
    await flush();
    await type("ns-url", "https://192.168.1.2/");
    await type("ns-secret", "1-abc");
    api.test.mockResolvedValueOnce({
      status: "ok",
      system: {
        hostname: "truenas",
        platform: "TrueNAS SCALE",
        version: "25.04.1",
        model: "i7",
        cores: 8,
        uptime_seconds: 1,
      },
      pending: "p-1",
    });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    expect(api.test).toHaveBeenCalledWith({
      kind: "truenas",
      url: "https://192.168.1.2/",
      secret: "1-abc",
    });
    expect(text("ns-ok")).toContain("truenas · TrueNAS SCALE 25.04.1 · i7");
    expect(q("ns-pending")).not.toBeNull();

    api.create.mockResolvedValueOnce(edit({ tls_pin_sha256: "" }));
    await click("ns-save");
    await vi.waitFor(() => expect(saved).toHaveBeenCalled());
    expect(api.create).toHaveBeenCalledWith({
      name: "TrueNAS",
      kind: "truenas",
      enabled: true,
      url: "https://192.168.1.2/",
      username: "",
      tls_pin_sha256: "",
      insecure_ack: false,
      secret: "1-abc",
      pending: "p-1",
    });
    expect(ui.success).toHaveBeenCalledWith("已保存");
    expect(open.value, "保存以后关掉").toBe(false);
  });

  it("证书不受信任：列出证书，信任以后带指纹再测，保存时钉住；改了地址以后不再用这个指纹", async () => {
    mountDialog();
    await flush();
    await type("ns-url", "https://192.168.1.2");
    await type("ns-secret", "1-abc");
    api.test.mockResolvedValueOnce({
      status: "certificate",
      message: "证书不受信任",
      certificate: {
        sha256: SHA,
        subject: "CN=truenas",
        issuer: "CN=truenas",
        not_before: "2026-01-01T00:00:00Z",
        not_after: "2020-01-01T00:00:00Z",
        dns_names: ["truenas.local"],
      },
    });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-cert")).not.toBeNull());
    const cert = text("ns-cert");
    expect(cert).toContain("证书不受信任");
    expect(cert).toContain("没有发出账号与密钥就断开了");
    expect(cert).toContain("CN=truenas（自签）");
    expect(cert).toContain("truenas.local");
    expect(cert).toContain("（已经过期）");
    expect(text("ns-cert-sha")).toBe(Array(32).fill("AB").join(":"));
    expect(q("ns-pin"), "还没信任").toBeNull();

    api.test.mockResolvedValueOnce({ status: "ok" });
    await click("ns-trust");
    await vi.waitFor(() => expect(api.test).toHaveBeenCalledTimes(2));
    expect(api.test.mock.calls[1]![0].trust_sha256).toBe(SHA);
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    expect(text("ns-pin")).toContain("AB:AB:AB:AB:…:AB:AB:AB:AB");

    api.create.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalled());
    expect(api.create.mock.calls[0]![0].tls_pin_sha256).toBe(SHA);
    expect(api.create.mock.calls[0]![0].pending, "这次没有暂存凭据").toBeUndefined();
  });

  it("要验证码：出输入框，带验证码再测", async () => {
    mountDialog();
    await flush();
    await pick("ns-kind", "飞牛（实验性）");
    await type("ns-url", "https://192.168.1.3:5667");
    await type("ns-username", "admin");
    await type("ns-secret", "pw");
    api.test.mockResolvedValueOnce({ status: "otp_required", message: "账号开了两步验证" });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-otp")).not.toBeNull());
    expect(text("ns-otp")).toContain("账号开了两步验证");
    expect(q("ns-otp-submit")!.hasAttribute("disabled"), "没填验证码不能提交").toBe(true);
    await type("ns-otp-input", " 123456 ");
    api.test.mockResolvedValueOnce({ status: "ok", pending: "p-otp" });
    await click("ns-otp-submit");
    await vi.waitFor(() => expect(api.test).toHaveBeenCalledTimes(2));
    expect(api.test.mock.calls[1]![0]).toEqual({
      kind: "fnos",
      url: "https://192.168.1.3:5667",
      username: "admin",
      secret: "pw",
      otp: "123456",
    });
  });

  it("登录失败、连不上与请求本身失败（429）写出原因", async () => {
    mountDialog();
    await flush();
    await type("ns-url", "https://192.168.1.2");
    await type("ns-secret", "bad");
    api.test.mockResolvedValueOnce({ status: "auth_failed", message: "API Key 不对" });
    await click("ns-test");
    await vi.waitFor(() => expect(text("ns-fail")).toBe("登录失败API Key 不对"));
    api.test.mockResolvedValueOnce({ status: "error", message: "20 秒内没有回应" });
    await type("ns-secret", "bad2");
    await click("ns-test");
    await vi.waitFor(() => expect(text("ns-fail")).toBe("连不上20 秒内没有回应"));
    api.test.mockRejectedValueOnce(new Error("刚测过，过几秒再试\n"));
    await click("ns-test");
    await vi.waitFor(() => expect(text("ns-problem")).toBe("刚测过，过几秒再试"));
    expect(q("ns-fail")).toBeNull();
  });

  it("改了地址、账号或密钥以后测试结果与 pending 作废；保存不再带 pending", async () => {
    mountDialog();
    await flush();
    await pick("ns-kind", "Glances");
    await type("ns-url", "https://192.168.1.4:61208");
    api.test.mockResolvedValue({ status: "ok", pending: "p-g" });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    await type("ns-username", "glances");
    expect(q("ns-ok"), "账号变了").toBeNull();
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    await type("ns-secret", "x");
    expect(q("ns-ok"), "密钥变了").toBeNull();
    api.create.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalled());
    expect(api.create.mock.calls[0]![0].pending).toBeUndefined();
    expect(api.create.mock.calls[0]![0].username).toBe("glances");
  });

  it("http:// 地址：不收的类型写明只收 https；要确认的类型出确认框，改了地址取消勾选", async () => {
    mountDialog();
    await flush();
    await type("ns-url", "http://192.168.1.2");
    expect(text("ns-http-refused")).toBe("TrueNAS 只收 https:// 地址。");
    expect(q("ns-ack")).toBeNull();

    await pick("ns-kind", "Glances");
    expect(q("ns-ack"), "Glances 不带账号时不用确认").toBeNull();
    await type("ns-username", "glances");
    expect(q("ns-ack")).not.toBeNull();

    await pick("ns-kind", "飞牛（实验性）");
    expect(q("ns-ack")).not.toBeNull();
    await check("ns-ack");
    expect(inputOf("ns-ack").checked).toBe(true);
    await type("ns-url", "http://192.168.1.3:5666");
    expect(inputOf("ns-ack").checked, "改了地址要重新确认").toBe(false);
    await check("ns-ack");
    await type("ns-username", "admin");
    await type("ns-secret", "pw");
    api.test.mockResolvedValueOnce({ status: "ok" });
    await click("ns-test");
    await vi.waitFor(() => expect(api.test).toHaveBeenCalled());
    expect(api.test.mock.calls[0]![0].insecure_ack).toBe(true);
    api.create.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalled());
    expect(api.create.mock.calls[0]![0].insecure_ack).toBe(true);
  });

  it("名字与地址空着时不发请求，写明要填什么", async () => {
    mountDialog();
    await flush();
    await click("ns-test");
    expect(text("ns-problem")).toBe("先填地址");
    await type("ns-name", " ");
    await click("ns-save");
    expect(text("ns-problem")).toBe("要填名字");
    await type("ns-name", "NAS");
    await click("ns-save");
    expect(text("ns-problem")).toBe("要填地址");
    expect(api.test).not.toHaveBeenCalled();
    expect(api.create).not.toHaveBeenCalled();
  });
});

describe("编辑来源", () => {
  it("类型改不了；密钥留空是不改，保存带上保存过的指纹", async () => {
    api.getSource.mockResolvedValueOnce(edit());
    const { saved } = mountDialog(7);
    await vi.waitFor(() => expect(q("ns-kind-fixed")).not.toBeNull());
    expect(api.getSource).toHaveBeenCalledWith(7);
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("编辑来源 · TrueNAS");
    expect(text("ns-kind-fixed")).toBe("TrueNAS");
    expect(inputOf("ns-secret").placeholder).toBe("已保存，不改就留空");
    expect(q("ns-pin")).not.toBeNull();

    api.test.mockResolvedValueOnce({ status: "ok" });
    await click("ns-test");
    await vi.waitFor(() => expect(api.test).toHaveBeenCalled());
    expect(api.test.mock.calls[0]![0]).toEqual({
      kind: "truenas",
      url: "https://192.168.1.2",
      id: 7,
      trust_sha256: SHA,
    });

    api.update.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(saved).toHaveBeenCalled());
    expect(api.update).toHaveBeenCalledWith(7, {
      name: "TrueNAS",
      kind: "truenas",
      enabled: true,
      url: "https://192.168.1.2",
      username: "",
      tls_pin_sha256: SHA,
      insecure_ack: false,
    });
  });

  it("改了地址：写明要重新填密钥，不能清除，保存过的指纹不再用；改回来又能用", async () => {
    api.getSource.mockResolvedValueOnce(edit());
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-kind-fixed")).not.toBeNull());
    expect(q("ns-clear")).not.toBeNull();
    await type("ns-url", "https://nas.example.com");
    expect(inputOf("ns-secret").placeholder).toBe("改了地址或账号，要重新填");
    expect(q("ns-clear")).toBeNull();
    expect(q("ns-pin")).toBeNull();
    await type("ns-url", "https://192.168.1.2/");
    expect(q("ns-pin"), "地址改回来（末尾的 / 不算）").not.toBeNull();
    expect(inputOf("ns-secret").placeholder).toBe("已保存，不改就留空");

    await type("ns-url", "https://nas.example.com");
    api.update.mockResolvedValueOnce(edit());
    await type("ns-secret", "2-new");
    await click("ns-save");
    await vi.waitFor(() => expect(api.update).toHaveBeenCalled());
    expect(api.update.mock.calls[0]![1].tls_pin_sha256).toBe("");
    expect(api.update.mock.calls[0]![1].secret).toBe("2-new");
  });

  it("不再信任：去掉指纹；换了证书时信任新的那张", async () => {
    api.getSource.mockResolvedValueOnce(edit());
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-pin")).not.toBeNull());
    await click("ns-drop-pin");
    expect(q("ns-pin")).toBeNull();
    api.test.mockResolvedValueOnce({
      status: "certificate",
      message: "证书和信任的那张不一样（证书换了？）",
      certificate: {
        sha256: SHA2,
        subject: "CN=new",
        issuer: "CN=ca",
        not_before: "2026-01-01T00:00:00Z",
        not_after: "2099-01-01T00:00:00Z",
        dns_names: null,
      },
    });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-cert")).not.toBeNull());
    expect(api.test.mock.calls[0]![0].trust_sha256, "去掉了就不带").toBeUndefined();
    expect(text("ns-cert")).toContain("证书和信任的那张不一样（证书换了？）");
    expect(text("ns-cert")).not.toContain("（自签）");
    api.test.mockResolvedValueOnce({ status: "ok" });
    await click("ns-trust");
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    api.update.mockResolvedValueOnce(edit({ tls_pin_sha256: SHA2 }));
    await click("ns-save");
    await vi.waitFor(() => expect(api.update).toHaveBeenCalled());
    expect(api.update.mock.calls[0]![1].tls_pin_sha256).toBe(SHA2);
  });

  it("清除保存的密钥：密钥框关掉，保存只带 clear_secret，不带密钥与 pending", async () => {
    api.getSource.mockResolvedValueOnce(
      edit({ kind: "fnos", username: "admin", url: "https://192.168.1.3:5667", has_extra: true }),
    );
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-clear")).not.toBeNull());
    expect(text("ns-clear")).toBe("清除保存的密钥与登录凭据（设备令牌）");
    await type("ns-secret", "pw");
    await check("ns-clear");
    expect(inputOf("ns-secret").disabled).toBe(true);
    expect(inputOf("ns-secret").value).toBe("");
    expect(inputOf("ns-secret").placeholder).toBe("保存时清除");
    api.update.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(api.update).toHaveBeenCalled());
    const body = api.update.mock.calls[0]![1];
    expect(body.clear_secret).toBe(true);
    expect(body.secret).toBeUndefined();
    expect(body.pending).toBeUndefined();
  });

  it("pending 过期时保存失败：测试结果一起作废，写明要再测一次", async () => {
    api.getSource.mockResolvedValueOnce(edit());
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-kind-fixed")).not.toBeNull());
    api.test.mockResolvedValueOnce({ status: "ok", pending: "p-old" });
    await click("ns-test");
    await vi.waitFor(() => expect(q("ns-ok")).not.toBeNull());
    api.update.mockRejectedValueOnce(new Error("测试结果过期了，请再测一次\n"));
    await click("ns-save");
    await vi.waitFor(() => expect(text("ns-problem")).toBe("测试结果过期了，请再测一次"));
    expect(q("ns-ok")).toBeNull();
    api.update.mockResolvedValueOnce(edit());
    await click("ns-save");
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
    expect(api.update.mock.calls[1]![1].pending, "作废的不再带").toBeUndefined();
  });

  it("读不到来源时写原因；这个版本不认的类型只能删除", async () => {
    api.getSource.mockRejectedValueOnce(new Error("没有这个来源\n"));
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-load-error")).not.toBeNull());
    expect(text("ns-load-error")).toContain("没有这个来源");
    expect(q("ns-save")).toBeNull();
    view?.unmount();

    api.getSource.mockResolvedValueOnce(edit({ kind: "synology" }));
    mountDialog(7);
    await vi.waitFor(() => expect(q("ns-kind-missing")).not.toBeNull());
    expect(text("ns-kind-missing")).toBe("这个版本不认「synology」这种来源，只能删除。");
    expect(q("ns-save")).toBeNull();
    expect(q("ns-test")).toBeNull();
  });

  it("关掉再打开：重新读，表单里填过的密钥不留着", async () => {
    api.getSource.mockResolvedValue(edit());
    const { open } = mountDialog(7);
    await vi.waitFor(() => expect(q("ns-kind-fixed")).not.toBeNull());
    await type("ns-secret", "typed");
    await click("ns-cancel");
    expect(open.value).toBe(false);
    open.value = true;
    await vi.waitFor(() => expect(api.getSource).toHaveBeenCalledTimes(2));
    await flush();
    expect(inputOf("ns-secret").value).toBe("");
  });
});
