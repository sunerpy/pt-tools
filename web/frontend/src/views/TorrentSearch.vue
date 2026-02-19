<script setup lang="ts">
import {
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  downloadersApi,
  type DownloaderSetting,
  type MultiSiteSearchRequest,
  searchApi,
  type SearchErrorItem,
  type SearchTorrentItem,
  siteCategoriesApi,
  type SiteCategoriesConfig,
  type SiteSearchParams,
  torrentPushApi,
  type TorrentPushItem,
} from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

// 搜索状态
const loading = ref(false);
const searchKeyword = ref("");
const selectedSites = ref<string[]>([]);
const availableSites = ref<string[]>([]);

// 搜索结果
const searchResults = ref<SearchTorrentItem[]>([]);
const siteResultCounts = ref<Record<string, number>>({});
const searchErrors = ref<SearchErrorItem[]>([]);
const searchTime = ref(0);
const totalResults = ref(0);

// 分页
const currentPage = ref(1);
const pageSize = ref(50);
const pageSizeOptions = [20, 50, 100];

// 选中的种子（用于批量操作）
const selectedTorrents = ref<SearchTorrentItem[]>([]);

// 下载器相关
const downloaders = ref<DownloaderSetting[]>([]);
const downloaderDirectories = ref<Record<number, DownloaderDirectory[]>>({});

// 推送对话框
const pushDialogVisible = ref(false);
const batchPushDialogVisible = ref(false);
const pushLoading = ref(false);
const currentPushTorrent = ref<SearchTorrentItem | null>(null);
const pushForm = ref({
  downloaderIds: [] as number[],
  savePath: "",
  category: "",
  tags: "",
  autoStart: true,
});

// 站点分类配置
const siteCategories = ref<Record<string, SiteCategoriesConfig>>({});
const selectedCategoryFilters = ref<Record<string, Record<string, string | number>>>({});

// 批量下载状态
const batchDownloading = ref(false);

// 缓存 key
const CACHE_KEY = "pt-tools-search-cache";

// 获取指定站点的分类配置
function getSiteCategoriesConfig(siteId: string): SiteCategoriesConfig | null {
  return siteCategories.value[siteId] || null;
}

// 判断站点是否有分类筛选
function siteHasCategories(siteId: string): boolean {
  const config = siteCategories.value[siteId];
  return config !== null && config !== undefined && config.categories.length > 0;
}

// 获取站点的已选筛选数量
function getSiteFilterCount(siteId: string): number {
  const filters = selectedCategoryFilters.value[siteId];
  if (!filters) return 0;
  return Object.values(filters).filter((v) => v !== "" && v !== undefined).length;
}

// 更新指定站点的分类筛选值
function updateSiteCategoryFilter(siteId: string, key: string, value: string | number | undefined) {
  if (!selectedCategoryFilters.value[siteId]) {
    selectedCategoryFilters.value[siteId] = {};
  }
  if (value === "" || value === undefined) {
    delete selectedCategoryFilters.value[siteId][key];
  } else {
    selectedCategoryFilters.value[siteId][key] = value;
  }
}

// 清除指定站点的分类筛选
function clearSiteCategoryFilters(siteId: string) {
  selectedCategoryFilters.value[siteId] = {};
}

// 构建 siteParams 用于搜索请求
function buildSiteParams(): Record<string, SiteSearchParams> | undefined {
  const result: Record<string, SiteSearchParams> = {};
  let hasParams = false;

  for (const [siteId, filters] of Object.entries(selectedCategoryFilters.value)) {
    const siteFilters: SiteSearchParams = {};
    for (const [key, value] of Object.entries(filters)) {
      if (value !== "" && value !== undefined) {
        siteFilters[key] = String(value);
        hasParams = true;
      }
    }
    if (Object.keys(siteFilters).length > 0) {
      result[siteId] = siteFilters;
    }
  }

  return hasParams ? result : undefined;
}

// 排序
const sortBy = ref<"sourceSite" | "publishTime" | "size" | "seeders" | "leechers" | "snatched">(
  "sourceSite",
);
const orderDesc = ref(false);

// 合并所有站点的种子列表（已排序）
const sortedResults = computed(() => {
  return [...searchResults.value].sort((a, b) => {
    let cmp = 0;
    switch (sortBy.value) {
      case "sourceSite":
        cmp = (a.sourceSite || "").localeCompare(b.sourceSite || "");
        break;
      case "publishTime":
        cmp = (a.uploadedAt || 0) - (b.uploadedAt || 0);
        break;
      case "size":
        cmp = a.sizeBytes - b.sizeBytes;
        break;
      case "seeders":
        cmp = a.seeders - b.seeders;
        break;
      case "leechers":
        cmp = a.leechers - b.leechers;
        break;
      case "snatched":
        cmp = a.snatched - b.snatched;
        break;
    }
    return orderDesc.value ? -cmp : cmp;
  });
});

