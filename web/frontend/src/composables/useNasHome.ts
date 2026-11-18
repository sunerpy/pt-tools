/*
 * NAS 监控的数据（首页 NAS 概览 H2）。GET /api/nas 只读 pt-tools 的内存（采集器 30 秒一拍），不对外发请求。
 *
 * useNasView：挂载后马上取一次，之后页面可见时每 30 秒再取（有来源还在「正在连」时 10 秒，连上以后马上看得到），
 * 隐藏时不取；切回标签页、refreshKey 变了时马上取。一次没取到时留着上一次的数据，原因记在 error 里。
 * now 每 10 秒走一次，「N 秒前更新」跟着它变。「NAS 监控」页也用它。
 *
 * useNasHome：首页的 NAS 概览。先用 /api/home 的 nas 块（首页那一次请求就带着），取到 GET /api/nas 以后用它。
 */
import { type HomeBlock, type HomeNas, type NasView, nasApi } from "@/api";
import { nasUsable } from "@/utils/nas";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

/** 重取 GET /api/nas 的间隔（同采集器的一拍） */
export const NAS_POLL_MS = 30_000;
/** 「N 秒前更新」走字的间隔；有来源还在「正在连」时也按它重取 */
export const NAS_TICK_MS = 10_000;
/** 一次请求的时限：只读内存，到点还没回就放弃，别占着浏览器的连接槽 */
const NAS_TIMEOUT_MS = 10_000;
/** 计时器可能比约好的早一点点到：差这么多以内算到点了 */
const NAS_SLACK_MS = 1_000;

/** 从 GET /api/nas 算首页的 nas 块（同服务端的 homeNASBlock：首页显示的那个来源与其余能用的来源） */
export function homeNasOf(view: NasView): HomeNas {
  return {
    source: view.sources.find((s) => s.id === view.home) ?? null,
    others: view.sources
      .filter((s) => s.id !== view.home && nasUsable(s))
      .map((s) => ({ id: s.id, name: s.name, kind: s.kind, state: s.state })),
  };
}

function messageOf(e: unknown): string {
  return ((e as Error)?.message ?? "").trim() || "读取 NAS 监控失败";
}

export function useNasView(refreshKey?: () => unknown) {
  const view = ref<NasView | null>(null);
  const error = ref("");
  const now = ref(new Date());
  let seq = 0;
  let last = 0;
  let timer: number | null = null;
  let alive = true;

  async function load() {
    const n = ++seq;
    last = Date.now();
    try {
      const got = await nasApi.get(AbortSignal.timeout(NAS_TIMEOUT_MS));
      if (!alive || n !== seq) return;
      view.value = got;
      error.value = "";
    } catch (e) {
      if (!alive || n !== seq) return;
      error.value = messageOf(e);
    } finally {
      if (alive && n === seq) now.value = new Date();
    }
  }

  /** 换成别处取到的一份（例如改设置的回应就是一整份）：比它早发出、晚到的那次重取不再盖掉它 */
  function apply(got: NasView) {
    seq += 1;
    last = Date.now();
    view.value = got;
    error.value = "";
    now.value = new Date();
  }

  /** 下次重取的间隔：有来源还在「正在连」时 10 秒，否则 30 秒 */
  function interval(): number {
    return view.value?.sources.some((s) => s.state === "connecting") ? NAS_TICK_MS : NAS_POLL_MS;
  }

  function tick() {
    if (document.hidden) return;
    now.value = new Date();
    if (Date.now() - last >= interval() - NAS_SLACK_MS) void load();
  }

  function onVisible() {
    if (document.hidden) return;
    now.value = new Date();
    void load();
  }

  onMounted(() => {
    void load();
    timer = window.setInterval(tick, NAS_TICK_MS);
    document.addEventListener("visibilitychange", onVisible);
  });

  onBeforeUnmount(() => {
    alive = false;
    if (timer !== null) window.clearInterval(timer);
    timer = null;
    document.removeEventListener("visibilitychange", onVisible);
  });

  // 页面的「刷新」：首页连同 /api/home 一起重取
  if (refreshKey) watch(refreshKey, () => void load());

  return { view, error, now, reload: load, apply };
}

export function useNasHome(block: () => HomeBlock<HomeNas> | null, refreshKey: () => unknown) {
  const nas = useNasView(refreshKey);

  /** 首页显示的那个来源与其余来源：取到过 GET /api/nas 就用它（更新），否则用首页接口带的 */
  const home = computed<HomeNas | null>(() =>
    nas.view.value ? homeNasOf(nas.view.value) : (block()?.data ?? null),
  );
  /** 两边都没有数据时的原因（有数据时为空：照常显示上一次的） */
  const failure = computed(() => {
    if (nas.view.value || block()?.data) return "";
    return block()?.error || nas.error.value;
  });

  return { ...nas, home, failure };
}
