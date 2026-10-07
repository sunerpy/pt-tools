<script setup lang="ts">
import type { DownloaderTorrentItem } from "@/api";
import { formatShortDateTime } from "@/utils/format";
import { computed, ref, watch } from "vue";

type RowAction =
  | "pause"
  | "resume"
  | "delete"
  | "delete_with_files"
  | "recheck"
  | "detail"
  | "set_category"
  | "set_tags"
  | "organize";

const props = defineProps<{
  data: DownloaderTorrentItem[];
  allData: DownloaderTorrentItem[];
  selectedRowKeys: string[];
  visibleColumns: string[];
  columnOrder: string[];
  density: "compact" | "comfortable";
  sortBy: string;
  sortOrder: "asc" | "desc";
  hideContextMenuToken?: number;
}>();

const emit = defineEmits<{
  (e: "selection-change", rows: DownloaderTorrentItem[]): void;
  (e: "selection-keys-change", keys: string[]): void;
  (e: "sort-change", payload: { prop: string; order: "ascending" | "descending" | null }): void;
  (e: "detail", row: DownloaderTorrentItem): void;
  (e: "header-contextmenu", payload: { x: number; y: number }): void;
  (e: "row-contextmenu-open"): void;
  (e: "context-action", payload: { action: RowAction; row: DownloaderTorrentItem }): void;
}>();

const contextMenuVisible = ref(false);
const contextMenuX = ref(0);
const contextMenuY = ref(0);
const contextRow = ref<DownloaderTorrentItem | null>(null);

type ColumnDef = {
  key: string;
  label: string;
  width: number;
  align?: "left" | "right" | "center";
  sortable?: boolean;
};

const columnDefs: Record<string, ColumnDef> = {
  status_bar: { key: "status_bar", label: "", width: 8 },
  downloader_name: { key: "downloader_name", label: "下载器", width: 180, sortable: true },
  title: { key: "title", label: "标题", width: 340, sortable: true },
  progress: { key: "progress", label: "进度", width: 180, sortable: true },
  seeds: { key: "seeds", label: "做种数", width: 90, align: "right", sortable: true },
  connections: { key: "connections", label: "连接数", width: 90, align: "right", sortable: true },
  size: { key: "size", label: "大小", width: 120, align: "right", sortable: true },
  upload_speed: {
    key: "upload_speed",
    label: "上传速度",
    width: 130,
    align: "right",
    sortable: true,
  },
  download_speed: {
    key: "download_speed",
    label: "下载速度",
    width: 130,
    align: "right",
    sortable: true,
  },
  added_at: { key: "added_at", label: "添加日期", width: 170, sortable: true },
  completed_at: { key: "completed_at", label: "完成日期", width: 170, sortable: true },
  ratio: { key: "ratio", label: "分享率", width: 90, align: "right", sortable: true },
  state: { key: "state", label: "状态", width: 120, sortable: true },
  eta: { key: "eta", label: "ETA", width: 100, align: "right", sortable: true },
  category: { key: "category", label: "分类", width: 120, sortable: true },
  tags: { key: "tags", label: "标签", width: 150, sortable: true },
};

const orderedColumns = computed(() =>
  props.columnOrder
    .filter((key) => props.visibleColumns.includes(key))
    .map((key) => columnDefs[key])
    .filter((value): value is ColumnDef => Boolean(value)),
);

const gridTemplateColumns = computed(() => {
  const widths = ["44px", ...orderedColumns.value.map((column) => `${column.width}px`), "88px"];
  return widths.join(" ");
});

const rowClass = computed(() => (props.density === "compact" ? "row-compact" : "row-comfortable"));

function rowKey(row: DownloaderTorrentItem): string {
  return `${row.downloader_id}:${row.task_id}`;
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i += 1;
  }
  return `${size.toFixed(2)} ${units[i]}`;
}

function formatDate(ts: number): string {
  if (!ts || ts <= 0) return "-";
  return new Date(ts * 1000).toLocaleString("zh-CN");
}