// 当前页的种子列表
const pagedTorrents = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return sortedResults.value.slice(start, end);
});

// 获取默认下载器
const defaultDownloader = computed(() => {
  return downloaders.value.find((d) => d.is_default && d.enabled);
});

// 获取下载器目录选项
function getDirectoryOptions(downloaderId: number): DownloaderDirectory[] {
  return downloaderDirectories.value[downloaderId] || [];
}

// 保存搜索结果到缓存
function saveToCache() {
  const cacheData = {
    keyword: searchKeyword.value,
    results: searchResults.value,
    siteResultCounts: siteResultCounts.value,
    errors: searchErrors.value,
    searchTime: searchTime.value,
    totalResults: totalResults.value,
    selectedSites: selectedSites.value,
    categoryFilters: selectedCategoryFilters.value,
    timestamp: Date.now(),
  };
  try {
    sessionStorage.setItem(CACHE_KEY, JSON.stringify(cacheData));
  } catch {
    console.warn("Failed to save search cache");
  }
}

// 从缓存加载搜索结果
function loadFromCache() {
  try {
    const cached = sessionStorage.getItem(CACHE_KEY);
    if (!cached) return false;

    const data = JSON.parse(cached);
    // 检查缓存是否过期（5分钟）
    if (Date.now() - data.timestamp > 5 * 60 * 1000) {
      sessionStorage.removeItem(CACHE_KEY);
      return false;
    }

    searchKeyword.value = data.keyword || "";
    searchResults.value = data.results || [];
    siteResultCounts.value = data.siteResultCounts || {};
    searchErrors.value = data.errors || [];
    searchTime.value = data.searchTime || 0;
    totalResults.value = data.totalResults || 0;
    selectedSites.value = data.selectedSites || [];
    selectedCategoryFilters.value = data.categoryFilters || {};
    return true;
  } catch {
    return false;
  }
}

onMounted(async () => {
  await Promise.all([loadAvailableSites(), loadDownloaders(), loadSiteCategories()]);
  // 尝试从缓存加载
  loadFromCache();
});

async function loadAvailableSites() {
  try {
    availableSites.value = await searchApi.getSites();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载站点列表失败");
  }
}

async function loadDownloaders() {
  try {
    downloaders.value = await downloadersApi.list();
    if (defaultDownloader.value && pushForm.value.downloaderIds.length === 0) {
      pushForm.value.downloaderIds = [defaultDownloader.value.id!];
    }
    await loadAllDirectories();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载下载器列表失败");
  }
}

async function loadAllDirectories() {
  try {
    downloaderDirectories.value = await downloaderDirectoriesApi.listAll();
  } catch (e: unknown) {
    console.error("加载下载器目录失败:", e);
  }
}

async function loadSiteCategories() {
  try {
    siteCategories.value = await siteCategoriesApi.getAll();
  } catch (e: unknown) {
    console.error("加载站点分类配置失败:", e);
  }
}

async function doSearch() {
  if (!searchKeyword.value.trim()) {
    ElMessage.warning("请输入搜索关键词");
    return;
  }

  loading.value = true;
  selectedTorrents.value = [];
  currentPage.value = 1;

  try {
    await loadAvailableSites();
    const validSelected = selectedSites.value.filter((s) => availableSites.value.includes(s));
    selectedSites.value = validSelected;
    const sitesToSearch = validSelected.length > 0 ? validSelected : availableSites.value;
    const req: MultiSiteSearchRequest = {
      keyword: searchKeyword.value.trim(),
      sites: sitesToSearch,
      sortBy: sortBy.value,
      orderDesc: orderDesc.value,
      siteParams: buildSiteParams(),
      timeoutSecs: 30, // Set 30 second timeout for search
    };
    const resp = await searchApi.multiSite(req);
    searchResults.value = resp.items || [];
    siteResultCounts.value = resp.siteResults || {};
    searchErrors.value = resp.errors || [];
    searchTime.value = resp.durationMs;
    totalResults.value = resp.totalResults;

    // 保存到缓存
    saveToCache();

    if (searchErrors.value.length > 0) {
      const failedNames = searchErrors.value.map((e) => `${e.site}: ${e.error}`).join("\n");
      ElMessage.warning({
        message: `部分站点搜索失败:\n${failedNames}`,
        duration: 5000,
      });
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "搜索失败");
  } finally {
    loading.value = false;
  }
}

function handleSelectionChange(selection: SearchTorrentItem[]) {
  selectedTorrents.value = selection;
}

function handlePageChange(page: number) {
  currentPage.value = page;
}

