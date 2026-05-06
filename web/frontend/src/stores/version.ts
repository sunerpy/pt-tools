import { ElNotification } from "element-plus";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
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
      upgrading.value = false;
      upgradeProgress.value = null;
    } catch (error) {
      console.error("Failed to cancel upgrade:", error);
    }
  }

  let progressPollTimer: number | null = null;

  function pollUpgradeProgress() {
    if (progressPollTimer) {
      clearInterval(progressPollTimer);
    }

    progressPollTimer = window.setInterval(async () => {
      try {
        upgradeProgress.value = await versionApi.getUpgradeProgress();

        if (
          upgradeProgress.value.status === "completed" ||
          upgradeProgress.value.status === "failed" ||
          upgradeProgress.value.status === "idle"
        ) {
          if (progressPollTimer) {
            clearInterval(progressPollTimer);
            progressPollTimer = null;
          }
          upgrading.value = false;

          if (upgradeProgress.value.status === "completed") {
            ElNotification({
              title: "升级完成",
              message: "请重启应用以使用新版本",
              type: "success",
              duration: 0,
            });
          } else if (upgradeProgress.value.status === "failed") {
            ElNotification({
              title: "升级失败",
              message: upgradeProgress.value.error || "未知错误",
              type: "error",
              duration: 0,
            });
          }
        }
      } catch (error) {
        console.error("Failed to poll upgrade progress:", error);
      }
    }, 1000);
  }

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
