<script setup lang="ts">
import { globalApi, type ArchiveTorrent, type PausedTorrent, pausedTorrentsApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox, type TableInstance } from "element-plus";
import { computed, onMounted, onUnmounted, ref, watch } from "vue";

const isMobile = useIsMobile();
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

/** 两个 tab 共用同一个站点筛选，0 行时用它区分「库里没有」和「筛掉了」 */
const hasFilters = computed(() => Boolean(siteFilter.value));

/**
 * 六态状态机（设计文档 §5），两个 tab 各一份。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— toast 两秒后消失，
 * 表格停在 empty 上，用户看到的是「还没有暂停任务」，而真相是请求失败了。
 * 401/403 也必须画成「无权访问」，否则用户会一直点重试。
 *
 * partial 在这一页拿不到：每个 tab 只有一个数据源，没有「部分站点失败」的概念，
 * 所以 failed 恒为 0。状态 key 照样整份透给 PtDataState，不做裁剪。
 */
const {
  loading: pausedLoading,
  state: pausedState,
  errorText: pausedErrorText,
  run: runPaused,
} = useDataState({ filtered: () => hasFilters.value });

const {
  loading: archiveLoading,
  state: archiveState,
  errorText: archiveErrorText,
  run: runArchive,
} = useDataState({ filtered: () => hasFilters.value });

/** 工具条上的刷新按钮不关心是哪个 tab 在加载 */

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const pausedStateSub = computed(() => {
  if (pausedState.value === "error" || pausedState.value === "perm") return pausedErrorText.value;
  return hasFilters.value
    ? "这个站点下没有暂停中的种子，换一个站点或清掉筛选"
    : "免费期结束时被暂停的种子会出现在这里";
});

const archiveStateSub = computed(() => {
  if (archiveState.value === "error" || archiveState.value === "perm")
    return archiveErrorText.value;
  return hasFilters.value
    ? "这个站点下没有归档记录，换一个站点或清掉筛选"
    : "被删除或处理完的暂停任务会归档到这里";
});

/*
 * 画板 head 的 sub —— 标题下面那行实时摘要（11.5/400 t3），由本页算出真实数字
 * 再 Teleport 进外壳页头。稿上的口径是「总量 + 分项」（例：「37 个任务 ·
 * 12 下载中 · 19 做种中」），这一页能拿到的分项只有当前页那一批行，
 * 所以合计明确写成「本页」，不把一页的和冒充成全量。拿不到数据时给空串。
 */
const pausedHeadSub = computed(() => {
  const rows = pausedTorrents.value;
  if (!rows.length) return "";
  const sites = new Set(rows.map((t) => t.site_name).filter(Boolean)).size;
  const bytes = rows.reduce((sum, t) => sum + (t.torrent_size || 0), 0);
  const done = rows.filter((t) => t.progress >= 100).length;
  const parts = [
    `${pausedTotal.value} 个暂停任务`,
    `本页 ${rows.length} 条`,
    `${sites} 个站点`,
    `本页合计 ${formatSize(bytes)}`,
  ];
  if (done > 0) parts.push(`${done} 个已下完`);
  if (siteFilter.value) parts.push(`筛选 ${siteFilter.value}`);
  return parts.join(" · ");
});

const archiveHeadSub = computed(() => {
  const rows = archiveTorrents.value;
  if (!rows.length) return "";
  const done = rows.filter((t) => t.is_completed).length;
  const parts = [
    `${archiveTotal.value} 条归档`,
    `本页 ${rows.length} 条`,
    `${done} 已完成`,
    `${rows.length - done} 未完成`,
  ];
  if (siteFilter.value) parts.push(`筛选 ${siteFilter.value}`);
  return parts.join(" · ");
});

/**
 * 画板 17 两个列表同时可见，所以摘要把两边并成一行 ——
 * 之前是按 tab 二选一，现在没有 tab 了。
 */
const headSub = computed(() => {
  const parts = [pausedHeadSub.value, archiveHeadSub.value].filter(Boolean);
  return parts.join(" ｜ ");
});

