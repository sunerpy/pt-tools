<script setup lang="ts">
/*
 * 整理历史（路线图 M10）：每个视频文件一条记录，写明整理到哪里、用什么方式、成功还是失败。
 * 失败的可以重试（整个种子重新整理）；删除记录时可以连库里整理出的文件一起删。
 * 画板没有这一页，沿用列表页的样式：页头 + 筛选 + 表格，手机是行卡。
 */
import { type MediaHistoryItem, type MediaTransferStatus, organizeApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes, formatShortDateTime } from "@/utils/format";
import {
  HISTORY_FILTERS,
  historyEntry,
  historyStatus,
  modeLabel,
  triggerLabel,
} from "@/utils/media";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

const PAGE_SIZE = 50;
const items = ref<MediaHistoryItem[]>([]);
const total = ref(0);
const page = ref(1);
const status = ref<"" | MediaTransferStatus>("");
const keyword = ref("");
const ds = useDataState({ filtered: () => Boolean(status.value || keyword.value.trim()) });

async function load() {
  const pending = ds.run(() =>
    organizeApi.history({
      status: status.value,
      q: keyword.value.trim(),
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
    }),
  );
  const data = await pending;
  if (ds.isStale(pending) || !data) return;
  items.value = data.items;
  total.value = data.total;
}

function search() {
  page.value = 1;
  void load();
}

const fileName = (p: string) => p.split(/[/\\]/).pop() || p;

const retrying = ref<number | null>(null);
async function retry(r: MediaHistoryItem) {
  retrying.value = r.id;
  try {
    const res = await organizeApi.retry(r.id);
    if (res.queued) ElMessage.info("整理还在后台进行，稍后刷新看结果");
    else if (res.plan?.problem) ElMessage.warning(res.plan.problem);
    else if (res.failed) ElMessage.warning(`新入库 ${res.created} 个，失败 ${res.failed} 个`);
    else ElMessage.success(`新入库 ${res.created} 个，已在库里 ${res.done} 个`);
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "重试失败"));
  } finally {
    retrying.value = null;
  }
}

// ---- 删除 ----
const delDialog = ref(false);
const deleting = ref(false);
const delTarget = ref<MediaHistoryItem | null>(null);
const delFiles = ref(false);
const canDeleteFiles = computed(
  () =>
    delTarget.value?.status === "done" &&
    delTarget.value.mode !== "move" &&
    Boolean(delTarget.value.target_path),
);

function askDelete(r: MediaHistoryItem) {
  delTarget.value = r;
  delFiles.value = false;
  delDialog.value = true;
}

async function confirmDelete() {
  if (!delTarget.value) return;
  deleting.value = true;
  try {
    await organizeApi.deleteHistory(delTarget.value.id, delFiles.value && canDeleteFiles.value);
    ElMessage.success(delFiles.value ? "已删除记录与库里的文件" : "已删除记录");
    delDialog.value = false;
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  } finally {
    deleting.value = false;
  }
}

