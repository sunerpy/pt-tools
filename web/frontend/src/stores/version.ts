import { ElNotification } from "element-plus";
import { defineStore } from "pinia";
import { computed, onScopeDispose, ref } from "vue";
import {
  type ReleaseInfo,
  versionApi,
  type VersionCheckResult,
  type VersionInfo,
  type RuntimeEnvironment,
  type UpgradeProgress,
} from "../api";

const DISMISSED_VERSIONS_KEY = "pt-tools-dismissed-versions";
const SHOW_PRERELEASE_KEY = "pt-tools-show-prerelease";

/**
 * 右下角的通知要让开底部那条 chrome：桌面是状态条，手机是底栏（状态行 + tab 行，再加安全区）。
 * Element 把 offset 当作离视口底边的距离，自己再加 16 的间距（默认 offset 0 就是 bottom: 16px）——
 * 之前没传 offset，「发现新版本」（duration 0、不会自己消失）一直压在状态条右端的版本按钮上，
 * 恰好是看到提示后要点的那一格；手机上则压住底栏的状态行。
 * 按实际渲染出来的高度量，不抄令牌：底栏还有一截 env(safe-area-inset-bottom)，令牌里没有。
 * 两套外壳都常驻 DOM、由媒体查询决定显示哪套，所以挑 getClientRects 非空的那一个。
 */
function bottomNotifyOffset(): number {
  const chrome = [...document.querySelectorAll<HTMLElement>(".pt-status, .pt-mnav")].find(
    (el) => el.getClientRects().length > 0,
  );
  return chrome ? Math.ceil(chrome.getBoundingClientRect().height) : 0;
}