function handleSizeChange(size: number) {
  pageSize.value = size;
  currentPage.value = 1;
}

// Handle table column sort change
function handleSortChange({ prop, order }: { prop: string | null; order: string | null }) {
  if (!prop || !order) {
    // Reset to default sort
    sortBy.value = "sourceSite";
    orderDesc.value = false;
    return;
  }

  // Map prop to sortBy value
  const propToSortBy: Record<string, typeof sortBy.value> = {
    sourceSite: "sourceSite",
    sizeBytes: "size",
    seeders: "seeders",
    leechers: "leechers",
    snatched: "snatched",
    uploadedAt: "publishTime",
  };

  if (propToSortBy[prop]) {
    sortBy.value = propToSortBy[prop];
    orderDesc.value = order === "descending";
  }
}

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return (bytes / Math.pow(1024, i)).toFixed(2) + " " + units[i];
}

function formatTime(timestamp?: number): string {
  if (!timestamp) return "-";
  try {
    return new Date(timestamp * 1000).toLocaleString("zh-CN");
  } catch {
    return "-";
  }
}

function getDiscountTag(torrent: SearchTorrentItem): {
  text: string;
  type: "success" | "warning" | "danger" | "info";
} {
  const level = (torrent.discountLevel || "").toUpperCase();

  // Handle specific discount levels
  switch (level) {
    case "2XFREE":
    case "_2X_FREE":
      return { text: "2xFree", type: "success" };
    case "FREE":
      return { text: "Free", type: "success" };
    case "PERCENT_50":
    case "50%":
      return { text: "50%", type: "warning" };
    case "PERCENT_30":
    case "30%":
      return { text: "30%", type: "warning" };
    case "PERCENT_70":
    case "70%":
      return { text: "70%", type: "warning" };
    case "2XUP":
    case "_2X_UP":
      return { text: "2xUp", type: "info" };
    case "2X50":
    case "_2X_PERCENT_50":
      return { text: "2x50%", type: "warning" };
    case "NONE":
    case "":
      return { text: "普通", type: "info" };
    default:
      // Fallback for unknown discount levels
      if (torrent.isFree) {
        return { text: "Free", type: "success" };
      }
      // Show the original discount level if not recognized
      if (level && level !== "NONE") {
        return { text: level, type: "warning" };
      }
      return { text: "普通", type: "info" };
  }
}

// 下载单个种子到本地
async function downloadTorrent(torrent: SearchTorrentItem) {
  if (!torrent.downloadUrl) {
    ElMessage.warning("该种子没有下载链接");
    return;
  }
  try {
    // Build download URL with title parameter for better filename
    let downloadUrl = torrent.downloadUrl;
    if (torrent.title) {
      // Add title parameter to the URL for filename generation
      const separator = downloadUrl.includes("?") ? "&" : "?";
      downloadUrl = `${downloadUrl}${separator}title=${encodeURIComponent(torrent.title)}`;
    }

    // Use fetch to download the file as blob to avoid browser security warnings
    const response = await fetch(downloadUrl);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }

    // Get filename from Content-Disposition header or use default
    const contentDisposition = response.headers.get("Content-Disposition");
    let filename = `${torrent.title}.torrent`;
    if (contentDisposition) {
      const filenameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/);
      if (filenameMatch && filenameMatch[1]) {
        filename = filenameMatch[1].replace(/['"]/g, "");
        // Decode URI encoded filename
        try {
          filename = decodeURIComponent(filename);
        } catch {
          // Keep original if decode fails
        }
      }
    }

    // Create blob and trigger download
    const blob = await response.blob();
    const blobUrl = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = blobUrl;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(blobUrl);

    ElMessage.success("种子文件下载成功");
  } catch (error) {
    console.error("Download failed:", error);
    ElMessage.error("下载失败: " + (error instanceof Error ? error.message : "未知错误"));
  }
}

// 复制下载链接
async function copyDownloadLink(torrent: SearchTorrentItem) {
  const link = torrent.downloadUrl || torrent.magnetLink;
  if (!link) {
    ElMessage.warning("没有可复制的链接");
    return;
  }

  // Build full URL for relative paths
  let fullLink = link;
  if (link.startsWith("/")) {
    fullLink = `${window.location.origin}${link}`;
  }

  // Add title parameter if it's a download URL
  if (torrent.downloadUrl && torrent.title && fullLink.includes("/api/site/")) {
    const separator = fullLink.includes("?") ? "&" : "?";
    fullLink = `${fullLink}${separator}title=${encodeURIComponent(torrent.title)}`;
  }

  try {
    // Try modern clipboard API first
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(fullLink);
    } else {
      // Fallback for non-HTTPS environments
      const textArea = document.createElement("textarea");
      textArea.value = fullLink;
      textArea.style.position = "fixed";
      textArea.style.left = "-9999px";
      textArea.style.top = "-9999px";
      document.body.appendChild(textArea);
      textArea.focus();
      textArea.select();
      const successful = document.execCommand("copy");
      document.body.removeChild(textArea);
      if (!successful) {
        throw new Error("execCommand copy failed");
      }
    }
    ElMessage.success("链接已复制到剪贴板");
  } catch (error) {
    console.error("Copy failed:", error);
    // Show link in a dialog as last resort
    ElMessageBox.alert(`请手动复制以下链接：\n\n${fullLink}`, "复制链接", {
      confirmButtonText: "确定",
      customClass: "copy-link-dialog",
    });
  }
}

