<script setup lang="ts">
import {
  type DownloaderCapability,
  downloaderTorrentsApi,
  downloadersApi,
  type DownloaderSetting,
  type TorrentDetailResponse,
  type DownloaderTorrentItem,
  type TorrentActionTarget,
} from "@/api";
import DownloaderTorrentDetail from "@/components/downloader/DownloaderTorrentDetail.vue";
import DownloaderTorrentTable from "@/components/downloader/DownloaderTorrentTable.vue";
import DownloaderTorrentVirtualTable from "@/components/downloader/DownloaderTorrentVirtualTable.vue";
import PtIcon from "@/components/PtIcon";
import PtLogo from "@/components/PtLogo";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage } from "element-plus";
import { useThemeStore } from "@/stores/theme";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
const themeStore = useThemeStore();

/*
 * 工具条那颗标志坐在 --pt-hover 上，明暗随模式变，所以变体得跟着表面换：
 * brand.md 按表面明暗判定 —— 浅色面用自带底板的彩色版，深色面用单色版。
 * 页头 hero 那颗不参与：它坐在 --pt-chrome 上，8 套配色里恒为深色，固定 mono。
 */
const logoVariant = computed(() => (themeStore.isDark ? "mono" : "plated"));

const COLUMN_STORAGE_KEY = "downloader-hub-visible-columns-v1";
const COLUMN_ORDER_STORAGE_KEY = "downloader-hub-column-order-v1";
const DENSITY_STORAGE_KEY = "downloader-hub-density-v1";
const LAYOUT_PRESET_STORAGE_KEY = "downloader-hub-layout-preset-v1";
const DETAIL_MODE_STORAGE_KEY = "downloader-hub-detail-mode-v1";
const HERO_VISIBLE_STORAGE_KEY = "downloader-hub-hero-visible-v1";
const SIDEBAR_VISIBLE_STORAGE_KEY = "downloader-hub-sidebar-visible-v1";
const SIDEBAR_WIDTH_STORAGE_KEY = "downloader-hub-sidebar-width-v1";
const SIDEBAR_WIDTH_MIN = 280;
const SIDEBAR_WIDTH_MAX = 420;
const SIDEBAR_WIDTH_DEFAULT = 320;
const MAX_ALL_TASK_ROWS = 5000;
const ALL_COLUMN_KEYS = [
  "status_bar",
  "downloader_name",
  "title",
  "progress",
  "seeds",
  "connections",
  "size",
  "upload_speed",
  "download_speed",
  "added_at",
  "completed_at",
  "ratio",
  "state",
  "eta",
  "category",
  "tags",
];
const DEFAULT_VISIBLE_COLUMNS = [
  "status_bar",
  "title",
  "progress",
  "seeds",
  "connections",
  "size",
  "upload_speed",
  "download_speed",
  "added_at",
  "completed_at",
  "ratio",
  "state",
  "eta",
  "category",
  "tags",
];

const loading = ref(false);
const actionLoading = ref(false);
const addLoading = ref(false);
const autoRefreshEnabled = ref(true);
let refreshTimer: ReturnType<typeof setInterval> | null = null;
let autoRefreshTick = 0;
const INTERACTION_IDLE_MS = 1200;
const HEIGHT_UPDATE_THROTTLE_MS = 120;

const page = ref(1);
const pageSize = ref(100);
const total = ref(0);
const sortBy = ref("added_at");
const sortOrder = ref<"asc" | "desc">("desc");
const showAllTasks = ref(false);
const useVirtualList = ref(true);
const detailMode = ref<"drawer" | "inline">(loadDetailMode());
const sidebarVisible = ref(localStorage.getItem(SIDEBAR_VISIBLE_STORAGE_KEY) !== "false");
const sidebarWidth = ref(loadSidebarWidth());
const heroVisible = ref(localStorage.getItem(HERO_VISIBLE_STORAGE_KEY) === "true");

const torrents = ref<DownloaderTorrentItem[]>([]);
const downloaders = ref<DownloaderSetting[]>([]);
const selectedRows = ref<DownloaderTorrentItem[]>([]);
const selectedRowKeys = ref<string[]>([]);
const allTasksLimited = ref(false);
const capabilities = ref<DownloaderCapability[]>([]);

const filters = ref({
  search: "",
  downloaderId: "all",
  state: "",
  category: "",
  tag: "",
});

const locationDialogVisible = ref(false);
const newLocation = ref("");

const addDialogVisible = ref(false);
const uploadFileBase64 = ref("");
const uploadFileName = ref("");
const addForm = ref({
  downloaderIds: [] as number[],
  sourceUrl: "",
  magnetLink: "",
  savePath: "",
  category: "",
  tags: "",
  addPaused: false,
});

const detailDrawerVisible = ref(false);
const detailLoading = ref(false);
const detail = ref<TorrentDetailResponse | null>(null);
const inlineDetailVisible = ref(false);
const detailActiveTab = ref("files");
let detailRequestId = 0;
const isUserInteracting = ref(false);
const virtualContainer = ref<HTMLElement | null>(null);
const hubLayoutRef = ref<HTMLElement | null>(null);
const hubMainRef = ref<HTMLElement | null>(null);
const tableCardBodyRef = ref<HTMLElement | null>(null);
const paginationRef = ref<HTMLElement | null>(null);
const headerColumnMenuVisible = ref(false);
const headerColumnMenuX = ref(0);
const headerColumnMenuY = ref(0);
const headerColumnMenuRef = ref<HTMLElement | null>(null);
const hideRowContextMenuToken = ref(0);
let interactionTimer: number | null = null;
let heightUpdateTimer: number | null = null;
let pendingHeightUpdate = false;
const visibleColumns = ref<string[]>(loadVisibleColumns());
const columnOrder = ref<string[]>(loadColumnOrder());
const tableDensity = ref<"compact" | "comfortable">(loadDensity());
const sidebarStyle = computed(() => ({
  width: `${sidebarWidth.value}px`,
  minWidth: `${sidebarWidth.value}px`,
}));

const sortOptions = [
  { label: "添加日期", value: "added_at" },
  { label: "完成日期", value: "completed_at" },
  { label: "标题", value: "title" },
  { label: "进度", value: "progress" },
  { label: "大小", value: "size" },
  { label: "分享率", value: "ratio" },
  { label: "做种数", value: "seeds" },
  { label: "连接数", value: "connections" },
  { label: "上传速度", value: "upload_speed" },
  { label: "下载速度", value: "download_speed" },
  { label: "状态", value: "state" },
  { label: "ETA", value: "eta" },
  { label: "下载器", value: "downloader_name" },
];

const selectedCount = computed(() => selectedRows.value.length);
const torrentStateCounters = computed(() => {
  let downloading = 0;
  let seeding = 0;
  let paused = 0;
  let stopped = 0;
  let error = 0;
  for (const torrent of torrents.value) {
    const normalizedState = normalizeTorrentState(torrent.state);
    switch (normalizedState) {
      case "downloading":
        downloading += 1;
        break;
      case "seeding":
        seeding += 1;
        break;
      case "paused":
        paused += 1;
        break;
      case "stopped":
        stopped += 1;
        break;
      case "error":
        error += 1;
        break;
      default:
        break;
    }
  }
  return { downloading, seeding, paused, stopped, error };
});
const downloadingCount = computed(() => torrentStateCounters.value.downloading);
const seedingCount = computed(() => torrentStateCounters.value.seeding);
const pausedCount = computed(() => torrentStateCounters.value.paused);
const stoppedCount = computed(() => torrentStateCounters.value.stopped);
const errorCount = computed(() => torrentStateCounters.value.error);
const allCategories = ref<string[]>([]);
const allTags = ref<string[]>([]);
const transferStats = ref<{
  total_upload_speed: number;
  total_download_speed: number;
  total_uploaded: number;
  total_downloaded: number;
  total_session_uploaded: number;
  total_session_downloaded: number;
  total_free_space: number;
} | null>(null);

const downloaderOptions = computed(() => [
  { label: "全部下载器", value: "all" },
  ...downloaders.value.map((d) => ({ label: `${d.name} (${d.type})`, value: String(d.id || "") })),
]);

const columnOptions = [
  { label: "状态条", value: "status_bar" },
  { label: "下载器", value: "downloader_name" },
  { label: "标题", value: "title" },
  { label: "进度", value: "progress" },
  { label: "做种数", value: "seeds" },
  { label: "连接数", value: "connections" },
  { label: "大小", value: "size" },
  { label: "上传速度", value: "upload_speed" },
  { label: "下载速度", value: "download_speed" },
  { label: "添加日期", value: "added_at" },
  { label: "完成日期", value: "completed_at" },
  { label: "分享率", value: "ratio" },
  { label: "状态", value: "state" },
  { label: "ETA", value: "eta" },
  { label: "分类", value: "category" },
  { label: "标签", value: "tags" },
];