function formatRatio(value: number): string {
  if (value < 0) return "-";
  return value.toFixed(2);
}

function rowStateClass(row: DownloaderTorrentItem): string {
  const state = (row.state || "").toLowerCase();
  if (state.includes("seed")) return "state-seeding";
  if (state.includes("download")) return "state-downloading";
  if (state.includes("pause") || state.includes("stop")) return "state-paused";
  if (state.includes("error")) return "state-error";
  return "state-unknown";
}

function cellValue(row: DownloaderTorrentItem, key: string): string {
  switch (key) {
    case "status_bar":
      return "";
    case "downloader_name":
      return `${row.downloader_name} (${row.downloader_type})`;
    case "title":
      return row.title;
    case "progress":
      return `${Math.round(row.progress)}%`;
    case "seeds":
      return String(row.seeds);
    case "connections":
      return String(row.connections);
    case "size":
      return formatSize(row.size);
    case "upload_speed":
      return `${formatSize(row.upload_speed)}/s`;
    case "download_speed":
      return `${formatSize(row.download_speed)}/s`;
    /* 紧凑格式单行（toLocaleString 的 19 个字符在 170 宽里放大字号就折行），完整时间在格子的 title */
    case "added_at":
      return formatShortDateTime(row.added_at * 1000);
    case "completed_at":
      return formatShortDateTime(row.completed_at * 1000);
    case "ratio":
      return formatRatio(row.ratio);
    case "state":
      return row.state || "-";
    case "eta":
      return row.eta ? String(row.eta) : "-";
    case "category":
      return row.category || "-";
    case "tags":
      return row.tags || "-";
    default:
      return "-";
  }
}

function onHeaderContextMenu(event: MouseEvent) {
  event.preventDefault();
  emit("header-contextmenu", { x: event.clientX, y: event.clientY });
}

function onHeaderSort(column: ColumnDef) {
  if (!column.sortable) {
    return;
  }
  if (props.sortBy === column.key) {
    emit("sort-change", {
      prop: column.key,
      order: props.sortOrder === "asc" ? "descending" : "ascending",
    });
    return;
  }
  emit("sort-change", { prop: column.key, order: "descending" });
}

function sortIndicator(column: ColumnDef): string {
  if (!column.sortable) return "";
  if (props.sortBy !== column.key) return "↕";
  return props.sortOrder === "asc" ? "↑" : "↓";
}

function hideContextMenu() {
  contextMenuVisible.value = false;
}

function onRowContextMenu(row: DownloaderTorrentItem, event: MouseEvent) {
  event.preventDefault();
  emit("row-contextmenu-open");
  contextRow.value = row;
  contextMenuX.value = event.clientX;
  contextMenuY.value = event.clientY;
  contextMenuVisible.value = true;
}

function emitContextAction(action: RowAction) {
  if (!contextRow.value) return;
  emit("context-action", { action, row: contextRow.value });
  hideContextMenu();
}

function toggleSelection(row: DownloaderTorrentItem, checked: boolean) {
  const key = rowKey(row);
  const next = new Set(props.selectedRowKeys);
  if (checked) {
    next.add(key);
  } else {
    next.delete(key);
  }
  emit("selection-keys-change", [...next]);
  emit(
    "selection-change",
    props.allData.filter((item) => next.has(rowKey(item))),
  );
}

function isChecked(row: DownloaderTorrentItem): boolean {
  return props.selectedRowKeys.includes(rowKey(row));
}

function onRowCheckboxChange(row: DownloaderTorrentItem, event: Event) {
  const target = event.target as HTMLInputElement | null;
  toggleSelection(row, Boolean(target?.checked));
}

watch(
  () => props.allData,
  (rows) => {
    const currentKeys = new Set(props.selectedRowKeys);
    const validKeys = new Set(rows.map((row) => rowKey(row)));
    const next = new Set([...currentKeys].filter((key) => validKeys.has(key)));
    if (next.size !== currentKeys.size) {
      emit("selection-keys-change", [...next]);
      emit(
        "selection-change",
        rows.filter((item) => next.has(rowKey(item))),
      );
    }
  },
  { deep: false },
);