// 批量下载种子为 tar 包
async function batchDownloadTorrents() {
  if (selectedTorrents.value.length === 0) {
    ElMessage.warning("请先选择要下载的种子");
    return;
  }

  try {
    await ElMessageBox.confirm(
      `确定要批量下载选中的 ${selectedTorrents.value.length} 个种子吗？\n将打包为 tar.gz 文件下载。`,
      "批量下载确认",
      {
        confirmButtonText: "下载",
        cancelButtonText: "取消",
        type: "info",
      },
    );
  } catch {
    return;
  }

  batchDownloading.value = true;
  try {
    // Build request body with torrent info
    const torrents = selectedTorrents.value.map((t) => ({
      siteId: t.sourceSite,
      torrentId: t.id,
      title: t.title,
    }));

    // Use fetch to POST and download the response as blob
    const response = await fetch("/api/v2/torrents/batch-download", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ torrents }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || `HTTP ${response.status}`);
    }

    // Get filename from Content-Disposition header or use default
    const contentDisposition = response.headers.get("Content-Disposition");
    let filename = `torrents_${new Date().toISOString().slice(0, 10)}.tar.gz`;
    if (contentDisposition) {
      const filenameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/);
      if (filenameMatch && filenameMatch[1]) {
        filename = filenameMatch[1].replace(/['"]/g, "");
      }
    }

    // Create blob and trigger download
    const blob = await response.blob();
    const blobUrl = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = blobUrl;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(blobUrl);

    ElMessage.success("开始下载种子包");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "批量下载失败");
  } finally {
    batchDownloading.value = false;
  }
}

// 打开推送对话框（单个）
function openPushDialog(torrent: SearchTorrentItem) {
  currentPushTorrent.value = torrent;
  pushForm.value = {
    downloaderIds: defaultDownloader.value ? [defaultDownloader.value.id!] : [],
    savePath: "",
    category: "",
    tags: "",
    autoStart: true,
  };
  pushDialogVisible.value = true;
}

// 打开批量推送对话框
function openBatchPushDialog() {
  if (selectedTorrents.value.length === 0) {
    ElMessage.warning("请先选择要推送的种子");
    return;
  }
  pushForm.value = {
    downloaderIds: defaultDownloader.value ? [defaultDownloader.value.id!] : [],
    savePath: "",
    category: "",
    tags: "",
    autoStart: true,
  };
  batchPushDialogVisible.value = true;
}

// 执行单个推送
async function doPush() {
  if (!currentPushTorrent.value) return;
  if (pushForm.value.downloaderIds.length === 0) {
    ElMessage.warning("请选择下载器");
    return;
  }

  pushLoading.value = true;
  try {
    const resp = await torrentPushApi.push({
      downloadUrl: currentPushTorrent.value.downloadUrl,
      magnetLink: currentPushTorrent.value.magnetLink,
      downloaderIds: pushForm.value.downloaderIds,
      savePath: pushForm.value.savePath || undefined,
      category: pushForm.value.category || undefined,
      tags: pushForm.value.tags || undefined,
      autoStart: pushForm.value.autoStart,
      torrentTitle: currentPushTorrent.value.title,
      sourceSite: currentPushTorrent.value.sourceSite,
      sizeBytes: currentPushTorrent.value.sizeBytes,
    });

    if (resp.success) {
      // 检查是否所有结果都是跳过的
      const allSkipped = resp.results.every((r) => r.skipped);
      const skippedCount = resp.results.filter((r) => r.skipped).length;
      const newPushCount = resp.results.filter((r) => r.success && !r.skipped).length;

      if (allSkipped) {
        ElMessage.warning("种子已存在于所有下载器中，已跳过");
      } else if (skippedCount > 0) {
        ElMessage.success(`推送成功 ${newPushCount} 个，跳过 ${skippedCount} 个（已存在）`);
      } else {
        ElMessage.success("推送成功");
      }
      pushDialogVisible.value = false;
    } else {
      const failedResults = resp.results.filter((r) => !r.success);
      const failedMsg = failedResults.map((r) => `${r.downloaderName}: ${r.message}`).join("\n");
      ElMessage.error(`部分推送失败:\n${failedMsg}`);
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "推送失败");
  } finally {
    pushLoading.value = false;
  }
}