type LayoutPreset = {
  visibleColumns: string[];
  columnOrder: string[];
  density: "compact" | "comfortable";
  sortBy: string;
  sortOrder: "asc" | "desc";
};

const draggingColumn = ref<string | null>(null);

const virtualScrollTop = ref(0);
const virtualOverscan = 20;
const tableMaxHeight = ref(620);
const tableLoading = computed(() => loading.value && torrents.value.length === 0);
let resizeObserver: ResizeObserver | null = null;
let sidebarResizeDragging = false;

const virtualRowHeight = computed(() => (tableDensity.value === "compact" ? 40 : 48));

const virtualStartIndex = computed(() => {
  const raw = Math.floor(virtualScrollTop.value / virtualRowHeight.value) - virtualOverscan;
  return Math.max(0, raw);
});

const virtualVisibleCount = computed(() => {
  const rowsInView = Math.ceil(tableMaxHeight.value / virtualRowHeight.value);
  return Math.max(1, rowsInView + virtualOverscan * 2);
});

const virtualEndIndex = computed(() => {
  return Math.min(torrents.value.length, virtualStartIndex.value + virtualVisibleCount.value);
});

const visibleVirtualRows = computed(() => {
  return torrents.value.slice(virtualStartIndex.value, virtualEndIndex.value);
});

const virtualTopSpacer = computed(() => virtualStartIndex.value * virtualRowHeight.value);
const virtualBottomSpacer = computed(
  () => (torrents.value.length - virtualEndIndex.value) * virtualRowHeight.value,
);

const tableRows = computed(() => {
  if (showAllTasks.value && useVirtualList.value) {
    return visibleVirtualRows.value;
  }
  return torrents.value;
});

/* 有筛选时的空结果是「没有匹配」而不是「还没有数据」，两者的下一步动作不同 */
const hasActiveFilter = computed(
  () =>
    Boolean(filters.value.search) ||
    filters.value.downloaderId !== "all" ||
    Boolean(filters.value.state) ||
    Boolean(filters.value.category) ||
    Boolean(filters.value.tag),
);

const isEmptyResult = computed(() => !loading.value && torrents.value.length === 0);

const densityOptions = [
  { label: "紧凑", value: "compact" },
  { label: "标准", value: "comfortable" },
];

const detailModeOptions = [
  { label: "侧边", value: "drawer" },
  { label: "下方", value: "inline" },
];

/* 视图范围与渲染方式原先是侧栏里的两个 el-switch，收起侧栏就摸不到，
   所以搬到工具条上；值是布尔，el-segmented 的 modelValue 支持布尔 */
const scopeOptions = [
  { label: "分页", value: false },
  { label: "全部", value: true },
];

const renderOptions = [
  { label: "常规", value: false },
  { label: "虚拟", value: true },
];

function clearFilters() {
  filters.value = { search: "", downloaderId: "all", state: "", category: "", tag: "" };
}

onMounted(async () => {
  await Promise.all([
    loadDownloaders(),
    loadCapabilities(),
    loadMeta(),
    loadTransferStats(),
    loadTorrents(),
  ]);
  await nextTick();
  scheduleTableHeightUpdate();
  refreshTimer = setInterval(() => {
    if (
      autoRefreshEnabled.value &&
      !document.hidden &&
      !isUserInteracting.value &&
      !headerColumnMenuVisible.value &&
      !actionLoading.value &&
      !addLoading.value &&
      !detailLoading.value
    ) {
      silentLoadTorrents();
    }
  }, 5000);
  if (typeof ResizeObserver !== "undefined") {
    resizeObserver = new ResizeObserver(() => {
      scheduleTableHeightUpdate();
    });
    if (hubMainRef.value) {
      resizeObserver.observe(hubMainRef.value);
    }
  }
  window.addEventListener("resize", scheduleTableHeightUpdate);
  window.addEventListener("click", closeHeaderColumnMenu);
  window.addEventListener("scroll", closeHeaderColumnMenu, true);
});

let searchTimer: number | null = null;

/*
 * 五个筛选字段原先各有一个 watch，「清空筛选」一次性重置就会连发五次请求，
 * 所以合成一个深监听：同一 tick 内的多处改动只回调一次。
 * 关键词仍然防抖 320ms（边打字边搜），其余筛选下一 tick 就发。
 */
let lastSearch = filters.value.search;

watch(
  filters,
  () => {
    const searchChanged = filters.value.search !== lastSearch;
    lastSearch = filters.value.search;
    if (searchTimer) {
      window.clearTimeout(searchTimer);
    }
    searchTimer = window.setTimeout(
      () => {
        page.value = 1;
        loadTorrents();
      },
      searchChanged ? 320 : 0,
    );
  },
  { deep: true },
);

watch(
  visibleColumns,
  (value) => {
    localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(value));
    const merged = [...columnOrder.value];
    for (const key of value) {
      if (!merged.includes(key)) {
        merged.push(key);
      }
    }
    columnOrder.value = normalizeColumnOrder(merged);
  },
  { deep: true },
);

watch(
  columnOrder,
  (value) => {
    localStorage.setItem(COLUMN_ORDER_STORAGE_KEY, JSON.stringify(value));
  },
  { deep: true },
);

watch(tableDensity, (value) => {
  localStorage.setItem(DENSITY_STORAGE_KEY, value);
});

watch(detailMode, (value) => {
  localStorage.setItem(DETAIL_MODE_STORAGE_KEY, value);
  if (detail.value) {
    detailDrawerVisible.value = value === "drawer";
    inlineDetailVisible.value = value === "inline";
  }
});

watch(showAllTasks, () => {
  if (!showAllTasks.value && torrents.value.length > pageSize.value) {
    torrents.value = torrents.value.slice(0, pageSize.value);
    restoreSelectedRows(torrents.value);
  }
  page.value = 1;
  loadTorrents();
  scheduleTableHeightUpdate();
});

watch(useVirtualList, () => {
  virtualScrollTop.value = 0;
  if (virtualContainer.value) {
    virtualContainer.value.scrollTop = 0;
  }
  scheduleTableHeightUpdate();
});

watch([selectedCount, heroVisible, sidebarVisible, total, loading], () => {
  scheduleTableHeightUpdate();
});

watch(sidebarWidth, (value) => {
  localStorage.setItem(SIDEBAR_WIDTH_STORAGE_KEY, String(value));
  scheduleTableHeightUpdate();
});

watch(sortBy, () => {
  page.value = 1;
  loadTorrents();
});

watch(sortOrder, () => {
  page.value = 1;
  loadTorrents();
});

onBeforeUnmount(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
  if (resizeObserver) {
    resizeObserver.disconnect();
    resizeObserver = null;
  }
  window.removeEventListener("resize", scheduleTableHeightUpdate);
  window.removeEventListener("click", closeHeaderColumnMenu);
  window.removeEventListener("scroll", closeHeaderColumnMenu, true);
  if (interactionTimer) {
    window.clearTimeout(interactionTimer);
    interactionTimer = null;
  }
  if (heightUpdateTimer) {
    window.clearTimeout(heightUpdateTimer);
    heightUpdateTimer = null;
  }
  stopSidebarResize();
  if (searchTimer) {
    window.clearTimeout(searchTimer);
  }
});

function updateTableMaxHeight() {
  if (!tableCardBodyRef.value) return;
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 0;
  const rect = tableCardBodyRef.value.getBoundingClientRect();
  if (viewportHeight <= 0 || rect.top <= 0) return;
  /* 页脚现在常驻（说明文字 + 分页），两种视图都要把它的高度让出来；
     +12 是 PtPanel 页脚上下各 6 的内边距，量不到但一定占位 */
  const footerHeight = paginationRef.value ? paginationRef.value.offsetHeight + 12 : 0;
  const availableHeight = viewportHeight - rect.top - footerHeight - 14;
  tableMaxHeight.value = Math.max(240, Math.floor(availableHeight));
}

function scheduleTableHeightUpdate() {
  if (pendingHeightUpdate) {
    return;
  }
  pendingHeightUpdate = true;
  if (heightUpdateTimer) {
    return;
  }
  heightUpdateTimer = window.setTimeout(() => {
    heightUpdateTimer = null;
    pendingHeightUpdate = false;
    updateTableMaxHeight();
  }, HEIGHT_UPDATE_THROTTLE_MS);
}

function markUserInteraction() {
  isUserInteracting.value = true;
  if (interactionTimer) {
    window.clearTimeout(interactionTimer);
  }
  interactionTimer = window.setTimeout(() => {
    isUserInteracting.value = false;
    interactionTimer = null;
  }, INTERACTION_IDLE_MS);
}