watch(
  () => props.hideContextMenuToken,
  () => {
    hideContextMenu();
  },
);
</script>

<template>
  <div class="virtual-table" @contextmenu.prevent>
    <div class="vt-header" :style="{ gridTemplateColumns }" @contextmenu="onHeaderContextMenu">
      <div class="vt-cell vt-checkbox" />
      <div
        v-for="column in orderedColumns"
        :key="`header-${column.key}`"
        class="vt-cell"
        :class="[`align-${column.align || 'left'}`, column.sortable ? 'sortable' : '']"
        @click="onHeaderSort(column)">
        <span>{{ column.label }}</span>
        <span v-if="column.sortable" class="sort-indicator">{{ sortIndicator(column) }}</span>
      </div>
      <div class="vt-cell align-center vt-action-header">操作</div>
    </div>

    <div
      v-for="row in props.data"
      :key="rowKey(row)"
      class="vt-row"
      :class="[rowClass, rowStateClass(row)]"
      :style="{ gridTemplateColumns }"
      @contextmenu="(event) => onRowContextMenu(row, event)">
      <label class="vt-cell vt-checkbox align-center">
        <input
          type="checkbox"
          :checked="isChecked(row)"
          @change="onRowCheckboxChange(row, $event)" />
      </label>
      <div
        v-for="column in orderedColumns"
        :key="`${rowKey(row)}-${column.key}`"
        class="vt-cell"
        :class="[
          `align-${column.align || 'left'}`,
          column.key === 'status_bar' ? rowStateClass(row) : '',
        ]"
        :title="
          column.key === 'added_at' || column.key === 'completed_at'
            ? formatDate(row[column.key])
            : undefined
        ">
        <template v-if="column.key === 'status_bar'">
          <div class="status-bar" :class="rowStateClass(row)" />
        </template>
        <template v-else-if="column.key === 'progress'">
          <div class="progress-cell">
            <div class="progress-track">
              <span class="progress-fill" :style="{ width: `${Math.round(row.progress)}%` }" />
            </div>
            <span>{{ Math.round(row.progress) }}%</span>
          </div>
        </template>
        <template v-else>
          {{ cellValue(row, column.key) }}
        </template>
      </div>
      <div class="vt-cell align-center vt-action-cell">
        <button type="button" class="detail-btn" @click="emit('detail', row)">详情</button>
      </div>
    </div>
  </div>

  <teleport to="body">
    <div
      v-if="contextMenuVisible"
      class="table-context-menu"
      :style="{ left: `${contextMenuX}px`, top: `${contextMenuY}px` }"
      @click.stop>
      <div class="menu-group-title">查看</div>
      <button type="button" @click="emitContextAction('detail')">查看详情</button>
      <div class="menu-divider" />
      <div class="menu-group-title">任务控制</div>
      <button type="button" @click="emitContextAction('pause')">暂停</button>
      <button type="button" @click="emitContextAction('resume')">开始</button>
      <button type="button" @click="emitContextAction('recheck')">复检</button>
      <div class="menu-divider" />
      <div class="menu-group-title">媒体</div>
      <button type="button" @click="emitContextAction('organize')">整理入库</button>
      <div class="menu-divider" />
      <div class="menu-group-title">分类/标签</div>
      <button type="button" @click="emitContextAction('set_category')">设置分类</button>
      <button type="button" @click="emitContextAction('set_tags')">设置标签</button>
      <div class="menu-group-title">危险操作</div>
      <button type="button" class="danger" @click="emitContextAction('delete')">删除任务</button>
      <button type="button" class="danger" @click="emitContextAction('delete_with_files')">
        删除任务和文件
      </button>
    </div>
  </teleport>
</template>

