import { ElMessage, ElMessageBox } from "element-plus";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import {
  controlApi,
  downloaderTorrentsApi,
  pausedTorrentsApi,
  type RssTaskStatus,
  sitesApi,
  tasksApi,
} from "../api";
import { rssTaskView } from "../utils/rssTask";

/**
 * 底部状态栏的数据源（设计稿 G 的 `statusbar`：RSS 任务 / 最后同步 / ↓ / ↑ / 剩余空间）。
 *
 * 只展示真实拿到的数字：/api/downloader-torrents/transfer-stats 是唯一能给出
 * 全局速率与剩余空间的接口，拿不到就显示 "—"，不猜、不填占位数。
 *
 * RSS 任务（原「调度器」）的格子按 /api/control/status 的真实状态显示：在跑几个订阅，
 * 或者为什么没在跑（没开自动启动、手动停止、下载器连不上……）。跟速率同一个轮询周期刷新，
 * 启停、切换自动启动以后再跟查一阵（见 followRss）。
 *
 * 顺带承担导航列的三个计数徽标（站点 / 任务 / 暂停任务）。都取 page_size=1
 * 的 total，请求很小；跟速率同一个轮询周期刷新，省得每个页面各自去数一遍。
 */
const POLL_MS = 30_000;

/**
 * 启停、切换自动启动以后，RSS 任务处在「启动中」时每隔多久再查一次、最多查几次（约 30 秒）。
 * 重载是异步的：配置变更先去抖 200ms，连默认下载器可能还要等几十秒。
 */
const RSS_FOLLOW_MS = 2_000;
const RSS_FOLLOW_MAX = 15;

/**
 * 单轮请求的时间预算。
 *
 * 浏览器对同一来源只保持 6 个并发 HTTP/1.1 连接。下载器离线时
 * transfer-stats 可能长时间不返回，几拍下来就能把连接池占满 ——
 * 之后所有页面的请求都在排队，表现是「整个界面点哪都没数据」。
 * 所以到点就 abort，把连接槽还回去。
 */
const REQUEST_TIMEOUT_MS = 8_000;

/** transfer-stats 连续失败后最多跳过多少拍（30s/拍） */
const MAX_STATS_SKIP_TICKS = 8;

