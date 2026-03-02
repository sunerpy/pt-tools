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
import DownloaderTorrentTable from "@/components/downloader/DownloaderTorrentTable.vue";
import DownloaderTorrentVirtualTable from "@/components/downloader/DownloaderTorrentVirtualTable.vue";
import { ArrowDown, ArrowUp } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();

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

watch(
  () => filters.value.downloaderId,
  () => {
    page.value = 1;
    loadTorrents();
  },
);

watch(
  () => filters.value.state,
  () => {
    page.value = 1;
    loadTorrents();
  },
);

watch(
  () => filters.value.category,
  () => {
    page.value = 1;
    loadTorrents();
  },
);
watch(
  () => filters.value.tag,
  () => {
    page.value = 1;
    loadTorrents();
  },
);
watch(
  () => filters.value.search,
  () => {
    if (searchTimer) {
      window.clearTimeout(searchTimer);
    }
    searchTimer = window.setTimeout(() => {
      page.value = 1;
      loadTorrents();
    }, 320);
  },
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
  const paginationHeight =
    !showAllTasks.value && paginationRef.value ? paginationRef.value.offsetHeight : 0;
  const availableHeight = viewportHeight - rect.top - paginationHeight - 14;
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
</script>

<template>
  <div class="page-container downloader-hub-page">
    <div v-if="heroVisible" class="hub-hero">
      <div>
        <h1 class="hub-title">混合下载器控制台</h1>
        <p class="hub-subtitle">聚合所有下载器任务，支持单下载器与全局视图。</p>
      </div>
      <div class="hub-hero-actions">
        <el-button type="primary" @click="openAddDialog">添加种子</el-button>
        <el-button @click="toggleHero">收起</el-button>
      </div>
    </div>

    <div ref="hubLayoutRef" class="hub-layout">
      <div v-show="sidebarVisible" class="hub-sidebar" :style="sidebarStyle">
        <div class="sidebar-header">
          <div class="sidebar-header-title">侧栏</div>
          <el-button size="small" text @click="toggleSidebar">收起</el-button>
        </div>
        <div class="sidebar-section sidebar-stats">
          <div class="sidebar-title">SPEED</div>
          <div class="stat-grid">
            <div class="stat-card stat-dl">
              <div class="stat-icon">↓</div>
              <div class="stat-body">
                <div class="stat-value">
                  {{ formatSize(transferStats?.total_download_speed || 0) }}/s
                </div>
                <div class="stat-label">下载速度</div>
              </div>
            </div>
            <div class="stat-card stat-ul">
              <div class="stat-icon">↑</div>
              <div class="stat-body">
                <div class="stat-value">
                  {{ formatSize(transferStats?.total_upload_speed || 0) }}/s
                </div>
                <div class="stat-label">上传速度</div>
              </div>
            </div>
          </div>
        </div>
        <div class="sidebar-section sidebar-stats">
          <div class="sidebar-title">SESSION</div>
          <div class="stat-grid">
            <div class="stat-card stat-dl">
              <div class="stat-icon">⬇</div>
              <div class="stat-body">
                <div class="stat-value">
                  {{ formatSize(transferStats?.total_session_downloaded || 0) }}
                </div>
                <div class="stat-label">本次下载</div>
              </div>
            </div>
            <div class="stat-card stat-ul">
              <div class="stat-icon">⬆</div>
              <div class="stat-body">
                <div class="stat-value">
                  {{ formatSize(transferStats?.total_session_uploaded || 0) }}
                </div>
                <div class="stat-label">本次上传</div>
              </div>
            </div>
          </div>
        </div>
        <div class="sidebar-section sidebar-stats">
          <div class="sidebar-title">ALL TIME</div>
          <div class="stat-grid">
            <div class="stat-card stat-dl">
              <div class="stat-icon">⬇</div>
              <div class="stat-body">
                <div class="stat-value">{{ formatSize(transferStats?.total_downloaded || 0) }}</div>
                <div class="stat-label">总下载量</div>
              </div>
            </div>
            <div class="stat-card stat-ul">
              <div class="stat-icon">⬆</div>
              <div class="stat-body">
                <div class="stat-value">{{ formatSize(transferStats?.total_uploaded || 0) }}</div>
                <div class="stat-label">总上传量</div>
              </div>
            </div>
          </div>
        </div>
        <div class="sidebar-section sidebar-stats">
          <div class="sidebar-title">DISK</div>
          <div class="stat-grid">
            <div class="stat-card">
              <div class="stat-icon">💾</div>
              <div class="stat-body">
                <div class="stat-value">{{ formatSize(transferStats?.total_free_space || 0) }}</div>
                <div class="stat-label">剩余空间</div>
              </div>
            </div>
          </div>
        </div>
        <div class="sidebar-section">
          <div class="sidebar-title">STATUS</div>
          <div class="sidebar-state-list">
            <div
              :class="['state-item state-all', { active: filters.state === '' }]"
              @click="applyQuickState('')">
              <span class="state-label">全部</span><span class="state-count">{{ total }}</span>
            </div>
            <div
              :class="['state-item state-dl', { active: filters.state === 'downloading' }]"
              @click="applyQuickState('downloading')">
              <span class="state-label">下载中</span
              ><span class="state-count">{{ downloadingCount }}</span>
            </div>
            <div
              :class="['state-item state-seed', { active: filters.state === 'seeding' }]"
              @click="applyQuickState('seeding')">
              <span class="state-label">做种中</span
              ><span class="state-count">{{ seedingCount }}</span>
            </div>
            <div
              :class="['state-item state-pause', { active: filters.state === 'paused' }]"
              @click="applyQuickState('paused')">
              <span class="state-label">暂停</span
              ><span class="state-count">{{ pausedCount }}</span>
            </div>
            <div
              :class="['state-item state-pause', { active: filters.state === 'stopped' }]"
              @click="applyQuickState('stopped')">
              <span class="state-label">已停止</span
              ><span class="state-count">{{ stoppedCount }}</span>
            </div>
            <div
              :class="['state-item state-err', { active: filters.state === 'error' }]"
              @click="applyQuickState('error')">
              <span class="state-label">错误</span><span class="state-count">{{ errorCount }}</span>
            </div>
          </div>
          <div class="sidebar-torrent-count">{{ total }} Torrents</div>
        </div>
        <div class="sidebar-section">
          <div class="sidebar-title">DOWNLOADER</div>
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
        </div>
        <div class="sidebar-section" v-if="allCategories.length > 0">
          <div class="sidebar-title">CATEGORY</div>
          <div class="sidebar-state-list">
            <div
              :class="['sidebar-category-item', { active: filters.category === '' }]"
              @click="filters.category = ''">
              <span class="state-label">全部</span>
            </div>
            <div
              v-for="cat in allCategories"
              :key="cat"
              :class="['sidebar-category-item', { active: filters.category === cat }]"
              @click="filters.category = filters.category === cat ? '' : cat">
              <span class="state-label">{{ cat }}</span>
            </div>
          </div>
        </div>
        <div class="sidebar-section" v-if="allTags.length > 0">
          <div class="sidebar-title">TAGS</div>
          <div class="sidebar-tags-wrap">
            <el-tag
              v-for="tag in allTags"
              :key="tag"
              :type="filters.tag === tag ? '' : 'info'"
              :effect="filters.tag === tag ? 'dark' : 'plain'"
              style="cursor: pointer"
              @click="filters.tag = filters.tag === tag ? '' : tag"
              >{{ tag }}</el-tag
            >
          </div>
        </div>
        <div class="sidebar-section">
          <div class="sidebar-title">VIEW</div>
          <el-switch
            v-model="showAllTasks"
            active-text="全部"
            inactive-text="分页"
            style="width: 100%" />
          <el-switch
            v-model="useVirtualList"
            :disabled="!showAllTasks"
            active-text="虚拟"
            inactive-text="标准"
            style="width: 100%; margin-top: 6px" />
        </div>
        <div class="sidebar-bottom">
          <el-tooltip content="保存布局" placement="top"
            ><el-button circle text @click="saveLayoutPreset">💾</el-button></el-tooltip
          >
          <el-tooltip content="加载布局" placement="top"
            ><el-button circle text @click="loadLayoutPreset">📂</el-button></el-tooltip
          >
          <el-tooltip content="刷新" placement="top"
            ><el-button circle text @click="loadTorrents">🔄</el-button></el-tooltip
          >
          <el-tooltip content="添加种子" placement="top"
            ><el-button circle text type="primary" @click="openAddDialog">+</el-button></el-tooltip
          >
        </div>
      </div>
      <div
        v-show="sidebarVisible"
        class="sidebar-resizer"
        title="拖拽调整侧栏宽度"
        @mousedown="onSidebarResizeStart" />

      <div ref="hubMainRef" class="hub-main">
        <div class="hub-toolbar-new">
          <el-button
            v-if="!sidebarVisible"
            size="small"
            class="toolbar-expand-sidebar"
            @click="toggleSidebar"
            >☰ 展开侧栏</el-button
          >
          <el-dropdown
            trigger="click"
            class="nav-dropdown"
            @command="
              (cmd: string) => {
                if (cmd === 'toggle-sidebar') {
                  toggleSidebar();
                } else if (cmd === 'toggle-hero') {
                  toggleHero();
                } else {
                  router.push('/' + cmd);
                }
              }
            ">
            <el-button class="nav-trigger">
              <span class="nav-logo">PT</span>
              <el-icon><ArrowDown /></el-icon>
            </el-button>
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
                <el-dropdown-item command="sites">站点与RSS</el-dropdown-item>
                <el-dropdown-item command="search">种子搜索</el-dropdown-item>
                <el-dropdown-item command="tasks">任务列表</el-dropdown-item>
                <el-dropdown-item command="logs">日志</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-input
            v-model="filters.search"
            placeholder="搜索"
            clearable
            size="small"
            class="toolbar-search" />
          <el-select v-model="sortBy" size="small" class="toolbar-sort">
            <el-option
              v-for="item in sortOptions"
              :key="item.value"
              :label="item.label"
              :value="item.value" />
          </el-select>
          <el-button size="small" circle @click="toggleSortOrder">
            <el-icon><ArrowUp v-if="sortOrder === 'asc'" /><ArrowDown v-else /></el-icon>
          </el-button>
          <el-radio-group v-model="tableDensity" size="small">
            <el-radio-button label="compact">紧凑</el-radio-button>
            <el-radio-button label="comfortable">标准</el-radio-button>
          </el-radio-group>
          <el-radio-group v-model="detailMode" size="small">
            <el-radio-button label="drawer">侧边</el-radio-button>
            <el-radio-button label="inline">下方</el-radio-button>
          </el-radio-group>
          <el-popover placement="bottom-end" :width="260" trigger="click">
            <template #reference><el-button size="small">列</el-button></template>
            <div class="column-setting-header">
              <span>显示列</span
              ><el-button text type="primary" @click="restoreDefaultColumns">恢复默认</el-button>
            </div>
            <el-checkbox-group v-model="visibleColumns" class="column-check-group">
              <el-checkbox v-for="item in columnOptions" :key="item.value" :label="item.value">{{
                item.label
              }}</el-checkbox>
            </el-checkbox-group>
            <div class="column-order-title">拖拽排序</div>
            <div class="column-order-list">
              <div
                v-for="columnKey in columnOrder"
                :key="columnKey"
                class="column-order-item"
                :class="{ hidden: !visibleColumns.includes(columnKey) }"
                draggable="true"
                @dragstart="onColumnDragStart(columnKey)"
                @dragover.prevent
                @drop="onColumnDrop(columnKey)">
                <span class="drag-handle">::</span><span>{{ getColumnLabel(columnKey) }}</span>
              </div>
            </div>
          </el-popover>
          <el-button type="primary" size="small" @click="openAddDialog">+</el-button>
          <el-switch
            v-model="autoRefreshEnabled"
            active-text="自动"
            inactive-text=""
            size="small" />
          <el-button size="small" type="primary" @click="showAllTasks = !showAllTasks">{{
            showAllTasks ? "全部" : "分页"
          }}</el-button>
          <span class="toolbar-count">{{ total }} 项</span>
          <span v-if="allTasksLimited" class="toolbar-count toolbar-warning"
            >仅渲染前 {{ MAX_ALL_TASK_ROWS }} 条（防卡死）</span
          >
        </div>

        <div v-show="selectedCount > 0" class="hub-batch-actions">
          <span>已选 {{ selectedCount }} 项</span>
          <el-button
            size="small"
            :loading="actionLoading"
            :disabled="!selectedCanUse('pause')"
            @click="batchAction('pause')"
            >暂停</el-button
          >
          <el-button
            size="small"
            :loading="actionLoading"
            :disabled="!selectedCanUse('resume')"
            @click="batchAction('resume')"
            >开始</el-button
          >
          <el-button
            size="small"
            :loading="actionLoading"
            :disabled="!selectedCanUse('recheck')"
            @click="batchAction('recheck')"
            >复检</el-button
          >
          <el-button
            size="small"
            :loading="actionLoading"
            :disabled="!selectedCanUse('set_location')"
            @click="openSetLocationDialog"
            >路径</el-button
          >
          <el-button
            size="small"
            type="danger"
            :loading="actionLoading"
            :disabled="!selectedCanUse('delete')"
            @click="batchAction('delete')"
            >删除</el-button
          >
          <el-button
            size="small"
            type="danger"
            plain
            :loading="actionLoading"
            :disabled="!selectedCanUse('delete_with_files')"
            @click="batchAction('delete_with_files')"
            >删除+文件</el-button
          >
        </div>
        <el-card v-loading="tableLoading" shadow="never" class="hub-table-card">
          <div ref="tableCardBodyRef" class="hub-table-body">
            <div class="hub-table-content" @contextmenu.prevent>
              <div
                v-if="showAllTasks && useVirtualList"
                ref="virtualContainer"
                class="virtual-table-shell"
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
            <div v-if="!showAllTasks" ref="paginationRef" class="hub-pagination">
              <el-pagination
                v-model:current-page="page"
                v-model:page-size="pageSize"
                :total="total"
                :page-sizes="[50, 100, 200]"
                layout="total, sizes, prev, pager, next, jumper"
                @size-change="loadTorrents"
                @current-change="loadTorrents" />
            </div>
          </div>
        </el-card>
        <teleport to="body">
          <div
            v-if="headerColumnMenuVisible"
            ref="headerColumnMenuRef"
            class="header-column-menu"
            :style="{ left: `${headerColumnMenuX}px`, top: `${headerColumnMenuY}px` }"
            @click.stop>
            <div class="column-setting-header">
              <span>显示列</span>
              <el-button text type="primary" @click="restoreDefaultColumns">恢复默认</el-button>
            </div>
            <el-checkbox-group v-model="visibleColumns" class="column-check-group">
              <el-checkbox v-for="item in columnOptions" :key="item.value" :label="item.value">{{
                item.label
              }}</el-checkbox>
            </el-checkbox-group>
          </div>
        </teleport>
        <el-card
          v-if="inlineDetailVisible"
          shadow="never"
          class="inline-detail-card detail-surface">
          <template #header>
            <div class="inline-detail-header">
              <span>任务详情</span>
              <el-button text @click="closeInlineDetail">关闭</el-button>
            </div>
          </template>
          <div v-loading="detailLoading" class="detail-shell">
            <template v-if="detail">
              <div class="detail-meta">
                <div><strong>标题：</strong>{{ detail.torrent.title }}</div>
                <div>
                  <strong>下载器：</strong>{{ detail.torrent.downloader_name }} ({{
                    detail.torrent.downloader_type
                  }})
                </div>
                <div><strong>Hash：</strong>{{ detail.torrent.info_hash || "-" }}</div>
                <div><strong>保存路径：</strong>{{ detail.torrent.save_path || "-" }}</div>
              </div>
              <el-tabs v-model="detailActiveTab" class="detail-tabs">
                <el-tab-pane :label="`文件 (${detail.files.length})`" name="files" lazy>
                  <el-table :data="detail.files" size="small" max-height="300">
                    <el-table-column prop="index" label="#" width="64" />
                    <el-table-column
                      prop="name"
                      label="文件"
                      min-width="260"
                      show-overflow-tooltip />
                    <el-table-column label="大小" width="120" align="right">
                      <template #default="{ row }">{{ formatSize(row.size) }}</template>
                    </el-table-column>
                    <el-table-column label="进度" width="160">
                      <template #default="{ row }">
                        <el-progress
                          :percentage="Math.round((row.progress || 0) * 100)"
                          :stroke-width="8" />
                      </template>
                    </el-table-column>
                    <el-table-column prop="priority" label="优先级" width="90" align="right" />
                  </el-table>
                </el-tab-pane>
                <el-tab-pane :label="`Tracker (${detail.trackers.length})`" name="trackers" lazy>
                  <el-table :data="detail.trackers" size="small" max-height="300">
                    <el-table-column prop="url" label="URL" min-width="320" show-overflow-tooltip />
                    <el-table-column prop="status" label="状态" width="80" align="center" />
                    <el-table-column prop="seeds" label="Seeds" width="90" align="right" />
                    <el-table-column prop="peers" label="Peers" width="90" align="right" />
                    <el-table-column prop="leeches" label="Leeches" width="90" align="right" />
                  </el-table>
                </el-tab-pane>
              </el-tabs>
            </template>
          </div>
        </el-card>
      </div>
    </div>

    <el-dialog v-model="locationDialogVisible" title="批量修改存储路径" width="480px"
      ><el-input v-model="newLocation" placeholder="例如 /downloads/tv" /><template #footer
        ><el-button @click="locationDialogVisible = false">取消</el-button
        ><el-button type="primary" :loading="actionLoading" @click="submitSetLocation"
          >确定修改</el-button
        ></template
      ></el-dialog
    >
    <el-dialog v-model="addDialogVisible" title="添加种子到下载器" width="620px"
      ><el-form label-width="100px"
        ><el-form-item label="下载器"
          ><el-select
            v-model="addForm.downloaderIds"
            multiple
            clearable
            collapse-tags
            style="width: 100%"
            ><el-option
              v-for="item in downloaders"
              :key="item.id"
              :label="`${item.name} (${item.type})`"
              :value="item.id" /></el-select></el-form-item
        ><el-form-item label="种子 URL"
          ><el-input
            v-model="addForm.sourceUrl"
            placeholder="https://.../xxx.torrent" /></el-form-item
        ><el-form-item label="磁力链接"
          ><el-input
            v-model="addForm.magnetLink"
            placeholder="magnet:?xt=urn:btih:..." /></el-form-item
        ><el-form-item label="Torrent 文件"
          ><el-upload
            action="#"
            :show-file-list="false"
            :auto-upload="false"
            :before-upload="beforeUpload"
            ><el-button>选择 .torrent 文件</el-button></el-upload
          ><span v-if="uploadFileName" style="margin-left: 10px">{{
            uploadFileName
          }}</span></el-form-item
        ><el-form-item label="保存路径"
          ><el-input v-model="addForm.savePath" placeholder="可选" /></el-form-item
        ><el-form-item label="分类"
          ><el-input v-model="addForm.category" placeholder="可选" /></el-form-item
        ><el-form-item label="标签"
          ><el-input v-model="addForm.tags" placeholder="可选，逗号分隔" /></el-form-item
        ><el-form-item label="添加为暂停"
          ><el-switch v-model="addForm.addPaused" /></el-form-item></el-form
      ><template #footer
        ><el-button @click="addDialogVisible = false">取消</el-button
        ><el-button type="primary" :loading="addLoading" @click="submitAdd"
          >添加</el-button
        ></template
      ></el-dialog
    >
    <el-drawer
      v-model="detailDrawerVisible"
      class="detail-drawer detail-surface"
      title="任务详情"
      size="56%"
      direction="rtl"
      :destroy-on-close="true">
      <div v-loading="detailLoading" class="detail-shell">
        <template v-if="detail">
          <div class="detail-meta">
            <div><strong>标题：</strong>{{ detail.torrent.title }}</div>
            <div>
              <strong>下载器：</strong>{{ detail.torrent.downloader_name }} ({{
                detail.torrent.downloader_type
              }})
            </div>
            <div><strong>Hash：</strong>{{ detail.torrent.info_hash || "-" }}</div>
            <div><strong>保存路径：</strong>{{ detail.torrent.save_path || "-" }}</div>
          </div>
          <el-tabs v-model="detailActiveTab" class="detail-tabs">
            <el-tab-pane :label="`文件 (${detail.files.length})`" name="files" lazy>
              <el-table :data="detail.files" size="small" max-height="300">
                <el-table-column prop="index" label="#" width="64" />
                <el-table-column prop="name" label="文件" min-width="260" show-overflow-tooltip />
                <el-table-column label="大小" width="120" align="right">
                  <template #default="{ row }">{{ formatSize(row.size) }}</template>
                </el-table-column>
                <el-table-column label="进度" width="160">
                  <template #default="{ row }">
                    <el-progress
                      :percentage="Math.round((row.progress || 0) * 100)"
                      :stroke-width="8" />
                  </template>
                </el-table-column>
                <el-table-column prop="priority" label="优先级" width="90" align="right" />
              </el-table>
            </el-tab-pane>
            <el-tab-pane :label="`Tracker (${detail.trackers.length})`" name="trackers" lazy>
              <el-table :data="detail.trackers" size="small" max-height="300">
                <el-table-column prop="url" label="URL" min-width="320" show-overflow-tooltip />
                <el-table-column prop="status" label="状态" width="80" align="center" />
                <el-table-column prop="seeds" label="Seeds" width="90" align="right" />
                <el-table-column prop="peers" label="Peers" width="90" align="right" />
                <el-table-column prop="leeches" label="Leeches" width="90" align="right" />
              </el-table>
            </el-tab-pane>
          </el-tabs>
        </template>
      </div>
    </el-drawer>
  </div>
</template>
<style scoped>
@import "@/styles/downloader-hub-page.css";

.detail-shell {
  color: #ffffff;
  background: #1a2e27;
  border-radius: 12px;
}

.detail-tabs {
  --el-text-color-regular: #a0b8ac;
}

:deep(.detail-surface .el-drawer__header) {
  color: #ffffff;
  font-weight: 700;
  font-size: 18px;
  margin-bottom: 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  background: #1a2e27;
}

:deep(.detail-surface .el-drawer__body) {
  color: #ffffff;
  background: #1a2e27;
  padding: 16px;
}

:deep(.inline-detail-card .el-card__header) {
  border-bottom-color: rgba(255, 255, 255, 0.1);
}

:deep(.inline-detail-card .el-tabs__item),
:deep(.detail-surface .el-tabs__item) {
  color: #a0b8ac;
  font-weight: 600;
  transition: color 0.2s;
}

:deep(.inline-detail-card .el-tabs__item:hover),
:deep(.detail-surface .el-tabs__item:hover) {
  color: #ffffff;
}

:deep(.inline-detail-card .el-tabs__item.is-active),
:deep(.detail-surface .el-tabs__item.is-active) {
  color: #ffffff;
  font-weight: 700;
}

:deep(.inline-detail-card .el-tabs__active-bar),
:deep(.detail-surface .el-tabs__active-bar) {
  background-color: #64ceaa;
  height: 3px;
}

:deep(.inline-detail-card .el-table),
:deep(.detail-surface .el-table) {
  --el-table-bg-color: #14241e;
  --el-table-tr-bg-color: #14241e;
  --el-table-header-bg-color: #0d1a15;
  --el-table-header-text-color: #a0b8ac;
  --el-table-text-color: #ffffff;
  --el-table-border-color: rgba(255, 255, 255, 0.08);
  --el-table-row-hover-bg-color: #1e382f;
  background-color: #14241e;
}

:deep(.inline-detail-card .el-table td),
:deep(.inline-detail-card .el-table th),
:deep(.detail-surface .el-table td),
:deep(.detail-surface .el-table th) {
  background: transparent;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

:deep(.detail-surface .el-table .el-table__row:hover > td.el-table__cell),
:deep(.inline-detail-card .el-table .el-table__row:hover > td.el-table__cell) {
  background-color: #1e382f !important;
  color: #ffffff !important;
}

:deep(.detail-surface .el-table .el-table__row.current-row > td.el-table__cell),
:deep(.inline-detail-card .el-table .el-table__row.current-row > td.el-table__cell) {
  background-color: #234237 !important;
  color: #ffffff !important;
}

:deep(.detail-surface .el-progress-bar__outer),
:deep(.inline-detail-card .el-progress-bar__outer) {
  background-color: rgba(255, 255, 255, 0.1) !important;
}

:deep(.detail-surface .el-progress-bar__inner),
:deep(.inline-detail-card .el-progress-bar__inner) {
  background-color: #64ceaa !important;
}

:deep(.detail-surface .el-progress__text),
:deep(.inline-detail-card .el-progress__text) {
  color: #ffffff !important;
  font-weight: 600;
}

:deep(.detail-surface .el-loading-mask) {
  background: rgba(17, 33, 28, 0.68);
  backdrop-filter: blur(2px);
}

:deep(.hub-pagination .el-pagination) {
  --el-pagination-bg-color: transparent;
  --el-pagination-text-color: #e8f6ef;
  --el-pagination-button-color: #e8f6ef;
  --el-pagination-button-bg-color: rgba(255, 255, 255, 0.12);
  --el-pagination-hover-color: #9ae6cf;
}

:deep(.hub-pagination .btn-prev),
:deep(.hub-pagination .btn-next),
:deep(.hub-pagination .el-pager li) {
  background: rgba(255, 255, 255, 0.12) !important;
  color: #eefbf5 !important;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.12);
}

:deep(.hub-pagination .el-pager li.is-active) {
  background: rgba(100, 206, 170, 0.34) !important;
  color: #ffffff !important;
  border-color: rgba(100, 206, 170, 0.58);
}

:deep(.hub-pagination .el-pagination__total),
:deep(.hub-pagination .el-pagination__jump),
:deep(.hub-pagination .el-pagination__sizes) {
  color: #dff3ea !important;
}

:deep(.hub-pagination .el-input__wrapper),
:deep(.hub-pagination .el-select__wrapper) {
  background: rgba(255, 255, 255, 0.12) !important;
  color: #eefbf5 !important;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.16) !important;
}

:deep(.hub-pagination .el-input__inner),
:deep(.hub-pagination .el-select__placeholder),
:deep(.hub-pagination .el-select__selected-item),
:deep(.hub-pagination .el-select .el-select__wrapper .el-select__placeholder span),
:deep(.hub-pagination .el-select__caret),
:deep(.hub-pagination .el-select__suffix),
:deep(.hub-pagination .el-input__suffix),
:deep(.hub-pagination .el-input__prefix) {
  color: #eefbf5 !important;
}

:deep(.hub-pagination .el-pagination__goto),
:deep(.hub-pagination .el-pagination__classifier) {
  color: #dff3ea !important;
}
</style>
