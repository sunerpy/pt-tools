<script setup lang="ts">
import { type TaskItem, type TaskListResponse, tasksApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const isMobile = useIsMobile();
const tasks = ref<TaskItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);

const filters = ref({
  q: "",
  site: "",
  downloaded: false,
  pushed: true,
  expired: false,
});
const selectedIds = ref<number[]>([]);

// 站点列表（从任务中提取）
const siteOptions = computed(() => {
  const sites = new Set<string>();
  tasks.value.forEach((t) => {
    if (t.siteName) sites.add(t.siteName);
  });
  return Array.from(sites);
});

/** 空表要分「还没跑过任务」和「筛掉了」两种：前者要去建 RSS，后者要放宽条件 */
const hasFilters = computed(() => {
  const f = filters.value;
  return Boolean(f.q || f.site || f.downloaded || f.pushed || f.expired);
});

/**
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— 两秒后 toast 消失，
 * 表格停在 empty 上，用户看到的是「还没有数据」。请求失败必须留在页面上，
 * 而且 401/403 要画成「无权访问」，否则用户会一直点重试。
 */
const { loading, state, errorText, run } = useDataState({ filtered: () => hasFilters.value });

const emptySub = computed(() =>
  hasFilters.value ? "换个关键词，或点重置清掉所有筛选" : "RSS 任务跑过之后这里会出现记录",
);

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  return emptySub.value;
});

onMounted(async () => {
  await loadTasks();
});

async function loadTasks() {
  const params = new URLSearchParams();
  params.set("page", page.value.toString());
  params.set("page_size", pageSize.value.toString());
  if (filters.value.q) params.set("q", filters.value.q);
  if (filters.value.site) params.set("site", filters.value.site);
  if (filters.value.downloaded) params.set("downloaded", "1");
  if (filters.value.pushed) params.set("pushed", "1");
  if (filters.value.expired) params.set("expired", "1");

  const data = await run<TaskListResponse>(() => tasksApi.list(params));
  if (!data) {
    // 失败时清空列表：留着上一次的数据配一个「加载失败」的空态更让人误解
    tasks.value = [];
    total.value = 0;
    return;
  }
  tasks.value = data.items || [];
  total.value = data.total || 0;
}

function applyFilters() {
  page.value = 1;
  loadTasks();
}

function clearFilters() {
  filters.value = { q: "", site: "", downloaded: false, pushed: false, expired: false };
  page.value = 1;
  loadTasks();
}

function handlePageChange(newPage: number) {
  page.value = newPage;
  loadTasks();
}

function handleSizeChange(newSize: number) {
  pageSize.value = newSize;
  page.value = 1;
  loadTasks();
}

function handleSelectionChange(selection: TaskItem[]) {
  selectedIds.value = selection.map((t) => t.id);
}

/** 移动端行卡上的勾选。el-table 的多选是它自己管的，卡片这边自己维护同一份 id 列表 */
function toggleSelect(task: TaskItem) {
  if (task.isPushed) return; // 与桌面 :selectable 一致：已推送的不可删，也就不可选
  const i = selectedIds.value.indexOf(task.id);
  if (i === -1) selectedIds.value.push(task.id);
  else selectedIds.value.splice(i, 1);
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return;

  try {
    await ElMessageBox.confirm(`确认删除 ${selectedIds.value.length} 条记录吗？`, "警告", {
      type: "warning",
      confirmButtonText: "确定",
      cancelButtonText: "取消",
    });

    const res = await tasksApi.batchDelete(selectedIds.value);
    if (res.failed > 0) {
      ElMessage.warning(`删除完成：成功 ${res.success} 条，失败 ${res.failed} 条`);
    } else {
      ElMessage.success(`成功删除 ${res.success} 条记录`);
    }
    selectedIds.value = [];
    loadTasks();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "批量删除失败");
    }
  }
}

