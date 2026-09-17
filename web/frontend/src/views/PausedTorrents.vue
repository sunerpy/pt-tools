<script setup lang="ts">
import { globalApi, type ArchiveTorrent, type PausedTorrent, pausedTorrentsApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

const TAB_OPTIONS = [
  { label: "暂停中", value: "paused" },
  { label: "历史归档", value: "archive" },
];

const activeTab = ref("paused");
const loading = ref(false);
const autoRefresh = ref(false);
const refreshTimer = ref<number | null>(null);
const autoDeleteOnFreeEnd = ref(false);
const savingAutoDelete = ref(false);

const pausedTorrents = ref<PausedTorrent[]>([]);
const pausedTotal = ref(0);
const pausedPage = ref(1);
const pausedPageSize = ref(20);

const archiveTorrents = ref<ArchiveTorrent[]>([]);
const archiveTotal = ref(0);
const archivePage = ref(1);
const archivePageSize = ref(20);

const siteFilter = ref("");
const selectedIds = ref<number[]>([]);

const deleteDialogVisible = ref(false);
const deleteTarget = ref<PausedTorrent | null>(null);

const siteOptions = computed(() => {
  const sites = new Set<string>();
  pausedTorrents.value.forEach((t) => {
    if (t.site_name) sites.add(t.site_name);
  });
  return Array.from(sites);
});

onMounted(async () => {
  await loadPausedTorrents();
  try {
    const settings = await globalApi.get();
    autoDeleteOnFreeEnd.value = settings.auto_delete_on_free_end ?? false;
  } catch {
    // ignore
  }
});

onUnmounted(() => {
  if (refreshTimer.value) {
    clearInterval(refreshTimer.value);
    refreshTimer.value = null;
  }
});

watch(autoRefresh, (val) => {
  if (val) {
    refreshTimer.value = window.setInterval(() => {
      if (activeTab.value === "paused") {
        loadPausedTorrents();
      } else {
        loadArchiveTorrents();
      }
    }, 30000);
    ElMessage.success("已开启自动刷新（30秒）");
  } else {
    if (refreshTimer.value) {
      clearInterval(refreshTimer.value);
      refreshTimer.value = null;
    }
    ElMessage.info("已关闭自动刷新");
  }
});

async function loadPausedTorrents() {
  loading.value = true;
  try {
    const data = await pausedTorrentsApi.list(
      pausedPage.value,
      pausedPageSize.value,
      siteFilter.value || undefined,
    );
    pausedTorrents.value = data.items || [];
    pausedTotal.value = data.total || 0;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
}

async function loadArchiveTorrents() {
  loading.value = true;
  try {
    const data = await pausedTorrentsApi.listArchive(
      archivePage.value,
      archivePageSize.value,
      siteFilter.value || undefined,
    );
    archiveTorrents.value = data.items || [];
    archiveTotal.value = data.total || 0;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
}

function handleTabChange(tab: string) {
  if (tab === "paused") {
    loadPausedTorrents();
  } else {
    loadArchiveTorrents();
  }
}

function handlePausedPageChange(newPage: number) {
  pausedPage.value = newPage;
  loadPausedTorrents();
}

function handlePausedSizeChange(newSize: number) {
  pausedPageSize.value = newSize;
  pausedPage.value = 1;
  loadPausedTorrents();
}

function handleArchivePageChange(newPage: number) {
  archivePage.value = newPage;
  loadArchiveTorrents();
}

function handleArchiveSizeChange(newSize: number) {
  archivePageSize.value = newSize;
  archivePage.value = 1;
  loadArchiveTorrents();
}

function handleSelectionChange(selection: PausedTorrent[]) {
  selectedIds.value = selection.map((t) => t.id);
}

async function resumeTorrent(torrent: PausedTorrent) {
  try {
    await ElMessageBox.confirm(`确定恢复下载 "${torrent.title}"？`, "确认恢复", {
      confirmButtonText: "恢复",
      cancelButtonText: "取消",
      type: "info",
    });

    const result = await pausedTorrentsApi.resume(torrent.id);
    if (result.success) {
      ElMessage.success("已恢复下载");
      await loadPausedTorrents();
    } else {
      ElMessage.error(result.message || "恢复失败");
    }
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "恢复失败");
    }
  }
}

async function performDelete(ids: number[], removeData: boolean) {
  try {
    const result = await pausedTorrentsApi.delete({ ids, remove_data: removeData });
    if (result.success > 0) {
      ElMessage.success(`成功删除 ${result.success} 个任务`);
    }
    if (result.failed > 0) {
      ElMessage.warning(`${result.failed} 个任务删除失败`);
    }
    selectedIds.value = [];
    await loadPausedTorrents();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "删除失败");
  }
}

