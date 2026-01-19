import { type SiteLevelRequirement, siteLevelsApi, type SiteLevelsResponse } from "@/api";
import { defineStore } from "pinia";
import { ref } from "vue";

export const useSiteLevelsStore = defineStore("siteLevels", () => {
  // 缓存所有站点的等级信息
  const levelsCache = ref<Record<string, SiteLevelsResponse>>({});
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref<string | null>(null);

  // 加载所有站点的等级信息
  async function loadAll() {
    if (loaded.value || loading.value) return;

    loading.value = true;
    error.value = null;

    try {
      const response = await siteLevelsApi.getAll();
      levelsCache.value = response.sites || {};
      loaded.value = true;
    } catch (e) {
      error.value = (e as Error).message || "加载等级信息失败";
    } finally {
      loading.value = false;
    }
  }

  // 获取指定站点的等级信息
  function getLevels(siteId: string): SiteLevelRequirement[] {
    const siteLower = siteId.toLowerCase();
    return levelsCache.value[siteLower]?.levels || [];
  }

  // 获取指定站点的名称
  function getSiteName(siteId: string): string {
    const siteLower = siteId.toLowerCase();
    return levelsCache.value[siteLower]?.siteName || siteId;
  }

  // 检查是否有指定站点的等级信息
  function hasLevels(siteId: string): boolean {
    const siteLower = siteId.toLowerCase();
    return !!levelsCache.value[siteLower]?.levels?.length;
  }

  // 重置缓存
  function reset() {
    levelsCache.value = {};
    loaded.value = false;
    error.value = null;
  }

  return {
    levelsCache,
    loading,
    loaded,
    error,
    loadAll,
    getLevels,
    getSiteName,
    hasLevels,
    reset,
  };
});
