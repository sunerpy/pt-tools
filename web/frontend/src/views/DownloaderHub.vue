<script setup lang="ts">
import {
  type DownloaderCapability,
  type DownloaderFailure,
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
import PtBars from "@/components/ui/PtBars.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

const COLUMN_STORAGE_KEY = "downloader-hub-visible-columns-v1";
const COLUMN_ORDER_STORAGE_KEY = "downloader-hub-column-order-v1";
const DENSITY_STORAGE_KEY = "downloader-hub-density-v1";
const LAYOUT_PRESET_STORAGE_KEY = "downloader-hub-layout-preset-v1";
const DETAIL_MODE_STORAGE_KEY = "downloader-hub-detail-mode-v1";
const SIDEBAR_VISIBLE_STORAGE_KEY = "downloader-hub-sidebar-visible-v1";
const SIDEBAR_WIDTH_STORAGE_KEY = "downloader-hub-sidebar-width-v1";
/* 画板 18 的左列是 276（p-io / p-state / p-filter 都是这个宽度） */
const SIDEBAR_WIDTH_MIN = 276;
const SIDEBAR_WIDTH_MAX = 420;
const SIDEBAR_WIDTH_DEFAULT = 276;
const MAX_ALL_TASK_ROWS = 5000;
/*
 * 移动端行卡的渲染上限。手机上不做虚拟滚动（行卡高度不定，撑不出稳定的行高），
 * 所以 limitRowsForSafety 那条防卡死上限在这里还得再收一档：「全部」范围下
 * torrents 已经被削到 5000，可 5000 张不定高的卡照样能让手机卡死。
 * 超出的条数在页脚说明，不静默丢。
 */
const MAX_MOBILE_CARD_ROWS = 200;
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

const isMobile = useIsMobile();
/**
 * 查询代次 —— 只由「用户意图变了」推进：换筛选、翻页、改排序、手动刷新，
 * 也就是每一次前台 loadTorrents。
 *
 * 落地前比一次代次，号不是最新的就整条丢弃：既不写数据也不清错误。
 * 没有这道校验会出错：静默请求用筛选 A 发出 → 用户切到筛选 B、前台加载失败并留下错误
 * → A 的响应晚到，把 B 的真实失败清掉并写回 A 的结果。页面于是把一次更新的失败
 * 显示成「已恢复」，摆着的还是旧筛选的数据。
 *
 * 5 秒的周期轮询**不推进**代次：它刷的是同一个查询，不是新查询。
 * 让它也推进会造成饥饿 —— 后端串行遍历下载器、单台 qBittorrent 的取列表可以等到 30s，
 * 请求耗时一旦超过 5 秒轮询间隔，每一拍都会作废上一拍，所有成功响应被永久丢弃，
 * 列表、partial 提示和错误恢复就再也不会自动更新了。
 */
let loadEpoch = 0;
/** 静默刷新单飞：上一拍还没回来就跳过这一拍，两个静默请求并存没有意义 */
let silentInFlight = false;
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

const torrents = ref<DownloaderTorrentItem[]>([]);
const downloaders = ref<DownloaderSetting[]>([]);
const selectedRows = ref<DownloaderTorrentItem[]>([]);
const selectedRowKeys = ref<string[]>([]);
const allTasksLimited = ref(false);
const capabilities = ref<DownloaderCapability[]>([]);
/**
 * 后端逐台上报的失败下载器。
 *
 * 以前某台下载器连不上会被静默跳过、接口照样返回 200，用户看到的是一份
 * 「少了一台下载器」的列表却毫无提示 —— 会以为任务真的没了。非空即部分失败：
 * 有任务时在列表上方挂提示条，一条任务都没有时 partial 成为主状态。
 */
const loadFailures = ref<DownloaderFailure[]>([]);

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
/**
 * 速率采样窗口 —— 画板 p-rate 386「速率」。
 *
 * 后端不存速率历史，但这一页本来就每 5 秒拉一次 transfer-stats，把每次的值留下来
 * 就是一条真实的滚动曲线（最近 24 个采样 ≈ 2 分钟）。**只在内存里**，刷新页面清零；
 * 不落 localStorage：它是「刚刚的走势」，不是历史数据。
 */
const RATE_SAMPLE_MAX = 24;
const rateSamples = ref<{ up: number; down: number }[]>([]);

function pushRateSample(up: number, down: number) {
  const next = [...rateSamples.value, { up, down }];
  rateSamples.value = next.length > RATE_SAMPLE_MAX ? next.slice(-RATE_SAMPLE_MAX) : next;
}

const downSeries = computed(() => rateSamples.value.map((s) => s.down));
const upSeries = computed(() => rateSamples.value.map((s) => s.up));

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

/**
 * 六态状态机（设计文档 §5）。
 *
 * 这一页以前只有一个 loading ref：loadTorrents 失败时弹个 toast，两秒后 toast 没了，
 * 而状态块固定按 hasActiveFilter 画成 empty / zero —— 一个 500 会被显示成
 * 「下载器里还没有任务」，用户以为下载器被清空了。401/403 还得单独画成「无权访问」，
 * 否则只会让人一直点重试。
 *
 * 多台下载器天生会部分失败，所以 failed 接的是失败下载器数：有任务时挂提示条
 * （hasPartialBanner），一条任务都没有时 partial 成为主状态。
 */
const { loading, state, errorText, run, hasPartialBanner, clearError } = useDataState({
  filtered: () => hasActiveFilter.value,
  failed: () => loadFailures.value.length,
});

/* 面板遮罩只在「还什么都没有」时铺满：手上已经有旧数据时盖一层遮罩会把它糊掉，
   刷新过程交给按钮上的 loading 表达 */
const tableLoading = computed(() => loading.value && torrents.value.length === 0);

/** 一行都没拿到时才用状态块顶掉列表；有行时 partial 只挂一条提示条 */
const showStateBlock = computed(() => torrents.value.length === 0);

/** 失败下载器清单，partial 作为主状态时当副标题用 */
const failureSummary = computed(() => {
  if (loadFailures.value.length === 0) return "";
  return `${loadFailures.value.length} 台下载器没有返回任务：${loadFailures.value
    .map((item) => item.downloader_name)
    .join("、")}`;
});

/* loading 留空，让 PtDataState 用它自带的「加载中」预设 */
const stateTitle = computed(() => {
  switch (state.value) {
    case "error":
      return "任务加载失败";
    case "perm":
      return "无权访问下载器任务";
    case "partial":
      return "没有下载器返回任务";
    case "zero":
      return "没有匹配的任务";
    case "empty":
      return "下载器里还没有任务";
    default:
      return "";
  }
});

/** 状态块的副标题：失败时给真实原因，空态时给下一步动作 */
/**
 * 画板 head 的 sub（11.5/400 t3）：任务总数、正在下载/做种，以及当前上下行速度。
 * 原来这些数字只在页内那条深色 hero 的副标题里说了一句静态介绍文案，
 * hero 去掉之后这一行是真实数字，比介绍文案有用。
 */
const headSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return "任务列表没加载出来";
  const parts = [`${total.value} 个任务`];
  const c = torrentStateCounters.value;
  if (c.downloading > 0) parts.push(`${c.downloading} 下载中`);
  if (c.seeding > 0) parts.push(`${c.seeding} 做种中`);
  /* 画板 18 的 sub 示例是「37 个任务 · 12 下载中 · 19 做种中 · 6 已暂停 · ↓… ↑…」五段，
     「已暂停」这一段此前漏了 —— 暂停着的任务恰恰是最需要在页头看见的那一类 */
  if (c.paused > 0) parts.push(`${c.paused} 已暂停`);
  const t = transferStats.value;
  if (t) {
    parts.push(`↓${formatSize(t.total_download_speed)}/s`);
    parts.push(`↑${formatSize(t.total_upload_speed)}/s`);
  }
  return parts.join(" · ");
});