export const useVersionStore = defineStore("version", () => {
  const versionInfo = ref<VersionInfo | null>(null);
  const checkResult = ref<VersionCheckResult | null>(null);
  const loading = ref(false);
  const checking = ref(false);
  const dismissedVersions = ref<string[]>(loadDismissedVersions());
  const showPrerelease = ref<boolean>(loadShowPrerelease());

  const runtime = ref<RuntimeEnvironment | null>(null);
  const upgradeProgress = ref<UpgradeProgress | null>(null);
  const upgrading = ref(false);

  const currentVersion = computed(() => versionInfo.value?.version || "unknown");

  const canSelfUpgrade = computed(() => runtime.value?.can_self_upgrade === true);
  const isDocker = computed(() => runtime.value?.is_docker === true);

  const hasUpdate = computed(() => {
    if (!checkResult.value?.has_update || !checkResult.value.new_releases) return false;
    return checkResult.value.new_releases.some((r) => !dismissedVersions.value.includes(r.version));
  });

  const hasNewRelease = computed(() => {
    return (
      checkResult.value?.has_update &&
      checkResult.value.new_releases &&
      checkResult.value.new_releases.length > 0
    );
  });

  const latestVersion = computed((): string | null => {
    const releases = checkResult.value?.new_releases;
    if (!releases || releases.length === 0) return null;
    const first = releases[0];
    return first ? first.version : null;
  });

  const allDismissed = computed(() => {
    return hasNewRelease.value && !hasUpdate.value;
  });

  const visibleReleases = computed<ReleaseInfo[]>(() => {
    if (!checkResult.value?.new_releases) return [];
    return checkResult.value.new_releases.filter(
      (r) => !dismissedVersions.value.includes(r.version),
    );
  });

  const hasMoreReleases = computed(() => checkResult.value?.has_more_releases || false);
  const changelogUrl = computed(() => checkResult.value?.changelog_url || "");

  const hasPrereleaseUpdate = computed(() =>
    visibleReleases.value.some((r) => r.prerelease === true),
  );
  const onlyPrereleaseUpdates = computed(
    () =>
      visibleReleases.value.length > 0 && visibleReleases.value.every((r) => r.prerelease === true),
  );

  function loadDismissedVersions(): string[] {
    try {
      const stored = localStorage.getItem(DISMISSED_VERSIONS_KEY);
      return stored ? JSON.parse(stored) : [];
    } catch {
      return [];
    }
  }

  function saveDismissedVersions() {
    localStorage.setItem(DISMISSED_VERSIONS_KEY, JSON.stringify(dismissedVersions.value));
  }

  function loadShowPrerelease(): boolean {
    try {
      return localStorage.getItem(SHOW_PRERELEASE_KEY) === "true";
    } catch {
      return false;
    }
  }

  function saveShowPrerelease() {
    localStorage.setItem(SHOW_PRERELEASE_KEY, showPrerelease.value ? "true" : "false");
  }

  function setShowPrerelease(value: boolean) {
    if (showPrerelease.value === value) return;
    showPrerelease.value = value;
    saveShowPrerelease();
    checkForUpdates({ force: true }, false);
  }

  async function fetchVersionInfo() {
    try {
      loading.value = true;
      versionInfo.value = await versionApi.getInfo();
    } catch (error) {
      console.error("Failed to fetch version info:", error);
    } finally {
      loading.value = false;
    }
  }

  async function fetchRuntime() {
    try {
      const result = await versionApi.getRuntime();
      runtime.value = result.runtime;
      upgradeProgress.value = result.upgrade_progress;
    } catch (error) {
      console.error("Failed to fetch runtime info:", error);
    }
  }

  async function startUpgrade(version: string, proxyUrl?: string) {
    try {
      upgrading.value = true;
      await versionApi.startUpgrade(version, proxyUrl);
      pollUpgradeProgress();
    } catch (error) {
      upgrading.value = false;
      throw error;
    }
  }

  async function cancelUpgrade() {
    try {
      await versionApi.cancelUpgrade();
      stopUpgradePolling();
      upgrading.value = false;
      upgradeProgress.value = null;
    } catch (error) {
      console.error("Failed to cancel upgrade:", error);
    }
  }

  /*
   * 升级进度串行轮询：一次请求回来才排下一次，慢请求不会叠起来。
   * 失败时按 2、4、8… 秒退避，最长 30 秒；连续失败 POLL_MAX_FAILURES 次（约 8 分钟）就停下并告知，
   * 而不是一直每秒请求一个已经 401 / 500 的接口。服务端拒绝并发升级，停下后复位 upgrading 不会重复开升级。
   */
  const POLL_INTERVAL_MS = 1000;
  const POLL_MAX_BACKOFF_MS = 30_000;
  const POLL_MAX_FAILURES = 20;

  let progressPollTimer: number | null = null;
  /** 每次开始或停止轮询换一代：上一代在途的请求回来后不再写状态、也不再排下一次 */
  let pollGeneration = 0;

  function stopUpgradePolling() {
    pollGeneration++;
    if (progressPollTimer !== null) {
      clearTimeout(progressPollTimer);
      progressPollTimer = null;
    }
  }

  function pollUpgradeProgress() {
    stopUpgradePolling();
    const generation = pollGeneration;
    let failures = 0;

    const schedule = (delay: number) => {
      progressPollTimer = window.setTimeout(() => void tick(), delay);
    };

    const tick = async () => {
      progressPollTimer = null;
      let progress: UpgradeProgress;
      try {
        progress = await versionApi.getUpgradeProgress();
      } catch (error) {
        if (generation !== pollGeneration) return;
        failures++;
        console.error("Failed to poll upgrade progress:", error);
        if (failures >= POLL_MAX_FAILURES) {
          upgrading.value = false;
          ElNotification({
            title: "无法获取升级进度",
            message: `${error instanceof Error ? error.message : "请求失败"}；刷新页面后可查看升级状态`,
            type: "warning",
            duration: 0,
          });
          return;
        }
        schedule(Math.min(POLL_INTERVAL_MS * 2 ** failures, POLL_MAX_BACKOFF_MS));
        return;
      }
      if (generation !== pollGeneration) return;
      failures = 0;
      upgradeProgress.value = progress;

      if (
        progress.status === "completed" ||
        progress.status === "failed" ||
        progress.status === "idle"
      ) {
        upgrading.value = false;
        if (progress.status === "completed") {
          ElNotification({
            title: "升级完成",
            message: "请重启应用以使用新版本",
            type: "success",
            duration: 0,
          });
        } else if (progress.status === "failed") {
          ElNotification({
            title: "升级失败",
            message: progress.error || "未知错误",
            type: "error",
            duration: 0,
          });
        }
        return;
      }
      schedule(POLL_INTERVAL_MS);
    };

    schedule(POLL_INTERVAL_MS);
  }

  // store 被销毁（$dispose）时不留定时器
  onScopeDispose(stopUpgradePolling);

  async function checkForUpdates(
    options?: { force?: boolean; proxy?: string },
    showNotification = true,
  ) {
    try {
      checking.value = true;

      if (options?.force) {
        dismissedVersions.value = [];
        saveDismissedVersions();
      }

      checkResult.value = await versionApi.checkUpdate({
        ...options,
        includePrerelease: showPrerelease.value,
      });

      if (showNotification && hasUpdate.value && visibleReleases.value.length > 0) {
        const latestRelease = visibleReleases.value[0];
        if (latestRelease) {
          ElNotification({
            title: "发现新版本",
            message: `pt-tools ${latestRelease.version} 已发布`,
            type: "info",
            duration: 0,
            position: "bottom-right",
            offset: bottomNotifyOffset(),
          });
        }
      }
    } catch (error) {
      console.error("Failed to check for updates:", error);
      if (showNotification) {
        ElNotification({
          title: "版本检查失败",
          message: error instanceof Error ? error.message : "请检查网络连接或配置代理",
          type: "warning",
          duration: 5000,
          position: "bottom-right",
          offset: bottomNotifyOffset(),
        });
      }
    } finally {
      checking.value = false;
    }
  }

  function dismissVersion(version: string) {
    if (!dismissedVersions.value.includes(version)) {
      dismissedVersions.value.push(version);
      saveDismissedVersions();
    }
  }

  function dismissAllVisible() {
    for (const release of visibleReleases.value) {
      if (!dismissedVersions.value.includes(release.version)) {
        dismissedVersions.value.push(release.version);
      }
    }
    saveDismissedVersions();
  }

  function clearDismissed() {
    dismissedVersions.value = [];
    saveDismissedVersions();
  }

  return {
    versionInfo,
    checkResult,
    loading,
    checking,
    currentVersion,
    hasUpdate,
    hasNewRelease,
    latestVersion,
    allDismissed,
    visibleReleases,
    hasMoreReleases,
    changelogUrl,
    hasPrereleaseUpdate,
    onlyPrereleaseUpdates,
    showPrerelease,
    runtime,
    upgradeProgress,
    upgrading,
    canSelfUpgrade,
    isDocker,
    fetchVersionInfo,
    checkForUpdates,
    dismissVersion,
    dismissAllVisible,
    clearDismissed,
    setShowPrerelease,
    fetchRuntime,
    startUpgrade,
    cancelUpgrade,
  };
});
