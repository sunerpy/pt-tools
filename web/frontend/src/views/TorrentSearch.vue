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
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();

// 搜索状态
const searchKeyword = ref("");
/**
 * 当前结果集对应的关键词。输入框里的 searchKeyword 随时在变，用它判断
 * 空结果是「还没搜过」还是「搜了没命中」，会在用户清空输入框时跳变。
 */
const searchedKeyword = ref("");
const selectedSites = ref<string[]>([]);
const availableSites = ref<string[]>([]);

// 搜索结果
const searchResults = ref<SearchTorrentItem[]>([]);
const siteResultCounts = ref<Record<string, number>>({});
const searchErrors = ref<SearchErrorItem[]>([]);
const searchTime = ref(0);
const totalResults = ref(0);

/**
 * 六态状态机（设计文档 §5）。
 *
 * 这页以前只有一个 loading ref：搜索请求整体失败时弹个 toast，两秒后 toast 没了，
 * 表格停在「先输入关键词」上 —— 用户看到的是「没搜到」，真相是请求根本没成功。
 * 401/403 还要单独画成「无权访问」，否则用户会一直点重试。
 *
 * 多站点搜索天生会部分失败，所以 failed 接的是失败站点数：
 * 还有结果时挂一条部分失败提示（hasPartialBanner），一条结果都没有时 partial 成为主状态。
 */
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  filtered: () => searchedKeyword.value !== "",
  failed: () => searchErrors.value.length,
});

/** 失败站点清单，partial 作为主状态时当副标题用 */
const failedSitesText = computed(() => {
  if (searchErrors.value.length === 0) return "";
  return `${searchErrors.value.length} 个站点没有返回结果：${searchErrors.value
    .map((e) => e.site)
    .join("、")}`;
});

/** 状态块的副标题：失败时给真实原因，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "partial") return failedSitesText.value;
  return searchedKeyword.value
    ? "换个关键词，或放开站点和分类筛选"
    : "先输入关键词，回车即可开始搜索";
});

// 分页
const currentPage = ref(1);
const pageSize = ref(50);
const pageSizeOptions = [20, 50, 100];

// 选中的种子（用于批量操作）
const selectedTorrents = ref<SearchTorrentItem[]>([]);

/**
 * 桌面表格的实例引用，只为了多选带上的「取消选择」能把 el-table 自己那份勾选状态也清掉：
 * 光清本页数组的话，表头复选框会停在半选态，行上的勾也还在。
 * 这里只声明用得到的那一个方法，省掉 element-plus 的类型导入。
 */
const tableRef = ref<{ clearSelection: () => void } | null>(null);

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
    // 缓存里的结果集就是这个关键词搜出来的，一起恢复，空结果才能正确画成 zero
    searchedKeyword.value = data.keyword || "";
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

  clearSelection();
  currentPage.value = 1;
  searchedKeyword.value = searchKeyword.value.trim();

  // 站点列表也套在 run 里：它在请求之前，不然这段时间面板上没有加载态
  const resp = await run(async () => {
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
    return await searchApi.multiSite(req);
  });

  if (!resp) {
    // 失败时清空：留着上一次的结果配一个「加载失败」的状态块更让人误解
    searchResults.value = [];
    siteResultCounts.value = {};
    searchErrors.value = [];
    searchTime.value = 0;
    totalResults.value = 0;
    ElMessage.error(errorText.value || "搜索失败");
    return;
  }

  searchResults.value = resp.items || [];
  siteResultCounts.value = resp.siteResults || {};
  searchErrors.value = resp.errors || [];
  searchTime.value = resp.durationMs;
  totalResults.value = resp.totalResults;

  // 保存到缓存
  saveToCache();

  // toast 照旧弹，但它只是提醒；失败清单同时留在结果上方的提示条（或 partial 状态块）里
  if (searchErrors.value.length > 0) {
    const failedNames = searchErrors.value.map((e) => `${e.site}: ${e.error}`).join("\n");
    ElMessage.warning({
      message: `部分站点搜索失败:\n${failedNames}`,
      duration: 5000,
    });
  }
}

function handleSelectionChange(selection: SearchTorrentItem[]) {
  selectedTorrents.value = selection;
}