function clampSidebarWidth(value: number): number {
  return Math.min(SIDEBAR_WIDTH_MAX, Math.max(SIDEBAR_WIDTH_MIN, Math.round(value)));
}

function loadSidebarWidth(): number {
  const raw = localStorage.getItem(SIDEBAR_WIDTH_STORAGE_KEY);
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) {
    return SIDEBAR_WIDTH_DEFAULT;
  }
  return clampSidebarWidth(parsed);
}

function setSidebarWidth(value: number) {
  sidebarWidth.value = clampSidebarWidth(value);
}

function onSidebarResizeStart(event: MouseEvent) {
  if (!sidebarVisible.value) return;
  event.preventDefault();
  markUserInteraction();
  sidebarResizeDragging = true;
  document.body.style.cursor = "col-resize";
  document.body.style.userSelect = "none";
  window.addEventListener("mousemove", onSidebarResizeMove);
  window.addEventListener("mouseup", onSidebarResizeEnd);
}

function onSidebarResizeMove(event: MouseEvent) {
  if (!sidebarResizeDragging || !hubLayoutRef.value) return;
  markUserInteraction();
  const layoutRect = hubLayoutRef.value.getBoundingClientRect();
  const nextWidth = event.clientX - layoutRect.left;
  setSidebarWidth(nextWidth);
}

function onSidebarResizeEnd() {
  stopSidebarResize();
}

function stopSidebarResize() {
  if (!sidebarResizeDragging) return;
  sidebarResizeDragging = false;
  document.body.style.cursor = "";
  document.body.style.userSelect = "";
  window.removeEventListener("mousemove", onSidebarResizeMove);
  window.removeEventListener("mouseup", onSidebarResizeEnd);
}

function openHeaderColumnMenu(payload: { x: number; y: number }) {
  markUserInteraction();
  hideRowContextMenuToken.value += 1;
  const padding = 8;
  headerColumnMenuX.value = payload.x;
  headerColumnMenuY.value = payload.y;
  headerColumnMenuVisible.value = true;
  nextTick(() => {
    if (!headerColumnMenuRef.value) return;
    const menuRect = headerColumnMenuRef.value.getBoundingClientRect();
    const maxX = window.innerWidth - menuRect.width - padding;
    const maxY = window.innerHeight - menuRect.height - padding;
    headerColumnMenuX.value = Math.max(padding, Math.min(maxX, payload.x));
    headerColumnMenuY.value = Math.max(padding, Math.min(maxY, payload.y));
  });
}

function closeHeaderColumnMenu() {
  headerColumnMenuVisible.value = false;
}

function onRowContextMenuOpen() {
  markUserInteraction();
  closeHeaderColumnMenu();
}

async function loadDownloaders() {
  try {
    downloaders.value = await downloadersApi.list();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "下载器列表加载失败");
  }
}

async function silentLoadTorrents() {
  try {
    const requestedShowAll = showAllTasks.value;
    const params = new URLSearchParams();
    params.set("page", String(page.value));
    params.set("page_size", requestedShowAll ? "0" : String(pageSize.value));
    if (filters.value.search.trim()) {
      params.set("search", filters.value.search.trim());
    }
    if (filters.value.downloaderId !== "all") {
      params.set("downloader_id", filters.value.downloaderId);
    }
    if (filters.value.state) {
      params.set("state", normalizeStateFilter(filters.value.state));
    }
    if (filters.value.category) {
      params.set("category", filters.value.category);
    }
    if (filters.value.tag) {
      params.set("tag", filters.value.tag);
    }
    params.set("sort_by", sortBy.value);
    params.set("sort_order", sortOrder.value);
    const resp = await downloaderTorrentsApi.list(params);
    if (requestedShowAll !== showAllTasks.value) {
      return;
    }
    const rows = limitRowsForSafety(resp.items, requestedShowAll);
    if (!isSameTorrentSnapshot(torrents.value, resp.items)) {
      torrents.value = rows;
      total.value = resp.total;
      restoreSelectedRows(rows);
    }
    autoRefreshTick += 1;
    if (autoRefreshTick % 6 === 0) {
      loadMeta();
      loadTransferStats();
    }
  } catch {
    /* silent */
  }
}

async function loadTorrents() {
  loading.value = true;
  try {
    const requestedShowAll = showAllTasks.value;
    const params = new URLSearchParams();
    params.set("page", String(page.value));
    params.set("page_size", requestedShowAll ? "0" : String(pageSize.value));
    if (filters.value.search.trim()) {
      params.set("search", filters.value.search.trim());
    }
    if (filters.value.downloaderId !== "all") {
      params.set("downloader_id", filters.value.downloaderId);
    }
    if (filters.value.state) {
      params.set("state", normalizeStateFilter(filters.value.state));
    }
    params.set("sort_by", sortBy.value);
    params.set("sort_order", sortOrder.value);
    if (filters.value.category) {
      params.set("category", filters.value.category);
    }
    if (filters.value.tag) {
      params.set("tag", filters.value.tag);
    }

    const resp = await downloaderTorrentsApi.list(params);
    if (requestedShowAll !== showAllTasks.value) {
      return;
    }
    const rows = limitRowsForSafety(resp.items, requestedShowAll);
    torrents.value = rows;
    total.value = resp.total;
    restoreSelectedRows(rows);
    virtualScrollTop.value = 0;
    if (virtualContainer.value) {
      virtualContainer.value.scrollTop = 0;
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "任务加载失败");
  } finally {
    loading.value = false;
  }
}

function normalizeTorrentState(state: string): string {
  const normalized = String(state || "").toLowerCase();
  if (normalized.includes("error")) return "error";
  if (normalized.includes("download")) return "downloading";
  if (normalized.includes("seed")) return "seeding";
  if (normalized.includes("pause")) return "paused";
  if (normalized.includes("stop")) return "stopped";
  if (normalized.includes("check")) return "checking";
  if (normalized.includes("queue")) return "queued";
  return normalized;
}

function normalizeStateFilter(state: string): string {
  return state;
}

function limitRowsForSafety(
  rows: DownloaderTorrentItem[],
  isAllMode: boolean,
): DownloaderTorrentItem[] {
  if (!isAllMode) {
    allTasksLimited.value = false;
    return rows;
  }
  if (rows.length <= MAX_ALL_TASK_ROWS) {
    allTasksLimited.value = false;
    return rows;
  }
  allTasksLimited.value = true;
  return rows.slice(0, MAX_ALL_TASK_ROWS);
}

function rowSelectionKey(row: DownloaderTorrentItem): string {
  return `${row.downloader_id}:${row.task_id}`;
}

function restoreSelectedRows(nextRows: DownloaderTorrentItem[]) {
  if (selectedRowKeys.value.length === 0) {
    selectedRows.value = [];
    return;
  }
  const selectedKeys = new Set(selectedRowKeys.value);
  selectedRows.value = nextRows.filter((row) => selectedKeys.has(rowSelectionKey(row)));
}

async function loadMeta() {
  try {
    const resp = await downloaderTorrentsApi.meta();
    allCategories.value = resp.categories || [];
    allTags.value = resp.tags || [];
  } catch {
    /* silent */
  }
}
async function loadTransferStats() {
  try {
    transferStats.value = await downloaderTorrentsApi.transferStats();
  } catch {
    /* silent */
  }
}

async function loadCapabilities() {
  try {
    const resp = await downloaderTorrentsApi.capabilities();
    capabilities.value = resp.items || [];
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "能力信息加载失败");
  }
}

function onSelectionChange(rows: DownloaderTorrentItem[]) {
  selectedRows.value = rows;
  selectedRowKeys.value = rows.map((row) => rowSelectionKey(row));
}

function onSelectionKeysChange(keys: string[]) {
  selectedRowKeys.value = keys;
  restoreSelectedRows(torrents.value);
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return `${size.toFixed(2)} ${units[i]}`;
}

function handleSortChange(payload: { prop: string; order: "ascending" | "descending" | null }) {
  if (!payload.order || !payload.prop) {
    sortBy.value = "added_at";
    sortOrder.value = "desc";
  } else {
    sortBy.value = payload.prop;
    sortOrder.value = payload.order === "ascending" ? "asc" : "desc";
  }
}

function toggleSortOrder() {
  sortOrder.value = sortOrder.value === "asc" ? "desc" : "asc";
}

function applyQuickState(state: string) {
  filters.value.state = state;
}