function fmtSpeed(bytesPerSec: number | null): string {
  if (bytesPerSec === null || !Number.isFinite(bytesPerSec) || bytesPerSec < 0) return "—";
  if (bytesPerSec < 1024) return `${Math.round(bytesPerSec)} B/s`;
  const units = ["KB/s", "MB/s", "GB/s"];
  let v = bytesPerSec / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v >= 10 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

function fmtSize(bytes: number | null): string {
  if (bytes === null || !Number.isFinite(bytes) || bytes < 0) return "—";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`;
}

function fmtClock(d: Date | null): string {
  if (!d) return "—";
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

export const useRuntimeStore = defineStore("runtime", () => {
  const downloadSpeed = ref<number | null>(null);
  const uploadSpeed = ref<number | null>(null);
  const freeSpace = ref<number | null>(null);
  const lastSync = ref<Date | null>(null);
  /** 拉取失败时把上一次的数字保留着，但标记为陈旧，状态栏据此变灰 */
  const stale = ref(true);
  /** RSS 任务的真实状态；null = 还没取到 */
  const rss = ref<RssTaskStatus | null>(null);
  const stopLoading = ref(false);
  const startLoading = ref(false);
  const autoStartLoading = ref(false);

  /** 导航徽标计数。null = 还没拿到，导航列就不画徽标 */
  const siteCount = ref<number | null>(null);
  const taskCount = ref<number | null>(null);
  const pausedCount = ref<number | null>(null);

  let timer: number | null = null;
  /** 上一轮还没回来时不再发起新的一轮，否则慢请求会累积 */
  let inFlight = false;
  let statsFailures = 0;
  let statsSkipTicks = 0;
  /** 操作以后的跟查：新的一次跟查开始时旧的作废（代次不对就不再排下一拍） */
  let rssFollowTimer: number | null = null;
  let rssFollowGen = 0;

  const downloadText = computed(() => fmtSpeed(downloadSpeed.value));
  const uploadText = computed(() => fmtSpeed(uploadSpeed.value));
  const freeSpaceText = computed(() => fmtSize(freeSpace.value));
  const lastSyncText = computed(() => fmtClock(lastSync.value));
  const pollMinutes = computed(() => Math.round(POLL_MS / 60_000) || 1);

  const rssView = computed(() => rssTaskView(rss.value));

  const badges = computed(() => ({
    sites: siteCount.value,
    tasks: taskCount.value,
    paused: pausedCount.value,
  }));

  /**
   * 这一拍是否该拉 transfer-stats。
   *
   * 四个请求里只有它要向每台下载器要数据，也只有它会在下载器离线时变慢。
   * 连续失败后按指数退避跳过若干拍，另外三个只读数据库的请求照常刷新，
   * 导航徽标不会因为一台下载器掉线而停止更新。
   *
   * 退避只作用于定时轮询。用户主动触发的刷新（切回标签页、启停 RSS 任务、页面上点刷新）
   * 一律真刷，不让用户对着一个「还在退避中」的界面干等。
   */
  function shouldPollStats(polled: boolean): boolean {
    if (!polled) {
      statsSkipTicks = 0;
      return true;
    }
    if (statsSkipTicks <= 0) return true;
    statsSkipTicks -= 1;
    return false;
  }

  async function refresh(opts?: { polled?: boolean }) {
    if (inFlight) return;
    inFlight = true;
    try {
      // 一轮共用一个超时预算：到点全部 abort，连接槽立即释放
      const signal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
      // 五个请求互不依赖，任何一个失败都不该拖掉其他的，所以用 allSettled 并发
      const [stats, sites, tasks, paused, rssStatus] = await Promise.allSettled([
        shouldPollStats(opts?.polled === true)
          ? downloaderTorrentsApi.transferStats(signal)
          : Promise.resolve(null),
        sitesApi.list(signal),
        tasksApi.list(new URLSearchParams({ page: "1", page_size: "1" }), signal),
        pausedTorrentsApi.list(1, 1, undefined, signal),
        controlApi.status(signal),
      ]);

      if (stats.status === "fulfilled" && stats.value) {
        downloadSpeed.value = stats.value.total_download_speed;
        uploadSpeed.value = stats.value.total_upload_speed;
        freeSpace.value = stats.value.total_free_space;
        lastSync.value = new Date();
        stale.value = false;
        statsFailures = 0;
        statsSkipTicks = 0;
      } else if (stats.status === "rejected") {
        // 没配下载器、下载器离线、超时都会走到这里。状态栏是背景信息，不弹提示。
        stale.value = true;
        statsFailures += 1;
        statsSkipTicks = Math.min(2 ** (statsFailures - 1), MAX_STATS_SKIP_TICKS);
      }

      if (sites.status === "fulfilled") siteCount.value = Object.keys(sites.value ?? {}).length;
      if (tasks.status === "fulfilled") taskCount.value = tasks.value.total;
      if (paused.status === "fulfilled") pausedCount.value = paused.value.total;
      // 查不到就保留上一次的结果：状态栏是背景信息，不弹提示
      if (rssStatus.status === "fulfilled") rss.value = rssStatus.value;
    } finally {
      inFlight = false;
    }
  }

  async function refreshRss() {
    try {
      rss.value = await controlApi.status(AbortSignal.timeout(REQUEST_TIMEOUT_MS));
    } catch {
      // 同上：保留上一次的结果
    }
  }

  /**
   * 操作以后跟查 RSS 任务的状态：先查一次，还在「启动中」就每 RSS_FOLLOW_MS 再查，
   * 最多 RSS_FOLLOW_MAX 次。新的跟查开始（或停止轮询）时旧的作废。
   */
  function followRss() {
    const gen = ++rssFollowGen;
    if (rssFollowTimer !== null) {
      window.clearTimeout(rssFollowTimer);
      rssFollowTimer = null;
    }
    let left = RSS_FOLLOW_MAX;
    const tick = async () => {
      rssFollowTimer = null;
      await refreshRss();
      left -= 1;
      if (gen === rssFollowGen && rss.value?.state === "starting" && left > 0) {
        rssFollowTimer = window.setTimeout(() => void tick(), RSS_FOLLOW_MS);
      }
    };
    void tick();
  }

  /** 只在页面可见时轮询：隐藏的标签页拉速率没有意义，还会白占下载器的连接 */
  function startPolling() {
    if (timer !== null) return;
    void refresh();
    timer = window.setInterval(() => {
      if (!document.hidden) void refresh({ polled: true });
    }, POLL_MS);
    document.addEventListener("visibilitychange", onVisible);
  }

  function stopPolling() {
    if (timer !== null) {
      window.clearInterval(timer);
      timer = null;
    }
    rssFollowGen += 1;
    if (rssFollowTimer !== null) {
      window.clearTimeout(rssFollowTimer);
      rssFollowTimer = null;
    }
    document.removeEventListener("visibilitychange", onVisible);
  }

  function onVisible() {
    // 切回来立刻补一次，否则要等满一个轮询周期才更新
    if (!document.hidden) void refresh();
  }

  /**
   * 启停 RSS 任务。confirm 为 null 时不弹确认框（页面顶上的提示条点「启动」就是要启动）。
   * confirm 的取消从 catch 里分出来 —— 旧写法靠 `!== "cancel"` 过滤，
   * 真实错误里带 "cancel" 字样时会被吞掉。
   */
  async function runControl(opts: {
    confirm: { text: string; type: "warning" | "info" } | null;
    loading: typeof stopLoading;
    call: () => Promise<void>;
    okMessage: string;
  }) {
    if (opts.confirm) {
      try {
        await ElMessageBox.confirm(opts.confirm.text, "确认", {
          confirmButtonText: "确定",
          cancelButtonText: "取消",
          type: opts.confirm.type,
        });
      } catch {
        return; // 用户取消
      }
    }
    opts.loading.value = true;
    try {
      await opts.call();
      ElMessage.success(opts.okMessage);
      followRss();
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || "操作失败");
    } finally {
      opts.loading.value = false;
    }
  }

  function stopAll() {
    return runControl({
      confirm: {
        text: "停止后 RSS 订阅不再拉取新种子，直到再次启动；刷流、辅种、自动删种等不受影响。确定停止 RSS 任务吗？",
        type: "warning",
      },
      loading: stopLoading,
      call: () => controlApi.stop(),
      okMessage: "已停止 RSS 任务",
    });
  }

  function startAll(opts: { confirm?: boolean } = {}) {
    return runControl({
      confirm:
        opts.confirm === false ? null : { text: "按当前配置启动 RSS 任务吗？", type: "info" },
      loading: startLoading,
      call: () => controlApi.start(),
      okMessage: "已启动 RSS 任务",
    });
  }

  /** 只切换「自动启动 RSS 任务」。打开时后端会像保存全局设置一样重载：没被手动停止的话 RSS 任务随即启动 */
  async function setAutoStart(enabled: boolean) {
    autoStartLoading.value = true;
    try {
      const next = await controlApi.setAutoStart(enabled);
      rss.value = next;
      if (!enabled) {
        ElMessage.success("已关闭自动启动：程序重启后要手动启动 RSS 任务");
      } else if (next.state === "paused") {
        ElMessage.success("已打开自动启动。RSS 任务现在是手动停止的，要运行请点「启动」");
      } else {
        ElMessage.success("已打开自动启动：以后程序启动时会自动运行 RSS 任务");
      }
      followRss();
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || "操作失败");
    } finally {
      autoStartLoading.value = false;
    }
  }

  return {
    downloadSpeed,
    uploadSpeed,
    freeSpace,
    lastSync,
    stale,
    rss,
    rssView,
    stopLoading,
    startLoading,
    autoStartLoading,
    siteCount,
    taskCount,
    pausedCount,
    badges,
    downloadText,
    uploadText,
    freeSpaceText,
    lastSyncText,
    pollMinutes,
    refresh,
    refreshRss,
    startPolling,
    stopPolling,
    stopAll,
    startAll,
    setAutoStart,
  };
});