/** 画板 gfoot 的左侧文案：「N 个任务 · 显示 a–b · 每页 c」 */
function rangeNote(total: number, page: number, size: number, unit: string): string {
  const from = (page - 1) * size + 1;
  const to = Math.min(page * size, total);
  return `${total} ${unit} · 显示 ${from}–${to} · 每页 ${size}`;
}

const pausedFootNote = computed(() =>
  rangeNote(pausedTotal.value, pausedPage.value, pausedPageSize.value, "个任务"),
);

const archiveFootNote = computed(() =>
  rangeNote(archiveTotal.value, archivePage.value, archivePageSize.value, "条记录"),
);

/** 工具栏右侧那行说明字（画板 note 11/400 t3），两个 tab 各一句 */
onMounted(async () => {
  // 画板 17 两个列表同时可见，所以首屏两边都要拉
  await Promise.all([loadPausedTorrents(), loadArchiveTorrents()]);
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
      // 两个列表都在页上，所以都要刷
      loadPausedTorrents();
      loadArchiveTorrents();
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
  const data = await runPaused(() =>
    pausedTorrentsApi.list(pausedPage.value, pausedPageSize.value, siteFilter.value || undefined),
  );
  if (!data) {
    // 失败时清空列表：留着上一次的数据配一个「加载失败」的空态更让人误解
    pausedTorrents.value = [];
    pausedTotal.value = 0;
    selectedIds.value = [];
    return;
  }
  pausedTorrents.value = data.items || [];
  pausedTotal.value = data.total || 0;
  pruneSelection();
}

async function loadArchiveTorrents() {
  const data = await runArchive(() =>
    pausedTorrentsApi.listArchive(
      archivePage.value,
      archivePageSize.value,
      siteFilter.value || undefined,
    ),
  );
  if (!data) {
    archiveTorrents.value = [];
    archiveTotal.value = 0;
    return;
  }
  archiveTorrents.value = data.items || [];
  archiveTotal.value = data.total || 0;
}

/**
 * 桌面上换页/刷新时 el-table 会自己清掉勾选，行卡这边没有表格代管，
 * 于是手动剔掉已经不在当前列表里的 id，免得批量删除打到看不见的行上。
 */
function pruneSelection() {
  if (selectedIds.value.length === 0) return;
  const ids = new Set(pausedTorrents.value.map((t) => t.id));
  selectedIds.value = selectedIds.value.filter((id) => ids.has(id));
}

/*
 * 多选条上的「取消选择」要同时清掉 el-table 自己那份勾选状态，
 * 只把 selectedIds 置空的话，表格里的复选框还是勾着的。
 */
const pausedTableRef = ref<TableInstance>();

function clearSelection() {
  pausedTableRef.value?.clearSelection();
  selectedIds.value = [];
}

/**
 * 两个列表一起重新加载（换站点筛选、点刷新都走这里）。
 * 先清勾选：重载后表格会重挂、它内部的勾选随之丢失，不清掉 selectedIds
 * 批量删除就会打在一批已经看不见的行上。
 */
function reloadAll() {
  clearSelection();
  loadPausedTorrents();
  loadArchiveTorrents();
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

/** 移动端行卡上的勾选。el-table 的多选是它自己管的，卡片这边自己维护同一份 id 列表 */
function toggleSelect(torrent: PausedTorrent) {
  const i = selectedIds.value.indexOf(torrent.id);
  if (i === -1) selectedIds.value.push(torrent.id);
  else selectedIds.value.splice(i, 1);
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
    <!-- 画板 head 的 sub：标题下面那行实时摘要，由本页把真实数字送进外壳页头 -->
    <PtHeadSub v-if="headSub">{{ headSub }}</PtHeadSub>

    <!--
    画板 17 没有工具栏带，所以站点筛选与两个页面级开关都进页头动作区（高 32）。
    窄屏下外壳把页头整条隐掉，这时 Teleport 关闭、控件就地留在卡片上方。
    -->
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-select
        v-model="siteFilter"
        placeholder="全部站点"
        clearable
        class="filter-select"
        @change="reloadAll">
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
    </Teleport>

    <!--
      画板 17 的主区是 head 之后**两张并列的通栏卡**（c-paused 1080×412 /
      c-archive 1080×254）—— 不是带式表格页，也不是两个 tab：两个列表同时可见。
      之前按「通用带式构成」实现是照猜测做的（当时画板 17 还没读到），现在按画板改回卡片。
    -->
    <div class="pt-cards pt-cards--wide">
      <PtPanel
        v-loading="pausedLoading"
        title="暂停中"
        icon="circle-pause"
        :count="pausedTotal"
        padding="none">
        <!-- roomy：进度列是「进度条 + 一行数字」的双行内容，34px 行高会裁掉下面那行 -->
        <el-table
          v-if="!isMobile"
          ref="pausedTableRef"
          :data="pausedTorrents"
          class="pt-grid pt-grid--roomy"
          style="width: 100%"
          @selection-change="handleSelectionChange">
          <template #empty>
            <PtDataState :state="pausedState" dense :sub="pausedStateSub">
              <template v-if="pausedState === 'error'" #action>
                <el-button size="small" @click="loadPausedTorrents">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column type="selection" width="46" />
          <el-table-column label="站点" prop="site_name" width="110">
            <template #default="{ row }">
              <span class="pt-cell-site">{{ row.site_name || "-" }}</span>
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

        <!--
          移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
          这张表桌面有 8 列，手机上横着滚既看不到列头，又和页面纵向滚动打架。
          卡上留的是真正要看的：标题 + 站点/大小/下载器/暂停时间 + 进度 + 恢复/删除。
        -->
        <div v-else class="cards">
          <PtDataState v-if="!pausedTorrents.length" :state="pausedState" :sub="pausedStateSub">
            <template v-if="pausedState === 'error'" #action>
              <el-button size="small" @click="loadPausedTorrents">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in pausedTorrents" :key="row.id">
            <template #lead>
              <el-checkbox
                :model-value="selectedIds.includes(row.id)"
                :aria-label="`选择 ${row.title}`"
                @update:model-value="toggleSelect(row)" />
            </template>

            <template #title>{{ row.title || "-" }}</template>

            <template #meta>
              <span class="pt-cell-site">{{ row.site_name || "-" }}</span>
              <span>{{ formatSize(row.torrent_size) }}</span>
              <span>
                <PtIcon name="hard-drive" :size="11" />
                {{ row.downloader_name || "-" }}
              </span>
              <span>
                <PtIcon name="clock" :size="11" />
                {{ formatTime(row.paused_at) }}
              </span>
            </template>

            <!-- 暂停原因跟着桌面走中性灰（那一列是 pt-cell-muted），不新造语义色 -->
            <template v-if="row.pause_reason" #status>
              <PtTag>
                <span class="reason-cap">{{ row.pause_reason }}</span>
              </PtTag>
            </template>

            <template #progress>
              <el-progress
                :percentage="Math.round(row.progress)"
                :stroke-width="4"
                :show-text="false"
                :color="getProgressColor(row.progress)" />
              <span class="progress-info">
                <span>{{ getDownloadedSize(row) }} / {{ formatSize(row.torrent_size) }}</span>
                <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
              </span>
            </template>

            <template #actions>
              <el-button size="small" @click="resumeTorrent(row)">
                <PtIcon name="play" :size="14" /><span>恢复</span>
              </el-button>
              <el-button type="danger" size="small" plain @click="openDeleteDialog(row)">
                <PtIcon name="trash-2" :size="14" /><span>删除</span>
              </el-button>
            </template>
          </PtRowCard>
        </div>

        <!-- 画板 selband 44：多选时列表下方那条强调色 6% 的横幅，批量操作放在这里 -->
        <div v-if="selectedIds.length > 0" class="pt-band--sel">
          <PtIcon name="square-check" :size="15" />
          <span>已选 {{ selectedIds.length }} 项</span>
          <span class="pt-band__spacer" />
          <el-button @click="clearSelection">
            <PtIcon name="x" :size="14" /><span>取消选择</span>
          </el-button>
          <el-button type="danger" plain @click="deleteTorrents(selectedIds, false)">
            <PtIcon name="trash-2" :size="14" /><span>删除任务</span>
          </el-button>
          <el-button type="danger" @click="deleteTorrents(selectedIds, true)">
            <PtIcon name="trash-2" :size="14" /><span>连数据一起删</span>
          </el-button>
        </div>

        <template v-if="pausedTotal > 0" #footer>
          <span class="pt-foot-note">{{ pausedFootNote }}</span>
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
        v-loading="archiveLoading"
        title="历史归档"
        icon="archive"
        :count="archiveTotal"
        padding="none">
        <el-table v-if="!isMobile" :data="archiveTorrents" class="pt-grid" style="width: 100%">
          <template #empty>
            <PtDataState :state="archiveState" dense :sub="archiveStateSub">
              <template v-if="archiveState === 'error'" #action>
                <el-button size="small" @click="loadArchiveTorrents">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column label="站点" prop="site_name" width="110">
            <template #default="{ row }">
              <span class="pt-cell-site">{{ row.site_name || "-" }}</span>
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

        <!-- 归档是只读列表：卡上没有操作，状态给完成与否，进度只有百分比（归档记录不带体积） -->
        <div v-else class="cards">
          <PtDataState v-if="!archiveTorrents.length" :state="archiveState" :sub="archiveStateSub">
            <template v-if="archiveState === 'error'" #action>
              <el-button size="small" @click="loadArchiveTorrents">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in archiveTorrents" :key="row.id">
            <template #title>{{ row.title || "-" }}</template>

            <template #meta>
              <span class="pt-cell-site">{{ row.site_name || "-" }}</span>
              <span>
                <PtIcon name="hard-drive" :size="11" />
                {{ row.downloader_name || "-" }}
              </span>
              <span>
                <PtIcon name="clock" :size="11" />
                {{ formatTime(row.archived_at) }}
              </span>
              <span v-if="row.pause_reason">{{ row.pause_reason }}</span>
            </template>

            <template #status>
              <PtStatusPill :tone="row.is_completed ? 'ok' : 'warn'" size="sm">
                {{ row.is_completed ? "已完成" : "未完成" }}
              </PtStatusPill>
            </template>

            <template #progress>
              <el-progress
                :percentage="Math.round(row.progress)"
                :stroke-width="4"
                :show-text="false"
                :color="getProgressColor(row.progress)" />
              <span class="progress-info">
                <span>完成进度</span>
                <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
              </span>
            </template>
          </PtRowCard>
        </div>

        <template v-if="archiveTotal > 0" #footer>
          <span class="pt-foot-note">{{ archiveFootNote }}</span>
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
    </div>

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
/*
 * 画板主区是一串全宽横向带（工具栏 40 → 表格 → 多选条 44 → 页脚带 34），
 * 带与带之间靠各自的发丝线收口，没有间距，所以这里不给 gap。
 * 需要留白的只有移动端行卡列表，它自己内缩 16。
 */
.paused-page {
  display: flex;
  flex-direction: column;
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

/* 移动端行卡列表：表格带本身不留边，卡片列表自己内缩 16（画板卡片层的口径） */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad);
}

/* 多选条里的按钮由 .pt-band--sel 统一压到 30 高，图标与文字要跟着居中 */
.pt-band--sel > .pt-icon {
  flex: 0 0 auto;
  color: var(--pt-p);
}

/* 暂停原因挂在卡片右上角，长文案要收住，否则会把标题挤成一条 */
.reason-cap {
  display: block;
  max-width: 32vw;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .filter-select {
    width: 100%;
  }

  /*
   * 两条带在窄屏都放不下一行：多选条是「已选 N 项 + 三个按钮」，页脚带是
   * 「口径说明 + 分页器」。共享件里两条都是 nowrap（按 1112 宽的画板定的），
   * 这里只在本页放开换行，不改公共样式。
   */
  .pt-band--sel,
  .pt-band--foot {
    flex-wrap: wrap;
    padding: var(--pt-space-2) var(--pt-space-3);
  }

  /* 换行后右侧那组不再需要被顶开，spacer 收掉免得单独占一行 */
  .pt-band--sel .pt-band__spacer,
  .pt-band--foot .pt-band__spacer {
    flex: 0 0 0;
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