async function deleteTorrents(ids: number[], removeData: boolean) {
  try {
    const count = ids.length;
    const dataHint = removeData ? "（包含数据文件）" : "（保留数据文件）";
    await ElMessageBox.confirm(`确定删除 ${count} 个暂停任务${dataHint}？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });

    await performDelete(ids, removeData);
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "操作取消");
    }
  }
}

function openDeleteDialog(row: PausedTorrent) {
  deleteTarget.value = row;
  deleteDialogVisible.value = true;
}

async function confirmDeleteRow(removeData: boolean) {
  if (!deleteTarget.value) return;
  deleteDialogVisible.value = false;
  await performDelete([deleteTarget.value.id], removeData);
}

async function toggleAutoDelete(val: boolean) {
  savingAutoDelete.value = true;
  try {
    const current = await globalApi.get();
    await globalApi.save({ ...current, auto_delete_on_free_end: val });
    autoDeleteOnFreeEnd.value = val;
    ElMessage.success(val ? "已开启免费结束自动删除" : "已关闭免费结束自动删除");
  } catch (e: unknown) {
    autoDeleteOnFreeEnd.value = !val;
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    savingAutoDelete.value = false;
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let unitIndex = 0;
  let size = bytes;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(2)} ${units[unitIndex]}`;
}

function getDownloadedSize(torrent: PausedTorrent): string {
  const downloaded = torrent.torrent_size * (torrent.progress / 100);
  return formatSize(downloaded);
}

/* el-progress 把 color 写成内联 background-color，var() 能解析，进度条跟着配色走 */
function getProgressColor(percentage: number): string {
  if (percentage < 30) return "var(--pt-dang)";
  if (percentage < 70) return "var(--pt-warn)";
  return "var(--pt-ok)";
}

function formatTime(timeStr: string | undefined): string {
  if (!timeStr || timeStr === "0001-01-01T00:00:00Z") return "-";
  try {
    return new Date(timeStr).toLocaleString("zh-CN");
  } catch {
    return timeStr;
  }
}

function formatProgress(progress: number): string {
  return `${progress.toFixed(1)}%`;
}
</script>

<template>
  <div class="paused-page">
    <PtToolbar standalone>
      <el-segmented
        v-model="activeTab"
        class="pt-seg"
        :options="TAB_OPTIONS"
        @change="handleTabChange" />
      <el-select
        v-model="siteFilter"
        placeholder="全部站点"
        clearable
        class="filter-select"
        @change="handleTabChange(activeTab)">
        <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
      </el-select>

      <el-tooltip content="每 30 秒自动刷新当前列表" placement="bottom">
        <label class="ctl">
          <el-switch v-model="autoRefresh" size="small" />
          <span>自动刷新</span>
        </label>
      </el-tooltip>
      <el-tooltip
        content="开启后，免费期结束时未完成的种子将自动从下载器删除（含数据文件）"
        placement="bottom">
        <label class="ctl">
          <el-switch
            v-model="autoDeleteOnFreeEnd"
            size="small"
            :loading="savingAutoDelete"
            style="--el-switch-on-color: var(--pt-dang)"
            @change="toggleAutoDelete" />
          <span>免费结束自动删除</span>
        </label>
      </el-tooltip>

      <template #right>
        <el-button type="primary" :loading="loading" @click="handleTabChange(activeTab)">
          <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtPanel
      v-if="activeTab === 'paused'"
      v-loading="loading"
      title="暂停中"
      icon="circle-pause"
      :count="`${pausedTotal} 个`"
      padding="none">
      <template v-if="selectedIds.length > 0" #actions>
        <span class="sel-count">已选 {{ selectedIds.length }} 项</span>
        <el-button type="danger" size="small" plain @click="deleteTorrents(selectedIds, false)">
          <PtIcon name="trash-2" :size="14" /><span>删除任务</span>
        </el-button>
        <el-button type="danger" size="small" @click="deleteTorrents(selectedIds, true)">
          <PtIcon name="trash-2" :size="14" /><span>连数据一起删</span>
        </el-button>
      </template>

      <el-table
        :data="pausedTorrents"
        class="pt-grid"
        style="width: 100%"
        @selection-change="handleSelectionChange">
        <template #empty>
          <PtDataState
            :state="siteFilter ? 'zero' : 'empty'"
            dense
            sub="免费期结束时被暂停的种子会出现在这里" />
        </template>

        <el-table-column type="selection" width="46" />
        <el-table-column label="站点" prop="site_name" width="110">
          <template #default="{ row }">
            <PtTag>{{ row.site_name || "-" }}</PtTag>
          </template>
        </el-table-column>
        <el-table-column label="标题" min-width="240" class-name="pt-cell-strong">
          <template #default="{ row }">
            <el-tooltip :content="row.title" placement="top" :show-after="500">
              <span class="title-text">{{ row.title }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="进度" width="180">
          <template #default="{ row }">
            <div class="progress-cell">
              <el-progress
                :percentage="Math.round(row.progress)"
                :stroke-width="4"
                :show-text="false"
                :color="getProgressColor(row.progress)" />
              <span class="progress-info">
                <span>{{ getDownloadedSize(row) }} / {{ formatSize(row.torrent_size) }}</span>
                <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          label="大小"
          width="96"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatSize(row.torrent_size) }}</template>
        </el-table-column>
        <el-table-column label="下载器" width="110" class-name="pt-cell-muted">
          <template #default="{ row }">{{ row.downloader_name || "-" }}</template>
        </el-table-column>
        <el-table-column label="暂停原因" min-width="140" class-name="pt-cell-muted">
          <template #default="{ row }">{{ row.pause_reason || "-" }}</template>
        </el-table-column>
        <el-table-column label="暂停时间" width="150" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatTime(row.paused_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="resumeTorrent(row)">
              <PtIcon name="play" :size="14" /><span>恢复</span>
            </el-button>
            <el-button link type="danger" size="small" @click="openDeleteDialog(row)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="pausedTotal > 0" #footer>
        <span class="pt-foot-note">恢复会把种子重新交给原下载器继续下载</span>
        <el-pagination
          v-model:current-page="pausedPage"
          v-model:page-size="pausedPageSize"
          class="pt-pager"
          :page-sizes="[10, 20, 50, 100]"
          :total="pausedTotal"
          :pager-count="5"
          layout="sizes, prev, pager, next"
          @size-change="handlePausedSizeChange"
          @current-change="handlePausedPageChange" />
      </template>
    </PtPanel>

    <PtPanel
      v-else
      v-loading="loading"
      title="历史归档"
      icon="archive"
      :count="`${archiveTotal} 条`"
      padding="none">
      <el-table :data="archiveTorrents" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState
            :state="siteFilter ? 'zero' : 'empty'"
            dense
            sub="被删除或处理完的暂停任务会归档到这里" />
        </template>

        <el-table-column label="站点" prop="site_name" width="110">
          <template #default="{ row }">
            <PtTag>{{ row.site_name || "-" }}</PtTag>
          </template>
        </el-table-column>
        <el-table-column label="标题" min-width="240" class-name="pt-cell-strong">
          <template #default="{ row }">
            <el-tooltip :content="row.title" placement="top" :show-after="500">
              <span class="title-text">{{ row.title }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="96">
          <template #default="{ row }">
            <PtStatusPill :tone="row.is_completed ? 'ok' : 'warn'" size="sm">
              {{ row.is_completed ? "已完成" : "未完成" }}
            </PtStatusPill>
          </template>
        </el-table-column>
        <el-table-column
          label="进度"
          width="90"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatProgress(row.progress) }}</template>
        </el-table-column>
        <el-table-column label="下载器" width="110" class-name="pt-cell-muted">
          <template #default="{ row }">{{ row.downloader_name || "-" }}</template>
        </el-table-column>
        <el-table-column label="暂停原因" min-width="140" class-name="pt-cell-muted">
          <template #default="{ row }">{{ row.pause_reason || "-" }}</template>
        </el-table-column>
        <el-table-column label="归档时间" width="150" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatTime(row.archived_at) }}</template>
        </el-table-column>
      </el-table>

      <template v-if="archiveTotal > 0" #footer>
        <span class="pt-foot-note">归档只作记录，不再占用下载器</span>
        <el-pagination
          v-model:current-page="archivePage"
          v-model:page-size="archivePageSize"
          class="pt-pager"
          :page-sizes="[10, 20, 50, 100]"
          :total="archiveTotal"
          :pager-count="5"
          layout="sizes, prev, pager, next"
          @size-change="handleArchiveSizeChange"
          @current-change="handleArchivePageChange" />
      </template>
    </PtPanel>

    <el-dialog
      v-model="deleteDialogVisible"
      class="pt-dialog"
      title="删除确认"
      width="440px"
      align-center>
      <p class="confirm-text">
        确定删除任务 <strong>{{ deleteTarget?.title }}</strong> 吗？
      </p>
      <div class="pt-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>「仅删除任务」只从下载器移除，已下载的数据文件保留在磁盘上。</span>
      </div>

      <template #footer>
        <el-button @click="deleteDialogVisible = false">取消</el-button>
        <el-button type="danger" plain @click="confirmDeleteRow(true)">
          <PtIcon name="trash-2" :size="14" /><span>同时删除数据</span>
        </el-button>
        <el-button type="primary" @click="confirmDeleteRow(false)">仅删除任务</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.paused-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.filter-select {
  width: 150px;
}

/* 开关 + 文字算一个整体控件，点文字也能切；label 天然带这个行为 */
.ctl {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  user-select: none;
}

.sel-count {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.progress-cell {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.progress-info {
  display: flex;
  gap: var(--pt-space-2);
  justify-content: space-between;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

.progress-pct {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

@media (max-width: 768px) {
  .filter-select {
    width: 100%;
  }
}
</style>

<style>
/* 对话框 teleport 到 body，scoped 选择器到不了，这两条只作用于本页的确认框 */
.pt-dialog .confirm-text {
  margin: 0 0 var(--pt-space-3);
  line-height: var(--pt-lh-body);
}

.pt-dialog .confirm-text strong {
  font-weight: 600;
  color: var(--pt-t1);
  word-break: break-all;
}
</style>