const canRetry = (r: MediaHistoryItem) => r.status === "failed" || r.status === "skipped";

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>每个视频文件一条记录：整理到哪里、用什么方式、成功还是失败</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="mh-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtPanel title="整理历史" icon="history" :count="total">
      <div class="mh-bar">
        <el-radio-group v-model="status" size="small" data-testid="mh-status" @change="search">
          <el-radio-button v-for="f in HISTORY_FILTERS" :key="f.value || 'all'" :value="f.value">{{
            f.label
          }}</el-radio-button>
        </el-radio-group>
        <el-input
          v-model="keyword"
          class="mh-search"
          clearable
          placeholder="种子名、标题或路径"
          data-testid="mh-keyword"
          @keyup.enter="search"
          @clear="search">
          <template #prefix><PtIcon name="search" :size="14" /></template>
        </el-input>
      </div>

      <div
        v-if="items.length && ds.error.value"
        class="pt-note pt-note--warn mh-stale"
        data-testid="mh-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!items.length"
        :state="ds.state.value"
        :title="ds.state.value === 'empty' ? '还没有整理记录' : ''"
        :sub="
          ds.errorText.value ||
          (ds.state.value === 'empty'
            ? '在「媒体库」里打开自动整理，或在任务列表、下载器 Web UI 里手动整理'
            : '')
        "
        data-testid="mh-state" />
      <el-table
        v-else-if="!isMobile"
        :data="items"
        row-key="id"
        class="pt-grid"
        data-testid="mh-table">
        <el-table-column label="时间" width="110">
          <template #default="{ row }">{{ formatShortDateTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column
          label="条目"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <div class="mh-strong">{{ historyEntry(row) || "没识别出来" }}</div>
            <div class="mh-sub">{{ row.torrent_name }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="文件"
          min-width="260"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ fileName(row.source_path) }}</div>
            <div v-if="row.target_path" class="mh-sub">
              → {{ row.target_path
              }}<template v-if="row.subtitles"> · {{ row.subtitles }} 个字幕</template>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="方式" width="110">
          <template #default="{ row }">
            <div>{{ modeLabel(row.mode) }}</div>
            <div class="mh-sub">{{ row.library_name || "" }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="状态"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <PtStatusPill :tone="historyStatus(row.status).tone" size="sm">{{
              historyStatus(row.status).label
            }}</PtStatusPill>
            <span class="mh-sub mh-trigger">{{ triggerLabel(row.trigger) }}</span>
            <div v-if="row.message" class="mh-sub">{{ row.message }}</div>
            <div v-if="row.next_retry_at" class="mh-sub">
              {{ formatShortDateTime(row.next_retry_at) }} 自动重试（第 {{ row.attempts }} 次失败）
            </div>
          </template>
        </el-table-column>
        <el-table-column label="" width="120" align="right">
          <template #default="{ row }">
            <el-button
              v-if="canRetry(row)"
              link
              :loading="retrying === row.id"
              :data-testid="`mh-retry-${row.id}`"
              @click="retry(row)"
              >重试</el-button
            >
            <el-button link type="danger" :data-testid="`mh-del-${row.id}`" @click="askDelete(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="mh-cards">
        <PtRowCard v-for="r in items" :key="r.id">
          <template #title>{{ historyEntry(r) || "没识别出来" }}</template>
          <template #meta>
            <span class="mh-full">{{ fileName(r.source_path) }}</span>
            <span v-if="r.target_path" class="mh-full">→ {{ r.target_path }}</span>
            <span
              >{{ modeLabel(r.mode) }} · {{ triggerLabel(r.trigger) }} ·
              {{ formatShortDateTime(r.updated_at) }}</span
            >
            <span v-if="r.size">{{ formatBytes(r.size) }}</span>
            <span v-if="r.message" class="mh-full">{{ r.message }}</span>
          </template>
          <template #status>
            <PtStatusPill dot :tone="historyStatus(r.status).tone" size="sm">{{
              historyStatus(r.status).label
            }}</PtStatusPill>
          </template>
          <template #actions>
            <el-button
              v-if="canRetry(r)"
              size="small"
              :loading="retrying === r.id"
              @click="retry(r)"
              >重试</el-button
            >
            <el-button size="small" type="danger" plain @click="askDelete(r)">删除</el-button>
          </template>
        </PtRowCard>
      </div>

      <div v-if="total > PAGE_SIZE" class="mh-pager">
        <el-pagination
          v-model:current-page="page"
          class="pt-pager"
          :page-size="PAGE_SIZE"
          :total="total"
          :pager-count="5"
          layout="prev, pager, next"
          @current-change="load" />
      </div>
    </PtPanel>

    <el-dialog
      v-model="delDialog"
      class="pt-dialog"
      title="删除整理记录"
      width="460px"
      align-center>
      <p class="mh-del-text">
        删除「{{
          delTarget ? fileName(delTarget.source_path) : ""
        }}」的整理记录？下载目录里的文件不动。
      </p>
      <el-checkbox v-model="delFiles" :disabled="!canDeleteFiles" data-testid="mh-del-files"
        >同时删除库里整理出的文件（视频、字幕、同名的 NFO 与图片）</el-checkbox
      >
      <p v-if="delTarget?.mode === 'move'" class="mh-sub mh-del-text">
        移动整理的文件是唯一的一份，这里不删。
      </p>
      <p v-else-if="!canDeleteFiles" class="mh-sub mh-del-text">这条记录没有整理出文件。</p>
      <template #footer>
        <el-button @click="delDialog = false">取消</el-button>
        <el-button
          type="danger"
          :loading="deleting"
          data-testid="mh-del-confirm"
          @click="confirmDelete"
          >删除</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.mh-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}

.mh-search {
  flex: 1 1 220px;
  max-width: 360px;
}

.mh-stale {
  margin-bottom: 12px;
}

.mh-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.mh-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.mh-trigger {
  margin-left: 6px;
}

.mh-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mh-full {
  flex-basis: 100%;
  word-break: break-all;
}

.mh-pager {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
}

.mh-del-text {
  margin: 0 0 10px;
  font-size: 13px;
  line-height: 1.6;
  word-break: break-all;
}
</style>