/**
 * 移动端行卡上的勾选。el-table 的多选是它自己管的，卡片这边自己维护同一份选中列表。
 * 按对象身份比对而不是 id：同一个种子 id 在不同站点会撞，而 pagedTorrents 里的对象
 * 就是 searchResults 里的那几个（sortedResults 只是浅拷贝后排序）。
 */
function isSelected(torrent: SearchTorrentItem): boolean {
  return selectedTorrents.value.includes(torrent);
}

function toggleSelect(torrent: SearchTorrentItem) {
  const i = selectedTorrents.value.indexOf(torrent);
  if (i === -1) selectedTorrents.value.push(torrent);
  else selectedTorrents.value.splice(i, 1);
}

/** 多选带上的「取消选择」：两套视图各自的选中态都要清 */
function clearSelection() {
  selectedTorrents.value = [];
  tableRef.value?.clearSelection();
}

/** 行卡的 key：搜索结果跨站点聚合，单靠站内 id 不唯一 */
function rowKey(torrent: SearchTorrentItem): string {
  return `${torrent.sourceSite}:${torrent.id}`;
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

/* 优惠是种子的状态而不是分类，所以走胶囊；这里把 el-tag 的类型名折到胶囊的语气上 */
function discountTone(torrent: SearchTorrentItem): "ok" | "warn" | "dang" | "neutral" {
  switch (getDiscountTag(torrent).type) {
    case "success":
      return "ok";
    case "warning":
      return "warn";
    case "danger":
      return "dang";
    default:
      return "neutral";
  }
}

/**
 * 画板 head 的 sub —— 标题下面那行实时摘要（11.5/400 t3）。
 *
 * 这一页的口径就是「这次搜索的战果」：命中数、有结果的站点数、耗时，失败站点补在末尾。
 * 还没搜过时返回空串（外壳里那一行 :empty 会自己收起来），不摆占位文案。
 */
const headSub = computed(() => {
  if (!searchedKeyword.value) return "";
  if (loading.value) return `正在搜索「${searchedKeyword.value}」`;
  const hitSites = Object.values(siteResultCounts.value).filter((n) => n > 0).length;
  const parts = [`命中 ${totalResults.value} 条`];
  if (hitSites > 0) parts.push(`来自 ${hitSites} 个站点`);
  parts.push(`耗时 ${searchTime.value} ms`);
  if (searchErrors.value.length > 0) parts.push(`${searchErrors.value.length} 个站点失败`);
  return parts.join(" · ");
});

/**
 * 画板 gfoot 的左侧说明（11.5/400 t3）：命中口径 + 当前页区间 + 每页条数，
 * 例「命中 128 条 · 显示 1–50 · 每页 50 · 耗时 1240 ms」。
 * 摘要行在手机上被外壳藏了页头，这行就是唯一能看到命中与耗时的地方，所以耗时也留着。
 */
const footNote = computed(() => {
  const total = sortedResults.value.length;
  if (total === 0) return "";
  const from = (currentPage.value - 1) * pageSize.value + 1;
  const to = Math.min(currentPage.value * pageSize.value, total);
  const parts = [`命中 ${total} 条`, `显示 ${from}–${to}`, `每页 ${pageSize.value}`];
  if (searchTime.value > 0) parts.push(`耗时 ${searchTime.value} ms`);
  if (searchErrors.value.length > 0) parts.push(`${searchErrors.value.length} 个站点失败`);
  return parts.join(" · ");
});
</script>

<template>
  <div class="search-page">
    <!-- 画板 head 的 sub：这次搜索的命中数与耗时，由本页把真实数字送进外壳页头 -->
    <Teleport v-if="headSub" to="#pt-head-sub">{{ headSub }}</Teleport>

    <!-- 表格带：画板 grid 是全宽平铺的带，不是圆角描边卡片 -->
    <div v-loading="loading" class="pt-band--grid" element-loading-text="正在聚合多站点搜索结果...">
      <!--
        工具栏带 —— 画板 bar-88（40 高）：这里只留筛选类控件（站点、排序、方向、分类 chip），
        右侧一个 28×28 图标钮。关键词与主搜索按钮按 search-head 的落法进页头。
      -->
      <PtToolbar band>
        <!--
          关键词 + 搜索。桌面送进页头 —— 画板 15 的 head 之所以是 88 高，就是因为
          搜索条长在页头里。移动端外壳把整条页头藏了（shell.css ≤768 时 .pt-head 是
          display:none），所以这里用 Teleport 的 disabled 让同一段控件原地落回工具栏，
          否则手机上连搜索入口都没有。
        -->
        <Teleport to="#pt-head-acts" :disabled="isMobile">
          <div class="hsearch">
            <el-input
              v-model="searchKeyword"
              class="hsearch__kw"
              placeholder="输入关键词，回车即搜"
              clearable
              @keyup.enter="doSearch">
              <template #prefix>
                <PtIcon name="search" :size="14" />
              </template>
            </el-input>
            <el-button type="primary" :loading="loading" @click="doSearch">
              <PtIcon v-if="!loading" name="search" :size="15" /><span>搜索</span>
            </el-button>
          </div>
        </Teleport>

        <!-- 站点多选：不选 = 搜全部，占位符直接把「全部 N 个站点」说出来，省掉一个标签 -->
        <el-tooltip
          :content="
            selectedSites.length === 0
              ? '未选择站点，将搜索所有可用站点'
              : `已选择 ${selectedSites.length} 个站点`
          "
          placement="bottom">
          <el-select
            v-model="selectedSites"
            class="tb__ctl tb__ctl--sites"
            multiple
            collapse-tags
            collapse-tags-tooltip
            :placeholder="
              availableSites.length > 0 ? `全部 ${availableSites.length} 个站点` : '加载中...'
            ">
            <template #header>
              <div class="site-head">
                <el-checkbox
                  :model-value="selectedSites.length === availableSites.length"
                  :indeterminate="
                    selectedSites.length > 0 && selectedSites.length < availableSites.length
                  "
                  @change="toggleAllSites">
                  全选
                </el-checkbox>
                <span class="site-head__hint">
                  {{ selectedSites.length === 0 ? "不选 = 搜全部" : "" }}
                </span>
              </div>
            </template>
            <el-option v-for="site in availableSites" :key="site" :label="site" :value="site">
              <div class="site-opt">
                <span>{{ site }}</span>
                <PtTag v-if="siteHasCategories(site)">可筛选</PtTag>
              </div>
            </el-option>
          </el-select>
        </el-tooltip>

        <!-- 排序：选项自带「按」字，这样 40 高的带里不用再多一个「排序」标签 -->
        <el-select v-model="sortBy" class="tb__ctl tb__ctl--sort">
          <el-option label="按站点" value="sourceSite" />
          <el-option label="按发布时间" value="publishTime" />
          <el-option label="按大小" value="size" />
          <el-option label="按做种数" value="seeders" />
          <el-option label="按下载数" value="leechers" />
          <el-option label="按完成数" value="snatched" />
        </el-select>

        <el-button class="tb__dir" @click="orderDesc = !orderDesc">
          <PtIcon name="arrow-up-down" :size="14" />
          <span>{{ orderDesc ? "降序" : "升序" }}</span>
        </el-button>

        <!-- 支持分类的站点各给一枚 chip（画板 chip 24 高），按钮上带已选条数 -->
        <template v-for="siteId in selectedSites" :key="siteId">
          <el-popover
            v-if="siteHasCategories(siteId)"
            placement="bottom-start"
            :width="400"
            trigger="click">
            <template #reference>
              <el-button
                size="small"
                :type="getSiteFilterCount(siteId) > 0 ? 'primary' : 'default'">
                <PtIcon name="list-filter" :size="14" />
                <span>{{ siteId }}</span>
                <span v-if="getSiteFilterCount(siteId) > 0" class="cats__n">
                  {{ getSiteFilterCount(siteId) }}
                </span>
              </el-button>
            </template>
            <div class="cat">
              <div class="cat__head">
                <span class="cat__title">
                  {{ getSiteCategoriesConfig(siteId)?.site_name || siteId }} 分类筛选
                </span>
                <el-button
                  v-if="getSiteFilterCount(siteId) > 0"
                  link
                  type="danger"
                  size="small"
                  @click="clearSiteCategoryFilters(siteId)">
                  <PtIcon name="x" :size="13" /><span>清除</span>
                </el-button>
              </div>
              <el-form label-position="top" class="pt-form">
                <el-form-item
                  v-for="category in getSiteCategoriesConfig(siteId)?.categories || []"
                  :key="category.key"
                  :label="category.name">
                  <el-select
                    :model-value="selectedCategoryFilters[siteId]?.[category.key]"
                    placeholder="全部"
                    clearable
                    style="width: 100%"
                    @update:model-value="updateSiteCategoryFilter(siteId, category.key, $event)">
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

        <template #right>
          <el-tooltip content="丢掉服务端的搜索缓存，下次搜索重新请求各站点" placement="bottom">
            <button
              type="button"
              class="pt-band__iconbtn"
              aria-label="清除搜索缓存"
              @click="clearCache">
              <PtIcon name="rotate-ccw" :size="15" />
            </button>
          </el-tooltip>
        </template>
      </PtToolbar>

      <!--
        部分失败（§5 的 partial）：还有结果可看时不能用一整块状态图顶掉列表 ——
        那等于把已经拿到的数据也藏了。所以有结果时在结果上方挂这条提示，
        一条结果都没有时才让 partial 成为主状态（见下面的 PtDataState）。
      -->
      <div v-if="hasPartialBanner(sortedResults.length)" class="pt-note pt-note--warn partial">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <div class="partial__body">
          <p class="partial__t">
            {{ searchErrors.length }} 个站点没有返回结果，下面只是其余站点的命中
          </p>
          <p v-for="err in searchErrors" :key="err.site" class="partial__l">
            {{ err.site }}：{{ err.error }}
          </p>
        </div>
        <el-button size="small" :loading="loading" @click="doSearch">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </div>

      <el-table
        v-if="!isMobile"
        ref="tableRef"
        :data="pagedTorrents"
        class="pt-grid"
        :default-sort="{ prop: 'sourceSite', order: 'ascending' }"
        @selection-change="handleSelectionChange"
        @sort-change="handleSortChange">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <!-- perm 不给重试：没权限点重试没有意义，只会让用户一直点 -->
            <template v-if="state === 'error' || state === 'partial'" #action>
              <el-button size="small" @click="doSearch">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column type="selection" width="45" align="center" />

        <el-table-column label="站点" prop="sourceSite" width="90" align="center" sortable="custom">
          <template #default="{ row }">
            <PtTag>{{ row.sourceSite }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column label="标题" min-width="400">
          <template #default="{ row }">
            <div class="ti">
              <el-tooltip :content="row.title" placement="top" :show-after="500">
                <a v-if="row.url" :href="row.url" target="_blank" rel="noopener" class="ti__t">
                  {{ row.title }}
                </a>
                <span v-else class="ti__t ti__t--plain">{{ row.title }}</span>
              </el-tooltip>
              <p v-if="row.subtitle" class="ti__sub">{{ row.subtitle }}</p>
              <div
                v-if="row.category || row.hasHR || (row.tags && row.tags.length > 0)"
                class="ti__tags">
                <PtTag v-if="row.category">{{ row.category }}</PtTag>
                <PtTag v-for="tag in row.tags || []" :key="tag">{{ tag }}</PtTag>
                <PtStatusPill v-if="row.hasHR" tone="dang" size="sm">H&amp;R</PtStatusPill>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column
          label="大小"
          prop="sizeBytes"
          width="95"
          align="center"
          sortable="custom"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatSize(row.sizeBytes) }}</template>
        </el-table-column>

        <el-table-column label="优惠" width="82" align="center">
          <template #default="{ row }">
            <PtStatusPill :tone="discountTone(row)" size="sm">
              {{ getDiscountTag(row).text }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column
          label="上传"
          prop="seeders"
          width="84"
          align="center"
          sortable="custom"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="peers peers--up">{{ row.seeders }}</span>
          </template>
        </el-table-column>

        <el-table-column
          label="下载"
          prop="leechers"
          width="84"
          align="center"
          sortable="custom"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="peers peers--down">{{ row.leechers }}</span>
          </template>
        </el-table-column>

        <el-table-column
          label="完成"
          prop="snatched"
          width="84"
          align="center"
          sortable="custom"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ row.snatched }}</template>
        </el-table-column>

        <el-table-column
          label="发布时间"
          prop="uploadedAt"
          width="152"
          sortable="custom"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <span class="ts">{{ formatTime(row.uploadedAt) }}</span>
          </template>
        </el-table-column>

        <el-table-column
          label="操作"
          width="132"
          align="center"
          fixed="right"
          class-name="pt-cell-act">
          <template #default="{ row }">
            <el-tooltip content="下载种子文件" placement="top">
              <span class="act">
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="下载种子文件"
                  :disabled="!row.downloadUrl"
                  @click="downloadTorrent(row)">
                  <PtIcon name="download" :size="15" />
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip content="复制下载链接" placement="top">
              <span class="act">
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="复制下载链接"
                  :disabled="!row.downloadUrl && !row.magnetLink"
                  @click="copyDownloadLink(row)">
                  <PtIcon name="copy" :size="15" />
                </el-button>
              </span>
            </el-tooltip>
            <el-tooltip content="推送到下载器" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                aria-label="推送到下载器"
                @click="openPushDialog(row)">
                <PtIcon name="upload" :size="15" />
              </el-button>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 10 列，手机上横着滚既看不到列头也和页面纵向滚动打架。
        卡上留的是判断一个种子够不够抢真正要看的：标题（+副标题）、站点/分类/大小/
        做种下载完成/发布时间、右上角优惠与 H&R，底下三个操作。
        排序仍然走上面搜索条里的「排序 / 方向」，所以表头的 sortable 不算丢功能。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!pagedTorrents.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error' || state === 'partial'" #action>
            <el-button size="small" @click="doSearch">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="torrent in pagedTorrents" :key="rowKey(torrent)">
          <template #lead>
            <el-checkbox
              :model-value="isSelected(torrent)"
              :aria-label="`选择 ${torrent.title}`"
              @update:model-value="toggleSelect(torrent)" />
          </template>

          <template #title>
            <a
              v-if="torrent.url"
              :href="torrent.url"
              target="_blank"
              rel="noopener"
              class="card-link">
              {{ torrent.title }}
            </a>
            <template v-else>{{ torrent.title }}</template>
          </template>

          <template #meta>
            <span v-if="torrent.subtitle" class="card-sub">{{ torrent.subtitle }}</span>
            <PtTag>{{ torrent.sourceSite }}</PtTag>
            <PtTag v-if="torrent.category">{{ torrent.category }}</PtTag>
            <span>{{ formatSize(torrent.sizeBytes) }}</span>
            <span class="card-peers">
              <span class="peers peers--up">做种 {{ torrent.seeders }}</span>
              <span class="peers peers--down">下载 {{ torrent.leechers }}</span>
              <span>完成 {{ torrent.snatched }}</span>
            </span>
            <span>
              <PtIcon name="clock" :size="11" />
              {{ formatTime(torrent.uploadedAt) }}
            </span>
          </template>

          <template #status>
            <span class="card-status">
              <PtStatusPill :tone="discountTone(torrent)" size="sm">
                {{ getDiscountTag(torrent).text }}
              </PtStatusPill>
              <PtStatusPill v-if="torrent.hasHR" tone="dang" size="sm">H&amp;R</PtStatusPill>
            </span>
          </template>

          <template #actions>
            <el-button
              size="small"
              :disabled="!torrent.downloadUrl"
              @click="downloadTorrent(torrent)">
              <PtIcon name="download" :size="14" /><span>下载</span>
            </el-button>
            <el-button
              size="small"
              :disabled="!torrent.downloadUrl && !torrent.magnetLink"
              @click="copyDownloadLink(torrent)">
              <PtIcon name="copy" :size="14" /><span>复制</span>
            </el-button>
            <el-button type="primary" size="small" @click="openPushDialog(torrent)">
              <PtIcon name="upload" :size="14" /><span>推送</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>
    </div>

    <!--
      画板 gfoot 34：左说明 + 右分页。板 15 的带序是 grid 128..430 → gfoot 430..464
      → selband 464..508，所以页脚在多选带之前，和任务列表页正好相反。
    -->
    <div v-if="sortedResults.length > 0" class="pt-band--foot">
      <span>{{ footNote }}</span>
      <span class="pt-band__spacer" />
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        class="pt-pager"
        :page-sizes="pageSizeOptions"
        :total="sortedResults.length"
        layout="sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handleSizeChange" />
    </div>

    <!-- 画板 selband 44（cyan@0.06）：批量操作从原来的面板头部挪到这条带上 -->
    <div v-if="selectedTorrents.length > 0" class="pt-band--sel">
      <PtIcon name="square-check" :size="15" />
      <span>已选 {{ selectedTorrents.length }} 个种子</span>
      <span class="pt-band__spacer" />
      <el-button :loading="batchDownloading" @click="batchDownloadTorrents">
        <PtIcon v-if="!batchDownloading" name="file-down" :size="15" /><span>打包下载</span>
      </el-button>
      <el-button type="primary" @click="openBatchPushDialog">
        <PtIcon name="upload" :size="15" /><span>批量推送</span>
      </el-button>
      <el-button link @click="clearSelection">取消选择</el-button>
    </div>

    <!--
      画板 p-sites 344,524：各站点的命中数属于表格下方的分析卡片，
      不是工具栏上的一排 chip —— 站点一多，那排 chip 会把 40 高的带挤爆。
    -->
    <div v-if="Object.keys(siteResultCounts).length > 0" class="pt-cards pt-cards--wide">
      <PtPanel
        title="站点命中"
        icon="layers"
        :count="`${Object.keys(siteResultCounts).length} 个站点`">
        <div class="sites">
          <PtTag v-for="(count, site) in siteResultCounts" :key="site">
            {{ site }} · {{ count }}
          </PtTag>
        </div>
      </PtPanel>
    </div>

    <!-- 单个推送对话框 -->
    <el-dialog
      v-model="pushDialogVisible"
      class="pt-dialog"
      title="推送到下载器"
      width="560px"
      align-center>
      <el-form :model="pushForm" label-position="top" class="pt-form">
        <div class="field-head">种子</div>
        <p class="push-title">{{ currentPushTorrent?.title }}</p>
        <div class="push-meta">
          <PtTag>{{ currentPushTorrent?.sourceSite }}</PtTag>
          <span>{{ formatSize(currentPushTorrent?.sizeBytes || 0) }}</span>
        </div>

        <div class="field-head">推送目标</div>
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
        <div class="field-row">
          <el-form-item label="分类">
            <el-input v-model="pushForm.category" placeholder="可选" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="pushForm.tags" placeholder="多个标签用逗号分隔" />
          </el-form-item>
        </div>
        <el-form-item label="添加后立即开始">
          <span class="sw"><el-switch v-model="pushForm.autoStart" /></span>
          <div class="field-tip">关掉则种子以暂停状态入队，适合先排队再统一开始</div>
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
      class="pt-dialog"
      title="批量推送到下载器"
      width="560px"
      align-center>
      <el-form :model="pushForm" label-position="top" class="pt-form">
        <div class="pt-note">
          <PtIcon name="package" :size="14" class="pt-note__icon" />
          <span>
            将推送选中的 <strong>{{ selectedTorrents.length }}</strong> 个种子，
            下面的设置对每个种子都生效。
          </span>
        </div>

        <div class="field-head">推送目标</div>
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
        <div class="field-row">
          <el-form-item label="分类">
            <el-input v-model="pushForm.category" placeholder="可选" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="pushForm.tags" placeholder="多个标签用逗号分隔" />
          </el-form-item>
        </div>
        <el-form-item label="添加后立即开始">
          <span class="sw"><el-switch v-model="pushForm.autoStart" /></span>
          <div class="field-tip">关掉则种子以暂停状态入队，适合先排队再统一开始</div>
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
/*
 * 画板主区是一串全宽横向带（工具栏 40 → 表格 → 页脚带 34 → 多选带 44 → 分析卡片），
 * 带与带之间没有间距，所以这里不给 gap；需要内缩的东西（提示条、卡片、移动端行卡）
 * 自己带 16 的留白。
 */
