import { ElNotification } from "element-plus";
import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { type ReleaseInfo, versionApi, type VersionCheckResult, type VersionInfo } from "../api";

const DISMISSED_VERSIONS_KEY = "pt-tools-dismissed-versions";

export const useVersionStore = defineStore("version", () => {
  const versionInfo = ref<VersionInfo | null>(null);
  const checkResult = ref<VersionCheckResult | null>(null);
  const loading = ref(false);
  const checking = ref(false);
  const dismissedVersions = ref<string[]>(loadDismissedVersions());

  const currentVersion = computed(() => versionInfo.value?.version || "unknown");

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

      checkResult.value = await versionApi.checkUpdate(options);

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
    fetchVersionInfo,
    checkForUpdates,
    dismissVersion,
    dismissAllVisible,
    clearDismissed,
  };
});
