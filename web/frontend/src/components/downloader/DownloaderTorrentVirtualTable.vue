<script setup lang="ts">
import type { DownloaderTorrentItem } from "@/api";
import { computed, ref, watch } from "vue";

type RowAction =
  | "pause"
  | "resume"
  | "delete"
  | "delete_with_files"
  | "recheck"
  | "detail"
  | "set_category"
  | "set_tags";

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
    case "added_at":
      return formatDate(row.added_at);
    case "completed_at":
      return formatDate(row.completed_at);
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
        ]">
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
.virtual-table {
  min-width: max-content;
  color: #dce8e2;
}

.vt-header,
.vt-row {
  display: grid;
  align-items: center;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.vt-header {
  position: sticky;
  top: 0;
  z-index: 2;
  background: #2f5e50;
  font-size: 12px;
  font-weight: 700;
}

.vt-row {
  background: rgba(255, 255, 255, 0.01);
}

.vt-row.row-compact {
  min-height: 40px;
}
.vt-row.row-comfortable {
  min-height: 48px;
}

.vt-row:hover {
  background: rgba(255, 255, 255, 0.05);
}

.vt-cell {
  padding: 6px 10px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.vt-header .vt-cell.sortable {
  cursor: pointer;
  user-select: none;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.vt-header .vt-cell.sortable:hover {
  color: #f4fff9;
  background: rgba(255, 255, 255, 0.05);
}

.sort-indicator {
  font-size: 12px;
  color: #8fdcc4;
}

.vt-checkbox {
  display: flex;
  align-items: center;
  justify-content: center;
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

.status-bar {
  width: 4px;
  height: 22px;
  border-radius: 999px;
  margin: 0 auto;
}

.state-downloading .status-bar {
  background: #7aca47;
}
.state-seeding .status-bar {
  background: #00b3fa;
}
.state-paused .status-bar {
  background: #f57c00;
}
.state-error .status-bar {
  background: #d32f2f;
}
.state-unknown .status-bar {
  background: #616161;
}

.progress-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.progress-track {
  flex: 1;
  height: 7px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
}

.progress-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: #64ceaa;
}

.vt-action-header,
.vt-action-cell {
  background-color: #2f5e50;
}

.vt-row:hover .vt-action-cell {
  background-color: rgba(100, 206, 170, 0.08);
}

.detail-btn {
  border: none;
  background: transparent;
  color: #8dd9c1;
  cursor: pointer;
}

.table-context-menu {
  position: fixed;
  z-index: 3000;
  min-width: 180px;
  background: #306052;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 10px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.menu-group-title {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  font-weight: 600;
  padding: 4px 8px 2px;
}

.menu-divider {
  height: 1px;
  background: rgba(255, 255, 255, 0.08);
  margin: 2px 0;
}

.table-context-menu button {
  text-align: left;
  border: none;
  background: transparent;
  color: #e0e0e0;
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
}

.table-context-menu button:hover {
  background: rgba(255, 255, 255, 0.08);
}

.table-context-menu button.danger {
  color: #f56c6c;
}

.progress-cell span {
  color: #e8f6ef;
  font-weight: 700;
  font-size: 12px;
}

.progress-track {
  flex: 1;
  height: 7px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.14);
}

.progress-fill {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: #64ceaa;
}
</style>