// 执行批量推送
async function doBatchPush() {
  if (selectedTorrents.value.length === 0) return;
  if (pushForm.value.downloaderIds.length === 0) {
    ElMessage.warning("请选择下载器");
    return;
  }

  try {
    await ElMessageBox.confirm(
      `确定要推送选中的 ${selectedTorrents.value.length} 个种子吗？`,
      "批量推送确认",
      {
        confirmButtonText: "推送",
        cancelButtonText: "取消",
        type: "info",
      },
    );
  } catch {
    return;
  }

  pushLoading.value = true;
  try {
    const torrents: TorrentPushItem[] = selectedTorrents.value.map((t) => ({
      downloadUrl: t.downloadUrl,
      magnetLink: t.magnetLink,
      torrentTitle: t.title,
      sourceSite: t.sourceSite,
      sizeBytes: t.sizeBytes,
    }));

    const resp = await torrentPushApi.batchPush({
      torrents,
      downloaderIds: pushForm.value.downloaderIds,
      savePath: pushForm.value.savePath || undefined,
      category: pushForm.value.category || undefined,
      tags: pushForm.value.tags || undefined,
      autoStart: pushForm.value.autoStart,
    });

    // 构建更详细的消息
    const parts: string[] = [];
    if (resp.successCount > 0) parts.push(`成功 ${resp.successCount}`);
    if (resp.skippedCount > 0) parts.push(`跳过 ${resp.skippedCount}`);
    if (resp.failedCount > 0) parts.push(`失败 ${resp.failedCount}`);
    const summary = parts.join("，");

    if (resp.success) {
      if (resp.skippedCount > 0 && resp.successCount === 0) {
        // 全部跳过
        ElMessage.warning(`批量推送完成: ${summary}（种子已存在）`);
      } else if (resp.skippedCount > 0) {
        // 部分跳过
        ElMessage.success(`批量推送完成: ${summary}`);
      } else {
        // 全部成功
        ElMessage.success(`批量推送完成: ${summary}`);
      }
      batchPushDialogVisible.value = false;
      selectedTorrents.value = [];
    } else {
      ElMessage.warning(`批量推送部分失败: ${summary}`);
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "批量推送失败");
  } finally {
    pushLoading.value = false;
  }
}

// 清除搜索缓存
async function clearCache() {
  try {
    await searchApi.clearCache();
    sessionStorage.removeItem(CACHE_KEY);
    ElMessage.success("缓存已清除");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "清除缓存失败");
  }
}

