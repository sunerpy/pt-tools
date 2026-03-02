<script setup lang="ts">
import type { DownloaderTorrentItem } from "@/api";
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

type SortChangePayload = {
  prop: string;
  order: "ascending" | "descending" | null;
};

const props = defineProps<{
  data: DownloaderTorrentItem[];
  visibleColumns: string[];
  columnOrder: string[];
  density: "compact" | "comfortable";
  maxHeight?: number;
  hideContextMenuToken?: number;
}>();

const emit = defineEmits<{
  (e: "selection-change", rows: DownloaderTorrentItem[]): void;
  (e: "sort-change", payload: SortChangePayload): void;
  (e: "detail", row: DownloaderTorrentItem): void;
  (e: "header-contextmenu", payload: { x: number; y: number }): void;
  (e: "row-contextmenu-open"): void;
  (
    e: "context-action",
    payload: {
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
    },
  ): void;
}>();

const contextMenuVisible = ref(false);
const contextMenuX = ref(0);
const contextMenuY = ref(0);
const contextRow = ref<DownloaderTorrentItem | null>(null);

function isVisible(columnKey: string): boolean {
  return props.visibleColumns.includes(columnKey);
}

function rowStateClass(row: DownloaderTorrentItem): string {
  const state = (row.state || "").toLowerCase();
  if (state.includes("seed")) return "state-seeding";
  if (state.includes("download")) return "state-downloading";
  if (state.includes("pause") || state.includes("stop")) return "state-paused";
  if (state.includes("error")) return "state-error";
  return "state-unknown";
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

function onSelectionChange(rows: DownloaderTorrentItem[]) {
  emit("selection-change", rows);
}

function onSortChange(payload: SortChangePayload) {
  emit("sort-change", payload);
}

function emitDetail(row: DownloaderTorrentItem) {
  emit("detail", row);
}

function onRowContextMenu(row: DownloaderTorrentItem, _column: unknown, event: Event) {
  const mouseEvent = event as MouseEvent;
  mouseEvent.preventDefault();
  emit("row-contextmenu-open");
  contextRow.value = row;
  contextMenuX.value = mouseEvent.clientX;
  contextMenuY.value = mouseEvent.clientY;
  contextMenuVisible.value = true;
}

function isMouseEvent(value: unknown): value is MouseEvent {
  return typeof value === "object" && value !== null && "clientX" in value && "clientY" in value;
}

function onHeaderContextMenu(arg1: unknown, arg2: unknown) {
  const event = isMouseEvent(arg2) ? arg2 : isMouseEvent(arg1) ? arg1 : null;
  if (!event) {
    return;
  }
  const mouseEvent = event;
  mouseEvent.preventDefault();
  emit("header-contextmenu", { x: mouseEvent.clientX, y: mouseEvent.clientY });
}

function rowClassName(arg: { row: DownloaderTorrentItem }): string {
  return `torrent-row ${rowStateClass(arg.row)}`;
}

function hideContextMenu() {
  contextMenuVisible.value = false;
}

function emitContextAction(
  action:
    | "pause"
    | "resume"
    | "delete"
    | "delete_with_files"
    | "recheck"
    | "detail"
    | "set_category"
    | "set_tags",
) {
  if (!contextRow.value) return;
  emit("context-action", { action, row: contextRow.value });
  hideContextMenu();
}

function tableSize(): "small" | "default" {
  return props.density === "compact" ? "small" : "default";
}

function tableClassName(): string {
  return props.density === "compact" ? "table-compact" : "table-comfortable";
}

const cellTooltipVisible = ref(false);
const cellTooltipText = ref("");
const cellTooltipX = ref(0);
const cellTooltipY = ref(0);
let cellTooltipTimer: ReturnType<typeof setTimeout> | null = null;

function onCellMouseEnter(
  row: DownloaderTorrentItem,
  column: { property?: string },
  _cell: HTMLElement,
  event: MouseEvent,
) {
  if (!column.property) return;
  const value = cellDisplayValue(row, column.property);
  if (!value || value === "-") return;
  cellTooltipTimer = setTimeout(() => {
    cellTooltipText.value = value;
    cellTooltipX.value = event.clientX + 12;
    cellTooltipY.value = event.clientY + 12;
    cellTooltipVisible.value = true;
  }, 350);
}

function onCellMouseLeave() {
  if (cellTooltipTimer) {
    clearTimeout(cellTooltipTimer);
    cellTooltipTimer = null;
  }
  cellTooltipVisible.value = false;
}

function onCellMouseMove(event: MouseEvent) {
  if (cellTooltipVisible.value) {
    cellTooltipX.value = event.clientX + 12;
    cellTooltipY.value = event.clientY + 12;
  }
}

function cellDisplayValue(row: DownloaderTorrentItem, prop: string): string {
  switch (prop) {
    case "title":
      return row.title || "";
    case "downloader_name":
      return `${row.downloader_name} (${row.downloader_type})`;
    case "state":
      return row.state || "";
    case "category":
      return row.category || "";
    case "tags":
      return row.tags || "";
    case "save_path":
      return row.save_path || "";
    default:
      return "";
  }
}

onMounted(() => {
  window.addEventListener("click", hideContextMenu);
  window.addEventListener("scroll", hideContextMenu, true);
  window.addEventListener("mousemove", onCellMouseMove, { passive: true });
});

onBeforeUnmount(() => {
  window.removeEventListener("click", hideContextMenu);
  window.removeEventListener("scroll", hideContextMenu, true);
  window.removeEventListener("mousemove", onCellMouseMove);
  if (cellTooltipTimer) {
    clearTimeout(cellTooltipTimer);
  }
});

watch(
  () => props.hideContextMenuToken,
  () => {
    hideContextMenu();
  },
);
</script>

<template>
  <div class="dtt-dark-wrapper">
    <el-table
      :data="props.data"
      :size="tableSize()"
      :class="tableClassName()"
      border
      stripe
      :max-height="props.maxHeight"
      :row-key="(row: DownloaderTorrentItem) => `${row.downloader_id}:${row.task_id}`"
      :row-class-name="rowClassName"
      @cell-mouse-enter="onCellMouseEnter"
      @cell-mouse-leave="onCellMouseLeave"
      @selection-change="onSelectionChange"
      @header-contextmenu="onHeaderContextMenu"
      @row-contextmenu="onRowContextMenu"
      @sort-change="onSortChange">
      <el-table-column type="selection" width="52" :reserve-selection="true" />
      <template v-for="columnKey in props.columnOrder" :key="columnKey">
        <el-table-column
          v-if="columnKey === 'status_bar' && isVisible('status_bar')"
          label=""
          width="8">
          <template #default="{ row }">
            <div class="status-bar" :class="rowStateClass(row)" />
          </template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'downloader_name' && isVisible('downloader_name')"
          label="下载器"
          prop="downloader_name"
          min-width="180"
          sortable="custom"
          :show-overflow-tooltip="false">
          <template #default="{ row }">
            <div class="dl-cell">
              <span class="dl-name">{{ row.downloader_name }}</span>
              <el-tag size="small" effect="plain">{{ row.downloader_type }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'title' && isVisible('title')"
          label="标题"
          prop="title"
          min-width="300"
          sortable="custom"
          :show-overflow-tooltip="false"
          class-name="title-cell">
          <template #default="{ row }">
            <span class="title-text">{{ row.title }}</span>
          </template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'progress' && isVisible('progress')"
          label="进度"
          prop="progress"
          width="170"
          sortable="custom">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.progress)" :stroke-width="8" />
          </template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'seeds' && isVisible('seeds')"
          label="做种数"
          prop="seeds"
          width="90"
          align="right"
          sortable="custom" />
        <el-table-column
          v-else-if="columnKey === 'connections' && isVisible('connections')"
          label="连接数"
          prop="connections"
          width="90"
          align="right"
          sortable="custom" />
        <el-table-column
          v-else-if="columnKey === 'size' && isVisible('size')"
          label="大小"
          prop="size"
          width="120"
          align="right"
          sortable="custom">
          <template #default="{ row }">{{ formatSize(row.size) }}</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'upload_speed' && isVisible('upload_speed')"
          label="上传速度"
          prop="upload_speed"
          width="130"
          align="right"
          sortable="custom">
          <template #default="{ row }">{{ formatSize(row.upload_speed) }}/s</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'download_speed' && isVisible('download_speed')"
          label="下载速度"
          prop="download_speed"
          width="130"
          align="right"
          sortable="custom">
          <template #default="{ row }">{{ formatSize(row.download_speed) }}/s</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'added_at' && isVisible('added_at')"
          label="添加日期"
          prop="added_at"
          width="170"
          sortable="custom">
          <template #default="{ row }">{{ formatDate(row.added_at) }}</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'completed_at' && isVisible('completed_at')"
          label="完成日期"
          prop="completed_at"
          width="170"
          sortable="custom">
          <template #default="{ row }">{{ formatDate(row.completed_at) }}</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'ratio' && isVisible('ratio')"
          label="分享率"
          prop="ratio"
          width="90"
          align="right"
          sortable="custom">
          <template #default="{ row }">{{ formatRatio(row.ratio) }}</template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'state' && isVisible('state')"
          label="状态"
          prop="state"
          width="120"
          sortable="custom"
          :show-overflow-tooltip="false" />
        <el-table-column
          v-else-if="columnKey === 'eta' && isVisible('eta')"
          label="ETA"
          prop="eta"
          width="100"
          sortable="custom"
          align="right" />
        <el-table-column
          v-else-if="columnKey === 'category' && isVisible('category')"
          label="分类"
          prop="category"
          width="120"
          sortable="custom"
          :show-overflow-tooltip="false" />
        <el-table-column
          v-else-if="columnKey === 'tags' && isVisible('tags')"
          label="标签"
          prop="tags"
          width="150"
          sortable="custom"
          :show-overflow-tooltip="false" />
      </template>
      <el-table-column label="操作" width="92" class-name="action-column">
        <template #default="{ row }">
          <el-button text type="primary" @click="emitDetail(row)">详情</el-button>
        </template>
      </el-table-column>
    </el-table>

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
    <teleport to="body">
      <div
        v-if="cellTooltipVisible"
        class="cell-follow-tooltip"
        :style="{ left: `${cellTooltipX}px`, top: `${cellTooltipY}px` }">
        {{ cellTooltipText }}
      </div>
    </teleport>
  </div>
</template>

<style scoped>
.status-bar {
  width: 4px;
  height: 26px;
  border-radius: 999px;
  margin: 0 auto;
}

.state-downloading {
  background: #7aca47;
}

.state-seeding {
  background: #00b3fa;
}

.state-paused {
  background: #f57c00;
}

.state-error {
  background: #d32f2f;
}

.state-unknown {
  background: #616161;
}

.dl-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.dl-name {
  font-weight: 600;
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
  letter-spacing: 0.3px;
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
  color: var(--el-color-danger);
}
</style>

<!-- NON-SCOPED: overrides Element Plus el-table internal CSS variables -->
<style>
.dtt-dark-wrapper {
  --el-fill-color-blank: #2f5e50;
  --el-bg-color: #2f5e50;
  --el-color-white: #2f5e50;
}

.dtt-dark-wrapper .el-table {
  contain: layout style paint;
  --el-table-bg-color: #2f5e50;
  --el-table-tr-bg-color: rgba(255, 255, 255, 0.01);
  --el-table-expanded-cell-bg-color: #2f5e50;
  --el-table-header-bg-color: rgba(255, 255, 255, 0.08);
  --el-table-header-text-color: #eefaf4;
  --el-table-text-color: #e5f3ec;
  --el-table-border-color: rgba(255, 255, 255, 0.14);
  --el-table-row-hover-bg-color: rgba(100, 206, 170, 0.12);
  --el-table-current-row-bg-color: rgba(100, 206, 170, 0.16);
  background-color: #2f5e50;
}

.dtt-dark-wrapper .el-table td.el-table__cell,
.dtt-dark-wrapper .el-table th.el-table__cell {
  background-color: transparent !important;
}

.dtt-dark-wrapper .el-table--striped .el-table__body tr.el-table__row--striped td.el-table__cell {
  background-color: rgba(255, 255, 255, 0.03) !important;
}

.dtt-dark-wrapper .el-table__body tr:hover > td.el-table__cell {
  background-color: rgba(100, 206, 170, 0.12) !important;
}

.dtt-dark-wrapper .el-table__body tr.current-row > td.el-table__cell {
  background-color: rgba(100, 206, 170, 0.16) !important;
}

.dtt-dark-wrapper .el-table__inner-wrapper,
.dtt-dark-wrapper .el-table__header-wrapper,
.dtt-dark-wrapper .el-table__body-wrapper,
.dtt-dark-wrapper .el-table__fixed,
.dtt-dark-wrapper .el-table__fixed-right,
.dtt-dark-wrapper .el-table__fixed-body-wrapper {
  background-color: #2f5e50 !important;
}

.dtt-dark-wrapper .el-table__empty-block,
.dtt-dark-wrapper .el-table__empty-text {
  background-color: #2f5e50;
  color: #e5f3ec;
}

.dtt-dark-wrapper .el-table__inner-wrapper::before {
  background-color: rgba(255, 255, 255, 0.14) !important;
}

.dtt-dark-wrapper .el-table.table-compact .el-table__cell {
  padding-top: 5px;
  padding-bottom: 5px;
}

.dtt-dark-wrapper .el-table.table-comfortable .el-table__cell {
  padding-top: 9px;
  padding-bottom: 9px;
}

.dtt-dark-wrapper .el-table .torrent-row.state-downloading .el-table__cell {
  background: linear-gradient(90deg, rgba(122, 202, 71, 0.06), transparent 20%) !important;
}

.dtt-dark-wrapper .el-table .torrent-row.state-seeding .el-table__cell {
  background: linear-gradient(90deg, rgba(0, 179, 250, 0.06), transparent 20%) !important;
}

.dtt-dark-wrapper .el-table .torrent-row.state-error .el-table__cell {
  background: linear-gradient(90deg, rgba(211, 47, 47, 0.06), transparent 20%) !important;
  background: linear-gradient(90deg, rgba(211, 47, 47, 0.06), transparent 20%) !important;
}

.cell-follow-tooltip {
  position: fixed;
  z-index: 9999;
  max-width: 420px;
  padding: 8px 14px;
  font-size: 13px;
  line-height: 1.5;
  color: #f0fff8;
  background: #1a3d32;
  border: 1px solid rgba(100, 206, 170, 0.3);
  border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0, 0, 0, 0.45);
  pointer-events: none;
  word-break: break-all;
  white-space: pre-wrap;
}

.dtt-dark-wrapper .el-progress__text {
  color: #e8f6ef !important;
  font-weight: 700;
  font-size: 12px !important;
}

.dtt-dark-wrapper .el-progress-bar__outer {
  background-color: rgba(255, 255, 255, 0.14) !important;
}

.dtt-dark-wrapper .el-progress-bar__inner {
  background-color: #64ceaa !important;
}

.dtt-dark-wrapper .el-table td.action-column,
.dtt-dark-wrapper .el-table th.action-column,
.dtt-dark-wrapper .el-table__fixed-right-patch,
.dtt-dark-wrapper .el-table__fixed-right .el-table__cell,
.dtt-dark-wrapper .el-table__fixed-right td.el-table__cell,
.dtt-dark-wrapper .el-table__fixed-right th.el-table__cell {
  background-color: #2f5e50 !important;
}

.dtt-dark-wrapper .el-table .el-table__row:hover td.action-column,
.dtt-dark-wrapper .el-table__fixed-right .el-table__row:hover > td.el-table__cell {
  background-color: #2f6b58 !important;
}

.dtt-dark-wrapper .el-table .el-table__cell.title-cell {
  overflow: hidden;
}

.dtt-dark-wrapper .el-table .title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}
</style>
