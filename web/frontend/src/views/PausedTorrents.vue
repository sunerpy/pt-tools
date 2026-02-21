<script setup lang="ts">
import { globalApi, type ArchiveTorrent, type PausedTorrent, pausedTorrentsApi } from "@/api";
import { Delete, InfoFilled, Refresh, Timer } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

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

function getProgressColor(percentage: number) {
  if (percentage < 30) return "#F56C6C";
  if (percentage < 70) return "#E6A23C";
  return "#67C23A";
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
  <div class="page-container paused-torrents-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">暂停任务管理</h1>
        <p class="page-subtitle">管理 RSS 订阅中因免费期结束而自动暂停的下载任务</p>
      </div>
      <div class="page-actions">
        <el-tooltip
          content="开启后，免费期结束时未完成的种子将自动从下载器删除（含数据文件）"
          placement="bottom">
          <div class="auto-refresh-switch control-pill">
            <el-switch
              v-model="autoDeleteOnFreeEnd"
              :loading="savingAutoDelete"
              :active-icon="Delete"
              :inactive-icon="Delete"
              style="--el-switch-on-color: var(--el-color-danger)"
              @change="toggleAutoDelete" />
            <span class="control-pill-label">自动删除</span>
          </div>
        </el-tooltip>
        <el-tooltip content="每 30 秒自动刷新列表数据" placement="bottom">
          <div class="auto-refresh-switch control-pill">
            <el-switch
              v-model="autoRefresh"
              :active-icon="Timer"
              :inactive-icon="Timer"
              style="--el-switch-on-color: var(--pt-color-success)" />
            <span class="control-pill-label">自动刷新</span>
          </div>
        </el-tooltip>
        <el-select
          v-model="siteFilter"
          class="site-filter-select"
          placeholder="全部站点"
          clearable
          @change="handleTabChange(activeTab)">
          <el-option label="全部站点" value="" />
          <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
        </el-select>
        <el-button
          type="primary"
          class="refresh-button"
          :icon="Refresh"
          :loading="loading"
          @click="handleTabChange(activeTab)">
          刷新
        </el-button>
      </div>
    </div>

    <div class="table-card">
      <el-tabs v-model="activeTab" class="custom-tabs" @tab-change="handleTabChange">
        <el-tab-pane label="暂停中" name="paused">
          <div v-if="selectedIds.length > 0" class="filter-bar batch-action-bar">
            <div class="filter-group">
              <span class="filter-group-label">已选择 {{ selectedIds.length }} 项</span>
              <el-button
                type="danger"
                plain
                size="small"
                class="batch-action-button"
                @click="deleteTorrents(selectedIds, false)">
                删除任务
              </el-button>
              <el-button
                type="danger"
                size="small"
                class="batch-action-button"
                @click="deleteTorrents(selectedIds, true)">
                删除任务和数据
              </el-button>
            </div>
          </div>

          <div class="table-wrapper">
            <el-table
              v-loading="loading"
              :data="pausedTorrents"
              class="pt-table paused-table"
              style="width: 100%"
              @selection-change="handleSelectionChange">
              <el-table-column type="selection" width="50" />
              <el-table-column label="站点" prop="site_name" min-width="128">
                <template #default="{ row }">
                  <el-tag size="small" effect="plain" class="status-badge status-badge--info">
                    {{ row.site_name }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="标题" min-width="250">
                <template #default="{ row }">
                  <el-tooltip :content="row.title" placement="top" :show-after="500">
                    <span class="table-cell-primary title-text">{{ row.title }}</span>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column label="进度" width="220">
                <template #default="{ row }">
                  <div class="progress-container">
                    <el-progress
                      :percentage="Math.round(row.progress)"
                      :stroke-width="10"
                      :show-text="false"
                      :color="getProgressColor"
                      class="custom-progress" />
                    <div class="progress-info">
                      <span class="progress-detail-text">
                        {{ getDownloadedSize(row) }} / {{ formatSize(row.torrent_size) }}
                      </span>
                      <span class="progress-percentage">
                        {{ formatProgress(row.progress) }}
                      </span>
                    </div>
                  </div>
                </template>
              </el-table-column>
              <el-table-column label="大小" width="100" align="right">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ formatSize(row.torrent_size) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="下载器" width="120">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ row.downloader_name || "-" }}</span>
                </template>
              </el-table-column>
              <el-table-column label="暂停原因" min-width="150">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ row.pause_reason || "-" }}</span>
                </template>
              </el-table-column>
              <el-table-column label="暂停时间" width="160">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ formatTime(row.paused_at) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="160" fixed="right">
                <template #default="{ row }">
                  <div class="table-cell-actions">
                    <el-button
                      link
                      type="primary"
                      size="small"
                      class="action-button"
                      @click="resumeTorrent(row)">
                      恢复
                    </el-button>
                    <el-button
                      link
                      type="danger"
                      size="small"
                      class="action-button"
                      @click="openDeleteDialog(row)">
                      删除
                    </el-button>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div class="pagination-container">
            <el-pagination
              v-if="pausedTotal > 0"
              v-model:current-page="pausedPage"
              v-model:page-size="pausedPageSize"
              :page-sizes="[10, 20, 50, 100]"
              :total="pausedTotal"
              layout="total, sizes, prev, pager, next, jumper"
              @size-change="handlePausedSizeChange"
              @current-change="handlePausedPageChange" />
          </div>

          <div v-if="!loading && pausedTorrents.length === 0" class="table-empty">
            <el-empty description="暂无暂停任务" />
          </div>
        </el-tab-pane>

        <el-tab-pane label="历史归档" name="archive">
          <div class="table-wrapper">
            <el-table
              v-loading="loading"
              :data="archiveTorrents"
              class="pt-table archive-table"
              style="width: 100%">
              <el-table-column label="站点" prop="site_name" min-width="128">
                <template #default="{ row }">
                  <el-tag size="small" effect="plain" class="status-badge status-badge--info">
                    {{ row.site_name }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="标题" min-width="250">
                <template #default="{ row }">
                  <el-tooltip :content="row.title" placement="top" :show-after="500">
                    <span class="table-cell-primary title-text">{{ row.title }}</span>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <el-tag
                    :type="row.is_completed ? 'success' : 'warning'"
                    size="small"
                    effect="light">
                    {{ row.is_completed ? "已完成" : "未完成" }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="进度" width="100">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ formatProgress(row.progress) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="下载器" width="120">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ row.downloader_name || "-" }}</span>
                </template>
              </el-table-column>
              <el-table-column label="暂停原因" min-width="150">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ row.pause_reason || "-" }}</span>
                </template>
              </el-table-column>
              <el-table-column label="归档时间" width="160">
                <template #default="{ row }">
                  <span class="table-cell-secondary">{{ formatTime(row.archived_at) }}</span>
                </template>
              </el-table-column>
            </el-table>
          </div>

          <div class="pagination-container">
            <el-pagination
              v-if="archiveTotal > 0"
              v-model:current-page="archivePage"
              v-model:page-size="archivePageSize"
              :page-sizes="[10, 20, 50, 100]"
              :total="archiveTotal"
              layout="total, sizes, prev, pager, next, jumper"
              @size-change="handleArchiveSizeChange"
              @current-change="handleArchivePageChange" />
          </div>

          <div v-if="!loading && archiveTorrents.length === 0" class="table-empty">
            <el-empty description="暂无归档记录" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- Delete Confirmation Dialog -->
    <el-dialog v-model="deleteDialogVisible" title="删除确认" width="450px" align-center>
      <div class="delete-confirm-content">
        <p class="confirm-text">
          确定要删除任务
          <span class="highlight-text">{{ deleteTarget?.title }}</span>
          吗？
        </p>
        <div class="confirm-tip">
          <el-icon><InfoFilled /></el-icon>
          <span>默认操作仅删除下载器中的任务，保留已下载的数据文件。</span>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer-custom">
          <el-button @click="deleteDialogVisible = false">取消</el-button>
          <div class="action-buttons">
            <el-button type="danger" plain @click="confirmDeleteRow(true)">同时删除数据</el-button>
            <el-button type="primary" @click="confirmDeleteRow(false)">仅删除任务</el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";
@import "@/styles/paused-torrents-page.css";
</style>