function loadVisibleColumns(): string[] {
  const raw = localStorage.getItem(COLUMN_STORAGE_KEY);
  if (!raw) {
    return [...DEFAULT_VISIBLE_COLUMNS];
  }
  try {
    const parsed = JSON.parse(raw) as string[];
    if (!Array.isArray(parsed) || parsed.length === 0) {
      return [...DEFAULT_VISIBLE_COLUMNS];
    }
    return parsed;
  } catch {
    return [...DEFAULT_VISIBLE_COLUMNS];
  }
}

function loadColumnOrder(): string[] {
  const raw = localStorage.getItem(COLUMN_ORDER_STORAGE_KEY);
  if (!raw) {
    return [...ALL_COLUMN_KEYS];
  }
  try {
    const parsed = JSON.parse(raw) as string[];
    if (!Array.isArray(parsed) || parsed.length === 0) {
      return [...ALL_COLUMN_KEYS];
    }
    return normalizeColumnOrder(parsed);
  } catch {
    return [...ALL_COLUMN_KEYS];
  }
}

function normalizeColumnOrder(input: string[]): string[] {
  const unique = new Set(input);
  const result: string[] = [];
  for (const key of ALL_COLUMN_KEYS) {
    if (unique.has(key)) {
      result.push(key);
      unique.delete(key);
    }
  }
  for (const key of unique) {
    if (ALL_COLUMN_KEYS.includes(key)) {
      result.push(key);
    }
  }
  return result;
}

function getColumnLabel(columnKey: string): string {
  return columnOptions.find((item) => item.value === columnKey)?.label || columnKey;
}

function restoreDefaultColumns() {
  visibleColumns.value = [...DEFAULT_VISIBLE_COLUMNS];
  columnOrder.value = [...ALL_COLUMN_KEYS];
}

function onColumnDragStart(columnKey: string) {
  draggingColumn.value = columnKey;
}

function onColumnDrop(targetKey: string) {
  const sourceKey = draggingColumn.value;
  draggingColumn.value = null;
  if (!sourceKey || sourceKey === targetKey) return;

  const sourceIndex = columnOrder.value.indexOf(sourceKey);
  const targetIndex = columnOrder.value.indexOf(targetKey);
  if (sourceIndex === -1 || targetIndex === -1) return;

  const next = [...columnOrder.value];
  next.splice(sourceIndex, 1);
  next.splice(targetIndex, 0, sourceKey);
  columnOrder.value = next;
}

function saveLayoutPreset() {
  const preset: LayoutPreset = {
    visibleColumns: [...visibleColumns.value],
    columnOrder: [...columnOrder.value],
    density: tableDensity.value,
    sortBy: sortBy.value,
    sortOrder: sortOrder.value,
  };
  localStorage.setItem(LAYOUT_PRESET_STORAGE_KEY, JSON.stringify(preset));
  ElMessage.success("布局已保存");
}

function loadLayoutPreset() {
  const raw = localStorage.getItem(LAYOUT_PRESET_STORAGE_KEY);
  if (!raw) {
    ElMessage.warning("还没有保存的布局");
    return;
  }
  try {
    const preset = JSON.parse(raw) as LayoutPreset;
    visibleColumns.value =
      Array.isArray(preset.visibleColumns) && preset.visibleColumns.length > 0
        ? preset.visibleColumns
        : [...DEFAULT_VISIBLE_COLUMNS];
    columnOrder.value =
      Array.isArray(preset.columnOrder) && preset.columnOrder.length > 0
        ? normalizeColumnOrder(preset.columnOrder)
        : [...DEFAULT_VISIBLE_COLUMNS];
    tableDensity.value = preset.density === "comfortable" ? "comfortable" : "compact";
    sortBy.value = preset.sortBy || "added_at";
    sortOrder.value = preset.sortOrder === "asc" ? "asc" : "desc";
    ElMessage.success("布局已加载");
  } catch {
    ElMessage.error("布局数据损坏，加载失败");
  }
}

function loadDensity(): "compact" | "comfortable" {
  const raw = localStorage.getItem(DENSITY_STORAGE_KEY);
  return raw === "comfortable" ? "comfortable" : "compact";
}

function loadDetailMode(): "drawer" | "inline" {
  const raw = localStorage.getItem(DETAIL_MODE_STORAGE_KEY);
  return raw === "inline" ? "inline" : "drawer";
}

function onVirtualScroll(event: Event) {
  markUserInteraction();
  const target = event.target as HTMLElement;
  virtualScrollTop.value = target.scrollTop;
}

function isSameTorrentSnapshot(
  current: DownloaderTorrentItem[],
  next: DownloaderTorrentItem[],
): boolean {
  if (current.length !== next.length) {
    return false;
  }
  for (let i = 0; i < current.length; i += 1) {
    const a = current[i];
    const b = next[i];
    if (!a || !b) {
      return false;
    }
    if (
      a.downloader_id !== b.downloader_id ||
      a.task_id !== b.task_id ||
      a.progress !== b.progress ||
      a.state !== b.state ||
      a.upload_speed !== b.upload_speed ||
      a.download_speed !== b.download_speed ||
      a.seeds !== b.seeds ||
      a.connections !== b.connections
    ) {
      return false;
    }
  }
  return true;
}

function targetsFromSelection(): TorrentActionTarget[] {
  return selectedRows.value.map((row) => ({
    downloader_id: row.downloader_id,
    task_id: row.task_id,
  }));
}

function capabilityByDownloader(downloaderId: number): DownloaderCapability | undefined {
  return capabilities.value.find((item) => item.downloader_id === downloaderId);
}

function selectedCanUse(
  action: "pause" | "resume" | "delete" | "delete_with_files" | "set_location" | "recheck",
): boolean {
  if (selectedRows.value.length === 0) return false;
  return selectedRows.value.every((row) => {
    const cap = capabilityByDownloader(row.downloader_id);
    if (!cap) return false;
    switch (action) {
      case "pause":
        return cap.can_pause;
      case "resume":
        return cap.can_resume;
      case "delete":
        return cap.can_delete;
      case "delete_with_files":
        return cap.can_delete_with_data;
      case "set_location":
        return cap.can_set_location;
      case "recheck":
        return cap.can_recheck;
      default:
        return false;
    }
  });
}

async function batchAction(
  action: "pause" | "resume" | "delete" | "delete_with_files" | "recheck",
) {
  if (selectedCount.value === 0) {
    ElMessage.warning("请先选择任务");
    return;
  }
  actionLoading.value = true;
  try {
    const resp = await downloaderTorrentsApi.batchAction({
      action,
      targets: targetsFromSelection(),
    });
    ElMessage.success(`完成: 成功 ${resp.success_count}，失败 ${resp.failed_count}`);
    await loadTorrents();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "批量操作失败");
  } finally {
    actionLoading.value = false;
  }
}

function openSetLocationDialog() {
  if (selectedCount.value === 0) {
    ElMessage.warning("请先选择任务");
    return;
  }
  newLocation.value = "";
  locationDialogVisible.value = true;
}

async function submitSetLocation() {
  if (!newLocation.value.trim()) {
    ElMessage.warning("请输入新存储路径");
    return;
  }
  actionLoading.value = true;
  try {
    const resp = await downloaderTorrentsApi.batchAction({
      action: "set_location",
      save_path: newLocation.value.trim(),
      targets: targetsFromSelection(),
    });
    ElMessage.success(`路径修改完成: 成功 ${resp.success_count}，失败 ${resp.failed_count}`);
    locationDialogVisible.value = false;
    await loadTorrents();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "修改路径失败");
  } finally {
    actionLoading.value = false;
  }
}

function openAddDialog() {
  addForm.value = {
    downloaderIds: [],
    sourceUrl: "",
    magnetLink: "",
    savePath: "",
    category: "",
    tags: "",
    addPaused: false,
  };
  uploadFileBase64.value = "";
  uploadFileName.value = "";
  addDialogVisible.value = true;
}

function beforeUpload(file: File): boolean {
  const reader = new FileReader();
  reader.onload = () => {
    const value = String(reader.result || "");
    const index = value.indexOf(",");
    uploadFileBase64.value = index > -1 ? value.slice(index + 1) : value;
    uploadFileName.value = file.name;
  };
  reader.readAsDataURL(file);
  return false;
}