const stateSub = computed(() => {
  switch (state.value) {
    case "error":
    case "perm":
      return errorText.value;
    case "partial":
      return failureSummary.value;
    case "zero":
      return "试试放宽状态、分类或关键词";
    case "empty":
      return "添加一个种子，任务会出现在这里";
    default:
      return "";
  }
});

/*
 * 移动端行卡列表（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
 * 桌面那两套表格原样保留，这里只是并列的第二套视图，所以不复用虚拟切片
 * （tableRows 在虚拟模式下只是可视窗口），而是从完整的 torrents 自己截。
 */
const mobileRows = computed(() => torrents.value.slice(0, MAX_MOBILE_CARD_ROWS));
const mobileRowsLimited = computed(() => torrents.value.length > MAX_MOBILE_CARD_ROWS);

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
      !detailLoading.value &&
      // 前台正在加载就跳过这一拍：两个请求并存只会互相作废，还白占一个连接
      !loading.value
    ) {
      void silentLoadTorrents();
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

/* 部分失败提示条挂在 tableCardBodyRef 上方，它一出现就把可用高度吃掉一块，
   所以失败条数也要触发一次重算 */
watch([selectedCount, sidebarVisible, total, loading, () => loadFailures.value.length], () => {
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
  if (silentInFlight) return;
  silentInFlight = true;
  // 只读当前代次，不推进：轮询刷的是同一个查询
  const epoch = loadEpoch;
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
    // 期间用户改过查询（换筛选、翻页、排序，或手动刷新）：这份结果已经过期
    if (epoch !== loadEpoch || requestedShowAll !== showAllTasks.value) {
      return;
    }
    /*
     * 这一拍成功了，上一次失败的结论就作废。
     *
     * 这条很要紧：错误态现在是常驻的（不再是两秒消失的 toast），而这个静默刷新
     * 不走 run()（run 会亮 loading 遮罩，5 秒闪一次没法用），所以不会自动清错误。
     * 少了这一句，服务恢复后如果恰好返回 0 行，页面会一直停在「任务加载失败」上，
     * 直到用户手动点重试 —— 把一个已经恢复的真实空态显示成故障。
     */
    clearError();

    const rows = limitRowsForSafety(resp.items, requestedShowAll);
    if (!isSameTorrentSnapshot(torrents.value, resp.items)) {
      torrents.value = rows;
      total.value = resp.total;
      restoreSelectedRows(rows);
    }
    /* 自动刷新也要跟着 failures 走：某台下载器是在两次刷新之间掉线的，
       不更新的话提示条会一直停在上一次的结论上。和上面一样先比一把，
       每 5 秒无条件换一个新数组会让提示条白白重渲染 */
    const nextFailures = resp.failures ?? [];
    if (!isSameFailureSnapshot(loadFailures.value, nextFailures)) {
      loadFailures.value = nextFailures;
    }
    autoRefreshTick += 1;
    if (autoRefreshTick % 6 === 0) {
      loadMeta();
      loadTransferStats();
    }
  } catch {
    /* silent */
  } finally {
    silentInFlight = false;
  }
}

async function loadTorrents() {
  const epoch = ++loadEpoch;
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

  const resp = await run(() => downloaderTorrentsApi.list(params));

  /*
   * 期间又发起了更新的请求（连点筛选、翻页），这一份已经过期：不写任何状态。
   *
   * 失败分支这里刻意不去清错误：错误是由「某一次」请求报出来的，更新的那次落地时
   * 会给出真相。宁可让用户多看一瞬错误，也不能把真实失败藏掉。
   */
  if (epoch !== loadEpoch) {
    return;
  }

  if (!resp) {
    /* 失败时清空：留着上一次的列表配一个「加载失败」的状态块更让人误解。
       toast 照旧弹，但它只是提醒 —— 错误常驻在状态块里 */
    torrents.value = [];
    total.value = 0;
    loadFailures.value = [];
    restoreSelectedRows([]);
    ElMessage.error(errorText.value || "任务加载失败");
    return;
  }
  if (requestedShowAll !== showAllTasks.value) {
    return;
  }
  const rows = limitRowsForSafety(resp.items, requestedShowAll);
  torrents.value = rows;
  total.value = resp.total;
  loadFailures.value = resp.failures ?? [];
  restoreSelectedRows(rows);
  virtualScrollTop.value = 0;
  if (virtualContainer.value) {
    virtualContainer.value.scrollTop = 0;
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
    const stats = await downloaderTorrentsApi.transferStats();
    transferStats.value = stats;
    pushRateSample(stats.total_upload_speed, stats.total_download_speed);
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

function isRowSelected(row: DownloaderTorrentItem): boolean {
  return selectedRowKeys.value.includes(rowSelectionKey(row));
}

/**
 * 移动端行卡上的勾选。
 *
 * 不另建一套选中态：桌面的 el-table 和虚拟表各自管自己的勾选，但两边最终都汇到
 * selectedRowKeys，所以这里也改这一份，再走 onSelectionKeysChange 让 selectedRows
 * 一起更新 —— 顶部那条批量操作条读的就是它。
 */
function toggleRowSelection(row: DownloaderTorrentItem) {
  const key = rowSelectionKey(row);
  const next = new Set(selectedRowKeys.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  onSelectionKeysChange([...next]);
}

/** 语义色沿用桌面表格的状态色条：下载 ok、做种 info、暂停/停止 warn、错误 dang */
function rowStateTone(
  row: DownloaderTorrentItem,
): "ok" | "warn" | "dang" | "info" | "primary" | "neutral" {
  switch (normalizeTorrentState(row.state)) {
    case "downloading":
      return "ok";
    case "seeding":
      return "info";
    case "paused":
    case "stopped":
      return "warn";
    case "error":
      return "dang";
    case "checking":
    case "queued":
      return "primary";
    default:
      return "neutral";
  }
}

/* 胶囊里给中文状态；下载器原样返回的是 stalledUP 这类内部值，手机上放不下也读不懂 */
function rowStateLabel(row: DownloaderTorrentItem): string {
  switch (normalizeTorrentState(row.state)) {
    case "downloading":
      return "下载中";
    case "seeding":
      return "做种中";
    case "paused":
      return "暂停";
    case "stopped":
      return "已停止";
    case "error":
      return "错误";
    case "checking":
      return "校验中";
    case "queued":
      return "排队中";
    default:
      return row.state || "未知";
  }
}

/* 与桌面表格的 progressTone 同一套：错误红、暂停黄、做种绿、其余走主色 */
function rowProgressTone(row: DownloaderTorrentItem): "primary" | "ok" | "warn" | "dang" {
  switch (normalizeTorrentState(row.state)) {
    case "error":
      return "dang";
    case "paused":
    case "stopped":
      return "warn";
    case "seeding":
      return "ok";
    default:
      return "primary";
  }
}

function isRowPaused(row: DownloaderTorrentItem): boolean {
  const normalized = normalizeTorrentState(row.state);
  return normalized === "paused" || normalized === "stopped";
}

/**
 * 单行的能力位判断，读的是 loadCapabilities 拿回来的同一份 capabilities，
 * 语义和批量操作条的 selectedCanUse 一致：拿不到能力信息就当不可用。
 */
function rowCanUse(row: DownloaderTorrentItem, action: "pause" | "resume"): boolean {
  const cap = capabilityByDownloader(row.downloader_id);
  if (!cap) return false;
  return action === "pause" ? cap.can_pause : cap.can_resume;
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

/* 行卡进度那行的「已完成」：接口只给百分比，没有已完成字节数 */
function downloadedSize(row: DownloaderTorrentItem): string {
  if (!row.size || row.size <= 0) return "-";
  return formatSize(row.size * (Math.min(100, Math.max(0, row.progress)) / 100));
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

/** 导出当前这一页（画板 bar-64 的 bi-file-down）。列表分页，手上只有这一页 */
function exportCsv() {
  const head = [
    "下载器",
    "标题",
    "状态",
    "进度",
    "大小",
    "上传速度",
    "下载速度",
    "分享率",
    "分类",
    "标签",
  ];
  const lines = [head.join(",")];
  for (const t of torrents.value) {
    const cells = [
      t.downloader_name,
      t.title,
      t.state,
      `${Math.round((t.progress ?? 0) * 100)}%`,
      formatSize(t.size ?? 0),
      `${formatSize(t.upload_speed ?? 0)}/s`,
      `${formatSize(t.download_speed ?? 0)}/s`,
      String(t.ratio ?? 0),
      t.category ?? "",
      t.tags ?? "",
    ];
    lines.push(cells.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","));
  }
  const blob = new Blob([`\uFEFF${lines.join("\n")}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `pt-tools-downloader-tasks-p${page.value}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  ElMessage.success(`已导出本页 ${torrents.value.length} 条`);
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

function isSameFailureSnapshot(current: DownloaderFailure[], next: DownloaderFailure[]): boolean {
  if (current.length !== next.length) {
    return false;
  }
  for (let i = 0; i < current.length; i += 1) {
    const a = current[i];
    const b = next[i];
    if (!a || !b || a.downloader_id !== b.downloader_id || a.error !== b.error) {
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
</script>

<template>
  <!--
    画板 18 把控制台画在外壳里（head 64 → bar-64 → 左列 276 / 右 788），
    不是整屏沉浸：rail 与导航列都在，所以这一页原来自己画的那条深色 hero
    与工具栏里的页内导航下拉都不再需要 —— 标题走外壳页头，导航走导航列。
  -->
  <div class="hub">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button type="primary" @click="openAddDialog">
        <PtIcon name="plus" :size="15" /><span>添加种子</span>
      </el-button>
    </Teleport>

    <!--
      画板 18 的 bar-64：head 之后一条 40 高的全宽带。这些控件原来长在任务卡内部，
      那样卡头和工具栏会叠成两层标题；按画板提到页面层之后，左右两栏的卡各自只有一层头。
    -->
    <PtToolbar band>
      <el-tooltip v-if="!sidebarVisible" content="展开侧栏" placement="bottom">
        <button type="button" class="hub__ico" @click="toggleSidebar">
          <PtIcon name="panel-left-open" :size="15" />
        </button>
      </el-tooltip>
      <el-input
        v-model="filters.search"
        placeholder="搜索标题、分类、标签…"
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
        <!-- 画板 38 的 pop-col 是 260 宽（原来写成 264） -->
        <!-- 画板 bar-64 右端的 bi-file-down：导出当前这一页 -->
        <el-tooltip content="导出本页为 CSV" placement="bottom">
          <button
            type="button"
            class="hub__ico"
            aria-label="导出"
            data-testid="hub-export-btn"
            @click="exportCsv">
            <PtIcon name="file-down" :size="15" />
          </button>
        </el-tooltip>
        <el-popover placement="bottom-end" :width="260" trigger="click">
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

    <!--
      画板 18 的左右两栏就是卡片层的两栏（276 / 788），所以挂 .pt-cards --rail：
      内缩 16、栏距 16 都由它给。栏宽仍可拖（宽度是内联样式，压过栅格的轨道），
      所以这里同时保留 flex 语义 —— 见下面 .hub__layout 的说明。
    -->
    <div ref="hubLayoutRef" class="hub__layout pt-cards pt-cards--rail">
      <!--
        画板 18 的左列是三张 276 宽的卡：p-io 208（传输）、p-state 268（状态）、
        p-filter 360（筛选）。之前这里是一整块 aside，从头到尾一个面，
        三组内容之间只有小标题，读起来分不出层级。
      -->
      <aside v-show="sidebarVisible" class="hub__side" :style="sidebarStyle">
        <PtPanel class="hub__card" title="传输" icon="activity">
          <template #actions>
            <el-tooltip content="收起侧栏" placement="right">
              <button type="button" class="hub__ico" @click="toggleSidebar">
                <PtIcon name="panel-left-close" :size="14" />
              </button>
            </el-tooltip>
          </template>
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
              <span class="hub__stat-v">{{
                formatSize(transferStats?.total_downloaded || 0)
              }}</span>
            </div>
            <div class="hub__stat">
              <PtIcon name="cloud-upload" :size="14" class="is-ul" />
              <span class="hub__stat-l">上传</span>
              <span class="hub__stat-v">{{ formatSize(transferStats?.total_uploaded || 0) }}</span>
            </div>
            <div class="hub__stat">
              <PtIcon name="hard-drive" :size="14" />
              <span class="hub__stat-l">剩余空间</span>
              <span class="hub__stat-v">{{
                formatSize(transferStats?.total_free_space || 0)
              }}</span>
            </div>
          </section>
        </PtPanel>

        <PtPanel class="hub__card" title="状态" icon="list-checks">
          <section class="hub__sec hub__sec--flat">
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
        </PtPanel>

        <PtPanel class="hub__card" title="筛选" icon="list-filter">
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
          <template #footer>
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
          </template>
        </PtPanel>
      </aside>
      <div
        v-show="sidebarVisible"
        class="hub__resizer"
        title="拖拽调整侧栏宽度"
        @mousedown="onSidebarResizeStart" />

      <!--
        右栏是「表格卡 + 下方两张 386 卡」上下两段（画板 p-grid 636,120 788×620，
        p-rate/p-note 636,752 与 1038,752），所以这里再套一层纵向容器。
      -->
      <div class="hub__right">
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
              <!-- 「添加种子」在外壳页头里已经有一枚，这里不再放第二枚 -->
              <el-tooltip content="每 5 秒自动刷新" placement="bottom">
                <span class="hub__auto">
                  <el-switch v-model="autoRefreshEnabled" size="small" />
                  <span>自动</span>
                </span>
              </el-tooltip>
            </template>

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

            <!--
            部分失败（§5 的 partial）：还有任务可看时不能用一整块状态图顶掉列表 ——
            那等于把已经拿到的任务也藏了。所以有任务时在列表上方挂这条提示，
            一个任务都没有时才让 partial 成为主状态（见下面的 PtDataState）。
          -->
            <div
              v-if="hasPartialBanner(torrents.length)"
              class="pt-note pt-note--warn hub__partial">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <div class="hub__partial-body">
                <p class="hub__partial-t">
                  {{ loadFailures.length }} 台下载器没有返回任务，下面只是其余下载器的列表
                </p>
                <p v-for="item in loadFailures" :key="item.downloader_id" class="hub__partial-l">
                  {{ item.downloader_name }}：{{ item.error }}
                </p>
              </div>
              <el-button size="small" :loading="loading" @click="loadTorrents">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </div>

            <div ref="tableCardBodyRef" class="hub__table">
              <PtDataState v-if="showStateBlock" :state="state" :title="stateTitle" :sub="stateSub">
                <!-- perm 不给重试：没权限点重试没有意义，只会让用户一直点 -->
                <template v-if="state !== 'perm' && state !== 'loading'" #action>
                  <el-button
                    v-if="state === 'error' || state === 'partial'"
                    size="small"
                    @click="loadTorrents">
                    <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                  </el-button>
                  <el-button v-else-if="hasActiveFilter" size="small" @click="clearFilters">
                    <PtIcon name="rotate-ccw" :size="14" /><span>清空筛选</span>
                  </el-button>
                  <el-button v-else type="primary" size="small" @click="openAddDialog">
                    <PtIcon name="plus" :size="14" /><span>添加种子</span>
                  </el-button>
                </template>
              </PtDataState>
              <div v-else-if="!isMobile" class="hub__grid" @contextmenu.prevent>
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

              <!--
              移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
              这张表桌面最多 16 列，手机上横着滚既看不到列头也和页面纵向滚动打架。
              卡上留的是判断一个任务要不要动手真正要看的：标题、下载器/大小/分类/标签、
              右上角状态、一条进度（已完成 / 总大小 + 百分比 + 上下行速率），底下主操作。
              排序仍然走上面工具条里的「排序 / 方向」，所以表头的 sortable 不算丢功能。
              高度沿用桌面那份测量值 tableMaxHeight，卡片列表自己滚，页脚照旧留在屏内。
            -->
              <div v-else class="hub__cards" :style="{ maxHeight: `${tableMaxHeight}px` }">
                <PtRowCard v-for="row in mobileRows" :key="rowSelectionKey(row)">
                  <template #lead>
                    <el-checkbox
                      :model-value="isRowSelected(row)"
                      :aria-label="`选择 ${row.title}`"
                      @update:model-value="toggleRowSelection(row)" />
                  </template>

                  <template #title>{{ row.title || "-" }}</template>

                  <template #meta>
                    <PtTag>{{ row.downloader_name }}</PtTag>
                    <span>{{ formatSize(row.size) }}</span>
                    <span v-if="row.category">
                      <PtIcon name="folder" :size="11" />
                      {{ row.category }}
                    </span>
                    <span v-if="row.tags">
                      <PtIcon name="tag" :size="11" />
                      {{ row.tags }}
                    </span>
                  </template>

                  <template #status>
                    <PtStatusPill :tone="rowStateTone(row)" size="sm">
                      {{ rowStateLabel(row) }}
                    </PtStatusPill>
                  </template>

                  <template #progress>
                    <PtProgress :percent="row.progress" :tone="rowProgressTone(row)" />
                    <span class="hub__card-pg">
                      <span>{{ downloadedSize(row) }} / {{ formatSize(row.size) }}</span>
                      <span class="hub__card-rate">
                        <span class="is-dl">↓ {{ formatSize(row.download_speed) }}/s</span>
                        <span class="is-ul">↑ {{ formatSize(row.upload_speed) }}/s</span>
                        <span class="hub__card-pct">{{ Math.round(row.progress) }}%</span>
                      </span>
                    </span>
                  </template>

                  <!--
                  主操作走 handleContextAction，和桌面右键菜单是同一条路径（同一份
                  batchAction + 重新加载 + 提示）；可用性读的也是同一份 capabilities。
                -->
                  <template #actions>
                    <el-button
                      v-if="isRowPaused(row)"
                      size="small"
                      :disabled="actionLoading || !rowCanUse(row, 'resume')"
                      @click="handleContextAction({ action: 'resume', row })">
                      <PtIcon name="play" :size="14" /><span>开始</span>
                    </el-button>
                    <el-button
                      v-else
                      size="small"
                      :disabled="actionLoading || !rowCanUse(row, 'pause')"
                      @click="handleContextAction({ action: 'pause', row })">
                      <PtIcon name="pause" :size="14" /><span>暂停</span>
                    </el-button>
                    <el-button type="primary" size="small" @click="openDetail(row)">
                      <PtIcon name="info" :size="14" /><span>详情</span>
                    </el-button>
                  </template>
                </PtRowCard>
              </div>
            </div>

            <template #footer>
              <div ref="paginationRef" class="hub__foot">
                <span class="pt-foot-note">
                  共 {{ total }} 个任务<template v-if="allTasksLimited">
                    ·
                    <span class="hub__warn">仅渲染前 {{ MAX_ALL_TASK_ROWS }} 条（防卡死）</span>
                  </template>
                  <!-- 行卡不虚拟滚动，所以移动端的上限比桌面那条更低，得单独说清楚 -->
                  <template v-if="isMobile && mobileRowsLimited">
                    ·
                    <span class="hub__warn">
                      行卡仅渲染前 {{ MAX_MOBILE_CARD_ROWS }} 条（防卡死）
                    </span>
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

        <!--
        画板右下两张 386 卡：p-rate（速率）与 p-note（这一页的口径）。
        速率曲线用的是本页每 5 秒那一拍 transfer-stats 攒出来的滚动窗口（只在内存里），
        不是后端的历史数据 —— 后端不存速率历史。
      -->
        <div class="hub__bottom">
          <PtPanel class="hub__card" title="速率" icon="activity">
            <div class="rate">
              <div class="rate__row">
                <PtIcon name="download" :size="14" class="is-dl" />
                <span class="rate__l">下载</span>
                <span class="rate__v">
                  {{ formatSize(transferStats?.total_download_speed || 0) }}/s
                </span>
              </div>
              <PtBars
                v-if="downSeries.length > 1"
                :values="downSeries"
                :count="RATE_SAMPLE_MAX"
                :bar-width="9"
                :gap="4"
                :height="28"
                hue="var(--pt-info)" />
              <div class="rate__row">
                <PtIcon name="upload" :size="14" class="is-ul" />
                <span class="rate__l">上传</span>
                <span class="rate__v">
                  {{ formatSize(transferStats?.total_upload_speed || 0) }}/s
                </span>
              </div>
              <PtBars
                v-if="upSeries.length > 1"
                :values="upSeries"
                :count="RATE_SAMPLE_MAX"
                :bar-width="9"
                :gap="4"
                :height="28"
                hue="var(--pt-ok)" />
            </div>
            <p class="rate__foot">
              每 5 秒一拍，最多留最近 {{ RATE_SAMPLE_MAX }} 拍（约 2 分钟）。刷新页面从零开始 ——
              后端不存速率历史，这条曲线只说明「刚刚」。
            </p>
          </PtPanel>

          <PtPanel class="hub__card" title="这一页的口径" icon="info">
            <ul class="hub__note">
              <li>
                这里列的是**下载器里的任务**，不是 pt-tools 的 RSS 任务；后者在「任务列表」。
                删除动作直接落到下载器，pt-tools 不做二次确认之外的拦截。
              </li>
              <li>
                「全部下载器」视图会逐台请求再合并；某台连不上时列表照常显示其余各台，
                并在上方挂一条部分失败提示，不静默跳过。
              </li>
              <li>
                「全部」范围最多取 {{ MAX_ALL_TASK_ROWS }} 行（手机上行卡再收到
                {{ MAX_MOBILE_CARD_ROWS }} 张），超出的条数写在页脚，不静默丢。
              </li>
            </ul>
          </PtPanel>
        </div>
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
 * 下载器控制台的皮肤。
 *
 * 这一页原来走沉浸模式（整屏，藏掉 rail / 导航列 / 页头 / 状态条），所以自己画了
 * 一条深色 hero 和一颗页内导航下拉。画板 18 把它画在外壳里 —— head 64 → bar-64 →
 * 左列 276 / 右 788 —— 那两件于是都删了：标题与动作走外壳页头，导航走导航列。
 * 留下的特殊之处只有一个：左右两栏之间可拖拽，宽度记在 localStorage 里。
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

/* ---------- 双列骨架 ---------- */
/*
 * 画板 18 的主区：左列卡 344,120 276 宽（右沿 620）→ 16 的间隔 → p-grid 636 起 788 宽
 * （右沿 1424）。内缩与栏距来自 .pt-cards --rail。
 *
 * 这里把 grid 改回 flex：侧栏宽度是用户拖出来的内联 width，flex 下它直接生效，
 * 而 grid 的轨道宽度由 grid-template-columns 说话，内联 width 压不住它 ——
 * 拖动会变成没反应。栏宽比例照画板（276 / 788）由下面的 flex-basis 表达。
 */
.hub__layout.pt-cards {
  display: flex;
  flex: 1;
  min-height: 0;
}

/* 侧栏宽度由 sidebarStyle 内联给（可拖拽），这里只管配色和滚动 */
/*
 * 左列是三张卡叠起来的一栏（画板 p-io / p-state / p-filter），
 * 所以这一层只负责纵向堆叠与滚动，边框圆角投影都归各张卡自己。
 */
.hub__side {
  display: flex;
  flex-direction: column;
  gap: var(--pt-pad);
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: var(--pt-border-strong) transparent;
}

/* 右栏：表格卡在上，两张 386 卡在下 */
.hub__right {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--pt-pad);
  min-width: 0;
}

/* 右栏下方两张 386 卡（画板 p-rate / p-note） */
.hub__bottom {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--pt-pad);
  min-width: 0;
}

.rate {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rate__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
}

.rate__l {
  flex: 1;
  color: var(--pt-t3);
}

.rate__v {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--pt-t1);
}

.rate__foot,
.hub__note {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

.hub__note {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding-left: 16px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

/* 卡片不许被 flex 压扁：内容多的那张（筛选）自己滚，别把三张一起挤扁 */
.hub__card {
  flex: 0 0 auto;
}

.hub__side::-webkit-scrollbar {
  width: 5px;
}

.hub__side::-webkit-scrollbar-thumb {
  background: var(--pt-border-strong);
  border-radius: var(--pt-radius-full);
}

.hub__side-foot {
  display: flex;
  gap: var(--pt-space-1);
  align-items: center;
  margin-top: auto;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

/* 两张卡之间的拖拽把手：16 的间隔里居中一条 2px 的线，hover 才显出来 */
.hub__resizer {
  position: relative;
  flex: 0 0 2px;
  margin: 0 -9px;
  cursor: col-resize;
  background: transparent;
  border-radius: 1px;
  transition: background var(--pt-transition-fast);
}

.hub__resizer:hover {
  background: var(--pt-p);
}

/* 内缩已经由 .hub__layout 给了，这里再加一层 padding 会让 p-grid 缩到 788 以内 */
.hub__main {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--pt-space-3);
  min-width: 0;
  overflow: hidden;
}

/* ---------- 侧栏区块 ---------- */
.hub__sec {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 卡内多个区块时彼此留 12；单区块的卡（状态）不需要这一档 */
.hub__sec + .hub__sec {
  margin-top: var(--pt-space-3);
}

/* 状态那张卡只有一组按钮，小标题由卡头顶替，所以不再留标题的位置 */
.hub__sec--flat {
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

/* ---------- 部分失败提示条 ---------- */
/* .pt-note 自带色条和底色，这里只补面板内的留白（正文是 padding="none"）与对齐 */
.hub__partial {
  align-items: flex-start;
  margin: var(--pt-space-3) var(--pt-pad);
}

.hub__partial-body {
  flex: 1;
  min-width: 0;
}

.hub__partial-t {
  margin: 0;
  font-weight: 500;
  color: var(--pt-t1);
}

.hub__partial-l {
  margin: 2px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  overflow-wrap: anywhere;
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

/* ---------- 移动端行卡列表 ---------- */
/* 面板正文是 padding="none"，所以留白由这里给；高度上限走内联的 tableMaxHeight，
   和桌面两套表格用的是同一份测量值，卡片列表自己滚，页脚照旧留在屏内 */
.hub__cards {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--pt-space-2);
  min-height: 0;
  padding: var(--pt-space-3);
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
}

/* 进度下面那行说明：左边已完成 / 总大小，右边速率与百分比 */
.hub__card-pg {
  display: flex;
  flex-wrap: wrap;
  gap: 2px var(--pt-space-2);
  align-items: center;
  justify-content: space-between;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

.hub__card-rate {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-variant-numeric: tabular-nums;
}

.hub__card-pct {
  font-weight: 600;
  color: var(--pt-t2);
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

  .hub__main {
    padding: var(--pt-space-2);
  }
}
</style>