.search-page {
  display: flex;
  flex-direction: column;
}

/*
 * 关键词 + 搜索按钮。桌面落在页头右侧（画板 search-head 里的那条搜索条），
 * 手机上原地落回工具栏，所以宽度在断点里换成弹性的。
 */
.hsearch {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.hsearch__kw {
  width: 260px;
}

/* 画板：页头里的控件高 32 */
.hsearch__kw :deep(.el-input__wrapper) {
  height: 32px;
}

/* 工具栏里的控件按画板的 28 高：Element 默认的 32 会把 40 高的带顶满 */
.tb__ctl {
  flex: 0 0 auto;
}

.tb__ctl :deep(.el-select__wrapper) {
  min-height: 28px;
  padding: 2px 8px;
  font-size: var(--pt-fz-sm);
}

.tb__ctl--sites {
  width: 208px;
}

.tb__ctl--sort {
  width: 132px;
}

.tb__dir {
  height: 28px;
  margin: 0;
}

.site-head {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--pt-space-2);
}

.site-head__hint {
  font-size: var(--pt-fz-label);
  color: var(--pt-t4);
}

.site-opt {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  justify-content: space-between;
}

/* 筛选按钮上的计数：原来用 el-badge 悬在角上，会被相邻按钮压住 */
.cats__n {
  padding: 0 5px;
  margin-left: 2px;
  font-size: 10px;
  font-weight: 600;
  line-height: 15px;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: 999px;
}

