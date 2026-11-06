// @vitest-environment happy-dom
/*
 * 钉子：首页 NAS 概览的数据（首页 NAS 概览 H2）。挂载后马上取一次 GET /api/nas，之后页面可见时每 30 秒再取、
 * 隐藏时不取，切回标签页与「刷新」马上取；取到以后用它而不是首页接口带的 nas 块；一次没取到留着上一次的数据；
 * 慢的旧请求晚到时不盖掉新的；两边都没数据时给原因。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createApp, defineComponent, h, nextTick, ref } from "vue";

import type { HomeBlock, HomeNas, NasSource, NasView } from "@/api";
import { NAS_POLL_MS, NAS_TICK_MS, homeNasOf, useNasHome } from "./useNasHome";

const nas = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, nasApi: { ...real.nasApi, ...nas } };
});

function source(id: string, over: Partial<NasSource> = {}): NasSource {
  return {
    id,
    name: id === "local" ? "本机" : `NAS ${id}`,
    kind: id === "local" ? "local" : "truenas",
    enabled: true,
    experimental: false,
    has_secret: false,
    insecure_ack: false,
    state: "ok",
    snapshot: null,
    history: [],
    ...over,
  };
}

function view(home: string, sources: NasSource[]): NasView {
  return {
    home_source: "",
    home,
    local_enabled: true,
    local_supported: true,
    kinds: [],
    sources,
  };
}

const flush = async () => {
  for (let i = 0; i < 4; i++) await Promise.resolve();
  await nextTick();
};

let unmount: (() => void) | null = null;

function harness(block: HomeBlock<HomeNas> | null = null) {
  const blockRef = ref(block);
  const refresh = ref(0);
  let state!: ReturnType<typeof useNasHome>;
  const Comp = defineComponent({
    setup() {
      state = useNasHome(
        () => blockRef.value,
        () => refresh.value,
      );
      return () => h("div");
    },
  });
  const el = document.createElement("div");
  document.body.appendChild(el);
  const app = createApp(Comp);
  app.mount(el);
  unmount = () => {
    app.unmount();
    el.remove();
  };
  return { state, blockRef, refresh };
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
  Object.defineProperty(document, "hidden", { configurable: true, get: () => false });
});

afterEach(() => {
  unmount?.();
  unmount = null;
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("首页 NAS 概览的数据", () => {
  it("homeNasOf：首页显示的那个来源，其余只列能用的（启用了、这个平台能用）", () => {
    const got = homeNasOf(
      view("2", [
        source("local"),
        source("2"),
        source("3", { enabled: false, state: "disabled" }),
        source("4", { state: "unsupported" }),
        source("5", { state: "error" }),
      ]),
    );
    expect(got.source?.id).toBe("2");
    expect(got.others.map((o) => o.id)).toEqual(["local", "5"]);
    expect(got.others[1]).toEqual({ id: "5", name: "NAS 5", kind: "truenas", state: "error" });
    expect(homeNasOf(view("", [source("local", { enabled: false })])).source).toBeNull();
  });

  it("先用首页接口带的，挂载后马上取一次 GET /api/nas，取到以后用它", async () => {
    let answer!: (v: NasView) => void;
    nas.get.mockReturnValueOnce(new Promise<NasView>((r) => (answer = r)));
    const { state } = harness({ data: { source: source("local"), others: [] } });
    expect(nas.get).toHaveBeenCalledTimes(1);
    expect(nas.get.mock.calls[0]![0]).toBeInstanceOf(AbortSignal);
    expect(state.home.value?.source?.id).toBe("local");
    answer(view("2", [source("local"), source("2")]));
    await flush();
    expect(state.home.value?.source?.id).toBe("2");
    expect(state.home.value?.others.map((o) => o.id)).toEqual(["local"]);
    expect(state.view.value?.home).toBe("2");
  });

  it("页面可见时每 30 秒再取；隐藏时不取，切回来马上取", async () => {
    nas.get.mockResolvedValue(view("local", [source("local")]));
    harness();
    await flush();
    expect(nas.get).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(NAS_POLL_MS - NAS_TICK_MS);
    expect(nas.get, "没到 30 秒不取").toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(NAS_TICK_MS);
    expect(nas.get).toHaveBeenCalledTimes(2);
    await flush();

    setHidden(true);
    vi.advanceTimersByTime(NAS_POLL_MS * 3);
    expect(nas.get, "隐藏的标签页不取").toHaveBeenCalledTimes(2);
    setHidden(false);
    expect(nas.get, "切回来马上取").toHaveBeenCalledTimes(3);
  });

  it("「刷新」马上取；now 跟着走，「N 秒前更新」会变", async () => {
    nas.get.mockResolvedValue(view("local", [source("local")]));
    const { state, refresh } = harness();
    await flush();
    const t0 = state.now.value.getTime();
    vi.advanceTimersByTime(NAS_TICK_MS);
    expect(state.now.value.getTime() - t0).toBeGreaterThanOrEqual(NAS_TICK_MS);
    refresh.value += 1;
    await flush();
    expect(nas.get).toHaveBeenCalledTimes(2);
  });

  it("一次没取到：留着上一次的数据，原因记在 error；两边都没有数据时 failure 给原因", async () => {
    nas.get
      .mockResolvedValueOnce(view("local", [source("local")]))
      .mockRejectedValueOnce(new Error("NAS 监控没有启动\n"));
    const { state, refresh } = harness({ data: null, error: "NAS 监控没有启动" });
    await flush();
    expect(state.home.value?.source?.id).toBe("local");
    refresh.value += 1;
    await flush();
    expect(state.home.value?.source?.id, "留着上一次的").toBe("local");
    expect(state.error.value).toBe("NAS 监控没有启动");
    expect(state.failure.value, "有数据时不报").toBe("");
  });

  it("两边都没有数据时 failure 先用首页接口的原因，再用 GET /api/nas 的", async () => {
    nas.get.mockRejectedValue(new Error(""));
    const { state, blockRef } = harness({ data: null, error: "15 秒内没取到" });
    await flush();
    expect(state.failure.value).toBe("15 秒内没取到");
    blockRef.value = null;
    await flush();
    expect(state.failure.value, "空的错误写默认的一句").toBe("读取 NAS 监控失败");
  });

  it("慢的旧请求晚到时不盖掉新的", async () => {
    let first!: (v: NasView) => void;
    nas.get
      .mockReturnValueOnce(new Promise<NasView>((r) => (first = r)))
      .mockResolvedValueOnce(view("2", [source("2")]));
    const { state, refresh } = harness();
    refresh.value += 1;
    await flush();
    expect(state.home.value?.source?.id).toBe("2");
    first(view("local", [source("local")]));
    await flush();
    expect(state.home.value?.source?.id).toBe("2");
  });

  it("有来源还在「正在连」时 10 秒再取，连上以后回到 30 秒", async () => {
    nas.get
      .mockResolvedValueOnce(view("2", [source("2", { state: "connecting" })]))
      .mockResolvedValue(view("2", [source("2")]));
    harness();
    await flush();
    vi.advanceTimersByTime(NAS_TICK_MS);
    expect(nas.get, "正在连：10 秒就再取").toHaveBeenCalledTimes(2);
    await flush();
    vi.advanceTimersByTime(NAS_TICK_MS * 2);
    expect(nas.get, "连上了：不到 30 秒不取").toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(NAS_TICK_MS);
    expect(nas.get).toHaveBeenCalledTimes(3);
  });

  it("apply：换成别处取到的一份，比它早发出、晚到的那次重取不再盖掉它", async () => {
    let slow!: (v: NasView) => void;
    nas.get
      .mockReturnValueOnce(new Promise<NasView>((r) => (slow = r)))
      .mockResolvedValue(view("local", [source("local")]));
    const { state } = harness();
    vi.advanceTimersByTime(NAS_POLL_MS - NAS_TICK_MS);
    state.apply(view("2", [source("2")]));
    expect(state.view.value?.home).toBe("2");
    slow(view("local", [source("local")]));
    await flush();
    expect(state.view.value?.home, "晚到的旧回应不盖掉").toBe("2");
    vi.advanceTimersByTime(NAS_TICK_MS);
    expect(nas.get, "apply 也算刚取过：离挂载 30 秒了也不取").toHaveBeenCalledTimes(1);
  });

  it("卸载以后不再取，晚到的回应也不再写", async () => {
    let answer!: (v: NasView) => void;
    nas.get.mockReturnValueOnce(new Promise<NasView>((r) => (answer = r)));
    const { state } = harness();
    unmount?.();
    unmount = null;
    answer(view("local", [source("local")]));
    await flush();
    expect(state.view.value).toBeNull();
    vi.advanceTimersByTime(NAS_POLL_MS * 2);
    setHidden(false);
    expect(nas.get).toHaveBeenCalledTimes(1);
  });
});
