// @vitest-environment happy-dom
/*
 * 钉子：AI 助手的输入框（真的 Element-Plus-X XSender）。写了字或者有传完的附件才能发；附件还在传时不能发；
 * 回答中只有停止；选的文件交给页面；删附件按钮一直在（不靠鼠标悬停）。
 * 语音输入：配了 Voltip 才有麦克风；点一下录、再点一下识别，文字插进输入框（英文之间补空格）；Esc 取消；录不了、识别失败写原因。
 */
import ElementPlus from "element-plus";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp, h, nextTick, reactive, ref } from "vue";

import { ApiError } from "@/api";
import type { ComposerAttachment } from "@/utils/assistant";
import AssistantComposer from "./AssistantComposer.vue";

/** 假的录音：start 按 rec.startError 成败，stop 给一段假 WAV；每次 new 记进 rec.made */
const rec = vi.hoisted(() => ({
  unavailable: "",
  startError: null as unknown,
  made: [] as {
    seconds: number;
    start: ReturnType<typeof vi.fn>;
    stop: ReturnType<typeof vi.fn>;
    cancel: ReturnType<typeof vi.fn>;
  }[],
}));
const api = vi.hoisted(() => ({ transcribe: vi.fn() }));
const ui = vi.hoisted(() => ({
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  success: vi.fn(),
}));