async function submitAdd() {
  if (!addForm.value.sourceUrl.trim() && !addForm.value.magnetLink.trim()) {
    if (!uploadFileBase64.value) {
      ElMessage.warning("请输入种子 URL / 磁力链接，或上传 .torrent 文件");
      return;
    }
  }
  if (!selectedCanAdd()) {
    ElMessage.warning("当前目标下载器不支持添加种子");
    return;
  }
  addLoading.value = true;
  try {
    const resp = await downloaderTorrentsApi.add({
      downloader_ids: addForm.value.downloaderIds,
      source_url: addForm.value.sourceUrl.trim(),
      magnet_link: addForm.value.magnetLink.trim(),
      torrent_base64: uploadFileBase64.value,
      save_path: addForm.value.savePath.trim(),
      category: addForm.value.category.trim(),
      tags: addForm.value.tags.trim(),
      add_paused: addForm.value.addPaused,
    });
    ElMessage.success(`添加完成: 成功 ${resp.success_count}，失败 ${resp.failed_count}`);
    addDialogVisible.value = false;
    await loadTorrents();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "添加任务失败");
  } finally {
    addLoading.value = false;
  }
}

function selectedCanAdd(): boolean {
  if (addForm.value.downloaderIds.length === 0) return true;
  return addForm.value.downloaderIds.every((id) => {
    const cap = capabilityByDownloader(id);
    return !!cap?.can_add_torrent;
  });
}

async function openDetail(row: DownloaderTorrentItem) {
  const currentRequestId = ++detailRequestId;
  detailActiveTab.value = "files";
  detail.value = null;
  detailLoading.value = true;
  detailDrawerVisible.value = detailMode.value === "drawer";
  inlineDetailVisible.value = detailMode.value === "inline";
  await nextTick();
  try {
    const detailResp = await downloaderTorrentsApi.detail(row.downloader_id, row.task_id);
    if (currentRequestId !== detailRequestId) {
      return;
    }
    detail.value = detailResp;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "任务详情加载失败");
    detail.value = null;
  } finally {
    if (currentRequestId === detailRequestId) {
      detailLoading.value = false;
    }
  }
}

function closeInlineDetail() {
  inlineDetailVisible.value = false;
}

async function handleContextAction(payload: {
  action:
    | "pause"
    | "resume"
    | "delete"
    | "delete_with_files"
    | "recheck"
    | "detail"
    | "set_category"
    | "set_tags";
  row: DownloaderTorrentItem;
}) {
  if (payload.action === "detail") {
    await openDetail(payload.row);
    return;
  }
  if (payload.action === "set_category" || payload.action === "set_tags") {
    ElMessage.info(
      `${payload.action === "set_category" ? "设置分类" : "设置标签"} - ${payload.row.title} (暂未实现后端接口)`,
    );
    return;
  }

  actionLoading.value = true;
  try {
    const resp = await downloaderTorrentsApi.batchAction({
      action: payload.action,
      targets: [{ downloader_id: payload.row.downloader_id, task_id: payload.row.task_id }],
    });
    ElMessage.success(`右键操作完成: 成功 ${resp.success_count}，失败 ${resp.failed_count}`);
    await loadTorrents();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "右键操作失败");
  } finally {
    actionLoading.value = false;
  }
}

function toggleSidebar() {
  sidebarVisible.value = !sidebarVisible.value;
  localStorage.setItem(SIDEBAR_VISIBLE_STORAGE_KEY, String(sidebarVisible.value));
}

function toggleHero() {
  heroVisible.value = !heroVisible.value;
  localStorage.setItem(HERO_VISIBLE_STORAGE_KEY, String(heroVisible.value));
}

/* 沉浸模式下全局导航被隐藏了，工具条上这颗下拉是本页唯一的出口 */
function onNavCommand(command: string) {
  if (command === "toggle-sidebar") {
    toggleSidebar();
    return;
  }
  if (command === "toggle-hero") {
    toggleHero();
    return;
  }
  router.push(`/${command}`);
}
</script>