function formatTime(timeStr: string): string {
  if (!timeStr || timeStr === "0001-01-01T00:00:00Z") return "-";
  try {
    return new Date(timeStr).toLocaleString("zh-CN");
  } catch {
    return timeStr;
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

/* el-progress 把 color 写成内联 background-color，所以这里可以直接给 var()，
   进度条就跟着当前配色走，不再是三个写死的 Element 默认色 */
function getProgressColor(progress: number): string {
  if (progress < 30) return "var(--pt-dang)";
  if (progress < 70) return "var(--pt-warn)";
  return "var(--pt-ok)";
}

function getDownloadedSize(task: TaskItem): string {
  const downloaded = task.torrentSize * (task.progress / 100);
  return formatSize(downloaded);
}

function formatProgress(progress: number): string {
  return `${progress.toFixed(1)}%`;
}

/** 已删除走中性灰：它是终态但不是失败，用 info 会和「无需处理」撞色 */
function getStatusTone(task: TaskItem): Tone {
  if (task.lastError === "种子已从下载器中删除") return "neutral";
  if (task.isExpired) return "dang";
  if (task.isPushed) return "ok";
  if (task.isDownloaded) return "warn";
  return "info";
}

function getStatusText(task: TaskItem): string {
  if (task.lastError === "种子已从下载器中删除") return "已删除";
  if (task.isExpired) return "已过期";
  if (task.isPushed) return "已推送";
  if (task.isDownloaded) return "已下载";
  return "无需处理";
}

/**
 * 优惠等级的语义色：免费是「白拿」→ ok，打折是「还要付一部分」→ warn，
 * 上传加成不省流量 → info，普通没有优惠 → 中性灰。
 */
function getDiscount(task: TaskItem): { text: string; tone: Tone } {
  const level = (task.freeLevel || "").toUpperCase();

  switch (level) {
    case "2XFREE":
    case "_2X_FREE":
      return { text: "2xFree", tone: "ok" };
    case "FREE":
      return { text: "Free", tone: "ok" };
    case "PERCENT_50":
    case "50%":
      return { text: "50%", tone: "warn" };
    case "PERCENT_30":
    case "30%":
      return { text: "30%", tone: "warn" };
    case "PERCENT_70":
    case "70%":
      return { text: "70%", tone: "warn" };
    case "2XUP":
    case "_2X_UP":
      return { text: "2xUp", tone: "info" };
    case "2X50":
    case "_2X_PERCENT_50":
      return { text: "2x50%", tone: "warn" };
    case "NONE":
    case "":
      return { text: "普通", tone: "neutral" };
    default:
      if (task.isFree) {
        return { text: "Free", tone: "ok" };
      }
      if (level) {
        return { text: level, tone: "warn" };
      }
      return { text: "普通", tone: "neutral" };
  }
}
</script>

<template>
  <div class="tasklist-page">
    <PtToolbar standalone>
      <el-input
        v-model="filters.q"
        placeholder="搜索标题 / Hash"
        clearable
        class="filter-input"
        @keyup.enter="applyFilters">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
      <el-select v-model="filters.site" placeholder="全部站点" clearable class="filter-select">
        <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
      </el-select>
      <el-checkbox v-model="filters.downloaded">已下载</el-checkbox>
      <el-checkbox v-model="filters.pushed">已推送</el-checkbox>
      <el-checkbox v-model="filters.expired">已过期</el-checkbox>

      <template #right>
        <el-button @click="clearFilters">
          <PtIcon name="rotate-ccw" :size="14" /><span>重置</span>
        </el-button>
        <el-button type="primary" :loading="loading" @click="applyFilters">
          <PtIcon name="list-filter" :size="14" /><span>筛选</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtPanel
      v-loading="loading"
      title="任务记录"
      icon="list-checks"
      :count="`${total} 条`"
      padding="none">
      <template #actions>
        <el-button
          v-if="selectedIds.length > 0"
          type="danger"
          size="small"
          plain
          @click="handleBatchDelete">
          <PtIcon name="trash-2" :size="14" /><span>删除 {{ selectedIds.length }} 条</span>
        </el-button>
        <el-button size="small" @click="loadTasks">
          <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
        </el-button>
      </template>

      <el-table
        v-if="!isMobile"
        :data="tasks"
        class="pt-grid"
        style="width: 100%"
        @selection-change="handleSelectionChange">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadTasks">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column
          type="selection"
          width="46"
          :selectable="(row: Record<string, any>) => !row.isPushed" />

        <el-table-column label="站点" prop="siteName" width="110">
          <template #default="{ row }">
            <PtTag>{{ row.siteName || "-" }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column label="优惠" width="86">
          <template #default="{ row }">
            <PtStatusPill :tone="getDiscount(row).tone" size="sm">
              {{ getDiscount(row).text }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="标题" min-width="280" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="title-cell">
              <span class="title-text">{{ row.title || "-" }}</span>
              <span v-if="row.category || row.tag" class="title-meta">
                <PtTag v-if="row.category">{{ row.category }}</PtTag>
                <PtTag v-if="row.tag">{{ row.tag }}</PtTag>
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Hash" width="120">
          <template #default="{ row }">
            <el-tooltip v-if="row.torrentHash" :content="row.torrentHash" placement="top">
              <code class="hash-cell">{{ row.torrentHash.slice(0, 8) }}</code>
            </el-tooltip>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column
          label="大小"
          width="96"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatSize(row.torrentSize) }}</template>
        </el-table-column>

        <el-table-column label="进度" width="170">
          <template #default="{ row }">
            <div v-if="row.torrentSize > 0" class="progress-cell">
              <el-progress
                :percentage="Math.round(row.progress)"
                :stroke-width="4"
                :show-text="false"
                :color="getProgressColor(row.progress)" />
              <span class="progress-info">
                <span>{{ getDownloadedSize(row) }} / {{ formatSize(row.torrentSize) }}</span>
                <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
              </span>
            </div>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column label="免费结束" width="150">
          <template #default="{ row }">
            <span :class="row.isExpired ? 'cell-dang' : 'cell-mute'">
              {{ formatTime(row.freeEndTime) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="最后检查" width="150" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatTime(row.lastCheckTime) }}</template>
        </el-table-column>

        <el-table-column label="推送时间" width="150">
          <template #default="{ row }">
            <span v-if="row.isPushed" class="cell-ok">{{ formatTime(row.pushTime) }}</span>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="96" fixed="right">
          <template #default="{ row }">
            <PtStatusPill :tone="getStatusTone(row)" size="sm">
              {{ getStatusText(row) }}
            </PtStatusPill>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 10 列，手机上横着滚既看不到列头也和页面纵向滚动打架。
        卡上留的是真正要看的：标题 + 站点/优惠/大小/时间 + 进度 + 状态。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!tasks.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="loadTasks">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="task in tasks" :key="task.id">
          <template #lead>
            <el-checkbox
              :model-value="selectedIds.includes(task.id)"
              :disabled="task.isPushed"
              :aria-label="`选择 ${task.title}`"
              @update:model-value="toggleSelect(task)" />
          </template>

          <template #title>{{ task.title || "-" }}</template>

          <template #meta>
            <PtTag>{{ task.siteName || "-" }}</PtTag>
            <PtStatusPill :tone="getDiscount(task).tone" size="sm">
              {{ getDiscount(task).text }}
            </PtStatusPill>
            <span>{{ formatSize(task.torrentSize) }}</span>
            <span :class="task.isExpired ? 'cell-dang' : undefined">
              <PtIcon name="clock" :size="11" />
              {{ formatTime(task.freeEndTime) }}
            </span>
          </template>

          <template #status>
            <PtStatusPill :tone="getStatusTone(task)" size="sm">
              {{ getStatusText(task) }}
            </PtStatusPill>
          </template>

          <template v-if="task.torrentSize > 0" #progress>
            <el-progress
              :percentage="Math.round(task.progress)"
              :stroke-width="4"
              :show-text="false"
              :color="getProgressColor(task.progress)" />
            <span class="progress-info">
              <span>{{ getDownloadedSize(task) }} / {{ formatSize(task.torrentSize) }}</span>
              <span class="progress-pct">{{ formatProgress(task.progress) }}</span>
            </span>
          </template>
        </PtRowCard>
      </div>

      <template v-if="total > 0" #footer>
        <span class="pt-foot-note">仅未推送的记录可以删除</span>
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          class="pt-pager"
          :page-sizes="[10, 20, 50, 100]"
          :total="total"
          :pager-count="5"
          layout="sizes, prev, pager, next"
          @size-change="handleSizeChange"
          @current-change="handlePageChange" />
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
.tasklist-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.filter-input {
  flex: 1 1 200px;
  max-width: 280px;
}

.filter-select {
  width: 150px;
}

/* 标题与分类标签同一行，标题占满剩余宽度后再省略号 */
.title-cell {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.title-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title-meta {
  display: flex;
  flex: 0 0 auto;
  gap: 4px;
}

.hash-cell {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  cursor: help;
}

/* 进度条 + 一行说明压在 34 高的行里：4px 细条 + 11 号说明刚好 */
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

.cell-mute {
  color: var(--pt-t2);
}

.cell-dim {
  color: var(--pt-t4);
}

.cell-ok {
  color: var(--pt-ok);
}

.cell-dang {
  color: var(--pt-dang);
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}

@media (max-width: 768px) {
  .filter-input,
  .filter-select {
    max-width: none;
    width: 100%;
  }
}
</style>