// 选择/取消全部站点
function toggleAllSites() {
  if (selectedSites.value.length === availableSites.value.length) {
    selectedSites.value = [];
  } else {
    selectedSites.value = [...availableSites.value];
  }
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h1 class="page-title">种子搜索</h1>
        <p class="page-subtitle">跨站点并发搜索，支持批量推送和下载</p>
      </div>
    </div>

    <!-- 搜索工具栏 -->
    <div class="common-card search-card search-surface">
      <div class="common-card-body">
        <el-form :inline="true" class="search-form modern-search-form" @submit.prevent="doSearch">
          <el-form-item label="关键词">
            <el-input
              v-model="searchKeyword"
              class="keyword-input"
              placeholder="输入搜索关键词"
              clearable
              @keyup.enter="doSearch">
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item label="站点">
            <div class="site-selector-wrapper">
              <el-tooltip
                :content="
                  selectedSites.length === 0
                    ? '未选择站点，将搜索所有可用站点'
                    : `已选择 ${selectedSites.length} 个站点`
                "
                placement="top">
                <el-select
                  v-model="selectedSites"
                  multiple
                  collapse-tags
                  collapse-tags-tooltip
                  :placeholder="
                    availableSites.length > 0 ? `全部 ${availableSites.length} 个站点` : '加载中...'
                  "
                  :class="[
                    { 'all-sites-selected': selectedSites.length === 0 },
                    'site-select-input',
                  ]">
                  <template #header>
                    <div class="site-select-header">
                      <el-checkbox
                        :model-value="selectedSites.length === availableSites.length"
                        :indeterminate="
                          selectedSites.length > 0 && selectedSites.length < availableSites.length
                        "
                        @change="toggleAllSites">
                        全选
                      </el-checkbox>
                      <span class="site-count-hint">
                        {{ selectedSites.length === 0 ? "(未选择 = 搜索全部)" : "" }}
                      </span>
                    </div>
                  </template>
                  <el-option v-for="site in availableSites" :key="site" :label="site" :value="site">
                    <div class="site-option-item">
                      <span>{{ site }}</span>
                      <el-tag
                        v-if="siteHasCategories(site)"
                        type="info"
                        size="small"
                        effect="plain">
                        可筛选
                      </el-tag>
                    </div>
                  </el-option>
                </el-select>
              </el-tooltip>
            </div>
          </el-form-item>

          <!-- 已选站点的分类筛选按钮 -->
          <el-form-item v-if="selectedSites.length > 0">
            <div class="selected-sites-filters">
              <template v-for="siteId in selectedSites" :key="siteId">
                <el-popover
                  v-if="siteHasCategories(siteId)"
                  placement="bottom-start"
                  :width="400"
                  trigger="click">
                  <template #reference>
                    <el-badge
                      :value="getSiteFilterCount(siteId)"
                      :hidden="getSiteFilterCount(siteId) === 0"
                      type="primary"
                      class="site-filter-badge">
                      <el-button
                        size="small"
                        :type="getSiteFilterCount(siteId) > 0 ? 'primary' : 'default'">
                        <el-icon class="filter-icon"><Filter /></el-icon>
                        {{ siteId }}
                      </el-button>
                    </el-badge>
                  </template>
                  <div class="site-filter-popover">
                    <div class="site-filter-header">
                      <span class="filter-title">
                        {{ getSiteCategoriesConfig(siteId)?.site_name || siteId }} 分类筛选
                      </span>
                      <el-button
                        v-if="getSiteFilterCount(siteId) > 0"
                        type="danger"
                        size="small"
                        text
                        @click="clearSiteCategoryFilters(siteId)">
                        清除
                      </el-button>
                    </div>
                    <el-form label-position="top" class="site-filter-form">
                      <el-form-item
                        v-for="category in getSiteCategoriesConfig(siteId)?.categories || []"
                        :key="category.key"
                        :label="category.name">
                        <el-select
                          :model-value="selectedCategoryFilters[siteId]?.[category.key]"
                          placeholder="全部"
                          clearable
                          style="width: 100%"
                          @update:model-value="
                            updateSiteCategoryFilter(siteId, category.key, $event)
                          ">
                          <el-option
                            v-for="opt in category.options"
                            :key="opt.value"
                            :label="opt.name"
                            :value="opt.value" />
                        </el-select>
                      </el-form-item>
                    </el-form>
                  </div>
                </el-popover>
              </template>
            </div>
          </el-form-item>

          <el-form-item label="排序">
            <el-select v-model="sortBy" class="sort-select">
              <el-option label="站点" value="sourceSite" />
              <el-option label="发布时间" value="publishTime" />
              <el-option label="大小" value="size" />
              <el-option label="做种数" value="seeders" />
              <el-option label="下载数" value="leechers" />
              <el-option label="完成数" value="snatched" />
            </el-select>
          </el-form-item>

          <el-form-item>
            <el-switch v-model="orderDesc" active-text="降序" inactive-text="升序" />
          </el-form-item>

          <el-form-item>
            <el-button type="primary" :loading="loading" class="search-btn" @click="doSearch"
              >搜索</el-button
            >
            <el-button class="clear-cache-btn" @click="clearCache">清除缓存</el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <!-- 搜索结果 -->
    <div
      v-loading="loading"
      class="table-card result-card search-result-panel"
      element-loading-text="正在聚合多站点搜索结果..."
      element-loading-background="rgba(20, 184, 166, 0.08)">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <span>搜索结果</span>
          <template v-if="totalResults > 0">
            <el-tag type="info" size="small" effect="plain" class="result-stats-tag">
              共 {{ totalResults }} 条
            </el-tag>
            <el-tag type="success" size="small" effect="plain" class="result-stats-tag">
              耗时 {{ searchTime }}ms
            </el-tag>
          </template>
        </div>
        <div class="table-card-header-actions">
          <el-button
            v-if="selectedTorrents.length > 0"
            size="small"
            type="success"
            class="batch-download-btn"
            :loading="batchDownloading"
            @click="batchDownloadTorrents">
            批量下载 ({{ selectedTorrents.length }})
          </el-button>
          <el-button
            v-if="selectedTorrents.length > 0"
            type="primary"
            size="small"
            class="batch-push-btn"
            @click="openBatchPushDialog">
            批量推送 ({{ selectedTorrents.length }})
          </el-button>
        </div>
      </div>

      <div class="table-wrapper">
        <!-- 站点状态摘要 -->
        <div v-if="Object.keys(siteResultCounts).length > 0" class="filter-bar site-summary-bar">
          <el-tag
            v-for="(count, site) in siteResultCounts"
            :key="site"
            type="success"
            size="small"
            class="site-summary-tag"
            effect="plain">
            {{ site }}: {{ count }}
          </el-tag>
          <el-tag
            v-for="err in searchErrors"
            :key="err.site"
            type="danger"
            size="small"
            class="site-summary-tag site-summary-tag--error"
            effect="plain">
            {{ err.site }}: 失败
          </el-tag>
        </div>

        <!-- 种子列表表格 -->
        <el-table
          :data="pagedTorrents"
          class="pt-table search-result-table"
          stripe
          :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }"
          :default-sort="{ prop: 'sourceSite', order: 'ascending' }"
          @selection-change="handleSelectionChange"
          @sort-change="handleSortChange">
          <el-table-column type="selection" width="45" align="center" />

          <el-table-column
            label="站点"
            prop="sourceSite"
            width="90"
            align="center"
            sortable="custom">
            <template #default="{ row }">
              <el-tag size="small" type="primary" effect="light" class="site-tag">{{
                row.sourceSite
              }}</el-tag>
            </template>
          </el-table-column>

          <el-table-column label="标题" min-width="400">
            <template #default="{ row }">
              <div class="title-cell">
                <el-tooltip :content="row.title" placement="top" :show-after="500">
                  <a
                    v-if="row.url"
                    :href="row.url"
                    target="_blank"
                    rel="noopener"
                    class="title-link">
                    {{ row.title }}
                  </a>
                  <span v-else class="title-text">{{ row.title }}</span>
                </el-tooltip>
                <div
                  v-if="row.subtitle || (row.tags && row.tags.length > 0)"
                  class="title-subtitle-row">
                  <span v-if="row.subtitle" class="subtitle">{{ row.subtitle }}</span>
                  <span v-if="row.tags && row.tags.length > 0" class="title-tags">
                    <el-tag
                      v-for="tag in row.tags"
                      :key="tag"
                      size="small"
                      type="info"
                      effect="plain">
                      {{ tag }}
                    </el-tag>
                  </span>
                </div>
                <div class="title-meta">
                  <el-tag
                    v-if="row.category"
                    size="small"
                    type="info"
                    effect="plain"
                    class="meta-tag">
                    {{ row.category }}
                  </el-tag>
                  <el-tag
                    v-if="row.hasHR"
                    size="small"
                    type="danger"
                    effect="plain"
                    class="meta-tag">
                    H&R
                  </el-tag>
                </div>
              </div>
            </template>
          </el-table-column>

          <el-table-column
            label="大小"
            prop="sizeBytes"
            width="95"
            align="center"
            sortable="custom">
            <template #default="{ row }">
              <span class="table-cell-secondary">{{ formatSize(row.sizeBytes) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="优惠" width="80" align="center">
            <template #default="{ row }">
              <el-tag
                :type="getDiscountTag(row).type"
                size="small"
                effect="dark"
                class="discount-tag">
                {{ getDiscountTag(row).text }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="上传" prop="seeders" width="65" align="center" sortable="custom">
            <template #default="{ row }">
              <span class="seeders">{{ row.seeders }}</span>
            </template>
          </el-table-column>

          <el-table-column label="下载" prop="leechers" width="65" align="center" sortable="custom">
            <template #default="{ row }">
              <span class="leechers">{{ row.leechers }}</span>
            </template>
          </el-table-column>

          <el-table-column label="完成" prop="snatched" width="65" align="center" sortable="custom">
            <template #default="{ row }">
              <span class="table-cell-secondary">{{ row.snatched }}</span>
            </template>
          </el-table-column>

          <el-table-column label="发布时间" prop="uploadedAt" width="150" sortable="custom">
            <template #default="{ row }">
              <span class="table-cell-secondary">{{ formatTime(row.uploadedAt) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="160" align="center" fixed="right">
            <template #default="{ row }">
              <div class="table-cell-actions">
                <el-tooltip content="下载种子" placement="top">
                  <el-button
                    type="success"
                    size="small"
                    circle
                    plain
                    class="action-icon-btn"
                    :disabled="!row.downloadUrl"
                    @click="downloadTorrent(row)">
                    <el-icon><Download /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="复制链接" placement="top">
                  <el-button
                    type="info"
                    size="small"
                    circle
                    plain
                    class="action-icon-btn"
                    :disabled="!row.downloadUrl && !row.magnetLink"
                    @click="copyDownloadLink(row)">
                    <el-icon><CopyDocument /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="推送到下载器" placement="top">
                  <el-button
                    type="primary"
                    size="small"
                    circle
                    plain
                    class="action-icon-btn"
                    @click="openPushDialog(row)">
                    <el-icon><Upload /></el-icon>
                  </el-button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div v-if="sortedResults.length > 0" class="pagination-container">
          <el-pagination
            v-model:current-page="currentPage"
            v-model:page-size="pageSize"
            :page-sizes="pageSizeOptions"
            :total="sortedResults.length"
            layout="total, sizes, prev, pager, next, jumper"
            @current-change="handlePageChange"
            @size-change="handleSizeChange" />
        </div>

        <!-- 空状态 -->
        <div v-if="!loading && sortedResults.length === 0" class="table-empty">
          <el-icon class="table-empty-icon"><Search /></el-icon>
          <p class="table-empty-text">
            {{ searchKeyword ? "没有找到结果" : "输入关键词开始搜索" }}
          </p>
        </div>
      </div>
    </div>

    <!-- 单个推送对话框 -->
    <el-dialog v-model="pushDialogVisible" title="推送到下载器" width="560px" class="push-dialog">
      <el-form :model="pushForm" label-width="100px">
        <el-form-item label="种子标题">
          <span class="dialog-text">{{ currentPushTorrent?.title }}</span>
        </el-form-item>
        <el-form-item label="来源站点">
          <el-tag size="small">{{ currentPushTorrent?.sourceSite }}</el-tag>
        </el-form-item>
        <el-form-item label="文件大小">
          <span>{{ formatSize(currentPushTorrent?.sizeBytes || 0) }}</span>
        </el-form-item>
        <el-form-item label="下载器" required>
          <el-select
            v-model="pushForm.downloaderIds"
            multiple
            placeholder="选择下载器"
            style="width: 100%">
            <el-option
              v-for="d in downloaders.filter((d) => d.enabled)"
              :key="d.id"
              :label="`${d.name} (${d.type})${d.is_default ? ' [默认]' : ''}`"
              :value="d.id!" />
          </el-select>
        </el-form-item>
        <el-form-item label="保存路径">
          <el-select
            v-model="pushForm.savePath"
            placeholder="使用默认路径"
            clearable
            style="width: 100%">
            <template v-for="downloaderId in pushForm.downloaderIds" :key="downloaderId">
              <el-option-group
                v-if="getDirectoryOptions(downloaderId).length > 0"
                :label="downloaders.find((d) => d.id === downloaderId)?.name">
                <el-option
                  v-for="dir in getDirectoryOptions(downloaderId)"
                  :key="dir.id"
                  :label="`${dir.alias || dir.path}${dir.is_default ? ' [默认]' : ''}`"
                  :value="dir.path" />
              </el-option-group>
            </template>
          </el-select>
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="pushForm.category" placeholder="可选" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="pushForm.tags" placeholder="多个标签用逗号分隔" />
        </el-form-item>
        <el-form-item label="自动开始">
          <el-switch v-model="pushForm.autoStart" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pushDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="pushLoading" @click="doPush">推送</el-button>
      </template>
    </el-dialog>

    <!-- 批量推送对话框 -->
    <el-dialog
      v-model="batchPushDialogVisible"
      title="批量推送到下载器"
      width="560px"
      class="push-dialog">
      <el-form :model="pushForm" label-width="100px">
        <el-form-item label="选中数量">
          <el-tag type="primary">{{ selectedTorrents.length }} 个种子</el-tag>
        </el-form-item>
        <el-form-item label="下载器" required>
          <el-select
            v-model="pushForm.downloaderIds"
            multiple
            placeholder="选择下载器"
            style="width: 100%">
            <el-option
              v-for="d in downloaders.filter((d) => d.enabled)"
              :key="d.id"
              :label="`${d.name} (${d.type})${d.is_default ? ' [默认]' : ''}`"
              :value="d.id!" />
          </el-select>
        </el-form-item>
        <el-form-item label="保存路径">
          <el-select
            v-model="pushForm.savePath"
            placeholder="使用默认路径"
            clearable
            style="width: 100%">
            <template v-for="downloaderId in pushForm.downloaderIds" :key="downloaderId">
              <el-option-group
                v-if="getDirectoryOptions(downloaderId).length > 0"
                :label="downloaders.find((d) => d.id === downloaderId)?.name">
                <el-option
                  v-for="dir in getDirectoryOptions(downloaderId)"
                  :key="dir.id"
                  :label="`${dir.alias || dir.path}${dir.is_default ? ' [默认]' : ''}`"
                  :value="dir.path" />
              </el-option-group>
            </template>
          </el-select>
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="pushForm.category" placeholder="可选" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="pushForm.tags" placeholder="多个标签用逗号分隔" />
        </el-form-item>
        <el-form-item label="自动开始">
          <el-switch v-model="pushForm.autoStart" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchPushDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="pushLoading" @click="doBatchPush">批量推送</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";
@import "@/styles/form-page.css";
@import "@/styles/torrent-search-page.css";
</style>