<template>
  <div class="hub">
    <header v-if="heroVisible" class="hub__hero">
      <PtLogo :size="30" variant="mono" title="pt-tools" />
      <div class="hub__hero-text">
        <h1>混合下载器控制台</h1>
        <p>聚合所有下载器任务，支持单下载器与全局视图。</p>
      </div>
      <div class="hub__hero-acts">
        <el-button type="primary" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加种子</span>
        </el-button>
        <el-button @click="toggleHero">
          <PtIcon name="chevron-up" :size="14" /><span>收起</span>
        </el-button>
      </div>
    </header>

    <div ref="hubLayoutRef" class="hub__layout">
      <aside v-show="sidebarVisible" class="hub__side" :style="sidebarStyle">
        <div class="hub__side-head">
          <PtIcon name="list-filter" :size="14" />
          <span>筛选</span>
          <el-tooltip content="收起侧栏" placement="right">
            <button type="button" class="hub__ico" @click="toggleSidebar">
              <PtIcon name="panel-left-close" :size="14" />
            </button>
          </el-tooltip>
        </div>
        <section class="hub__sec">
          <h3 class="hub__sec-t">实时速度</h3>
          <div class="hub__stat">
            <PtIcon name="download" :size="14" class="is-dl" />
            <span class="hub__stat-l">下载</span>
            <span class="hub__stat-v"
              >{{ formatSize(transferStats?.total_download_speed || 0) }}/s</span
            >
          </div>
          <div class="hub__stat">
            <PtIcon name="upload" :size="14" class="is-ul" />
            <span class="hub__stat-l">上传</span>
            <span class="hub__stat-v"
              >{{ formatSize(transferStats?.total_upload_speed || 0) }}/s</span
            >
          </div>
        </section>
        <section class="hub__sec">
          <h3 class="hub__sec-t">本次会话</h3>
          <div class="hub__stat">
            <PtIcon name="cloud-download" :size="14" class="is-dl" />
            <span class="hub__stat-l">下载</span>
            <span class="hub__stat-v">{{
              formatSize(transferStats?.total_session_downloaded || 0)
            }}</span>
          </div>
          <div class="hub__stat">
            <PtIcon name="cloud-upload" :size="14" class="is-ul" />
            <span class="hub__stat-l">上传</span>
            <span class="hub__stat-v">{{
              formatSize(transferStats?.total_session_uploaded || 0)
            }}</span>
          </div>
        </section>
        <section class="hub__sec">
          <h3 class="hub__sec-t">累计</h3>
          <div class="hub__stat">
            <PtIcon name="cloud-download" :size="14" class="is-dl" />
            <span class="hub__stat-l">下载</span>
            <span class="hub__stat-v">{{ formatSize(transferStats?.total_downloaded || 0) }}</span>
          </div>
          <div class="hub__stat">
            <PtIcon name="cloud-upload" :size="14" class="is-ul" />
            <span class="hub__stat-l">上传</span>
            <span class="hub__stat-v">{{ formatSize(transferStats?.total_uploaded || 0) }}</span>
          </div>
          <div class="hub__stat">
            <PtIcon name="hard-drive" :size="14" />
            <span class="hub__stat-l">剩余空间</span>
            <span class="hub__stat-v">{{ formatSize(transferStats?.total_free_space || 0) }}</span>
          </div>
        </section>
        <section class="hub__sec">
          <h3 class="hub__sec-t">状态</h3>
          <button
            type="button"
            class="hub__pick"
            :class="{ 'is-on': filters.state === '' }"
            @click="applyQuickState('')">
            <PtIcon name="list" :size="14" />
            <span class="hub__pick-l">全部</span>
            <span class="hub__pick-n">{{ total }}</span>
          </button>
          <button
            type="button"
            class="hub__pick is-dl"
            :class="{ 'is-on': filters.state === 'downloading' }"
            @click="applyQuickState('downloading')">
            <PtIcon name="download" :size="14" />
            <span class="hub__pick-l">下载中</span>
            <span class="hub__pick-n">{{ downloadingCount }}</span>
          </button>
          <button
            type="button"
            class="hub__pick is-seed"
            :class="{ 'is-on': filters.state === 'seeding' }"
            @click="applyQuickState('seeding')">
            <PtIcon name="upload" :size="14" />
            <span class="hub__pick-l">做种中</span>
            <span class="hub__pick-n">{{ seedingCount }}</span>
          </button>
          <button
            type="button"
            class="hub__pick is-pause"
            :class="{ 'is-on': filters.state === 'paused' }"
            @click="applyQuickState('paused')">
            <PtIcon name="pause" :size="14" />
            <span class="hub__pick-l">暂停</span>
            <span class="hub__pick-n">{{ pausedCount }}</span>
          </button>
          <button
            type="button"
            class="hub__pick is-pause"
            :class="{ 'is-on': filters.state === 'stopped' }"
            @click="applyQuickState('stopped')">
            <PtIcon name="circle-pause" :size="14" />
            <span class="hub__pick-l">已停止</span>
            <span class="hub__pick-n">{{ stoppedCount }}</span>
          </button>
          <button
            type="button"
            class="hub__pick is-err"
            :class="{ 'is-on': filters.state === 'error' }"
            @click="applyQuickState('error')">
            <PtIcon name="triangle-alert" :size="14" />
            <span class="hub__pick-l">错误</span>
            <span class="hub__pick-n">{{ errorCount }}</span>
          </button>
        </section>
        <section class="hub__sec">
          <h3 class="hub__sec-t">下载器</h3>
          <el-select
            v-model="filters.downloaderId"
            size="small"
            style="width: 100%"
            placeholder="全部下载器">
            <el-option
              v-for="item in downloaderOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value" />
          </el-select>
        </section>
        <section v-if="allCategories.length > 0" class="hub__sec">
          <h3 class="hub__sec-t">分类</h3>
          <button
            type="button"
            class="hub__pick"
            :class="{ 'is-on': filters.category === '' }"
            @click="filters.category = ''">
            <PtIcon name="folder-open" :size="14" />
            <span class="hub__pick-l">全部</span>
          </button>
          <button
            v-for="cat in allCategories"
            :key="cat"
            type="button"
            class="hub__pick"
            :class="{ 'is-on': filters.category === cat }"
            @click="filters.category = filters.category === cat ? '' : cat">
            <PtIcon name="folder" :size="14" />
            <span class="hub__pick-l">{{ cat }}</span>
          </button>
        </section>
        <section v-if="allTags.length > 0" class="hub__sec">
          <h3 class="hub__sec-t">标签</h3>
          <div class="hub__tags">
            <button
              v-for="tag in allTags"
              :key="tag"
              type="button"
              class="hub__tag"
              :class="{ 'is-on': filters.tag === tag }"
              @click="filters.tag = filters.tag === tag ? '' : tag">
              <PtIcon name="tag" :size="12" />
              <span>{{ tag }}</span>
            </button>
          </div>
        </section>
        <div class="hub__side-foot">
          <el-tooltip content="保存当前布局" placement="top">
            <button type="button" class="hub__ico" @click="saveLayoutPreset">
              <PtIcon name="save" :size="15" />
            </button>
          </el-tooltip>
          <el-tooltip content="载入已保存布局" placement="top">
            <button type="button" class="hub__ico" @click="loadLayoutPreset">
              <PtIcon name="folder-open" :size="15" />
            </button>
          </el-tooltip>
          <el-tooltip content="立即刷新" placement="top">
            <button type="button" class="hub__ico" @click="loadTorrents">
              <PtIcon name="refresh-cw" :size="15" />
            </button>
          </el-tooltip>
          <el-tooltip content="添加种子" placement="top">
            <button type="button" class="hub__ico is-primary" @click="openAddDialog">
              <PtIcon name="plus" :size="15" />
            </button>
          </el-tooltip>
        </div>
      </aside>
      <div
        v-show="sidebarVisible"
        class="hub__resizer"
        title="拖拽调整侧栏宽度"
        @mousedown="onSidebarResizeStart" />

      <div ref="hubMainRef" class="hub__main">
        <PtPanel
          v-loading="tableLoading"
          element-loading-text="加载任务…"
          class="hub__panel"
          title="任务列表"
          icon="list-checks"
          :count="total"
          padding="none">
          <template #actions>
            <el-tooltip content="每 5 秒自动刷新" placement="bottom">
              <span class="hub__auto">
                <el-switch v-model="autoRefreshEnabled" size="small" />
                <span>自动</span>
              </span>
            </el-tooltip>
            <el-button type="primary" size="small" @click="openAddDialog">
              <PtIcon name="plus" :size="14" /><span>添加种子</span>
            </el-button>
          </template>

          <PtToolbar>
            <el-dropdown trigger="click" @command="onNavCommand">
              <button type="button" class="hub__nav">
                <PtLogo :size="16" :variant="logoVariant" title="pt-tools" />
                <PtIcon name="chevron-down" :size="13" />
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="toggle-sidebar">{{
                    sidebarVisible ? "隐藏侧栏" : "显示侧栏"
                  }}</el-dropdown-item>
                  <el-dropdown-item command="toggle-hero">{{
                    heroVisible ? "隐藏页头" : "显示页头"
                  }}</el-dropdown-item>
                  <el-dropdown-item divided command="global">全局设置</el-dropdown-item>
                  <el-dropdown-item command="userinfo">用户统计</el-dropdown-item>
                  <el-dropdown-item command="downloaders">下载器管理</el-dropdown-item>
                  <el-dropdown-item command="sites">站点与 RSS</el-dropdown-item>
                  <el-dropdown-item command="search">种子搜索</el-dropdown-item>
                  <el-dropdown-item command="tasks">任务列表</el-dropdown-item>
                  <el-dropdown-item command="logs">日志</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-tooltip v-if="!sidebarVisible" content="展开侧栏" placement="bottom">
              <button type="button" class="hub__ico" @click="toggleSidebar">
                <PtIcon name="panel-left-open" :size="15" />
              </button>
            </el-tooltip>
            <el-input
              v-model="filters.search"
              placeholder="搜索标题"
              clearable
              size="small"
              class="hub__search">
              <template #prefix><PtIcon name="search" :size="13" /></template>
            </el-input>
            <el-select v-model="sortBy" size="small" class="hub__sort">
              <el-option
                v-for="item in sortOptions"
                :key="item.value"
                :label="item.label"
                :value="item.value" />
            </el-select>
            <el-tooltip :content="sortOrder === 'asc' ? '升序，点击改降序' : '降序，点击改升序'">
              <button type="button" class="hub__ico" @click="toggleSortOrder">
                <PtIcon :name="sortOrder === 'asc' ? 'chevron-up' : 'chevron-down'" :size="15" />
              </button>
            </el-tooltip>

            <template #right>
              <el-tooltip content="行高" placement="bottom">
                <el-segmented v-model="tableDensity" class="pt-seg" :options="densityOptions" />
              </el-tooltip>
              <el-tooltip content="详情展示位置" placement="bottom">
                <el-segmented v-model="detailMode" class="pt-seg" :options="detailModeOptions" />
              </el-tooltip>
              <el-tooltip content="数据范围" placement="bottom">
                <el-segmented v-model="showAllTasks" class="pt-seg" :options="scopeOptions" />
              </el-tooltip>
              <el-tooltip content="列表渲染方式，仅全部范围可选" placement="bottom">
                <el-segmented
                  v-model="useVirtualList"
                  class="pt-seg"
                  :options="renderOptions"
                  :disabled="!showAllTasks" />
              </el-tooltip>
              <el-popover placement="bottom-end" :width="264" trigger="click">
                <template #reference>
                  <button type="button" class="hub__ico" aria-label="显示列">
                    <PtIcon name="columns-3" :size="15" />
                  </button>
                </template>
                <div class="hub__cols-head">
                  <span>显示列</span>
                  <el-button text type="primary" size="small" @click="restoreDefaultColumns">
                    恢复默认
                  </el-button>
                </div>
                <el-checkbox-group v-model="visibleColumns" class="hub__cols">
                  <el-checkbox
                    v-for="item in columnOptions"
                    :key="item.value"
                    :value="item.value"
                    :label="item.label" />
                </el-checkbox-group>
                <div class="hub__cols-head">
                  <span>拖拽排序</span>
                </div>
                <div class="hub__order">
                  <div
                    v-for="columnKey in columnOrder"
                    :key="columnKey"
                    class="hub__order-item"
                    :class="{ 'is-off': !visibleColumns.includes(columnKey) }"
                    draggable="true"
                    @dragstart="onColumnDragStart(columnKey)"
                    @dragover.prevent
                    @drop="onColumnDrop(columnKey)">
                    <PtIcon name="grip-vertical" :size="13" />
                    <span>{{ getColumnLabel(columnKey) }}</span>
                  </div>
                </div>
              </el-popover>
            </template>
          </PtToolbar>

          <div v-show="selectedCount > 0" class="pt-strip hub__bulk">
            <PtIcon name="check-check" :size="14" />
            <span>已选 {{ selectedCount }} 项</span>
            <span class="pt-strip__end hub__bulk-acts">
              <el-button
                size="small"
                :loading="actionLoading"
                :disabled="!selectedCanUse('pause')"
                @click="batchAction('pause')">
                <PtIcon name="pause" :size="13" /><span>暂停</span>
              </el-button>
              <el-button
                size="small"
                :loading="actionLoading"
                :disabled="!selectedCanUse('resume')"
                @click="batchAction('resume')">
                <PtIcon name="play" :size="13" /><span>开始</span>
              </el-button>
              <el-button
                size="small"
                :loading="actionLoading"
                :disabled="!selectedCanUse('recheck')"
                @click="batchAction('recheck')">
                <PtIcon name="refresh-cw" :size="13" /><span>复检</span>
              </el-button>
              <el-button
                size="small"
                :loading="actionLoading"
                :disabled="!selectedCanUse('set_location')"
                @click="openSetLocationDialog">
                <PtIcon name="move" :size="13" /><span>路径</span>
              </el-button>
              <el-button
                size="small"
                type="danger"
                :loading="actionLoading"
                :disabled="!selectedCanUse('delete')"
                @click="batchAction('delete')">
                <PtIcon name="trash-2" :size="13" /><span>删除</span>
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :loading="actionLoading"
                :disabled="!selectedCanUse('delete_with_files')"
                @click="batchAction('delete_with_files')">
                <PtIcon name="trash-2" :size="13" /><span>删除+文件</span>
              </el-button>
            </span>
          </div>

          <div ref="tableCardBodyRef" class="hub__table">
            <PtDataState
              v-if="isEmptyResult"
              :state="hasActiveFilter ? 'zero' : 'empty'"
              :title="hasActiveFilter ? '没有匹配的任务' : '下载器里还没有任务'"
              :sub="
                hasActiveFilter ? '试试放宽状态、分类或关键词' : '添加一个种子，任务会出现在这里'
              ">
              <template #action>
                <el-button v-if="hasActiveFilter" size="small" @click="clearFilters">
                  <PtIcon name="rotate-ccw" :size="14" /><span>清空筛选</span>
                </el-button>
                <el-button v-else type="primary" size="small" @click="openAddDialog">
                  <PtIcon name="plus" :size="14" /><span>添加种子</span>
                </el-button>
              </template>
            </PtDataState>
            <div v-else class="hub__grid" @contextmenu.prevent>
              <div
                v-if="showAllTasks && useVirtualList"
                ref="virtualContainer"
                class="hub__vshell"
                :style="{ maxHeight: `${tableMaxHeight}px` }"
                @scroll.passive="onVirtualScroll">
                <div :style="{ height: `${virtualTopSpacer}px` }" />
                <DownloaderTorrentVirtualTable
                  :data="tableRows"
                  :all-data="torrents"
                  :selected-row-keys="selectedRowKeys"
                  :visible-columns="visibleColumns"
                  :column-order="columnOrder"
                  :density="tableDensity"
                  :sort-by="sortBy"
                  :sort-order="sortOrder"
                  :hide-context-menu-token="hideRowContextMenuToken"
                  @selection-change="onSelectionChange"
                  @selection-keys-change="onSelectionKeysChange"
                  @sort-change="handleSortChange"
                  @header-contextmenu="openHeaderColumnMenu"
                  @row-contextmenu-open="onRowContextMenuOpen"
                  @context-action="handleContextAction"
                  @detail="openDetail" />
                <div :style="{ height: `${virtualBottomSpacer}px` }" />
              </div>
              <DownloaderTorrentTable
                v-else
                :data="tableRows"
                :visible-columns="visibleColumns"
                :column-order="columnOrder"
                :density="tableDensity"
                :max-height="tableMaxHeight"
                :hide-context-menu-token="hideRowContextMenuToken"
                @selection-change="onSelectionChange"
                @sort-change="handleSortChange"
                @header-contextmenu="openHeaderColumnMenu"
                @row-contextmenu-open="onRowContextMenuOpen"
                @context-action="handleContextAction"
                @detail="openDetail" />
            </div>
          </div>

          <template #footer>
            <div ref="paginationRef" class="hub__foot">
              <span class="pt-foot-note">
                共 {{ total }} 个任务<template v-if="allTasksLimited">
                  ·
                  <span class="hub__warn">仅渲染前 {{ MAX_ALL_TASK_ROWS }} 条（防卡死）</span>
                </template>
              </span>
              <el-pagination
                v-if="!showAllTasks"
                v-model:current-page="page"
                v-model:page-size="pageSize"
                class="pt-pager"
                :total="total"
                :page-sizes="[50, 100, 200]"
                layout="sizes, prev, pager, next, jumper"
                @size-change="loadTorrents"
                @current-change="loadTorrents" />
            </div>
          </template>
        </PtPanel>
        <teleport to="body">
          <div
            v-if="headerColumnMenuVisible"
            ref="headerColumnMenuRef"
            class="hub__hmenu"
            :style="{ left: `${headerColumnMenuX}px`, top: `${headerColumnMenuY}px` }"
            @click.stop>
            <div class="hub__cols-head">
              <span>显示列</span>
              <el-button text type="primary" size="small" @click="restoreDefaultColumns">
                恢复默认
              </el-button>
            </div>
            <el-checkbox-group v-model="visibleColumns" class="hub__cols">
              <el-checkbox
                v-for="item in columnOptions"
                :key="item.value"
                :value="item.value"
                :label="item.label" />
            </el-checkbox-group>
          </div>
        </teleport>
        <PtPanel v-if="inlineDetailVisible" class="hub__detail" title="任务详情" icon="info">
          <template #actions>
            <el-button text size="small" @click="closeInlineDetail">
              <PtIcon name="x" :size="14" /><span>关闭</span>
            </el-button>
          </template>
          <DownloaderTorrentDetail
            v-model:tab="detailActiveTab"
            :detail="detail"
            :loading="detailLoading" />
        </PtPanel>
      </div>
    </div>

    <el-dialog
      v-model="locationDialogVisible"
      class="pt-dialog"
      title="批量修改存储路径"
      width="480px">
      <el-input v-model="newLocation" placeholder="例如 /downloads/tv" />
      <template #footer>
        <el-button @click="locationDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="actionLoading" @click="submitSetLocation">
          确定修改
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="addDialogVisible" class="pt-dialog" title="添加种子到下载器" width="620px">
      <el-form class="pt-form" label-position="top">
        <el-form-item label="下载器">
          <el-select v-model="addForm.downloaderIds" multiple clearable collapse-tags>
            <el-option
              v-for="item in downloaders"
              :key="item.id"
              :label="`${item.name} (${item.type})`"
              :value="item.id" />
          </el-select>
          <div class="field-tip">留空则不提交；可同时投递到多个下载器。</div>
        </el-form-item>
        <div class="field-row">
          <el-form-item label="种子 URL">
            <el-input v-model="addForm.sourceUrl" placeholder="https://.../xxx.torrent" />
          </el-form-item>
          <el-form-item label="磁力链接">
            <el-input v-model="addForm.magnetLink" placeholder="magnet:?xt=urn:btih:..." />
          </el-form-item>
        </div>
        <el-form-item label="Torrent 文件">
          <div class="hub__upload">
            <el-upload
              action="#"
              :show-file-list="false"
              :auto-upload="false"
              :before-upload="beforeUpload">
              <el-button>
                <PtIcon name="upload" :size="14" /><span>选择 .torrent 文件</span>
              </el-button>
            </el-upload>
            <span v-if="uploadFileName" class="hub__upload-name">{{ uploadFileName }}</span>
          </div>
          <div class="field-tip">URL、磁力、文件三者任选其一。</div>
        </el-form-item>
        <div class="field-row">
          <el-form-item label="保存路径">
            <el-input v-model="addForm.savePath" placeholder="可选" />
          </el-form-item>
          <el-form-item label="分类">
            <el-input v-model="addForm.category" placeholder="可选" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="addForm.tags" placeholder="可选，逗号分隔" />
          </el-form-item>
        </div>
        <el-form-item label="添加为暂停">
          <el-switch v-model="addForm.addPaused" />
          <div class="field-tip">开启后任务只入列不开跑，适合先核对路径再手动开始。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="submitAdd">添加</el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="detailDrawerVisible"
      class="hub__drawer"
      title="任务详情"
      size="56%"
      direction="rtl"
      :destroy-on-close="true">
      <DownloaderTorrentDetail
        v-model:tab="detailActiveTab"
        :detail="detail"
        :loading="detailLoading" />
    </el-drawer>
  </div>