.cat {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.cat__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.cat__title {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
}

/* 站点命中数：现在长在表格下方的分析卡片里，留白由 PtPanel 的 16 内边距给 */
.sites {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

/* 部分失败提示条：.pt-note 自带色条和底色，这里只补带内的留白与右侧按钮 */
.partial {
  align-items: flex-start;
  margin: var(--pt-space-3) var(--pt-pad);
}

.partial__body {
  flex: 1;
  min-width: 0;
}

.partial__t {
  margin: 0;
  font-weight: 500;
  color: var(--pt-t1);
}

.partial__l {
  margin: 2px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  overflow-wrap: anywhere;
}

/* 标题列是这张表的重心：标题一行、副标题一行、标签一行，其余列都只放一个数 */
.ti {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 2px 0;
}

.ti__t {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
  overflow-wrap: anywhere;
}

.ti__t:hover {
  text-decoration: underline;
}

.ti__t--plain {
  color: var(--pt-t1);
}

.ti__sub {
  margin: 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.ti__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

/* 做种/下载两列上色：一眼看出这个种子是好抢还是难抢 */
.peers {
  font-weight: 600;
}

.peers--up {
  color: var(--pt-ok);
}

.peers--down {
  color: var(--pt-warn);
}

.ts {
  font-size: var(--pt-fz-sm);
  font-variant-numeric: tabular-nums;
}

/* el-tooltip 要一个能接事件的宿主，disabled 的按钮自己不派发 mouseenter */
.act,
.sw {
  display: inline-flex;
}

.push-title {
  margin: 0 0 var(--pt-space-2);
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}

.push-meta {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 移动端行卡列表：表格带是贴边的，行卡这一支自己留 16 内边距（同样板 .cards） */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad);
}

.card-link {
  color: var(--pt-p);
  text-decoration: none;
}

/* 副标题独占 meta 的第一行，别和站点、大小挤在一起 */
.card-sub {
  flex: 0 0 100%;
  overflow-wrap: anywhere;
}

/* 做种/下载/完成三个数算一组，不占三个 meta 位；
   组内间距靠 margin 而不是覆写 gap —— gap 是 PtRowCard 给每个 meta 子项定的 */
.card-peers span + span {
  margin-left: 4px;
}

/* 优惠和 H&R 都是状态，竖着叠在右上角 */
.card-status {
  display: inline-flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
}

@media (max-width: 768px) {
  /* 页头被外壳藏了，搜索那段控件原地落在工具栏里：占满一行，输入框吃掉剩余宽度 */
  .hsearch {
    flex: 1 1 100%;
  }

  .hsearch__kw {
    flex: 1 1 auto;
    width: auto;
  }

  .tb__ctl--sites,
  .tb__ctl--sort {
    width: 140px;
  }
}
</style>