vi.mock("@/utils/voiceRecorder", () => ({
  VOICE_MAX_SECONDS: 300,
  voiceUnavailable: () => rec.unavailable,
  VoiceRecorder: class {
    seconds = 0;
    onLevel?: (l: number) => void;
    start = vi.fn(async () => {
      if (rec.startError) throw rec.startError;
    });
    stop = vi.fn(async () => new Blob(["RIFF"], { type: "audio/wav" }));
    cancel = vi.fn(async () => undefined);
    constructor() {
      rec.made.push(this);
    }
  },
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, assistantApi: { ...real.assistantApi, ...api } };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

const flush = () => new Promise((r) => setTimeout(r, 0));
/** x-sender 的 setText、clear 有一段放在 setImmediate 里做：等它做完再卸载，免得卸载以后才跑 */
const settle = async () => {
  await new Promise((r) => setImmediate(r));
  await flush();
};
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

let unmount: (() => void) | null = null;
afterEach(() => {
  unmount?.();
  unmount = null;
  document.body.innerHTML = "";
  rec.unavailable = "";
  rec.startError = null;
  rec.made.length = 0;
  vi.clearAllMocks();
});

function mount(
  initial: { running?: boolean; attachments?: ComposerAttachment[]; voice?: boolean } = {},
) {
  document.body.innerHTML = '<div id="app"></div>';
  const state = reactive({
    running: initial.running ?? false,
    attachments: initial.attachments ?? [],
    model: "deepseek-flash",
    effort: "" as const,
  });
  const events: { submit: string[]; cancel: number; files: File[][]; remove: string[] } = {
    submit: [],
    cancel: 0,
    files: [],
    remove: [],
  };
  const composer = ref<InstanceType<typeof AssistantComposer> | null>(null);
  const app = createApp({
    render: () =>
      h(AssistantComposer, {
        ref: composer,
        running: state.running,
        sending: false,
        attachments: state.attachments,
        models: ["deepseek-flash", "deepseek-v4-pro"],
        modelsLoading: false,
        modelsError: "",
        defaultModel: "deepseek-flash",
        voice: initial.voice ?? false,
        model: state.model,
        "onUpdate:model": (v: string) => (state.model = v),
        effort: state.effort,
        onSubmit: (t: string) => events.submit.push(t),
        onCancel: () => events.cancel++,
        onFiles: (f: File[]) => events.files.push(f),
        onRemove: (uid: string) => events.remove.push(uid),
      }),
  });
  app.use(ElementPlus);
  app.mount("#app");
  unmount = () => app.unmount();
  return { state, events, composer };
}

describe("AssistantComposer", () => {
  it("写了字才能发；发的是去掉首尾空白的文字", async () => {
    const { events, composer } = mount();
    await flush();
    const send = q("as-send") as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    composer.value!.setText("  下载器怎么样  ");
    await nextTick();
    expect(send.disabled).toBe(false);
    send.click();
    // 等 XSender 把这一帧的输入记进模型再读（见 submit 的注释）
    await vi.waitFor(() => expect(events.submit).toEqual(["下载器怎么样"]));
    composer.value!.clear();
    await nextTick();
    expect(send.disabled).toBe(true);
    await settle();
  });

  it("附件传完了不写字也能发，还在传时不能发；删附件的按钮一直在", async () => {
    const att = (status: ComposerAttachment["status"]): ComposerAttachment => ({
      uid: "a1",
      name: "qb.log",
      kind: "text",
      size: 20,
      status,
      id: status === "done" ? 3 : undefined,
    });
    const { state, events } = mount({ attachments: [att("uploading")] });
    await flush();
    expect((q("as-send") as HTMLButtonElement).disabled).toBe(true);
    state.attachments = [att("done")];
    await flush();
    expect((q("as-send") as HTMLButtonElement).disabled).toBe(false);
    expect(q("as-attachments")!.textContent).toContain("qb.log");
    q("as-attachment-del-a1")!.click();
    expect(events.remove).toEqual(["a1"]);
  });

  it("回答中只有停止", async () => {
    const { events } = mount({ running: true });
    await flush();
    expect(q("as-send")).toBeNull();
    q("as-stop")!.click();
    expect(events.cancel).toBe(1);
  });

  it("选的文件交给页面；最多 4 个附件时不能再加", async () => {
    const full = Array.from({ length: 4 }, (_, i) => ({
      uid: `a${i}`,
      name: `${i}.txt`,
      kind: "text" as const,
      size: 1,
      status: "done" as const,
      id: i + 1,
    }));
    const { state, events } = mount();
    await flush();
    const file = q("as-file") as HTMLInputElement;
    const f = new File(["x"], "a.md", { type: "text/markdown" });
    Object.defineProperty(file, "files", { value: [f], configurable: true });
    file.dispatchEvent(new Event("change"));
    expect(events.files).toEqual([[f]]);
    expect(file.getAttribute("accept")).toContain("image/png");
    expect(file.getAttribute("accept")).toContain(".log");
    state.attachments = full;
    await flush();
    expect((q("as-attach") as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("推理等级", () => {
  it("没选时显示「推理：默认」，不是占位文字", async () => {
    mount();
    await flush();
    expect(q("as-effort")!.textContent).toContain("推理：默认");
    expect(q("as-effort")!.textContent).not.toContain("请选择");
  });
});

describe("语音输入", () => {
  it("没配语音输入时没有麦克风", async () => {
    mount();
    await flush();
    expect(q("as-mic")).toBeNull();
  });

  it("点一下开始录（显示时长），再点一下识别：文字接在已有的字后面，可以直接发", async () => {
    const { events, composer } = mount({ voice: true });
    await flush();
    composer.value!.setText("帮我");
    await settle();
    api.transcribe
      .mockResolvedValueOnce(" 把沙丘重新整理一下 ")
      .mockResolvedValueOnce("and pause it");
    const mic = q("as-mic") as HTMLButtonElement;
    expect(mic.getAttribute("aria-label")).toBe("语音输入");
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    expect(mic.getAttribute("aria-pressed")).toBe("true");
    expect(q("as-mic-clock")!.textContent).toBe("0:00");
    mic.click();
    await vi.waitFor(() => expect(api.transcribe).toHaveBeenCalledTimes(1));
    expect(api.transcribe.mock.calls[0]![0]).toBeInstanceOf(Blob);
    expect(rec.made[0]!.stop).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(mic.className).toContain("is-idle"));
    await settle();

    // 英文接英文补一个空格
    composer.value!.setText(" Dune 2");
    await settle();
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    mic.click();
    await vi.waitFor(() => expect(api.transcribe).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(mic.className).toContain("is-idle"));
    await settle();
    (q("as-send") as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(events.submit).toEqual(["帮我把沙丘重新整理一下 Dune 2 and pause it"]),
    );
    await settle();
  });

  it("Esc 取消录音：不识别", async () => {
    mount({ voice: true });
    await flush();
    const mic = q("as-mic") as HTMLButtonElement;
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await vi.waitFor(() => expect(rec.made[0]!.cancel).toHaveBeenCalledTimes(1));
    expect(mic.className).toContain("is-idle");
    expect(api.transcribe).not.toHaveBeenCalled();
  });

  it("录到 5 分钟自动停下来识别", async () => {
    mount({ voice: true });
    await flush();
    api.transcribe.mockResolvedValue("很长的一段");
    const mic = q("as-mic") as HTMLButtonElement;
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    rec.made[0]!.seconds = 300;
    await vi.waitFor(() => expect(api.transcribe).toHaveBeenCalledTimes(1));
    expect(ui.info.mock.calls[0]![0]).toContain("最多录 5 分钟");
    await settle();
  });

  it("不是 HTTPS 时点了写明原因，不录音；没有麦克风权限时写怎么打开", async () => {
    rec.unavailable = "浏览器只在 HTTPS 或 localhost 下允许用麦克风";
    mount({ voice: true });
    await flush();
    const mic = q("as-mic") as HTMLButtonElement;
    mic.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith(rec.unavailable);
    expect(rec.made).toHaveLength(0);

    rec.unavailable = "";
    rec.startError = new DOMException("denied", "NotAllowedError");
    mic.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledTimes(1));
    expect(ui.error.mock.calls[0]![0]).toContain("没有麦克风权限");
    expect(rec.made[0]!.cancel).toHaveBeenCalled();
    expect(mic.className).toContain("is-idle");
  });

  it("识别失败写出原因；没听到说话时说一声，不动输入框", async () => {
    mount({ voice: true });
    await flush();
    const mic = q("as-mic") as HTMLButtonElement;
    api.transcribe.mockRejectedValue(
      new ApiError(502, "连不上 Voltip：确认 Voltip 的本机服务开着"),
    );
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    mic.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledTimes(1));
    expect(ui.error.mock.calls[0]![0]).toContain("连不上 Voltip");
    await vi.waitFor(() => expect(mic.className).toContain("is-idle"));

    api.transcribe.mockResolvedValue("  ");
    mic.click();
    await vi.waitFor(() => expect(mic.className).toContain("is-recording"));
    mic.click();
    await vi.waitFor(() => expect(ui.info).toHaveBeenCalledWith("没有听到说话"));
    expect((q("as-send") as HTMLButtonElement).disabled).toBe(true);
  });
});