</template>

<style scoped>
/*
 * 沉浸模式页面：App.vue 把整块视口交给这一页（.pt-shell.is-immersive 隐藏了
 * rail / 导航列 / 页头 / 状态条），所以页头、筛选侧栏、导航下拉都由本页自己画。
 * 其他页面不该照抄这个结构——它们的标题和导航仍由 shell 提供。
 *
 * 原先这一页的皮肤在 styles/downloader-hub-page.css，一整套 --vt-* 深绿硬编码，
 * 换成主题令牌后那份文件再无第二个消费者，于是折进本文件一起删掉。
 */
.hub {
  display: flex;
  flex-direction: column;
  min-height: 100%;
  color: var(--pt-t2);
  background: var(--pt-canvas);
}

/* ---------- 页头 ---------- */
/*
 * 这条 hero 用的是深色 chrome 底，8 套配色里都是深色，所以文字必须走
 * --pt-chrome-t1/t2 而不是 --pt-t1/t3 —— 后者在浅色模式下是深灰，
 * 会变成深底深字。标志同理取该表面主文字色的单色版。
 */
.hub__hero {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  padding: var(--pt-space-3) var(--pt-pad);
  color: var(--pt-chrome-t1);
  background: var(--pt-chrome);
  border-bottom: 1px solid var(--pt-chrome-border);
}