<style scoped>
/*
 * 虚拟表格是手写的 grid，不走 .pt-grid 皮肤（那层挂在 el-table 上）。
 * 这里刻意保持和 .pt-grid 一致的度量：表头 30 高走 hover 底 + borderStrong 下沿，
 * 行线走 border，字号 13/表头 11，读起来才和其他页的网格是同一张表。
 */
.virtual-table {
  min-width: max-content;
  font-size: var(--pt-fz-body);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
}

.vt-header,
.vt-row {
  display: grid;
  align-items: center;
  border-bottom: 1px solid var(--pt-border);
}

.vt-header {
  position: sticky;
  top: 0;
  z-index: 2;
  height: var(--pt-row-head-h);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-bottom: 1px solid var(--pt-border-strong);
}

.vt-row.row-compact {
  min-height: 40px;
}
.vt-row.row-comfortable {
  min-height: 48px;
}

.vt-row:hover {
  background: var(--pt-hover);
}

.vt-cell {
  padding: 0 var(--pt-space-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.vt-header .vt-cell.sortable {
  cursor: pointer;
  user-select: none;
  display: inline-flex;
  align-items: center;
  gap: var(--pt-space-2);
  height: 100%;
}

.vt-header .vt-cell.sortable:hover {
  color: var(--pt-t1);
}

.sort-indicator {
  font-size: var(--pt-fz-label);
  color: var(--pt-p);
}

.vt-checkbox {
  display: flex;
  align-items: center;
  justify-content: center;
}

/* 原生复选框：这里用不起 el-checkbox（一屏上千个组件实例），accent-color 就够换色 */
.vt-checkbox input {
  accent-color: var(--pt-p);
  cursor: pointer;
}

.align-left {
  text-align: left;
}
.align-right {
  text-align: right;
}
.align-center {
  text-align: center;
}

/* 状态色条：下载=ok、做种=info、暂停=warn、错误=dang，其余走最弱的文字色 */
.status-bar {
  width: 3px;
  height: 22px;
  border-radius: var(--pt-radius-full);
  margin: 0 auto;
}

.state-downloading .status-bar {
  background: var(--pt-ok);
}
.state-seeding .status-bar {
  background: var(--pt-info);
}
.state-paused .status-bar {
  background: var(--pt-warn);
}
.state-error .status-bar {
  background: var(--pt-dang);
}
.state-unknown .status-bar {
  background: var(--pt-t4);
}

/*
 * 进度条这里没有换成 PtProgress：虚拟列表一屏能画上千行，
 * 每行多一个组件实例比多两个 div 贵得多，度量照 PtProgress 抄就够了。
 */
.progress-cell {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
}

.progress-track {
  flex: 1;
  height: 5px;
  border-radius: var(--pt-radius-full);
  background: var(--pt-border-strong);
}

html.dark .progress-track {
  background: var(--pt-border);
}

.progress-fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--pt-p);
}

.progress-cell span {
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t2);
}

.detail-btn {
  border: none;
  background: transparent;
  color: var(--pt-p);
  font-size: var(--pt-fz-sm);
  cursor: pointer;
}

.detail-btn:hover {
  text-decoration: underline;
}

/* 右键菜单被 teleport 到 body，但仍带作用域属性；配色只能用全局令牌 */
.table-context-menu {
  position: fixed;
  z-index: 3000;
  min-width: 180px;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: var(--pt-space-1);
  font-size: var(--pt-fz-sm);
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-lg);
}

.menu-group-title {
  padding: var(--pt-space-2) var(--pt-space-2) var(--pt-space-1);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
}

.menu-divider {
  height: 1px;
  margin: var(--pt-space-1) 0;
  background: var(--pt-border);
}

.table-context-menu button {
  text-align: left;
  border: none;
  background: transparent;
  color: var(--pt-t1);
  font-size: var(--pt-fz-sm);
  border-radius: var(--pt-r-sm);
  padding: 6px var(--pt-space-2);
  cursor: pointer;
}

.table-context-menu button:hover {
  background: var(--pt-hover);
}

.table-context-menu button.danger {
  color: var(--pt-dang);
}
</style>
