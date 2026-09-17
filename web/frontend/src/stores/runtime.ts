import { ElMessage, ElMessageBox } from "element-plus";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { controlApi, downloaderTorrentsApi, pausedTorrentsApi, sitesApi, tasksApi } from "../api";

/**
 * 底部状态栏的数据源（设计稿 G 的 `statusbar`：调度器 / 最后同步 / ↓ / ↑ / 剩余空间）。
 *
 * 只展示真实拿到的数字：/api/downloader-torrents/transfer-stats 是唯一能给出
 * 全局速率与剩余空间的接口，拿不到就显示 "—"，不猜、不填占位数。
 *
 * 调度器没有查询接口（controlApi 只有 stop/start），所以状态栏里的调度器格子
 * 显示的是「本次会话里最后一次操作的结果」而不是真实运行态，未操作时为未知。
 *
 * 顺带承担导航列的三个计数徽标（站点 / 任务 / 暂停任务）。都取 page_size=1
 * 的 total，请求很小；跟速率同一个轮询周期刷新，省得每个页面各自去数一遍。
 */
export type SchedulerHint = "unknown" | "running" | "stopped";

const POLL_MS = 30_000;

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
  const schedulerHint = ref<SchedulerHint>("unknown");
  const stopLoading = ref(false);
  const startLoading = ref(false);

  /** 导航徽标计数。null = 还没拿到，导航列就不画徽标 */
  const siteCount = ref<number | null>(null);
  const taskCount = ref<number | null>(null);
  const pausedCount = ref<number | null>(null);

  let timer: number | null = null;

  const downloadText = computed(() => fmtSpeed(downloadSpeed.value));
  const uploadText = computed(() => fmtSpeed(uploadSpeed.value));
  const freeSpaceText = computed(() => fmtSize(freeSpace.value));
  const lastSyncText = computed(() => fmtClock(lastSync.value));
  const pollMinutes = computed(() => Math.round(POLL_MS / 60_000) || 1);

  const schedulerText = computed(() => {
    if (schedulerHint.value === "running") return "调度器已启动";
    if (schedulerHint.value === "stopped") return "调度器已停止";
    return "调度器";
  });

  const badges = computed(() => ({
    sites: siteCount.value,
    tasks: taskCount.value,
    paused: pausedCount.value,
  }));

  async function refresh() {
    // 四个请求互不依赖，任何一个失败都不该拖掉其他的，所以用 allSettled 并发
    const [stats, sites, tasks, paused] = await Promise.allSettled([
      downloaderTorrentsApi.transferStats(),
      sitesApi.list(),
      tasksApi.list(new URLSearchParams({ page: "1", page_size: "1" })),
      pausedTorrentsApi.list(1, 1),
    ]);

    if (stats.status === "fulfilled") {
      downloadSpeed.value = stats.value.total_download_speed;
      uploadSpeed.value = stats.value.total_upload_speed;
      freeSpace.value = stats.value.total_free_space;
      lastSync.value = new Date();
      stale.value = false;
    } else {
      // 没配下载器、下载器离线都会走到这里。状态栏是背景信息，不弹提示。
      stale.value = true;
    }

    if (sites.status === "fulfilled") siteCount.value = Object.keys(sites.value ?? {}).length;
    if (tasks.status === "fulfilled") taskCount.value = tasks.value.total;
    if (paused.status === "fulfilled") pausedCount.value = paused.value.total;
  }

  /** 只在页面可见时轮询：隐藏的标签页拉速率没有意义，还会白占下载器的连接 */
  function startPolling() {
    if (timer !== null) return;
    void refresh();
    timer = window.setInterval(() => {
      if (!document.hidden) void refresh();
    }, POLL_MS);
    document.addEventListener("visibilitychange", onVisible);
  }

  function stopPolling() {
    if (timer !== null) {
      window.clearInterval(timer);
      timer = null;
    }
    document.removeEventListener("visibilitychange", onVisible);
  }

  function onVisible() {
    // 切回来立刻补一次，否则要等满一个轮询周期才更新
    if (!document.hidden) void refresh();
  }

  /**
   * 文案、确认框类型和错误提示都保持旧顶栏的原样（"确认" / warning / info），
   * 只是把 confirm 的取消从 catch 里分出来 —— 旧写法靠 `!== "cancel"` 过滤，
   * 真实错误里带 "cancel" 字样时会被吞掉。
   */
  async function confirmAnd(
    text: string,
    type: "warning" | "info",
    loading: typeof stopLoading,
    call: () => Promise<void>,
    okMessage: string,
    hint: SchedulerHint,
  ) {
    try {
      await ElMessageBox.confirm(text, "确认", {
        confirmButtonText: "确定",
        cancelButtonText: "取消",
        type,
      });
    } catch {
      return; // 用户取消
    }
    loading.value = true;
    try {
      await call();
      schedulerHint.value = hint;
      ElMessage.success(okMessage);
      void refresh();
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || "操作失败");
    } finally {
      loading.value = false;
    }
  }

  function stopAll() {
    return confirmAnd(
      "确定要停止所有任务吗？",
      "warning",
      stopLoading,
      () => controlApi.stop(),
      "已停止所有任务",
      "stopped",
    );
  }

  function startAll() {
    return confirmAnd(
      "确定要启动所有任务吗？",
      "info",
      startLoading,
      () => controlApi.start(),
      "已启动所有任务",
      "running",
    );
  }

  return {
    downloadSpeed,
    uploadSpeed,
    freeSpace,
    lastSync,
    stale,
    schedulerHint,
    stopLoading,
    startLoading,
    siteCount,
    taskCount,
    pausedCount,
    badges,
    downloadText,
    uploadText,
    freeSpaceText,
    lastSyncText,
    pollMinutes,
    schedulerText,
    refresh,
    startPolling,
    stopPolling,
    stopAll,
    startAll,
  };
});