.hub__hero-text {
  min-width: 0;
}

.hub__hero-text h1 {
  margin: 0;
  font-size: var(--pt-fz-h1);
  font-weight: 600;
  line-height: var(--pt-lh-tight);
  color: var(--pt-chrome-t1);
}

.hub__hero-text p {
  margin: 2px 0 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-chrome-t2);
}

.hub__hero-acts {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  margin-left: auto;
}

/* ---------- 双列骨架 ---------- */
.hub__layout {
  display: flex;
  flex: 1;
  min-height: 0;
}

/* 侧栏宽度由 sidebarStyle 内联给（可拖拽），这里只管配色和滚动 */
.hub__side {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
  padding: var(--pt-space-3);
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--pt-border-strong) transparent;
  background: var(--pt-surface);
  border-right: 1px solid var(--pt-border);
}

.hub__side::-webkit-scrollbar {
  width: 5px;
}

.hub__side::-webkit-scrollbar-thumb {
  background: var(--pt-border-strong);
  border-radius: var(--pt-radius-full);
}

.hub__side-head {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.hub__side-head > .hub__ico {
  margin-left: auto;
}

.hub__side-foot {
  display: flex;
  gap: var(--pt-space-1);
  align-items: center;
  margin-top: auto;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

/* 8px 的拖拽把手：默认只是一条比边框稍亮的缝，悬停才提示可拖 */
.hub__resizer {
  flex: 0 0 8px;
  cursor: col-resize;
  background: var(--pt-hover);
  border-right: 1px solid var(--pt-border);
}

.hub__resizer:hover {
  background: var(--pt-p-soft);
}

.hub__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--pt-space-3);
  min-width: 0;
  padding: var(--pt-space-3);
  overflow: hidden;
}

/* ---------- 侧栏区块 ---------- */
.hub__sec {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.hub__sec-t {
  margin: 0 0 var(--pt-space-1);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
}

/* [图标] 标签 …… 数值：一行一个指标，比原来的 2×2 卡片省一半高度 */
.hub__stat {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  height: 24px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.hub__stat-l {
  flex: 1;
  min-width: 0;
}

.hub__stat-v {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
  font-variant-numeric: tabular-nums;
}

/* 上下行方向统一配色：下载走 ok、上传走 info，和表格里的状态色条一致 */
.is-dl {
  color: var(--pt-ok);
}

.is-ul {
  color: var(--pt-info);
}

/* 状态 / 分类筛选项：原来是可点的 div，换成 button 才有键盘焦点 */
.hub__pick {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  width: 100%;
  height: 28px;
  padding: 0 var(--pt-space-2);
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: var(--pt-r-md);
}

.hub__pick:hover {
  background: var(--pt-hover);
}

.hub__pick.is-on {
  color: var(--pt-p);
  background: var(--pt-p-soft);
}

.hub__pick.is-dl :deep(svg) {
  color: var(--pt-ok);
}

.hub__pick.is-seed :deep(svg) {
  color: var(--pt-info);
}

.hub__pick.is-pause :deep(svg) {
  color: var(--pt-warn);
}

.hub__pick.is-err :deep(svg) {
  color: var(--pt-dang);
}

.hub__pick-l {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hub__pick-n {
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
  font-variant-numeric: tabular-nums;
}

.hub__pick.is-on .hub__pick-n {
  color: var(--pt-p);
}

/* 标签是分类不是状态，所以做成一片可折行的胶囊而不是整行按钮 */
.hub__tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-1);
}

.hub__tag {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  max-width: 100%;
  padding: 2px var(--pt-space-2);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-radius-full);
}

.hub__tag span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hub__tag:hover {
  border-color: var(--pt-border-strong);
}

.hub__tag.is-on {
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-color: color-mix(in srgb, var(--pt-p) 40%, transparent);
}

/* ---------- 通用图标按钮 ---------- */
.hub__ico {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  color: var(--pt-t3);
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: var(--pt-r-md);
}

.hub__ico:hover {
  color: var(--pt-t1);
  background: var(--pt-hover);
}

/* ---------- 工具条 ---------- */
.hub__nav {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 26px;
  padding: 0 var(--pt-space-2);
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.hub__nav:hover {
  color: var(--pt-t1);
  border-color: var(--pt-border-strong);
}

/*
 * brand.md 要求 mono 变体取所在表面的主文字色，而工具条整体是次级色（悬停才提到主色）。
 * 只把标志钉到 --pt-t1，折叠箭头留在次级色上，保住工具条本来的层级。
 * plated 变体自带品牌色，不受 currentColor 影响，所以只选 mono。
 */
.hub__nav .pt-logo--mono {
  color: var(--pt-t1);
}

.hub__search {
  flex: 1 1 180px;
  min-width: 140px;
  max-width: 320px;
}

.hub__sort {
  flex: 0 0 132px;
}

/* 面板头里的自动刷新：开关和文字要当成一个整体，tooltip 才有落点 */
.hub__auto {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* ---------- 批量操作条 ---------- */
/* 紧贴工具条下方，所以去掉 .pt-strip 自带的上沿，改在下沿分隔表头 */
.hub__bulk {
  border-top: 0;
  border-bottom: 1px solid var(--pt-border);
}

.hub__bulk-acts {
  display: inline-flex;
  gap: var(--pt-space-1);
  align-items: center;
}

/* ---------- 表格区 ---------- */
.hub__panel {
  flex: 1;
  min-height: 0;
}

.hub__table,
.hub__grid {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

/* 虚拟列表自己滚：contain + will-change 把重排关在这一层里 */
.hub__vshell {
  height: 100%;
  overflow: auto;
  contain: layout style;
  will-change: scroll-position;
  -webkit-overflow-scrolling: touch;
}

.hub__foot {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  width: 100%;
  flex-wrap: wrap;
}

.hub__warn {
  color: var(--pt-warn);
}

.hub__detail {
  flex: 0 0 auto;
}

/* ---------- 列设置（工具条气泡与表头右键菜单共用） ---------- */
.hub__cols-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 24px;
  margin-bottom: var(--pt-space-1);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
}

.hub__cols {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 2px var(--pt-space-2);
}

.hub__cols :deep(.el-checkbox) {
  height: 24px;
  margin-right: 0;
}

.hub__cols :deep(.el-checkbox__label) {
  font-size: var(--pt-fz-sm);
}

.hub__order {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 200px;
  overflow-y: auto;
}

.hub__order-item {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  height: 24px;
  padding: 0 var(--pt-space-1);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: grab;
  border-radius: var(--pt-r-sm);
}

.hub__order-item:hover {
  background: var(--pt-hover);
}

.hub__order-item.is-off {
  color: var(--pt-t4);
}

/* 表头右键菜单被 teleport 到 body，配色只能用全局令牌 */
.hub__hmenu {
  position: fixed;
  z-index: 3100;
  width: 260px;
  max-height: 360px;
  padding: var(--pt-space-2);
  overflow: auto;
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-lg);
}

/* ---------- 添加种子对话框 ---------- */
.hub__upload {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.hub__upload-name {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  word-break: break-all;
}

/* ---------- 详情抽屉 ---------- */
/* atoms.css 里只有 .pt-dialog 皮肤，没有抽屉皮肤，所以这里按令牌单独铺一层 */
.hub__drawer :deep(.el-drawer__header) {
  height: 42px;
  padding: 0 var(--pt-pad);
  margin: 0;
  font-size: var(--pt-fz-h2);
  font-weight: 600;
  color: var(--pt-t1);
  background: var(--pt-surface);
  border-bottom: 1px solid var(--pt-border);
}

.hub__drawer :deep(.el-drawer__body) {
  padding: var(--pt-pad);
  background: var(--pt-surface);
}

/* ---------- 窄屏：侧栏落到上方，拖拽把手无意义 ---------- */
@media (max-width: 900px) {
  .hub__layout {
    flex-direction: column;
  }

  .hub__resizer {
    display: none;
  }

  .hub__side {
    /* 宽度是拖拽存下来的内联样式，窄屏必须压过它 */
    width: 100% !important;
    max-height: 42vh;
    border-right: none;
    border-bottom: 1px solid var(--pt-border);
  }

  .hub__hero {
    flex-wrap: wrap;
  }

  .hub__hero-acts {
    width: 100%;
    margin-left: 0;
  }

  .hub__main {
    padding: var(--pt-space-2);
  }
}
</style>
