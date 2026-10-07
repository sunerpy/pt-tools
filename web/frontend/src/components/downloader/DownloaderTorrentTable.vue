<script setup lang="ts">
import type { DownloaderTorrentItem } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtProgress from "@/components/ui/PtProgress.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { formatShortDateTime } from "@/utils/format";
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
        | "set_tags"
        | "organize";
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

/* 进度条跟着任务状态换色：错误红、暂停黄、做种绿、其余走主色 */
function progressTone(row: DownloaderTorrentItem): "primary" | "ok" | "warn" | "dang" {
  switch (rowStateClass(row)) {
    case "state-error":
      return "dang";
    case "state-paused":
      return "warn";
    case "state-seeding":
      return "ok";
    default:
      return "primary";
  }
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
    | "set_tags"
    | "organize",
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
  <div class="dtt">
    <el-table
      :data="props.data"
      :size="tableSize()"
      class="pt-grid"
      :class="tableClassName()"
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
              <PtTag>{{ row.downloader_type }}</PtTag>
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
          class-name="title-cell pt-cell-strong">
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
            <div class="pg">
              <PtProgress :percent="row.progress" :tone="progressTone(row)" />
              <span class="pg__n">{{ Math.round(row.progress) }}%</span>
            </div>
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
          sortable="custom"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <span :title="formatDate(row.added_at)">{{
              formatShortDateTime(row.added_at * 1000)
            }}</span>
          </template>
        </el-table-column>
        <el-table-column
          v-else-if="columnKey === 'completed_at' && isVisible('completed_at')"
          label="完成日期"
          prop="completed_at"
          width="170"
          sortable="custom"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <span :title="formatDate(row.completed_at)">
              {{ formatShortDateTime(row.completed_at * 1000) }}
            </span>
          </template>
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
          class-name="pt-cell-1line"
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
      <el-table-column label="操作" width="92" class-name="action-column pt-cell-act">
        <template #default="{ row }">
          <el-button link type="primary" @click="emitDetail(row)">
            <PtIcon name="info" :size="14" /><span>详情</span>
          </el-button>
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
/*
 * 表格本体走全局的 .pt-grid 皮肤，这里只补三件皮肤管不到的事：
 * 密度切换的行高、状态色条/标题列的贴边，以及两个 teleport 出去的浮层。
 * 原先那一大段非作用域覆盖是为了把表强制染成深绿，换皮肤后整表跟着主题走，
 * 不需要再钉住 el-table 的内部变量。
 */
.dtt {
  min-width: 0;
}

/* 长列表滚动时把重排限制在表内，原来的非作用域块里带着这条，不能丢 */
.pt-grid {
  contain: layout style paint;
}

/* 状态色条：下载=ok、做种=info、暂停=warn、错误=dang，其余走最弱的文字色 */
.status-bar {
  width: 3px;
  height: 22px;
  border-radius: var(--pt-radius-full);
  margin: 0 auto;
}

.state-downloading {
  background: var(--pt-ok);
}

.state-seeding {
  background: var(--pt-info);
}

.state-paused {
  background: var(--pt-warn);
}

.state-error {
  background: var(--pt-dang);
}

.state-unknown {
  background: var(--pt-t4);
}

.dl-cell {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
  min-width: 0;
}

.dl-name {
  overflow: hidden;
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 进度：细条在左、百分比贴右，数字不换行才能和右侧数字列对齐 */
.pg {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.pg__n {
  min-width: 34px;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t2);
  text-align: right;
}

/* 密度开关只改行高：.pt-grid 的行高走 --pt-row-h，覆盖变量比覆盖内边距干净 */
.pt-grid.table-compact {
  --pt-row-h: 32px;
}

.pt-grid.table-comfortable {
  --pt-row-h: 44px;
}

/* 状态色条列宽 8，不能留 .pt-grid 的 16 内边距，否则色条被挤出格 */
.pt-grid :deep(td.el-table__cell .cell:has(.status-bar)) {
  padding: 0;
}

/* 标题列单行截断：这一列关掉了 Element 的 tooltip，改用跟随鼠标的自绘提示 */
.pt-grid :deep(.el-table__cell.title-cell) {
  overflow: hidden;
}

.pt-grid :deep(.title-text) {
  display: block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 行左缘的状态渐变：让下载/做种/错误三种行在长列表里能被扫出来 */
.pt-grid :deep(.torrent-row.state-downloading td.el-table__cell:first-child) {
  box-shadow: inset 2px 0 0 var(--pt-ok);
}

.pt-grid :deep(.torrent-row.state-seeding td.el-table__cell:first-child) {
  box-shadow: inset 2px 0 0 var(--pt-info);
}

.pt-grid :deep(.torrent-row.state-error td.el-table__cell:first-child) {
  box-shadow: inset 2px 0 0 var(--pt-dang);
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

/* 跟随鼠标的单元格提示：同样被 teleport 出去 */
.cell-follow-tooltip {
  position: fixed;
  z-index: 9999;
  max-width: 420px;
  padding: var(--pt-space-2) var(--pt-space-3);
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t1);
  word-break: break-all;
  white-space: pre-wrap;
  pointer-events: none;
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  box-shadow: var(--pt-shadow-lg);
}
</style>
